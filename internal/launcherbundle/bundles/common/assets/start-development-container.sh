#!/usr/bin/env bash
# Runs exclusively inside the standard Docker-enabled sbx shell sandbox.
set -euo pipefail
agent="${1:-both}"
sandbox_name="${2:-}"
resume="${3:-0}"
case "$agent" in both|codex|claude) ;; *) echo 'Invalid coding agent.' >&2; exit 2 ;; esac
# Never inherit a context pointing to some other Docker engine.
unset DOCKER_CONTEXT
export DOCKER_HOST=unix:///var/run/docker.sock
command -v docker >/dev/null 2>&1 || { echo 'Error: the sandbox template does not include Docker. Recreate it with the standard shell-docker template.' >&2; exit 1; }
docker info --format 'Sandbox Docker Engine: {{.ServerVersion}}' || {
  echo 'Error: sandbox-local Docker Engine is unavailable. No host Docker socket will be used.' >&2; exit 1;
}
command -v node >/dev/null 2>&1 && command -v npm >/dev/null 2>&1 || {
  echo 'Error: Node.js and npm must be available in the standard shell-docker sandbox.' >&2; exit 1;
}
node -e 'if (Number(process.versions.node.split(".")[0]) < 20) process.exit(1)' || { echo 'Error: Node.js 20 or newer is required inside the sandbox.' >&2; exit 1; }
config_uri="$(node code/.sandbox/development-container.js config-uri "$sandbox_name")"
cli_version=0.90.0
cli_root="$HOME/.local/share/vscode-sandboxes/devcontainers-cli/$cli_version"
cli="$cli_root/node_modules/.bin/devcontainer"
if [[ ! -x "$cli" ]]; then
  echo "Installing Dev Containers CLI $cli_version inside the sandbox..."
  npm install --prefix "$cli_root" --no-audit --no-fund "@devcontainers/cli@$cli_version"
fi
# Credentials live in the sandbox user's home, not in the project or an image.
mkdir -p "$HOME/.codex" "$HOME/.claude" "$HOME/.continue" "$HOME/.local/share/vscode-sandboxes/certificates"
chmod 700 "$HOME/.codex" "$HOME/.claude" "$HOME/.continue"
cp /etc/ssl/certs/ca-certificates.crt "$HOME/.local/share/vscode-sandboxes/certificates/ca-certificates.crt"
bash code/.sandbox/install-coding-agents.sh
continue_bundle="$(python3 code/.sandbox/prepare-continue.py)"
node code/.sandbox/development-container.js prepare
# Use the same workspace/config labels as the VS Code extension, so it reuses
# this container instead of starting a second one when the editor connects.
container_args=(--workspace-folder "$PWD" --config "$PWD/code/.sandbox/devcontainer.json" --id-label "devcontainer.local_folder=$PWD" --id-label "devcontainer.config_file=$config_uri")
reuse_container=0
if [[ "$resume" == 1 ]] && [[ -n "$(docker ps -aq --filter "label=devcontainer.local_folder=$PWD" --filter "label=devcontainer.config_file=$config_uri")" ]]; then
  reuse_container=1
fi
"$cli" up "${container_args[@]}"
# Install the pinned remote extension before VS Code can activate it. This does
# not read or seed any host VS Code user state or Continue conversation history.
if ! "$cli" exec "${container_args[@]}" node code/.sandbox/install-continue.js check; then
  tar -C "$(dirname -- "$continue_bundle")" -cf - "$(basename -- "$continue_bundle")" | "$cli" exec "${container_args[@]}" node code/.sandbox/install-continue.js install
fi
# Existing development containers may predate the dedicated Continue bind mount.
# Stream the sandbox-only configuration to their home before extensions open.
if [[ -f "$HOME/.continue/.sandbox-managed" ]]; then
  "$cli" exec "${container_args[@]}" sh -c 'set -eu
umask 077
mkdir -p "$HOME/.continue"
cd "$HOME/.continue"
if [ ! -e .sandbox-managed ]; then
  if [ -e config.yaml ]; then cp -p config.yaml .sandbox-original-config.yaml; fi
  touch .sandbox-managed
fi
cat > .sandbox-config.tmp
mv .sandbox-config.tmp config.yaml' < "$HOME/.continue/config.yaml"
else
  "$cli" exec "${container_args[@]}" sh -c 'set -eu
if [ -e "$HOME/.continue/.sandbox-managed" ]; then
  cd "$HOME/.continue"
  if [ -e .sandbox-original-config.yaml ]; then mv .sandbox-original-config.yaml config.yaml; else rm -f config.yaml; fi
  rm -f .sandbox-managed
fi'
fi
if [[ "$reuse_container" == 1 ]]; then
  echo 'Reusing the initialized development container; checking its environment.'
else
  "$cli" exec "${container_args[@]}" bash code/.sandbox/initialize-development-container.sh
fi
"$cli" exec "${container_args[@]}" bash code/.sandbox/verify-development-container.sh
node code/.sandbox/development-container.js record
