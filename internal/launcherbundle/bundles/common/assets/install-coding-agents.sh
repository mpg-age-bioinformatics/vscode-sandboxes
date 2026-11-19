#!/usr/bin/env bash
# Install both agents in the current sandbox/container user's persistent home.
set -euo pipefail
export PATH="$HOME/.local/bin:$PATH"
mkdir -p "$HOME/.local/bin" "$HOME/.codex" "$HOME/.claude"
chmod 700 "$HOME/.codex" "$HOME/.claude"
for entry in 'codex @openai/codex 0.163.0' 'claude @anthropic-ai/claude-code 2.1.328'; do
  read -r binary package version <<< "$entry"
  agent_root="$HOME/.local/share/vscode-sandboxes/agents/$binary-$version"
  if [[ ! -x "$agent_root/node_modules/.bin/$binary" ]]; then
    npm install --prefix "$agent_root" --no-audit --no-fund "$package@$version"
  fi
  ln -sf "$agent_root/node_modules/.bin/$binary" "$HOME/.local/bin/$binary"
  "$HOME/.local/bin/$binary" --version
done
# Make both commands available in SSH terminals as well as container terminals.
for profile in "$HOME/.profile" "$HOME/.bashrc"; do
  line='export PATH="$HOME/.local/bin:$PATH"'
  if ! grep -Fqx "$line" "$profile" 2>/dev/null; then printf '\n%s\n' "$line" >> "$profile"; fi
done
