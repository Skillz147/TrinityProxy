package dashboard

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Skillz147/TrinityProxy/internal/storage"
)

func TestHandleEnqueueNodeCommand(t *testing.T) {
	server := newTestServer(t)
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	node := &storage.ProxyNode{
		IP: "203.0.113.77", Port: 1080,
		Username: "proxy", Password: "secret", Country: "US", Platform: "linux",
	}
	if err := server.nodes.UpsertNode(node); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	token := loginWithPasswordChanged(t, server, mux)
	nodeID := "203.0.113.77:1080"

	body, _ := json.Marshal(map[string]string{"action": "restart"})
	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/nodes/"+nodeID+"/commands", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Status  string `json:"status"`
		Command struct {
			ID     string `json:"id"`
			Action string `json:"action"`
			Status string `json:"status"`
		} `json:"command"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "queued" {
		t.Fatalf("status = %q", resp.Status)
	}
	if resp.Command.Action != "restart" || resp.Command.Status != storage.CommandStatusPending {
		t.Fatalf("command = %+v", resp.Command)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/dashboard/nodes", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listRec := httptest.NewRecorder()
	mux.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("nodes status = %d", listRec.Code)
	}

	var nodesResp struct {
		Nodes []map[string]any `json:"nodes"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &nodesResp); err != nil {
		t.Fatalf("decode nodes: %v", err)
	}
	if len(nodesResp.Nodes) != 1 {
		t.Fatalf("nodes count = %d", len(nodesResp.Nodes))
	}
	rc, ok := nodesResp.Nodes[0]["remote_command"].(map[string]any)
	if !ok {
		t.Fatalf("remote_command missing: %#v", nodesResp.Nodes[0])
	}
	if rc["action"] != "restart" || rc["status"] != storage.CommandStatusPending {
		t.Fatalf("remote_command = %#v", rc)
	}
}

func TestHandleEnqueueNodeCommandNotFound(t *testing.T) {
	server := newTestServer(t)
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	token := loginWithPasswordChanged(t, server, mux)

	body, _ := json.Marshal(map[string]string{"action": "status"})
	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/nodes/missing:1080/commands", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
