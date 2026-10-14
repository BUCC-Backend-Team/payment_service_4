package main

import (
	"encoding/json"
	"net/http"

	"github.com/BUCC-Backend-Team/payment_service_4/internal/payments"
)

func HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", HandleHealth)

	mux.HandleFunc("POST /webhooks/paystack", payments.HandleWebhook)

	mux.HandleFunc("POST /payments", payments.MakePaymentHandler)

	mux.HandleFunc("GET /payments/{id}", payments.GetPaymentByIdHandler)
}