package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"

	"github.com/BUCC-Backend-Team/payment_service_4/internal/api"
	"github.com/BUCC-Backend-Team/payment_service_4/internal/callbacks"
	"github.com/BUCC-Backend-Team/payment_service_4/internal/config"
	"github.com/BUCC-Backend-Team/payment_service_4/internal/database"
	"github.com/BUCC-Backend-Team/payment_service_4/internal/jobs"
	"github.com/BUCC-Backend-Team/payment_service_4/internal/payments"
	"github.com/BUCC-Backend-Team/payment_service_4/internal/providers"
	refunds "github.com/BUCC-Backend-Team/payment_service_4/internal/refunds"
	"github.com/BUCC-Backend-Team/payment_service_4/internal/webhooks"
)

func main() {
	run()
}

func Connect(databaseURL string) (*sql.DB, error) {
	conn, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("db: open connection: %w", err)
	}

	if err := conn.Ping(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}

	conn.SetMaxOpenConns(8)
	conn.SetMaxIdleConns(4)

	return conn, nil
}

func run() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	store, err := Connect(cfg.DatabaseURL)
	if err != nil {
		logger.Error("connect database", "error", err)
		os.Exit(1)
	}
	defer func() { _ = store.Close() }()

	queries := database.NewStore(store)

	paystackProvider := providers.NewPaystack(cfg.PaystackSecretKey, logger)
	paymentTimeout := time.Duration(cfg.PaymentTimeoutSeconds) * time.Second

	paymentService := payments.NewService(queries, paystackProvider, logger, paymentTimeout)
	refundService := refunds.NewService(queries, paystackProvider, paymentService, logger)
	webhookService := webhooks.NewService(queries, paystackProvider, paymentService, logger, cfg.PaystackSecretKey)
	expiryService := jobs.NewService(queries, paystackProvider, paymentService, logger, paymentTimeout)
	callbackWorker := callbacks.NewWorker(cfg.CallbackURL, cfg.CallbackSecret, queries, cfg.CallbackCheckInterval, logger)

	workerCtx, stopWorkers := context.WithCancel(context.Background())
	defer stopWorkers()

	go callbackWorker.Run(workerCtx)
	go expiryService.Run(workerCtx, cfg.ExpiryWorkerInterval)

	handler := api.Routes(queries, api.Config{TestMode: cfg.TestMode, MerchantAPIKey: cfg.MerchantAPIKey}, paymentService, refundService, webhookService)

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("payment service listening", "port", cfg.Port, "testMode", cfg.TestMode)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "error", err)
			stopWorkers()
			os.Exit(1)
		}
	}()

	<-signalCtx.Done()
	logger.Info("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}
	stopWorkers()
	logger.Info("server stopped")
}
