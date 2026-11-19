#!/usr/bin/env bash
set -euo pipefail

target_dir="${1:-}"
agent="${2:-both}"
script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
skill_dir="$(dirname -- "$script_dir")"
skills_repository="${BIOINFORMATICS_SANDBOX_SKILLS_REPOSITORY:-https://github.com/mpg-age-bioinformatics/skills.git}"
project_runner_source="${VSCODE_SANDBOX_PROJECT_RUNNER_SOURCE:-}"
project_runner_agent="${VSCODE_SANDBOX_PROJECT_RUNNER_AGENT:-}"
project_app_template="${VSCODE_SANDBOX_PROJECT_APP_TEMPLATE:-}"
project_app_icon="${VSCODE_SANDBOX_PROJECT_APP_ICON:-}"

if [[ -z "$target_dir" || "$target_dir" != /* || ! -d "$target_dir" ]]; then
  echo "Usage: setup-project.sh <absolute-existing-project-directory> [both]" >&2
  exit 2
fi
case "$agent" in
  both|codex|claude) ;;
  *) echo "Error: unsupported legacy agent argument." >&2; exit 2 ;;
esac

cd "$target_dir"
mkdir -p code/.sandbox data .vscode

skills_setup="$skill_dir/assets/setup-skills.sh"
if [[ ! -f "$skills_setup" ]]; then skills_setup="$skill_dir/../common/assets/setup-skills.sh"; fi
source "$skills_setup"
setup_sandbox_skills "$skills_repository"

install_if_absent_or_identical() {
  local source_path="$1"
  local destination_path="$2"
  if [[ -e "$destination_path" ]]; then
    if ! cmp -s "$source_path" "$destination_path"; then
      echo "Error: refusing to overwrite existing $destination_path" >&2
      exit 1
    fi
  else
    mkdir -p "$(dirname -- "$destination_path")"
    cp "$source_path" "$destination_path"
  fi
}

if [[ ! -e AGENTS.md ]]; then
  cp "$skill_dir/assets/AGENTS.md" AGENTS.md
elif ! grep -Fq '<!-- bioinformatics-sandbox:scientific-libraries:start -->' AGENTS.md; then
  printf '\n' >> AGENTS.md
  sed -n '/<!-- bioinformatics-sandbox:scientific-libraries:start -->/,/<!-- bioinformatics-sandbox:scientific-libraries:end -->/p' \
    "$skill_dir/assets/AGENTS.md" >> AGENTS.md
fi
install_if_absent_or_identical "$skill_dir/assets/.instructions.md" .instructions.md
shared_assets="$skill_dir/assets"
if [[ ! -f "$shared_assets/install-coding-agents.sh" ]]; then shared_assets="$skill_dir/../common/assets"; fi
install_if_absent_or_identical "$shared_assets/install-coding-agents.sh" code/.sandbox/install-coding-agents.sh
for shared in prepare-continue.py install-continue.js setup-continue.sh; do
  install_if_absent_or_identical "$shared_assets/$shared" "code/.sandbox/$shared"
done
install_if_absent_or_identical "$skill_dir/assets/extensions.json" .vscode/extensions.json
install_if_absent_or_identical "$skill_dir/assets/settings.json" .vscode/settings.json
install_if_absent_or_identical "$skill_dir/assets/run-bioinformatics-sandbox.sh" code/.sandbox/run-bioinformatics-sandbox.sh
chmod +x code/.sandbox/run-bioinformatics-sandbox.sh
generated_agent="$(mktemp)"
trap 'rm -f "$generated_agent"' EXIT
printf '%s\n' "$agent" > "$generated_agent"
# Keep the launcher's notices with generated code without licensing user code.
notice_root="$skill_dir"
if [[ ! -f "$notice_root/LICENSE" ]]; then
  # Direct development invocation from the tracked vendored bundle.
  notice_root="$skill_dir/../../../.."
fi
install_if_absent_or_identical "$notice_root/LICENSE" code/.sandbox/VS-Code-Sandboxes-LICENSE.txt
install_if_absent_or_identical "$notice_root/THIRD-PARTY-NOTICES.txt" code/.sandbox/VS-Code-Sandboxes-THIRD-PARTY-NOTICES.txt
setup_paths=(code/.sandbox/prepare-continue.py code/.sandbox/install-continue.js code/.sandbox/setup-continue.sh code/.sandbox/install-coding-agents.sh .vscode/settings.json .vscode/extensions.json .gitignore .gitattributes AGENTS.md .instructions.md code/.sandbox/run-bioinformatics-sandbox.sh code/.sandbox/VS-Code-Sandboxes-LICENSE.txt code/.sandbox/VS-Code-Sandboxes-THIRD-PARTY-NOTICES.txt)
if [[ -n "$project_runner_source" ]]; then
  [[ -f "$project_runner_source" ]] || { echo "Error: bundled Windows project runner is unavailable: $project_runner_source" >&2; exit 1; }
  [[ "$project_runner_agent" == "$agent" ]] || { echo "Error: bundled Windows project runner agent does not match the setup selection." >&2; exit 1; }
  install_if_absent_or_identical "$project_runner_source" "code/.sandbox/Run Bioinformatics Sandbox.exe"
  install_if_absent_or_identical "$generated_agent" code/.sandbox/.bioinformatics-sandbox-agent
  setup_paths+=("code/.sandbox/Run Bioinformatics Sandbox.exe" code/.sandbox/.bioinformatics-sandbox-agent)
fi

if [[ -n "$project_app_template" ]]; then
  install_if_absent_or_identical "$generated_agent" code/.sandbox/.bioinformatics-sandbox-agent
  setup_paths+=(code/.sandbox/.bioinformatics-sandbox-agent)
  [[ -d "$project_app_template" && -f "$project_app_icon" ]] || { echo "Error: bundled macOS project launcher is incomplete." >&2; exit 1; }
  mkdir -p "code/.sandbox/Run Bioinformatics Sandbox.app/Contents/MacOS" "code/.sandbox/Run Bioinformatics Sandbox.app/Contents/Resources"
  install_if_absent_or_identical "$project_app_template/Contents/Info.plist" "code/.sandbox/Run Bioinformatics Sandbox.app/Contents/Info.plist"
  install_if_absent_or_identical "$project_app_template/Contents/MacOS/run-bioinformatics-sandbox" "code/.sandbox/Run Bioinformatics Sandbox.app/Contents/MacOS/run-bioinformatics-sandbox"
  install_if_absent_or_identical "$project_app_icon" "code/.sandbox/Run Bioinformatics Sandbox.app/Contents/Resources/BioinformaticsSandbox.icns"
  for notice in LICENSE THIRD-PARTY-NOTICES.txt; do
    install_if_absent_or_identical "$notice_root/$notice" "code/.sandbox/Run Bioinformatics Sandbox.app/Contents/Resources/$notice"
  done
  for binary in "Sandbox Desktop" "Sandbox Window"; do
    install_if_absent_or_identical "${VSCODE_SANDBOX_DESKTOP_RESOURCES:-$project_app_template/Contents/Resources}/$binary" "code/.sandbox/Run Bioinformatics Sandbox.app/Contents/Resources/$binary"
    chmod 755 "code/.sandbox/Run Bioinformatics Sandbox.app/Contents/Resources/$binary"
  done
  chmod 755 "code/.sandbox/Run Bioinformatics Sandbox.app/Contents/MacOS/run-bioinformatics-sandbox"
  setup_paths+=("code/.sandbox/Run Bioinformatics Sandbox.app")
fi

if [[ ! -e .gitignore ]]; then
  cp "$skill_dir/assets/gitignore" .gitignore
else
  while IFS= read -r rule; do
    [[ -z "$rule" ]] && continue
    grep -Fqx "$rule" .gitignore || printf '%s\n' "$rule" >> .gitignore
  done < "$skill_dir/assets/gitignore"
fi

if [[ ! -e .gitattributes ]]; then
  cp "$skill_dir/assets/gitattributes" .gitattributes
else
  while IFS= read -r rule; do
    [[ -z "$rule" ]] && continue
    grep -Fqx "$rule" .gitattributes || printf '%s\n' "$rule" >> .gitattributes
  done < "$skill_dir/assets/gitattributes"
fi

# Keep the policy last so prior allow rules cannot re-include data directories.
if [[ "$(tail -n 1 .gitignore)" != 'data*/' ]]; then
  printf '\ndata*/\n' >> .gitignore
fi
if ! grep -Fq '<!-- sandbox:repository-policy:start -->' AGENTS.md; then
  sed -n '/<!-- sandbox:repository-policy:start -->/,/<!-- sandbox:repository-policy:end -->/p' "$skill_dir/assets/AGENTS.md" >> AGENTS.md
fi

git init
if [[ -z "$(git config user.name || true)" ]]; then git config user.name 'Sandbox Launcher'; fi
if [[ -z "$(git config user.email || true)" ]]; then git config user.email 'sandbox@localhost'; fi
hooks_dir="$(git rev-parse --git-path hooks)"
mkdir -p "$hooks_dir"
if [[ -f "$hooks_dir/pre-commit" ]] && ! cmp -s "$skill_dir/assets/pre-commit" "$hooks_dir/pre-commit"; then
  if [[ ! -e "$hooks_dir/pre-commit.before-sandbox" ]]; then
    mv "$hooks_dir/pre-commit" "$hooks_dir/pre-commit.before-sandbox"
  else
    echo "Error: cannot preserve existing pre-commit hook." >&2
    exit 1
  fi
fi
cp "$skill_dir/assets/pre-commit" "$hooks_dir/pre-commit"
chmod +x "$hooks_dir/pre-commit"
if git rev-parse --verify HEAD >/dev/null 2>&1; then
  git add -f -A -- "${setup_paths[@]}"
  if git diff --cached --quiet -- "${setup_paths[@]}"; then
    echo "Bioinformatics Sandbox setup is already committed."
  else
    git commit --only -m "Add bioinformatics sandbox setup" -- "${setup_paths[@]}"
  fi
else
  git add -f -A -- "${setup_paths[@]}"
  git commit --only -m "Initial bioinformatics sandbox setup" -- "${setup_paths[@]}"
fi

git status --short
git show --stat --oneline HEAD
