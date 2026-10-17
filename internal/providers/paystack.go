package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

//whether a failed Paystack call can be retried safely.
type Class int

const (
	ClassReject   Class = iota
	ClassRetry
	ClassAmbiguous
)

type ProviderCallError struct {
	Class     Class
	HTTPStat  int
	Message   string
	Retryable bool
}

func (e *ProviderCallError) Error() string {
	return fmt.Sprintf("paystack call failed: class=%d http=%d message=%q retryable=%t", e.Class, e.HTTPStat, e.Message, e.Retryable)
}

// RetriablePaystackErr tells callers whether a failed call should be retried.
func RetriablePaystackErr(err error) bool {
	e, ok := err.(*ProviderCallError)
	if !ok {
		return false
	}
	return e.Class != ClassReject
}

func IsRetryable(err error) bool {
	call, ok := err.(*ProviderCallError)
	if !ok {
		return true
	}
	return call.Class != ClassReject
}

func IsRejection(err error) bool {
	e, ok := err.(*ProviderCallError)
	if !ok {
		return false
	}
	return e.Class == ClassReject
}

type PaystackProvider struct {
	SecretKey string
	BaseURL   string
	HTTP      *http.Client
	Logger    *slog.Logger
}

const defaultPaystackBaseURL = "https://api.paystack.co"

func NewPaystack(secretKey string, logger *slog.Logger) *PaystackProvider {
	baseURL := strings.TrimSpace(os.Getenv("PAYSTACK_BASE_URL"))
	if baseURL == "" {
		baseURL = defaultPaystackBaseURL
	}

	return &PaystackProvider{
		SecretKey: secretKey,
		BaseURL:   baseURL,
		HTTP: &http.Client{
			Timeout: 30 * time.Second,
		},
		Logger: logger,
	}
}

type paystackEnvelope struct {
	Status  bool            `json:"status"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type paystackInitData struct {
	AuthorizationURL string `json:"authorization_url"`
	AccessCode       string `json:"access_code"`
	Reference        string `json:"reference"`
}

type paystackVerifyData struct {
	Reference string `json:"reference"`
	Status    string `json:"status"`
	Amount    int64  `json:"amount"`
	Currency  string `json:"currency"`
	Channel   string `json:"channel"`
}

type paystackRefundData struct {
	ID       int64  `json:"id"`
	Status   string `json:"status"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

func (p *PaystackProvider) do(ctx context.Context, method, path string, payload any) (paystackEnvelope, error) {
	var bodyReader io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return paystackEnvelope{}, fmt.Errorf("provider: encode request: %w", err)
		}
		bodyReader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, p.BaseURL+path, bodyReader)
	if err != nil {
		return paystackEnvelope{}, fmt.Errorf("provider: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.SecretKey)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := p.HTTP.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || isNetTimeout(err) {
			return paystackEnvelope{}, providerErr(ClassAmbiguous, 0, "request timed out")
		}
		return paystackEnvelope{}, providerErr(ClassRetry, 0, err.Error())
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return paystackEnvelope{}, providerErr(ClassAmbiguous, resp.StatusCode, "unreadable Paystack response")
	}

	var envelope paystackEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return paystackEnvelope{}, providerErr(ClassAmbiguous, resp.StatusCode, fmt.Sprintf("bad Paystack response: %v", err))
	}

	if resp.StatusCode >= 500 {
		return paystackEnvelope{}, providerErr(ClassAmbiguous, resp.StatusCode, envelope.Message)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return paystackEnvelope{}, providerErr(ClassRetry, resp.StatusCode, envelope.Message)
	}
	if resp.StatusCode >= 400 {
		return paystackEnvelope{}, providerErr(ClassReject, resp.StatusCode, envelope.Message)
	}
	if !envelope.Status {
		return paystackEnvelope{}, providerErr(ClassReject, resp.StatusCode, envelope.Message)
	}

	return envelope, nil
}

func providerErr(class Class, status int, message string) *ProviderCallError {
	return &ProviderCallError{
		Class:     class,
		HTTPStat:  status,
		Message:   message,
		Retryable: class != ClassReject,
	}
}

func isNetTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func (p *PaystackProvider) InitializeTransaction(ctx context.Context, req InitializeRequest) (Checkout, error) {
	envelope, err := p.do(ctx, http.MethodPost, "/transaction/initialize", map[string]any{
		"email":        req.Email,
		"amount":       req.Amount,
		"currency":     req.Currency,
		"reference":    req.Reference,
		"callback_url": req.CallbackURL,
		"channels":     []string{"card", "bank_transfer"},
	})
	if err != nil {
		return Checkout{}, err
	}

	var data paystackInitData
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		return Checkout{}, providerErr(ClassAmbiguous, 0, "bad Paystack initialize payload")
	}

	if strings.TrimSpace(data.AuthorizationURL) == "" {
		return Checkout{}, providerErr(ClassAmbiguous, 0, "Paystack returned no authorization URL")
	}
	if data.Reference != req.Reference {
		return Checkout{}, providerErr(ClassAmbiguous, 0, "Paystack returned a mismatched reference")
	}

	return Checkout{
		AuthorizationURL: data.AuthorizationURL,
		AccessCode:       data.AccessCode,
		Reference:        data.Reference,
	}, nil
}

func (p *PaystackProvider) VerifyTransaction(ctx context.Context, reference string) (Transaction, error) {
	envelope, err := p.do(ctx, http.MethodGet, "/transaction/verify/"+url.PathEscape(reference), nil)
	if err != nil {
		return Transaction{}, err
	}

	var data paystackVerifyData
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		return Transaction{}, providerErr(ClassAmbiguous, 0, "bad Paystack verify payload")
	}

	return Transaction{
		Reference: data.Reference,
		Status:    data.Status,
		Amount:    data.Amount,
		Currency:  data.Currency,
		Channel:   data.Channel,
	}, nil
}

func (p *PaystackProvider) CreateRefund(ctx context.Context, req RefundRequest) (Refund, error) {
	envelope, err := p.do(ctx, http.MethodPost, "/refund", map[string]any{
		"transaction": req.Transaction,
		"amount":      req.Amount,
	})
	if err != nil {
		return Refund{}, err
	}

	var data paystackRefundData
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		return Refund{}, providerErr(ClassAmbiguous, 0, "bad Paystack refund payload")
	}

	return Refund{
		ID:     data.ID,
		Status: data.Status,
		Amount: data.Amount,
	}, nil
}
