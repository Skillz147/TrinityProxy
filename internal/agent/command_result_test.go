package agent

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Skillz147/TrinityProxy/internal/config"
)

func TestPostCommandResultRetriesAndAuth(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if got := r.Header.Get("X-Agent-Token"); got != "agent-secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if attempts < 2 {
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var payload map[string]string
		if err := json.Unmarshal(body, &payload); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if payload["command_id"] != "cmd123" || payload["status"] != "success" || payload["node_id"] != "203.0.113.1:1080" {
			http.Error(w, "bad payload", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	t.Cleanup(server.Close)

	cfg := config.Config{
		ControllerURL: server.URL,
		AgentKey:      "agent-secret",
	}
	err := postCommandResult(cfg, "agent-secret", &NodeMetadata{
		IP: "203.0.113.1", Port: 1080, Username: "u", Password: "p",
	}, CommandOutcome{
		CommandID: "cmd123",
		Status:    "success",
		Result:    "ok",
	})
	if err != nil {
		t.Fatalf("postCommandResult: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestLinuxDynamicStatusUsesGoChecks(t *testing.T) {
	out, err := linuxDynamicStatus()
	if err != nil {
		t.Fatalf("linuxDynamicStatus: %v", err)
	}
	for _, want := range []string{
		"=== TrinityProxy Agent Status",
		"Proxy mode:",
		"SOCKS port:",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("status missing %q:\n%s", want, out)
		}
	}
}

func TestProcessPendingCommandsReportsResult(t *testing.T) {
	var gotStatus string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/api/agent/command-result"):
			body, _ := io.ReadAll(r.Body)
			var payload map[string]string
			_ = json.Unmarshal(body, &payload)
			gotStatus = payload["status"]
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	cfg := config.Config{ControllerURL: server.URL}
	ProcessPendingCommands(cfg, "", &NodeMetadata{
		IP: "203.0.113.90", Port: 1080, Username: "proxy", Password: "secret",
	}, []RemoteCommand{{
		ID:     "abc",
		Action: "reboot",
	}})
	if gotStatus != "failure" {
		t.Fatalf("reported status = %q, want failure", gotStatus)
	}
}
