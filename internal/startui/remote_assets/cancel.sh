#!/usr/bin/env bash
set -euo pipefail
run="$1"; token="$2"; job="$3"
[[ "$job" =~ ^[0-9]+$ && "$(cat "$run/owner-token")" == "$token" && "$(cat "$run/job-id")" == "$job" ]] || { echo 'Refusing to cancel an allocation with a mismatched identity.' >&2; exit 1; }
# Scheduler IDs may eventually be reused. Check the live job's unique comment,
# rather than trusting an old job-id file when cancelling a saved allocation.
# Every SSH request has its own shell; restore the selected module environment.
# Older allocations used compute-only setup commands and keep their old behavior.
if [[ -r "$run/modules.sh" ]] && grep -Fqx '# VS Code Sandboxes login and compute modules' "$run/modules.sh"; then source "$run/modules.sh"; fi
identity="$(squeue --noheader --jobs "$job" --format='%i|%k')"
[[ -n "$identity" ]] || { echo 'Allocation already ended.'; exit 0; }
[[ "$identity" == "$job|vscode-sandboxes:$token" ]] || { echo 'Refusing to cancel a different live allocation.' >&2; exit 1; }
scancel "$job"
