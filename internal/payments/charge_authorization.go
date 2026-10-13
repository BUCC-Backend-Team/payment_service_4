
package payments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const caURL = "https://api.paystack.co/transaction/initialize"

type caBody struct {
	Amount   int64  `json:"amount"`
	Email    string `json:"email"`
	Reference string `json:"reference"`
	Currency string `json:"currency"`
}

type caData struct {
	AuthorizationUrl string `json:"authorization_url"`
	AccessCode       string `json:"access_code"`
	Reference        string `json:"reference"`
}

type caResponse struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Data    caData `json:"data"`
}

func chargeAuthorization(
	ctx context.Context,
	amount int64,
	email string,
	reference string,
) (caResponse, error) {
	if cfg == nil {
		return caResponse{}, errors.New("payment service is not initialized")
	}

	if strings.TrimSpace(cfg.paystackSecretKey) == "" {
		return caResponse{}, errors.New("Paystack secret key is not configured")
	}

	body := caBody{
		Amount:    amount,
		Email:     email,
		Reference: reference,
		Currency:  "NGN",
	}

	data, err := json.Marshal(body)
	if err != nil {
		if cfg.logger != nil {
			cfg.logger.Error("failed to encode Paystack request body", "err", err)
		}
		return caResponse{}, fmt.Errorf("encode Paystack request: %w", err)
	}

	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", cfg.paystackSecretKey),
		"Content-Type":  "application/json",
		"Accept":        "application/json",
	}

	res, err := SendRequest(ctx, caURL, "POST", data, headers)
	if err != nil {
		if cfg.logger != nil {
			cfg.logger.Error("failed to initialize Paystack transaction", "err", err)
		}
		return caResponse{}, fmt.Errorf("send Paystack request: %w", err)
	}

	var resBody caResponse
	if err := json.Unmarshal(res.Body, &resBody); err != nil {
		if cfg.logger != nil {
			cfg.logger.Error("failed to decode Paystack response", "err", err)
		}
		return caResponse{}, fmt.Errorf("decode Paystack response: %w", err)
	}

	if !resBody.Status {
		return caResponse{}, fmt.Errorf("Paystack initialization failed: %s", resBody.Message)
	}

	if strings.TrimSpace(resBody.Data.AuthorizationUrl) == "" {
		return caResponse{}, errors.New("Paystack returned an empty authorization URL")
	}

	if resBody.Data.Reference != reference {
		return caResponse{}, errors.New("Paystack returned a mismatched transaction reference")
	}

	return resBody, nil
}
