# Sourced only by interactive Bash in the allocation-local home.
_vscode_sandbox_compact_venv_prompt() {
 local previous_status=$?
 local wrapped_venv='^(\(\([^()]*\)) +\)(.*)$'
 if [[ -n "${VIRTUAL_ENV:-}" && "${PS1-}" =~ $wrapped_venv ]]; then
  PS1="${BASH_REMATCH[1]})${BASH_REMATCH[2]}"
 fi
 return "$previous_status"
}

# Keep existing prompt hooks and their exit-status behavior; do not register
# twice when a login shell reads both .profile and .bashrc.
if [[ "$(declare -p PROMPT_COMMAND 2>/dev/null)" == 'declare -a'* ]]; then
 _vscode_sandbox_has_prompt_hook=0
 for _vscode_sandbox_prompt_hook in "${PROMPT_COMMAND[@]}"; do
  if [[ "$_vscode_sandbox_prompt_hook" == _vscode_sandbox_compact_venv_prompt ]]; then _vscode_sandbox_has_prompt_hook=1;fi
 done
 if [[ "$_vscode_sandbox_has_prompt_hook" == 0 ]]; then PROMPT_COMMAND+=(_vscode_sandbox_compact_venv_prompt);fi
 unset _vscode_sandbox_has_prompt_hook _vscode_sandbox_prompt_hook
else
 case "${PROMPT_COMMAND-}" in
  *_vscode_sandbox_compact_venv_prompt*) ;;
  *)
   if [[ -n "${PROMPT_COMMAND-}" ]]; then PROMPT_COMMAND+=$'\n';fi
   PROMPT_COMMAND+=_vscode_sandbox_compact_venv_prompt
   ;;
 esac
fi
