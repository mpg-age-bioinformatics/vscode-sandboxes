#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
Usage: ./scripts/create-release.sh <sandbox> <platform>

Sandboxes:
  python | python-sandbox
  r | r-sandbox
  bioinformatics | bioinformatics-sandbox

Platforms:
  macos | windows

Examples:
  ./scripts/create-release.sh python macos
  ./scripts/create-release.sh r windows
EOF
}

[[ $# -eq 2 ]] || {
  usage
  exit 2
}

case "$1" in
  python | python-sandbox)
    sandbox_dir="python-sandbox"
    artifact_stem="Python-Sandbox"
    app_name="Python Sandbox"
    ;;
  r | r-sandbox)
    sandbox_dir="r-sandbox"
    artifact_stem="R-Sandbox"
    app_name="R Sandbox"
    ;;
  bioinformatics | bioinformatics-sandbox)
    sandbox_dir="bioinformatics-sandbox"
    artifact_stem="Bioinformatics-Sandbox"
    app_name="Bioinformatics Sandbox"
    ;;
  *)
    echo "Error: unknown sandbox: $1" >&2
    usage
    exit 2
    ;;
esac

case "$2" in
  mac | macos | darwin) platform="macos" ;;
  win | windows) platform="windows" ;;
  *)
    echo "Error: unknown platform: $2" >&2
    usage
    exit 2
    ;;
esac

script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
repository_root="$(dirname -- "$script_dir")"
sandbox_path="$repository_root/$sandbox_dir"
build_root="$(mktemp -d "${TMPDIR:-/tmp}/vscode-sandbox-release.XXXXXX")"

cleanup() {
  rm -rf -- "$build_root"
}
trap cleanup EXIT

[[ -d "$sandbox_path" ]] || {
  echo "Error: sandbox directory is unavailable: $sandbox_path" >&2
  exit 1
}

if [[ "$platform" == "macos" ]]; then
  [[ "$(uname -s)" == "Darwin" ]] || {
    echo "Error: macOS releases must be built on macOS." >&2
    exit 1
  }
  for dependency in bash hdiutil osacompile plutil; do
    command -v "$dependency" >/dev/null 2>&1 || {
      echo "Error: $dependency is required to build a macOS release." >&2
      exit 1
    }
  done

  temporary_artifact="$build_root/$artifact_stem.dmg"
  final_artifact="$sandbox_path/assets/$artifact_stem.dmg"
  app_path="$sandbox_path/assets/$app_name.app"

  echo "Validating $app_name application sources..."
  plutil -lint "$app_path/Contents/Info.plist" >/dev/null
  bash -n \
    "$app_path/Contents/MacOS/${sandbox_dir%-sandbox}-sandbox" \
    "$app_path/Contents/Resources/$app_name.command" \
    "$sandbox_path/scripts/build-macos-dmg.sh"
  if [[ -f "$app_path/Contents/Resources/Setup Form.js" ]]; then
    osacompile -l JavaScript -o "$build_root/Setup Form.scpt" \
      "$app_path/Contents/Resources/Setup Form.js"
  fi

  echo "Building $temporary_artifact..."
  "$sandbox_path/scripts/build-macos-dmg.sh" "$temporary_artifact"
  hdiutil verify "$temporary_artifact" >/dev/null
else
  for dependency in go file; do
    command -v "$dependency" >/dev/null 2>&1 || {
      echo "Error: $dependency is required to build a Windows release." >&2
      exit 1
    }
  done

  temporary_artifact="$build_root/$artifact_stem.exe"
  final_artifact="$sandbox_path/assets/$artifact_stem.exe"

  echo "Building $temporary_artifact..."
  "$sandbox_path/scripts/build-windows-exe.sh" "$temporary_artifact"
  file_description="$(file "$temporary_artifact")"
  case "$file_description" in
    *"PE32+ executable"*"x86-64"*) ;;
    *)
      echo "Error: the Windows build is not an x86-64 PE32+ executable: $file_description" >&2
      exit 1
      ;;
  esac
fi

cp "$temporary_artifact" "$final_artifact"
echo "Release artifact updated: $final_artifact"
if command -v shasum >/dev/null 2>&1; then
  shasum -a 256 "$final_artifact"
elif command -v sha256sum >/dev/null 2>&1; then
  sha256sum "$final_artifact"
fi
