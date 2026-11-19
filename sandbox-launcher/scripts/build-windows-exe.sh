#!/usr/bin/env bash
set -euo pipefail
repository_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
output="${1:-$repository_root/sandbox-launcher/assets/VS-Code-Sandboxes.exe}"
[[ ! -e "$output" ]] || { echo "Error: refusing to overwrite $output" >&2; exit 1; }
go_winres="${GO_WINRES:-$(go env GOPATH)/bin/go-winres}"
[[ -x "$go_winres" ]] || { echo "Error: go-winres v0.3.3 is required. Run: go install github.com/tc-hib/go-winres@v0.3.3" >&2; exit 1; }
cd "$repository_root"
"$go_winres" simply --arch amd64 --out "$repository_root/sandbox-launcher/windows/rsrc" \
  --product-version 1.1.2 --file-version 1.1.2 --manifest cli \
  --file-description "VS Code - Docker Sandboxes" --product-name "VS Code - Docker Sandboxes" \
  --original-filename "VS-Code-Sandboxes.exe" --icon "$repository_root/sandbox-launcher/assets/SandboxesIcon-1024.png"
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags='-s -w -H windowsgui' -o "$output" ./sandbox-launcher/windows
echo "$output"
