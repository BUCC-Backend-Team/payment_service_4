package refunds

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"

	apperrors "github.com/BUCC-Backend-Team/payment_service_4/internal/errors"
	"github.com/BUCC-Backend-Team/payment_service_4/src/db"
	"github.com/BUCC-Backend-Team/payment_service_4/internal/payments"
	"github.com/BUCC-Backend-Team/payment_service_4/internal/providers"
)

const (
	StatusPending   = "pending"
	StatusProcessed = "processed"
	StatusFailed    = "failed"

	refundCallWait = 10 * time.Second
	submitAttempts = 6
)

type Result struct {
	RefundID string `json:"refundId"`
	Amount   int64  `json:"amount"`
	Status   string `json:"status"`
}

type Service struct {
	store    *db.Store
	provider providers.Provider
	payments *payments.Service
	log      *slog.Logger
}

func NewService(store *db.Store, provider providers.Provider, paymentService *payments.Service, log *slog.Logger) *Service {
	return &Service{
		store:    store,
		provider: provider,
		payments: paymentService,
		log:      log,
	}
}


func (s *Service) Create(ctx context.Context, paymentID uuid.UUID, idemKey string, amount int64) (Result, bool, error) {
	if idemKey == "" {
		return Result{}, false, apperrors.New(400, "missing_idempotency_key", "Idempotency-Key header is required")
	}
	if amount < 0 {
		return Result{}, false, apperrors.New(400, "invalid_amount", "amount must be a positive integer of kobo")
	}

	bodyHash := requestHash(strconv.FormatInt(amount, 10))
	key := db.RefundScope + idemKey

	rec, err := s.store.GetIdempotencyRecord(ctx, key)
	if err == nil {
		if rec.RequestHash != bodyHash {
			return Result{}, false, apperrors.New(422, "key_reuse_mismatch", "Idempotency-Key reused with a different body")
		}
		if rec.ResponseBody != "" {
			var saved Result
			if err := json.Unmarshal([]byte(rec.ResponseBody), &saved); err != nil {
				return Result{}, false, fmt.Errorf("refunds: decode stored response: %w", err)
			}
			return saved, true, nil
		}
	} else if !errors.Is(err, db.ErrNotFound) {
		return Result{}, false, fmt.Errorf("refunds: read idempotency record: %w", err)
	}

	reserved, err := s.store.ReserveIdempotencyKey(ctx, key, bodyHash)
	if err != nil {
		return Result{}, false, fmt.Errorf("refunds: reserve idempotency key: %w", err)
	}
	if !reserved {
		return s.waitForWinner(ctx, key, bodyHash)
	}

	result, err := s.insertRefund(ctx, paymentID, idemKey, amount, key)
	if err != nil {
		if appErr, ok := err.(*apperrors.AppError); ok && appErr.HTTPStatus() != 500 {
			_ = s.store.DeleteIdempotencyKey(ctx, key)
		}
		return Result{}, false, err
	}
	return result, false, nil
}

func (s *Service) insertRefund(ctx context.Context, paymentID uuid.UUID, idemKey string, requested int64, key string) (Result, error) {
	tx, err := s.store.BeginTx(ctx, nil)
	if err != nil {
		return Result{}, fmt.Errorf("refunds: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	txStore := db.NewStore(tx)

	payment, err := txStore.GetPaymentByIDForUpdate(ctx, paymentID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return Result{}, apperrors.New(404, "payment_not_found", "Payment not found")
		}
		return Result{}, fmt.Errorf("refunds: lock payment: %w", err)
	}

	if payment.Status != payments.StatusSucceeded && payment.Status != payments.StatusPartiallyRefunded {
		return Result{}, apperrors.New(409, "not_refundable", "Payment is not in a refundable state")
	}

	activeTotal, err := txStore.SumActiveRefunds(ctx, payment.ID)
	if err != nil {
		return Result{}, fmt.Errorf("refunds: sum active refunds: %w", err)
	}

	remaining := payment.Amount - activeTotal
	if remaining <= 0 {
		return Result{}, apperrors.New(409, "not_refundable", "There is nothing left to refund")
	}

	amount := requested
	if amount == 0 {
		amount = remaining
	}
	if amount > remaining {
		return Result{}, apperrors.New(409, "not_refundable", "Requested refund exceeds the remaining balance")
	}

	refundID := uuid.New()
	if _, err := txStore.CreateRefund(ctx, db.Refund{
		ID:        refundID,
		PaymentID: payment.ID,
		Amount:    amount,
		Status:    StatusPending,
	}); err != nil {
		return Result{}, fmt.Errorf("refunds: create refund: %w", err)
	}

	result := Result{
		RefundID: refundID.String(),
		Amount:   amount,
		Status:   StatusPending,
	}

	raw, err := json.Marshal(result)
	if err != nil {
		return Result{}, fmt.Errorf("refunds: encode response: %w", err)
	}
	if err := txStore.StoreIdempotencyResponse(ctx, key, 202, string(raw)); err != nil {
		return Result{}, fmt.Errorf("refunds: store idempotency response: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Result{}, fmt.Errorf("refunds: commit: %w", err)
	}

	go s.submit(context.WithoutCancel(ctx), refundID, payment.Reference, amount)

	return result, nil
}

func (s *Service) waitForWinner(ctx context.Context, key, bodyHash string) (Result, bool, error) {
	deadline := time.Now().Add(refundCallWait)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return Result{}, false, ctx.Err()
		case <-time.After(40 * time.Millisecond):
		}

		rec, err := s.store.GetIdempotencyRecord(ctx, key)
		if err == nil {
			if rec.RequestHash != bodyHash {
				return Result{}, false, apperrors.New(422, "key_reuse_mismatch", "Idempotency-Key reused with a different body")
			}
			if rec.ResponseBody != "" {
				var saved Result
				if err := json.Unmarshal([]byte(rec.ResponseBody), &saved); err != nil {
					return Result{}, false, fmt.Errorf("refunds: decode stored response: %w", err)
				}
				return saved, true, nil
			}
		} else if !errors.Is(err, db.ErrNotFound) {
			return Result{}, false, fmt.Errorf("refunds: poll idempotency record: %w", err)
		}
	}
	return Result{}, false, apperrors.New(409, "idempotency_in_progress", "A request with this Idempotency-Key is still in flight")
}

func requestHash(amount string) string {
	sum := sha256.Sum256([]byte("refund|" + amount))
	return hex.EncodeToString(sum[:])
}

