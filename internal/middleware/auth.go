package middleware

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
)

// APIKeyAuth returns middleware that validates API key authentication.
// It checks Authorization: Bearer <key> and X-API-Key: <key> headers.
// If apiKey is empty, the middleware is a pass-through (no auth required).
func APIKeyAuth(apiKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if apiKey == "" {
			return next
		}

		expected := []byte(apiKey)

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var key string

			// Check Authorization: Bearer <key>
			if auth := r.Header.Get("Authorization"); len(auth) > 7 && auth[:7] == "Bearer " {
				key = auth[7:]
			}

			// Check X-API-Key header
			if key == "" {
				key = r.Header.Get("X-API-Key")
			}

			if key == "" {
				writeJSONError(w, http.StatusUnauthorized, "missing API key")
				return
			}

			if subtle.ConstantTimeCompare([]byte(key), expected) != 1 {
				writeJSONError(w, http.StatusUnauthorized, "invalid API key")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	b, _ := json.Marshal(map[string]string{"error": msg})
	_, _ = w.Write(b)
}
