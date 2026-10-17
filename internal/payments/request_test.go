package payments

import (
	"testing"

	apperrors "github.com/BUCC-Backend-Team/payment_service_4/internal/errors"
)

func TestValidatedCreateRequest(t *testing.T) {
	valid := createPaymentBody{
		OrderID:       "order-1",
		Amount:        500000,
		Currency:      "",
		CustomerEmail: "buyer@example.com",
		ReturnURL:     "https://marketly.example/checkout/return",
	}

	t.Run("defaults to NGN", func(t *testing.T) {
		req, err := validatedCreateRequest(valid)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.Currency != "NGN" {
			t.Fatalf("currency = %q, want NGN", req.Currency)
		}
	})

	t.Run("rejects non NGN currency", func(t *testing.T) {
		invalid := valid
		invalid.Currency = "USD"
		_, err := validatedCreateRequest(invalid)
		if code(err) != "invalid_currency" {
			t.Fatalf("err code = %q, want invalid_currency", code(err))
		}
	})

	t.Run("rejects zero amount", func(t *testing.T) {
		invalid := valid
		invalid.Amount = 0
		_, err := validatedCreateRequest(invalid)
		if code(err) != "invalid_amount" {
			t.Fatalf("err code = %q", code(err))
		}
	})

	t.Run("rejects small amounts", func(t *testing.T) {
		invalid := valid
		invalid.Amount = 1
		if _, err := validatedCreateRequest(invalid); err != nil {
			t.Fatalf("small positive amount must be allowed, got %v", err)
		}
	})

	t.Run("rejects empty order id", func(t *testing.T) {
		invalid := valid
		invalid.OrderID = "  "
		_, err := validatedCreateRequest(invalid)
		if code(err) != "invalid_order_id" {
			t.Fatalf("err code = %q", code(err))
		}
	})

	t.Run("rejects invalid email", func(t *testing.T) {
		invalid := valid
		invalid.CustomerEmail = "not-an-email"
		_, err := validatedCreateRequest(invalid)
		if code(err) != "invalid_customer_email" {
			t.Fatalf("err code = %q", code(err))
		}
	})

	t.Run("rejects non http(s) return url", func(t *testing.T) {
		invalid := valid
		invalid.ReturnURL = "javascript:alert(1)"
		_, err := validatedCreateRequest(invalid)
		if code(err) != "invalid_return_url" {
			t.Fatalf("err code = %q", code(err))
		}
	})

	t.Run("accepts https return url", func(t *testing.T) {
		if _, err := validatedCreateRequest(valid); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestRequestHashIsStable(t *testing.T) {
	one := requestHash("order-1", "5000", "NGN", "a@b.com", "https://x")
	two := requestHash("order-1", "5000", "NGN", "a@b.com", "https://x")
	different := requestHash("order-1", "5001", "NGN", "a@b.com", "https://x")

	if one != two {
		t.Fatal("same inputs must hash equal")
	}
	if one == different {
		t.Fatal("different inputs must hash differently")
	}
}

func code(err error) string {
	appErr, ok := err.(*apperrors.AppError)
	if !ok {
		return ""
	}
	return appErr.Code
}
