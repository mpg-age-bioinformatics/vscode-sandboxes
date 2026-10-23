#!/usr/bin/env bash
# Exercise generated launchers without host Docker and reject unsafe fallbacks.
set -euo pipefail
repository_root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
test_root="$(mktemp -d "${TMPDIR:-/tmp}/vscode-podman.XXXXXX")"
trap 'rm -rf -- "$test_root"' EXIT
mkdir -p "$test_root/bin"
cat > "$test_root/bin/podman" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
printf 'podman %s\n' "$*" >> "$WORKFLOW_LOG"
case "$1" in
 info) [[ "${FAIL_PODMAN:-0}" == 0 ]] || exit 1; echo linux ;;
 build) ;;
 save) [[ "$2:$3" == '--format:docker-archive' ]]; touch "${!#}" ;;
 *) exit 1 ;;
esac
MOCK
cat > "$test_root/bin/docker" <<'MOCK'
#!/usr/bin/env bash
echo 'Unexpected host Docker invocation' >&2
exit 99
MOCK
cat > "$test_root/bin/sbx" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
printf 'sbx %s\n' "$*" >> "$WORKFLOW_LOG"
case "$1" in
 version) printf 'Client Version:  v0.39.0 test\nServer Version:  v0.39.0 test\n' ;;
 daemon) echo '{"status":"running"}' ;;
 setup|diagnose) ;;
 template) [[ "$2" == load && -f "$3" ]] ;;
 ls) ;;
 run) ;;
 exec)
   if [[ "$2" == --user ]]; then
     [[ "$3" == root && "${!#}" == *'podman --storage-driver=vfs --events-backend=file info'* ]]
     [[ "${FAIL_BOOTSTRAP:-0}" == 0 ]] || exit 1
   elif [[ "${!#}" == 'pwd -P' ]]; then
     echo /workspace/project
   fi
   ;;
 *) exit 1 ;;
esac
MOCK
chmod +x "$test_root/bin/"*
for language in python r bioinformatics; do
  project="$test_root/$language"
  mkdir -p "$project/code"
  bundle="$repository_root/internal/launcherbundle/bundles/$language-sandbox/assets"
  cp "$bundle/run-$language-sandbox.sh" "$project/code/"
  if [[ -f "$bundle/Dockerfile.template" ]]; then
    sed -e s/__PYTHON_VERSION__/3.13/g -e s/__R_VERSION__/4.5/g "$bundle/Dockerfile.template" > "$project/code/Dockerfile"
  fi
  export WORKFLOW_LOG="$test_root/$language.log"
  export PATH="$test_root/bin:$PATH"
  export PYTHON_SANDBOX_SKIP_VSCODE=1 R_SANDBOX_SKIP_VSCODE=1 BIOINFORMATICS_SANDBOX_SKIP_VSCODE=1
  bash "$project/code/run-$language-sandbox.sh" codex >/dev/null
  grep -F 'sbx exec --user root' "$WORKFLOW_LOG" >/dev/null
  if [[ "$language" != bioinformatics ]]; then
    grep -F 'podman save --format docker-archive' "$WORKFLOW_LOG" >/dev/null
    grep -F 'sbx run --detached --pull never' "$WORKFLOW_LOG" >/dev/null
    : > "$WORKFLOW_LOG"
    if FAIL_PODMAN=1 bash "$project/code/run-$language-sandbox.sh" codex >/dev/null 2>&1; then
      echo 'Unavailable Podman was incorrectly accepted' >&2; exit 1
    fi
    ! grep -F 'sbx run ' "$WORKFLOW_LOG" >/dev/null
  else
    ! grep -F 'podman ' "$WORKFLOW_LOG" >/dev/null
  fi
  if FAIL_BOOTSTRAP=1 bash "$project/code/run-$language-sandbox.sh" codex >/dev/null 2>&1; then
    echo 'Failed sandbox Podman setup was incorrectly accepted' >&2; exit 1
  fi
done
echo 'Podman workflow tests passed.'
