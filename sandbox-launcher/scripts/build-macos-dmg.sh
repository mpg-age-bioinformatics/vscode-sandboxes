#!/usr/bin/env bash
set -euo pipefail
repository_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
output="${1:-$repository_root/sandbox-launcher/assets/VS-Code-Sandboxes.dmg}"
[[ ! -e "$output" ]] || { echo "Error: refusing to overwrite $output" >&2; exit 1; }
"$repository_root/scripts/build-desktop.sh"
build_root="$(mktemp -d "${TMPDIR:-/tmp}/unified-sandboxes.XXXXXX")"
trap 'rm -rf -- "$build_root"' EXIT
mkdir -p "$build_root/staging"
cp -R "$repository_root/sandbox-launcher/assets/VS Code Sandboxes.app" "$build_root/staging/"
cp "$repository_root/LICENSE" "$repository_root/THIRD-PARTY-NOTICES.txt" "$build_root/staging/"
ln -s /Applications "$build_root/staging/Applications"
hdiutil create -quiet -volname 'VS Code Sandboxes' -srcfolder "$build_root/staging" -format UDZO "$output"
echo "$output"
