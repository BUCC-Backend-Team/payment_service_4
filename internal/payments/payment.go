package payments

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/BUCC-Backend-Team/payment_service_4/internal/database"
)

type MakePaymentRequest struct {
	OrderID        string
	IdempotencyKey string
	Amount         int64
	CustomerEmail  string
}

type MakePaymentResponse struct {
	PaymentID   string `json:"payment_id"`
	OrderID     string `json:"order_id"`
	Reference   string `json:"reference"`
	Amount      int64  `json:"amount"`
	Currency    string `json:"currency"`
	Status      string `json:"status"`
	CheckoutURL string `json:"checkout_url"`
	AccessCode  string `json:"access_code"`
}

func makePayment(ctx context.Context, req MakePaymentRequest) (MakePaymentResponse, error) {
	if cfg == nil || cfg.db == nil {
		return MakePaymentResponse{}, errors.New("payment service is not initialized")
	}

	req.OrderID = strings.TrimSpace(req.OrderID)
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	req.CustomerEmail = strings.TrimSpace(req.CustomerEmail)

	if req.OrderID == "" ||
		req.IdempotencyKey == "" ||
		req.CustomerEmail == "" ||
		req.Amount <= 0 {
		return MakePaymentResponse{}, errors.New("order ID, idempotency key, customer email, and positive amount are required")
	}

	if strings.ContainsAny(req.CustomerEmail, " \t\r\n") ||
		!strings.Contains(req.CustomerEmail, "@") {
		return MakePaymentResponse{}, errors.New("a valid customer email is required")
	}

	const currency = "NGN"

	payment, err := cfg.db.GetPaymentByIdempotencyKey(ctx, req.IdempotencyKey)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return MakePaymentResponse{}, fmt.Errorf("get existing payment: %w", err)
	}

	if errors.Is(err, sql.ErrNoRows) {
		reference := uuid.NewString()

		payment, err = cfg.db.CreatePayment(ctx, database.CreatePaymentParams{
			IdempotencyKey:   req.IdempotencyKey,
			OrderID:          req.OrderID,
			Amount:           req.Amount,
			Currency:         currency,
			CustomerEmail:    req.CustomerEmail,
			PaystackReference: reference,
		})
		if err != nil {
			createdPayment, getErr := cfg.db.GetPaymentByIdempotencyKey(ctx, req.IdempotencyKey)
			if getErr != nil {
				return MakePaymentResponse{}, fmt.Errorf("create payment: %w", err)
			}
			payment = createdPayment
		}
	}

	if payment.OrderID != req.OrderID ||
		payment.Amount != req.Amount ||
		payment.CustomerEmail != req.CustomerEmail ||
		payment.Currency != currency {
		return MakePaymentResponse{}, errors.New("idempotency key was already used for a different payment")
	}

	if payment.Status != "pending" {
		return MakePaymentResponse{}, fmt.Errorf(
			"payment cannot be initialized because its status is %q",
			payment.Status,
		)
	}

	if payment.CheckoutUrl.Valid && payment.CheckoutUrl.String != "" {
		response := MakePaymentResponse{
			PaymentID:   payment.ID.String(),
			OrderID:     payment.OrderID,
			Reference:   payment.PaystackReference,
			Amount:      payment.Amount,
			Currency:    payment.Currency,
			Status:      payment.Status,
			CheckoutURL: payment.CheckoutUrl.String,
		}

		if payment.AccessCode.Valid {
			response.AccessCode = payment.AccessCode.String
		}

		return response, nil
	}

	paystackRes, err := chargeAuthorization(
		ctx,
		req.Amount,
		req.CustomerEmail,
		payment.PaystackReference,
	)
	if err != nil {
		return MakePaymentResponse{}, fmt.Errorf("initialize payment with Paystack: %w", err)
	}

	updated, err := cfg.db.UpdatePaymentCheckout(ctx, database.UpdatePaymentCheckoutParams{
		ID:                payment.ID,
		CheckoutUrl:       sql.NullString{String: paystackRes.Data.AuthorizationUrl, Valid: true},
		PaystackReference: payment.PaystackReference,
		AccessCode:        sql.NullString{String: paystackRes.Data.AccessCode, Valid: paystackRes.Data.AccessCode != ""},
	})
	if err != nil {
		return MakePaymentResponse{}, fmt.Errorf("save payment checkout details: %w", err)
	}

	return MakePaymentResponse{
		PaymentID:   updated.ID.String(),
		OrderID:     updated.OrderID,
		Reference:   updated.PaystackReference,
		Amount:      updated.Amount,
		Currency:    updated.Currency,
		Status:      updated.Status,
		CheckoutURL: paystackRes.Data.AuthorizationUrl,
		AccessCode:  paystackRes.Data.AccessCode,
	}, nil
}
