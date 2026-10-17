package api

import (
	"net/http"

	"github.com/BUCC-Backend-Team/payment_service_4/internal/database"
	apperrors "github.com/BUCC-Backend-Team/payment_service_4/internal/errors"
	"github.com/BUCC-Backend-Team/payment_service_4/internal/payments"
)

func ResetHandler(store *db.Store, testMode bool) http.HandlerFunc {
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
