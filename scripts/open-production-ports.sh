#!/usr/bin/env bash
#
# Open TrinityProxy production ports on the host firewall (ufw) and print cloud VPC rules.
# Usage: sudo ./scripts/open-production-ports.sh

set -euo pipefail

export PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin${PATH:+:$PATH}"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=scripts/lib/production-common.sh
source "$ROOT/scripts/lib/production-common.sh"

production_configure_ufw_if_active
production_print_cloud_firewall_instructions
