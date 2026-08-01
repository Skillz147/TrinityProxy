package deployment

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSyncFromExternalFillsEmptyDashboardDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "dashboard.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	caddyPath := filepath.Join(dir, "site.caddy")
	envPath := filepath.Join(dir, "controller.env")
	if err := os.WriteFile(caddyPath, []byte(`*.gelcgolf.com, gelcgolf.com {
	tls { dns cloudflare {env.CLOUDFLARE_API_TOKEN} }
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envPath, []byte("CONTROLLER_URL=http://203.0.113.10:3100\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := store.SyncFromExternal(SyncOptions{
		Sources: ExternalSources{
			CaddySitePath:     caddyPath,
			ControllerEnvPath: envPath,
			CaddyActiveCheck:  func() bool { return true },
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated {
		t.Fatal("expected update")
	}
	if result.PublicDomain != "gelcgolf.com" {
		t.Fatalf("domain = %q", result.PublicDomain)
	}
	if result.ControllerURL != "https://api.gelcgolf.com" {
		t.Fatalf("controller = %q", result.ControllerURL)
	}
	if result.SSLMode != SSLModeCaddy {
		t.Fatalf("ssl = %q", result.SSLMode)
	}
}

func TestSyncFromExternalOverrideDomainOnSSLFailure(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "dashboard.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	envPath := filepath.Join(dir, "controller.env")
	if err := os.WriteFile(envPath, []byte("CONTROLLER_URL=http://203.0.113.10:3100\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := store.SyncFromExternal(SyncOptions{
		Sources: ExternalSources{
			ControllerEnvPath: envPath,
			CaddyActiveCheck:  func() bool { return false },
		},
		OverrideDomain: "gelcgolf.com",
		OverrideSSLMode: SSLModeNone,
		Force:          true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated {
		t.Fatal("expected update")
	}
	if result.PublicDomain != "gelcgolf.com" {
		t.Fatalf("domain = %q", result.PublicDomain)
	}
	if result.ControllerURL != "http://203.0.113.10:3100" {
		t.Fatalf("controller = %q", result.ControllerURL)
	}
}
