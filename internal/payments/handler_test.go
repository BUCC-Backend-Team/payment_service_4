package payments

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	apperrors "github.com/BUCC-Backend-Team/payment_service_4/internal/errors"
)

func TestPaymentStatusConstantsMatchSpec(t *testing.T) {
	tests := []struct {
		got  string
		want string
	}{
		{StatusPending, "pending"},
		{StatusSucceeded, "succeeded"},
		{StatusExpired, "expired"},
		{StatusPartiallyRefunded, "partially_refunded"},
		{StatusRefunded, "refunded"},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("status constant %q != expected %q", tt.got, tt.want)
		}
	}
}

func TestWriteErrorUsesAppErrorStatusAndCode(t *testing.T) {
	recorder := httptest.NewRecorder()
	WriteError(recorder, apperrors.New(http.StatusConflict, "payment_exists", "order already paid"))

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type = %q", got)
	}

	var payload map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode body %q: %v", recorder.Body.String(), err)
	}
	if payload["error"] != "payment_exists" {
		t.Fatalf("error code = %q, want payment_exists", payload["error"])
	}
}

func TestWriteJSONAddsIdempotentReplayedHeader(t *testing.T) {
	recorder := httptest.NewRecorder()

	WriteJSON(recorder, http.StatusCreated, bytes.NewBufferString("ignored").String(), map[string]string{
		"Idempotent-Replayed": "true",
	})

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", recorder.Code)
	}
	if got := recorder.Header().Get("Idempotent-Replayed"); got != "true" {
		t.Fatalf("Idempotent-Replayed = %q, want true", got)
	}
}
