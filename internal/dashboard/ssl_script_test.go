package dashboard

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveSSLScript(t *testing.T) {
	t.Run("from TRINITY_SCRIPTS_DIR", func(t *testing.T) {
		dir := t.TempDir()
		script := filepath.Join(dir, sslScriptCloudflare)
		if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("TRINITY_SCRIPTS_DIR", dir)
		got, err := resolveSSLScript(sslScriptCloudflare)
		if err != nil {
			t.Fatal(err)
		}
		want, _ := filepath.Abs(script)
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})

	t.Run("missing", func(t *testing.T) {
		t.Setenv("TRINITY_SCRIPTS_DIR", t.TempDir())
		_, err := resolveSSLScript("nonexistent.sh")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}
