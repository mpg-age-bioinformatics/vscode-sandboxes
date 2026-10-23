#!/usr/bin/env bash
set -euo pipefail

repository_root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
test_root="$(mktemp -d "${TMPDIR:-/tmp}/vscode-sandbox-self-contained.XXXXXX")"
trap 'rm -rf -- "$test_root"' EXIT

mock_bin="$test_root/bin"
mkdir -p "$mock_bin"
cat > "$mock_bin/git" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
for argument in "$@"; do
  if [[ "$argument" == clone ]]; then
    destination="${!#}"
    mkdir -p "$destination/.git"
    printf 'This clone deliberately contains no launcher files.\n' > "$destination/EMPTY_SKILLS_CLONE"
    exit 0
  fi
done
if [[ " $* " == *" rev-parse --verify HEAD "* ]]; then
  exit 1
fi
exit 0
MOCK
chmod +x "$mock_bin/git"

runner_source="$test_root/installer.exe"
printf 'self-contained runner\n' > "$runner_source"

run_case() {
  local slug="$1" version="$2" agent="codex"
  local bundle="$repository_root/internal/launcherbundle/bundles/$slug"
  local project="$test_root/$slug-project"
  mkdir -p "$project"

  local -a args=("$project")
  if [[ -n "$version" ]]; then
    args+=("$version")
  fi
  args+=("$agent")
  env PATH="$mock_bin:$PATH" \
    VSCODE_SANDBOX_PROJECT_RUNNER_SOURCE="$runner_source" \
    VSCODE_SANDBOX_PROJECT_RUNNER_AGENT="$agent" \
    "$bundle/scripts/setup-project.sh" "${args[@]}" >/dev/null

  [[ -f "$project/skills/EMPTY_SKILLS_CLONE" ]]
  case "$slug" in
    python-sandbox)
      runner_name="Run Python Sandbox.exe"
      cmp "$bundle/assets/run-python-sandbox.sh" "$project/code/run-python-sandbox.sh"
      [[ "$(< "$project/code/.python-sandbox-agent")" == "$agent" ]]
      ;;
    r-sandbox)
      runner_name="Run R Sandbox.exe"
      cmp "$bundle/assets/run-r-sandbox.sh" "$project/code/run-r-sandbox.sh"
      [[ "$(< "$project/code/.r-sandbox-agent")" == "$agent" ]]
      for script in activate-project.R setup-renv.R verify-renv.R; do
        cmp "$bundle/assets/$script" "$project/code/$script"
      done
      cmp "$bundle/assets/Rprofile" "$project/.Rprofile"
      # Use real Git here: ensure reproduction files survive the root allowlist.
      git -C "$project" init -q
      mkdir -p "$project/renv/cache" "$project/renv/library" "$project/.r-library/pkg"
      for tracked in .Rprofile .Renviron renv.lock renv/activate.R renv/settings.json code/setup-renv.R data/demo.csv data/demo.png; do
        touch "$project/$tracked"
        if git -C "$project" check-ignore -q "$tracked"; then
          echo "Reproduction file ignored: $tracked" >&2
          exit 1
        fi
      done
      for ignored in .r-library/pkg/file renv/cache/file renv/library/file skills/EMPTY_SKILLS_CLONE; do
        git -C "$project" check-ignore -q "$ignored"
      done
      ;;
    bioinformatics-sandbox)
      runner_name="Run Bioinformatics Sandbox.exe"
      cmp "$bundle/assets/run-bioinformatics-sandbox.sh" "$project/code/run-bioinformatics-sandbox.sh"
      [[ "$(< "$project/code/.bioinformatics-sandbox-agent")" == "$agent" ]]
      ;;
  esac
  cmp "$runner_source" "$project/code/$runner_name"
  [[ -z "$(find "$project" -path "$project/skills" -prune -o -name EMPTY_SKILLS_CLONE -print)" ]]
}

run_case python-sandbox 3.13
run_case r-sandbox 4.5
run_case bioinformatics-sandbox ""

echo "Self-contained launcher tests passed."
