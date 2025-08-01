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
	fmt.Println("3. View current environment variable")
	fmt.Println("4. Clear existing environment variable")
	fmt.Print("\nEnter your choice (1-4): ")

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
	case "3":
		currentRole := os.Getenv("TRINITY_ROLE")
		if currentRole == "" {
			fmt.Println("[!] No TRINITY_ROLE environment variable set")
		} else {
			fmt.Printf("[*] Current TRINITY_ROLE: %s\n", currentRole)
		}
		return promptForRole() // Ask again
	case "4":
		fmt.Println("[*] Environment variable will be cleared for this session")
		os.Unsetenv("TRINITY_ROLE")
		return promptForRole() // Ask again
	default:
		return "", fmt.Errorf("invalid choice '%s'. Please enter 1-4", input)
	}
}

func setEnvironmentVariable(key, value string) error {
	fmt.Printf("\n[*] Setting %s=%s for current session\n", key, value)

	// Set for current process
	err := os.Setenv(key, value)
	if err != nil {
		return err
	}

	fmt.Printf("\n[*] To persist this role permanently, run one of these commands:\n")
	fmt.Printf("   For bash: echo 'export %s=%s' >> ~/.bashrc && source ~/.bashrc\n", key, value)
	fmt.Printf("   For zsh:  echo 'export %s=%s' >> ~/.zshrc && source ~/.zshrc\n", key, value)
	fmt.Printf("   For fish: echo 'set -gx %s %s' >> ~/.config/fish/config.fish\n", key, value)

	// Ask if user wants to persist automatically
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("\n[?] Would you like to automatically persist this to your shell profile? (y/N): ")
	response, _ := reader.ReadString('\n')
	response = strings.ToLower(strings.TrimSpace(response))

	if response == "y" || response == "yes" {
		return persistEnvironmentVariable(key, value)
	}

	return nil
}

func persistEnvironmentVariable(key, value string) error {
	// Detect shell and persist accordingly
	shell := os.Getenv("SHELL")
	var configFile string
	var exportCmd string

	switch {
	case strings.Contains(shell, "zsh"):
		configFile = os.Getenv("HOME") + "/.zshrc"
		exportCmd = fmt.Sprintf("export %s=%s", key, value)
	case strings.Contains(shell, "bash"):
		configFile = os.Getenv("HOME") + "/.bashrc"
		exportCmd = fmt.Sprintf("export %s=%s", key, value)
	case strings.Contains(shell, "fish"):
		configFile = os.Getenv("HOME") + "/.config/fish/config.fish"
		exportCmd = fmt.Sprintf("set -gx %s %s", key, value)
	default:
		return fmt.Errorf("unsupported shell: %s", shell)
	}

	// Check if the environment variable already exists in the file
	if fileExists(configFile) {
		content, err := os.ReadFile(configFile)
		if err == nil && strings.Contains(string(content), key+"=") {
			fmt.Printf("[!] %s already exists in %s\n", key, configFile)
			return nil
		}
	}

	// Append to config file
	file, err := os.OpenFile(configFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open %s: %v", configFile, err)
	}
	defer file.Close()

	_, err = file.WriteString(fmt.Sprintf("\n# TrinityProxy role setting\n%s\n", exportCmd))
	if err != nil {
		return fmt.Errorf("failed to write to %s: %v", configFile, err)
	}

	fmt.Printf("[+] Successfully added %s=%s to %s\n", key, value, configFile)
	fmt.Printf("[*] Restart your terminal or run: source %s\n", configFile)
	return nil
}

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
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

	// Always show current status and allow override
	if role != "" {
		fmt.Printf("[*] Current TRINITY_ROLE: %s\n", role)

		reader := bufio.NewReader(os.Stdin)
		fmt.Print("[?] Use existing role? (Y/n): ")
		response, _ := reader.ReadString('\n')
		response = strings.ToLower(strings.TrimSpace(response))

		if response == "n" || response == "no" {
			fmt.Println("[*] Overriding existing role...")
			role = "" // Force re-selection
		}
	}

	// If no role is set or user wants to override, prompt for selection
	if role == "" {
		fmt.Println("[!] TRINITY_ROLE environment variable not set or being overridden.")

		selectedRole, err := promptForRole()
		if err != nil {
			log.Fatalf("[-] Setup failed: %v", err)
		}

		role = selectedRole

		// Set the environment variable for current session and optionally persist
		if err := setEnvironmentVariable("TRINITY_ROLE", role); err != nil {
			log.Printf("[!] Warning: Failed to set environment variable: %v", err)
		}

		fmt.Printf("\n[+] Role set to: %s\n", role)
	} else {
		fmt.Printf("[*] Using role: %s\n", role)
	}

	// Validate and start the selected role
	switch role {
	case "controller":
		fmt.Println("\n[*] Starting in Controller mode...")
		fmt.Println("[*] This will start the API server for managing proxy nodes")
		runAPIController()
	case "agent":
		fmt.Println("\n[*] Starting in Agent mode...")
		fmt.Println("[*] This will install SOCKS5 proxy and start heartbeat reporting")
		runInstaller()
		runHeartbeatAgent()
	default:
		log.Fatalf("[-] Invalid TRINITY_ROLE '%s'. Valid options are 'controller' or 'agent'", role)
	}
}
