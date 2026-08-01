package dashboard

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	dashauth "github.com/Skillz147/TrinityProxy/internal/dashboard/auth"
	"github.com/Skillz147/TrinityProxy/internal/dashboard/deployment"
	"github.com/Skillz147/TrinityProxy/internal/storage"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()

	dir := t.TempDir()
	authStore, err := dashauth.NewStore(filepath.Join(dir, "auth.db"), time.Hour)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = authStore.Close() })

	if _, err := authStore.CreateAdminUser("admin", "temp-pass"); err != nil {
		t.Fatalf("CreateAdminUser: %v", err)
	}

	deployStore, err := deployment.NewStore(filepath.Join(dir, "deploy.db"))
	if err != nil {
		t.Fatalf("deployment NewStore: %v", err)
	}
	t.Cleanup(func() { _ = deployStore.Close() })

	nodes, err := storage.NewNodeStorage(filepath.Join(dir, "nodes.db"))
	if err != nil {
		t.Fatalf("NewNodeStorage: %v", err)
	}
	t.Cleanup(func() { _ = nodes.Close() })

	cfg := Config{SessionTTL: time.Hour}
	return NewServer(cfg, authStore, deployStore, nodes, nil)
}

func TestAuthSessionPersistsViaCookie(t *testing.T) {
	server := newTestServer(t)
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	loginBody, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": "temp-pass",
	})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(loginBody))
	loginRec := httptest.NewRecorder()
	mux.ServeHTTP(loginRec, loginReq)

	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", loginRec.Code, loginRec.Body.String())
	}

	loginRes := loginRec.Result()
	sessionCookie := findCookie(loginRes.Cookies(), dashauth.SessionCookieName)
	if sessionCookie == nil || sessionCookie.Value == "" {
		t.Fatal("expected session cookie on login response")
	}
	if !sessionCookie.HttpOnly {
		t.Fatal("expected HttpOnly session cookie")
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	meReq.AddCookie(sessionCookie)
	meRec := httptest.NewRecorder()
	mux.ServeHTTP(meRec, meReq)

	if meRec.Code != http.StatusOK {
		t.Fatalf("/me status = %d, body = %s", meRec.Code, meRec.Body.String())
	}

	var mePayload struct {
		User struct {
			Username string `json:"username"`
		} `json:"user"`
	}
	if err := json.Unmarshal(meRec.Body.Bytes(), &mePayload); err != nil {
		t.Fatalf("decode /me: %v", err)
	}
	if mePayload.User.Username != "admin" {
		t.Fatalf("username = %q, want admin", mePayload.User.Username)
	}

	refreshedCookie := findCookie(meRec.Result().Cookies(), dashauth.SessionCookieName)
	if refreshedCookie == nil || refreshedCookie.MaxAge <= 0 {
		t.Fatal("expected refreshed session cookie on /me")
	}

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	logoutReq.AddCookie(sessionCookie)
	logoutRec := httptest.NewRecorder()
	mux.ServeHTTP(logoutRec, logoutReq)

	if logoutRec.Code != http.StatusOK {
		t.Fatalf("logout status = %d", logoutRec.Code)
	}

	clearedCookie := findCookie(logoutRec.Result().Cookies(), dashauth.SessionCookieName)
	if clearedCookie == nil || clearedCookie.MaxAge != -1 {
		t.Fatal("expected cleared session cookie on logout")
	}

	meAfterLogoutReq := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	meAfterLogoutReq.AddCookie(sessionCookie)
	meAfterLogoutRec := httptest.NewRecorder()
	mux.ServeHTTP(meAfterLogoutRec, meAfterLogoutReq)

	if meAfterLogoutRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 after logout, got %d", meAfterLogoutRec.Code)
	}
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func TestAuthMeWithoutCookieOrHeader(t *testing.T) {
	server := newTestServer(t)
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	if len(body) == 0 {
		t.Fatal("expected error body")
	}
}
