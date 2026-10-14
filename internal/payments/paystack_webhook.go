package payments

import (
	"crypto/hmac"
	"crypto/sha512"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/BUCC-Backend-Team/payment_service_4/internal/database"
)

type PaystackWebhookEvent struct {
	Event string `json:"event"`
	Data struct {
		ID int64 `json:"id"`
		Status    string `json:"status"`
		Reference string `json:"reference"`
		Amount    int64  `json:"amount"`
		Currency  string `json:"currency"`
		Channel string `json:"channel"`		
	} `json:"data"`
}

func HandleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	signature := r.Header.Get("x-paystack-signature")

	if cfg.paystackSecretKey == "" || signature == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	mac := hmac.New(sha512.New, []byte(cfg.paystackSecretKey))
	_, _ = mac.Write(body)
	expectedSignature := hex.EncodeToString(mac.Sum(nil))

	if subtle.ConstantTimeCompare([]byte(expectedSignature), []byte(signature)) != 1 {
		cfg.logger.Warn("Invalid paystack webhook signature")
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}

	var event PaystackWebhookEvent

	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "Invalid webhook payload", http.StatusBadRequest)
		return
	}

	if event.Event != "charge.success" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if event.Data.Status != "success" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if event.Data.Reference == "" || event.Data.Amount <= 0 || event.Data.Currency == "" || event.Data.Channel == "" {
		http.Error(w, "Invalid tramsaction data", http.StatusBadRequest)
		return
	}

	payment, err := cfg.db.GetPaymentByPaystackReference(r.Context(), event.Data.Reference)
	if err != nil {
		msg := fmt.Sprintf("payment record with reference %s not found", event.Data.Reference)
		cfg.logger.Error(msg)
		http.Error(w, msg, http.StatusNotFound)
		return
	}

	if payment.Amount != event.Data.Amount || payment.Currency != event.Data.Currency{
		http.Error(w, "Invalid Payment", http.StatusBadRequest)
		return
	}

	_, err = cfg.db.MarkPaymentSucceeded(r.Context(), database.MarkPaymentSucceededParams{
		PaystackReference: event.Data.Reference,
		Amount: event.Data.Amount,
		Currency: event.Data.Currency,
		Channel: sql.NullString{String: event.Data.Channel, Valid: true},
	})
	if err != nil {
		msg := "failed to mark payment succeeded"
		cfg.logger.Error(msg)
		http.Error(w, msg, http.StatusNotFound)
		return
	}

	cfg.logger.Info("paystack payment confirmed", "reference", event.Data.Reference, "amount", event.Data.Amount, "currency", event.Data.Currency)

	w.WriteHeader(http.StatusOK)
}