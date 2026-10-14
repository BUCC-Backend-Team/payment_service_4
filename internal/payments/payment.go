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
	OrderID        string `json:"orderId"`
	IdempotencyKey string `json:"Idempotency-Key"`
	Amount         int64 `json:"amount"`
	CustomerEmail  string `json:"customerEmail"`
	ReturnUrl string `json:"returnUrl"`
	Currency string `json:"currency"`
}

type MakePaymentResponse struct {
	PaymentID   string `json:"paymentId"`
	OrderID     string `json:"orderId"`
	Status      string `json:"status"`
	Amount      int64  `json:"amount"`
	Currency    string `json:"currency"`
	CheckoutURL string `json:"checkoutUrl"`
}

type GetPaymentByIdResponse struct {
	Status string `json:"status"`
	Channel string `json:"channel"`
	AmountRefunded uint `json:"amountRefunded"`
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
		req.ReturnUrl == "" ||
		req.Amount <= 0 {
		return MakePaymentResponse{}, errors.New("order ID, idempotency key, customer email, returnUrl and positive amount are required")
	}

	if strings.ContainsAny(req.CustomerEmail, " \t\r\n") ||
		!strings.Contains(req.CustomerEmail, "@") {
		return MakePaymentResponse{}, errors.New("a valid customer email is required")
	}

	var currency string
	if req.Currency == "" {
		currency = "NGN"
	}

	if req.Currency != "" && req.Currency != "NGN" {
		return MakePaymentResponse{}, errors.New("currency must be NGN")
	}

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
			ReturnUrl: req.ReturnUrl,
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
			Amount:      payment.Amount,
			Currency:    payment.Currency,
			Status:      payment.Status,
			CheckoutURL: payment.CheckoutUrl.String,
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
		Amount:      updated.Amount,
		Currency:    updated.Currency,
		Status:      updated.Status,
		CheckoutURL: paystackRes.Data.AuthorizationUrl,
	}, nil
}


func GetPaymentByID(ctx context.Context, paymentId string) (GetPaymentByIdResponse, error) {
	if cfg == nil || cfg.db == nil {
		return GetPaymentByIdResponse{}, errors.New("payment service is not initialized")
	}

	if paymentId == "" {
		return GetPaymentByIdResponse{}, errors.New("payment id not provided")
	}

	pId, err := uuid.Parse(paymentId)
	if err != nil {
		return GetPaymentByIdResponse{}, errors.New("invalid payment id")
	}

	payment, err := cfg.db.GetPaymentById(ctx, pId)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return GetPaymentByIdResponse{}, errors.New("payment not found")
		}
		return GetPaymentByIdResponse{}, errors.New("failed to fetch payment")
	}

	return GetPaymentByIdResponse{
		Status: payment.Status,
		Channel: payment.Channel.String,
		AmountRefunded: uint(payment.Amount),
	}, nil
}