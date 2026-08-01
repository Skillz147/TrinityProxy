#!/usr/bin/env bash
#
# Install TrinityProxy controller + dashboard as systemd services (VPS production).
# Delegates to start-production.sh (idempotent full bootstrap).
#
# Usage: sudo ./scripts/install-production.sh
#    or: make install-production

export PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin${PATH:+:$PATH}"
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=scripts/lib/production-common.sh
source "$ROOT/scripts/lib/production-common.sh"
exec "$ROOT/scripts/start-production.sh"
