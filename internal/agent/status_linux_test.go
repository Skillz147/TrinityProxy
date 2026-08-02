//go:build linux

package agent

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestResolveStatusPortFromPortFile(t *testing.T) {
	dir := t.TempDir()
	portFile := filepath.Join(dir, "trinityproxy-port")
	if err := os.WriteFile(portFile, []byte("36593"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TRINITY_DATA_DIR", dir)
	t.Setenv("TRINITY_SOCKS_PORT", "")

	if got := resolveStatusPort(); got != 36593 {
		t.Fatalf("resolveStatusPort() = %d, want 36593", got)
	}
}

func TestResolveStatusPortPrefersEnv(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "trinityproxy-port"), []byte("36593"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TRINITY_DATA_DIR", dir)
	t.Setenv("TRINITY_SOCKS_PORT", "1080")

	if got := resolveStatusPort(); got != 1080 {
		t.Fatalf("resolveStatusPort() = %d, want 1080", got)
	}
}

func TestLinuxDynamicStatusReportsDanteAndPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "trinityproxy-port"), []byte(strconv.Itoa(port)), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TRINITY_DATA_DIR", dir)
	t.Setenv("TRINITY_SOCKS_PORT", strconv.Itoa(port))
	t.Setenv("TRINITY_SKIP_INSTALLER", "")

	out, err := linuxDynamicStatus()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Proxy mode: Dante SOCKS5",
		"SOCKS port: " + strconv.Itoa(port),
		"Port " + strconv.Itoa(port) + ": LISTENING",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("status missing %q:\n%s", want, out)
		}
	}
}

func TestLinuxDynamicStatusEmbeddedMode(t *testing.T) {
	t.Setenv("TRINITY_SKIP_INSTALLER", "1")
	t.Setenv("TRINITY_SOCKS_PORT", "10888")
	t.Setenv("TRINITY_SOCKS_USER", "status-user")
	t.Setenv("TRINITY_SOCKS_PASSWORD", "status-pass")

	out, err := linuxDynamicStatus()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Proxy mode: embedded SOCKS5 (Go)",
		"SOCKS port: 10888",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("status missing %q:\n%s", want, out)
		}
	}
}
