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

# Auto-detect local dev when .dev/dashboard.db exists or TRINITY_DEV=1.
if [[ "${TRINITY_DEV:-}" == "1" ]] || [[ -f "$ROOT/.dev/dashboard.db" ]]; then
	if [[ -z "${DASHBOARD_DB_PATH:-}" ]]; then
		export DASHBOARD_DB_PATH="$ROOT/.dev/dashboard.db"
	fi
	if [[ -z "${DASHBOARD_URL:-}" ]]; then
		export DASHBOARD_URL="http://localhost:8080"
	fi
fi

# shellcheck source=scripts/lib/production-common.sh
source "$ROOT/scripts/lib/production-common.sh"

DASHBOARD_DB="${DASHBOARD_DB_PATH:-$STATE_DIR/dashboard.db}"
DASHBOARD_BIN="${TRINITY_DASHBOARD_BIN:-}"

usage() {
	echo "Usage: sudo $0   (production VPS)"
	echo "       make reset-dashboard-admin-dev   (local dev on macOS/Linux)"
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
	echo "[-] Error: production reset requires root (run on your VPS: sudo make reset-dashboard-admin)" >&2
	echo "    Local dev: make reset-dashboard-admin-dev" >&2
	exit 1
fi

is_dev_db=0
if [[ "$DASHBOARD_DB" == "$ROOT/.dev/dashboard.db" ]] || [[ "${TRINITY_DEV:-}" == "1" ]]; then
	is_dev_db=1
fi

if [[ $is_dev_db -eq 1 ]]; then
	echo "[*] Local dev reset (no sudo): $DASHBOARD_DB"
fi

if [[ ! -f "$DASHBOARD_DB" ]]; then
	echo "[-] Error: dashboard database not found: $DASHBOARD_DB" >&2
	if [[ $is_dev_db -eq 1 ]]; then
		echo "    Run 'make start-dev' first to create the dev database." >&2
	else
		echo "    Install first: sudo make start  (or set DASHBOARD_DB_PATH for dev)" >&2
	fi
	exit 1
fi

dashboard_bin_supports_reset_admin() {
	local bin="$1"
	if [[ ! -x "$bin" ]]; then
		return 1
	fi
	if command -v strings >/dev/null 2>&1; then
		strings "$bin" 2>/dev/null | grep -qF '--reset-admin'
	else
		grep -aqF '--reset-admin' "$bin" 2>/dev/null
	fi
}

build_dashboard_for_reset() {
	echo "[*] Building dashboard binary for --reset-admin (go build)..."
	mkdir -p "$ROOT/build"
	export PATH="/usr/local/go/bin:${PATH}"
	if ! command -v go >/dev/null 2>&1; then
		echo "[-] Error: go not found; install Go or set TRINITY_DASHBOARD_BIN to a binary with --reset-admin" >&2
		return 1
	fi
	go build -o "$ROOT/build/trinityproxy-dashboard" ./cmd/dashboard
}

resolve_dashboard_bin() {
	if [[ -n "$DASHBOARD_BIN" && -x "$DASHBOARD_BIN" ]]; then
		echo "$DASHBOARD_BIN"
		return
	fi
	local candidate
	for candidate in \
		"$ROOT/build/trinityproxy-dashboard" \
		"$OPT_BIN_DIR/trinityproxy-dashboard"; do
		if [[ -x "$candidate" ]] && dashboard_bin_supports_reset_admin "$candidate"; then
			echo "$candidate"
			return
		fi
	done
	echo ""
}

echo "[*] Resolving dashboard binary..."
DASHBOARD_BIN="$(resolve_dashboard_bin)"
if [[ -z "$DASHBOARD_BIN" ]] || ! dashboard_bin_supports_reset_admin "$DASHBOARD_BIN"; then
	if [[ -n "$DASHBOARD_BIN" ]] && ! dashboard_bin_supports_reset_admin "$DASHBOARD_BIN"; then
		echo "[!] $DASHBOARD_BIN lacks --reset-admin (deployed binary may be stale); rebuilding..."
	fi
	build_dashboard_for_reset
	DASHBOARD_BIN="$ROOT/build/trinityproxy-dashboard"
fi
if [[ ! -x "$DASHBOARD_BIN" ]] || ! dashboard_bin_supports_reset_admin "$DASHBOARD_BIN"; then
	echo "[-] Error: no dashboard binary with --reset-admin support" >&2
	exit 1
fi
echo "[+] Using dashboard binary: $DASHBOARD_BIN"

DASHBOARD_URL="${DASHBOARD_URL:-}"
if [[ -z "$DASHBOARD_URL" ]]; then
	DASHBOARD_URL="$(production_http_url "${DASHBOARD_PORT:-8081}")"
fi

stopped_dashboard=0
if production_resolve_cmd systemctl >/dev/null 2>&1 && [[ $EUID -eq 0 ]]; then
	if production_systemctl is-active --quiet trinityproxy-dashboard 2>/dev/null; then
		production_systemctl_stop_unit trinityproxy-dashboard 45
		stopped_dashboard=1
	fi
fi

if [[ $EUID -eq 0 ]] && production_resolve_cmd systemctl >/dev/null 2>&1; then
	if production_systemctl is-active --quiet trinityproxy-dashboard 2>/dev/null; then
		echo "[-] Error: trinityproxy-dashboard is still running; cannot reset (SQLite lock)" >&2
		echo "    Try: sudo systemctl kill -s SIGKILL trinityproxy-dashboard" >&2
		exit 1
	fi
fi

cleanup() {
	if [[ "$stopped_dashboard" -eq 1 ]]; then
		production_systemctl_start_unit trinityproxy-dashboard 60 || true
	fi
}
trap cleanup EXIT

echo "[*] Resetting dashboard admin in $DASHBOARD_DB ..."
export DASHBOARD_DB_PATH="$DASHBOARD_DB"
export DASHBOARD_URL
if command -v timeout >/dev/null 2>&1; then
	if ! timeout 120 "$DASHBOARD_BIN" --reset-admin; then
		code=$?
		if [[ "$code" -eq 124 ]]; then
			echo "[-] Error: --reset-admin timed out (binary may have started the server instead of resetting)" >&2
			echo "    Rebuild/install dashboard: make build-dashboard && sudo make start-production" >&2
		fi
		exit "$code"
	fi
else
	"$DASHBOARD_BIN" --reset-admin
fi

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
