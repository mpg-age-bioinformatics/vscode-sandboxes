#!/usr/bin/env bash
# Host-side dispatcher: sbx and VS Code only. Docker runs inside the sandbox.
set -euo pipefail
language="${1:-}"
agent="${2:-both}"
case "$language:$agent" in python:both|python:codex|python:claude|r:both|r:codex|r:claude) ;; *) echo 'Usage: run-language-sandbox.sh <python|r> [both]' >&2; exit 2 ;; esac
agent=both
project_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
windows_git_bash=0
case "${OSTYPE:-}:${MSYSTEM:-}" in msys*:MINGW*|cygwin*:MINGW*) windows_git_bash=1 ;; esac
host_path() { if (( windows_git_bash )); then cygpath -w "$1"; else printf '%s\n' "$1"; fi; }
native_exec() { if (( windows_git_bash )); then MSYS2_ARG_CONV_EXCL='*' "$@"; else "$@"; fi; }
sbx_daemon_is_running() {
  local daemon_status
  daemon_status="$(native_exec sbx daemon status --json 2>/dev/null)" || return 1
  grep -Eq '"status"[[:space:]]*:[[:space:]]*"running"' <<< "$daemon_status"
}
ensure_sbx_daemon() {
  [[ "$(uname -s 2>/dev/null || true)" == Darwin ]] || return 0
  sbx_daemon_is_running && return 0
  echo 'Starting the Docker Sandboxes daemon in the background...'
  native_exec sbx daemon start --detach || return 1
  local attempt
  for attempt in {1..15}; do sbx_daemon_is_running && return 0; sleep 1; done
  echo 'Error: the Docker Sandboxes daemon did not become ready within 15 seconds.' >&2
  return 1
}
command -v sbx >/dev/null 2>&1 || { echo 'Error: install Docker Sandboxes (sbx).' >&2; exit 1; }
sbx_version="$(native_exec sbx version 2>&1)" || { echo "Error: sbx version failed: $sbx_version" >&2; exit 1; }
if [[ ! "$sbx_version" =~ (Client[[:space:]]Version:|sbx[[:space:]]version:)[[:space:]]+v?([0-9]+)\.([0-9]+)\.([0-9]+) ]] ||
   (( 10#${BASH_REMATCH[2]} == 0 && 10#${BASH_REMATCH[3]} < 39 )); then
  echo "Error: Docker Sandboxes 0.39.0 or newer is required. Detected: $sbx_version" >&2; exit 1
fi
ensure_sbx_daemon
native_exec sbx setup ssh
if (( ! windows_git_bash )); then native_exec sbx diagnose; fi
project_name="$(basename -- "$project_root" | tr '[:upper:]_' '[:lower:]-' | tr -cd 'a-z0-9.-' | sed -E 's/^[.-]+//; s/[.-]+$//' | cut -c 1-32)"
project_name="${project_name:-project}"
workspace_key="$project_root"
if (( windows_git_bash )); then workspace_key="$(cygpath -m "$project_root" | tr '[:upper:]' '[:lower:]')"; fi
if workspace_stat="$(stat -f '%d:%i' "$project_root" 2>/dev/null)"; then :
elif workspace_stat="$(stat -c '%d:%i' "$project_root" 2>/dev/null)"; then :
else echo 'Error: cannot determine workspace identity.' >&2; exit 1; fi
workspace_id="$(printf '%s:%s' "$workspace_key" "$workspace_stat" | cksum | awk '{print $1}')"
# Use new names so older custom-template sandboxes are never silently reused.
sandbox_name="${language}-${project_name}-${workspace_id}-dev"
if [[ -n "${VSCODE_SANDBOX_REUSE_NAME:-}" ]]; then sandbox_name="$VSCODE_SANDBOX_REUSE_NAME"; fi
echo "Sandbox: $sandbox_name"
echo "Workspace: $project_root"
echo 'Architecture: general sandbox → sandbox-local Docker → development container'
startup_args=()
if [[ -n "${VSCODE_SANDBOX_STARTUP_HELPER:-}" ]]; then startup_args=(--deny-network '**'); fi
resume=0
if native_exec sbx ls --quiet | grep -Fqx "$sandbox_name"; then
  resume="${VSCODE_SANDBOX_RESUME:-0}"
  if [[ -n "${VSCODE_SANDBOX_STARTUP_HELPER:-}" ]]; then
    native_exec sbx stop "$sandbox_name"
    native_exec "$VSCODE_SANDBOX_STARTUP_HELPER" --startup-guard "$sandbox_name" "$agent"
  fi
  native_exec sbx run --detached --name "$sandbox_name"
else
  # The shell agent's standard shell-docker template has its own Docker Engine.
  native_exec sbx run --detached ${startup_args[@]+"${startup_args[@]}"} --name "$sandbox_name" shell "$(host_path "$project_root")"
fi
if [[ -n "${VSCODE_SANDBOX_STARTUP_HELPER:-}" ]]; then
  native_exec "$VSCODE_SANDBOX_STARTUP_HELPER" --startup-apply "$sandbox_name" "$agent"
fi
sandbox_project_root="$(native_exec sbx exec "$sandbox_name" sh -c 'pwd -P' | tail -n 1)"
case "$sandbox_project_root" in /*) ;; *) echo 'Error: cannot determine sandbox workspace path.' >&2; exit 1 ;; esac
native_exec sbx exec --workdir "$sandbox_project_root" "$sandbox_name" bash code/.sandbox/start-development-container.sh "$agent" "$sandbox_name" "$resume"

if [[ -n "${VSCODE_SANDBOX_STARTUP_HELPER:-}" ]]; then
  native_exec "$VSCODE_SANDBOX_STARTUP_HELPER" --startup-ready "$sandbox_name" "$agent"
fi

if [[ "$language" == python && "${PYTHON_SANDBOX_SKIP_VSCODE:-0}" == 1 ]] || [[ "$language" == r && "${R_SANDBOX_SKIP_VSCODE:-0}" == 1 ]]; then
  echo 'Skipped VS Code startup; the development container is running.'
  exit 0
fi
command -v ssh >/dev/null 2>&1 || { echo 'Error: OpenSSH is required for VS Code.' >&2; exit 1; }
code_cli="$(command -v code || true)"
if [[ -z "$code_cli" ]]; then
  for candidate in '/Applications/Visual Studio Code.app/Contents/Resources/app/bin/code' "$HOME/Applications/Visual Studio Code.app/Contents/Resources/app/bin/code" "$HOME/Desktop/Visual Studio Code.app/Contents/Resources/app/bin/code"; do
    if [[ -x "$candidate" ]]; then code_cli="$candidate"; break; fi
  done
fi
[[ -n "$code_cli" ]] || { echo 'Error: install Visual Studio Code and enable its code command.' >&2; exit 1; }
native_exec "$code_cli" --install-extension ms-vscode-remote.remote-ssh
native_exec "$code_cli" --install-extension ms-vscode-remote.remote-containers
# Encode on the sandbox's Node runtime; no Node or Docker CLI is needed on host.
folder_uri="$(native_exec sbx exec --workdir "$sandbox_project_root" "$sandbox_name" node code/.sandbox/development-container.js uri "$sandbox_name" | tail -n 1)"
case "$folder_uri" in vscode-remote://dev-container+*) ;; *) echo 'Error: invalid Dev Containers connection URI.' >&2; exit 1 ;; esac
echo "VS Code target: $language development container inside $sandbox_name"
native_exec "$code_cli" --folder-uri "$folder_uri"
