#!/usr/bin/env bash
set -euo pipefail
umask 077
state="$1"; token="$2"
case "$state" in '~/'*) state="$HOME/${state:2}";; esac
mkdir -p "$state/uploads"
state="$(cd "$state" && pwd -P)"
stage="$state/uploads/$token"
mkdir "$stage"
printf 'STAGE=%s\n' "$stage"
