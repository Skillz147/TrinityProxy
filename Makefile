# TrinityProxy Makefile
# Easy build and deployment for SOCKS5 proxy network

.PHONY: help build clean install deps test run-controller run-agent setup-dev check-deps format lint setup-system vps-setup setup-api-controller quickstart

# Default target
all: deps build

# Help target - shows available commands
help:
	@echo "TrinityProxy Build System"
	@echo "========================="
	@echo "Available targets:"
	@echo ""
	@echo "Quick Setup:"
	@echo "  make quickstart        - Standard setup (after system dependencies)"
	@echo "  make vps-setup         - Complete VPS setup (includes system setup)"
	@echo "  make setup-system      - Install system dependencies (Go, Dante, etc.)"
	@echo ""
	@echo "Build & Dependencies:"
	@echo "  make all               - Install dependencies and build everything"
	@echo "  make build             - Build all binaries"
	@echo "  make deps              - Install Go dependencies"
	@echo "  make install           - Install system dependencies (requires sudo)"
	@echo "  make clean             - Clean build artifacts"
	@echo ""
	@echo "Development:"
	@echo "  make setup-dev         - Complete development setup"
	@echo "  make test              - Run tests"
	@echo "  make format            - Format Go code"
	@echo "  make lint              - Run linter"
	@echo "  make check-deps        - Check system dependencies"
	@echo ""
	@echo "Runtime:"
	@echo "  make run-controller    - Start in controller mode"
	@echo "  make run-agent         - Start in agent mode"
	@echo "  make run               - Interactive role selection"
	@echo ""
	@echo "VPS Deployment:"
	@echo "  make setup-api-controller - Setup controller with SSL/NGINX"
	@echo "  make deploy-vps        - Deploy to VPS (set VPS_HOST variable)"
	@echo "  make install-dante     - Install Dante SOCKS5 server only"

# Variables
BINARY_NAME=trinityproxy
BUILD_DIR=build
GO_FILES=$(shell find . -name "*.go" -type f)
INSTALLER_BINARY=$(BUILD_DIR)/installer
API_BINARY=$(BUILD_DIR)/api

# Go build flags
LDFLAGS=-ldflags "-X main.Version=$(shell git describe --tags --always --dirty 2>/dev/null || echo 'dev')"

# Build all binaries
build: $(BUILD_DIR)/$(BINARY_NAME) $(INSTALLER_BINARY) $(API_BINARY)
	@echo "[+] Build complete!"
	@echo "[*] Main binary: $(BUILD_DIR)/$(BINARY_NAME)"
	@echo "[*] Installer: $(INSTALLER_BINARY)"
	@echo "[*] API Server: $(API_BINARY)"

# Build main binary
$(BUILD_DIR)/$(BINARY_NAME): $(GO_FILES) | $(BUILD_DIR)
	@echo "[*] Building main binary..."
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .

# Build installer binary
$(INSTALLER_BINARY): cmd/installer/installer.go | $(BUILD_DIR)
	@echo "[*] Building installer..."
	go build $(LDFLAGS) -o $(INSTALLER_BINARY) ./cmd/installer

# Build API server binary
$(API_BINARY): cmd/api/enhanced_main.go | $(BUILD_DIR)
	@echo "[*] Building API server..."
	go build $(LDFLAGS) -o $(API_BINARY) ./cmd/api

# Create build directory
$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

# Install Go dependencies
deps:
	@echo "[*] Installing Go dependencies..."
	go mod download
	go mod tidy
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
	@command -v go >/dev/null 2>&1 || (echo "[-] Go is not installed!" && exit 1)
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
	go test -v ./...

# Format Go code
format:
	@echo "[*] Formatting Go code..."
	go fmt ./...
	@echo "[+] Code formatted!"

# Run linter (requires golangci-lint)
lint:
	@echo "[*] Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "[!] golangci-lint not found. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

# Clean build artifacts
clean:
	@echo "[*] Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	go clean
	@echo "[+] Clean complete!"

# Runtime targets
run: build
	@echo "[*] Starting TrinityProxy with interactive setup..."
	./$(BUILD_DIR)/$(BINARY_NAME)

run-controller: build
	@echo "[*] Starting TrinityProxy in Controller mode..."
	TRINITY_ROLE=controller ./$(BUILD_DIR)/$(BINARY_NAME)

run-agent: build
	@echo "[*] Starting TrinityProxy in Agent mode..."
	TRINITY_ROLE=agent ./$(BUILD_DIR)/$(BINARY_NAME)

# Development helpers
dev-controller: build
	@echo "[*] Starting development controller (with auto-restart)..."
	@if command -v entr >/dev/null 2>&1; then \
		find . -name "*.go" | entr -r make run-controller; \
	else \
		echo "[!] Install 'entr' for auto-restart: apt-get install entr"; \
		make run-controller; \
	fi

dev-agent: build
	@echo "[*] Starting development agent (with auto-restart)..."
	@if command -v entr >/dev/null 2>&1; then \
		find . -name "*.go" | entr -r make run-agent; \
	else \
		echo "[!] Install 'entr' for auto-restart: apt-get install entr"; \
		make run-agent; \
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

# Quick start for new clones
quickstart:
	@echo "TrinityProxy Quick Start"
	@echo "======================="
	@echo "[1/5] Setting up system dependencies..."
	@make setup-system
	@echo "[2/5] Checking dependencies..."
	@make check-deps
	@echo "[3/5] Installing Go dependencies..."
	@make deps
	@echo "[4/5] Building binaries..."
	@make build
	@echo "[5/5] Ready to run!"
	@echo ""
	@echo "Next steps:"
	@echo "  make run              - Interactive setup"
	@echo "  make run-controller   - Start controller"
	@echo "  make run-agent        - Start agent"
	@echo ""

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

# VPS-specific quickstart (includes system setup)
vps-setup: setup-system quickstart
	@echo ""
	@echo "[+] VPS Setup Complete!"
	@echo "======================"
	@echo "Your VPS is now ready to run TrinityProxy."
	@echo ""
	@echo "Quick commands:"
	@echo "  make run-agent        - Start as SOCKS5 proxy agent"
	@echo "  make run-controller   - Start as management controller"
	@echo "  make run              - Interactive role selection"
	@echo ""

# API Controller setup with SSL (uses setup_api.sh)
setup-api-controller:
	@if [ -f "scripts/setup_api.sh" ]; then \
		echo "[*] Setting up API controller with SSL..."; \
		chmod +x scripts/setup_api.sh; \
		sudo bash scripts/setup_api.sh; \
		echo "[+] API controller setup complete!"; \
	else \
		echo "[!] API setup script not found. Using basic controller setup..."; \
		make run-controller; \
	fi

# Version info
version:
	@echo "TrinityProxy Build System"
	@echo "Git Version: $(shell git describe --tags --always --dirty 2>/dev/null || echo 'dev')"
	@echo "Go Version: $(shell go version)"
	@echo "Build Date: $(shell date)"

# Show project status
status:
	@echo "TrinityProxy Project Status"
	@echo "=========================="
	@echo "Repository: $(shell git remote get-url origin 2>/dev/null || echo 'No remote')"
	@echo "Branch: $(shell git branch --show-current 2>/dev/null || echo 'No git')"
	@echo "Last Commit: $(shell git log -1 --pretty=format:'%h - %s (%cr)' 2>/dev/null || echo 'No commits')"
	@echo "Go Modules: $(shell go list -m all | wc -l) dependencies"
	@echo "Build Status: $(shell [ -f $(BUILD_DIR)/$(BINARY_NAME) ] && echo 'Built' || echo 'Not built')"
