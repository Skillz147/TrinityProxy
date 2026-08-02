package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const nodeTokenFileName = "node-token"

// nodeTokenPath returns the platform-specific path for the persisted per-node token.
func nodeTokenPath() string {
	if v := strings.TrimSpace(os.Getenv("TRINITY_NODE_TOKEN_FILE")); v != "" {
		return v
	}
	switch runtime.GOOS {
	case "linux":
		if _, err := os.Stat("/var/lib/trinityproxy-agent"); err == nil {
			return "/var/lib/trinityproxy-agent/" + nodeTokenFileName
		}
		return "/etc/trinityproxy-" + nodeTokenFileName
	case "darwin":
		home, _ := os.UserHomeDir()
		if home == "" {
			home = "."
		}
		return filepath.Join(home, "Library", "Application Support", "TrinityProxy", nodeTokenFileName)
	case "windows":
		pf := os.Getenv("ProgramFiles")
		if pf == "" {
			pf = `C:\Program Files`
		}
		return filepath.Join(pf, "TrinityProxy", nodeTokenFileName)
	default:
		return filepath.Join(".", nodeTokenFileName)
	}
}

// LoadNodeToken reads the persisted per-node token from disk.
func LoadNodeToken() (string, error) {
	path := nodeTokenPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// SaveNodeToken persists the per-node token to the platform config file.
func SaveNodeToken(token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("empty node token")
	}
	path := nodeTokenPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		return err
	}
	return nil
}

// ResolveAuthToken returns the token to send on agent API requests.
// Priority: TRINITY_NODE_TOKEN env → persisted node token → TRINITY_ENROLLMENT_KEY → TRINITY_AGENT_KEY (legacy).
func ResolveAuthToken() string {
	if v := strings.TrimSpace(os.Getenv("TRINITY_NODE_TOKEN")); v != "" {
		return v
	}
	if saved, err := LoadNodeToken(); err == nil && saved != "" {
		return saved
	}
	if v := strings.TrimSpace(os.Getenv("TRINITY_ENROLLMENT_KEY")); v != "" {
		return v
	}
	return strings.TrimSpace(os.Getenv("TRINITY_AGENT_KEY"))
}
