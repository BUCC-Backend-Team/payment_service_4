package refunds

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"github.com/BUCC-Backend-Team/payment_service_4/internal/database"
	"github.com/BUCC-Backend-Team/payment_service_4/internal/providers"
)

func (s *Service) submit(ctx context.Context, refundID uuid.UUID, transactionReference string, amount int64) {
	outcome := providers.Refund{}
	ambiguous := false

	delay := submitBackoffBase
	for attempt := 1; attempt <= submitAttempts; attempt++ {
		if attempt > 1 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			delay *= 2
		}

		refund, err := s.provider.CreateRefund(ctx, providers.RefundRequest{
			Transaction: transactionReference,
			Amount:      amount,
		})
		if err == nil {
			outcome = refund
			ambiguous = false
			break
		}

		if providers.IsRejection(err) {
			s.failRefund(ctx, refundID, err)
			return
		}

		if attempt < submitAttempts {
			s.log.Warn("refunds: retrying Paystack refund",
				"refundId", refundID, "attempt", attempt, "error", err)
		} else {
			ambiguous = true
		}
	}

	if outcome.ID > 0 {
		if err := s.store.SetRefundPaystackID(ctx, database.SetRefundPaystackIDParams{
			ID:               refundID,
			PaystackRefundID: sql.NullInt64{Int64: outcome.ID, Valid: true},
		}); err != nil {
			s.log.Error("refunds: save paystack refund id", "refundId", refundID, "error", err)
		}
	}

	if ambiguous {
		s.log.Warn("refunds: outcome uncertain after retries; leaving pending for reconciliation", "refundId", refundID)
	}
}

func (s *Service) failRefund(ctx context.Context, refundID uuid.UUID, cause error) {
	s.log.Warn("refunds: paystack rejected refund", "refundId", refundID, "error", cause)

	tx, err := s.store.BeginTx(ctx, nil)
	if err != nil {
		s.log.Error("refunds: begin fail tx", "refundId", refundID, "error", err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	txStore := s.store.WithTx(tx)
	if _, err := txStore.GetRefundByIDForUpdate(ctx, refundID); err != nil {
		s.log.Error("refunds: cannot fail unknown refund", "refundId", refundID, "error", err)
		return
	}

	if err := s.payments.ApplyRefundFailed(ctx, txStore, refundID); err != nil {
		s.log.Error("refunds: apply refund failed state", "refundId", refundID, "error", err)
		return
	}

	if err := tx.Commit(); err != nil {
		s.log.Error("refunds: commit fail tx", "refundId", refundID, "error", err)
	}
}
