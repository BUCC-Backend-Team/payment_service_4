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

	if strings.TrimSpace(req.OrderID) == "" ||
		strings.TrimSpace(req.IdempotencyKey) == "" ||
		strings.TrimSpace(req.CustomerEmail) == "" ||
		req.Amount <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "order_id, idempotency_key, customer_email, and a positive amount are required",
		})
		return
	}

	response, err := makePayment(r.Context(), req)
	if err != nil {
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
		}

		writeJSON(w, status, map[string]string{
			"error": message,
		})
		return
	}

	writeJSON(w, http.StatusOK, response)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		return
	}
} 