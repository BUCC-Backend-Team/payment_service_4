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
	"strings"
	"syscall"
	"time"

	"github.com/spf13/viper"
	"github.com/BUCC-Backend-Team/payment_service_4/internal/database"
	"github.com/BUCC-Backend-Team/payment_service_4/internal/payments"
)

type config struct {
	logger *slog.Logger
	db *database.Queries
	DB *sql.DB
}

var cfg *config

var logger = initializeLogger()

func LoadEnv() error {
	viper.SetConfigFile(".env")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to load .env: %w", err)
	}

	return nil
}

func initializeLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

func loggerWithLevel(raw string) *slog.Logger {
	level := slog.LevelInfo
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}

func run () {
	if err := LoadEnv(); err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}

	logger = loggerWithLevel(viper.GetString("LOG_LEVEL"))
	slog.SetDefault(logger)

	dbURL := viper.GetString("DATABASE_URL")
	db, err := connectPostgres(dbURL)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
	defer db.Close()

	paystackSecretKey := viper.GetString("PAYSTACK_SECRET_KEY")
	merchantApiKey := viper.GetString("MERCHANT_API_KEY")

	cfg = &config{
		logger: logger,
		db: database.New(db),
		DB: db,
	}

	payments.New(paystackSecretKey, logger, cfg.db, db, merchantApiKey)

	mux := http.NewServeMux()
	registerRoutes(mux)

	addr := fmt.Sprintf("0.0.0.0:%s", viper.GetString("PORT"))
	server := http.Server{
		Addr: addr,
		Handler: mux,
	}

	shutdownCtx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	go func() {
		logger.Info("payment service starting", "address", addr)

		if err := server.ListenAndServe(); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
				cfg.logger.Error("server error", "error", err)
				os.Exit(1)
			}
	}()

	<-shutdownCtx.Done()

	cfg.logger.Info("shutdown signal rceived")

	ctx, cancel := context.WithTimeout(
		context.Background(),
		30*time.Second,
	)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		cfg.logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
	
	cfg.logger.Info("server stopped")
}