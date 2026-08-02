package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Skillz147/TrinityProxy/internal/storage"
)

func TestValidateAgentAccessEnrollAndNodeToken(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewNodeStorage(filepath.Join(dir, "nodes.db"))
	if err != nil {
		t.Fatalf("NewNodeStorage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	node := &storage.ProxyNode{
		IP: "203.0.113.77", Port: 1080,
		Username: "u", Password: "p", Country: "US",
	}
	if err := store.UpsertNode(node); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	cfg := AgentAuthConfig{
		EnrollmentKey: "enroll-secret",
		Log:           slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	nodeID := "203.0.113.77:1080"

	enroll, err := cfg.ValidateAgentAccess(store, nodeID, "enroll-secret")
	if err != nil || !enroll {
		t.Fatalf("first access enroll=%v err=%v", enroll, err)
	}

	token, err := store.IssueNodeToken(nodeID)
	if err != nil {
		t.Fatalf("IssueNodeToken: %v", err)
	}

	enroll, err = cfg.ValidateAgentAccess(store, nodeID, token)
	if err != nil || enroll {
		t.Fatalf("node token access enroll=%v err=%v", enroll, err)
	}

	_, err = cfg.ValidateAgentAccess(store, nodeID, "enroll-secret")
	if err != errInvalidAgentToken {
		t.Fatalf("fleet key on enrolled node: err=%v", err)
	}
}

func TestValidateAgentAccessHTTP(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewNodeStorage(filepath.Join(dir, "nodes.db"))
	if err != nil {
		t.Fatalf("NewNodeStorage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	cfg := AgentAuthConfig{EnrollmentKey: "enroll-secret"}

	rec := httptest.NewRecorder()
	enroll, ok := cfg.ValidateAgentAccessHTTP(rec, store, "1.2.3.4:1080", "bad")
	if ok || enroll {
		t.Fatalf("expected auth failure, ok=%v enroll=%v", ok, enroll)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestExtractAgentTokenPrefersXAgentToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Agent-Token", "node-token")
	req.Header.Set("Authorization", "Bearer fleet-key")

	if got := ExtractAgentToken(req); got != "node-token" {
		t.Fatalf("ExtractAgentToken = %q, want node-token", got)
	}
}

func TestLegacyAgentKeyIssuesDeprecationPath(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewNodeStorage(filepath.Join(dir, "nodes.db"))
	if err != nil {
		t.Fatalf("NewNodeStorage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	cfg := AgentAuthConfig{
		LegacyAgentKey: "legacy-fleet",
		Log:            slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	enroll, err := cfg.ValidateAgentAccess(store, "9.9.9.9:1080", "legacy-fleet")
	if err != nil || !enroll {
		t.Fatalf("legacy enroll=%v err=%v", enroll, err)
	}
}

func TestAuthDisabledAllowsEmptyToken(t *testing.T) {
	cfg := AgentAuthConfig{}
	if !cfg.AuthDisabled() {
		t.Fatal("expected auth disabled with no keys")
	}

	dir := t.TempDir()
	store, err := storage.NewNodeStorage(filepath.Join(dir, "nodes.db"))
	if err != nil {
		t.Fatalf("NewNodeStorage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	enroll, err := cfg.ValidateAgentAccess(store, "1.2.3.4:1080", "")
	if err != nil || enroll {
		t.Fatalf("dev mode enroll=%v err=%v", enroll, err)
	}
}

func TestNodeIDFromMeta(t *testing.T) {
	if got := NodeIDFromMeta(HeartbeatMeta{IP: " 203.0.113.1 ", Port: 1080}); got != "203.0.113.1:1080" {
		t.Fatalf("NodeIDFromMeta = %q", got)
	}
}

func TestParseHeartbeatMeta(t *testing.T) {
	body := `{"ip":"203.0.113.1","port":1080,"username":"u"}`
	req := httptest.NewRequest(http.MethodPost, "/api/heartbeat", strings.NewReader(body))
	meta, raw, err := ParseHeartbeatMeta(req)
	if err != nil {
		t.Fatalf("ParseHeartbeatMeta: %v", err)
	}
	if meta.IP != "203.0.113.1" || meta.Port != 1080 {
		t.Fatalf("meta = %+v", meta)
	}
	if string(raw) != body {
		t.Fatalf("raw body mismatch")
	}
}
