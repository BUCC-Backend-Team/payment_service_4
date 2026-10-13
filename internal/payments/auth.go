
package payments

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

func authenticateMerchant(w http.ResponseWriter, r *http.Request) bool {
	expectedKey := cfg.merchantApiKey

	if expectedKey == "" {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "merchant authentication is not configured",
		})
		return false
	}

	parts := strings.Fields(r.Header.Get("Authorization"))

	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "missing or invalid merchant authorization",
		})
		return false
	}

	providedKey := parts[1]

	if subtle.ConstantTimeCompare([]byte(providedKey), []byte(expectedKey)) != 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "invalid merchant API key",
		})
		return false
	}

	return true
}
