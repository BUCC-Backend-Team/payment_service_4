package payments

import (
	"context"
	"encoding/json"
	"fmt"
)

type PaystackVerificationResponse struct {
	Status  bool                     `json:"status"`
	Message string                   `json:"message"`
	Data    PaystackTransactionData  `json:"data"`
}

type PaystackTransactionData struct {
	ID                      int64                  `json:"id"`
	Status                  string                 `json:"status"`
	Reference               string                 `json:"reference"`
	Amount                  int64                  `json:"amount"`
	PaidAt                  string                 `json:"paid_at"`
	CreatedAt               string                 `json:"created_at"`
	Channel                 string                 `json:"channel"`
	Currency                string                 `json:"currency"`
	Authorization           PaystackAuthorization  `json:"authorization"`
	Customer                PaystackCustomer       `json:"customer"`
	TransactionDate         string                 `json:"transaction_date"`
}

type PaystackAuthorization struct {
	AuthorizationCode string  `json:"authorization_code"`
	CardType          string  `json:"card_type"`
	Bank              string  `json:"bank"`
	CountryCode       string  `json:"country_code"`
	Signature         string  `json:"signature"`
}

type PaystackCustomer struct {
	ID                      int64   `json:"id"`
	Email                   string  `json:"email"`
	CustomerCode            string  `json:"customer_code"`
}

var vtURL string = "api.paystack.co/transaction/verify/"

func verifyTransaction(ctx context.Context, reference string) (PaystackVerificationResponse, error) {
	url := fmt.Sprintf(vtURL, reference)

	headers := make(map[string]string)
	headers["Authorization"] = fmt.Sprintf("Bearer %s", cfg.paystackSecretKey)

	res, err := SendRequest(ctx, url, "GET", nil, headers)
	if err != nil {
		cfg.logger.Error("failed to verify transaction", "err", err)
		return PaystackVerificationResponse{}, err
	}

	var vtRes PaystackVerificationResponse
	if err := json.Unmarshal(res.Body, vtRes); err != nil {
		cfg.logger.Error("failed to decode response", "err", err)
		return PaystackVerificationResponse{}, err
	}

	return vtRes, nil
}