// internal/agent/heartbeat.go

package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
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

	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}

	req, err := http.NewRequest("POST", cfg.HeartbeatURL(), bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("request error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.AgentKey != "" {
		req.Header.Set("X-API-Key", cfg.AgentKey)
		req.Header.Set("Authorization", "Bearer "+cfg.AgentKey)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("post error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	return nil
}
