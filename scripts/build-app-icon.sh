#!/usr/bin/env bash
# Recreate the shared app icon from the mark used in internal/startui/window.html.
set -euo pipefail
repository_root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
build_root="$(mktemp -d "${TMPDIR:-/tmp}/sandboxes-icon.XXXXXX")"
trap 'rm -rf -- "$build_root"' EXIT
assets="$repository_root/sandbox-launcher/assets"
swift -module-cache-path "${TMPDIR:-/tmp}/vscode-sandbox-clang-cache" \
  "$repository_root/scripts/render-app-icon.swift" "$assets" "$build_root/Sandboxes.iconset"
iconutil -c icns "$build_root/Sandboxes.iconset" \
  -o "$assets/VS Code Sandboxes.app/Contents/Resources/Sandboxes.icns"
