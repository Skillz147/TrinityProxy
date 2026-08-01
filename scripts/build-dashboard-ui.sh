#!/usr/bin/env bash
#
# Build dashboard React UI (npm) and sync web/dashboard/dist into cmd/dashboard/dist for go:embed.
# On VPS without Node.js, uses existing web/dashboard/dist or prior cmd/dashboard/dist embed tree.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

SRC_DIST="$ROOT/web/dashboard/dist"
EMBED_DIST="$ROOT/cmd/dashboard/dist"

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
elif [[ -f "$SRC_DIST/index.html" ]]; then
	echo "[+] npm not found — using existing $SRC_DIST"
elif [[ -f "$EMBED_DIST/index.html" ]]; then
	echo "[+] npm not found — reusing embedded UI tree at $EMBED_DIST"
	exit 0
else
	echo "[-] Dashboard UI not built and npm not found."
	echo "    Build on a dev machine (make build) or install Node 18+ and re-run."
	exit 1
fi

if [[ ! -f "$SRC_DIST/index.html" ]]; then
	echo "[-] Missing $SRC_DIST/index.html after build"
	exit 1
fi

mkdir -p "$EMBED_DIST"
rsync -a --delete "$SRC_DIST/" "$EMBED_DIST/"
echo "[+] Synced UI assets to $EMBED_DIST (go:embed in cmd/dashboard)"
