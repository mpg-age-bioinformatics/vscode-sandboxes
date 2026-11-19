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
if [[ " $* " == *" config --local --get remote.origin.url "* ]]; then
  printf 'https://github.com/mpg-age-bioinformatics/skills.git\n'
  exit 0
fi
if [[ " $* " == *" rev-parse --git-path hooks "* ]]; then
  printf '.git/hooks\n'
  exit 0
fi
if [[ " $* " == *" rev-parse --verify HEAD "* ]]; then
  exit 1
fi
exit 0
MOCK
chmod +x "$mock_bin/git"

runner_source="$test_root/installer.exe"
printf 'self-contained runner\n' > "$runner_source"

run_case() {
  local slug="$1" version="$2" agent="both"
  local bundle="$repository_root/internal/launcherbundle/bundles/$slug"
  if [[ -n "${SANDBOX_TEST_PACKAGED_LAUNCHERS:-}" ]]; then
    bundle="$SANDBOX_TEST_PACKAGED_LAUNCHERS/$slug/Setup"
  fi
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
  cmp "$repository_root/LICENSE" "$project/code/.sandbox/VS-Code-Sandboxes-LICENSE.txt"
  cmp "$repository_root/THIRD-PARTY-NOTICES.txt" "$project/code/.sandbox/VS-Code-Sandboxes-THIRD-PARTY-NOTICES.txt"
  case "$slug" in
    python-sandbox)
      runner_name="Run Python Sandbox.exe"
      cmp "$bundle/assets/run-python-sandbox.sh" "$project/code/.sandbox/run-python-sandbox.sh"
      [[ "$(< "$project/code/.sandbox/.python-sandbox-agent")" == "$agent" ]]
      ;;
    r-sandbox)
      runner_name="Run R Sandbox.exe"
      cmp "$bundle/assets/run-r-sandbox.sh" "$project/code/.sandbox/run-r-sandbox.sh"
      [[ "$(< "$project/code/.sandbox/.r-sandbox-agent")" == "$agent" ]]
      for script in activate-project.R setup-renv.R verify-renv.R setup-vscode-r.R; do
        cmp "$bundle/assets/$script" "$project/code/.sandbox/$script"
      done
      cmp "$bundle/assets/Rprofile" "$project/.Rprofile"
      # Use real Git here: ensure reproduction files survive the root allowlist.
      git -C "$project" init -q
      mkdir -p "$project/renv/cache" "$project/renv/library" "$project/.r-library/pkg"
      for tracked in .Rprofile .Renviron renv.lock renv/activate.R renv/settings.json code/.sandbox/setup-renv.R; do
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
      cmp "$bundle/assets/run-bioinformatics-sandbox.sh" "$project/code/.sandbox/run-bioinformatics-sandbox.sh"
      [[ "$(< "$project/code/.sandbox/.bioinformatics-sandbox-agent")" == "$agent" ]]
      ;;
  esac
  if [[ "$slug" != bioinformatics-sandbox ]]; then
    [[ -f "$project/code/.sandbox/devcontainer.json" && -f "$project/code/.sandbox/sandbox-runtime.json" ]]
    [[ -f "$project/code/Dockerfile" ]]
    [[ "$(find "$project/code" -mindepth 1 -maxdepth 1 ! -name .sandbox ! -name Dockerfile | wc -l | tr -d ' ')" == 0 ]]
    for shared in run-language-sandbox.sh start-development-container.sh development-container.js initialize-development-container.sh verify-development-container.sh; do
      cmp "$repository_root/internal/launcherbundle/bundles/common/assets/$shared" "$project/code/.sandbox/$shared"
    done
  fi
  for helper in prepare-continue.py install-continue.js setup-continue.sh; do
    cmp "$repository_root/internal/launcherbundle/bundles/common/assets/$helper" "$project/code/.sandbox/$helper"
  done
  cmp "$repository_root/internal/launcherbundle/bundles/common/assets/install-coding-agents.sh" "$project/code/.sandbox/install-coding-agents.sh"
  # Exercise real Git ignore and commit protection for all sandbox variants.
  git -C "$project" init -q
  mkdir -p "$project/code/data_nested" "$project/code/analysis/data_archive" "$project/data_12345678"
  for ignored in data/demo.csv data_12345678/result.txt code/data_nested/input.csv code/analysis/data_archive/figure.png; do
    touch "$project/$ignored"
    git -C "$project" check-ignore -q "$ignored"
    git -C "$project" add -f -- "$ignored"
  done
  if (cd "$project" && .git/hooks/pre-commit) >"$test_root/hook-output" 2>&1; then
    echo "Data commit guard accepted staged data in $slug" >&2
    exit 1
  fi
  grep -q 'data\* directories must never be committed' "$test_root/hook-output"
  git -C "$project" rm --cached -q -r -- data data_12345678 code/data_nested code/analysis/data_archive
  (cd "$project" && .git/hooks/pre-commit)
  [[ -f "$project/data/demo.csv" ]]
  cmp "$runner_source" "$project/code/.sandbox/$runner_name"
  # All flavors must also install a self-contained macOS project window.
  local app_name icon_name
  case "$slug" in
    python-sandbox) app_name="Python Sandbox"; icon_name=PythonSandbox ;;
    r-sandbox) app_name="R Sandbox"; icon_name=RSandbox ;;
    bioinformatics-sandbox) app_name="Bioinformatics Sandbox"; icon_name=BioinformaticsSandbox ;;
  esac
  local desktop_resources="$test_root/desktop-resources"
  mkdir -p "$desktop_resources"
  printf 'desktop controller fixture\n' > "$desktop_resources/Sandbox Desktop"
  printf 'native window fixture\n' > "$desktop_resources/Sandbox Window"
  env PATH="$mock_bin:$PATH" \
    VSCODE_SANDBOX_PROJECT_APP_TEMPLATE="$bundle/assets/Run $app_name.app.template" \
    VSCODE_SANDBOX_PROJECT_APP_ICON="$repository_root/$slug/assets/$app_name.app/Contents/Resources/$icon_name.icns" \
    VSCODE_SANDBOX_DESKTOP_RESOURCES="$desktop_resources" \
    "$bundle/scripts/setup-project.sh" "${args[@]}" >/dev/null
  local installed_app="$project/code/.sandbox/Run $app_name.app"
  cmp "$desktop_resources/Sandbox Desktop" "$installed_app/Contents/Resources/Sandbox Desktop"
  cmp "$desktop_resources/Sandbox Window" "$installed_app/Contents/Resources/Sandbox Window"
  [[ -x "$installed_app/Contents/MacOS/run-$slug" ]]
  [[ "$(< "$project/code/.sandbox/.$slug-agent")" == "$agent" ]]
  ! grep -F 'open -a Terminal' "$installed_app/Contents/MacOS/run-$slug"
  [[ -z "$(find "$project" -path "$project/skills" -prune -o -name EMPTY_SKILLS_CLONE -print)" ]]
}

run_case python-sandbox 3.13
run_case r-sandbox 4.5
run_case bioinformatics-sandbox ""

echo "Self-contained launcher tests passed."
