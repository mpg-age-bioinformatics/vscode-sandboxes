#!/usr/bin/env python3
"""Run the actual sandbox apt helper against transient and permanent failures."""
import os
from pathlib import Path
import subprocess
import tempfile

root = Path(__file__).resolve().parents[1]
helpers = []
for language in ('python', 'r', 'bioinformatics'):
    source = (root / f'internal/launcherbundle/bundles/{language}-sandbox/assets/run-{language}-sandbox.sh').read_text()
    helpers.append(source.split('    apt_with_lock_retry() {', 1)[1].split('\n  fi', 1)[0])
assert helpers[0] == helpers[1] == helpers[2], 'Sandbox apt helpers differ'
script = 'set -e\napt_with_lock_retry() {' + helpers[0]

with tempfile.TemporaryDirectory() as directory:
    work = Path(directory)
    apt = work / 'apt-get'
    apt.write_text('''#!/bin/sh
printf '%s\\n' "$1" >> "$APT_TEST_CALLS"
case "$APT_TEST_MODE" in
  transient)
    if [ ! -f "$APT_TEST_STATE" ]; then
      touch "$APT_TEST_STATE"
      echo 'E: Could not get lock /var/lib/apt/lists/lock. It is held by process 116 (apt-get)' >&2
      exit 100
    fi ;;
  permanent) echo 'E: Unable to locate package podman' >&2; exit 100 ;;
  locked) echo 'E: Unable to acquire the dpkg frontend lock' >&2; exit 100 ;;
esac
''')
    sleep = work / 'sleep'
    sleep.write_text('#!/bin/sh\nprintf "wait\\n" >> "$APT_TEST_WAITS"\n')
    apt.chmod(0o755)
    sleep.chmod(0o755)
    for mode, expected_status, expected_calls, expected_waits in (
        ('transient', 0, 3, 1), ('permanent', 100, 1, 0), ('locked', 100, 60, 59)
    ):
        calls, waits, state = (work / f'{mode}.{suffix}' for suffix in ('calls', 'waits', 'state'))
        env = dict(os.environ, PATH=f'{work}:{os.environ["PATH"]}', APT_TEST_MODE=mode,
                   APT_TEST_CALLS=str(calls), APT_TEST_WAITS=str(waits), APT_TEST_STATE=str(state))
        result = subprocess.run(['sh', '-c', script], env=env, capture_output=True, text=True)
        assert result.returncode == expected_status, result.stderr
        assert len(calls.read_text().splitlines()) == expected_calls
        assert (len(waits.read_text().splitlines()) if waits.exists() else 0) == expected_waits
print('APT lock retry tests passed.')
