#!/usr/bin/env bash
# Shared local-dev environment for TrinityProxy (sourced, not executed).
# Forces isolated DB paths, local controller URL, and dev-only keys.
# Never reads /etc/trinityproxy/ or /var/lib/trinityproxy/.

dev_env_root="${DEV_ENV_ROOT:-}"
if [[ -z "$dev_env_root" ]]; then
	if [[ -n "${BASH_SOURCE[0]:-}" ]] && [[ "$BASH_SOURCE[0]" != "${0:-}" ]]; then
		dev_env_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
	else
		dev_env_root="$(pwd)"
	fi
fi

DEV_DIR="${TRINITY_DEV_DIR:-$dev_env_root/.dev}"
DEV_ENV_FILE="$DEV_DIR/dev.env"
DEV_CONTROLLER_ENV="$DEV_DIR/.env.controller"

mkdir -p "$DEV_DIR"

export TRINITY_DEV=1
export TRINITY_ENV=development
export TRINITY_DEV_DIR="$DEV_DIR"

export DASHBOARD_DB_PATH="$DEV_DIR/dashboard.db"
export DB_PATH="$DEV_DIR/trinityproxy.db"
export CONTROLLER_URL="http://127.0.0.1:${CONTROLLER_PORT:-3100}"
export DASHBOARD_URL="http://localhost:${VITE_PORT:-8080}"
export CONTROLLER_ENV_FILE="$DEV_CONTROLLER_ENV"

# Prevent dashboard from syncing deployment settings from production host paths.
export TRINITY_CONTROLLER_ENV_PATH="/dev/null"
export TRINITY_CADDY_SITE_PATH="/dev/null"
export TRINITY_SYNC_CONTROLLER_URL="$CONTROLLER_URL"
export TRINITY_SYNC_SSL_MODE=none
export TRINITY_SYNC_PUBLIC_DOMAIN=localhost

dev_env_random_hex() {
	if command -v openssl >/dev/null 2>&1; then
		openssl rand -hex 32
	elif command -v python3 >/dev/null 2>&1; then
		python3 -c 'import secrets; print(secrets.token_hex(32))'
	else
		date +%s%N | shasum -a 256 | cut -c1-64
	fi
}

dev_env_ensure_secrets() {
	if [[ -f "$DEV_ENV_FILE" ]]; then
		# shellcheck source=/dev/null
		. "$DEV_ENV_FILE"
	fi

	local changed=0
	if [[ -z "${TRINITY_AGENT_KEY:-}" ]]; then
		TRINITY_AGENT_KEY="$(dev_env_random_hex)"
		changed=1
	fi
	if [[ -z "${TRINITY_API_KEY:-}" ]]; then
		TRINITY_API_KEY="$(dev_env_random_hex)"
		changed=1
	fi

	if [[ $changed -eq 1 ]]; then
		cat >"$DEV_ENV_FILE" <<EOF
# TrinityProxy local dev secrets (auto-generated — NOT connected to production)
# Regenerate: rm $DEV_ENV_FILE && make start-dev
TRINITY_AGENT_KEY=${TRINITY_AGENT_KEY}
TRINITY_API_KEY=${TRINITY_API_KEY}
EOF
		chmod 600 "$DEV_ENV_FILE"
	fi

	export TRINITY_AGENT_KEY
	export TRINITY_API_KEY
}

dev_env_warn_production_urls() {
	local url
	for url in \
		"${CONTROLLER_URL_BEFORE_OVERRIDE:-}" \
		"${USER_CONTROLLER_URL:-}"; do
		[[ -z "$url" ]] && continue
		case "$url" in
		http://127.0.0.1:* | http://localhost:* | https://127.0.0.1:* | https://localhost:*)
			continue
			;;
		esac
		echo "[!] WARNING: Ignoring production/non-local CONTROLLER_URL in your environment: $url"
		echo "    Local dev always uses http://127.0.0.1:${CONTROLLER_PORT:-3100}"
	done

	if [[ -f "$dev_env_root/.env.controller" ]] && [[ "$dev_env_root/.env.controller" != "$DEV_CONTROLLER_ENV" ]]; then
		if grep -qE '^[^#]*TRINITY_AGENT_KEY=' "$dev_env_root/.env.controller" 2>/dev/null; then
			echo "[!] WARNING: Ignoring repo-root .env.controller (may contain production keys)."
			echo "    Dev uses isolated keys in $DEV_CONTROLLER_ENV"
		fi
	fi
}

dev_env_apply() {
	USER_CONTROLLER_URL="${CONTROLLER_URL:-}"
	dev_env_ensure_secrets
	dev_env_warn_production_urls

	cat >"$DEV_CONTROLLER_ENV" <<EOF
# TrinityProxy local dev controller env (isolated — NOT production)
TRINITY_AGENT_KEY=${TRINITY_AGENT_KEY}
TRINITY_API_KEY=${TRINITY_API_KEY}
API_PORT=${CONTROLLER_PORT:-3100}
DB_PATH=${DB_PATH}
CONTROLLER_URL=${CONTROLLER_URL}
EOF
	chmod 600 "$DEV_CONTROLLER_ENV"
}

dev_env_print_banner() {
	echo ""
	echo "============================================================"
	echo "  LOCAL DEV ONLY — not connected to production"
	echo "============================================================"
	echo "  Controller:  $CONTROLLER_URL"
	echo "  Dashboard:   $DASHBOARD_URL"
	echo "  Auth DB:     $DASHBOARD_DB_PATH"
	echo "  Nodes DB:    $DB_PATH"
	echo "  Agent key:   ${TRINITY_AGENT_KEY:0:8}... (dev-only, in $DEV_ENV_FILE)"
	echo ""
	echo "  Do NOT use sudo for local dev on macOS."
	echo "  Production VPS: ssh to server and run 'make start' there."
	echo "============================================================"
	echo ""
}
