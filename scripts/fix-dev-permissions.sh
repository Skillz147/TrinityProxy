#!/usr/bin/env bash
#
# Restore user ownership on dashboard UI dist dirs (after accidental sudo make start on macOS).

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=scripts/lib/dev-ui-permissions.sh
source "$ROOT/scripts/lib/dev-ui-permissions.sh"

auto=0
for arg in "$@"; do
	case "$arg" in
	--auto) auto=1 ;;
	esac
done

dev_ui_fix_dist_permissions "$ROOT" "$auto"
