#!/usr/bin/env bash
set -uo pipefail

skills_repository="${PYTHON_SANDBOX_SKILLS_REPOSITORY:-https://github.com/mpg-age-bioinformatics/skills.git}"
skills_ref="${PYTHON_SANDBOX_SKILLS_REF:-}"
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
      echo "Python Sandbox setup finished."
    else
      echo "Python Sandbox setup stopped with an error (status $status)."
    fi
    read -r -p "Press Return to close this window..." _
  fi
  exit "$status"
}
trap finish EXIT

command -v git >/dev/null 2>&1 || {
  echo "Error: Git is required to download the Python Sandbox setup files." >&2
  exit 1
}
download_root="$(mktemp -d "${temporary_root%/}/python-sandbox-app.XXXXXX")"
downloaded_skills="$download_root/skills"
echo "Downloading Python Sandbox setup files..."
git clone --quiet --depth 1 "$skills_repository" "$downloaded_skills"
if [[ -n "$skills_ref" ]]; then
  git -C "$downloaded_skills" fetch --quiet --depth 1 origin "$skills_ref"
  git -C "$downloaded_skills" checkout --quiet --detach FETCH_HEAD
fi

launcher="$downloaded_skills/python-sandbox/assets/Python Sandbox.command"
[[ -x "$launcher" ]] || {
  echo "Error: downloaded Python Sandbox launcher is unavailable: $launcher" >&2
  exit 1
}
"$launcher" "$@"
