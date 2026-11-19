#!/usr/bin/env bash
# Recorded OS requirements for the R development container, never the host.
set -euo pipefail
packages=(build-essential ca-certificates cmake curl gfortran git libbz2-dev libcurl4-openssl-dev libfontconfig1-dev libfreetype-dev libfribidi-dev libgit2-dev libharfbuzz-dev libjpeg-dev liblzma-dev libpcre2-dev libpng-dev libssl-dev libtiff-dev libuv1-dev libxml2-dev pkg-config zlib1g-dev)
lock=code/.sandbox/r-system-packages.lock
missing=()
for package in "${packages[@]}"; do
  installed="$(dpkg-query -W -f='${db:Status-Status}' "$package" 2>/dev/null || true)"
  version=""
  if [[ -f "$lock" ]]; then version="$(awk -F= -v name="$package" '$1==name {print substr($0,index($0,"=")+1)}' "$lock")"; fi
  if [[ "$installed" != installed ]]; then
    if [[ -n "$version" ]]; then missing+=("$package=$version"); else missing+=("$package"); fi
  elif [[ -n "$version" && "$(dpkg-query -W -f='${Version}' "$package")" != "$version" ]]; then
    echo "Error: $package differs from $lock. Restore the pinned container or deliberately update the system-package lock." >&2
    exit 1
  fi
done
if (( ${#missing[@]} )); then
  sudo env DEBIAN_FRONTEND=noninteractive apt-get -o DPkg::Lock::Timeout=300 update --error-on=any
  sudo env DEBIAN_FRONTEND=noninteractive apt-get -o DPkg::Lock::Timeout=300 install -y --no-install-recommends "${missing[@]}"
fi
temporary="$(mktemp code/.sandbox/r-system-packages.lock.XXXXXX)"
trap 'rm -f "$temporary"' EXIT
for package in "${packages[@]}"; do
  installed_version="$(dpkg-query -W -f='${Version}' "$package")"
  [[ -n "$installed_version" ]] || { echo "Error: no installed version found for $package." >&2; exit 1; }
  printf '%s=%s\n' "$package" "$installed_version" >> "$temporary"
done
mv "$temporary" "$lock"
