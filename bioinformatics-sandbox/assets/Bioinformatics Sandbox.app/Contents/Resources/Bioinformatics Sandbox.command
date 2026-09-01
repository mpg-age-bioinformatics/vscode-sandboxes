#!/usr/bin/env bash
set -uo pipefail

agent="${1:-}"
project_dir="${2:-}"
skills_repository="${BIOINFORMATICS_SANDBOX_SKILLS_REPOSITORY:-https://github.com/mpg-age-bioinformatics/skills.git}"
skills_ref="${BIOINFORMATICS_SANDBOX_SKILLS_REF:-}"
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
      echo "Bioinformatics Sandbox setup finished."
    else
      echo "Bioinformatics Sandbox setup stopped with an error (status $status)."
    fi
    read -r -p "Press Return to close this window..." _
  fi
  exit "$status"
}
trap finish EXIT

asset_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
launcher="$asset_dir/bioinformatics-sandox.sh"

if [[ ! -x "$launcher" ]]; then
  command -v git >/dev/null 2>&1 || {
    echo "Error: Git is required to download the Bioinformatics Sandbox setup files." >&2
    exit 1
  }
  download_root="$(mktemp -d "${temporary_root%/}/bioinformatics-sandbox-command.XXXXXX")"
  downloaded_skills="$download_root/skills"
  echo "Downloading Bioinformatics Sandbox setup files..."
  git clone --quiet --depth 1 "$skills_repository" "$downloaded_skills"
  if [[ -n "$skills_ref" ]]; then
    git -C "$downloaded_skills" fetch --quiet --depth 1 origin "$skills_ref"
    git -C "$downloaded_skills" checkout --quiet --detach FETCH_HEAD
  fi
  launcher="$downloaded_skills/bioinformatics-sandbox/assets/bioinformatics-sandox.sh"
fi

[[ -x "$launcher" ]] || {
  echo "Error: downloaded Bioinformatics Sandbox launcher is unavailable: $launcher" >&2
  exit 1
}

if [[ -z "$agent" ]]; then
  read -r -p "Agent (codex or claude): " agent
fi
if [[ -z "$project_dir" ]]; then
  default_project="$HOME/Desktop/bioinformatics-sandbox-project"
  read -r -p "Project directory [$default_project]: " project_dir
  project_dir="${project_dir:-$default_project}"
  case "$project_dir" in
    "~") project_dir="$HOME" ;;
    "~/"*) project_dir="$HOME/${project_dir#\~/}" ;;
  esac
  if [[ "$project_dir" != /* ]]; then
    project_dir="$(pwd -P)/$project_dir"
  fi
  if [[ ! -e "$project_dir" ]]; then
    echo "Creating project directory: $project_dir"
    mkdir -p "$project_dir"
  fi
fi

"$launcher" "$agent" "$project_dir"
