#!/usr/bin/env bash
set -uo pipefail

r_version="${1:-}"
agent="${2:-}"
project_dir="${3:-}"

finish() {
  status=$?
  trap - EXIT
  if [[ -t 0 ]]; then
    echo
    if [[ $status -eq 0 ]]; then
      echo "R Sandbox setup finished."
    else
      echo "R Sandbox setup stopped with an error (status $status)."
    fi
    read -r -p "Press Return to close this window..." _
  fi
  exit "$status"
}
trap finish EXIT

require_command() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "Error: $1 is required. $2" >&2
    exit 1
  }
}

if ! command -v code >/dev/null 2>&1; then
  for code_bin in \
    "/Applications/Visual Studio Code.app/Contents/Resources/app/bin" \
    "$HOME/Desktop/Visual Studio Code.app/Contents/Resources/app/bin"; do
    if [[ -x "$code_bin/code" ]]; then
      export PATH="$code_bin:$PATH"
      break
    fi
  done
fi

require_command git "Install Git from https://git-scm.com/download/mac and reopen the app."
require_command ssh "Install or restore the macOS OpenSSH client and reopen the app."
require_command code "Install Visual Studio Code from https://code.visualstudio.com/docs/setup/mac and enable its shell command."
require_command sbx "Install Docker Sandboxes from https://docs.docker.com/ai/sandboxes/install/."
require_command docker "Install Docker Desktop (or another supported Docker engine) and start it."

asset_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
setup_dir="$asset_dir/Setup"
if [[ ! -x "$setup_dir/scripts/setup-project.sh" ]]; then
  repository_root="$(CDPATH= cd -- "$asset_dir/../../../../.." 2>/dev/null && pwd)" || repository_root=""
  source_setup_dir="$repository_root/internal/launcherbundle/bundles/r-sandbox"
  if [[ -x "$source_setup_dir/scripts/setup-project.sh" ]]; then
    setup_dir="$source_setup_dir"
  fi
fi
[[ -x "$setup_dir/scripts/setup-project.sh" ]] || {
  echo "Error: embedded R Sandbox setup is unavailable: $setup_dir/scripts/setup-project.sh" >&2
  exit 1
}

sbx_daemon_is_running() {
  local daemon_status
  daemon_status="$(sbx daemon status --json 2>/dev/null)" || return 1
  grep -Eq '"status"[[:space:]]*:[[:space:]]*"running"' <<< "$daemon_status"
}

ensure_sbx_daemon() {
  sbx_daemon_is_running && return 0
  echo "Starting the Docker Sandboxes daemon in the background..."
  sbx daemon start --detach || {
    echo "Error: could not start the Docker Sandboxes daemon." >&2
    return 1
  }
  local attempt
  for attempt in {1..15}; do
    sbx_daemon_is_running && return 0
    sleep 1
  done
  echo "Error: the Docker Sandboxes daemon did not become ready within 15 seconds." >&2
  return 1
}

sbx_version="$(sbx version 2>&1)" || {
  echo "Error: Docker Sandboxes is installed but 'sbx version' failed: $sbx_version" >&2
  exit 1
}
if [[ ! "$sbx_version" =~ (Client[[:space:]]Version:|sbx[[:space:]]version:)[[:space:]]v?([0-9]+)\.([0-9]+)\.([0-9]+) ]] ||
   (( 10#${BASH_REMATCH[2]} == 0 && 10#${BASH_REMATCH[3]} < 39 )); then
  echo "Error: Docker Sandboxes 0.39.0 or newer is required. Detected: $sbx_version" >&2
  exit 1
fi
ensure_sbx_daemon || exit 1
if ! sbx diagnose; then
  echo "Error: Docker Sandboxes diagnostics failed. Confirm virtualization is available and run 'sbx login'." >&2
  exit 1
fi
if ! docker info >/dev/null 2>&1; then
  echo "Error: the Docker daemon is not available. Start Docker Desktop (or your Docker engine) and retry." >&2
  exit 1
fi

setup_script="$setup_dir/scripts/setup-project.sh"

if [[ -z "$r_version" ]]; then
  read -r -p "R version (major.minor or major.minor.patch): " r_version
fi
if [[ -z "$agent" ]]; then
  read -r -p "Agent (codex or claude): " agent
fi
if [[ -z "$project_dir" ]]; then
  default_project="$HOME/Desktop/r-sandbox-project"
  read -r -p "Project directory [$default_project]: " project_dir
  project_dir="${project_dir:-$default_project}"
fi
case "$project_dir" in
  "~") project_dir="$HOME" ;;
  "~/"*) project_dir="$HOME/${project_dir#\~/}" ;;
esac
if [[ "$project_dir" != /* ]]; then
  project_dir="$(pwd -P)/$project_dir"
fi
if [[ ! -e "$project_dir" ]]; then
  echo "Creating project directory: $project_dir"
  mkdir -p "$project_dir" || {
    echo "Error: could not create project directory: $project_dir" >&2
    exit 1
  }
fi
[[ -d "$project_dir" ]] || {
  echo "Error: project path exists but is not a directory: $project_dir" >&2
  exit 1
}

"$setup_script" "$project_dir" "$r_version" "$agent" || exit 1
"$project_dir/code/run-r-sandbox.sh" "$agent"
