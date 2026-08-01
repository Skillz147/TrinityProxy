package config

import (
	"testing"
)

func TestProbeLocalFallbackDefaults(t *testing.T) {
	t.Setenv("TRINITY_PROBE_LOCAL_FALLBACK", "")
	t.Setenv("TRINITY_ENV", "")
	if !Load().ProbeLocalFallback() {
		t.Fatal("expected local fallback enabled by default")
	}
}

func TestProbeLocalFallbackProductionEnv(t *testing.T) {
	t.Setenv("TRINITY_PROBE_LOCAL_FALLBACK", "")
	t.Setenv("TRINITY_ENV", "production")
	if Load().ProbeLocalFallback() {
		t.Fatal("expected local fallback disabled in production env")
	}
}

func TestProbeLocalFallbackNonInteractiveIgnored(t *testing.T) {
	t.Setenv("TRINITY_PROBE_LOCAL_FALLBACK", "")
	t.Setenv("TRINITY_ENV", "")
	t.Setenv("TRINITY_NONINTERACTIVE", "1")
	if !Load().ProbeLocalFallback() {
		t.Fatal("TRINITY_NONINTERACTIVE must not disable probe local fallback")
	}
}

func TestProbeLocalFallbackExplicitOverride(t *testing.T) {
	t.Setenv("TRINITY_ENV", "production")
	t.Setenv("TRINITY_PROBE_LOCAL_FALLBACK", "1")
	if !Load().ProbeLocalFallback() {
		t.Fatal("expected explicit TRINITY_PROBE_LOCAL_FALLBACK=1 to enable fallback")
	}

	t.Setenv("TRINITY_ENV", "")
	t.Setenv("TRINITY_PROBE_LOCAL_FALLBACK", "0")
	if Load().ProbeLocalFallback() {
		t.Fatal("expected explicit TRINITY_PROBE_LOCAL_FALLBACK=0 to disable fallback")
	}
}
