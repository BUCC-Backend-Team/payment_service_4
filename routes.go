package main

import (
	"net/http"

	"github.com/BUCC-Backend-Team/payment_service_4/internal/api"
	"github.com/BUCC-Backend-Team/payment_service_4/internal/config"
	"github.com/BUCC-Backend-Team/payment_service_4/internal/payments"
	"github.com/BUCC-Backend-Team/payment_service_4/internal/refunds"
	"github.com/BUCC-Backend-Team/payment_service_4/internal/webhooks"
)





func registerRoutes(mux *http.ServeMux, paymentService *payments.Service, refundService *refunds.Service, webhookService *webhooks.Service, testMode bool) {
	paymentsHandler := payments.NewHandler(paymentService)
	refundHandler := refunds.NewHandler(refundService)
	guard := api.MerchantGuard(config.Config.MerchantAPIKey)

	mux.Handle("POST /payments", guard(http.HandlerFunc(paymentsHandler.Create)))
	mux.Handle("GET /payments/{id}", guard(http.HandlerFunc(paymentsHandler.Get)))

	mux.Handle("POST /payments/{id}/refunds", guard(http.HandlerFunc(refundHandler.Create)))

	mux.HandleFunc("GET /health", api.Health())

	mux.Handle("POST /webhooks/paystack", http.HandlerFunc(webhookService.Handle))

	mux.Handle("POST /admin/reset", guard(api.ResetHandler(cfg, testMode)))
}