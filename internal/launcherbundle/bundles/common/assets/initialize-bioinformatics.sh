#!/usr/bin/env sh
set -eu
  if ! command -v podman >/dev/null 2>&1; then
    # apt update does not honor DPkg::Lock::Timeout for its lists lock.
    # Retry lock contention only; leave genuine package/network errors visible.
    apt_with_lock_retry() {
      attempt=0
      apt_output="$(mktemp)"
      while :; do
        if LC_ALL=C DEBIAN_FRONTEND=noninteractive apt-get "$@" >"$apt_output" 2>&1; then
          cat "$apt_output"
          rm -f "$apt_output"
          return 0
        else
          apt_status=$?
        fi
        cat "$apt_output" >&2
        attempt=$((attempt + 1))
        if ! grep -Eq "Could not get lock|Unable to acquire.*lock|Unable to lock directory" "$apt_output" || [ "$attempt" -ge 60 ]; then
          rm -f "$apt_output"
          return "$apt_status"
        fi
        echo "Waiting for the sandbox package manager (attempt $attempt/60)..." >&2
        sleep 5
      done
    }
    apt_with_lock_retry update --error-on=any
    apt_with_lock_retry install -y --no-install-recommends podman uidmap slirp4netns fuse-overlayfs
  fi
  mkdir -p /run/user/0/libpod/tmp
  chmod 700 /run/user/0
  XDG_RUNTIME_DIR=/run/user/0 podman --storage-driver=vfs --events-backend=file info >/dev/null
