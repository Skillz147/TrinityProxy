package deployment

import "testing"

func TestNormalizeDomain(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"trinityproxy.local", "trinityproxy.local"},
		{"https://api.trinityproxy.local", "trinityproxy.local"},
		{"http://trinityproxy.local/path", "trinityproxy.local"},
		{"api.trinityproxy.local", "trinityproxy.local"},
		{"HTTPS://API.Example.COM:443/foo", "example.com"},
		{"", ""},
	}
	for _, tc := range tests {
		if got := NormalizeDomain(tc.in); got != tc.want {
			t.Errorf("NormalizeDomain(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDeriveControllerURL(t *testing.T) {
	tests := []struct {
		domain, sslMode, want string
	}{
		{"trinityproxy.local", SSLModeNone, "http://api.trinityproxy.local:3100"},
		{"trinityproxy.local", SSLModeDevMkcert, "https://api.trinityproxy.local"},
		{"trinityproxy.local", SSLModeCaddy, "https://api.trinityproxy.local"},
		{"https://api.trinityproxy.local", SSLModeNone, "http://api.trinityproxy.local:3100"},
	}
	for _, tc := range tests {
		if got := DeriveControllerURL(tc.domain, tc.sslMode); got != tc.want {
			t.Errorf("DeriveControllerURL(%q, %q) = %q, want %q", tc.domain, tc.sslMode, got, tc.want)
		}
	}
}

func TestNormalizeControllerURL(t *testing.T) {
	tests := []struct {
		raw, sslMode, want string
	}{
		{"api.trinityproxy.local", SSLModeNone, "http://api.trinityproxy.local:3100"},
		{"http://api.trinityproxy.local:3100", SSLModeNone, "http://api.trinityproxy.local:3100"},
		{"api.trinityproxy.local", SSLModeDevMkcert, "https://api.trinityproxy.local"},
		{"https://api.trinityproxy.local", SSLModeDevMkcert, "https://api.trinityproxy.local"},
		{"", SSLModeNone, ""},
	}
	for _, tc := range tests {
		if got := NormalizeControllerURL(tc.raw, tc.sslMode); got != tc.want {
			t.Errorf("NormalizeControllerURL(%q, %q) = %q, want %q", tc.raw, tc.sslMode, got, tc.want)
		}
	}
}

func TestBuildDNSHintsDevMode(t *testing.T) {
	hints := BuildDNSHints("https://api.trinityproxy.local", "127.0.0.1", SSLModeNone)
	if hints.Domain != "trinityproxy.local" {
		t.Fatalf("domain = %q, want trinityproxy.local", hints.Domain)
	}
	if len(hints.Records) != 1 || hints.Records[0].Type != "hosts" {
		t.Fatalf("expected single hosts record, got %+v", hints.Records)
	}
	want := "127.0.0.1 trinityproxy.local api.trinityproxy.local"
	if hints.Records[0].Value != want {
		t.Fatalf("hosts value = %q, want %q", hints.Records[0].Value, want)
	}
}

func TestBuildDNSHintsProduction(t *testing.T) {
	hints := BuildDNSHints("trinityproxy.local", "203.0.113.1", SSLModeCaddy)
	if len(hints.Records) != 2 || hints.Records[0].Type != "A" {
		t.Fatalf("expected A records, got %+v", hints.Records)
	}
	if hints.Records[0].Name != "api.trinityproxy.local" {
		t.Fatalf("api host = %q", hints.Records[0].Name)
	}
}
