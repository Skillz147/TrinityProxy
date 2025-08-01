package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"trinityproxy/internal/agent"
)

func runCommand(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("[-] Failed to run %s: %v", name, err)
	}
}

func runInstaller() {
	log.Println("[*] Running TrinityProxy installer...")
	runCommand("go", "run", "./cmd/installer/installer.go")
}

func runHeartbeatAgent() {
	log.Println("[*] Starting heartbeat agent...")
	go agent.StartHeartbeatLoop()
	select {} // block forever
}

func runAPIController() {
	log.Println("[*] Starting API server...")
	runCommand("go", "run", "./cmd/api/main.go")
}

func main() {
	role := strings.ToLower(os.Getenv("TRINITY_ROLE"))
	fmt.Printf("[*] Detected role: %s\n", role)

	switch role {
	case "controller":
		runAPIController()
	case "agent":
		runInstaller()
		runHeartbeatAgent()
	default:
		log.Fatalf("[-] Unknown or missing TRINITY_ROLE. Use 'controller' or 'agent'")
	}
}
