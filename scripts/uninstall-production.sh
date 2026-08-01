#!/usr/bin/env bash
#
# Remove TrinityProxy production install (systemd, binaries, state, config).
# Idempotent — safe to re-run. Does not remove your git clone.
#
# Usage: sudo ./scripts/uninstall-production.sh [--keep-user]
#    or: sudo make uninstall-production
#        sudo make uninstall-production UNINSTALL_OPTS=--keep-user

set -euo pipefail

export PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin${PATH:+:$PATH}"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=scripts/lib/production-common.sh
source "$ROOT/scripts/lib/production-common.sh"

KEEP_USER=0
for arg in "$@"; do
	case "$arg" in
	--keep-user) KEEP_USER=1 ;;
	-h | --help)
		echo "Usage: sudo $0 [--keep-user]"
		echo "  --keep-user  Do not delete the ${DASHBOARD_USER} system account"
		exit 0
		;;
	*)
		echo "[-] Unknown option: $arg (try --help)" >&2
		exit 1
		;;
	esac
done

if [[ "$(uname -s)" == "Darwin" ]]; then
	echo "[-] uninstall-production is for Linux VPS (systemd). On macOS use: make stop"
	exit 1
fi

if [[ $EUID -ne 0 ]]; then
	echo "[-] Error: run as root (e.g. sudo $0)" >&2
	exit 1
fi

if ! production_resolve_cmd systemctl >/dev/null 2>&1; then
	echo "[-] Error: systemd required for production uninstall" >&2
	exit 1
fi

rm_bin="$(production_resolve_cmd rm)" || {
	echo "[-] Error: rm not found" >&2
	exit 1
}

declare -a REMOVED=()
declare -a SKIPPED=()

note_removed() { REMOVED+=("$1"); }
note_skipped() { SKIPPED+=("$1"); }

stop_disable_unit() {
	local unit="$1"
	local unit_file="/etc/systemd/system/${unit}.service"
	local had=0
	if [[ -f "$unit_file" ]]; then
		had=1
	fi
	if production_systemctl cat "$unit" &>/dev/null 2>&1; then
		had=1
	fi
	if [[ $had -eq 0 ]]; then
		note_skipped "systemd unit $unit (not installed)"
		return 0
	fi
	echo "[*] Stopping and disabling $unit..."
	production_systemctl stop "$unit" 2>/dev/null || true
	production_systemctl disable "$unit" 2>/dev/null || true
	note_removed "systemd service $unit (stopped + disabled)"
}

remove_path() {
	local label="$1"
	local path="$2"
	if [[ -e "$path" ]]; then
		echo "[*] Removing $path ..."
		"$rm_bin" -rf -- "$path"
		note_removed "$label ($path)"
	else
		note_skipped "$label ($path not present)"
	fi
}

echo "============================================"
echo "  TrinityProxy production uninstall"
echo "============================================"
echo ""

for unit in trinityproxy-controller trinityproxy-dashboard; do
	stop_disable_unit "$unit"
done

for unit in trinityproxy-controller trinityproxy-dashboard; do
	unit_file="/etc/systemd/system/${unit}.service"
	if [[ -f "$unit_file" ]]; then
		echo "[*] Removing $unit_file ..."
		"$rm_bin" -f -- "$unit_file"
		note_removed "unit file $unit_file"
	fi
done

echo "[*] Reloading systemd..."
production_systemctl daemon-reload
note_removed "systemd daemon-reload"

remove_path "production binaries" "$OPT_PREFIX"
remove_path "state + databases" "$STATE_DIR"
remove_path "controller config" "$TRINITY_DIR"

if [[ $KEEP_USER -eq 1 ]]; then
	note_skipped "system user $DASHBOARD_USER (--keep-user)"
else
	id_bin="$(production_resolve_cmd id 2>/dev/null || true)"
	if [[ -n "$id_bin" ]] && "$id_bin" "$DASHBOARD_USER" &>/dev/null; then
		if production_remove_system_user "$DASHBOARD_USER"; then
			if ! "$id_bin" "$DASHBOARD_USER" &>/dev/null; then
				note_removed "system user $DASHBOARD_USER"
			else
				note_skipped "system user $DASHBOARD_USER (could not remove — use --keep-user or remove manually)"
			fi
		else
			note_skipped "system user $DASHBOARD_USER (removal failed)"
		fi
	else
		note_skipped "system user $DASHBOARD_USER (not present)"
	fi
fi

echo ""
echo "============================================"
echo "  Uninstall summary"
echo "============================================"
echo ""
if [[ ${#REMOVED[@]} -gt 0 ]]; then
	echo "Removed or updated:"
	for item in "${REMOVED[@]}"; do
		echo "  - $item"
	done
fi
if [[ ${#SKIPPED[@]} -gt 0 ]]; then
	echo ""
	echo "Skipped (already absent or opted out):"
	for item in "${SKIPPED[@]}"; do
		echo "  - $item"
	done
fi
echo ""
echo "Your TrinityProxy source repo was not modified."
echo ""
echo "Fresh install with new dashboard credentials:"
echo "  sudo make start"
echo ""
