package refunds

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/BUCC-Backend-Team/payment_service_4/internal/database"
	apperrors "github.com/BUCC-Backend-Team/payment_service_4/internal/errors"
	"github.com/BUCC-Backend-Team/payment_service_4/internal/payments"
	"github.com/BUCC-Backend-Team/payment_service_4/internal/providers"
)

const (
	StatusPending   = "pending"
	StatusProcessed = "processed"
	StatusFailed    = "failed"

	refundCallWait    = 10 * time.Second
	refundPollEvery   = 40 * time.Millisecond
	submitAttempts    = 6
	submitBackoffBase = 1 * time.Second
)

type Result struct {
	RefundID string `json:"refundId"`
	Amount   int64  `json:"amount"`
	Status   string `json:"status"`
}

type Service struct {
	store    *database.Store
	provider providers.Provider
	payments *payments.Service
	log      *slog.Logger
}

func NewService(store *database.Store, provider providers.Provider, paymentService *payments.Service, log *slog.Logger) *Service {
	return &Service{
		store:    store,
		provider: provider,
		payments: paymentService,
		log:      log,
	}
}

func RefundScope(idempotencyKey string) string {
	return "refund:" + idempotencyKey
}

func (s *Service) Create(ctx context.Context, paymentID uuid.UUID, idemKey string, amount int64) (Result, bool, error) {
	if idemKey == "" {
		return Result{}, false, apperrors.New(http.StatusBadRequest, "missing_idempotency_key", "Idempotency-Key header is required")
	}
	if amount < 0 {
		return Result{}, false, apperrors.New(http.StatusBadRequest, "invalid_amount", "amount must be a positive integer of kobo")
	}

	bodyHash := requestHash(strconv.FormatInt(amount, 10))
	key := RefundScope(idemKey)

	rec, err := s.store.GetIdempotencyRecord(ctx, key)
	if err == nil {
		if rec.RequestHash != bodyHash {
			return Result{}, false, apperrors.New(http.StatusUnprocessableEntity, "key_reuse_mismatch", "Idempotency-Key reused with a different body")
		}
		if rec.ResponseBody != "" {
			var saved Result
			if err := json.Unmarshal([]byte(rec.ResponseBody), &saved); err != nil {
				return Result{}, false, fmt.Errorf("refunds: decode stored response: %w", err)
			}
			return saved, true, nil
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Result{}, false, fmt.Errorf("refunds: read idempotency record: %w", err)
	}

	rows, err := s.store.ReserveIdempotencyKey(ctx, database.ReserveIdempotencyKeyParams{
		Key:         key,
		RequestHash: bodyHash,
	})
	if err != nil {
		return Result{}, false, fmt.Errorf("refunds: reserve idempotency key: %w", err)
	}
	if rows == 0 {
		return s.waitForWinner(ctx, key, bodyHash)
	}

	result, err := s.insertRefund(ctx, paymentID, key, amount)
	if err != nil {
		if appErr, ok := err.(*apperrors.AppError); ok && appErr.HTTPStatus() != http.StatusInternalServerError && appErr.HTTPStatus() != http.StatusNotFound {
			_ = s.store.DeleteIdempotencyKey(ctx, key)
		}
		return Result{}, false, err
	}
	return result, false, nil
}

func (s *Service) insertRefund(ctx context.Context, paymentID uuid.UUID, key string, requested int64) (Result, error) {
	tx, err := s.store.BeginTx(ctx, nil)
	if err != nil {
		return Result{}, fmt.Errorf("refunds: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	txStore := s.store.WithTx(tx)

	payment, err := txStore.GetPaymentByIDForUpdate(ctx, paymentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Result{}, apperrors.New(http.StatusNotFound, "payment_not_found", "Payment not found")
		}
		return Result{}, fmt.Errorf("refunds: lock payment: %w", err)
	}

	if payment.Status != payments.StatusSucceeded && payment.Status != payments.StatusPartiallyRefunded {
		return Result{}, apperrors.New(http.StatusConflict, "not_refundable", "Payment is not in a refundable state")
	}

	activeTotal, err := txStore.SumActiveRefunds(ctx, payment.ID)
	if err != nil {
		return Result{}, fmt.Errorf("refunds: sum active refunds: %w", err)
	}

	remaining := payment.Amount - activeTotal
	if remaining <= 0 {
		return Result{}, apperrors.New(http.StatusConflict, "not_refundable", "There is nothing left to refund")
	}

	amount := requested
	if amount == 0 {
		amount = remaining
	}
	if amount > remaining {
		return Result{}, apperrors.New(http.StatusConflict, "not_refundable", "Requested refund exceeds the remaining balance")
	}

	refundID := uuid.New()
	if _, err := txStore.CreateRefund(ctx, database.CreateRefundParams{
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
	if err := txStore.StoreIdempotencyResponse(ctx, database.StoreIdempotencyResponseParams{
		Key:            key,
		ResponseBody:   string(raw),
		ResponseStatus: sql.NullInt32{Int32: http.StatusAccepted, Valid: true},
	}); err != nil {
		return Result{}, fmt.Errorf("refunds: store idempotency response: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Result{}, fmt.Errorf("refunds: commit: %w", err)
	}

	go s.submit(context.WithoutCancel(ctx), refundID, payment.Reference.String(), amount)

	return result, nil
}

func (s *Service) waitForWinner(ctx context.Context, key, bodyHash string) (Result, bool, error) {
	deadline := time.Now().Add(refundCallWait)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return Result{}, false, ctx.Err()
		case <-time.After(refundPollEvery):
		}

		rec, err := s.store.GetIdempotencyRecord(ctx, key)
		if err == nil {
			if rec.RequestHash != bodyHash {
				return Result{}, false, apperrors.New(http.StatusUnprocessableEntity, "key_reuse_mismatch", "Idempotency-Key reused with a different body")
			}
			if rec.ResponseBody != "" {
				var saved Result
				if err := json.Unmarshal([]byte(rec.ResponseBody), &saved); err != nil {
					return Result{}, false, fmt.Errorf("refunds: decode stored response: %w", err)
				}
				return saved, true, nil
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return Result{}, false, fmt.Errorf("refunds: poll idempotency record: %w", err)
		}
	}
	return Result{}, false, apperrors.New(http.StatusConflict, "idempotency_in_progress", "A request with this Idempotency-Key is still in flight")
}

func requestHash(amount string) string {
	sum := sha256.Sum256([]byte("refund|" + amount))
	return hex.EncodeToString(sum[:])
}
