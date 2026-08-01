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

	commands := BuildDeployCommands(settings, "", "test-agent-key-hex", DefaultLogLevel)

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

	commands := BuildDeployCommands(settings, "", "key", DefaultLogLevel)

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

func TestWindowsBootstrapOneLiner(t *testing.T) {
	settings := &Settings{
		PublicDomain: "example.com",
		SSLMode:      SSLModeCaddy,
	}
	commands := BuildDeployCommands(settings, "", "secret-key-abc", DefaultLogLevel)
	win := findPlatform(commands.Platforms, "windows")
	if win.ID != "windows" {
		t.Fatalf("windows platform missing")
	}
	if !strings.Contains(win.Command, "secret-key-abc") {
		t.Errorf("windows command missing agent key")
	}
	if !strings.Contains(win.Command, "iwr -UseBasicParsing") {
		t.Errorf("windows command missing script download")
	}
	if strings.Contains(win.Command, "git clone") {
		t.Errorf("windows command must not use git clone")
	}
	if !strings.Contains(win.Command, "raw.githubusercontent.com") {
		t.Errorf("windows command missing raw install script URL")
	}
	if !strings.Contains(win.Command, "Join-Path $env:TEMP 'tp-install.ps1'") {
		t.Errorf("windows command missing temp installer path")
	}
	if strings.Contains(win.Command, ".\\scripts\\install-agent-windows.ps1") {
		t.Errorf("windows command should not require manual cd to repo")
	}
	if !strings.Contains(win.Command, commands.ProductionControllerURL) {
		t.Errorf("windows command missing controller URL")
	}
	one := windowsBootstrapOneLiner(commands.ProductionControllerURL, "secret-key-abc", DefaultLogLevel)
	if strings.Contains(one, "\n") {
		t.Errorf("bootstrap one-liner should be a single line")
	}
}

func TestWindowsBootstrapOneLinerNoKey(t *testing.T) {
	line := windowsBootstrapOneLiner("https://api.example.com", "", DefaultLogLevel)
	if strings.Contains(line, "TRINITY_AGENT_KEY") {
		t.Errorf("no-key one-liner should not set TRINITY_AGENT_KEY")
	}
}

func TestWindowsRemoveOneLiner(t *testing.T) {
	line := windowsRemoveOneLiner()
	if strings.Contains(line, "\n") {
		t.Errorf("remove one-liner should be a single line")
	}
	for _, want := range []string{
		"TrinityProxyAgent",
		"Stop-Service",
		"sc.exe delete",
		"TrinityProxy SOCKS5",
		"Remove-NetFirewallRule",
		"ProgramFiles",
		"Remove-Item",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("remove one-liner missing %q", want)
		}
	}
}

func TestWindowsStatusOneLiner(t *testing.T) {
	line := windowsStatusOneLiner()
	if strings.Contains(line, "\n") {
		t.Errorf("status one-liner should be a single line")
	}
	for _, want := range []string{
		"TrinityProxyAgent",
		"Get-Service",
		"trinityproxy-port",
		"Get-NetTCPConnection",
		"LISTENING",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("status one-liner missing %q", want)
		}
	}
}

func TestPlatformOperations(t *testing.T) {
	settings := &Settings{
		PublicDomain: "example.com",
		SSLMode:      SSLModeCaddy,
	}
	commands := BuildDeployCommands(settings, "", "test-key", DefaultLogLevel)

	win := findPlatform(commands.Platforms, "windows")
	if len(win.Operations) != 4 {
		t.Fatalf("windows operations = %d, want 4", len(win.Operations))
	}
	remove := win.Operations[1]
	if remove.ID != "remove" {
		t.Errorf("remove op id = %q, want remove", remove.ID)
	}
	if !strings.Contains(remove.Command, "Remove-NetFirewallRule") {
		t.Errorf("remove command missing firewall cleanup")
	}
	if !strings.Contains(remove.Command, "sc.exe delete") {
		t.Errorf("remove command missing service delete")
	}

	linux := findPlatform(commands.Platforms, "linux-vps")
	if len(linux.Operations) != 4 {
		t.Fatalf("linux operations = %d, want 4", len(linux.Operations))
	}
	if linux.Operations[0].ID != "install" {
		t.Errorf("first linux op = %q, want install", linux.Operations[0].ID)
	}
}

func TestBuildDeployCommandsLogLevel(t *testing.T) {
	settings := &Settings{
		PublicDomain: "example.com",
		SSLMode:      SSLModeCaddy,
	}

	commands := BuildDeployCommands(settings, "", "test-key", "silent")

	linux := findPlatform(commands.Platforms, "linux-vps")
	if !strings.Contains(linux.Command, "TRINITY_LOG_LEVEL=silent") {
		t.Errorf("linux command missing silent log level: %q", linux.Command)
	}
	if strings.Contains(linux.Operations[1].Command, "TRINITY_LOG_LEVEL") {
		t.Errorf("linux remove command should not include TRINITY_LOG_LEVEL")
	}

	macos := findPlatform(commands.Platforms, "macos")
	if !strings.Contains(macos.Operations[0].Command, "TRINITY_LOG_LEVEL=silent") {
		t.Errorf("macos install missing silent log level")
	}

	win := findPlatform(commands.Platforms, "windows")
	if !strings.Contains(win.Operations[0].Command, "$env:TRINITY_LOG_LEVEL='silent'") {
		t.Errorf("windows install missing silent log level: %q", win.Operations[0].Command)
	}
	if strings.Contains(win.Operations[1].Command, "TRINITY_LOG_LEVEL") {
		t.Errorf("windows remove command should not include TRINITY_LOG_LEVEL")
	}

	debug := BuildDeployCommands(settings, "", "test-key", "debug")
	linuxDebug := findPlatform(debug.Platforms, "linux-vps")
	if !strings.Contains(linuxDebug.Command, "TRINITY_LOG_LEVEL=debug") {
		t.Errorf("linux command missing debug log level")
	}

	invalid := BuildDeployCommands(settings, "", "test-key", "verbose")
	linuxDefault := findPlatform(invalid.Platforms, "linux-vps")
	if !strings.Contains(linuxDefault.Command, "TRINITY_LOG_LEVEL=info") {
		t.Errorf("invalid log level should default to info")
	}
}

func TestNormalizeLogLevel(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", DefaultLogLevel},
		{"INFO", "info"},
		{" silent ", "silent"},
		{"quiet", "quiet"},
		{"debug", "debug"},
		{"invalid", DefaultLogLevel},
	}
	for _, tc := range tests {
		if got := NormalizeLogLevel(tc.in); got != tc.want {
			t.Errorf("NormalizeLogLevel(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
