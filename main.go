package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/Skillz147/TrinityProxy/internal/agent"
)

func runCommand(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("[-] Failed to run %s: %v", name, err)
	}
}

func promptForRole() (string, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("\n[*] TrinityProxy Setup")
	fmt.Println("======================")
	fmt.Println("Please select the role for this instance:")
	fmt.Println("1. Controller - API server and management interface")
	fmt.Println("2. Agent - Worker node that connects to controller")
	fmt.Print("\nEnter your choice (1 or 2): ")

	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read input: %v", err)
	}

	input = strings.TrimSpace(input)

	switch input {
	case "1":
		fmt.Println("[+] Selected: Controller")
		return "controller", nil
	case "2":
		fmt.Println("[+] Selected: Agent")
		return "agent", nil
	default:
		return "", fmt.Errorf("invalid choice '%s'. Please enter 1 or 2", input)
	}
}

func setEnvironmentVariable(key, value string) error {
	// For demonstration, we'll show the command to set the environment variable
	fmt.Printf("\n[*] To persist this role, run one of the following commands:\n")
	fmt.Printf("   For current session: export %s=%s\n", key, value)
	fmt.Printf("   For bash persistence: echo 'export %s=%s' >> ~/.bashrc\n", key, value)
	fmt.Printf("   For zsh persistence: echo 'export %s=%s' >> ~/.zshrc\n", key, value)

	// Set for current process
	return os.Setenv(key, value)
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
	runCommand("go", "run", "./cmd/api/enhanced_main.go")
}

func main() {
	role := strings.ToLower(os.Getenv("TRINITY_ROLE"))

	// If no role is set, prompt for interactive setup
	if role == "" {
		fmt.Println("[!] TRINITY_ROLE environment variable not set.")

		selectedRole, err := promptForRole()
		if err != nil {
			log.Fatalf("[-] Setup failed: %v", err)
		}

		role = selectedRole

		// Set the environment variable for current session
		if err := setEnvironmentVariable("TRINITY_ROLE", role); err != nil {
			log.Printf("[!] Warning: Failed to set environment variable: %v", err)
		}

		fmt.Printf("\n[+] Role set to: %s\n", role)
	} else {
		fmt.Printf("[*] Using existing role: %s\n", role)
	}

	// Validate the role
	switch role {
	case "controller":
		fmt.Println("[*] Starting in Controller mode...")
		runAPIController()
	case "agent":
		fmt.Println("[*] Starting in Agent mode...")
		runInstaller()
		runHeartbeatAgent()
	default:
		log.Fatalf("[-] Invalid TRINITY_ROLE '%s'. Valid options are 'controller' or 'agent'", role)
	}
}
