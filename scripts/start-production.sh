#!/usr/bin/env bash
#
# TrinityProxy production bootstrap — one command for VPS controller + dashboard.
# Idempotent: safe to re-run after partial failure.
#
# Usage: make start
#    or: sudo ./scripts/start-production.sh

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# macOS / local dev — delegate to dev stack
if [[ "$(uname -s)" == "Darwin" ]]; then
	exec "$ROOT/scripts/start-dashboard-dev.sh"
fi

if [[ ! -f "cmd/api/enhanced_main.go" ]] || [[ ! -f "cmd/dashboard/main.go" ]]; then
	echo "[-] Error: run from the TrinityProxy repository root"
	exit 1
fi

if ! command -v systemctl >/dev/null 2>&1; then
	echo "[-] Error: systemd is required for production bootstrap (Linux VPS)"
	exit 1
fi

SUDO=""
if [[ $EUID -ne 0 ]]; then
	if command -v sudo >/dev/null 2>&1; then
		SUDO="sudo"
	else
		echo "[-] Error: run as root or install sudo for systemd setup"
		exit 1
	fi
fi

run_as_root() {
	if [[ $EUID -eq 0 ]]; then
		"$@"
	else
		$SUDO "$@"
	fi
}

# shellcheck source=scripts/lib/production-common.sh
source "$ROOT/scripts/lib/production-common.sh"

install_pkg() {
	local pkg="$1"
	if command -v apt-get >/dev/null 2>&1; then
		run_as_root apt-get update -y
		run_as_root apt-get install -y "$pkg"
	elif command -v dnf >/dev/null 2>&1; then
		run_as_root dnf install -y "$pkg"
	elif command -v yum >/dev/null 2>&1; then
		run_as_root yum install -y "$pkg"
	elif command -v pacman >/dev/null 2>&1; then
		run_as_root pacman -S --noconfirm "$pkg"
	elif command -v apk >/dev/null 2>&1; then
		run_as_root apk add --no-cache "$pkg"
	else
		echo "[-] No supported package manager to install $pkg"
		return 1
	fi
}

ensure_command() {
	local name="$1"
	shift
	if command -v "$name" >/dev/null 2>&1; then
		echo "[+] $name present"
		return 0
	fi
	echo "[*] Installing $name..."
	for pkg in "$@"; do
		if install_pkg "$pkg"; then
			command -v "$name" >/dev/null 2>&1 && return 0
		fi
	done
	echo "[-] Failed to install $name (tried: $*)"
	exit 1
}

ensure_system_deps() {
	echo "[1/10] Checking system dependencies (no Dante on controller)..."
	ensure_command curl curl
	ensure_command make make
	ensure_command git git
	ensure_command wget wget
	ensure_command sqlite3 sqlite3 sqlite
	if command -v gcc >/dev/null 2>&1; then
		echo "[+] gcc present"
	elif command -v apt-get >/dev/null 2>&1; then
		ensure_command gcc build-essential
	elif command -v dnf >/dev/null 2>&1 || command -v yum >/dev/null 2>&1; then
		ensure_command gcc gcc
		command -v g++ >/dev/null 2>&1 || install_pkg gcc-c++ || true
	elif command -v pacman >/dev/null 2>&1; then
		ensure_command gcc base-devel
	elif command -v apk >/dev/null 2>&1; then
		ensure_command gcc build-base
	fi
}

ensure_go() {
	echo "[2/10] Checking Go..."
	export PATH="/usr/local/go/bin:$PATH"
	if command -v go >/dev/null 2>&1; then
		echo "[+] Go: $(go version)"
		return 0
	fi
	echo "[*] Go not found — running setup.sh (controller-only, no Dante)..."
	chmod +x scripts/setup.sh
	run_as_root env TRINITY_SKIP_DANTE=1 bash scripts/setup.sh
	export PATH="/usr/local/go/bin:$PATH"
	if ! command -v go >/dev/null 2>&1; then
		echo "[-] Go installation failed"
		exit 1
	fi
	echo "[+] Go: $(go version)"
}

build_all() {
	echo "[4/10] Building binaries + dashboard UI..."
	export PATH="/usr/local/go/bin:$PATH"
	chmod +x scripts/build-dashboard-ui.sh
	make build
}

install_systemd_units() {
	echo "[6/10] Installing systemd services..."
	export SKIP_BUILD=1 SKIP_START=1
	chmod +x scripts/install-service.sh scripts/install-dashboard-service.sh
	run_as_root env SKIP_BUILD=1 SKIP_START=1 bash scripts/install-service.sh
	run_as_root env SKIP_BUILD=1 SKIP_START=1 bash scripts/install-dashboard-service.sh
}

start_services() {
	echo "[9/10] Starting services..."
	run_as_root systemctl daemon-reload
	run_as_root systemctl enable trinityproxy-controller trinityproxy-dashboard
	run_as_root systemctl restart trinityproxy-controller
	run_as_root systemctl restart trinityproxy-dashboard
	sleep 2
}

echo "============================================"
echo "  TrinityProxy production bootstrap"
echo "============================================"
echo ""

ensure_system_deps
ensure_go

echo "[3/10] Preparing controller environment..."
run_as_root bash -c "
	set -euo pipefail
	cd '$ROOT'
	# shellcheck source=scripts/lib/production-common.sh
	source '$ROOT/scripts/lib/production-common.sh'
	production_ensure_controller_env
	production_ensure_trinityproxy_user
	production_ensure_state_dir
"

build_all

install_systemd_units

echo "[7/10] Bootstrapping dashboard admin..."
run_as_root bash -c "
	set -euo pipefail
	cd '$ROOT'
	export PATH='/usr/local/go/bin:\$PATH'
	# shellcheck source=scripts/lib/production-common.sh
	source '$ROOT/scripts/lib/production-common.sh'
	production_init_dashboard_admin
"

echo "[8/10] Syncing agent key to controller.env..."
run_as_root bash -c "
	set -euo pipefail
	cd '$ROOT'
	# shellcheck source=scripts/lib/production-common.sh
	source '$ROOT/scripts/lib/production-common.sh'
	production_sync_agent_key_to_controller_env
"

start_services

echo "[10/10] Verifying services..."
CTRL_OK=0
DASH_OK=0
if run_as_root systemctl is-active trinityproxy-controller >/dev/null 2>&1; then
	CTRL_OK=1
fi
if run_as_root systemctl is-active trinityproxy-dashboard >/dev/null 2>&1; then
	DASH_OK=1
fi
if [[ $CTRL_OK -eq 0 ]] || [[ $DASH_OK -eq 0 ]]; then
	echo "[!] One or more services failed to start:"
	run_as_root systemctl status trinityproxy-controller trinityproxy-dashboard --no-pager || true
	exit 1
fi

production_print_summary
