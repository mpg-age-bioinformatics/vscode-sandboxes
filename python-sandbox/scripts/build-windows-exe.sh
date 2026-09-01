#!/usr/bin/env bash
set -euo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
sandbox_dir="$(dirname -- "$script_dir")"
repository_root="$(dirname -- "$sandbox_dir")"
output_path="${1:-$sandbox_dir/assets/Python-Sandbox.exe}"
icon_path="$sandbox_dir/assets/PythonSandboxIcon-1024.png"
[[ "$output_path" == /* ]] || output_path="$(pwd -P)/$output_path"
[[ ! -e "$output_path" ]] || { echo "Error: refusing to overwrite existing output: $output_path" >&2; exit 1; }
command -v go >/dev/null 2>&1 || { echo "Error: Go is required to build the Windows launcher." >&2; exit 1; }
go_winres="${GO_WINRES:-$(go env GOPATH)/bin/go-winres}"
[[ -x "$go_winres" ]] || { echo "Error: go-winres v0.3.3 is required. Run: go install github.com/tc-hib/go-winres@v0.3.3" >&2; exit 1; }
[[ -f "$icon_path" ]] || { echo "Error: icon source is unavailable: $icon_path" >&2; exit 1; }

cd "$repository_root"
"$go_winres" simply --arch amd64 --out "$sandbox_dir/windows/rsrc" \
  --product-version 1.0.0 --file-version 1.0.0 --manifest cli \
  --file-description "Python Sandbox launcher" --product-name "Python Sandbox" \
  --original-filename "Python-Sandbox.exe" --icon "$icon_path"
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' \
  -o "$output_path" "./python-sandbox/windows"
echo "$output_path"

