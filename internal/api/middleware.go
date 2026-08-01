package api

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// ExtractAPIKey reads the API key from X-API-Key or Authorization: Bearer headers.
func ExtractAPIKey(r *http.Request) string {
	if key := strings.TrimSpace(r.Header.Get("X-API-Key")); key != "" {
		return key
	}

	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	const prefix = "Bearer "
	if len(auth) >= len(prefix) && strings.EqualFold(auth[:len(prefix)], prefix) {
		return strings.TrimSpace(auth[len(prefix):])
	}

	return ""
}

// ValidAPIKey compares provided and expected keys in constant time.
func ValidAPIKey(provided, expected string) bool {
	if expected == "" {
		return true
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

// WithAPIKey wraps an HTTP handler with API key authentication.
// When expectedKey is empty, the handler is exposed without auth (development mode).
func WithAPIKey(expectedKey, realm string, handler http.HandlerFunc) http.HandlerFunc {
	if expectedKey == "" {
		return handler
	}

	return func(w http.ResponseWriter, r *http.Request) {
		provided := ExtractAPIKey(r)
		if !ValidAPIKey(provided, expectedKey) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="`+realm+`"`)
			http.Error(w, "invalid or missing API key", http.StatusUnauthorized)
			return
		}
		handler(w, r)
	}
}
