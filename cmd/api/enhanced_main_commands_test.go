package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Skillz147/TrinityProxy/internal/api"
	"github.com/Skillz147/TrinityProxy/internal/storage"
)

func TestHandleHeartbeatIssuesNodeToken(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewNodeStorage(filepath.Join(dir, "nodes.db"))
	if err != nil {
		t.Fatalf("NewNodeStorage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	const enrollKey = "test-enrollment-key"
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := NewAPIServerWithAuth(store, api.AgentAuthConfig{
		EnrollmentKey: enrollKey,
		Log:           log,
	}, log, testProber())
	server.tokens = store

	body := `{"ip":"203.0.113.42","port":1080,"username":"u","password":"p","country":"US"}`
	req := httptest.NewRequest(http.MethodPost, "/api/heartbeat", strings.NewReader(body))
	req.Header.Set("X-Agent-Token", enrollKey)
	rec := httptest.NewRecorder()
	server.handleHeartbeat(rec, req)

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
		t.Fatalf("expected node_token in first heartbeat response, got %s", rec.Body.String())
	}

	has, err := store.NodeHasToken("203.0.113.42:1080")
	if err != nil || !has {
		t.Fatalf("NodeHasToken = %v, err = %v", has, err)
	}

	// Second heartbeat must use node token, not enrollment key
	req2 := httptest.NewRequest(http.MethodPost, "/api/heartbeat", strings.NewReader(body))
	req2.Header.Set("X-Agent-Token", enrollKey)
	rec2 := httptest.NewRecorder()
	server.handleHeartbeat(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("enrolled node with fleet key: status = %d, want 401", rec2.Code)
	}

	req3 := httptest.NewRequest(http.MethodPost, "/api/heartbeat", strings.NewReader(body))
	req3.Header.Set("X-Agent-Token", resp.NodeToken)
	rec3 := httptest.NewRecorder()
	server.handleHeartbeat(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("node token heartbeat: status = %d, body = %s", rec3.Code, rec3.Body.String())
	}
}

func TestHandleHeartbeatReturnsPendingCommands(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewNodeStorage(filepath.Join(dir, "nodes.db"))
	if err != nil {
		t.Fatalf("NewNodeStorage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	node := &storage.ProxyNode{
		IP: "203.0.113.42", Port: 1080,
		Username: "u", Password: "p", Country: "US",
	}
	if err := store.UpsertNode(node); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	cmd, err := store.EnqueueCommand("203.0.113.42:1080", storage.CommandActionStatus, nil)
	if err != nil {
		t.Fatalf("EnqueueCommand: %v", err)
	}

	server := newTestServer(store)
	body := `{"ip":"203.0.113.42","port":1080,"username":"u","password":"p","country":"US"}`
	req := httptest.NewRequest(http.MethodPost, "/api/heartbeat", strings.NewReader(body))
	rec := httptest.NewRecorder()
	server.handleHeartbeat(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		PendingCommands []struct {
			ID     string `json:"id"`
			Action string `json:"action"`
		} `json:"pending_commands"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.PendingCommands) != 1 {
		t.Fatalf("pending = %+v", resp.PendingCommands)
	}
	if resp.PendingCommands[0].ID != cmd.ID || resp.PendingCommands[0].Action != "status" {
		t.Fatalf("pending command = %+v", resp.PendingCommands[0])
	}

	running, err := store.GetLatestCommandForNode("203.0.113.42:1080")
	if err != nil {
		t.Fatalf("GetLatestCommandForNode: %v", err)
	}
	if running.Status != storage.CommandStatusRunning {
		t.Fatalf("status after heartbeat = %q, want running", running.Status)
	}
}

func TestHandleCommandResultUninstallDeletesNode(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewNodeStorage(filepath.Join(dir, "nodes.db"))
	if err != nil {
		t.Fatalf("NewNodeStorage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	node := &storage.ProxyNode{
		IP: "203.0.113.99", Port: 1080,
		Username: "u", Password: "p", Country: "US",
	}
	if err := store.UpsertNode(node); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	cmd, err := store.EnqueueCommand("203.0.113.99:1080", storage.CommandActionUninstall, nil)
	if err != nil {
		t.Fatalf("EnqueueCommand: %v", err)
	}
	if err := store.MarkCommandsRunning([]string{cmd.ID}); err != nil {
		t.Fatalf("MarkCommandsRunning: %v", err)
	}

	server := newTestServer(store)
	body := `{"command_id":"` + cmd.ID + `","node_id":"203.0.113.99:1080","username":"u","password":"p","status":"success","result":"TrinityProxy agent removed"}`
	req := httptest.NewRequest(http.MethodPost, "/api/agent/command-result", strings.NewReader(body))
	rec := httptest.NewRecorder()
	server.handleCommandResult(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	remaining, err := store.GetNodeByID("203.0.113.99:1080")
	if err != nil {
		t.Fatalf("GetNodeByID: %v", err)
	}
	if remaining != nil {
		t.Fatalf("expected node deleted after uninstall, still have %+v", remaining)
	}

	latest, err := store.GetLatestCommandForNode("203.0.113.99:1080")
	if err != nil {
		t.Fatalf("GetLatestCommandForNode: %v", err)
	}
	if latest != nil {
		t.Fatalf("expected commands cleared after uninstall, got %+v", latest)
	}
}

func TestHeartbeatClearsStaleCommandsOnReregistration(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewNodeStorage(filepath.Join(dir, "nodes.db"))
	if err != nil {
		t.Fatalf("NewNodeStorage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	nodeID := "203.0.113.77:1080"
	cmd, err := store.EnqueueCommand(nodeID, storage.CommandActionUninstall, nil)
	if err != nil {
		t.Fatalf("EnqueueCommand: %v", err)
	}
	if err := store.CompleteCommand(cmd.ID, storage.CommandStatusSuccess, "removed"); err != nil {
		t.Fatalf("CompleteCommand: %v", err)
	}

	server := newTestServer(store)
	body := `{"ip":"203.0.113.77","port":1080,"username":"u","password":"p","country":"US"}`
	req := httptest.NewRequest(http.MethodPost, "/api/heartbeat", strings.NewReader(body))
	rec := httptest.NewRecorder()
	server.handleHeartbeat(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("heartbeat status = %d, body = %s", rec.Code, rec.Body.String())
	}

	latest, err := store.GetLatestCommandForNode(nodeID)
	if err != nil {
		t.Fatalf("GetLatestCommandForNode: %v", err)
	}
	if latest != nil {
		t.Fatalf("expected stale commands cleared on re-registration, got %+v", latest)
	}
}

func TestHandleCommandResultRepairClearsCommands(t *testing.T) {
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

	oldCmd, err := store.EnqueueCommand("203.0.113.55:1080", storage.CommandActionUninstall, nil)
	if err != nil {
		t.Fatalf("EnqueueCommand uninstall: %v", err)
	}
	if err := store.CompleteCommand(oldCmd.ID, storage.CommandStatusSuccess, "removed"); err != nil {
		t.Fatalf("CompleteCommand uninstall: %v", err)
	}

	repairCmd, err := store.EnqueueCommand("203.0.113.55:1080", storage.CommandActionRepair, nil)
	if err != nil {
		t.Fatalf("EnqueueCommand repair: %v", err)
	}
	if err := store.MarkCommandsRunning([]string{repairCmd.ID}); err != nil {
		t.Fatalf("MarkCommandsRunning: %v", err)
	}

	server := newTestServer(store)
	body := `{"command_id":"` + repairCmd.ID + `","node_id":"203.0.113.55:1080","username":"u","password":"p","status":"success","result":"Repair complete"}`
	req := httptest.NewRequest(http.MethodPost, "/api/agent/command-result", strings.NewReader(body))
	rec := httptest.NewRecorder()
	server.handleCommandResult(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	latest, err := store.GetLatestCommandForNode("203.0.113.55:1080")
	if err != nil {
		t.Fatalf("GetLatestCommandForNode: %v", err)
	}
	if latest != nil {
		t.Fatalf("expected commands cleared after repair, got %+v", latest)
	}
}

func TestHandleCommandResult(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewNodeStorage(filepath.Join(dir, "nodes.db"))
	if err != nil {
		t.Fatalf("NewNodeStorage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	node := &storage.ProxyNode{
		IP: "203.0.113.50", Port: 1080,
		Username: "u", Password: "p", Country: "US",
	}
	if err := store.UpsertNode(node); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	cmd, err := store.EnqueueCommand("203.0.113.50:1080", storage.CommandActionRestart, nil)
	if err != nil {
		t.Fatalf("EnqueueCommand: %v", err)
	}
	if err := store.MarkCommandsRunning([]string{cmd.ID}); err != nil {
		t.Fatalf("MarkCommandsRunning: %v", err)
	}

	server := newTestServer(store)
	body := `{"command_id":"` + cmd.ID + `","node_id":"203.0.113.50:1080","username":"u","password":"p","status":"success","result":"active"}`
	req := httptest.NewRequest(http.MethodPost, "/api/agent/command-result", strings.NewReader(body))
	rec := httptest.NewRecorder()
	server.handleCommandResult(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	latest, err := store.GetLatestCommandForNode("203.0.113.50:1080")
	if err != nil {
		t.Fatalf("GetLatestCommandForNode: %v", err)
	}
	if latest.Status != storage.CommandStatusSuccess || latest.Result != "active" {
		t.Fatalf("latest = %+v", latest)
	}
}

func TestHeartbeatCommandLifecycle(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewNodeStorage(filepath.Join(dir, "nodes.db"))
	if err != nil {
		t.Fatalf("NewNodeStorage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	node := &storage.ProxyNode{
		IP: "203.0.113.88", Port: 1080,
		Username: "proxy", Password: "secret", Country: "US", Platform: "linux",
	}
	if err := store.UpsertNode(node); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	cmd, err := store.EnqueueCommand("203.0.113.88:1080", storage.CommandActionStatus, nil)
	if err != nil {
		t.Fatalf("EnqueueCommand: %v", err)
	}

	server := newTestServer(store)
	body := `{"ip":"203.0.113.88","port":1080,"username":"proxy","password":"secret","country":"US","platform":"linux"}`
	req := httptest.NewRequest(http.MethodPost, "/api/heartbeat", strings.NewReader(body))
	rec := httptest.NewRecorder()
	server.handleHeartbeat(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("heartbeat status = %d, body = %s", rec.Code, rec.Body.String())
	}

	running, err := store.GetLatestCommandForNode("203.0.113.88:1080")
	if err != nil {
		t.Fatalf("GetLatestCommandForNode: %v", err)
	}
	if running.Status != storage.CommandStatusRunning {
		t.Fatalf("after heartbeat status = %q, want running", running.Status)
	}

	resultBody := `{"command_id":"` + cmd.ID + `","node_id":"203.0.113.88:1080","username":"proxy","password":"secret","status":"success","result":"Port 1080: LISTENING"}`
	resultReq := httptest.NewRequest(http.MethodPost, "/api/agent/command-result", strings.NewReader(resultBody))
	resultRec := httptest.NewRecorder()
	server.handleCommandResult(resultRec, resultReq)
	if resultRec.Code != http.StatusOK {
		t.Fatalf("command-result status = %d, body = %s", resultRec.Code, resultRec.Body.String())
	}

	done, err := store.GetLatestCommandForNode("203.0.113.88:1080")
	if err != nil {
		t.Fatalf("GetLatestCommandForNode after result: %v", err)
	}
	if done.Status != storage.CommandStatusSuccess || done.Result == "" {
		t.Fatalf("completed command = %+v", done)
	}
}

func TestHandleCommandResultRejectsNodeMismatch(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewNodeStorage(filepath.Join(dir, "nodes.db"))
	if err != nil {
		t.Fatalf("NewNodeStorage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	node := &storage.ProxyNode{
		IP: "203.0.113.50", Port: 1080,
		Username: "u", Password: "p", Country: "US",
	}
	if err := store.UpsertNode(node); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	cmd, err := store.EnqueueCommand("203.0.113.50:1080", storage.CommandActionRestart, nil)
	if err != nil {
		t.Fatalf("EnqueueCommand: %v", err)
	}
	if err := store.MarkCommandsRunning([]string{cmd.ID}); err != nil {
		t.Fatalf("MarkCommandsRunning: %v", err)
	}

	server := newTestServer(store)
	body := `{"command_id":"` + cmd.ID + `","node_id":"203.0.113.99:1080","username":"u","password":"p","status":"success","result":"active"}`
	req := httptest.NewRequest(http.MethodPost, "/api/agent/command-result", strings.NewReader(body))
	rec := httptest.NewRecorder()
	server.handleCommandResult(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCommandResultRejectsCredentialMismatch(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewNodeStorage(filepath.Join(dir, "nodes.db"))
	if err != nil {
		t.Fatalf("NewNodeStorage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	node := &storage.ProxyNode{
		IP: "203.0.113.50", Port: 1080,
		Username: "u", Password: "p", Country: "US",
	}
	if err := store.UpsertNode(node); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	cmd, err := store.EnqueueCommand("203.0.113.50:1080", storage.CommandActionRestart, nil)
	if err != nil {
		t.Fatalf("EnqueueCommand: %v", err)
	}
	if err := store.MarkCommandsRunning([]string{cmd.ID}); err != nil {
		t.Fatalf("MarkCommandsRunning: %v", err)
	}

	server := newTestServer(store)
	body := `{"command_id":"` + cmd.ID + `","node_id":"203.0.113.50:1080","username":"wrong","password":"wrong","status":"success","result":"active"}`
	req := httptest.NewRequest(http.MethodPost, "/api/agent/command-result", strings.NewReader(body))
	rec := httptest.NewRecorder()
	server.handleCommandResult(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", rec.Code, rec.Body.String())
	}
}
