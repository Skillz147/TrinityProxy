package deployment

import (
	"strings"
	"testing"
)

func TestBuildCloudflareSetupEmptyDomain(t *testing.T) {
	setup := BuildCloudflareSetup("", "")
	if setup.Summary == "" {
		t.Fatal("expected summary for empty domain")
	}
	if len(setup.TokenSteps) < 3 {
		t.Fatalf("expected token steps, got %d", len(setup.TokenSteps))
	}
	if setup.TokenURL == "" {
		t.Fatal("expected token URL")
	}
}

func TestBuildCloudflareSetupWithDomain(t *testing.T) {
	setup := BuildCloudflareSetup("example.com", "203.0.113.10")
	if setup.Domain != "example.com" {
		t.Fatalf("domain = %q", setup.Domain)
	}
	if setup.APIHost != "api.example.com" {
		t.Fatalf("api host = %q", setup.APIHost)
	}
	if len(setup.DNSRecords) != 2 {
		t.Fatalf("records = %d, want 2", len(setup.DNSRecords))
	}
	if setup.DNSRecords[0].Name != "api.example.com" {
		t.Fatalf("first record name = %q", setup.DNSRecords[0].Name)
	}
	if setup.DNSRecords[0].Value != "203.0.113.10" {
		t.Fatalf("first record value = %q", setup.DNSRecords[0].Value)
	}
	if !strings.Contains(setup.DNSRecords[0].Notes, "Proxied") {
		t.Fatalf("expected proxied note, got %q", setup.DNSRecords[0].Notes)
	}
	if !strings.Contains(setup.Summary, "wildcard") {
		t.Fatalf("summary should mention wildcard: %q", setup.Summary)
	}
	if setup.RenewalNote == "" {
		t.Fatal("expected renewal note")
	}
}
