package webhooks

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/BUCC-Backend-Team/payment_service_4/internal/database"
	"github.com/BUCC-Backend-Team/payment_service_4/internal/payments"
	"github.com/BUCC-Backend-Team/payment_service_4/internal/providers"
)

const maxWebhookBody = 1 << 20

type Service struct {
	store     *database.Store
	provider  providers.Provider
	payments  *payments.Service
	log       *slog.Logger
	secretKey string
}

func NewService(store *database.Store, provider providers.Provider, paymentService *payments.Service, log *slog.Logger, secretKey string) *Service {
	return &Service{
		store:     store,
		provider:  provider,
		payments:  paymentService,
		log:       log,
		secretKey: secretKey,
	}
}

func (s *Service) Handle(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBody))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("unreadable body"))
		return
	}

	signature := r.Header.Get("x-paystack-signature")
	if !validSignature(s.secretKey, body, signature) {
		s.log.Warn("webhooks: invalid paystack signature")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid signature"))
		return
	}

	var event PaystackEvent
	if err := json.Unmarshal(body, &event); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid payload"))
		return
	}

	switch event.Event {
	case "charge.success":
		s.handleChargeSuccess(r.Context(), w, event)
	case "refund.processed":
		s.handleRefundEvent(r.Context(), w, event, true)
	case "refund.failed":
		s.handleRefundEvent(r.Context(), w, event, false)
	default:
		w.WriteHeader(http.StatusOK)
	}
}

func validSignature(secret string, body []byte, signature string) bool {
	if signature == "" || secret == "" {
		return false
	}

	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(expected), []byte(signature)) == 1
}

func (s *Service) handleChargeSuccess(ctx context.Context, w http.ResponseWriter, event PaystackEvent) {
	reference := event.Data.Reference
	if reference == "" {
		w.WriteHeader(http.StatusOK)
		return
	}

	paymentID, err := uuid.Parse(reference)
	if err != nil {
		s.log.Warn("webhooks: charge.success unknown reference", "reference", reference)
		w.WriteHeader(http.StatusOK)
		return
	}

	payment, err := s.store.GetPaymentByReference(ctx, paymentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.log.Warn("webhooks: charge.success unknown payment", "reference", reference)
			w.WriteHeader(http.StatusOK)
			return
		}
		s.log.Error("webhooks: load payment for webhook", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	transaction, err := s.provider.VerifyTransaction(ctx, reference)
	if err != nil || !matchesPayment(transaction, payment) {
		s.log.Warn("webhooks: charge.success failed verification",
			"reference", reference,
			"verified", err == nil,
			"error", err)
		w.WriteHeader(http.StatusOK)
		return
	}

	late := payment.Status == payments.StatusExpired

	tx, err := s.store.BeginTx(ctx, nil)
	if err != nil {
		s.log.Error("webhooks: begin tx", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback() }()

	txStore := s.store.WithTx(tx)

	dedup := DeduplicationKey(event)
	rows, err := txStore.RecordProcessedEvent(ctx, dedup)
	if err != nil {
		s.log.Error("webhooks: record dedup key", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if rows == 0 {
		s.log.Warn("webhooks: duplicate charge.success ignored", "reference", reference)
		w.WriteHeader(http.StatusOK)
		return
	}

	if err := s.payments.MarkSucceeded(ctx, txStore, payment, transaction.Channel, late); err != nil {
		s.log.Error("webhooks: mark payment succeeded", "error", err)
		return
	}

	if err := tx.Commit(); err != nil {
		s.log.Error("webhooks: commit succeeded tx", "error", err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Service) handleRefundEvent(ctx context.Context, w http.ResponseWriter, event PaystackEvent, processed bool) {
	reference := event.Data.TransactionReferenceOf()
	if reference == "" {
		w.WriteHeader(http.StatusOK)
		return
	}

	paymentID, err := uuid.Parse(reference)
	if err != nil {
		s.log.Warn("webhooks: refund event unknown reference", "reference", reference)
		w.WriteHeader(http.StatusOK)
		return
	}

	payment, err := s.store.GetPaymentByReference(ctx, paymentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.log.Warn("webhooks: refund event unknown payment", "reference", reference)
			w.WriteHeader(http.StatusOK)
			return
		}
		s.log.Error("webhooks: load payment for refund webhook", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	tx, err := s.store.BeginTx(ctx, nil)
	if err != nil {
		s.log.Error("webhooks: begin refund event tx", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback() }()

	txStore := s.store.WithTx(tx)

	dedup := DeduplicationKey(event)
	rows, err := txStore.RecordProcessedEvent(ctx, dedup)
	if err != nil {
		s.log.Error("webhooks: record dedup key", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if rows == 0 {
		s.log.Warn("webhooks: duplicate refund event ignored", "reference", reference)
		w.WriteHeader(http.StatusOK)
		return
	}

	active, err := txStore.ListActiveRefundsByPaymentID(ctx, payment.ID)
	if err != nil {
		s.log.Error("webhooks: list active refunds", "error", err)
		return
	}

	refund, found := matchRefund(active, event.Data)
	if !found {
		_ = tx.Rollback()
		s.log.Warn("webhooks: refund event has no matching pending refund", "reference", reference)
		w.WriteHeader(http.StatusOK)
		return
	}

	if event.Data.ID > 0 {
		refund.PaystackRefundID = sql.NullInt64{Int64: event.Data.ID, Valid: true}
	}

	var applyErr error
	if processed {
		applyErr = s.payments.ApplyRefundProcessed(ctx, txStore, refund)
	} else {
		applyErr = s.payments.ApplyRefundFailed(ctx, txStore, refund.ID)
	}
	if applyErr != nil {
		s.log.Error("webhooks: apply refund event", "error", applyErr)
		return
	}

	if err := tx.Commit(); err != nil {
		s.log.Error("webhooks: commit refund event tx", "error", err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func matchRefund(active []database.Refund, data EventData) (database.Refund, bool) {
	if data.ID > 0 {
		for _, refund := range active {
			if refund.PaystackRefundID.Valid && refund.PaystackRefundID.Int64 == data.ID {
				return refund, true
			}
		}
	}

	refundAmount := data.RefundAmount()
	for _, refund := range active {
		if refund.Status != "pending" {
			continue
		}
		if refundAmount <= 0 || refundAmount == refund.Amount {
			return refund, true
		}
	}
	return database.Refund{}, false
}

func matchesPayment(transaction providers.Transaction, payment database.Payment) bool {
	return transaction.Status == "success" &&
		transaction.Amount == payment.Amount &&
		transaction.Currency == payment.Currency
}
