#!/usr/bin/env bash
set -euo pipefail
state="$1"; token="$2"
case "$state" in '~/'*) state="$HOME/${state:2}";; esac
run="$state/runs/$token"
if [[ ! -d "$run" ]]; then echo 'STATE=ENDED';exit 0;fi
[[ "$(cat "$run/owner-token")" == "$token" ]] || { echo 'Allocation identity does not match.' >&2; exit 1; }
run="$(cd "$run" && pwd -P)"
# Every SSH request has its own shell; restore the selected module environment.
# Older allocations used compute-only setup commands and keep their old behavior.
if [[ -r "$run/modules.sh" ]] && grep -Fqx '# VS Code Sandboxes login and compute modules' "$run/modules.sh"; then source "$run/modules.sh"; fi
if [[ ! -s "$run/job-id" ]]; then
  jobs="$(squeue --noheader --user "$(id -un)" --name=vscode-sandboxes --format='%i|%k')"
  found=''
  while IFS='|' read -r job comment; do
    if [[ "$comment" == "vscode-sandboxes:$token" ]]; then
      [[ -z "$found" && "$job" =~ ^[0-9]+$ ]] || { echo 'Ambiguous allocation identity.' >&2; exit 1; }
      found="$job"
    fi
  done <<< "$jobs"
  [[ -n "$found" ]] || { echo 'STATE=ENDED';exit 0; }
  printf '%s\n' "$found" > "$run/job-id"
fi
printf 'RUN=%s\nJOB=%s\n' "$run" "$(cat "$run/job-id")"
