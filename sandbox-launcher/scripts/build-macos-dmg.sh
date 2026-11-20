#!/usr/bin/env bash
set -euo pipefail
repository_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
output="${1:-$repository_root/sandbox-launcher/assets/VS-Code-Sandboxes.dmg}"
[[ ! -e "$output" ]] || { echo "Error: refusing to overwrite $output" >&2; exit 1; }
"$repository_root/scripts/build-desktop.sh"
build_root="$(mktemp -d "${TMPDIR:-/tmp}/unified-sandboxes.XXXXXX")"
mount_point=""
cleanup() {
  if [[ -n "$mount_point" ]]; then hdiutil detach "$mount_point" -quiet || true; fi
  rm -rf -- "$build_root"
}
trap cleanup EXIT
mkdir -p "$build_root/staging/.background"
cp -R "$repository_root/sandbox-launcher/assets/VS Code Sandboxes.app" "$build_root/staging/"
cp "$repository_root/LICENSE" "$repository_root/THIRD-PARTY-NOTICES.txt" "$repository_root/DISCLAIMER.txt" "$build_root/staging/"
ln -s /Applications "$build_root/staging/Applications"
swift -module-cache-path "${TMPDIR:-/tmp}/vscode-sandbox-clang-cache" \
  "$repository_root/internal/desktop/DMGBackground.swift" "$build_root/staging/.background/installation.tiff"
volume_name='VS Code - Sandboxes'
read_write_dmg="$build_root/VS-Code-Sandboxes-rw.dmg"
hdiutil create -quiet -volname "$volume_name" -srcfolder "$build_root/staging" -format UDRW "$read_write_dmg"
attach_output="$(hdiutil attach -readwrite -noverify -noautoopen "$read_write_dmg")"
mount_point="$(printf '%s\n' "$attach_output" | sed -n 's#^.*\(/Volumes/.*\)$#\1#p' | tail -n 1)"
[[ -n "$mount_point" && -d "$mount_point" ]] || { echo 'Error: could not locate the mounted DMG.' >&2; exit 1; }
layout_ready=0
for attempt in 1 2 3 4 5 6 7 8 9 10; do
  if osascript "$repository_root/sandbox-launcher/scripts/layout-dmg.applescript" "$(basename "$mount_point")" "$mount_point" >/dev/null 2>&1; then
    layout_ready=1
    break
  fi
  sleep 1
done
[[ "$layout_ready" -eq 1 ]] || { echo 'Error: Finder could not save the DMG installation layout.' >&2; exit 1; }
sync
hdiutil detach "$mount_point" -quiet
mount_point=""
hdiutil convert -quiet "$read_write_dmg" -format UDZO -imagekey zlib-level=9 -o "$output"
echo "$output"
