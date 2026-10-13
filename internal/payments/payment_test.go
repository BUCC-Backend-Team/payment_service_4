
package payments

import (
	"context"
	"strings"
	"testing"

	"github.com/BUCC-Backend-Team/payment_service_4/internal/database"
)

func TestMakePayment(t *testing.T) {
	originalCfg := cfg
	t.Cleanup(func() {
		cfg = originalCfg
	})

	tests := []struct {
		name          string
		setup         func()
		req           MakePaymentRequest
		wantErr       bool
		wantErrString string
	}{
		{
			name: "service not initialized",
			setup: func() {
				cfg = nil
			},
			req: MakePaymentRequest{
				OrderID:        "order-123",
				IdempotencyKey: "idem-123",
				Amount:         50000,
				CustomerEmail:  "customer@example.com",
			},
			wantErr:       true,
			wantErrString: "payment service is not initialized",
		},
		{
			name: "database not initialized",
			setup: func() {
				cfg = &config{}
			},
			req: MakePaymentRequest{
				OrderID:        "order-123",
				IdempotencyKey: "idem-123",
				Amount:         50000,
				CustomerEmail:  "customer@example.com",
			},
			wantErr:       true,
			wantErrString: "payment service is not initialized",
		},
		{
			name: "missing order ID",
			setup: func() {
				cfg = &config{db: database.New(nil)}
			},
			req: MakePaymentRequest{
				IdempotencyKey: "idem-123",
				Amount:         50000,
				CustomerEmail:  "customer@example.com",
			},
			wantErr:       true,
			wantErrString: "order ID, idempotency key, customer email, and positive amount are required",
		},
		{
			name: "missing idempotency key",
			setup: func() {
				cfg = &config{db: database.New(nil)}
			},
			req: MakePaymentRequest{
				OrderID:       "order-123",
				Amount:        50000,
				CustomerEmail: "customer@example.com",
			},
			wantErr:       true,
			wantErrString: "order ID, idempotency key, customer email, and positive amount are required",
		},
		{
			name: "missing customer email",
			setup: func() {
				cfg = &config{db: database.New(nil)}
			},
			req: MakePaymentRequest{
				OrderID:        "order-123",
				IdempotencyKey: "idem-123",
				Amount:         50000,
			},
			wantErr:       true,
			wantErrString: "order ID, idempotency key, customer email, and positive amount are required",
		},
		{
			name: "zero amount",
			setup: func() {
				cfg = &config{db: database.New(nil)}
			},
			req: MakePaymentRequest{
				OrderID:        "order-123",
				IdempotencyKey: "idem-123",
				Amount:         0,
				CustomerEmail:  "customer@example.com",
			},
			wantErr:       true,
			wantErrString: "order ID, idempotency key, customer email, and positive amount are required",
		},
		{
			name: "negative amount",
			setup: func() {
				cfg = &config{db: database.New(nil)}
			},
			req: MakePaymentRequest{
				OrderID:        "order-123",
				IdempotencyKey: "idem-123",
				Amount:         -100,
				CustomerEmail:  "customer@example.com",
			},
			wantErr:       true,
			wantErrString: "order ID, idempotency key, customer email, and positive amount are required",
		},
		{
			name: "email without at sign",
			setup: func() {
				cfg = &config{db: database.New(nil)}
			},
			req: MakePaymentRequest{
				OrderID:        "order-123",
				IdempotencyKey: "idem-123",
				Amount:         50000,
				CustomerEmail:  "invalid-email",
			},
			wantErr:       true,
			wantErrString: "a valid customer email is required",
		},
		{
			name: "email containing whitespace",
			setup: func() {
				cfg = &config{db: database.New(nil)}
			},
			req: MakePaymentRequest{
				OrderID:        "order-123",
				IdempotencyKey: "idem-123",
				Amount:         50000,
				CustomerEmail:  "customer @example.com",
			},
			wantErr:       true,
			wantErrString: "a valid customer email is required",
		},
		{
			name: "email containing newline",
			setup: func() {
				cfg = &config{db: database.New(nil)}
			},
			req: MakePaymentRequest{
				OrderID:        "order-123",
				IdempotencyKey: "idem-123",
				Amount:         50000,
				CustomerEmail:  "customer\n@example.com",
			},
			wantErr:       true,
			wantErrString: "a valid customer email is required",
		},
		{
			name: "request fields are trimmed before validation",
			setup: func() {
				cfg = &config{db: database.New(nil)}
			},
			req: MakePaymentRequest{
				OrderID:        "  ",
				IdempotencyKey: "idem-123",
				Amount:         50000,
				CustomerEmail:  "customer@example.com",
			},
			wantErr:       true,
			wantErrString: "order ID, idempotency key, customer email, and positive amount are required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			_, err := makePayment(context.Background(), tt.req)

			if tt.wantErr {
				if err == nil {
					t.Fatal("makePayment() expected an error, got nil")
				}

				if !strings.Contains(err.Error(), tt.wantErrString) {
					t.Fatalf(
						"makePayment() error = %q, want error containing %q",
						err.Error(),
						tt.wantErrString,
					)
				}

				return
			}

			if err != nil {
				t.Fatalf("makePayment() unexpected error: %v", err)
			}
		})
	}
}
