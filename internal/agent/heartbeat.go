// internal/agent/heartbeat.go

package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

const (
	heartbeatInterval = 60 * time.Second
	controlAPIURL     = "https://api.sauronstore.com/api/heartbeat" // central HTTPS API endpoint
)

func StartHeartbeatLoop() {
	for {
		err := sendHeartbeat()
		if err != nil {
			log.Printf("[-] Heartbeat failed: %v", err)
		} else {
			log.Println("[+] Heartbeat sent successfully")
		}
		time.Sleep(heartbeatInterval)
	}
}

func sendHeartbeat() error {
	meta, err := GatherMetadata()
	if err != nil {
		return fmt.Errorf("metadata error: %w", err)
	}

	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}

	req, err := http.NewRequest("POST", controlAPIURL, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("request error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

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
