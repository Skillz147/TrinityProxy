package agent

import (
	"path/filepath"
	"testing"
)

func TestResolveAuthTokenPriority(t *testing.T) {
	t.Setenv("TRINITY_NODE_TOKEN", "env-node-token")
	t.Setenv("TRINITY_ENROLLMENT_KEY", "enroll")
	t.Setenv("TRINITY_AGENT_KEY", "legacy")

	if got := ResolveAuthToken(); got != "env-node-token" {
		t.Fatalf("ResolveAuthToken = %q, want env-node-token", got)
	}
}

func TestSaveAndLoadNodeToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "node-token")
	t.Setenv("TRINITY_NODE_TOKEN_FILE", path)
	t.Setenv("TRINITY_NODE_TOKEN", "")
	t.Setenv("TRINITY_ENROLLMENT_KEY", "")
	t.Setenv("TRINITY_AGENT_KEY", "")

	if err := SaveNodeToken("per-node-secret"); err != nil {
		t.Fatalf("SaveNodeToken: %v", err)
	}

	got, err := LoadNodeToken()
	if err != nil || got != "per-node-secret" {
		t.Fatalf("LoadNodeToken = %q, err = %v", got, err)
	}

	if token := ResolveAuthToken(); token != "per-node-secret" {
		t.Fatalf("ResolveAuthToken = %q, want persisted token", token)
	}
}

func TestResolveAuthTokenEnrollmentFallback(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TRINITY_NODE_TOKEN_FILE", filepath.Join(dir, "missing"))
	t.Setenv("TRINITY_NODE_TOKEN", "")
	t.Setenv("TRINITY_ENROLLMENT_KEY", "enroll-key")
	t.Setenv("TRINITY_AGENT_KEY", "legacy-key")

	if got := ResolveAuthToken(); got != "enroll-key" {
		t.Fatalf("ResolveAuthToken = %q, want enroll-key", got)
	}
}

func TestResolveAuthTokenLegacyFallback(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TRINITY_NODE_TOKEN_FILE", filepath.Join(dir, "missing"))
	t.Setenv("TRINITY_NODE_TOKEN", "")
	t.Setenv("TRINITY_ENROLLMENT_KEY", "")
	t.Setenv("TRINITY_AGENT_KEY", "legacy-key")

	if got := ResolveAuthToken(); got != "legacy-key" {
		t.Fatalf("ResolveAuthToken = %q, want legacy-key", got)
	}
}

func TestSaveNodeTokenRejectsEmpty(t *testing.T) {
	if err := SaveNodeToken("  "); err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestLoadNodeTokenMissingFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TRINITY_NODE_TOKEN_FILE", filepath.Join(dir, "missing"))
	got, err := LoadNodeToken()
	if err != nil || got != "" {
		t.Fatalf("LoadNodeToken missing: got=%q err=%v", got, err)
	}
}
