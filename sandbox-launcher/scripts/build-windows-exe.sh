#!/usr/bin/env bash
set -euo pipefail
repository_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
output="${1:-$repository_root/sandbox-launcher/assets/VS-Code-Sandboxes.exe}"
[[ ! -e "$output" ]] || { echo "Error: refusing to overwrite $output" >&2; exit 1; }
cd "$repository_root"
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags='-s -w -H windowsgui' -o "$output" ./sandbox-launcher/windows
echo "$output"
