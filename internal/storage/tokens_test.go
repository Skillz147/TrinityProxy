package storage

import (
	"path/filepath"
	"testing"

	"github.com/Skillz147/TrinityProxy/internal/auth"
)

func TestNodeTokenLifecycle(t *testing.T) {
	dir := t.TempDir()
	store, err := NewNodeStorage(filepath.Join(dir, "nodes.db"))
	if err != nil {
		t.Fatalf("NewNodeStorage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	node := &ProxyNode{
		IP: "203.0.113.1", Port: 1080,
		Username: "u", Password: "p", Country: "US",
	}
	if err := store.UpsertNode(node); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}
	nodeID := "203.0.113.1:1080"

	has, err := store.NodeHasToken(nodeID)
	if err != nil {
		t.Fatalf("NodeHasToken: %v", err)
	}
	if has {
		t.Fatal("new node should not have token")
	}

	token, err := store.IssueNodeToken(nodeID)
	if err != nil {
		t.Fatalf("IssueNodeToken: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	ok, err := store.ValidateNodeToken(nodeID, token)
	if err != nil || !ok {
		t.Fatalf("ValidateNodeToken valid: ok=%v err=%v", ok, err)
	}

	ok, err = store.ValidateNodeToken(nodeID, "wrong-token")
	if err != nil || ok {
		t.Fatalf("ValidateNodeToken invalid: ok=%v err=%v", ok, err)
	}

	if err := store.RevokeNodeToken(nodeID); err != nil {
		t.Fatalf("RevokeNodeToken: %v", err)
	}
	has, err = store.NodeHasToken(nodeID)
	if err != nil || has {
		t.Fatalf("after revoke has=%v err=%v", has, err)
	}
}

func TestSetNodeTokenHashPreservesOtherFields(t *testing.T) {
	dir := t.TempDir()
	store, err := NewNodeStorage(filepath.Join(dir, "nodes.db"))
	if err != nil {
		t.Fatalf("NewNodeStorage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	node := &ProxyNode{
		IP: "203.0.113.2", Port: 1080,
		Username: "u", Password: "p", Country: "US",
	}
	if err := store.UpsertNode(node); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	hash := auth.HashNodeToken("test-token-value")
	if err := store.SetNodeTokenHash("203.0.113.2:1080", hash); err != nil {
		t.Fatalf("SetNodeTokenHash: %v", err)
	}

	got, err := store.GetNodeByID("203.0.113.2:1080")
	if err != nil || got == nil {
		t.Fatalf("GetNodeByID: %v", err)
	}
	if got.Username != "u" {
		t.Fatalf("username = %q, want u", got.Username)
	}
}
