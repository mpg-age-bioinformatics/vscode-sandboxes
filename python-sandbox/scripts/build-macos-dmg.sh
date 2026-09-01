#!/usr/bin/env bash
set -euo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
skill_dir="$(dirname -- "$script_dir")"
app_path="$skill_dir/assets/Python Sandbox.app"
layout_script="$script_dir/layout-dmg.applescript"
output_path="${1:-$skill_dir/assets/Python-Sandbox.dmg}"
volume_name="Python Sandbox"
temporary_root="${TMPDIR:-/tmp}"
build_root="$(mktemp -d "${temporary_root%/}/python-sandbox-dmg.XXXXXX")"
mount_point=""

cleanup() {
  if [[ -n "$mount_point" && -d "$mount_point" ]]; then
    hdiutil detach "$mount_point" -quiet || true
  fi
  rm -rf -- "$build_root"
}
trap cleanup EXIT

[[ -d "$app_path" ]] || { echo "Error: application bundle is unavailable: $app_path" >&2; exit 1; }
[[ -f "$layout_script" ]] || { echo "Error: DMG layout script is unavailable: $layout_script" >&2; exit 1; }
[[ "$output_path" == /* ]] || output_path="$(pwd -P)/$output_path"
[[ ! -e "$output_path" ]] || { echo "Error: refusing to overwrite existing output: $output_path" >&2; exit 1; }

staging_dir="$build_root/staging"
read_write_dmg="$build_root/Python-Sandbox-rw.dmg"
mkdir -p "$staging_dir"
cp -R "$app_path" "$staging_dir/Python Sandbox.app"
ln -s /Applications "$staging_dir/Applications"

hdiutil create -quiet -volname "$volume_name" -srcfolder "$staging_dir" -format UDRW "$read_write_dmg"
attach_output="$(hdiutil attach -readwrite -noverify -noautoopen "$read_write_dmg")"
mount_point="$(printf '%s\n' "$attach_output" | sed -n 's#^.*\(/Volumes/.*\)$#\1#p' | tail -n 1)"
[[ -n "$mount_point" && -d "$mount_point" ]] || { echo "Error: could not determine the mounted DMG path." >&2; exit 1; }

layout_ready=0
for attempt in 1 2 3 4 5 6 7 8 9 10; do
  if osascript "$layout_script" "$volume_name" >/dev/null 2>&1; then
    layout_ready=1
    break
  fi
  sleep 1
done
[[ $layout_ready -eq 1 ]] || {
  osascript "$layout_script" "$volume_name"
  echo "Error: Finder could not apply the DMG layout." >&2
  exit 1
}
sync
hdiutil detach "$mount_point" -quiet
mount_point=""
hdiutil convert -quiet "$read_write_dmg" -format UDZO -imagekey zlib-level=9 -o "$output_path"
echo "$output_path"
