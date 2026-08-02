// internal/agent/heartbeat.go

package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Skillz147/TrinityProxy/internal/config"
	"github.com/Skillz147/TrinityProxy/internal/logutil"
	"github.com/Skillz147/TrinityProxy/internal/proxy"
)

func StartHeartbeatLoop() {
	log := logutil.New("agent")
	cfg := config.Load()
	log.Info("heartbeat loop started",
		"target", cfg.HeartbeatURL(),
		"interval", cfg.HeartbeatInterval.String(),
	)
	if srv := proxy.Active(); srv != nil {
		log.Info("reporting embedded SOCKS endpoint",
			"port", srv.Port,
			"username", srv.Username,
		)
	}

	for {
		err := sendHeartbeat(cfg)
		if err != nil {
			log.Warn("heartbeat failed", "err", err)
		} else {
			log.Info("heartbeat sent")
		}
		time.Sleep(cfg.HeartbeatInterval)
	}
}

func sendHeartbeat(cfg config.Config) error {
	meta, err := GatherMetadata()
	if err != nil {
		return fmt.Errorf("metadata error: %w", err)
	}
	return postNodePayload(cfg.HeartbeatURL(), ResolveAuthToken(), *meta, cfg)
}

// SendDeregister notifies the controller that this agent is shutting down.
func SendDeregister(cfg config.Config) error {
	meta, err := GatherMetadata()
	if err != nil {
		return fmt.Errorf("metadata error: %w", err)
	}
	return postNodePayload(cfg.DeregisterURL(), ResolveAuthToken(), *meta, cfg)
}

func postNodePayload(url, authToken string, meta NodeMetadata, cfg config.Config) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("request error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	setAgentAuthHeaders(req, authToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("post error: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var hbResp heartbeatResponse
	if err := json.Unmarshal(body, &hbResp); err == nil {
		if hbResp.NodeToken != "" {
			if err := SaveNodeToken(hbResp.NodeToken); err != nil {
				logutil.Component("agent").Warn("failed to persist node token", "err", err)
			} else {
				logutil.Component("agent").Info("per-node token saved locally")
			}
		}
		if len(hbResp.PendingCommands) > 0 {
			ProcessPendingCommands(cfg, ResolveAuthToken(), &meta, hbResp.PendingCommands)
			return nil
		}
	}

	// Fallback: poll commands endpoint if heartbeat returned plain "ok"
	if cfg.ControllerURL != "" {
		if commands, err := fetchPendingCommands(cfg, ResolveAuthToken(), meta); err == nil && len(commands) > 0 {
			ProcessPendingCommands(cfg, ResolveAuthToken(), &meta, commands)
		}
	}

	return nil
}

func setAgentAuthHeaders(req *http.Request, token string) {
	if token == "" {
		return
	}
	req.Header.Set("X-Agent-Token", token)
	req.Header.Set("Authorization", "Bearer "+token)
}
