package deployment

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadCaddySiteDomain(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trinityproxy.caddy")
	content := `*.gelcgolf.com, gelcgolf.com {
	tls {
		dns cloudflare {env.CLOUDFLARE_API_TOKEN}
	}

	@api host api.gelcgolf.com
	handle @api {
		reverse_proxy 127.0.0.1:3100
	}

	@dashboard host gelcgolf.com
	handle @dashboard {
		reverse_proxy 127.0.0.1:8081
	}
}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	domain, ok := ReadCaddySiteDomain(path)
	if !ok || domain != "gelcgolf.com" {
		t.Fatalf("domain = %q ok=%v, want gelcgolf.com", domain, ok)
	}
	if !caddySiteUsesTLS(path) {
		t.Fatal("expected tls detection")
	}
}

func TestReadControllerEnvDeployment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "controller.env")
	content := `# test
TRINITY_API_KEY=abc
PUBLIC_DOMAIN=gelcgolf.com
CONTROLLER_URL=https://api.gelcgolf.com
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	domain, controllerURL := ReadControllerEnvDeployment(path)
	if domain != "gelcgolf.com" {
		t.Fatalf("domain = %q", domain)
	}
	if controllerURL != "https://api.gelcgolf.com" {
		t.Fatalf("controllerURL = %q", controllerURL)
	}
}

func TestDiscoverExternalPrefersCaddyDomain(t *testing.T) {
	dir := t.TempDir()
	caddyPath := filepath.Join(dir, "site.caddy")
	envPath := filepath.Join(dir, "controller.env")

	if err := os.WriteFile(caddyPath, []byte(`*.example.com, example.com {
	tls { dns cloudflare {env.CLOUDFLARE_API_TOKEN} }
	@dashboard host example.com
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envPath, []byte("CONTROLLER_URL=http://203.0.113.10:3100\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	snap := DiscoverExternal(ExternalSources{
		CaddySitePath:     caddyPath,
		ControllerEnvPath: envPath,
		CaddyActiveCheck:  func() bool { return true },
	})

	if snap.PublicDomain != "example.com" {
		t.Fatalf("domain = %q", snap.PublicDomain)
	}
	if snap.SSLMode != SSLModeCaddy {
		t.Fatalf("ssl = %q", snap.SSLMode)
	}
	if snap.ControllerURL != "https://api.example.com" {
		t.Fatalf("controller = %q", snap.ControllerURL)
	}
}

func TestDiscoverExternalInactiveCaddyUsesControllerEnv(t *testing.T) {
	dir := t.TempDir()
	caddyPath := filepath.Join(dir, "site.caddy")
	envPath := filepath.Join(dir, "controller.env")

	if err := os.WriteFile(caddyPath, []byte(`*.example.com, example.com {
	tls { dns cloudflare {env.CLOUDFLARE_API_TOKEN} }
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envPath, []byte("PUBLIC_DOMAIN=example.com\nCONTROLLER_URL=http://203.0.113.10:3100\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	snap := DiscoverExternal(ExternalSources{
		CaddySitePath:     caddyPath,
		ControllerEnvPath: envPath,
		CaddyActiveCheck:  func() bool { return false },
	})

	if snap.SSLMode != SSLModeNone {
		t.Fatalf("ssl = %q, want none", snap.SSLMode)
	}
	if snap.ControllerURL != "http://203.0.113.10:3100" {
		t.Fatalf("controller = %q", snap.ControllerURL)
	}
}
