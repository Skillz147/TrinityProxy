package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Skillz147/TrinityProxy/internal/config"
)

type heartbeatResponse struct {
	Status          string          `json:"status"`
	NodeToken       string          `json:"node_token,omitempty"`
	PendingCommands []RemoteCommand `json:"pending_commands"`
}

func postCommandResult(cfg config.Config, agentKey string, meta *NodeMetadata, outcome CommandOutcome) error {
	url := cfg.CommandResultURL()
	if url == "" || cfg.ControllerURL == "" {
		return fmt.Errorf("controller URL not configured")
	}
	if meta == nil {
		return fmt.Errorf("node metadata required for command result")
	}

	payload := map[string]string{
		"command_id": outcome.CommandID,
		"node_id":    fmt.Sprintf("%s:%d", meta.IP, meta.Port),
		"username":   meta.Username,
		"password":   meta.Password,
		"status":     outcome.Status,
		"result":     outcome.Result,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		if err := postCommandResultOnce(url, agentKey, data); err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
			continue
		}
		return nil
	}
	return lastErr
}

func postCommandResultOnce(url, agentKey string, data []byte) error {
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if agentKey != "" {
		req.Header.Set("X-Agent-Token", agentKey)
		req.Header.Set("Authorization", "Bearer "+agentKey)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("command result API returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func fetchPendingCommands(cfg config.Config, agentKey string, meta NodeMetadata) ([]RemoteCommand, error) {
	nodeID := fmt.Sprintf("%s:%d", meta.IP, meta.Port)
	url := fmt.Sprintf("%s?node_id=%s", cfg.AgentCommandsURL(), nodeID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if agentKey != "" {
		req.Header.Set("X-Agent-Token", agentKey)
		req.Header.Set("Authorization", "Bearer "+agentKey)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("commands API returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		PendingCommands []RemoteCommand `json:"pending_commands"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.PendingCommands, nil
}
