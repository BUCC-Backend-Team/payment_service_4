package refunds

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"

	apperrors "github.com/BUCC-Backend-Team/payment_service_4/internal/errors"
	"github.com/BUCC-Backend-Team/payment_service_4/internal/payments"
)

type apiHandler struct {
	service *Service
}

func NewHandler(service *Service) *apiHandler {
	return &apiHandler{service: service}
}

type refundBody struct {
	Amount *int64 `json:"amount"`
}

func (h *apiHandler) Create(w http.ResponseWriter, r *http.Request) {
	idemKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idemKey == "" {
		payments.WriteError(w, apperrors.New(http.StatusBadRequest, "missing_idempotency_key", "Idempotency-Key header is required"))
		return
	}

	paymentID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		payments.WriteError(w, apperrors.New(http.StatusBadRequest, "invalid_payment_id", "Payment id must be a UUID"))
		return
	}

	var amount int64
	if r.ContentLength > 0 {
		var body refundBody
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, payments.MaxBodyBytes))
		if err := decoder.Decode(&body); err != nil {
			payments.WriteError(w, apperrors.New(http.StatusBadRequest, "invalid_request_body", "Request body must be valid JSON"))
			return
		}
		if body.Amount != nil {
			amount = *body.Amount
		}
	}

	result, replayed, createErr := h.service.Create(r.Context(), paymentID, idemKey, amount)
	if createErr != nil {
		payments.WriteError(w, createErr)
		return
	}

	headers := map[string]string{}
	if replayed {
		headers["Idempotent-Replayed"] = "true"
	}
	payments.WriteJSON(w, http.StatusAccepted, result, headers)
}
