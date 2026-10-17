package config

import (
	"testing"
	"time"
)

func setEnv(t *testing.T, mapping map[string]string) {
	t.Helper()
	for k, v := range mapping {
		t.Setenv(k, v)
	}
}

func baseEnv() map[string]string {
	return map[string]string{
		"DATABASE_URL":                              "postgres://localhost/payments_test",
		"PAYSTACK_SECRET_KEY":                       "sk_test_abc123",
		"MERCHANT_API_KEY":                          "mkt_test_key",
		"CALLBACK_SECRET":                           "cb_secret",
		"CALLBACK_URL":                              "http://localhost:5000/callbacks",
		"PAYMENT_TIMEOUT_SECONDS":                   "60",
	}
}

func TestLoadFullConfig(t *testing.T) {
	setEnv(t, baseEnv())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Port != "4000" {
		t.Errorf("Port = %q, want %q", cfg.Port, "4000")
	}
	if cfg.DatabaseURL != "postgres://localhost/payments_test" {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.TestMode {
		t.Errorf("TestMode = true, want false")
	}
	if cfg.PaymentTimeoutSeconds != 60 {
		t.Errorf("PaymentTimeoutSeconds = %d, want 60", cfg.PaymentTimeoutSeconds)
	}
	if cfg.ExpiryWorkerInterval != 30*time.Second {
		t.Errorf("ExpiryWorkerInterval = %v, want 30s", cfg.ExpiryWorkerInterval)
	}
	if cfg.CallbackCheckInterval != 1*time.Second {
		t.Errorf("CallbackCheckInterval = %v, want 1s", cfg.CallbackCheckInterval)
	}
}

func TestLoadDefaults(t *testing.T) {
	setEnv(t, baseEnv())
	setEnv(t, map[string]string{
		"PORT":                      "",
		"TEST_MODE":                 "true",
		"PAYMENT_TIMEOUT_SECONDS":   "",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Port != defaultPort {
		t.Errorf("Port = %q, want default %q", cfg.Port, defaultPort)
	}
	if !cfg.TestMode {
		t.Errorf("TestMode = false, want true")
	}
	if cfg.PaymentTimeoutSeconds != 1800 {
		t.Errorf("PaymentTimeoutSeconds = %d, want default 1800", cfg.PaymentTimeoutSeconds)
	}
}

func TestLoadInvalidTimeout(t *testing.T) {
	setEnv(t, baseEnv())
	setEnv(t, map[string]string{"PAYMENT_TIMEOUT_SECONDS": "-5"})

	if _, err := Load(); err == nil {
		t.Fatal("Load() expected error for negative PAYMENT_TIMEOUT_SECONDS, got nil")
	}
}

func TestLoadMissingRequired(t *testing.T) {
	for _, key := range []string{"DATABASE_URL", "PAYSTACK_SECRET_KEY", "MERCHANT_API_KEY", "CALLBACK_SECRET"} {
		t.Run(key, func(t *testing.T) {
			setEnv(t, baseEnv())
			t.Setenv(key, "")

			if _, err := Load(); err == nil {
				t.Fatalf("Load() expected error when %s is empty, got nil", key)
			}
		})
	}
}

func TestLoadTestModeValues(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"true", true},
		{"TRUE", true},
		{"1", true},
		{"yes", true},
		{"false", false},
		{"0", false},
		{"", false},
		{"nonsense", false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			setEnv(t, baseEnv())
			t.Setenv("TEST_MODE", tt.value)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.TestMode != tt.want {
				t.Errorf("TestMode = %v, want %v for TEST_MODE=%q", cfg.TestMode, tt.want, tt.value)
			}
		})
	}
}
