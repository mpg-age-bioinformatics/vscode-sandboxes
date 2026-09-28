#!/usr/bin/env bash
set -euo pipefail

agent="${1:-}"
case "$agent" in
  codex) agent_template="codex" ;;
  claude) agent_template="claude-code" ;;
  *) echo "Usage: $0 <codex|claude>" >&2; exit 2 ;;
esac

project_root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
windows_git_bash=0
case "${OSTYPE:-}:${MSYSTEM:-}" in
  msys*:MINGW* | cygwin*:MINGW*) windows_git_bash=1 ;;
esac

host_path() {
  if (( windows_git_bash )); then
    cygpath -w "$1"
  else
    printf '%s\n' "$1"
  fi
}

native_exec() {
  if (( windows_git_bash )); then
    MSYS2_ARG_CONV_EXCL='*' "$@"
  else
    "$@"
  fi
}

sbx_daemon_is_running() {
  local daemon_status
  daemon_status="$(native_exec sbx daemon status --json 2>/dev/null)" || return 1
  grep -Eq '"status"[[:space:]]*:[[:space:]]*"running"' <<< "$daemon_status"
}

ensure_sbx_daemon() {
  [[ "$(uname -s 2>/dev/null || true)" == "Darwin" ]] || return 0
  sbx_daemon_is_running && return 0
  echo "Starting the Docker Sandboxes daemon in the background..."
  native_exec sbx daemon start --detach || {
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

project_name="$(basename -- "$project_root" | tr '[:upper:]_' '[:lower:]-' | tr -cd 'a-z0-9.-' | sed -E 's/^[.-]+//; s/[.-]+$//' | cut -c 1-32)"
project_name="${project_name:-project}"
workspace_key="$project_root"
if (( windows_git_bash )); then
  workspace_key="$(cygpath -m "$project_root" | tr '[:upper:]' '[:lower:]')"
fi
if workspace_stat="$(stat -f '%d:%i' "$project_root" 2>/dev/null)"; then
  :
elif workspace_stat="$(stat -c '%d:%i' "$project_root" 2>/dev/null)"; then
  :
else
  echo "Error: could not determine the project directory identity: $project_root" >&2
  exit 1
fi
workspace_id="$(printf '%s:%s' "$workspace_key" "$workspace_stat" | cksum | awk '{print $1}')"
command -v docker >/dev/null 2>&1 || { echo "Error: Docker CLI is required to build the sandbox template." >&2; exit 1; }
command -v sbx >/dev/null 2>&1 || { echo "Error: Docker Sandboxes CLI (sbx) is not installed." >&2; exit 1; }
if [[ "${PYTHON_SANDBOX_SKIP_VSCODE:-0}" != "1" ]]; then
  command -v ssh >/dev/null 2>&1 || { echo "Error: an OpenSSH client is required for VS Code Remote-SSH." >&2; exit 1; }
fi
sbx_version="$(native_exec sbx version 2>&1)" || { echo "Error: 'sbx version' failed: $sbx_version" >&2; exit 1; }
if [[ ! "$sbx_version" =~ (Client[[:space:]]Version:|sbx[[:space:]]version:)[[:space:]]v?([0-9]+)\.([0-9]+)\.([0-9]+) ]] ||
   (( 10#${BASH_REMATCH[2]} == 0 && 10#${BASH_REMATCH[3]} < 39 )); then
  echo "Error: Docker Sandboxes 0.39.0 or newer is required. Detected: $sbx_version" >&2
  exit 1
fi
ensure_sbx_daemon
native_exec sbx setup ssh
if (( ! windows_git_bash )); then
  native_exec sbx diagnose || { echo "Error: Docker Sandboxes diagnostics failed after SSH setup. Confirm virtualization and authentication." >&2; exit 1; }
fi
docker_os="$(native_exec docker info --format '{{.OSType}}' 2>/dev/null)" || { echo "Error: the Docker daemon is unavailable. Start Docker Desktop and retry." >&2; exit 1; }
[[ "$(printf '%s' "$docker_os" | tr '[:upper:]' '[:lower:]')" == "linux" ]] || { echo "Error: Docker must be running Linux containers; detected: $docker_os" >&2; exit 1; }

sandbox_name="py-${project_name}-${workspace_id}-${agent}"
template_tag="python-sandbox-${project_name}-${workspace_id}-${agent}:local"
temporary_root="${TMPDIR:-/tmp}"
template_tar="$(mktemp "${temporary_root%/}/python-sandbox-template.XXXXXX")"
trap 'rm -f "$template_tar"' EXIT

native_exec docker build --build-arg "AGENT_TEMPLATE=$agent_template" -t "$template_tag" -f "$(host_path "$project_root/code/Dockerfile")" "$(host_path "$project_root/code")"
native_exec docker image save "$template_tag" -o "$(host_path "$template_tar")"
native_exec sbx template load "$(host_path "$template_tar")"

echo "Sandbox: $sandbox_name"
echo "VS Code Remote-SSH host: ${sandbox_name}.sbx"
echo "Workspace: $project_root"

if native_exec sbx ls --quiet | grep -Fqx "$sandbox_name"; then
  native_exec sbx run --detached --name "$sandbox_name"
else
  native_exec sbx run --detached --name "$sandbox_name" --template "$template_tag" "$agent" "$(host_path "$project_root")"
fi

python_series="$(sed -n 's/^ARG PYTHON_IMAGE=python:\([0-9][0-9]*\.[0-9][0-9]*\)-bookworm$/\1/p' "$project_root/code/Dockerfile")"
[[ -n "$python_series" ]] || { echo "Error: could not determine the Python series from code/Dockerfile." >&2; exit 1; }
sandbox_project_root="$(native_exec sbx exec "$sandbox_name" sh -c 'pwd -P')"
case "$sandbox_project_root" in
  /*) ;;
  *) echo "Error: could not determine the project path inside the sandbox: $sandbox_project_root" >&2; exit 1 ;;
esac
native_exec sbx exec --env "EXPECTED_PYTHON_SERIES=$python_series" --workdir "$sandbox_project_root" "$sandbox_name" sh -c \
  'set -e
   if [ -e .venv/Scripts/python.exe ] && [ ! -x .venv/bin/python ]; then
     echo "Error: .venv was created for Windows and cannot be used inside the Linux sandbox. Rename or remove .venv, then rerun the launcher." >&2
     exit 1
   fi
   # Project directories are bind-mounted from the host. On Windows, Linux
   # symlinks created by venv on NTFS can appear executable but fail with
   # ENOENT, so install real interpreter copies on every platform.
   venv_created=0
   if ! .venv/bin/python -c "" >/dev/null 2>&1; then
     python -m venv --clear --copies --without-pip .venv
     venv_created=1
   fi
   venv_site_packages="$(.venv/bin/python -c "import sysconfig; print(sysconfig.get_path(\"purelib\"))")"
   cp code/windows-venv-sitecustomize.py "$venv_site_packages/sitecustomize.py"
   if [ "$venv_created" = 1 ]; then
     if ! ensurepip_output="$(.venv/bin/python -m ensurepip --upgrade --default-pip 2>&1)"; then
       if ! .venv/bin/python -m pip --version >/dev/null 2>&1; then
         printf "%s\n" "$ensurepip_output" >&2
         exit 1
       fi
       echo "Warning: ensurepip reported a Windows mount permission limitation after pip was installed; continuing with verified pip."
     else
       printf "%s\n" "$ensurepip_output"
     fi
   fi
   test -x .venv/bin/python
   .venv/bin/python -c "import os, sys; expected = tuple(map(int, os.environ[\"EXPECTED_PYTHON_SERIES\"].split(\".\"))); assert sys.version_info[:2] == expected, (sys.version, expected); print(sys.executable); print(sys.version)"'

if [[ "${PYTHON_SANDBOX_SKIP_VSCODE:-0}" == "1" ]]; then
  echo "Skipped VS Code setup because PYTHON_SANDBOX_SKIP_VSCODE=1."
  exit 0
fi

if command -v code >/dev/null 2>&1; then
  code_cli="$(command -v code)"
else
  code_cli=""
  for candidate in \
    "/Applications/Visual Studio Code.app/Contents/Resources/app/bin/code" \
    "$HOME/Applications/Visual Studio Code.app/Contents/Resources/app/bin/code" \
    "/Applications/Visual Studio Code - Insiders.app/Contents/Resources/app/bin/code" \
    "$HOME/Desktop/Visual Studio Code.app/Contents/Resources/app/bin/code"; do
    if [[ -x "$candidate" ]]; then
      code_cli="$candidate"
      break
    fi
  done
  [[ -n "$code_cli" ]] || {
    echo "Error: neither the code CLI nor a Visual Studio Code application bundle was found." >&2
    exit 1
  }

  if [[ ! -e /usr/local/bin/code && -w /usr/local/bin ]]; then
    ln -s "$code_cli" /usr/local/bin/code
    code_cli="/usr/local/bin/code"
    echo "Enabled the code CLI at /usr/local/bin/code."
  else
    echo "Using Visual Studio Code's bundled CLI at $code_cli."
  fi
fi

native_exec "$code_cli" --install-extension ms-vscode-remote.remote-ssh
remote_authority="ssh-remote+${sandbox_name}.sbx"
required_extensions=(ms-python.python ms-python.vscode-pylance ms-toolsai.jupyter charliermarsh.ruff REditorSupport.r openai.chatgpt anthropic.claude-code)
for extension in "${required_extensions[@]}"; do
  native_exec "$code_cli" --remote "$remote_authority" --install-extension "$extension"
done
remote_extensions="$(native_exec "$code_cli" --remote "$remote_authority" --list-extensions | tr '[:upper:]' '[:lower:]')"
for extension in "${required_extensions[@]}"; do
  if ! printf '%s\n' "$remote_extensions" | grep -Fxiq "$(printf '%s' "$extension" | tr '[:upper:]' '[:lower:]')"; then
    echo "Error: VS Code extension was not installed in $remote_authority: $extension" >&2
    exit 1
  fi
done
native_exec "$code_cli" --remote "$remote_authority" "$sandbox_project_root"
