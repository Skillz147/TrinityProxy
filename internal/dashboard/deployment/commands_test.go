package deployment

import (
	"strings"
	"testing"
)

func TestBuildDeployCommands(t *testing.T) {
	settings := &Settings{
		PublicDomain:        "trinityproxy.local",
		ControllerPublicURL: "",
		SSLMode:             SSLModeDevMkcert,
	}

	commands := BuildDeployCommands(settings, "", "test-agent-key-hex")

	if commands.ProductionControllerURL != "https://api.trinityproxy.local" {
		t.Errorf("production URL = %q, want https://api.trinityproxy.local", commands.ProductionControllerURL)
	}
	if commands.LocalControllerURL != "http://127.0.0.1:3100" {
		t.Errorf("local URL = %q, want http://127.0.0.1:3100", commands.LocalControllerURL)
	}
	if len(commands.Platforms) != 5 {
		t.Fatalf("platform count = %d, want 5", len(commands.Platforms))
	}

	linux := commands.Platforms[0]
	if linux.ID != "linux-vps" {
		t.Fatalf("first platform id = %q, want linux-vps", linux.ID)
	}
	if linux.ControllerURL != commands.ProductionControllerURL {
		t.Errorf("linux controller URL mismatch")
	}
	if !strings.Contains(linux.Command, "test-agent-key-hex") {
		t.Errorf("linux command missing agent key")
	}

	macos := findPlatform(commands.Platforms, "macos")
	if macos.ControllerURL != commands.LocalControllerURL {
		t.Errorf("macos controller URL = %q, want local", macos.ControllerURL)
	}
	if !strings.Contains(macos.Command, "make install-agent-macos") {
		t.Errorf("macos command missing make target")
	}

	docker := findPlatform(commands.Platforms, "docker")
	if docker.ControllerURL != dockerControllerURL {
		t.Errorf("docker controller URL = %q", docker.ControllerURL)
	}
	if !strings.Contains(docker.Command, "make docker-agent") {
		t.Errorf("docker command missing make docker-agent")
	}
}

func TestBuildDeployCommandsVPSIPMode(t *testing.T) {
	settings := &Settings{
		PublicDomain: "203.0.113.10",
		SSLMode:      SSLModeNone,
	}

	commands := BuildDeployCommands(settings, "", "key")

	want := "http://api.203.0.113.10:3100"
	if commands.ProductionControllerURL != want {
		t.Errorf("production URL = %q, want %q", commands.ProductionControllerURL, want)
	}
}

func findPlatform(platforms []DeployPlatform, id string) DeployPlatform {
	for _, p := range platforms {
		if p.ID == id {
			return p
		}
	}
	return DeployPlatform{}
}
