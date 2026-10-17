package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port                  string
	DatabaseURL           string
	PaystackSecretKey     string
	MerchantAPIKey        string
	CallbackURL           string
	CallbackSecret        string
	PaymentTimeoutSeconds int
	TestMode              bool
	ExpiryWorkerInterval  time.Duration
	CallbackCheckInterval time.Duration
}

const (
	defaultPort             = "4000"
	defaultTimeoutSeconds   = 1800
	defaultExpiryInterval   = 30 * time.Second
	defaultCallbackInterval = 1 * time.Second
)

func Load() (*Config, error) {
	if err := loadDotEnv(".env"); err != nil {
		return nil, err
	}

	cfg := &Config{
		Port:                  envString("PORT", defaultPort),
		DatabaseURL:           strings.TrimSpace(os.Getenv("DATABASE_URL")),
		PaystackSecretKey:     strings.TrimSpace(os.Getenv("PAYSTACK_SECRET_KEY")),
		MerchantAPIKey:        strings.TrimSpace(os.Getenv("MERCHANT_API_KEY")),
		CallbackURL:           strings.TrimSpace(os.Getenv("CALLBACK_URL")),
		CallbackSecret:        strings.TrimSpace(os.Getenv("CALLBACK_SECRET")),
		PaymentTimeoutSeconds: envInt("PAYMENT_TIMEOUT_SECONDS", defaultTimeoutSeconds),
		TestMode:              envBool("TEST_MODE", false),
		ExpiryWorkerInterval:  envDurationSeconds("PAYMENT_EXPIRY_WORKER_INTERVAL_SECONDS", defaultExpiryInterval),
		CallbackCheckInterval: envDurationSeconds("CALLBACK_WORKER_INTERVAL_SECONDS", defaultCallbackInterval),
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if c.DatabaseURL == "" {
		return errors.New("config: DATABASE_URL is required")
	}
	if c.PaystackSecretKey == "" {
		return errors.New("config: PAYSTACK_SECRET_KEY is required")
	}
	if c.MerchantAPIKey == "" {
		return errors.New("config: MERCHANT_API_KEY is required")
	}
	if c.CallbackSecret == "" {
		return errors.New("config: CALLBACK_SECRET is required")
	}
	if c.PaymentTimeoutSeconds <= 0 {
		return fmt.Errorf("config: PAYMENT_TIMEOUT_SECONDS must be positive, got %d", c.PaymentTimeoutSeconds)
	}
	return nil
}

func envString(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envBool(key string, fallback bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func envDurationSeconds(key string, fallback time.Duration) time.Duration {
	n := envInt(key, 0)
	if n <= 0 {
		return fallback
	}
	return time.Duration(n) * time.Second
}
