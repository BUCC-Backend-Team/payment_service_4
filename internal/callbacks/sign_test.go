package callbacks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"
)

func TestSignFormat(t *testing.T) {
	secret := "test_secret"
	body := []byte(`{"id":"123"}`)
	ts := time.Unix(1700000000, 0)

	got := Sign(secret, ts, body)

	wantMAC := hmac.New(sha256.New, []byte(secret))
	_, _ = wantMAC.Write([]byte("1700000000."))
	_, _ = wantMAC.Write(body)
	want := "t=1700000000,v1=" + hex.EncodeToString(wantMAC.Sum(nil))

	if got != want {
		t.Fatalf("Sign() = %q, want %q (format t=unix,v1=hmac-ts.body)", got, want)
	}
}

func TestSignIsBodySensitive(t *testing.T) {
	secret := "test_secret"
	ts := time.Now().Unix()

	one := Sign(secret, time.Unix(ts, 0), []byte("one"))
	two := Sign(secret, time.Unix(ts, 0), []byte("two"))

	if one == two {
		t.Fatal("signatures for different bodies must differ")
	}
}

func TestSignIsSecretSensitive(t *testing.T) {
	body := []byte("payload")
	ts := time.Unix(42, 0)

	first := Sign("secret_a", ts, body)
	second := Sign("secret_b", ts, body)

	if first == second {
		t.Fatal("signatures for different secrets must differ")
	}
}

func TestEnvelopeJSONShape(t *testing.T) {
	event := Event{
		Type: EventPaymentSucceeded,
		Data: EventData{
			PaymentID:      "pay-1",
			OrderID:        "order-1",
			Status:         "succeeded",
			Amount:         5000,
			AmountRefunded: 0,
			Currency:       "NGN",
			Late:           false,
		},
	}

	if event.Type != "payment.succeeded" {
		t.Errorf("event type = %q", event.Type)
	}
	if event.Data.Currency != "NGN" {
		t.Errorf("currency = %q", event.Data.Currency)
	}
}
