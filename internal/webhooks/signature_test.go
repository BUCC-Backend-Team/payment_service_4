package webhooks

import (
	"encoding/json"
	"testing"
)

func TestDeduplicationKeyUsesEventAndID(t *testing.T) {
	event := PaystackEvent{
		Event: "charge.success",
		Data:  EventData{ID: 424242, Reference: "pay-ref-1"},
	}

	got := DeduplicationKey(event)
	if got != "charge.success:424242" {
		t.Fatalf("DeduplicationKey() = %q, want charge.success:424242", got)
	}
}

func TestDeduplicationKeyFallsBackToReference(t *testing.T) {
	event := PaystackEvent{
		Event:        "charge.success",
		Data:         EventData{Reference: "pay-ref-1"},
	}

	got := DeduplicationKey(event)
	if got != "charge.success:pay-ref-1" {
		t.Fatalf("DeduplicationKey() = %q, want charge.success:pay-ref-1", got)
	}
}

func TestTransactionReferenceOf(t *testing.T) {
	tests := []struct {
		name string
		data EventData
		want string
	}{
		{"explicit transaction_reference", EventData{TransactionReference: "tref"}, "tref"},
		{"nested transaction reference", EventData{Transaction: struct {
			Reference string `json:"reference"`
			Amount    int64  `json:"amount"`
		}{Reference: "nested"}}, "nested"},
		{"fallback to reference", EventData{Reference: "plain"}, "plain"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.data.TransactionReferenceOf(); got != tt.want {
				t.Fatalf("TransactionReferenceOf() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRefundAmount(t *testing.T) {
	data := EventData{Amount: 700}
	if got := data.RefundAmount(); got != 700 {
		t.Fatalf("RefundAmount() = %d, want 700", got)
	}

	nested := EventData{Transaction: struct {
		Reference string `json:"reference"`
		Amount    int64  `json:"amount"`
	}{Reference: "r", Amount: 300}}
	if got := nested.RefundAmount(); got != 300 {
		t.Fatalf("RefundAmount() nested = %d, want 300", got)
	}
}

func TestEventDataJSONUnmarshal(t *testing.T) {
	raw := []byte(`{"event":"refund.processed","data":{"id":77,"amount":1234,"transaction_reference":"txref"}}`)

	var event PaystackEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if event.Event != "refund.processed" {
		t.Errorf("event = %q", event.Event)
	}
	if event.Data.ID != 77 {
		t.Errorf("id = %d", event.Data.ID)
	}
	if event.Data.Amount != 1234 {
		t.Errorf("amount = %d", event.Data.Amount)
	}
	if event.Data.TransactionReferenceOf() != "txref" {
		t.Errorf("reference = %q", event.Data.TransactionReferenceOf())
	}
}
