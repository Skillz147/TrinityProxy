package main

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/Skillz147/TrinityProxy/internal/agent"
	"github.com/Skillz147/TrinityProxy/internal/config"
	"github.com/Skillz147/TrinityProxy/internal/logutil"
	"github.com/Skillz147/TrinityProxy/internal/proxy"
	"golang.org/x/term"
)

func runCommand(log *slog.Logger, name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		logutil.Fatal(log, "failed to run command", "cmd", name, "err", err)
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

func isNonInteractive() bool {
	if os.Getenv("TRINITY_NONINTERACTIVE") == "1" {
		return true
	}
	return !term.IsTerminal(int(os.Stdin.Fd()))
}

func runInstaller(log *slog.Logger) {
	installerPath := resolveBuildBinary("installer")
	log.Info("running TrinityProxy installer", "path", installerPath)
	runCommand(log, installerPath)
}

func runHeartbeatAgent(log *slog.Logger) {
	cfg := config.Load()
	log.Info("starting heartbeat agent")
	go agent.StartHeartbeatLoop()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	log.Info("shutdown signal received, deregistering agent")
	if err := agent.SendDeregister(cfg); err != nil {
		log.Warn("deregister failed", "err", err)
	}
}

func startEmbeddedSOCKS(log *slog.Logger) {
	srv, err := proxy.StartEmbedded()
	if err != nil {
		logutil.Fatal(log, "failed to start embedded SOCKS proxy", "err", err)
	}
	log.Info("embedded SOCKS proxy started",
		"port", srv.Port,
		"username", srv.Username,
	)
}

func skipAgentInstaller() bool {
	return proxy.UseEmbedded()
}

func deviceClass() string {
	if v := strings.TrimSpace(os.Getenv("TRINITY_DEVICE_CLASS")); v != "" {
		return v
	}
	switch runtime.GOOS {
	case "darwin":
		return "macos"
	case "linux":
		return "linux"
	default:
		return runtime.GOOS
	}
}

func logAgentEnv(log *slog.Logger, cfgControllerURL string) {
	cls := deviceClass()
	if os.Getenv("TRINITY_DEVICE_CLASS") == "" {
		log.Info("TRINITY_DEVICE_CLASS unset — using auto-detected default",
			"device_class", cls,
			"hint", "set TRINITY_DEVICE_CLASS=macos|linux|vps|desktop to label this agent")
	} else {
		log.Info("agent environment", "device_class", cls)
	}
	if cfgControllerURL != "" {
		log.Info("controller configured", "controller_url", cfgControllerURL)
	} else {
		log.Warn("CONTROLLER_URL unset — heartbeats need a controller base URL")
	}
	if os.Getenv("TRINITY_AGENT_KEY") != "" {
		log.Info("heartbeat auth enabled via TRINITY_AGENT_KEY")
	}
}

func resolveBuildBinary(name string) string {
	if root := os.Getenv("TRINITY_ROOT"); root != "" {
		return filepath.Join(root, "build", name)
	}

	exe, err := os.Executable()
	if err != nil {
		return filepath.Join("build", name)
	}

	return filepath.Join(filepath.Dir(exe), name)
}

func runAPIController(log *slog.Logger) {
	apiPath := resolveBuildBinary("trinityproxy-api")
	log.Info("starting API server", "path", apiPath)
	runCommand(log, apiPath)
}

func main() {
	log := logutil.New("launcher")
	nonInteractive := isNonInteractive()
	role := strings.ToLower(os.Getenv("TRINITY_ROLE"))

	if role != "" {
		fmt.Printf("[*] Current TRINITY_ROLE: %s\n", role)

		if nonInteractive {
			fmt.Printf("[*] Using role: %s (non-interactive mode)\n", role)
		} else {
			reader := bufio.NewReader(os.Stdin)
			fmt.Print("[?] Use existing role? (Y/n): ")
			response, _ := reader.ReadString('\n')
			response = strings.ToLower(strings.TrimSpace(response))

			if response == "n" || response == "no" {
				fmt.Println("[*] Overriding existing role...")
				role = ""
			}
		}
	}

	if role == "" {
		if nonInteractive {
			logutil.Fatal(log, "TRINITY_ROLE must be set in non-interactive mode",
				"hint", "TRINITY_ROLE=agent or TRINITY_ROLE=controller")
		}

		fmt.Println("[!] TRINITY_ROLE environment variable not set or being overridden.")

		selectedRole, err := promptForRole()
		if err != nil {
			logutil.Fatal(log, "setup failed", "err", err)
		}

		role = selectedRole

		if err := setEnvironmentVariable("TRINITY_ROLE", role); err != nil {
			log.Warn("failed to set environment variable", "err", err)
		}

		fmt.Printf("\n[+] Role set to: %s\n", role)
	} else if !nonInteractive {
		fmt.Printf("[*] Using role: %s\n", role)
	}

	// Validate and start the selected role
	switch role {
	case "controller":
		fmt.Println("\n[*] Starting in Controller mode...")
		fmt.Println("[*] This will start the API server for managing proxy nodes")
		runAPIController(log)
	case "agent":
		cfg := config.Load()
		logAgentEnv(log, cfg.ControllerURL)
		if skipAgentInstaller() {
			fmt.Println("\n[*] Embedded SOCKS mode — Go proxy, no Dante installer")
			fmt.Printf("[*] TRINITY_DEVICE_CLASS=%s (override with env)\n", deviceClass())
			fmt.Println("[*] SOCKS5 on TRINITY_SOCKS_PORT (default 1080); heartbeats to CONTROLLER_URL/api/heartbeat")
			startEmbeddedSOCKS(log)
		} else {
			fmt.Println("\n[*] Starting in Agent mode...")
			fmt.Printf("[*] TRINITY_DEVICE_CLASS=%s (override with env)\n", deviceClass())
			fmt.Println("[*] This will install SOCKS5 proxy and start heartbeat reporting")
			runInstaller(log)
		}
		runHeartbeatAgent(log)
	default:
		logutil.Fatal(log, "invalid TRINITY_ROLE", "role", role, "valid", "controller, agent")
	}
}
