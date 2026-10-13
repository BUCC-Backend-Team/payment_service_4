package payments

import (
	"crypto/hmac"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
)

type PaystackWebhookEvent struct {
	Event string `json:"event"`
	Data struct {
		ID int64 `json:"id"`
		Status    string `json:"status"`
		Reference string `json:"reference"`
		Amount    int64  `json:"amount"`
		Currency  string `json:"currency"`
		Domain    string `json:"domain"`		
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

	if event.Data.Reference == "" || event.Data.Amount <= 0 || event.Data.Currency == "" {
		http.Error(w, "Invalid tramsaction data", http.StatusBadRequest)
		return
	}

	//update db record

	cfg.logger.Info("paystack payment confirmed", "reference", event.Data.Reference, "amount", event.Data.Amount, "currency", event.Data.Currency)

	w.WriteHeader(http.StatusOK)
}