#!/usr/bin/env bash
#
# Sync PUBLIC_DOMAIN / controller URL from Caddy + controller.env into dashboard.db.
#
# Usage:
#   sudo ./scripts/sync-deployment-settings.sh
#   TRINITY_SYNC_PUBLIC_DOMAIN=example.com TRINITY_SYNC_SSL_MODE=none sudo ./scripts/sync-deployment-settings.sh
#
set -euo pipefail

export PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin${PATH:+:$PATH}"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
# shellcheck source=scripts/lib/production-common.sh
source "$ROOT/scripts/lib/production-common.sh"

DASHBOARD_DB="${DASHBOARD_DB_PATH:-$STATE_DIR/dashboard.db}"
export DASHBOARD_DB_PATH="$DASHBOARD_DB"

resolve_dashboard_bin() {
	local candidate
	for candidate in \
		"$ROOT/build/trinityproxy-dashboard" \
		"${OPT_BIN_DIR:-/opt/trinityproxy/bin}/trinityproxy-dashboard" \
		"/opt/trinityproxy/bin/trinityproxy-dashboard"; do
		if [[ -x "$candidate" ]]; then
			echo "$candidate"
			return 0
		fi
	done
	return 1
}

if ! DASHBOARD_BIN="$(resolve_dashboard_bin)"; then
	echo "[!] trinityproxy-dashboard binary not found — run make build first" >&2
	exit 1
fi

if [[ ! -f "$DASHBOARD_DB" ]]; then
	echo "[!] Dashboard database not found: $DASHBOARD_DB" >&2
	echo "    Run make start or trinityproxy-dashboard --init-only first." >&2
	exit 1
fi

if [[ -n "${PUBLIC_DOMAIN:-}" && -z "${TRINITY_SYNC_PUBLIC_DOMAIN:-}" ]]; then
	export TRINITY_SYNC_PUBLIC_DOMAIN="$PUBLIC_DOMAIN"
fi

if [[ -n "${TRINITY_SYNC_PUBLIC_DOMAIN:-}" && -z "${TRINITY_SYNC_FORCE:-}" ]]; then
	export TRINITY_SYNC_FORCE=1
fi

echo "[*] Syncing deployment settings into $DASHBOARD_DB"
set +e
output="$("$DASHBOARD_BIN" --sync-deployment 2>&1)"
rc=$?
set -e
if [[ $rc -ne 0 ]]; then
	echo "$output" >&2
	exit $rc
fi
echo "$output"

if [[ "$(id -u)" -eq 0 ]] && [[ "$DASHBOARD_DB" == "$STATE_DIR/dashboard.db" ]]; then
	production_fixup_state_dir
fi
