package callbacks

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/BUCC-Backend-Team/payment_service_4/src/db"
)

const (
	EventPaymentSucceeded    = "payment.succeeded"
	EventPaymentExpired      = "payment.expired"
	EventPaymentRefunded     = "payment.refunded"
	EventPaymentRefundFailed = "payment.refund_failed"
)

type EventData struct {
	PaymentID      string `json:"paymentId"`
	OrderID        string `json:"orderId"`
	Status         string `json:"status"`
	Amount         int64  `json:"amount"`
	AmountRefunded int64  `json:"amountRefunded"`
	Currency       string `json:"currency"`
	Late           bool   `json:"late"`
}

type Event struct {
	Type string     `json:"type"`
	Data EventData  `json:"data"`
	ID   *uuid.UUID `json:"-"`
}

type envelope struct {
	ID        uuid.UUID `json:"id"`
	Type      string    `json:"type"`
	CreatedAt string    `json:"createdAt"`
	Data      EventData `json:"data"`
}


func Enqueue(ctx context.Context, store *db.Store, eventType string, data EventData) error {
	id := uuid.New()

	payload, err := json.Marshal(envelope{
		ID:        id,
		Type:      eventType,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Data:      data,
	})
	if err != nil {
		return fmt.Errorf("callbacks: encode callback payload: %w", err)
	}

	return store.EnqueueCallback(ctx, id, eventType, payload)
}
