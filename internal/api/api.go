package api

import (
	"crypto/subtle"
	"net/http"
	"strings"

	apperrors "github.com/BUCC-Backend-Team/payment_service_4/internal/errors"
	"github.com/BUCC-Backend-Team/payment_service_4/internal/payments"
)

// reject requests without Authorization: Bearer MERCHANT_API_KEY.
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
