#!/usr/bin/env bash
# Build-time metadata only: installed launchers never need Git or network access.
sandbox_build_version="$(git -C "$repository_root" describe --tags --always --dirty 2>/dev/null || true)"
if [[ ! "$sandbox_build_version" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]]; then
  sandbox_build_version=development
fi
sandbox_version_ldflags="-X=github.com/mpg-age-bioinformatics/vscode-sandboxes/internal/startui.BuildVersion=$sandbox_build_version"
