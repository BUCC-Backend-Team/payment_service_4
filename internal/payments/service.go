package payments

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
	"github.com/lib/pq"

	"github.com/BUCC-Backend-Team/payment_service_4/internal/callbacks"
	"github.com/BUCC-Backend-Team/payment_service_4/internal/database"
	apperrors "github.com/BUCC-Backend-Team/payment_service_4/internal/errors"
	"github.com/BUCC-Backend-Team/payment_service_4/internal/providers"
)

const (
	StatusPending           = "pending"
	StatusSucceeded         = "succeeded"
	StatusExpired           = "expired"
	StatusPartiallyRefunded = "partially_refunded"
	StatusRefunded          = "refunded"

	defaultTimeoutSeconds = 1800
	waiterInterval        = 40 * time.Millisecond
	idempotencyWaitLimit  = 10 * time.Second
)

type CreateRequest struct {
	OrderID       string
	Amount        int64
	Currency      string
	CustomerEmail string
	ReturnURL     string
}

type CreateResult struct {
	PaymentID   string `json:"paymentId"`
	OrderID     string `json:"orderId"`
	Status      string `json:"status"`
	Amount      int64  `json:"amount"`
	Currency    string `json:"currency"`
	CheckoutURL string `json:"checkoutUrl"`
}

type Payment struct {
	ID             uuid.UUID `json:"id"`
	OrderID        string    `json:"orderId"`
	Amount         int64     `json:"amount"`
	Currency       string    `json:"currency"`
	Status         string    `json:"status"`
	Channel        string    `json:"channel"`
	AmountRefunded int64     `json:"amountRefunded"`
	CreatedAt      time.Time `json:"createdAt"`
}

type Service struct {
	store    *database.Store
	provider providers.Provider
	log      *slog.Logger
	timeout  time.Duration
}

func NewService(store *database.Store, provider providers.Provider, log *slog.Logger, paymentTimeout time.Duration) *Service {
	if paymentTimeout <= 0 {
		paymentTimeout = defaultTimeoutSeconds * time.Second
	}
	return &Service{
		store:    store,
		provider: provider,
		log:      log,
		timeout:  paymentTimeout,
	}
}

func PaymentScope(idempotencyKey string) string {
	return "payment:" + idempotencyKey
}

func (s *Service) Create(ctx context.Context, req CreateRequest, idemKey string) (CreateResult, bool, error) {
	rec, ok, err := s.consumeIdempotency(ctx, req, idemKey)
	if err != nil {
		return CreateResult{}, false, err
	}
	if ok {
		return rec.Value, rec.Replayed, nil
	}

	created, err := s.createPayment(ctx, req, idemKey)
	if err != nil {
		return CreateResult{}, false, err
	}
	return created, false, nil
}

type idempotencyReplay struct {
	Value    CreateResult
	Replayed bool
}

func (s *Service) consumeIdempotency(ctx context.Context, req CreateRequest, idemKey string) (idempotencyReplay, bool, error) {
	bodyHash := requestHash(req.OrderID, strconv.FormatInt(req.Amount, 10), req.Currency, req.CustomerEmail, req.ReturnURL)
	key := PaymentScope(idemKey)

	var zero idempotencyReplay
	record, err := s.store.GetIdempotencyRecord(ctx, key)
	if err == nil {
		if record.RequestHash != bodyHash {
			return zero, false, apperrors.New(http.StatusUnprocessableEntity, "key_reuse_mismatch", "Idempotency-Key reused with a different body")
		}
		if record.ResponseBody != "" {
			var saved CreateResult
			if err := json.Unmarshal([]byte(record.ResponseBody), &saved); err != nil {
				return zero, false, fmt.Errorf("payments: decode stored response: %w", err)
			}
			return idempotencyReplay{Value: saved, Replayed: true}, true, nil
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return zero, false, fmt.Errorf("payments: read idempotency record: %w", err)
	}

	rows, err := s.store.ReserveIdempotencyKey(ctx, database.ReserveIdempotencyKeyParams{
		Key:         key,
		RequestHash: bodyHash,
	})
	if err != nil {
		return zero, false, fmt.Errorf("payments: reserve idempotency key: %w", err)
	}
	if rows > 0 {
		return zero, false, nil
	}

	winner, found, err := s.waitForKey(ctx, key, bodyHash, idempotencyWaitLimit)
	if err != nil {
		return zero, false, err
	}
	if found {
		var saved CreateResult
		if err := json.Unmarshal([]byte(winner.ResponseBody), &saved); err != nil {
			return zero, false, fmt.Errorf("payments: decode stored response: %w", err)
		}
		return idempotencyReplay{Value: saved, Replayed: true}, true, nil
	}

	return zero, false, apperrors.New(http.StatusConflict, "idempotency_in_progress", "A request with this Idempotency-Key is still in flight")
}

func (s *Service) waitForKey(ctx context.Context, key, bodyHash string, timeout time.Duration) (database.GetIdempotencyRecordRow, bool, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return database.GetIdempotencyRecordRow{}, false, ctx.Err()
		case <-time.After(waiterInterval):
		}

		record, err := s.store.GetIdempotencyRecord(ctx, key)
		if err == nil {
			if record.RequestHash != bodyHash {
				return record, false, apperrors.New(http.StatusUnprocessableEntity, "key_reuse_mismatch", "Idempotency-Key reused with a different body")
			}
			if record.ResponseBody != "" {
				return record, true, nil
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return database.GetIdempotencyRecordRow{}, false, fmt.Errorf("payments: poll idempotency record: %w", err)
		}
	}

	return database.GetIdempotencyRecordRow{}, false, nil
}

func (s *Service) createPayment(ctx context.Context, req CreateRequest, idemKey string) (CreateResult, error) {
	paymentID := uuid.New()

	created, err := s.store.CreatePayment(ctx, database.CreatePaymentParams{
		ID:            paymentID,
		OrderID:       req.OrderID,
		Amount:        req.Amount,
		Currency:      req.Currency,
		CustomerEmail: req.CustomerEmail,
		ReturnUrl:     req.ReturnURL,
		Status:        StatusPending,
		Reference:     paymentID,
		ExpiresAt:     sql.NullTime{Time: time.Now().UTC().Add(s.timeout), Valid: true},
	})
	if err != nil {
		if isUniqueViolation(err) {
			_ = s.store.DeleteIdempotencyKey(ctx, PaymentScope(idemKey))
			return CreateResult{}, apperrors.New(http.StatusConflict, "payment_exists", "A payment already exists for this order")
		}
		return CreateResult{}, fmt.Errorf("payments: create payment: %w", err)
	}

	return s.initialize(ctx, created, idemKey)
}

func (s *Service) initialize(ctx context.Context, payment database.Payment, idemKey string) (CreateResult, error) {
	checkout, err := s.provider.InitializeTransaction(ctx, providers.InitializeRequest{
		Reference:   payment.Reference.String(),
		Amount:      payment.Amount,
		Currency:    payment.Currency,
		Email:       payment.CustomerEmail,
		CallbackURL: payment.ReturnUrl,
	})
	if err != nil {
		return CreateResult{}, fmt.Errorf("payments: initialize transaction: %w", err)
	}

	if err := s.store.UpdatePaymentCheckout(ctx, database.UpdatePaymentCheckoutParams{
		ID:          payment.ID,
		CheckoutUrl: sql.NullString{String: checkout.AuthorizationURL, Valid: true},
	}); err != nil {
		return CreateResult{}, fmt.Errorf("payments: save checkout url: %w", err)
	}

	result := CreateResult{
		PaymentID:   payment.ID.String(),
		OrderID:     payment.OrderID,
		Status:      payment.Status,
		Amount:      payment.Amount,
		Currency:    payment.Currency,
		CheckoutURL: checkout.AuthorizationURL,
	}

	raw, err := json.Marshal(result)
	if err != nil {
		return CreateResult{}, fmt.Errorf("payments: encode response: %w", err)
	}
	if err := s.store.StoreIdempotencyResponse(ctx, database.StoreIdempotencyResponseParams{
		Key:            PaymentScope(idemKey),
		ResponseBody:   string(raw),
		ResponseStatus: sql.NullInt32{Int32: http.StatusCreated, Valid: true},
	}); err != nil {
		return CreateResult{}, fmt.Errorf("payments: store idempotency response: %w", err)
	}

	return result, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Payment, error) {
	row, err := s.store.GetPaymentByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Payment{}, apperrors.New(http.StatusNotFound, "payment_not_found", "Payment not found")
		}
		return Payment{}, fmt.Errorf("payments: get payment: %w", err)
	}

	refunded, err := s.store.SumProcessedRefunds(ctx, row.ID)
	if err != nil {
		return Payment{}, fmt.Errorf("payments: sum processed refunds: %w", err)
	}

	return Payment{
		ID:             row.ID,
		OrderID:        row.OrderID,
		Amount:         row.Amount,
		Currency:       row.Currency,
		Status:         row.Status,
		Channel:        row.Channel.String,
		AmountRefunded: refunded,
		CreatedAt:      row.CreatedAt,
	}, nil
}

func (s *Service) MarkSucceeded(ctx context.Context, txStore *database.Queries, payment database.Payment, channel string, late bool) error {
	if payment.Status == StatusSucceeded || payment.Status == StatusPartiallyRefunded || payment.Status == StatusRefunded {
		return nil
	}

	if err := txStore.MarkPaymentSucceeded(ctx, database.MarkPaymentSucceededParams{
		ID:      payment.ID,
		Late:    late,
		Channel: sql.NullString{String: channel, Valid: channel != ""},
	}); err != nil {
		return fmt.Errorf("payments: mark succeeded: %w", err)
	}

	return callbacks.Enqueue(ctx, txStore, callbacks.EventPaymentSucceeded, callbacks.EventData{
		PaymentID:      payment.ID.String(),
		OrderID:        payment.OrderID,
		Status:         StatusSucceeded,
		Amount:         payment.Amount,
		AmountRefunded: 0,
		Currency:       payment.Currency,
		Late:           late,
	})
}

func (s *Service) MarkExpired(ctx context.Context, txStore *database.Queries, payment database.Payment) error {
	if payment.Status != StatusPending {
		return nil
	}

	if err := txStore.MarkPaymentExpired(ctx, payment.ID); err != nil {
		return fmt.Errorf("payments: mark expired: %w", err)
	}

	return callbacks.Enqueue(ctx, txStore, callbacks.EventPaymentExpired, callbacks.EventData{
		PaymentID:      payment.ID.String(),
		OrderID:        payment.OrderID,
		Status:         StatusExpired,
		Amount:         payment.Amount,
		AmountRefunded: 0,
		Currency:       payment.Currency,
		Late:           false,
	})
}

func (s *Service) ApplyRefundProcessed(ctx context.Context, txStore *database.Queries, refund database.Refund) error {
	if err := txStore.MarkRefundProcessed(ctx, database.MarkRefundProcessedParams{
		ID:      refund.ID,
		Column2: refund.PaystackRefundID.Int64,
	}); err != nil {
		return fmt.Errorf("payments: mark refund processed: %w", err)
	}

	payment, err := txStore.GetPaymentByIDForUpdate(ctx, refund.PaymentID)
	if err != nil {
		return fmt.Errorf("payments: lock payment after refund: %w", err)
	}

	total, err := txStore.SumProcessedRefunds(ctx, payment.ID)
	if err != nil {
		return fmt.Errorf("payments: sum processed refunds: %w", err)
	}

	if err := txStore.SetPaymentRefundState(ctx, database.SetPaymentRefundStateParams{
		ID:     payment.ID,
		Amount: total,
	}); err != nil {
		return fmt.Errorf("payments: set refund state: %w", err)
	}

	newStatus := StatusPartiallyRefunded
	if total >= payment.Amount {
		newStatus = StatusRefunded
	}

	return callbacks.Enqueue(ctx, txStore, callbacks.EventPaymentRefunded, callbacks.EventData{
		PaymentID:      payment.ID.String(),
		OrderID:        payment.OrderID,
		Status:         newStatus,
		Amount:         payment.Amount,
		AmountRefunded: total,
		Currency:       payment.Currency,
		Late:           payment.Late,
	})
}

func (s *Service) ApplyRefundFailed(ctx context.Context, txStore *database.Queries, refundID uuid.UUID) error {
	refund, err := txStore.GetRefundByIDForUpdate(ctx, refundID)
	if err != nil {
		return fmt.Errorf("payments: lock refund for failure: %w", err)
	}

	if err := txStore.MarkRefundFailed(ctx, refund.ID); err != nil {
		return fmt.Errorf("payments: mark refund failed: %w", err)
	}

	payment, err := txStore.GetPaymentByIDForUpdate(ctx, refund.PaymentID)
	if err != nil {
		return fmt.Errorf("payments: lock payment after failed refund: %w", err)
	}

	total, err := txStore.SumProcessedRefunds(ctx, payment.ID)
	if err != nil {
		return fmt.Errorf("payments: sum processed refunds: %w", err)
	}

	return callbacks.Enqueue(ctx, txStore, callbacks.EventPaymentRefundFailed, callbacks.EventData{
		PaymentID:      payment.ID.String(),
		OrderID:        payment.OrderID,
		Status:         payment.Status,
		Amount:         payment.Amount,
		AmountRefunded: total,
		Currency:       payment.Currency,
		Late:           payment.Late,
	})
}

func requestHash(parts ...string) string {
	joined := make([]byte, 0, 128)
	for i, part := range parts {
		if i > 0 {
			joined = append(joined, 0x1f)
		}
		joined = append(joined, part...)
	}
	sum := sha256.Sum256(joined)
	return hex.EncodeToString(sum[:])
}

func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == pqUniqueViolation
}

const pqUniqueViolation = "23505"
