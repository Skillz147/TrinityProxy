package dashboard

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	sslProvisionUnitName = "trinityproxy-ssl-provision.service"
	defaultSSLProvisionEnvDir = "/run/trinityproxy"
	sslProvisionEnvFileName   = "ssl-provision.env"
)

type sslProvisionParams struct {
	Domain             string
	Email              string
	ServerIP           string
	CloudflareAPIToken string
}

func sslProvisionEnvDir() string {
	if dir := strings.TrimSpace(os.Getenv("TRINITY_SSL_PROVISION_ENV_DIR")); dir != "" {
		return dir
	}
	return defaultSSLProvisionEnvDir
}

func sslProvisionEnvPath() string {
	return filepath.Join(sslProvisionEnvDir(), sslProvisionEnvFileName)
}

func systemdEnvLine(key, value string) string {
	return key + "=" + systemdEnvValue(value)
}

func systemdEnvValue(value string) string {
	if value == "" {
		return ""
	}
	if !strings.ContainsAny(value, " #\"\\$%\t\n\r") {
		return value
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range value {
		switch r {
		case '\\', '"':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func writeSSLProvisionEnv(params sslProvisionParams) error {
	dir := sslProvisionEnvDir()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create ssl provision env dir: %w", err)
	}
	lines := []string{
		systemdEnvLine("PUBLIC_DOMAIN", params.Domain),
		systemdEnvLine("EMAIL", params.Email),
		systemdEnvLine("SERVER_IP", params.ServerIP),
		systemdEnvLine("CLOUDFLARE_API_TOKEN", params.CloudflareAPIToken),
		"SKIP_DNS_WAIT=1",
	}
	content := strings.Join(lines, "\n") + "\n"
	path := sslProvisionEnvPath()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write ssl provision env: %w", err)
	}
	return nil
}

func systemctlPath() string {
	if p := strings.TrimSpace(os.Getenv("TRINITY_SYSTEMCTL")); p != "" {
		return p
	}
	return "systemctl"
}

func journalctlPath() string {
	if p := strings.TrimSpace(os.Getenv("TRINITY_JOURNALCTL")); p != "" {
		return p
	}
	return "journalctl"
}

func runSSLProvision(ctx context.Context, params sslProvisionParams) (string, error) {
	if err := writeSSLProvisionEnv(params); err != nil {
		return "", err
	}

	unit := sslProvisionUnitName
	cmd := exec.CommandContext(ctx, systemctlPath(), "start", "--wait", unit)
	out, err := cmd.CombinedOutput()
	journal := fetchSSLProvisionJournal(ctx)
	combined := strings.TrimSpace(strings.Join([]string{string(out), journal}, "\n"))

	if err != nil {
		if combined == "" {
			combined = err.Error()
		}
		return combined, fmt.Errorf("systemctl start --wait %s: %w", unit, err)
	}
	if combined == "" {
		combined = "SSL provisioning unit completed successfully"
	}
	return combined, nil
}

func fetchSSLProvisionJournal(ctx context.Context) string {
	cmd := exec.CommandContext(ctx, journalctlPath(),
		"-u", sslProvisionUnitName,
		"-n", "200",
		"--no-pager",
		"-o", "cat",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
