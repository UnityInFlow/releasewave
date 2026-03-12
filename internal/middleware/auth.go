package middleware

import (
	"crypto/subtle"
	"net/http"
)

// APIKeyAuth returns middleware that validates API key authentication.
// It checks Authorization: Bearer <key>, X-API-Key: <key>, and ?api_key=<key>.
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

			// Check query parameter
			if key == "" {
				key = r.URL.Query().Get("api_key")
			}

			if key == "" {
				http.Error(w, `{"error":"missing API key"}`, http.StatusUnauthorized)
				return
			}

			if subtle.ConstantTimeCompare([]byte(key), expected) != 1 {
				http.Error(w, `{"error":"invalid API key"}`, http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
