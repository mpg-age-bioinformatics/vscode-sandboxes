#!/usr/bin/env bash
set -uo pipefail

r_version="${1:-}"
agent="${2:-}"
project_dir="${3:-}"
skills_repository="${R_SANDBOX_SKILLS_REPOSITORY:-https://github.com/mpg-age-bioinformatics/skills.git}"
skills_ref="${R_SANDBOX_SKILLS_REF:-}"
temporary_root="${TMPDIR:-/tmp}"
download_root=""

finish() {
  status=$?
  trap - EXIT
  if [[ -n "$download_root" && -d "$download_root" ]]; then
    rm -rf -- "$download_root"
  fi
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

if ! command -v code >/dev/null 2>&1 && [[ -x "/Applications/Visual Studio Code.app/Contents/Resources/app/bin/code" ]]; then
  PATH="/Applications/Visual Studio Code.app/Contents/Resources/app/bin:$PATH"
  export PATH
fi

require_command git "Install Git from https://git-scm.com/download/mac and reopen the app."
require_command ssh "Install or restore the macOS OpenSSH client and reopen the app."
require_command code "Install Visual Studio Code from https://code.visualstudio.com/docs/setup/mac and enable its shell command."
require_command sbx "Install Docker Sandboxes from https://docs.docker.com/ai/sandboxes/install/."
require_command docker "Install Docker Desktop (or another supported Docker engine) and start it."

sbx_version="$(sbx version 2>&1)" || {
  echo "Error: Docker Sandboxes is installed but 'sbx version' failed: $sbx_version" >&2
  exit 1
}
if [[ ! "$sbx_version" =~ (Client[[:space:]]Version:|sbx[[:space:]]version:)[[:space:]]v?([0-9]+)\.([0-9]+)\.([0-9]+) ]] ||
   (( 10#${BASH_REMATCH[2]} == 0 && 10#${BASH_REMATCH[3]} < 39 )); then
  echo "Error: Docker Sandboxes 0.39.0 or newer is required. Detected: $sbx_version" >&2
  exit 1
fi
if ! sbx diagnose; then
  echo "Error: Docker Sandboxes diagnostics failed. Confirm virtualization is available and run 'sbx login'." >&2
  exit 1
fi
if ! docker info >/dev/null 2>&1; then
  echo "Error: the Docker daemon is not available. Start Docker Desktop (or your Docker engine) and retry." >&2
  exit 1
fi

asset_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
launcher="$asset_dir/r-sandox.sh"

if [[ ! -x "$launcher" ]]; then
  download_root="$(mktemp -d "${temporary_root%/}/r-sandbox-command.XXXXXX")"
  downloaded_skills="$download_root/skills"
  echo "Downloading R Sandbox setup files..."
  git clone --quiet --depth 1 "$skills_repository" "$downloaded_skills"
  if [[ -n "$skills_ref" ]]; then
    git -C "$downloaded_skills" fetch --quiet --depth 1 origin "$skills_ref"
    git -C "$downloaded_skills" checkout --quiet --detach FETCH_HEAD
  fi
  launcher="$downloaded_skills/r-sandbox/assets/r-sandox.sh"
fi

[[ -x "$launcher" ]] || {
  echo "Error: downloaded R Sandbox launcher is unavailable: $launcher" >&2
  exit 1
}

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

"$launcher" "$r_version" "$agent" "$project_dir"
