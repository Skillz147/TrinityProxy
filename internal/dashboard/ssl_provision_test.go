package dashboard

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSystemdEnvValue(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"plain", "plain"},
		{"has space", `"has space"`},
		{`quote"here`, `"quote\"here"`},
		{"tok$en", `"tok$en"`},
	}
	for _, tc := range tests {
		if got := systemdEnvValue(tc.in); got != tc.want {
			t.Errorf("systemdEnvValue(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestWriteSSLProvisionEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TRINITY_SSL_PROVISION_ENV_DIR", dir)

	params := sslProvisionParams{
		Domain:             "example.com",
		Email:              "ssl@example.com",
		ServerIP:           "203.0.113.10",
		CloudflareAPIToken: "secret-token",
	}
	if err := writeSSLProvisionEnv(params); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, sslProvisionEnvFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, want := range []string{
		"PUBLIC_DOMAIN=example.com",
		"EMAIL=ssl@example.com",
		"SERVER_IP=203.0.113.10",
		"CLOUDFLARE_API_TOKEN=secret-token",
		"SKIP_DNS_WAIT=1",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("env file missing %q:\n%s", want, body)
		}
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("env file mode = %o, want 0600", st.Mode().Perm())
	}
}

func TestRunSSLProvisionUsesSystemctl(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	fake := filepath.Join(bin, "systemctl")
	script := `#!/bin/sh
if [ "$1" = start ] && [ "$2" = --wait ]; then
  echo started
  exit 0
fi
echo unexpected >&2
exit 1
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TRINITY_SSL_PROVISION_ENV_DIR", dir)
	t.Setenv("TRINITY_SYSTEMCTL", fake)

	out, err := runSSLProvision(context.Background(), sslProvisionParams{
		Domain:             "example.com",
		Email:              "ssl@example.com",
		ServerIP:           "1.2.3.4",
		CloudflareAPIToken: "tok",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "started") {
		t.Fatalf("output = %q", out)
	}
}
