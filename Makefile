# TrinityProxy Makefile
# Easy build and deployment for SOCKS5 proxy network

.PHONY: help build build-main build-dashboard build-dashboard-ui build-windows-agent build-darwin-agent build-linux-amd64 build-linux-arm64 install-agent-macos clean install deps test run-controller start-controller run-agent run-agent-dev docker-agent test-agent-docker docker-agent-down setup-dev check-deps format lint setup-system vps-setup setup-api-controller quickstart debug cleanup install-service install-dashboard-service install-production start-service stop-service start start-dev stop stop-production uninstall-production uninstall dashboard dashboard-dev dashboard-init dashboard-run dashboard-up run-dashboard sync-agent-key setup-domain

# Catch accidental "make run dashboard" (space) — the target is run-dashboard (hyphen).
ifneq (,$(filter dashboard,$(MAKECMDGOALS)))
ifeq ($(filter run dashboard,$(MAKECMDGOALS)),run dashboard)
$(error Wrong command: use 'make run-dashboard' (hyphen, not space). Run 'make dashboard-dev' for instructions.)
endif
endif

# Default target
all: deps build

# Help target - shows primary user commands
help:
	@echo "TrinityProxy"
	@echo "============"
	@echo ""
	@echo "  make start          - PRODUCTION: install + run on VPS (deps, build, secrets, systemd)"
	@echo "  make uninstall-production - Remove prod install (systemd, /opt, data, config); fresh: sudo make start"
	@echo "  make uninstall          - Alias for uninstall-production"
	@echo "  make setup-domain   - VPS: interactive domain + Cloudflare wildcard SSL (sudo)"
	@echo "  make start-dev      - LOCAL DEV: Vite :8080, dashboard API :8081, controller :3100"
	@echo "  make stop           - Stop local dev servers"
	@echo "  make run-agent-dev  - macOS/local dev agent (embedded SOCKS :1080, foreground)"
	@echo "  make build          - Build all binaries"

# Variables
BINARY_NAME=trinityproxy
BUILD_DIR=build
GO_FILES=$(shell find . -name "*.go" -type f)
INSTALLER_BINARY=$(BUILD_DIR)/installer
API_BINARY=$(BUILD_DIR)/trinityproxy-api
DASHBOARD_BINARY=$(BUILD_DIR)/trinityproxy-dashboard

# Go build flags
LDFLAGS=-ldflags "-X main.Version=$(shell export PATH="/usr/local/go/bin:$$PATH"; git describe --tags --always --dirty 2>/dev/null || echo 'dev')"

# Build all binaries
build: build-main build-dashboard $(INSTALLER_BINARY) $(API_BINARY)
	@echo "[+] Build complete!"
	@echo "[*] Main binary: $(BUILD_DIR)/$(BINARY_NAME)"
	@echo "[*] Installer: $(INSTALLER_BINARY)"
	@echo "[*] API Server: $(API_BINARY)"
	@echo "[*] Dashboard: $(DASHBOARD_BINARY)"

# Build main agent/controller binary only
build-main: $(BUILD_DIR)/$(BINARY_NAME)

# Build main binary
$(BUILD_DIR)/$(BINARY_NAME): $(GO_FILES) | $(BUILD_DIR)/.dir
	@echo "[*] Building main binary..."
	@export PATH="/usr/local/go/bin:$$PATH"; go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .

# Cross-compile agent binary for Windows (copy to PC, then run scripts/install-agent-windows.ps1)
build-windows-agent: $(BUILD_DIR)/trinityproxy.exe
	@echo "[+] Windows agent binary: $(BUILD_DIR)/trinityproxy.exe"
	@echo "[*] Copy to a Windows PC and run scripts/install-agent-windows.ps1 (see docs/WINDOWS_AGENT.md)"
	@echo "[*] On Windows, test in foreground before installing the service:"
	@echo "      set TRINITY_ROLE=agent TRINITY_NONINTERACTIVE=1 TRINITY_SKIP_INSTALLER=1"
	@echo "      set CONTROLLER_URL=https://api.example.com TRINITY_AGENT_KEY=... TRINITY_SOCKS_PORT=1080"
	@echo "      build\\trinityproxy.exe"

$(BUILD_DIR)/trinityproxy.exe: $(GO_FILES) | $(BUILD_DIR)/.dir
	@echo "[*] Building Windows agent binary (GOOS=windows GOARCH=amd64)..."
	@export PATH="/usr/local/go/bin:$$PATH"; GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/trinityproxy.exe .

# Build installer binary
$(INSTALLER_BINARY): cmd/installer/installer.go | $(BUILD_DIR)/.dir
	@echo "[*] Building installer..."
	@export PATH="/usr/local/go/bin:$$PATH"; go build $(LDFLAGS) -o $(INSTALLER_BINARY) ./cmd/installer

# Build API server binary
$(API_BINARY): cmd/api/enhanced_main.go | $(BUILD_DIR)/.dir
	@echo "[*] Building API server..."
	@export PATH="/usr/local/go/bin:$$PATH"; go build $(LDFLAGS) -o $(API_BINARY) ./cmd/api

# Build dashboard UI (npm) and sync into cmd/dashboard/dist for go:embed
build-dashboard-ui:
	@chmod +x scripts/build-dashboard-ui.sh
	@./scripts/build-dashboard-ui.sh

# Build dashboard server binary (all dashboard packages — not only main.go)
DASHBOARD_GO_SRCS := $(shell find cmd/dashboard internal/dashboard -name '*.go' 2>/dev/null)
DASHBOARD_UI_SRCS := $(shell find cmd/dashboard/dist -type f 2>/dev/null)

build-dashboard: build-dashboard-ui $(DASHBOARD_BINARY)

$(DASHBOARD_BINARY): $(DASHBOARD_GO_SRCS) $(DASHBOARD_UI_SRCS) | $(BUILD_DIR)/.dir
	@echo "[*] Building dashboard server..."
	@export PATH="/usr/local/go/bin:$$PATH"; go build $(LDFLAGS) -o $(DASHBOARD_BINARY) ./cmd/dashboard

# Create build directory (must not be named "build" — that is the top-level build target)
$(BUILD_DIR)/.dir:
	@mkdir -p $(BUILD_DIR)
	@touch $@

# Cross-compile agent binary for macOS (amd64 + arm64)
build-darwin-agent: | $(BUILD_DIR)/.dir
	@echo "[*] Building agent for darwin/amd64..."
	@export PATH="/usr/local/go/bin:$$PATH"; \
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/trinityproxy-darwin-amd64 .
	@echo "[*] Building agent for darwin/arm64..."
	@export PATH="/usr/local/go/bin:$$PATH"; \
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/trinityproxy-darwin-arm64 .
	@echo "[+] Agent binaries: $(BUILD_DIR)/trinityproxy-darwin-amd64 $(BUILD_DIR)/trinityproxy-darwin-arm64"

# Cross-compile agent binary for Linux amd64
build-linux-amd64: | $(BUILD_DIR)/.dir
	@echo "[*] Building agent for linux/amd64..."
	@export PATH="/usr/local/go/bin:$$PATH"; \
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/trinityproxy-linux-amd64 .
	@echo "[+] Agent binary: $(BUILD_DIR)/trinityproxy-linux-amd64"

# Cross-compile agent binary for Linux arm64
build-linux-arm64: | $(BUILD_DIR)/.dir
	@echo "[*] Building agent for linux/arm64..."
	@export PATH="/usr/local/go/bin:$$PATH"; \
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/trinityproxy-linux-arm64 .
	@echo "[+] Agent binary: $(BUILD_DIR)/trinityproxy-linux-arm64"

# Install agent as macOS launchd service (requires CONTROLLER_URL)
install-agent-macos: build-main
	@if [ "$$(uname -s)" != "Darwin" ]; then \
		echo "[!] install-agent-macos is for macOS only. On Linux use: make install-agent-service"; \
		exit 1; \
	fi
	@if [ -f .env.controller ]; then \
		echo "[*] Loading .env.controller (TRINITY_AGENT_KEY)"; \
	fi
	@chmod +x scripts/install-agent-macos.sh
	@export PATH="/usr/local/go/bin:$$PATH"; \
	$(load-controller-env) \
	CONTROLLER_URL="$${CONTROLLER_URL:-http://127.0.0.1:3100}" \
	./scripts/install-agent-macos.sh

# Install Go dependencies
deps:
	@echo "[*] Installing Go dependencies..."
	@export PATH="/usr/local/go/bin:$$PATH"; go mod download
	@export PATH="/usr/local/go/bin:$$PATH"; go mod tidy
	@echo "[+] Dependencies installed!"

# Install system dependencies (Ubuntu/Debian)
install-dante:
	@echo "[*] Installing Dante SOCKS5 server..."
	@if command -v apt-get >/dev/null 2>&1; then \
		sudo apt-get update && sudo apt-get install -y dante-server; \
	elif command -v yum >/dev/null 2>&1; then \
		sudo yum install -y dante-server; \
	elif command -v dnf >/dev/null 2>&1; then \
		sudo dnf install -y dante-server; \
	elif command -v pacman >/dev/null 2>&1; then \
		sudo pacman -S --noconfirm dante; \
	else \
		echo "[-] Unsupported package manager. Please install dante-server manually."; \
		exit 1; \
	fi
	@echo "[+] Dante SOCKS5 server installed!"

# Complete system installation
install: install-dante
	@echo "[*] Installing SQLite..."
	@if command -v apt-get >/dev/null 2>&1; then \
		sudo apt-get install -y sqlite3; \
	elif command -v yum >/dev/null 2>&1; then \
		sudo yum install -y sqlite; \
	elif command -v dnf >/dev/null 2>&1; then \
		sudo dnf install -y sqlite; \
	elif command -v pacman >/dev/null 2>&1; then \
		sudo pacman -S --noconfirm sqlite; \
	fi
	@echo "[+] System dependencies installed!"

# Check if required dependencies are available
check-deps:
	@echo "[*] Checking dependencies..."
	@export PATH="/usr/local/go/bin:$$PATH"; \
	if command -v go >/dev/null 2>&1; then \
		echo "[+] Go found: $$(go version)"; \
	else \
		echo "[-] Go is not installed!"; \
		exit 1; \
	fi
	@command -v git >/dev/null 2>&1 || (echo "[-] Git is not installed!" && exit 1)
	@command -v sqlite3 >/dev/null 2>&1 || echo "[!] SQLite3 not found - run 'make install'"
	@command -v sockd >/dev/null 2>&1 || echo "[!] Dante server not found - run 'make install-dante'"
	@echo "[+] Dependency check complete!"

# Development setup
setup-dev: deps build check-deps
	@echo "[+] Development environment ready!"
	@echo "[*] Run 'make run' to start TrinityProxy"

# Run tests
test:
	@echo "[*] Running tests..."
	@export PATH="/usr/local/go/bin:$$PATH"; go test -v ./...

# Format Go code
format:
	@echo "[*] Formatting Go code..."
	@export PATH="/usr/local/go/bin:$$PATH"; go fmt ./...
	@echo "[+] Code formatted!"

# Run linter (requires golangci-lint)
lint:
	@echo "[*] Running linter..."
	@export PATH="/usr/local/go/bin:$$PATH"; \
	if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "[!] golangci-lint not found. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

# Clean build artifacts
clean:
	@echo "[*] Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	@export PATH="/usr/local/go/bin:$$PATH"; go clean
	@echo "[+] Clean complete!"

# Runtime targets
run: build
	@echo "[*] Starting TrinityProxy with interactive setup..."
	@export PATH="/usr/local/go/bin:$$PATH"; ./$(BUILD_DIR)/$(BINARY_NAME)

# Load .env.controller (TRINITY_AGENT_KEY) when present — written by make sync-agent-key
define load-controller-env
set -a; \
if [ -f .env.controller ]; then \
	. ./.env.controller; \
fi; \
set +a;
endef

# Smart controller setup - handles all controller requirements automatically
run-controller: build-main
	@echo "[*] Starting TrinityProxy in Controller mode..."
	@if [ -f .env.controller ]; then \
		echo "[*] Loading .env.controller (TRINITY_AGENT_KEY)"; \
	else \
		echo "[!] No .env.controller — run 'make sync-agent-key' after dashboard generates an agent key"; \
	fi
	@export PATH="/usr/local/go/bin:$$PATH"; \
	$(load-controller-env) \
	TRINITY_NONINTERACTIVE=1 \
	TRINITY_ROLE=controller ./$(BUILD_DIR)/$(BINARY_NAME)

# Sync agent key from dashboard DB, then start controller (full dev stack helper)
start-controller: sync-agent-key run-controller

# Smart agent setup - handles all agent requirements automatically  
run-agent:
	@if [ "$$(uname -s)" = "Darwin" ]; then \
		echo "[!] systemd and Dante are Linux-only."; \
		echo "[*] On macOS, use: make run-agent-dev"; \
		exit 1; \
	fi
	@echo "[*] Setting up TrinityProxy Agent (complete setup)..."
	@echo "[*] Checking agent requirements..."
	@# Ensure system dependencies are installed
	@if ! command -v sockd >/dev/null 2>&1; then \
		echo "[*] Installing required system dependencies..."; \
		make setup-system; \
	fi
	@# Build if needed
	@if [ ! -f "$(BUILD_DIR)/$(BINARY_NAME)" ]; then \
		echo "[*] Building binaries..."; \
		make build; \
	fi
	@# Install as systemd service if not already installed
	@if [ ! -f /etc/systemd/system/trinityproxy-agent.service ]; then \
		echo "[*] Installing agent as systemd service..."; \
		make install-agent-service; \
	fi
	@# Start the service
	@echo "[*] Starting TrinityProxy Agent service..."
	@sudo systemctl start trinityproxy-agent
	@sleep 3
	@echo ""
	@if sudo systemctl is-active trinityproxy-agent >/dev/null 2>&1; then \
		echo "🚀 SUCCESS! TrinityProxy Agent is running!"; \
		echo ""; \
		echo "✅ Agent Status: sudo systemctl status trinityproxy-agent"; \
		echo "✅ View Logs: sudo journalctl -u trinityproxy-agent -f"; \
		echo "✅ SOCKS5 proxy is active and reporting to controller"; \
		echo ""; \
		echo "🎯 Agent running in background via systemd service!"; \
	else \
		echo "❌ Agent service failed to start. Check logs:"; \
		sudo journalctl -u trinityproxy-agent --no-pager -n 20; \
	fi

# macOS / local dev — embedded SOCKS, no systemd or Dante
run-agent-dev: build-main
	@echo ""
	@echo "============================================"
	@echo "  macOS dev mode — embedded SOCKS on :1080 (no Dante)"
	@echo "============================================"
	@echo ""
	@if [ -f .env.controller ]; then \
		echo "[*] Loading .env.controller (TRINITY_AGENT_KEY)"; \
	else \
		echo "[!] No .env.controller — run 'make sync-agent-key' after dashboard generates an agent key"; \
	fi
	@echo "[*] Controller: $${CONTROLLER_URL:-http://127.0.0.1:3100}"
	@echo "[*] SOCKS5: :$${TRINITY_SOCKS_PORT:-1080} (user=dev, pass=dev)"
	@echo "[*] Press Ctrl+C to stop."
	@echo ""
	@export PATH="/usr/local/go/bin:$$PATH"; \
	$(load-controller-env) \
	CONTROLLER_URL="$${CONTROLLER_URL:-http://127.0.0.1:3100}" \
	TRINITY_ROLE=agent \
	TRINITY_NONINTERACTIVE=1 \
	TRINITY_SKIP_INSTALLER=1 \
	TRINITY_DEVICE_CLASS=desktop \
	TRINITY_SOCKS_PORT="$${TRINITY_SOCKS_PORT:-1080}" \
	./$(BUILD_DIR)/$(BINARY_NAME)

DOCKER_COMPOSE_DEV = docker compose -f docker/docker-compose.dev.yml

# Linux agent in Docker — simulates a VPS agent on macOS (heartbeats → host :3100)
docker-agent test-agent-docker:
	@if ! command -v docker >/dev/null 2>&1; then \
		echo "[!] Docker not found. Install Docker Desktop for Mac: https://www.docker.com/products/docker-desktop/"; \
		exit 1; \
	fi
	@if [ ! -f .env.controller ]; then \
		echo "[!] No .env.controller — run 'make start-dev' (syncs key automatically) or 'make sync-agent-key'"; \
		exit 1; \
	fi
	@echo "[*] Building and starting Linux agent container..."
	@set -a; . ./.env.controller; set +a; \
	export CONTROLLER_URL="$${CONTROLLER_URL:-http://host.docker.internal:3100}"; \
	$(DOCKER_COMPOSE_DEV) up -d --build
	@echo ""
	@echo "Agent container started — check dashboard Agents page in ~60s"
	@echo ""
	@echo "  Logs:    docker logs -f trinityproxy-agent-dev"
	@echo "  Stop:    make docker-agent-down"

docker-agent-down:
	@if command -v docker >/dev/null 2>&1; then \
		$(DOCKER_COMPOSE_DEV) down; \
		echo "[+] Agent container stopped."; \
	else \
		echo "[!] Docker not found."; \
	fi

# Development helpers
dev-controller: build-main
	@echo "[*] Starting development controller (with auto-restart)..."
	@if command -v entr >/dev/null 2>&1; then \
		find . -name "*.go" | entr -r make run-controller; \
	else \
		echo "[!] Install 'entr' for auto-restart: apt-get install entr"; \
		make run-controller; \
	fi

dev-agent: build-main
	@echo "[*] Starting development agent (with auto-restart)..."
	@if [ "$$(uname -s)" = "Darwin" ]; then \
		AGENT_TARGET=run-agent-dev; \
	else \
		AGENT_TARGET=run-agent; \
	fi; \
	if command -v entr >/dev/null 2>&1; then \
		find . -name "*.go" | entr -r make $$AGENT_TARGET; \
	else \
		echo "[!] Install 'entr' for auto-restart: apt-get install entr (Linux) or brew install entr (macOS)"; \
		make $$AGENT_TARGET; \
	fi

# Production — full VPS bootstrap (deps, build, secrets, systemd, credentials)
start:
	@chmod +x scripts/start-production.sh
	@./scripts/start-production.sh

# Local dev — dashboard API (:8081) + Vite UI (:8080) + controller (:3100)
start-dev dashboard:
	@chmod +x scripts/start-dashboard-dev.sh
	@./scripts/start-dashboard-dev.sh

stop:
	@chmod +x scripts/stop-dashboard-dev.sh
	@./scripts/stop-dashboard-dev.sh

stop-production:
	@chmod +x scripts/stop-production.sh
	@./scripts/stop-production.sh

uninstall-production:
	@chmod +x scripts/uninstall-production.sh
	@./scripts/uninstall-production.sh $(UNINSTALL_OPTS)

uninstall: uninstall-production


# Dashboard API on :8081 (Vite UI dev server uses :8080)
run-dashboard dashboard-run: build-dashboard
	@echo "[*] Starting TrinityProxy Dashboard API on :8081..."
	@echo ""
	@echo "  This terminal = API only (:8081). Open the UI in a second terminal:"
	@echo ""
	@echo "    cd web/dashboard && npm run dev"
	@echo ""
	@echo "  Then browse: http://localhost:8080  (Vite proxies /api → :8081)"
	@echo ""
	@echo "  Tip: 'make dashboard-up' checks both ports. 'make dashboard-dev' lists all steps."
	@if lsof -ti:8081 >/dev/null 2>&1; then \
		echo "[!] Warning: :8081 already in use — stop the other process or pick another DASHBOARD_PORT."; \
	fi
	@export PATH="/usr/local/go/bin:$$PATH"; DASHBOARD_PORT=8081 ./$(DASHBOARD_BINARY)

# Sync TRINITY_AGENT_KEY from dashboard.db into .env.controller for the controller API
sync-agent-key:
	@chmod +x scripts/sync-agent-key.sh
	@./scripts/sync-agent-key.sh


# Regenerate dashboard admin credentials (production: sudo make reset-dashboard-admin)
reset-dashboard-admin reset-admin:
	@chmod +x scripts/reset-dashboard-admin.sh
	@./scripts/reset-dashboard-admin.sh

# Bootstrap initial dashboard admin (prints temp credentials once)
dashboard-init: build-dashboard
	@echo "[*] Initializing dashboard admin user..."
	@export PATH="/usr/local/go/bin:$$PATH"; ./$(DASHBOARD_BINARY) --init-only

# Quick reminder for dashboard dev
dashboard-dev:
	@echo "Start the dashboard:  make start-dev"
	@echo "Open in browser:      http://localhost:8080"
	@echo "Stop when done:       make stop"

# Check whether Vite (:8080), dashboard API (:8081), and controller (:3100) are listening
dashboard-up:
	@echo "TrinityProxy — service check"
	@echo "=============================="
	@VITE_UP=0; API_UP=0; CTRL_UP=0; \
	if lsof -ti:8080 >/dev/null 2>&1; then \
		VITE_UP=1; \
		echo "[+] :8080 — Dashboard UI running (http://localhost:8080)"; \
	else \
		echo "[-] :8080 — Dashboard UI not running"; \
	fi; \
	if lsof -ti:8081 >/dev/null 2>&1; then \
		API_UP=1; \
		echo "[+] :8081 — Dashboard API running"; \
	else \
		echo "[-] :8081 — Dashboard API not running"; \
	fi; \
	if lsof -ti:3100 >/dev/null 2>&1; then \
		CTRL_UP=1; \
		echo "[+] :3100 — Controller API running"; \
	else \
		echo "[-] :3100 — Controller API not running"; \
	fi; \
	echo ""; \
	if [ $$VITE_UP -eq 1 ] && [ $$API_UP -eq 1 ] && [ $$CTRL_UP -eq 1 ]; then \
		echo "Ready — open http://localhost:8080"; \
	elif [ $$VITE_UP -eq 1 ] && [ $$API_UP -eq 1 ]; then \
		echo "Dashboard ready; controller missing — run 'make start-dev'"; \
		exit 1; \
	else \
		echo "Not ready — run 'make start-dev'"; \
		exit 1; \
	fi

# Deployment helpers
deploy-vps:
	@if [ -z "$(VPS_HOST)" ]; then \
		echo "[-] VPS_HOST not set. Usage: make deploy-vps VPS_HOST=user@your-vps.com"; \
		exit 1; \
	fi
	@echo "[*] Deploying to VPS: $(VPS_HOST)"
	rsync -avz --progress . $(VPS_HOST):~/TrinityProxy/
	ssh $(VPS_HOST) "cd ~/TrinityProxy && make setup-dev && sudo make install"
	@echo "[+] Deployment complete!"

# Internal build helper (used by vps-setup / agent-setup). Prefer make start or make start-dev.
quickstart:
	@echo "[!] quickstart is deprecated — use 'make start' (VPS) or 'make start-dev' (local)."
	@echo "[*] Running legacy quickstart steps..."
	@make setup-system
	@make check-deps
	@make deps
	@make build

# Complete VPS setup (runs setup script)
setup-system:
	@if [ -f "scripts/setup.sh" ]; then \
		echo "[*] Running system setup script..."; \
		chmod +x scripts/setup.sh; \
		sudo bash scripts/setup.sh; \
		echo "[+] System setup complete!"; \
	else \
		echo "[!] Setup script not found. Installing basic dependencies..."; \
		make install; \
	fi

# VPS-specific quickstart (deps + controller systemd — TLS via Caddy scripts or dashboard Cloudflare modal)
vps-setup: setup-system deps build install-service
	@echo ""
	@echo "[+] VPS Setup Complete!"
	@echo "======================"
	@echo "Checking port 3100 and starting TrinityProxy Controller..."
	@echo ""
	@# Kill anything blocking port 3100
	@if command -v lsof >/dev/null 2>&1; then \
		BLOCKING_PID=$$(lsof -ti:3100 2>/dev/null); \
		if [ ! -z "$$BLOCKING_PID" ]; then \
			echo "[*] Killing process blocking port 3100 (PID: $$BLOCKING_PID)"; \
			sudo kill -9 $$BLOCKING_PID 2>/dev/null || true; \
			sleep 2; \
		fi \
	fi
	@# Start the service
	@echo "[*] Starting TrinityProxy Controller service..."
	@sudo systemctl start trinityproxy-controller
	@sleep 3
	@echo ""
	@if sudo systemctl is-active trinityproxy-controller >/dev/null 2>&1; then \
		echo "🚀 SUCCESS! TrinityProxy Controller is running!"; \
		echo ""; \
		echo "✅ API listening on :3100 — expose via your CONTROLLER_DOMAIN (see scripts/setup-ssl-*.sh)"; \
		echo "✅ Service Status: sudo systemctl status trinityproxy-controller"; \
		echo "✅ View Logs: sudo journalctl -u trinityproxy-controller -f"; \
		echo ""; \
		echo "🎯 Your VPS is ready! Controller is running in background."; \
	else \
		echo "❌ Service failed to start. Check logs:"; \
		sudo journalctl -u trinityproxy-controller --no-pager -n 20; \
	fi
	@echo ""

# Agent VPS setup (system setup + agent service)
agent-setup: setup-system deps build install-agent-service
	@echo ""
	@echo "[+] Agent VPS Setup Complete!"
	@echo "============================="
	@echo "Starting TrinityProxy Agent service..."
	@echo ""
	@# Start the agent service
	@echo "[*] Starting TrinityProxy Agent service..."
	@sudo systemctl start trinityproxy-agent
	@sleep 3
	@echo ""
	@if sudo systemctl is-active trinityproxy-agent >/dev/null 2>&1; then \
		echo "🚀 SUCCESS! TrinityProxy Agent is running!"; \
		echo ""; \
		echo "✅ Agent Status: sudo systemctl status trinityproxy-agent"; \
		echo "✅ View Logs: sudo journalctl -u trinityproxy-agent -f"; \
		echo "✅ SOCKS5 proxy is active and reporting to controller"; \
		echo ""; \
		echo "🎯 Your Agent VPS is ready! SOCKS5 proxy running in background."; \
	else \
		echo "❌ Agent service failed to start. Check logs:"; \
		sudo journalctl -u trinityproxy-agent --no-pager -n 20; \
	fi
	@echo ""


# Interactive domain + Cloudflare wildcard SSL (VPS CLI wrapper around setup-ssl-caddy-cloudflare.sh)
setup-domain:
	@chmod +x scripts/setup-domain.sh scripts/setup-ssl-caddy-cloudflare.sh
	@sudo bash scripts/setup-domain.sh

# Deprecated — SSL is configured via dashboard Cloudflare modal or Caddy scripts (not Make).
setup-api-controller:
	@echo "[!] setup-api-controller is deprecated."
	@echo "[*] Production TLS options:"
	@echo "    1. Dashboard → Settings → Cloudflare SSL (DNS-01 wildcard, proxied A records)"
	@echo "    2. sudo make setup-domain  (or sudo ./scripts/setup-domain.sh)"
	@echo "    3. sudo CONTROLLER_DOMAIN=api.example.com SERVER_IP=... EMAIL=... ./scripts/setup-ssl-caddy.sh"
	@echo "[*] Controller systemd only: make install-service && sudo systemctl start trinityproxy-controller"

# Install controller as systemd service (runs in background)
install-service: build-main $(API_BINARY)
	@if [ -f "scripts/install-service.sh" ]; then \
		echo "[*] Installing TrinityProxy Controller as systemd service..."; \
		chmod +x scripts/install-service.sh; \
		sudo bash scripts/install-service.sh; \
		echo "[+] TrinityProxy Controller service installed!"; \
	else \
		echo "[!] Service installation script not found."; \
		exit 1; \
	fi

# Install dashboard as systemd service (API + built UI on :8081)
install-dashboard-service: build-dashboard
	@if [ -f "scripts/install-dashboard-service.sh" ]; then \
		echo "[*] Installing TrinityProxy Dashboard as systemd service..."; \
		chmod +x scripts/install-dashboard-service.sh; \
		sudo bash scripts/install-dashboard-service.sh; \
		echo "[+] TrinityProxy Dashboard service installed!"; \
	else \
		echo "[!] Dashboard service installation script not found."; \
		exit 1; \
	fi

# Backward-compat alias — use make start for production bootstrap
install-production:
	@echo "[!] install-production is deprecated — use 'make start' instead."
	@$(MAKE) start

# Install agent as systemd service (runs in background)
install-agent-service: build-main
	@if [ -f "scripts/install-agent-service.sh" ]; then \
		echo "[*] Installing TrinityProxy Agent as systemd service..."; \
		chmod +x scripts/install-agent-service.sh; \
		sudo bash scripts/install-agent-service.sh; \
		echo "[+] TrinityProxy Agent service installed!"; \
	else \
		echo "[!] Agent service installation script not found."; \
		exit 1; \
	fi

# Start/restart the systemd service
start-service:
	@if [ -f /etc/systemd/system/trinityproxy-controller.service ]; then \
		echo "[*] Starting TrinityProxy Controller service..."; \
		sudo systemctl start trinityproxy-controller; \
		sudo systemctl status trinityproxy-controller; \
	else \
		echo "[!] Service not installed. Run 'make install-service' first."; \
		exit 1; \
	fi

# Stop the systemd service
stop-service:
	@if [ -f /etc/systemd/system/trinityproxy-controller.service ]; then \
		echo "[*] Stopping TrinityProxy Controller service..."; \
		sudo systemctl stop trinityproxy-controller; \
		echo "[+] Service stopped."; \
	else \
		echo "[!] Service not installed."; \
	fi

# Start/restart the agent systemd service
start-agent-service:
	@if [ -f /etc/systemd/system/trinityproxy-agent.service ]; then \
		echo "[*] Starting TrinityProxy Agent service..."; \
		sudo systemctl start trinityproxy-agent; \
		sudo systemctl status trinityproxy-agent; \
	else \
		echo "[!] Agent service not installed. Run 'make install-agent-service' first."; \
		exit 1; \
	fi

# Stop the agent systemd service
stop-agent-service:
	@if [ -f /etc/systemd/system/trinityproxy-agent.service ]; then \
		echo "[*] Stopping TrinityProxy Agent service..."; \
		sudo systemctl stop trinityproxy-agent; \
		echo "[+] Agent service stopped."; \
	else \
		echo "[!] Agent service not installed."; \
	fi

# Version info
version:
	@echo "TrinityProxy Build System"
	@echo "Git Version: $(shell git describe --tags --always --dirty 2>/dev/null || echo 'dev')"
	@export PATH="/usr/local/go/bin:$$PATH"; echo "Go Version: $$(go version 2>/dev/null || echo 'Go not found')"
	@echo "Build Date: $(shell date)"

# Show project status
status:
	@echo "TrinityProxy Project Status"
	@echo "=========================="
	@echo "Repository: $(shell git remote get-url origin 2>/dev/null || echo 'No remote')"
	@echo "Branch: $(shell git branch --show-current 2>/dev/null || echo 'No git')"
	@echo "Last Commit: $(shell git log -1 --pretty=format:'%h - %s (%cr)' 2>/dev/null || echo 'No commits')"
	@export PATH="/usr/local/go/bin:$$PATH"; echo "Go Modules: $$(go list -m all 2>/dev/null | wc -l || echo 'N/A') dependencies"
	@echo "Build Status: $(shell [ -f $(BUILD_DIR)/$(BINARY_NAME) ] && echo 'Built' || echo 'Not built')"

# Debug PATH and environment
debug:
	@echo "TrinityProxy Debug Information"
	@echo "============================="
	@echo "Current PATH: $$PATH"
	@echo "Go in PATH: $$(command -v go 2>/dev/null || echo 'Not found')"
	@echo "Go in /usr/local/go/bin: $$(ls -la /usr/local/go/bin/go 2>/dev/null || echo 'Not found')"
	@export PATH="/usr/local/go/bin:$$PATH"; echo "Go with updated PATH: $$(command -v go 2>/dev/null || echo 'Not found')"
	@export PATH="/usr/local/go/bin:$$PATH"; echo "Go version: $$(go version 2>/dev/null || echo 'Not accessible')"
	@echo "Sockd in PATH: $$(command -v sockd 2>/dev/null || echo 'Not found')"
	@echo "[*] Current directory: $$(pwd)"
	@echo "User: $$(whoami)"

# Remove old TrinityProxy installation
cleanup:
	@if [ -f "scripts/cleanup.sh" ]; then \
		echo "[*] Running TrinityProxy cleanup script..."; \
		chmod +x scripts/cleanup.sh; \
		sudo bash scripts/cleanup.sh; \
	else \
		echo "[-] Cleanup script not found at scripts/cleanup.sh"; \
		echo "[*] Manual cleanup instructions:"; \
		echo "  sudo systemctl stop trinityproxy"; \
		echo "  sudo systemctl disable trinityproxy"; \
		echo "  sudo rm -f /etc/systemd/system/trinityproxy.service"; \
		echo "  sudo rm -f /etc/danted.conf /etc/trinityproxy-*"; \
		echo "  sudo rm -rf /root/TrinityProxy ~/TrinityProxy"; \
	fi
