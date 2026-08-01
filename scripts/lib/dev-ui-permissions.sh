#!/usr/bin/env bash
# Shared helpers for local dev dashboard UI dist directories (sourced, not executed).

dev_ui_dir_writable() {
	local dir="$1"
	[[ -d "$dir" ]] || return 0
	local probe="$dir/.trinity-write-test"
	if touch "$probe" 2>/dev/null; then
		rm -f "$probe"
		return 0
	fi
	return 1
}

# Fix root-owned web/dashboard/dist or cmd/dashboard/dist after accidental sudo make start.
dev_ui_fix_dist_permissions() {
	local root="${1:-}"
	local auto="${2:-0}"
	local dir user group

	if [[ -z "$root" ]]; then
		echo "[-] dev_ui_fix_dist_permissions: missing repo root" >&2
		return 1
	fi

	user="$(id -un)"
	group="$(id -gn)"

	for dir in "$root/web/dashboard/dist" "$root/cmd/dashboard/dist"; do
		[[ -d "$dir" ]] || continue
		if dev_ui_dir_writable "$dir"; then
			continue
		fi
		if [[ "$auto" == "1" ]]; then
			echo "[!] $dir is not writable — attempting to fix ownership..."
		else
			echo "[*] Fixing permissions: $dir"
		fi
		if [[ $EUID -eq 0 ]]; then
			local owner="${SUDO_USER:-$user}"
			chown -R "$owner:$(id -gn "$owner" 2>/dev/null || echo staff)" "$dir"
		elif command -v sudo >/dev/null 2>&1; then
			if ! sudo -n chown -R "$user:$group" "$dir" 2>/dev/null; then
				echo "    Run: sudo chown -R $user:$group $dir"
				echo "    Or:  make fix-dev-permissions   (will prompt for sudo)"
				return 1
			fi
		else
			echo "[-] $dir is not writable and sudo is unavailable." >&2
			return 1
		fi
		chmod -R u+rwX "$dir" 2>/dev/null || true
		if ! dev_ui_dir_writable "$dir"; then
			echo "[-] Still not writable: $dir" >&2
			return 1
		fi
		echo "[+] Fixed permissions on $dir"
	done
	return 0
}
