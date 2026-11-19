#!/usr/bin/env bash
set -euo pipefail
script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
shared="$script_dir/run-language-sandbox.sh"
if [[ ! -f "$shared" ]]; then shared="$script_dir/../../common/assets/run-language-sandbox.sh"; fi
exec bash "$shared" r "$@"
