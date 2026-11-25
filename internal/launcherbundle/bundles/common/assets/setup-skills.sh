#!/usr/bin/env bash
# Run in the project directory. Preserve an incomplete, originless repository.
# Shared workspaces may briefly report EEXIST before a just-created parent is
# visible. Never replace an existing file; retry only that narrow error class.
ensure_sandbox_directory() {
  local directory attempt error_file status
  for directory in "$@"; do
    [[ ! -d "$directory" ]] || continue
    if [[ -e "$directory" || -L "$directory" ]]; then
      echo "Error: expected a directory, but another entry exists: $directory" >&2
      return 1
    fi
    error_file="$(mktemp)" || return
    for attempt in 1 2 3; do
      if mkdir -p -- "$directory" 2>"$error_file"; then status=0;break;else status=$?;fi
      if [[ -d "$directory" ]]; then status=0;break;fi
      if [[ -e "$directory" || -L "$directory" ]] || ! LC_ALL=C grep -Eiq 'already exists|file exists|EEXIST' "$error_file" || [[ "$attempt" == 3 ]]; then break;fi
      sleep 0.1
    done
    if [[ "$status" != 0 ]]; then cat "$error_file" >&2;rm -f -- "$error_file";return "$status";fi
    rm -f -- "$error_file"
  done
}

setup_sandbox_skills() (
  set -euo pipefail
  local repository="$1" existing_origin status staging='' backup
  trap 'if [[ -n "$staging" ]]; then rm -rf -- "$staging"; fi' EXIT

  if [[ -e skills || -L skills ]]; then
    [[ ! -L skills ]] || { echo "Error: skills/ is a symbolic link. Move the link aside before retrying." >&2; exit 1; }
    [[ -e skills/.git ]] || { echo "Error: skills/ exists and is not a Git clone. Move it aside before retrying." >&2; exit 1; }
    # Do not hide Git errors (permissions, dubious ownership, corrupt config).
    git -C skills rev-parse --git-dir >/dev/null
    if existing_origin="$(git -C skills config --local --get remote.origin.url)"; then
      if [[ -n "$existing_origin" && "$existing_origin" != "$repository" ]]; then
        echo "Error: skills/ has a different origin. Expected $repository. Move skills/ aside before retrying." >&2
        exit 1
      fi
    else
      status=$?
      [[ "$status" == 1 ]] || exit "$status"
      existing_origin=''
    fi
    if [[ -n "$existing_origin" ]]; then
      git -C skills sparse-checkout set --no-cone /LICENSE /match-code-data/ /set-vscode-r-version/
      git -C skills checkout
      exit 0
    fi
  fi

  # Complete the replacement before touching the original directory.
  staging="$(mktemp -d .sandbox-skills.XXXXXX)"
  git -c core.autocrlf=false clone --no-checkout "$repository" "$staging/skills"
  git -C "$staging/skills" sparse-checkout set --no-cone /LICENSE /match-code-data/ /set-vscode-r-version/
  git -C "$staging/skills" checkout
  if [[ -e skills || -L skills ]]; then
    backup="$(mktemp -d skills.backup.XXXXXX)"
    mv -- skills "$backup/skills"
    if ! mv -- "$staging/skills" skills; then
      mv -- "$backup/skills" skills
      exit 1
    fi
    echo "Warning: skills/ had no origin. Preserved it in $backup/skills and installed the configured skills repository."
  else
    mv -- "$staging/skills" skills
  fi
)
