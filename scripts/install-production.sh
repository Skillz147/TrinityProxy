#!/usr/bin/env bash
#
# Install TrinityProxy controller + dashboard as systemd services (VPS production).
# Delegates to start-production.sh (idempotent full bootstrap).
#
# Usage: sudo ./scripts/install-production.sh
#    or: make install-production

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
exec "$ROOT/scripts/start-production.sh"
