#!/usr/bin/env bash
#
# Regenerate TrinityProxy dashboard admin login (username + temp password).
# Does not remove agent keys, deployment settings, or controller secrets.
#
# Usage:
#   sudo ./scripts/reset-dashboard-admin.sh
#   sudo make reset-dashboard-admin
#
# Dev (local dashboard.db in repo):
#   DASHBOARD_DB_PATH=./dashboard.db make reset-dashboard-admin

export PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin${PATH:+:$PATH}"
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
# shellcheck source=scripts/lib/production-common.sh
source "$ROOT/scripts/lib/production-common.sh"

DASHBOARD_DB="${DASHBOARD_DB_PATH:-$STATE_DIR/dashboard.db}"
DASHBOARD_BIN="${TRINITY_DASHBOARD_BIN:-}"

usage() {
	echo "Usage: sudo $0"
	echo "  Resets dashboard admin credentials in: $DASHBOARD_DB"
	echo "  Agent keys and deployment data in the same database are kept."
}

for arg in "$@"; do
	case "$arg" in
	-h | --help)
		usage
		exit 0
		;;
	*)
		echo "[-] Unknown option: $arg (try --help)" >&2
		exit 1
		;;
	esac
done

if [[ "$DASHBOARD_DB" == "$STATE_DIR/dashboard.db" ]] && [[ $EUID -ne 0 ]]; then
	echo "[-] Error: production reset requires root (e.g. sudo make reset-dashboard-admin)" >&2
	exit 1
fi

if [[ ! -f "$DASHBOARD_DB" ]]; then
	echo "[-] Error: dashboard database not found: $DASHBOARD_DB" >&2
	echo "    Install first: sudo make start  (or set DASHBOARD_DB_PATH for dev)" >&2
	exit 1
fi

resolve_dashboard_bin() {
	if [[ -n "$DASHBOARD_BIN" && -x "$DASHBOARD_BIN" ]]; then
		echo "$DASHBOARD_BIN"
		return
	fi
	if [[ -x "$OPT_BIN_DIR/trinityproxy-dashboard" ]]; then
		echo "$OPT_BIN_DIR/trinityproxy-dashboard"
		return
	fi
	if [[ -x "$ROOT/build/trinityproxy-dashboard" ]]; then
		echo "$ROOT/build/trinityproxy-dashboard"
		return
	fi
	echo ""
}

DASHBOARD_BIN="$(resolve_dashboard_bin)"
if [[ -z "$DASHBOARD_BIN" ]]; then
	echo "[*] Building dashboard binary..."
	make -C "$ROOT" build-dashboard >/dev/null
	DASHBOARD_BIN="$ROOT/build/trinityproxy-dashboard"
fi

DASHBOARD_URL="${DASHBOARD_URL:-}"
if [[ -z "$DASHBOARD_URL" ]]; then
	DASHBOARD_URL="$(production_http_url "${DASHBOARD_PORT:-8081}")"
fi

stopped_dashboard=0
if production_resolve_cmd systemctl >/dev/null 2>&1 && [[ $EUID -eq 0 ]]; then
	if production_systemctl is-active --quiet trinityproxy-dashboard 2>/dev/null; then
		echo "[*] Stopping trinityproxy-dashboard (avoids SQLite lock)..."
		production_systemctl stop trinityproxy-dashboard
		stopped_dashboard=1
	fi
fi

cleanup() {
	if [[ "$stopped_dashboard" -eq 1 ]]; then
		echo "[*] Starting trinityproxy-dashboard..."
		production_systemctl start trinityproxy-dashboard || true
	fi
}
trap cleanup EXIT

echo "[*] Resetting dashboard admin in $DASHBOARD_DB ..."
export DASHBOARD_DB_PATH="$DASHBOARD_DB"
export DASHBOARD_URL
"$DASHBOARD_BIN" --reset-admin

if [[ $EUID -eq 0 ]] && [[ "$DASHBOARD_DB" == "$STATE_DIR/dashboard.db" ]]; then
	chown_bin="$(production_resolve_cmd chown 2>/dev/null || true)"
	chmod_bin="$(production_resolve_cmd chmod 2>/dev/null || true)"
	if [[ -n "$chown_bin" ]]; then
		"$chown_bin" "$DASHBOARD_USER:$DASHBOARD_USER" "$DASHBOARD_DB"
	fi
	if [[ -n "$chmod_bin" ]]; then
		"$chmod_bin" 640 "$DASHBOARD_DB"
	fi
fi

echo "[+] Dashboard admin reset complete. Use the username and temp password printed above."
