#!/usr/bin/env bash
#
# TrinityProxy production bootstrap — one command for VPS controller + dashboard.
# Idempotent: safe to re-run after partial failure.
#
# Usage: make start
#    or: sudo ./scripts/start-production.sh

set -euo pipefail

export PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin${PATH:+:$PATH}"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# macOS / local dev — delegate to dev stack
if [[ "$(uname -s)" == "Darwin" ]]; then
	if [[ $EUID -eq 0 ]]; then
		echo "[-] Error: do not use 'sudo make start' on macOS."
		echo "    Local dev:  make start-dev   (or make start — same thing on macOS)"
		echo "    Production: ssh to your Linux VPS and run 'sudo make start' there."
		exit 1
	fi
	echo "[*] macOS detected — starting LOCAL DEV stack (not production VPS bootstrap)."
	echo "[*] Use 'make start-dev' explicitly if you prefer."
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

production_preserve_path() {
	echo "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin${PATH:+:$PATH}"
}

run_as_root() {
	local preserved_path
	preserved_path="$(production_preserve_path)"
	if [[ $EUID -eq 0 ]]; then
		env PATH="$preserved_path" "$@"
	else
		$SUDO env PATH="$preserved_path" "$@"
	fi
}

# shellcheck source=scripts/lib/production-common.sh
source "$ROOT/scripts/lib/production-common.sh"

apt_queue_pkg() {
	local cmd="$1" pkg="$2"
	if [[ -n "$cmd" ]] && command -v "$cmd" >/dev/null 2>&1; then
		echo "[+] $cmd present"
		return 0
	fi
	if command -v dpkg >/dev/null 2>&1 && dpkg -s "$pkg" >/dev/null 2>&1; then
		echo "[+] $pkg installed"
		if [[ -n "$cmd" ]] && ! command -v "$cmd" >/dev/null 2>&1; then
			APT_INSTALL_QUEUE+=("$pkg")
		fi
		return 0
	fi
	APT_INSTALL_QUEUE+=("$pkg")
}

apt_dedupe_queue() {
	local -a out=()
	local p seen
	for p in "${APT_INSTALL_QUEUE[@]}"; do
		seen=0
		for q in "${out[@]}"; do
			[[ "$q" == "$p" ]] && seen=1 && break
		done
		[[ $seen -eq 0 ]] && out+=("$p")
	done
	APT_INSTALL_QUEUE=("${out[@]}")
}

apt_flush_install_queue() {
	apt_dedupe_queue
	if [[ ${#APT_INSTALL_QUEUE[@]} -eq 0 ]]; then
		return 0
	fi
	echo "[*] Installing packages: ${APT_INSTALL_QUEUE[*]}"
	run_as_root apt-get update -y
	run_as_root apt-get install -y "${APT_INSTALL_QUEUE[@]}"
	APT_INSTALL_QUEUE=()
}

ensure_system_deps_debian() {
	APT_INSTALL_QUEUE=()
	apt_queue_pkg curl curl
	apt_queue_pkg wget wget
	apt_queue_pkg git git
	apt_queue_pkg make make
	apt_queue_pkg sqlite3 sqlite3
	apt_queue_pkg openssl openssl
	if ! command -v gcc >/dev/null 2>&1; then
		apt_queue_pkg gcc build-essential
	else
		echo "[+] gcc present"
	fi
	if ! command -v install >/dev/null 2>&1 || ! command -v id >/dev/null 2>&1 || ! command -v chown >/dev/null 2>&1; then
		apt_queue_pkg install coreutils
		apt_queue_pkg id coreutils
		apt_queue_pkg chown coreutils
	fi
	if ! command -v systemctl >/dev/null 2>&1; then
		APT_INSTALL_QUEUE+=("systemd")
	fi
	if production_have_user_mgmt; then
		echo "[+] adduser/useradd present"
	else
		APT_INSTALL_QUEUE+=("adduser" "passwd")
	fi
	if command -v dpkg >/dev/null 2>&1 && ! dpkg -s ca-certificates >/dev/null 2>&1; then
		APT_INSTALL_QUEUE+=("ca-certificates")
	else
		echo "[+] ca-certificates present"
	fi
	if ! command -v ip >/dev/null 2>&1; then
		apt_queue_pkg ip iproute2
	fi
	apt_flush_install_queue

	local cmd
	for cmd in curl wget git make sqlite3 openssl gcc install id chown systemctl; do
		if ! command -v "$cmd" >/dev/null 2>&1; then
			echo "[-] Required command missing after install: $cmd"
			exit 1
		fi
	done
	if ! production_have_user_mgmt; then
		echo "[-] Required user management tool missing (adduser or useradd)"
		exit 1
	fi
}

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
	local pkgs=("$@")
	if command -v apt-get >/dev/null 2>&1; then
		APT_INSTALL_QUEUE=("${pkgs[@]}")
		apt_flush_install_queue
		if command -v "$name" >/dev/null 2>&1; then
			return 0
		fi
	else
		for pkg in "${pkgs[@]}"; do
			if install_pkg "$pkg"; then
				command -v "$name" >/dev/null 2>&1 && return 0
			fi
		done
	fi
	echo "[-] Failed to install $name (tried: $*)"
	exit 1
}

ensure_system_deps() {
	echo "[1/11] Checking system dependencies (no Dante on controller)..."
	if command -v apt-get >/dev/null 2>&1; then
		ensure_system_deps_debian
		return 0
	fi
	ensure_command curl curl
	ensure_command make make
	ensure_command git git
	ensure_command wget wget
	ensure_command sqlite3 sqlite3 sqlite
	ensure_command openssl openssl
	ensure_command install coreutils
	ensure_command id coreutils
	ensure_command chown coreutils
	if production_have_user_mgmt; then
		echo "[+] adduser/useradd present"
	else
		ensure_command useradd passwd || ensure_command adduser adduser
		if ! production_have_user_mgmt; then
			echo "[-] Required user management tool missing (adduser or useradd)"
			exit 1
		fi
	fi
	if command -v gcc >/dev/null 2>&1; then
		echo "[+] gcc present"
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
	echo "[2/11] Checking Go..."
	export PATH="/usr/local/go/bin:$(production_preserve_path)"
	if command -v go >/dev/null 2>&1; then
		echo "[+] Go: $(go version)"
		return 0
	fi
	echo "[*] Go not found — running setup.sh (controller-only, no Dante)..."
	chmod +x scripts/setup.sh
	run_as_root env TRINITY_SKIP_DANTE=1 bash scripts/setup.sh
	export PATH="/usr/local/go/bin:$(production_preserve_path)"
	if ! command -v go >/dev/null 2>&1; then
		echo "[-] Go installation failed"
		exit 1
	fi
	echo "[+] Go: $(go version)"
}

build_all() {
	echo "[4/11] Building binaries + dashboard UI..."
	export PATH="/usr/local/go/bin:$(production_preserve_path)"
	chmod +x scripts/build-dashboard-ui.sh
	make build
}

install_systemd_units() {
	echo "[5/11] Installing systemd services..."
	export SKIP_BUILD=1 SKIP_START=1
	chmod +x scripts/install-service.sh scripts/install-dashboard-service.sh
	run_as_root env SKIP_BUILD=1 SKIP_START=1 bash -c "cd '$ROOT' && bash scripts/install-service.sh"
	run_as_root env SKIP_BUILD=1 SKIP_START=1 bash -c "cd '$ROOT' && bash scripts/install-dashboard-service.sh"
}


echo "============================================"
echo "  TrinityProxy production bootstrap"
echo "============================================"
echo ""

ensure_system_deps
ensure_go

echo "[3/11] Preparing controller environment..."
run_as_root bash -c "
	set -euo pipefail
	cd '$ROOT'
	# shellcheck source=scripts/lib/production-common.sh
	source '$ROOT/scripts/lib/production-common.sh'
	production_ensure_bootstrap_prereqs
	production_ensure_controller_env
"

build_all

install_systemd_units

echo "[6/11] Bootstrapping dashboard, syncing keys, and starting services..."
run_as_root bash -c "
	set -euo pipefail
	cd '$ROOT'
	# shellcheck source=scripts/lib/production-common.sh
	source '$ROOT/scripts/lib/production-common.sh'
	production_init_dashboard_admin
	production_sync_agent_key_to_controller_env
	production_fixup_state_dir
	production_install_binaries "$ROOT" trinityproxy-api trinityproxy-dashboard
	production_install_scripts "$ROOT"
	production_systemctl daemon-reload
	production_systemctl enable trinityproxy-controller trinityproxy-dashboard
	production_systemctl restart trinityproxy-dashboard
	production_systemctl restart trinityproxy-controller
	sleep 2
	if ! production_systemctl is-active trinityproxy-controller >/dev/null 2>&1 || ! production_systemctl is-active trinityproxy-dashboard >/dev/null 2>&1; then
		echo '[!] One or more services failed to start:'
		production_systemctl status trinityproxy-controller trinityproxy-dashboard --no-pager || true
		echo ''
		echo '[!] trinityproxy-controller journal (last 20 lines):'
		production_journalctl -u trinityproxy-controller -n 20 --no-pager || true
		exit 1
	fi
	true
"

echo "[7/11] Host firewall (ufw)..."
run_as_root bash -c "
	set -euo pipefail
	# shellcheck source=scripts/lib/production-common.sh
	source '$ROOT/scripts/lib/production-common.sh'
	production_configure_ufw_if_active
"


bootstrap_domain_https_step() {
	local caddy_site="/etc/caddy/trinityproxy.caddy"
	local setup_script="$ROOT/scripts/setup-domain.sh"
	local ans domain_rc

	if [[ ! -t 0 ]]; then
		echo "[*] No interactive TTY — skipping domain + HTTPS (run later: sudo make setup-domain)"
		export TRINITY_DOMAIN_SETUP_SKIPPED=1
		return 0
	fi

	if [[ -f "$caddy_site" ]]; then
		read -r -p "[?] Domain + HTTPS already configured ($caddy_site). Re-run setup? [y/N/skip]: " ans
		case "$ans" in
		y | Y | yes | YES) ;;
		skip | SKIP | s | S)
			export TRINITY_DOMAIN_SETUP_SKIPPED=1
			echo "[*] Keeping existing Caddy config."
			return 0
			;;
		*)
			echo "[+] Keeping existing Caddy configuration."
			return 0
			;;
		esac
	else
		read -r -p "[?] Set up domain + HTTPS now? [Y/n/skip]: " ans
		case "$ans" in
		n | N | no | NO | skip | SKIP | s | S)
			export TRINITY_DOMAIN_SETUP_SKIPPED=1
			echo "[*] Skipping domain + HTTPS for now."
			return 0
			;;
		'' | y | Y | yes | YES) ;;
		*)
			export TRINITY_DOMAIN_SETUP_SKIPPED=1
			echo "[*] Skipping domain + HTTPS for now."
			return 0
			;;
		esac
	fi

	chmod +x "$setup_script" "$ROOT/scripts/setup-ssl-caddy-cloudflare.sh" 2>/dev/null || true
	echo ""
	if [[ $EUID -eq 0 ]]; then
		set +e
		bash "$setup_script" --from-bootstrap
		domain_rc=$?
		set -e
	else
		set +e
		$SUDO bash "$setup_script" --from-bootstrap
		domain_rc=$?
		set -e
	fi

	case "$domain_rc" in
	0)
		echo "[+] Domain + HTTPS configured."
		;;
	2)
		export TRINITY_DOMAIN_SETUP_SKIPPED=1
		echo "[*] Continuing without HTTPS (dashboard on :8081)."
		;;
	1)
		echo "[-] Domain setup failed."
		exit 1
		;;
	*)
		echo "[-] Domain setup exited with unexpected code: ${domain_rc}"
		exit 1
		;;
	esac
}

echo "[8/11] Production port checklist..."
chmod +x "$ROOT/scripts/open-production-ports.sh" 2>/dev/null || true

echo "[9/11] Verifying dashboard reachability..."
run_as_root bash -c "
	set -euo pipefail
	# shellcheck source=scripts/lib/production-common.sh
	source '$ROOT/scripts/lib/production-common.sh'
	production_verify_dashboard_reachable || true
"


echo "[10/11] Domain + HTTPS (optional)..."
bootstrap_domain_https_step

run_as_root env TRINITY_DOMAIN_SETUP_SKIPPED="${TRINITY_DOMAIN_SETUP_SKIPPED:-}" bash -c "
	set -euo pipefail
	cd '$ROOT'
	# shellcheck source=scripts/lib/production-common.sh
	source '$ROOT/scripts/lib/production-common.sh'
	production_print_summary
"
