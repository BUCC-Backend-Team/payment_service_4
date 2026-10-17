package webhooks

import (
	"fmt"
)


type PaystackEvent struct {
	Event string `json:"event"`
	Data  EventData `json:"data"`
}

type EventData struct {
	ID                   int64  `json:"id"`
	Reference            string `json:"reference"`
	TransactionReference string `json:"transaction_reference"`
	Amount               int64  `json:"amount"`
	Transaction struct {
		Reference string `json:"reference"`
		Amount    int64  `json:"amount"`
	} `json:"transaction"`
}


func (d EventData) TransactionReferenceOf() string {
	if d.TransactionReference != "" {
		return d.TransactionReference
	}
	if d.Transaction.Reference != "" {
		return d.Transaction.Reference
	}
	return d.Reference
}


func (d EventData) RefundAmount() int64 {
	if d.Amount > 0 {
		return d.Amount
	}
	return d.Transaction.Amount
}

func DeduplicationKey(event PaystackEvent) string {
	if event.Data.ID != 0 {
		return fmt.Sprintf("%s:%d", event.Event, event.Data.ID)
	}
	return event.Event + ":" + event.Data.TransactionReferenceOf()
}
