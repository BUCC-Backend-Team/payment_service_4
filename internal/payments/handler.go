package payments

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

func MakePaymentHandler(w http.ResponseWriter, r *http.Request) {
	if !authenticateMerchant(w, r) {
		return
	}
	defer r.Body.Close()

	var req MakePaymentRequest
	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string {
			"error": "idempotency key not provided",
		})
		return
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid JSON request body",
		}) 
		return
	}

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, http.ErrBodyReadAfterClose) && err == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "request body must contain only one JSON object",
		})
		return
	} else if err != nil && !errors.Is(err, errors.New("EOF")) {
		if strings.TrimSpace(err.Error()) != "EOF" {
			writeJSON(w, http.StatusBadRequest, map[string]string {
				"error": "request body must contain only one JSON object",
			})
			return
		}
	}

	req.IdempotencyKey = idempotencyKey

	if strings.TrimSpace(req.OrderID) == "" ||
		strings.TrimSpace(req.CustomerEmail) == "" ||
		req.Amount <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "order_id, customer_email, and a positive amount are required",
		})
		return
	}

	response, err := makePayment(r.Context(), req)
	if err != nil {
		cfg.logger.Error("failed to initialize payment", "err", err)
		status := http.StatusInternalServerError
		message := "failed to initialize payment"

		if strings.Contains(err.Error(), "already used for a different payment") {
			status = http.StatusConflict
			message = err.Error()
		} else if strings.Contains(err.Error(), "cannot be initialized") {
			status = http.StatusConflict
			message = err.Error()
		} else if strings.Contains(err.Error(), "required") ||
			strings.Contains(err.Error(), "valid customer email") {
			status = http.StatusBadRequest
			message = err.Error()
		} else if strings.Contains(err.Error(), "currency must be") {
			status = http.StatusBadRequest
			message = err.Error()
		}

		writeJSON(w, status, map[string]string{
			"error": message,
		})
		return
	}

	writeJSON(w, http.StatusOK, response)
}

func GetPaymentByIdHandler(w http.ResponseWriter, r *http.Request) {
	if !authenticateMerchant(w, r) {
		return
	}
	defer r.Body.Close()

	paymentId := r.PathValue("id")

	payment, err := GetPaymentByID(r.Context(), paymentId)
	if err != nil {
		cfg.logger.Error("failed to fetch payment", "err", err)
		status := http.StatusInternalServerError
		message := "failed to fetch payment"

		if strings.Contains(err.Error(), "payment id not provided") {
			status = http.StatusBadRequest
			message = err.Error()
		} else if strings.Contains(err.Error(), "invalid payment id") {
			status = http.StatusBadRequest
			message = err.Error()
		} else if strings.Contains(err.Error(), "payment not found") {
			status = http.StatusNotFound
			message = err.Error()
		} else if strings.Contains(err.Error(), "failed to fetch payment") {
			status = http.StatusInternalServerError
			message = err.Error()
		}

		writeJSON(w, status, map[string]string{
			"error": message,
		})
		return
	}

	writeJSON(w, http.StatusOK, payment)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		return
	}
} 