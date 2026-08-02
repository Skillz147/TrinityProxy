package agent

import (
	"strings"
	"testing"
)

func TestExecuteRemoteCommandUnknownAction(t *testing.T) {
	outcome := executeRemoteCommand(RemoteCommand{
		ID:     "abc123",
		Action: "reboot",
	})
	if outcome.Status != "failure" {
		t.Fatalf("status = %q, want failure", outcome.Status)
	}
	if outcome.CommandID != "abc123" {
		t.Fatalf("command id = %q", outcome.CommandID)
	}
}

func TestExecuteRemoteCommandStatus(t *testing.T) {
	outcome := executeRemoteCommand(RemoteCommand{
		ID:     "status1",
		Action: "status",
	})
	// On dev machines without installed service this may fail — either outcome is valid.
	if outcome.Status != "success" && outcome.Status != "failure" {
		t.Fatalf("unexpected status %q", outcome.Status)
	}
	if outcome.Result == "" && outcome.Status == "success" {
		t.Fatal("expected non-empty result on success")
	}
}

func TestLinuxManagedLocallyRespectsEnv(t *testing.T) {
	t.Setenv("TRINITY_CONTAINER", "1")
	if !linuxManagedLocally() {
		t.Fatal("expected linuxManagedLocally true when TRINITY_CONTAINER=1")
	}
	t.Setenv("TRINITY_CONTAINER", "")
}

func TestLinuxContainerScriptsAvoidSystemctl(t *testing.T) {
	for name, script := range map[string]string{
		"uninstall": linuxContainerUninstallScript(),
		"restart":   linuxContainerRestartScript(),
		"repair":    linuxContainerRepairScript("info"),
	} {
		if strings.Contains(script, "systemctl") {
			t.Fatalf("%s script must not use systemctl:\n%s", name, script)
		}
	}
}

func TestLinuxServiceScriptsUseSystemctl(t *testing.T) {
	if !strings.Contains(linuxUninstallScript(), "systemctl") {
		t.Fatal("expected systemd uninstall script to use systemctl")
	}
}
