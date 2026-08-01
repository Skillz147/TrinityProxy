package dashboard

import (
	"fmt"
	"os"
	"path/filepath"
)

const sslScriptCloudflare = "setup-ssl-caddy-cloudflare.sh"

func resolveSSLScript(name string) (string, error) {
	dirs := []string{
		os.Getenv("TRINITY_SCRIPTS_DIR"),
		"/opt/trinityproxy/scripts",
		filepath.Join(".", "scripts"),
	}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		path := filepath.Join(dir, name)
		st, err := os.Stat(path)
		if err != nil || st.IsDir() {
			continue
		}
		if abs, err := filepath.Abs(path); err == nil {
			return abs, nil
		}
		return path, nil
	}
	return "", fmt.Errorf("SSL setup script %q not found (checked TRINITY_SCRIPTS_DIR, /opt/trinityproxy/scripts, ./scripts)", name)
}
