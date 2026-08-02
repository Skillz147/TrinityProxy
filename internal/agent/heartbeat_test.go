package agent

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Skillz147/TrinityProxy/internal/config"
	"github.com/Skillz147/TrinityProxy/internal/storage"
)

func TestPostNodePayloadProcessesCommandsSynchronously(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewNodeStorage(filepath.Join(dir, "nodes.db"))
	if err != nil {
		t.Fatalf("NewNodeStorage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	var resultPosted bool
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/heartbeat":
			commands, err := store.GetPendingCommands("203.0.113.90:1080")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			ids := make([]string, len(commands))
			pending := make([]map[string]any, 0, len(commands))
			for i, cmd := range commands {
				ids[i] = cmd.ID
				pending = append(pending, map[string]any{
					"id": cmd.ID, "action": cmd.Action, "params": cmd.Params,
				})
			}
			if len(ids) > 0 {
				if err := store.MarkCommandsRunning(ids); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "ok", "pending_commands": pending,
			})
		case "/api/agent/command-result":
			body, _ := io.ReadAll(r.Body)
			var payload struct {
				CommandID string `json:"command_id"`
				NodeID    string `json:"node_id"`
				Status    string `json:"status"`
				Result    string `json:"result"`
			}
			_ = json.Unmarshal(body, &payload)
			if err := store.CompleteCommandForNode(payload.CommandID, payload.NodeID, payload.Status, payload.Result); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			resultPosted = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(api.Close)

	node := &storage.ProxyNode{
		IP: "203.0.113.90", Port: 1080,
		Username: "proxy", Password: "secret", Country: "US", Platform: "linux",
	}
	if err := store.UpsertNode(node); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}
	if _, err := store.EnqueueCommand("203.0.113.90:1080", storage.CommandActionStatus, nil); err != nil {
		t.Fatalf("EnqueueCommand: %v", err)
	}

	cfg := config.Config{ControllerURL: api.URL}
	meta := NodeMetadata{
		IP: "203.0.113.90", Port: 1080,
		Username: "proxy", Password: "secret", Country: "US", Platform: "linux",
	}
	if err := postNodePayload(api.URL+"/api/heartbeat", "", meta, cfg); err != nil {
		t.Fatalf("postNodePayload: %v", err)
	}
	if !resultPosted {
		t.Fatal("expected command result POST during heartbeat handling")
	}

	latest, err := store.GetLatestCommandForNode("203.0.113.90:1080")
	if err != nil {
		t.Fatalf("GetLatestCommandForNode: %v", err)
	}
	if latest.Status != storage.CommandStatusSuccess {
		t.Fatalf("latest status = %q, want success", latest.Status)
	}
}
