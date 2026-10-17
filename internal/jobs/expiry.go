package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/BUCC-Backend-Team/payment_service_4/src/db"
	"github.com/BUCC-Backend-Team/payment_service_4/internal/payments"
	"github.com/BUCC-Backend-Team/payment_service_4/internal/providers"
)

type Service struct {
	store        *db.Store
	provider     providers.Provider
	payments     *payments.Service
	log          *slog.Logger
	timeout      time.Duration
	expiryGrace  time.Duration
	refundGrace  time.Duration
	batchSize    int
}

func NewService(store *db.Store, provider providers.Provider, paymentService *payments.Service, log *slog.Logger, timeout time.Duration) *Service {
	return &Service{
		store:       store,
		provider:    provider,
		payments:    paymentService,
		log:         log,
		timeout:     timeout,
		expiryGrace: 0,
		refundGrace: 30 * time.Second,
		batchSize:   50,
	}
}

func (s *Service) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}

	s.runOnce(ctx)
	s.log.Info("jobs: expiry and reconciliation worker started", "interval", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runOnce(ctx)
		}
	}
}

func (s *Service) runOnce(ctx context.Context) {
	if err := s.reconcilePayments(ctx); err != nil {
		s.log.Error("jobs: payment reconciliation failed", "error", err)
	}
	if err := s.reconcileRefunds(ctx); err != nil {
		s.log.Error("jobs: refund reconciliation failed", "error", err)
	}
}


func (s *Service) reconcilePayments(ctx context.Context) error {
	rows, err := s.store.ListPendingPayments(ctx, s.batchSize)
	if err != nil {
		return fmt.Errorf("jobs: list pending payments: %w", err)
	}

	for _, payment := range rows {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		transaction, err := s.provider.VerifyTransaction(ctx, payment.Reference)
		if err != nil {
			s.log.Warn("jobs: verify payment failed", "reference", payment.Reference, "error", err)
			continue
		}

		if transaction.Status == "success" && transaction.Amount == payment.Amount && transaction.Currency == payment.Currency {
			if err := s.markSucceeded(ctx, payment.Reference, transaction.Channel, false); err != nil {
				s.log.Error("jobs: reconcile succeeded failed", "reference", payment.Reference, "error", err)
			}
			continue
		}

		if payment.ExpiresAt.Valid && !time.Now().Before(payment.ExpiresAt.Time) {
			if err := s.markExpired(ctx, payment.ID); err != nil {
				s.log.Error("jobs: expire payment failed", "reference", payment.Reference, "error", err)
			}
		}
	}

	return nil
}


func (s *Service) markSucceeded(ctx context.Context, reference, channel string, late bool) error {
	tx, err := s.store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("jobs: begin succeeded tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	txStore := db.NewStore(tx)

	payment, err := txStore.GetPaymentByReferenceForUpdate(ctx, reference)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("jobs: lock payment by reference: %w", err)
	}

	if err := s.payments.MarkSucceeded(ctx, txStore, payment, channel, late); err != nil {
		return fmt.Errorf("jobs: mark succeeded: %w", err)
	}

	return tx.Commit()
}

func (s *Service) markExpired(ctx context.Context, id uuid.UUID) error {
	tx, err := s.store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("jobs: begin expired tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	txStore := db.NewStore(tx)

	payment, err := txStore.GetPaymentByIDForUpdate(ctx, id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("jobs: lock payment by id: %w", err)
	}

	if err := s.payments.MarkExpired(ctx, txStore, payment); err != nil {
		return fmt.Errorf("jobs: mark expired: %w", err)
	}

	return tx.Commit()
}


func (s *Service) reconcileRefunds(ctx context.Context) error {
	stuck, err := s.store.ListStuckPendingRefunds(ctx, s.refundGrace, s.batchSize)
	if err != nil {
		return fmt.Errorf("jobs: list stuck pending refunds: %w", err)
	}

	for _, refund := range stuck {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := s.reconcileOneRefund(ctx, refund); err != nil {
			s.log.Error("jobs: reconcile refund failed", "refundId", refund.ID, "error", err)
		}
	}

	return nil
}

func (s *Service) reconcileOneRefund(ctx context.Context, refund db.Refund) error {
	tx, err := s.store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("jobs: begin stuck refund tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	txStore := db.NewStore(tx)
	locked, err := txStore.GetPaymentByIDForUpdate(ctx, refund.PaymentID)
	if err != nil {
		return fmt.Errorf("jobs: lock payment for refund reconcile: %w", err)
	}

	refundResult, err := s.provider.CreateRefund(ctx, providers.RefundRequest{
		Transaction: locked.Reference,
		Amount:      refund.Amount,
	})
	if err != nil {
		if providers.IsRejection(err) {
			if err := s.payments.ApplyRefundFailed(ctx, txStore, refund.ID); err != nil {
				return fmt.Errorf("jobs: mark stuck refund failed: %w", err)
			}
			return tx.Commit()
		}
		s.log.Warn("jobs: stuck refund still ambiguous, remains pending", "refundId", refund.ID)
		return nil
	}

	if refundResult.ID > 0 {
		if err := txStore.SetRefundPaystackID(ctx, refund.ID, refundResult.ID); err != nil {
			return fmt.Errorf("jobs: save paystack refund id: %w", err)
		}
	}

	return tx.Commit()
}
