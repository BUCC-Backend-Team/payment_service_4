
package payments

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthenticateMerchant(t *testing.T) {
	const validKey = "mkt_test_7f9c2a8d4e6b1f305a7c9d2e8b4f6103"

	tests := []struct {
		name           string
		expectedKey    string
		authHeader     string
		wantAuthenticated bool
		wantStatus     int
	}{
		{
			name:              "valid bearer token",
			expectedKey:       validKey,
			authHeader:        "Bearer " + validKey,
			wantAuthenticated: true,
			wantStatus:        http.StatusOK,
		},
		{
			name:              "valid bearer token with lowercase scheme",
			expectedKey:       validKey,
			authHeader:        "bearer " + validKey,
			wantAuthenticated: true,
			wantStatus:        http.StatusOK,
		},
		{
			name:              "missing authorization header",
			expectedKey:       validKey,
			authHeader:        "",
			wantAuthenticated: false,
			wantStatus:        http.StatusUnauthorized,
		},
		{
			name:              "invalid api key",
			expectedKey:       validKey,
			authHeader:        "Bearer wrong-key",
			wantAuthenticated: false,
			wantStatus:        http.StatusUnauthorized,
		},
		{
			name:              "missing bearer scheme",
			expectedKey:       validKey,
			authHeader:        validKey,
			wantAuthenticated: false,
			wantStatus:        http.StatusUnauthorized,
		},
		{
			name:              "incorrect authorization scheme",
			expectedKey:       validKey,
			authHeader:        "Basic " + validKey,
			wantAuthenticated: false,
			wantStatus:        http.StatusUnauthorized,
		},
		{
			name:              "bearer without token",
			expectedKey:       validKey,
			authHeader:        "Bearer",
			wantAuthenticated: false,
			wantStatus:        http.StatusUnauthorized,
		},
		{
			name:              "extra authorization fields",
			expectedKey:       validKey,
			authHeader:        "Bearer " + validKey + " extra",
			wantAuthenticated: false,
			wantStatus:        http.StatusUnauthorized,
		},
		{
			name:              "unconfigured merchant api key",
			expectedKey:       "",
			authHeader:        "Bearer " + validKey,
			wantAuthenticated: false,
			wantStatus:        http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalCfg := cfg

			t.Cleanup(func() {
				cfg = originalCfg
			})

			cfg = &config{
				merchantApiKey: tt.expectedKey,
			}

			req := httptest.NewRequest(http.MethodPost, "/api/payments", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			rec := httptest.NewRecorder()

			gotAuthenticated := authenticateMerchant(rec, req)

			if gotAuthenticated != tt.wantAuthenticated {
				t.Errorf(
					"authenticateMerchant() = %v, want %v",
					gotAuthenticated,
					tt.wantAuthenticated,
				)
			}

			if got := rec.Code; got != tt.wantStatus {
				t.Errorf(
					"HTTP status = %d, want %d",
					got,
					tt.wantStatus,
				)
			}

			if tt.wantAuthenticated {
				if rec.Body.Len() != 0 {
					t.Errorf(
						"successful authentication wrote unexpected response: %s",
						rec.Body.String(),
					)
				}
				return
			}

			if got := rec.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf(
					"Content-Type = %q, want application/json",
					got,
				)
			}

			if rec.Body.Len() == 0 {
				t.Error("expected an error response body")
			}
		})
	}
}
