package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExtractAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		headers  map[string]string
		expected string
	}{
		{
			name:     "X-API-Key header",
			headers:  map[string]string{"X-API-Key": "secret-key"},
			expected: "secret-key",
		},
		{
			name:     "Bearer authorization",
			headers:  map[string]string{"Authorization": "Bearer bearer-key"},
			expected: "bearer-key",
		},
		{
			name:     "X-API-Key preferred over Bearer",
			headers:  map[string]string{"X-API-Key": "header-key", "Authorization": "Bearer bearer-key"},
			expected: "header-key",
		},
		{
			name:     "missing key",
			headers:  map[string]string{},
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			if got := ExtractAPIKey(req); got != tc.expected {
				t.Fatalf("ExtractAPIKey() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestValidAPIKey(t *testing.T) {
	if !ValidAPIKey("secret", "secret") {
		t.Fatal("expected matching keys to be valid")
	}
	if ValidAPIKey("wrong", "secret") {
		t.Fatal("expected mismatched keys to be invalid")
	}
	if !ValidAPIKey("", "") {
		t.Fatal("expected empty expected key to allow access (dev mode)")
	}
	if !ValidAPIKey("anything", "") {
		t.Fatal("expected dev mode to accept any provided key")
	}
}

func TestWithAPIKeyUnauthorized(t *testing.T) {
	handler := WithAPIKey("secret", "trinity-api", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(rec.Body.String(), "invalid or missing API key") {
		t.Fatalf("body = %q, want auth error", rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("WWW-Authenticate"), `realm="trinity-api"`) {
		t.Fatalf("WWW-Authenticate = %q, want trinity-api realm", rec.Header().Get("WWW-Authenticate"))
	}
}

func TestWithAPIKeyAuthorized(t *testing.T) {
	called := false
	handler := WithAPIKey("secret", "trinity-api", func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name    string
		setAuth func(*http.Request)
	}{
		{
			name: "X-API-Key",
			setAuth: func(r *http.Request) {
				r.Header.Set("X-API-Key", "secret")
			},
		},
		{
			name: "Bearer token",
			setAuth: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer secret")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			called = false
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			tc.setAuth(req)
			rec := httptest.NewRecorder()

			handler(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if !called {
				t.Fatal("expected inner handler to be called")
			}
		})
	}
}

func TestWithAPIKeyDevMode(t *testing.T) {
	called := false
	handler := WithAPIKey("", "trinity-api", func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !called {
		t.Fatal("expected handler to run without API key in dev mode")
	}
}
