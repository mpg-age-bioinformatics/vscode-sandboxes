#!/usr/bin/env bash
# Exercise both host launchers with forbidden host Docker/Podman commands.
set -euo pipefail
repository_root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
test_root="$(mktemp -d "${TMPDIR:-/tmp}/vscode-development-containers.XXXXXX")"
trap 'rm -rf -- "$test_root"' EXIT
mkdir -p "$test_root/bin"
for command in docker podman; do
  cat > "$test_root/bin/$command" <<'MOCK'
#!/usr/bin/env bash
echo 'Forbidden host container runtime invocation' >&2
exit 99
MOCK
done
cat > "$test_root/bin/sbx" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
printf 'sbx %s\n' "$*" >> "$WORKFLOW_LOG"
case "$1" in
 version) echo 'Client Version: v0.39.0 test' ;;
 daemon) echo '{"status":"running"}' ;;
 setup|diagnose|stop) ;;
 ls) if [[ "${EXISTING_SANDBOX:-0}" == 1 ]];then cat "$SANDBOX_STATE";fi ;;
 run)
   [[ " $* " != *' --template '* && " $* " != *' --pull '* ]]
   while [[ $# -gt 0 ]]; do if [[ "$1" == --name ]];then echo "$2" > "$SANDBOX_STATE";break;fi;shift;done
   ;;
 exec)
   if [[ "${!#}" == 'pwd -P' ]];then echo '/workspace/project with spaces'
   elif [[ " $* " == *' bash code/.sandbox/start-development-container.sh '* ]];then
     [[ "${FAIL_CONTAINER:-0}" == 0 ]] || exit 1
   elif [[ " $* " == *' node code/.sandbox/development-container.js uri '* ]];then
     printf 'vscode-remote://dev-container+7b7d@ssh-remote+test.sbx/workspace/project%%20with%%20spaces\n'
   else echo "Unexpected sbx command: $*" >&2;exit 2;fi
   ;;
 *) echo "Unexpected sbx command: $*" >&2;exit 2 ;;
esac
MOCK
cat > "$test_root/bin/code" <<'MOCK'
#!/usr/bin/env bash
printf 'code %s\n' "$*" >> "$WORKFLOW_LOG"
MOCK
cat > "$test_root/bin/ssh" <<'MOCK'
#!/usr/bin/env bash
exit 0
MOCK
cat > "$test_root/bin/startup-helper" <<'MOCK'
#!/usr/bin/env bash
printf 'startup-helper %s\n' "$*" >> "$WORKFLOW_LOG"
[[ "${FAIL_POLICY:-0}" == 0 ]]
MOCK
chmod +x "$test_root/bin/"*
export PATH="$test_root/bin:$PATH"
for language in python r; do
 project="$test_root/$language project";mkdir -p "$project/code/.sandbox"
 cp "$repository_root/internal/launcherbundle/bundles/$language-sandbox/assets/run-$language-sandbox.sh" "$project/code/.sandbox/"
 cp "$repository_root/internal/launcherbundle/bundles/common/assets/"* "$project/code/.sandbox/"
 export WORKFLOW_LOG="$test_root/$language.log" SANDBOX_STATE="$test_root/$language.sandbox"
 bash "$project/code/.sandbox/run-$language-sandbox.sh" codex >/dev/null
 grep -F 'shell ' "$WORKFLOW_LOG" >/dev/null
 grep -F 'bash code/.sandbox/start-development-container.sh both' "$WORKFLOW_LOG" >/dev/null
 grep -F 'code --install-extension ms-vscode-remote.remote-containers' "$WORKFLOW_LOG" >/dev/null
 grep -F 'code --folder-uri vscode-remote://dev-container+' "$WORKFLOW_LOG" >/dev/null
 ! grep -E 'sbx template|podman|sbx exec.*--user root' "$WORKFLOW_LOG" >/dev/null
 : > "$WORKFLOW_LOG"
 if FAIL_CONTAINER=1 bash "$project/code/.sandbox/run-$language-sandbox.sh" codex >/dev/null 2>&1;then echo 'Failed container startup was accepted' >&2;exit 1;fi
 ! grep -F 'code --folder-uri' "$WORKFLOW_LOG" >/dev/null
 : > "$WORKFLOW_LOG"
 if VSCODE_SANDBOX_STARTUP_HELPER="$test_root/bin/startup-helper" FAIL_POLICY=1 bash "$project/code/.sandbox/run-$language-sandbox.sh" codex >/dev/null 2>&1;then echo 'Failed policy setup was accepted' >&2;exit 1;fi
 grep -F -- '--deny-network **' "$WORKFLOW_LOG" >/dev/null
 ! grep -F 'bash code/.sandbox/start-development-container.sh' "$WORKFLOW_LOG" >/dev/null
 : > "$WORKFLOW_LOG"
 EXISTING_SANDBOX=1 VSCODE_SANDBOX_STARTUP_HELPER="$test_root/bin/startup-helper" bash "$project/code/.sandbox/run-$language-sandbox.sh" codex >/dev/null
 grep -F 'startup-helper --startup-guard' "$WORKFLOW_LOG" >/dev/null
 grep -F 'startup-helper --startup-apply' "$WORKFLOW_LOG" >/dev/null
 previous="$(cat "$SANDBOX_STATE")"
 bash "$project/code/.sandbox/run-$language-sandbox.sh" claude >/dev/null
 [[ "$(cat "$SANDBOX_STATE")" == "$previous" ]]
 grep -F 'bash code/.sandbox/start-development-container.sh both'  "$WORKFLOW_LOG" >/dev/null
done
echo 'Development-container host workflow tests passed.'
