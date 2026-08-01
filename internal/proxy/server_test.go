package proxy

import (
	"testing"
	"time"

	"github.com/Skillz147/TrinityProxy/internal/health"
)

func TestStartEmbeddedAuthProbe(t *testing.T) {
	t.Setenv("TRINITY_DATA_DIR", t.TempDir())
	t.Setenv("TRINITY_SOCKS_PORT", "0")
	t.Setenv("TRINITY_SOCKS_USER", "alice")
	t.Setenv("TRINITY_SOCKS_PASS", "secret")

	srv, err := StartEmbedded()
	if err != nil {
		t.Fatalf("StartEmbedded: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	prober := health.NewProber(health.WithTimeout(2 * time.Second))
	if !prober.ProbeFresh("127.0.0.1", srv.Port, "alice", "secret") {
		t.Fatal("expected healthy probe for valid credentials")
	}
	if prober.ProbeFresh("127.0.0.1", srv.Port, "alice", "wrong") {
		t.Fatal("expected unhealthy probe for invalid credentials")
	}

	active := Active()
	if active == nil || active.Username != "alice" || active.Password != "secret" {
		t.Fatalf("Active() = %+v, want alice/secret", active)
	}
}

func TestSocksPortDefault(t *testing.T) {
	t.Setenv("TRINITY_SOCKS_PORT", "")
	t.Setenv("TRINITY_DEV_PROXY_PORT", "")
	if got := SocksPort(); got != defaultPort {
		t.Fatalf("SocksPort() = %d, want %d", got, defaultPort)
	}
}
