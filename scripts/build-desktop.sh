#!/usr/bin/env bash
# Build the shared macOS startup controller and native window for all source apps.
set -euo pipefail
repository_root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
build_root="$(mktemp -d "${TMPDIR:-/tmp}/sandbox-desktop.XXXXXX")"
trap 'rm -rf -- "$build_root"' EXIT
cd "$repository_root"
export GOCACHE="${GOCACHE:-${TMPDIR:-/tmp}/vscode-sandbox-go-cache}"
export CLANG_MODULE_CACHE_PATH="${TMPDIR:-/tmp}/vscode-sandbox-clang-cache"
go build -trimpath -o "$build_root/Sandbox Desktop" ./cmd/sandbox-desktop
swiftc -O -module-cache-path "$CLANG_MODULE_CACHE_PATH" internal/desktop/Window.swift -o "$build_root/Sandbox Window"
install_helpers() {
  local destination="$1" binary temporary_binary
  for binary in "Sandbox Desktop" "Sandbox Window"; do
    # Replace the inode rather than overwriting an executable which a preview
    # may still have mapped; macOS caches its code signature for that inode.
    temporary_binary="$(mktemp "$destination/.sandbox-helper.XXXXXX")"
    cp "$build_root/$binary" "$temporary_binary"
    chmod 755 "$temporary_binary"
    mv -f "$temporary_binary" "$destination/$binary"
  done
}
for language in python r bioinformatics; do
  case "$language" in python) name=Python ;; r) name=R ;; bioinformatics) name=Bioinformatics ;; esac
  resources="$repository_root/$language-sandbox/assets/$name Sandbox.app/Contents/Resources"
  install_helpers "$resources"
done

# The primary application contains all three workers and setup bundles.
unified_resources="$repository_root/sandbox-launcher/assets/VS Code Sandboxes.app/Contents/Resources"
mkdir -p "$unified_resources/Launchers"
install_helpers "$unified_resources"
cp "$repository_root/LICENSE" "$repository_root/THIRD-PARTY-NOTICES.txt" "$repository_root/DISCLAIMER.txt" "$unified_resources/"
for language in python r bioinformatics; do
  case "$language" in python) name=Python ;; r) name=R ;; bioinformatics) name=Bioinformatics ;; esac
  worker="$unified_resources/Launchers/$language-sandbox"
  mkdir -p "$worker"
  cp "$repository_root/$language-sandbox/assets/$name Sandbox.app/Contents/Resources/$name Sandbox.command" "$worker/"
  cp "$repository_root/$language-sandbox/assets/$name Sandbox.app/Contents/Resources/${name}Sandbox.icns" "$worker/"
  rm -rf -- "$worker/Setup"
  cp -R "$repository_root/internal/launcherbundle/bundles/$language-sandbox" "$worker/Setup"
  cp "$repository_root/internal/launcherbundle/bundles/common/assets/"* "$worker/Setup/assets/"
  cp "$repository_root/LICENSE" "$repository_root/THIRD-PARTY-NOTICES.txt" "$repository_root/DISCLAIMER.txt" "$worker/Setup/"
  ln -sf '../../Sandbox Desktop' "$worker/Sandbox Desktop"
  ln -sf '../../Sandbox Window' "$worker/Sandbox Window"
done
