
package payments

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
)

func TestChargeAuthorization(t *testing.T) {
	originalCfg := cfg
	originalSendRequest := sendRequest

	t.Cleanup(func() {
		cfg = originalCfg
		sendRequest = originalSendRequest
	})

	tests := []struct {
		name          string
		setup         func(t *testing.T)
		wantErr       bool
		wantErrString string
	}{
		{
			name: "service not initialized",
			setup: func(t *testing.T) {
				cfg = nil
			},
			wantErr:       true,
			wantErrString: "payment service is not initialized",
		},
		{
			name: "missing Paystack secret key",
			setup: func(t *testing.T) {
				cfg = &config{}
			},
			wantErr:       true,
			wantErrString: "Paystack secret key is not configured",
		},
		{
			name: "whitespace Paystack secret key",
			setup: func(t *testing.T) {
				cfg = &config{paystackSecretKey: "   "}
			},
			wantErr:       true,
			wantErrString: "Paystack secret key is not configured",
		},
		{
			name: "request failure",
			setup: func(t *testing.T) {
				cfg = &config{paystackSecretKey: "test-secret"}
				sendRequest = func(
					ctx context.Context,
					url string,
					method string,
					body []byte,
					headers map[string]string,
				) (*Response, error) {
					return nil, errors.New("network error")
				}
			},
			wantErr:       true,
			wantErrString: "send Paystack request",
		},
		{
			name: "invalid JSON response",
			setup: func(t *testing.T) {
				cfg = &config{paystackSecretKey: "test-secret"}
				sendRequest = func(
					ctx context.Context,
					url string,
					method string,
					body []byte,
					headers map[string]string,
				) (*Response, error) {
					return &Response{
						StatusCode: 200,
						Body:       []byte("invalid JSON"),
					}, nil
				}
			},
			wantErr:       true,
			wantErrString: "decode Paystack response",
		},
		{
			name: "Paystack initialization rejected",
			setup: func(t *testing.T) {
				cfg = &config{paystackSecretKey: "test-secret"}
				sendRequest = func(
					ctx context.Context,
					url string,
					method string,
					body []byte,
					headers map[string]string,
				) (*Response, error) {
					return &Response{
						StatusCode: 400,
						Body: []byte(`{
							"status": false,
							"message": "Invalid amount",
							"data": {}
						}`),
					}, nil
				}
			},
			wantErr:       true,
			wantErrString: "Paystack initialization failed: Invalid amount",
		},
		{
			name: "empty authorization URL",
			setup: func(t *testing.T) {
				cfg = &config{paystackSecretKey: "test-secret"}
				sendRequest = func(
					ctx context.Context,
					url string,
					method string,
					body []byte,
					headers map[string]string,
				) (*Response, error) {
					return &Response{
						StatusCode: 200,
						Body: []byte(`{
							"status": true,
							"message": "Success",
							"data": {
								"authorization_url": "",
								"access_code": "access-123",
								"reference": "ref-123"
							}
						}`),
					}, nil
				}
			},
			wantErr:       true,
			wantErrString: "Paystack returned an empty authorization URL",
		},
		{
			name: "mismatched transaction reference",
			setup: func(t *testing.T) {
				cfg = &config{paystackSecretKey: "test-secret"}
				sendRequest = func(
					ctx context.Context,
					url string,
					method string,
					body []byte,
					headers map[string]string,
				) (*Response, error) {
					return &Response{
						StatusCode: 200,
						Body: []byte(`{
							"status": true,
							"message": "Success",
							"data": {
								"authorization_url": "https://checkout.paystack.com/test",
								"access_code": "access-123",
								"reference": "wrong-reference"
							}
						}`),
					}, nil
				}
			},
			wantErr:       true,
			wantErrString: "Paystack returned a mismatched transaction reference",
		},
		{
			name: "successful initialization",
			setup: func(t *testing.T) {
				cfg = &config{paystackSecretKey: "test-secret"}
				sendRequest = func(
					ctx context.Context,
					url string,
					method string,
					body []byte,
					headers map[string]string,
				) (*Response, error) {
					if url != caURL {
						t.Errorf("request URL = %q, want %q", url, caURL)
					}

					if method != "POST" {
						t.Errorf("request method = %q, want POST", method)
					}

					if headers["Authorization"] != "Bearer test-secret" {
						t.Errorf("Authorization header = %q", headers["Authorization"])
					}

					if headers["Content-Type"] != "application/json" {
						t.Errorf("Content-Type header = %q", headers["Content-Type"])
					}

					if headers["Accept"] != "application/json" {
						t.Errorf("Accept header = %q", headers["Accept"])
					}

					var request caBody
					if err := json.Unmarshal(body, &request); err != nil {
						t.Fatalf("decode request body: %v", err)
					}

					if request.Amount != 50000 {
						t.Errorf("request amount = %d, want 50000", request.Amount)
					}

					if request.Email != "customer@example.com" {
						t.Errorf("request email = %q", request.Email)
					}

					if request.Reference != "ref-123" {
						t.Errorf("request reference = %q", request.Reference)
					}

					if request.Currency != "NGN" {
						t.Errorf("request currency = %q, want NGN", request.Currency)
					}

					return &Response{
						StatusCode: 200,
						Body: []byte(`{
							"status": true,
							"message": "Authorization URL created",
							"data": {
								"authorization_url": "https://checkout.paystack.com/test",
								"access_code": "access-123",
								"reference": "ref-123"
							}
						}`),
					}, nil
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg = originalCfg
			sendRequest = originalSendRequest

			tt.setup(t)

			got, err := chargeAuthorization(
				context.Background(),
				50000,
				"customer@example.com",
				"ref-123",
			)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}

				if !strings.Contains(err.Error(), tt.wantErrString) {
					t.Fatalf(
						"error = %q, want substring %q",
						err.Error(),
						tt.wantErrString,
					)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !got.Status {
				t.Error("response status = false, want true")
			}

			if got.Data.AuthorizationUrl != "https://checkout.paystack.com/test" {
				t.Errorf("authorization URL = %q", got.Data.AuthorizationUrl)
			}

			if got.Data.AccessCode != "access-123" {
				t.Errorf("access code = %q", got.Data.AccessCode)
			}

			if got.Data.Reference != "ref-123" {
				t.Errorf("reference = %q", got.Data.Reference)
			}
		})
	}
}

func TestChargeAuthorizationWithLogger(t *testing.T) {
	originalCfg := cfg
	originalSendRequest := sendRequest

	t.Cleanup(func() {
		cfg = originalCfg
		sendRequest = originalSendRequest
	})

	cfg = &config{
		paystackSecretKey: "test-secret",
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	sendRequest = func(
		ctx context.Context,
		url string,
		method string,
		body []byte,
		headers map[string]string,
	) (*Response, error) {
		return nil, errors.New("network error")
	}

	_, err := chargeAuthorization(
		context.Background(),
		50000,
		"customer@example.com",
		"ref-123",
	)

	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}
