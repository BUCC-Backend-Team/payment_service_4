package providers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) (*PaystackProvider, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(func() { server.Close() })

	provider := NewPaystack("sk_test_dummy", nil)
	provider.BaseURL = server.URL
	return provider, server
}

func TestInitializeTransactionSuccess(t *testing.T) {
	provider, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/transaction/initialize" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk_test_dummy" {
			t.Errorf("authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":true,"message":"Authorization URL created","data":{"authorization_url":"https://checkout.paystack.com/abc","access_code":"ac_1","reference":"pay-1"}}`)
	})

	checkout, err := provider.InitializeTransaction(context.Background(), InitializeRequest{
		Reference: "pay-1",
		Amount:    500000,
		Currency:  "NGN",
		Email:     "buyer@example.com",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if checkout.AuthorizationURL != "https://checkout.paystack.com/abc" {
		t.Errorf("authorization url = %q", checkout.AuthorizationURL)
	}
	if checkout.AccessCode != "ac_1" {
		t.Errorf("access code = %q", checkout.AccessCode)
	}
	if checkout.Reference != "pay-1" {
		t.Errorf("reference = %q", checkout.Reference)
	}
}

func TestInitializeTransactionRejectsUnexpectedReference(t *testing.T) {
	provider, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"status":true,"message":"ok","data":{"authorization_url":"https://checkout.paystack.com/abc","reference":"other"}}`)
	})

	_, err := provider.InitializeTransaction(context.Background(), InitializeRequest{Reference: "pay-1"})
	if err == nil {
		t.Fatal("expected error for mismatched reference")
	}
	if strings.Contains(err.Error(), "nil") {
		t.Errorf("weird error: %v", err)
	}
}

func TestVerifyTransactionSuccess(t *testing.T) {
	provider, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/transaction/verify/pay-1" {
			t.Errorf("path = %q", got)
		}
		fmt.Fprint(w, `{"status":true,"message":"Verification successful","data":{"reference":"pay-1","status":"success","amount":500000,"currency":"NGN","channel":"card"}}`)
	})

	transaction, err := provider.VerifyTransaction(context.Background(), "pay-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if transaction.Status != "success" {
		t.Errorf("status = %q", transaction.Status)
	}
	if transaction.Amount != 500000 {
		t.Errorf("amount = %d", transaction.Amount)
	}
	if transaction.Channel != "card" {
		t.Errorf("channel = %q", transaction.Channel)
	}
}

func TestCreateRefundSuccess(t *testing.T) {
	provider, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/refund" {
			t.Errorf("path = %q", r.URL.Path)
		}
		fmt.Fprint(w, `{"status":true,"message":"Refund created","data":{"id":9,"status":"pending","amount":250000,"currency":"NGN"}}`)
	})

	refund, err := provider.CreateRefund(context.Background(), RefundRequest{Transaction: "pay-1", Amount: 250000})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if refund.ID != 9 {
		t.Errorf("id = %d", refund.ID)
	}
	if refund.Status != "pending" {
		t.Errorf("status = %q", refund.Status)
	}
}

func TestCallClassification(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantIsRe   bool
		wantRetri  bool
	}{
		{"4xx reject", http.StatusBadRequest, `{"status":false,"message":"bad ref"}`, true, false},
		{"5xx ambiguous", http.StatusInternalServerError, `{"status":false,"message":"boom"}`, false, true},
		{"429 retryable", http.StatusTooManyRequests, `{"status":false,"message":"slow down"}`, false, true},
		{"200 with status false", http.StatusOK, `{"status":false,"message":"rejected"}`, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				fmt.Fprint(w, tt.body)
			})

			_, err := provider.VerifyTransaction(context.Background(), "pay-1")
			if err == nil {
				t.Fatal("expected error")
			}
			if got := IsRejection(err); got != tt.wantIsRe {
				t.Fatalf("IsRejection() = %v, want %v", got, tt.wantIsRe)
			}
			if got := IsRetryable(err); got != tt.wantRetri {
				t.Fatalf("IsRetryable() = %v, want %v", got, tt.wantRetri)
			}
		})
	}
}

func TestNetworkErrorsAreAmbiguous(t *testing.T) {
	closed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	closed.Close()

	provider := NewPaystack("sk_test_dummy", nil)
	provider.BaseURL = closed.URL

	_, err := provider.VerifyTransaction(context.Background(), "pay-1")
	if err == nil {
		t.Fatal("expected network error")
	}
	if IsRejection(err) {
		t.Fatal("connection failure must not be a rejection: outcome is unknown")
	}
}
