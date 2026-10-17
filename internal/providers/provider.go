package providers

import (
	"context"
	"errors"
)

type Checkout struct {
	AuthorizationURL string
	AccessCode       string
	Reference        string
}

type InitializeRequest struct {
	Reference  string
	Amount     int64
	Currency   string
	Email      string
	CallbackURL string
}

type Transaction struct {
	Reference string
	Status    string
	Amount    int64
	Currency  string
	Channel   string
}

type RefundRequest struct {
	Transaction string
	Amount      int64
}

type Refund struct {
	ID     int64
	Status string
	Amount int64
}

var ErrAmbiguous = errors.New("provider Paystack call failed with an ambiguous outcome")

func IsAmbiguous(err error) bool {
	call, ok := err.(*ProviderCallError)
	if !ok {
		return true
	}
	return call.Class == ClassAmbiguous
}


type Provider interface {
	InitializeTransaction(ctx context.Context, req InitializeRequest) (Checkout, error)
	VerifyTransaction(ctx context.Context, reference string) (Transaction, error)
	CreateRefund(ctx context.Context, req RefundRequest) (Refund, error)
}
