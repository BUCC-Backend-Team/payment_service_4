package api

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/BUCC-Backend-Team/payment_service_4/internal/database"
	apperrors "github.com/BUCC-Backend-Team/payment_service_4/internal/errors"
	"github.com/BUCC-Backend-Team/payment_service_4/internal/payments"
	"github.com/BUCC-Backend-Team/payment_service_4/internal/refunds"
	"github.com/BUCC-Backend-Team/payment_service_4/internal/webhooks"
)

func MerchantGuard(expected string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			provided, ok := bearerToken(r.Header.Get("Authorization"))
			if !ok || subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
				payments.WriteError(w, apperrors.New(http.StatusUnauthorized, "unauthorized", "Authorization: Bearer <MERCHANT_API_KEY> required"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func bearerToken(header string) (string, bool) {
	rest, found := strings.CutPrefix(header, "Bearer ")
	if !found {
		rest, found = strings.CutPrefix(header, "bearer ")
	}
	if !found || strings.TrimSpace(rest) == "" {
		return "", false
	}
	return strings.TrimSpace(rest), true
}

func Health() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}
}

func Reset(store *database.Store, testMode bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !testMode {
			payments.WriteError(w, apperrors.New(http.StatusForbidden, "test_mode_only", "POST /admin/reset requires TEST_MODE=true"))
			return
		}
		if err := store.ResetAll(r.Context()); err != nil {
			payments.WriteError(w, apperrors.New(http.StatusInternalServerError, "reset_failed", "Failed to reset data"))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func Routes(store *database.Store, cfg Config, paymentService *payments.Service, refundService *refunds.Service, webhookService *webhooks.Service) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", Health())
	mux.HandleFunc("POST /webhooks/paystack", webhookService.Handle)
	mux.HandleFunc("POST /admin/reset", Reset(store, cfg.TestMode))

	guard := MerchantGuard(cfg.MerchantAPIKey)
	paymentsHandler := payments.NewHandler(paymentService)
	refundsHandler := refunds.NewHandler(refundService)

	mux.Handle("POST /payments", guard(http.HandlerFunc(paymentsHandler.Create)))
	mux.Handle("GET /payments/{id}", guard(http.HandlerFunc(paymentsHandler.Get)))
	mux.Handle("POST /payments/{id}/refunds", guard(http.HandlerFunc(refundsHandler.Create)))

	return mux
}
