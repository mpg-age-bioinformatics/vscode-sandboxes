#!/usr/bin/env bash
set -euo pipefail

repository_root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
test_root="$(mktemp -d "${TMPDIR:-/tmp}/vscode-sandbox-daemon.XXXXXX")"
trap 'rm -rf -- "$test_root"' EXIT
mock_bin="$test_root/bin"
mkdir -p "$mock_bin" "$test_root/project"

cat > "$mock_bin/sbx" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
printf 'sbx %s\n' "$*" >> "$DAEMON_TEST_LOG"
case "${1:-}:${2:-}" in
  version:)
    echo 'sbx version: v0.39.0 test'
    ;;
  daemon:status)
    if [[ -f "$DAEMON_TEST_STATE" ]]; then
      printf '{"status":"running"}\n'
    else
      printf '{"status":"stopped"}\n'
    fi
    ;;
  daemon:start)
    [[ "${3:-}" == "--detach" ]]
    : > "$DAEMON_TEST_STATE"
    ;;
  diagnose: | setup:ssh)
    exit 1
    ;;
  *)
    exit 1
    ;;
esac
MOCK

for command in git ssh code docker; do
  cat > "$mock_bin/$command" <<'MOCK'
#!/usr/bin/env bash
exit 0
MOCK
done
cat > "$mock_bin/uname" <<'MOCK'
#!/usr/bin/env bash
echo Darwin
MOCK
chmod +x "$mock_bin"/*

run_case() {
  local name="$1" script="$2"
  shift 2
  local state="$test_root/$name.state" log="$test_root/$name.log"
  : > "$log"
  env PATH="$mock_bin:$PATH" DAEMON_TEST_STATE="$state" DAEMON_TEST_LOG="$log" \
    bash "$script" "$@" >/dev/null 2>&1 || true
  grep -F 'sbx daemon status --json' "$log" >/dev/null
  grep -F 'sbx daemon start --detach' "$log" >/dev/null
  [[ -f "$state" ]]
}

run_case python-app "$repository_root/python-sandbox/assets/Python Sandbox.app/Contents/Resources/Python Sandbox.command" 3.13 codex "$test_root/project"
run_case r-app "$repository_root/r-sandbox/assets/R Sandbox.app/Contents/Resources/R Sandbox.command" 4.5 codex "$test_root/project"
run_case bioinformatics-app "$repository_root/bioinformatics-sandbox/assets/Bioinformatics Sandbox.app/Contents/Resources/Bioinformatics Sandbox.command" codex "$test_root/project"
run_case python-runtime "$repository_root/internal/launcherbundle/bundles/python-sandbox/assets/run-python-sandbox.sh" codex
run_case r-runtime "$repository_root/internal/launcherbundle/bundles/r-sandbox/assets/run-r-sandbox.sh" codex
run_case bioinformatics-runtime "$repository_root/internal/launcherbundle/bundles/bioinformatics-sandbox/assets/run-bioinformatics-sandbox.sh" codex

echo "Daemon auto-start tests passed."
