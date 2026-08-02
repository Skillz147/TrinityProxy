package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Skillz147/TrinityProxy/internal/storage"
)

func TestHandleRegenerateNodeToken(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewNodeStorage(filepath.Join(dir, "nodes.db"))
	if err != nil {
		t.Fatalf("NewNodeStorage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	node := &storage.ProxyNode{
		IP: "203.0.113.55", Port: 1080,
		Username: "u", Password: "p", Country: "US",
	}
	if err := store.UpsertNode(node); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	s := &Server{nodes: store}

	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/nodes/203.0.113.55:1080/regenerate-token", nil)
	req.SetPathValue("id", "203.0.113.55:1080")
	rec := httptest.NewRecorder()
	s.handleRegenerateNodeToken(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		NodeToken string `json:"node_token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.NodeToken == "" {
		t.Fatal("expected node_token in response")
	}

	ok, err := store.ValidateNodeToken("203.0.113.55:1080", resp.NodeToken)
	if err != nil || !ok {
		t.Fatalf("ValidateNodeToken after regenerate: ok=%v err=%v", ok, err)
	}
}

func TestHandleRevokeNodeToken(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewNodeStorage(filepath.Join(dir, "nodes.db"))
	if err != nil {
		t.Fatalf("NewNodeStorage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	node := &storage.ProxyNode{
		IP: "203.0.113.56", Port: 1080,
		Username: "u", Password: "p", Country: "US",
	}
	if err := store.UpsertNode(node); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}
	if _, err := store.IssueNodeToken("203.0.113.56:1080"); err != nil {
		t.Fatalf("IssueNodeToken: %v", err)
	}

	s := &Server{nodes: store}
	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/nodes/203.0.113.56:1080/revoke-token", nil)
	req.SetPathValue("id", "203.0.113.56:1080")
	rec := httptest.NewRecorder()
	s.handleRevokeNodeToken(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	has, err := store.NodeHasToken("203.0.113.56:1080")
	if err != nil || has {
		t.Fatalf("NodeHasToken after revoke = %v, err = %v", has, err)
	}
}
