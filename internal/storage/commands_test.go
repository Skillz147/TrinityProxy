package storage

import (
	"path/filepath"
	"testing"
)

func TestCommandQueueLifecycle(t *testing.T) {
	dir := t.TempDir()
	store, err := NewNodeStorage(filepath.Join(dir, "nodes.db"))
	if err != nil {
		t.Fatalf("NewNodeStorage: %v", err)
	}
	defer store.Close()

	nodeID := "203.0.113.10:1080"

	cmd, err := store.EnqueueCommand(nodeID, CommandActionRestart, nil)
	if err != nil {
		t.Fatalf("EnqueueCommand: %v", err)
	}
	if cmd.Status != CommandStatusPending {
		t.Fatalf("status = %q, want pending", cmd.Status)
	}

	pending, err := store.GetPendingCommands(nodeID)
	if err != nil {
		t.Fatalf("GetPendingCommands: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != cmd.ID {
		t.Fatalf("pending = %+v, want one command %s", pending, cmd.ID)
	}

	if err := store.MarkCommandsRunning([]string{cmd.ID}); err != nil {
		t.Fatalf("MarkCommandsRunning: %v", err)
	}

	pending, err = store.GetPendingCommands(nodeID)
	if err != nil {
		t.Fatalf("GetPendingCommands after running: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected no pending commands, got %d", len(pending))
	}

	if err := store.CompleteCommand(cmd.ID, CommandStatusSuccess, "restarted ok"); err != nil {
		t.Fatalf("CompleteCommand: %v", err)
	}

	latest, err := store.GetLatestCommandForNode(nodeID)
	if err != nil {
		t.Fatalf("GetLatestCommandForNode: %v", err)
	}
	if latest == nil {
		t.Fatal("expected latest command")
	}
	if latest.Status != CommandStatusSuccess {
		t.Fatalf("latest status = %q", latest.Status)
	}
	if latest.Result != "restarted ok" {
		t.Fatalf("latest result = %q", latest.Result)
	}
}

func TestClearCommandsForNode(t *testing.T) {
	dir := t.TempDir()
	store, err := NewNodeStorage(filepath.Join(dir, "nodes.db"))
	if err != nil {
		t.Fatalf("NewNodeStorage: %v", err)
	}
	defer store.Close()

	nodeID := "203.0.113.10:1080"
	if _, err := store.EnqueueCommand(nodeID, CommandActionRestart, nil); err != nil {
		t.Fatalf("EnqueueCommand: %v", err)
	}

	if err := store.ClearCommandsForNode(nodeID); err != nil {
		t.Fatalf("ClearCommandsForNode: %v", err)
	}

	latest, err := store.GetLatestCommandForNode(nodeID)
	if err != nil {
		t.Fatalf("GetLatestCommandForNode: %v", err)
	}
	if latest != nil {
		t.Fatalf("expected no commands after clear, got %+v", latest)
	}
}

func TestGetCommandByID(t *testing.T) {
	dir := t.TempDir()
	store, err := NewNodeStorage(filepath.Join(dir, "nodes.db"))
	if err != nil {
		t.Fatalf("NewNodeStorage: %v", err)
	}
	defer store.Close()

	cmd, err := store.EnqueueCommand("203.0.113.10:1080", CommandActionStatus, nil)
	if err != nil {
		t.Fatalf("EnqueueCommand: %v", err)
	}

	found, err := store.GetCommandByID(cmd.ID)
	if err != nil {
		t.Fatalf("GetCommandByID: %v", err)
	}
	if found == nil || found.ID != cmd.ID || found.Action != CommandActionStatus {
		t.Fatalf("found = %+v", found)
	}

	missing, err := store.GetCommandByID("nonexistent")
	if err != nil {
		t.Fatalf("GetCommandByID missing: %v", err)
	}
	if missing != nil {
		t.Fatalf("expected nil for missing command, got %+v", missing)
	}
}

func TestEnqueueCommandInvalidAction(t *testing.T) {
	dir := t.TempDir()
	store, err := NewNodeStorage(filepath.Join(dir, "nodes.db"))
	if err != nil {
		t.Fatalf("NewNodeStorage: %v", err)
	}
	defer store.Close()

	if _, err := store.EnqueueCommand("1.2.3.4:1080", "reboot", nil); err == nil {
		t.Fatal("expected error for invalid action")
	}
}

func TestCompleteCommandForNodeRejectsWrongNode(t *testing.T) {
	dir := t.TempDir()
	store, err := NewNodeStorage(filepath.Join(dir, "nodes.db"))
	if err != nil {
		t.Fatalf("NewNodeStorage: %v", err)
	}
	defer store.Close()

	cmd, err := store.EnqueueCommand("203.0.113.10:1080", CommandActionRestart, nil)
	if err != nil {
		t.Fatalf("EnqueueCommand: %v", err)
	}
	if err := store.MarkCommandsRunning([]string{cmd.ID}); err != nil {
		t.Fatalf("MarkCommandsRunning: %v", err)
	}

	if err := store.CompleteCommandForNode(cmd.ID, "203.0.113.99:1080", CommandStatusSuccess, "nope"); err == nil {
		t.Fatal("expected error completing command for wrong node")
	}
}

func TestEnqueueCommandRepairWithLogLevel(t *testing.T) {
	dir := t.TempDir()
	store, err := NewNodeStorage(filepath.Join(dir, "nodes.db"))
	if err != nil {
		t.Fatalf("NewNodeStorage: %v", err)
	}
	defer store.Close()

	cmd, err := store.EnqueueCommand("1.2.3.4:1080", CommandActionRepair, map[string]string{
		"log_level": "debug",
	})
	if err != nil {
		t.Fatalf("EnqueueCommand: %v", err)
	}
	if cmd.Params["log_level"] != "debug" {
		t.Fatalf("params = %#v", cmd.Params)
	}
}
