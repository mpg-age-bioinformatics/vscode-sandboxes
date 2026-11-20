#!/usr/bin/env bash
set -euo pipefail
run="$1"; token="$2"; job="$3"; offset="${4:-0}"
[[ "$(cat "$run/owner-token")" == "$token" && "$(cat "$run/job-id")" == "$job" ]] || { echo 'Remote allocation identity does not match.' >&2; exit 1; }
# Every SSH request has its own shell; restore the selected module environment.
# Older allocations used compute-only setup commands and keep their old behavior.
if [[ -r "$run/modules.sh" ]] && grep -Fqx '# VS Code Sandboxes login and compute modules' "$run/modules.sh"; then source "$run/modules.sh"; fi
# An ended job may make `squeue --jobs` fail instead of returning an empty list.
# Query our user and select the exact ID so startup failures still include logs.
all_jobs="$(squeue --noheader --user "$(id -u)" --format='%i|%T|%N|%R|%k')"
queue="$(while IFS= read -r row; do [[ "${row%%|*}" != "$job" ]] || printf '%s\n' "$row"; done <<< "$all_jobs")"
if [[ -z "$queue" ]]; then
  echo 'STATE=ENDED'
  tail -c 12000 "$run/slurm-$job.log" 2>/dev/null || true
  exit 0
fi
[[ "${queue##*|}" == "vscode-sandboxes:$token" ]] || { echo 'STATE=ENDED'; echo 'The saved Slurm job ID now belongs to another allocation.'; exit 0; }
# Reuse/restart the same server monitor during normal app interaction. This
# registration does not renew the sandbox's authenticated-connection clock.
if [[ -f "$run/watchdog.py" ]]; then python3 "$run/watchdog.py" register "$run" "$token" "$job" >/dev/null; fi
printf 'QUEUE=%s\n' "$queue"
# Return bounded incremental output without interpreting log lines as protocol fields.
python3 - "$run" "$job" "$offset" <<'PYLOG'
import base64,pathlib,sys
run=pathlib.Path(sys.argv[1])
try:
    with (run/('slurm-'+sys.argv[2]+'.log')).open('rb') as stream:
        offset=max(0,int(sys.argv[3]))
        if offset>stream.seek(0,2): offset=0
        stream.seek(offset)
        data=stream.read(32768)
        print('LOG_OFFSET='+str(stream.tell()))
        print('LOG64='+base64.b64encode(data).decode())
except FileNotFoundError: pass
try:
    phase=(run/'startup-phase').read_text()[:300]
    print('STARTUP_PHASE='+phase.replace('\n',' ').replace('\r',' ').strip())
except FileNotFoundError: pass
PYLOG
if [[ -s "$run/ready" ]]; then
  printf 'STATE=READY\n'
  cat "$run/ready"

else
  echo 'STATE=WAITING'
fi
