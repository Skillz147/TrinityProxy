#!/usr/bin/env bash
#
# Build dashboard React UI (npm) and sync web/dashboard/dist into cmd/dashboard/dist for go:embed.
# On VPS without Node.js, uses existing web/dashboard/dist or prior cmd/dashboard/dist embed tree.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# shellcheck source=scripts/lib/dev-ui-permissions.sh
source "$ROOT/scripts/lib/dev-ui-permissions.sh"

SRC_DIST="$ROOT/web/dashboard/dist"
EMBED_DIST="$ROOT/cmd/dashboard/dist"
FALLBACK_DIST="$ROOT/.dev/vite-dist"
# Vite runs with cwd web/dashboard — use a path relative to that directory.
FALLBACK_VITE_OUT="../../.dev/vite-dist"

DID_NPM_BUILD=0
BUILT_DIST=""

embed_tree_valid() {
	[[ -f "$EMBED_DIST/index.html" ]]
}

src_tree_valid() {
	local dir="${1:-$SRC_DIST}"
	[[ -f "$dir/index.html" ]]
}

resolve_trinity_vite_out_dir() {
	local raw="${TRINITY_VITE_OUT_DIR:-}"
	local abs
	if [[ -z "$raw" ]]; then
		return 1
	fi
	if [[ "$raw" = /* ]]; then
		abs="$raw"
	else
		abs="$ROOT/$raw"
	fi
	printf '%s\n' "$abs"
}

vite_out_rel_from_repo_path() {
	local repo_rel="${1#./}"
	printf '../../%s\n' "$repo_rel"
}

embed_matches_src() {
	local dir="${1:-$SRC_DIST}"
	src_tree_valid "$dir" && embed_tree_valid && cmp -s "$dir/index.html" "$EMBED_DIST/index.html"
}

sync_dist_to_embed() {
	local src_dir="${1:-$SRC_DIST}"
	local force="${2:-0}"

	if [[ "$force" != "1" ]] && embed_matches_src "$src_dir"; then
		echo "[+] Embed UI already up to date at $EMBED_DIST"
		touch "$EMBED_DIST/.ui-sync-stamp"
		return 0
	fi

	if ! src_tree_valid "$src_dir"; then
		echo "[-] Missing $src_dir/index.html"
		exit 1
	fi

	if ! dev_ui_dir_writable "$EMBED_DIST"; then
		dev_ui_fix_dist_permissions "$ROOT" 1 || {
			echo "[-] Cannot sync UI — $EMBED_DIST is not writable."
			echo "    Run: make fix-dev-permissions"
			exit 1
		}
	fi

	mkdir -p "$EMBED_DIST"
	if command -v rsync >/dev/null 2>&1; then
		rsync -a --delete "$src_dir/" "$EMBED_DIST/"
	else
		rm -rf "${EMBED_DIST:?}"/*
		cp -a "$src_dir"/. "$EMBED_DIST/"
	fi
	echo "[+] Synced UI assets to $EMBED_DIST (go:embed in cmd/dashboard)"
	touch "$EMBED_DIST/.ui-sync-stamp"
}

build_with_npm() {
	DID_NPM_BUILD=1

	if [[ ! -d "$ROOT/web/dashboard/node_modules" ]]; then
		echo "[*] Installing dashboard UI dependencies..."
		(cd "$ROOT/web/dashboard" && npm install)
	fi

	local vite_out_rel="dist"
	BUILT_DIST="$SRC_DIST"

	if abs="$(resolve_trinity_vite_out_dir 2>/dev/null || true)" && [[ -n "$abs" ]]; then
		echo "[*] Using TRINITY_VITE_OUT_DIR=$abs"
		BUILT_DIST="$abs"
		if [[ "${TRINITY_VITE_OUT_DIR:-}" = /* ]]; then
			vite_out_rel="$abs"
		else
			vite_out_rel="$(vite_out_rel_from_repo_path "${TRINITY_VITE_OUT_DIR}")"
		fi
		rm -rf "$abs"
	elif [[ -d "$SRC_DIST" ]] && ! rm -rf "$SRC_DIST" 2>/dev/null; then
		echo "[!] $SRC_DIST is not writable (often root-owned after 'sudo make start')."
		echo "    Building to $FALLBACK_DIST instead."
		echo "    To fix permanently: make fix-dev-permissions   (or: sudo rm -rf $SRC_DIST)"
		BUILT_DIST="$FALLBACK_DIST"
		vite_out_rel="$FALLBACK_VITE_OUT"
		rm -rf "$FALLBACK_DIST"
	fi

	echo "[*] Building dashboard UI (npm run build)..."
	if [[ "$BUILT_DIST" == "$SRC_DIST" ]] && [[ -z "${TRINITY_VITE_OUT_DIR:-}" ]]; then
		(cd "$ROOT/web/dashboard" && npm run build)
	else
		(cd "$ROOT/web/dashboard" && npm run build -- --outDir "$vite_out_rel" --emptyOutDir)
	fi
	echo ""

	if ! src_tree_valid "$BUILT_DIST"; then
		echo "[-] Vite build finished but $BUILT_DIST/index.html is missing."
		exit 1
	fi
}

if command -v npm >/dev/null 2>&1; then
	build_with_npm
elif src_tree_valid; then
	echo "[+] npm not found — using existing $SRC_DIST"
	BUILT_DIST="$SRC_DIST"
elif src_tree_valid "$FALLBACK_DIST"; then
	echo "[+] npm not found — using existing $FALLBACK_DIST"
	BUILT_DIST="$FALLBACK_DIST"
elif embed_tree_valid; then
	echo "[+] npm not found — reusing embedded UI tree at $EMBED_DIST"
	exit 0
else
	echo "[-] Dashboard UI not built and npm not found."
	echo "    Build on a dev machine (make build) or install Node 18+ and re-run."
	exit 1
fi

if [[ -z "$BUILT_DIST" ]]; then
	echo "[-] Internal error: UI build output directory not set."
	exit 1
fi

sync_dist_to_embed "$BUILT_DIST" "$DID_NPM_BUILD"
