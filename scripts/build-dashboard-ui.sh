#!/usr/bin/env bash
#
# Build dashboard React UI (npm) and sync web/dashboard/dist into cmd/dashboard/dist for go:embed.
# On VPS without Node.js, uses existing web/dashboard/dist or prior cmd/dashboard/dist embed tree.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

SRC_DIST="$ROOT/web/dashboard/dist"
EMBED_DIST="$ROOT/cmd/dashboard/dist"

embed_tree_valid() {
	[[ -f "$EMBED_DIST/index.html" ]]
}

src_tree_valid() {
	[[ -f "$SRC_DIST/index.html" ]]
}

embed_matches_src() {
	src_tree_valid && embed_tree_valid && cmp -s "$SRC_DIST/index.html" "$EMBED_DIST/index.html"
}

sync_dist_to_embed() {
	if embed_matches_src; then
		echo "[+] Embed UI already up to date at $EMBED_DIST"
		return 0
	fi

	if ! src_tree_valid; then
		echo "[-] Missing $SRC_DIST/index.html"
		exit 1
	fi

	mkdir -p "$EMBED_DIST"
	if command -v rsync >/dev/null 2>&1; then
		rsync -a --delete "$SRC_DIST/" "$EMBED_DIST/"
	else
		rm -rf "${EMBED_DIST:?}"/*
		cp -a "$SRC_DIST"/. "$EMBED_DIST/"
	fi
	echo "[+] Synced UI assets to $EMBED_DIST (go:embed in cmd/dashboard)"
}

build_with_npm() {
	if [[ ! -d "$ROOT/web/dashboard/node_modules" ]]; then
		echo "[*] Installing dashboard UI dependencies..."
		(cd "$ROOT/web/dashboard" && npm install)
	fi
	echo "[*] Building dashboard UI (npm run build)..."
	(cd "$ROOT/web/dashboard" && npm run build)
}

if command -v npm >/dev/null 2>&1; then
	build_with_npm
elif src_tree_valid; then
	echo "[+] npm not found — using existing $SRC_DIST"
elif embed_tree_valid; then
	echo "[+] npm not found — reusing embedded UI tree at $EMBED_DIST"
	exit 0
else
	echo "[-] Dashboard UI not built and npm not found."
	echo "    Build on a dev machine (make build) or install Node 18+ and re-run."
	exit 1
fi

sync_dist_to_embed
