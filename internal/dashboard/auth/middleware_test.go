package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestExtractTokenFromCookie(t *testing.T) {
	store := newTestStore(t)
	mw := NewMiddleware(store)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "abc123session"})

	if got := mw.ExtractToken(req); got != "abc123session" {
		t.Fatalf("ExtractToken() = %q, want abc123session", got)
	}
}

func TestExtractTokenPrefersHeaderOverCookie(t *testing.T) {
	store := newTestStore(t)
	mw := NewMiddleware(store)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req.Header.Set("Authorization", "Bearer header-token")
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "cookie-token"})

	if got := mw.ExtractToken(req); got != "header-token" {
		t.Fatalf("ExtractToken() = %q, want header-token", got)
	}
}

func TestSetAndClearSessionCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	rec := httptest.NewRecorder()

	SetSessionCookie(rec, req, "session-token-value", time.Hour)

	res := rec.Result()
	cookies := res.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != SessionCookieName {
		t.Fatalf("cookie name = %q, want %q", cookie.Name, SessionCookieName)
	}
	if cookie.Value != "session-token-value" {
		t.Fatalf("cookie value = %q", cookie.Value)
	}
	if !cookie.HttpOnly {
		t.Fatal("expected HttpOnly cookie")
	}
	if cookie.Path != "/" {
		t.Fatalf("cookie path = %q, want /", cookie.Path)
	}
	if cookie.MaxAge <= 0 {
		t.Fatalf("cookie MaxAge = %d, want > 0", cookie.MaxAge)
	}

	clearRec := httptest.NewRecorder()
	ClearSessionCookie(clearRec, req)
	clearCookies := clearRec.Result().Cookies()
	if len(clearCookies) != 1 {
		t.Fatalf("expected 1 cleared cookie, got %d", len(clearCookies))
	}
	if clearCookies[0].MaxAge != -1 {
		t.Fatalf("cleared cookie MaxAge = %d, want -1", clearCookies[0].MaxAge)
	}
}

func TestTouchSessionExtendsExpiry(t *testing.T) {
	store := newTestStore(t)

	user, err := store.CreateAdminUser("admin", "temp-pass")
	if err != nil {
		t.Fatalf("CreateAdminUser: %v", err)
	}

	login, err := store.Login("admin", "temp-pass")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	tokenHash := hashToken(login.Token)
	var expiresBefore time.Time
	if err := store.db.QueryRow(
		`SELECT expires_at FROM dashboard_sessions WHERE token_hash = ?`,
		tokenHash,
	).Scan(&expiresBefore); err != nil {
		t.Fatalf("query expires_at: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	if err := store.TouchSession(login.Token); err != nil {
		t.Fatalf("TouchSession: %v", err)
	}

	var expiresAfter time.Time
	if err := store.db.QueryRow(
		`SELECT expires_at FROM dashboard_sessions WHERE token_hash = ?`,
		tokenHash,
	).Scan(&expiresAfter); err != nil {
		t.Fatalf("query expires_at after touch: %v", err)
	}

	if !expiresAfter.After(expiresBefore) {
		t.Fatalf("expected extended expiry, before=%v after=%v", expiresBefore, expiresAfter)
	}

	sessionUser, err := store.ValidateSession(login.Token)
	if err != nil {
		t.Fatalf("ValidateSession: %v", err)
	}
	if sessionUser.ID != user.ID {
		t.Fatalf("session user id = %d, want %d", sessionUser.ID, user.ID)
	}
}
