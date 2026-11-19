#!/usr/bin/env bash
# Compatibility entry point; project preparation runs inside sbx via the controller.
set -euo pipefail
resources="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
exec "$resources/Sandbox Desktop" "R Sandbox" "r-sandbox" "4.5" "R version" "$0"
