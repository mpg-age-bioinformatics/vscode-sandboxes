#!/usr/bin/env bash
set -euo pipefail

target_dir="${1:-}"
r_version="${2:-}"
agent="${3:-both}"
script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
skill_dir="$(dirname -- "$script_dir")"
skills_repository="${R_SANDBOX_SKILLS_REPOSITORY:-https://github.com/mpg-age-bioinformatics/skills.git}"
project_runner_source="${VSCODE_SANDBOX_PROJECT_RUNNER_SOURCE:-}"
project_runner_agent="${VSCODE_SANDBOX_PROJECT_RUNNER_AGENT:-}"
project_app_template="${VSCODE_SANDBOX_PROJECT_APP_TEMPLATE:-}"
project_app_icon="${VSCODE_SANDBOX_PROJECT_APP_ICON:-}"

if [[ -z "$target_dir" || -z "$r_version" || -z "$agent" ]]; then
  echo "Usage: setup-project.sh <absolute-project-directory> <R-version> [both]" >&2
  exit 2
fi
[[ "$target_dir" == /* && -d "$target_dir" ]] || { echo "Error: project directory must be an absolute existing path." >&2; exit 2; }
[[ "$r_version" =~ ^[0-9]+\.[0-9]+(\.[0-9]+)?$ ]] || { echo "Error: R version must be major.minor or major.minor.patch." >&2; exit 2; }
case "$agent" in
  both|codex|claude) ;;
  *) echo "Error: unsupported legacy agent argument." >&2; exit 2 ;;
esac

IFS=. read -r r_major r_minor _ <<< "$r_version"
image_version="$r_major.$r_minor"
cd "$target_dir"
mkdir -p code/.sandbox data .vscode .r-library

skills_setup="$skill_dir/assets/setup-skills.sh"
if [[ ! -f "$skills_setup" ]]; then skills_setup="$skill_dir/../common/assets/setup-skills.sh"; fi
source "$skills_setup"
setup_sandbox_skills "$skills_repository"

install_if_absent_or_identical() {
  local source_path="$1" destination_path="$2"
  if [[ -e "$destination_path" ]]; then
    cmp -s "$source_path" "$destination_path" || { echo "Error: refusing to overwrite existing $destination_path" >&2; exit 1; }
  else
    mkdir -p "$(dirname -- "$destination_path")"
    cp "$source_path" "$destination_path"
  fi
}

generated_devcontainer="$(mktemp)"
generated_runtime="$(mktemp)"
generated_dockerfile="$(mktemp)"
generated_agent="$(mktemp)"
trap 'rm -f "$generated_devcontainer" "$generated_runtime" "$generated_dockerfile" "$generated_agent"' EXIT
sed -e "s/__R_VERSION__/${image_version}/g" -e "s/__AGENT__/${agent}/g" "$skill_dir/assets/devcontainer.json.template" > "$generated_devcontainer"
printf '{"language":"r","series":"%s","baseImage":"ghcr.io/rocker-org/devcontainer/r-ver:%s"}\n' "$image_version" "$image_version" > "$generated_runtime"
printf '%s\n' "$agent" > "$generated_agent"
sed "s/__R_VERSION__/${image_version}/g" "$skill_dir/assets/Dockerfile.template" > "$generated_dockerfile"
install_if_absent_or_identical "$generated_dockerfile" code/Dockerfile
install_if_absent_or_identical "$generated_devcontainer" code/.sandbox/devcontainer.json
install_if_absent_or_identical "$generated_runtime" code/.sandbox/sandbox-runtime.json
shared_assets="$skill_dir/assets"
if [[ ! -f "$shared_assets/run-language-sandbox.sh" ]]; then shared_assets="$skill_dir/../common/assets"; fi
for shared in prepare-continue.py install-continue.js setup-continue.sh agent-sandbox-seccomp.json agent-sandbox-seccomp-LICENSE.txt agent-sandbox-seccomp-NOTICE.txt install-coding-agents.sh run-language-sandbox.sh start-development-container.sh development-container.js initialize-development-container.sh verify-development-container.sh; do
  install_if_absent_or_identical "$shared_assets/$shared" "code/.sandbox/$shared"
done
chmod +x code/.sandbox/install-coding-agents.sh code/.sandbox/run-language-sandbox.sh code/.sandbox/start-development-container.sh code/.sandbox/initialize-development-container.sh code/.sandbox/verify-development-container.sh
install_if_absent_or_identical "$skill_dir/assets/run-r-sandbox.sh" code/.sandbox/run-r-sandbox.sh
chmod +x code/.sandbox/run-r-sandbox.sh
# Keep the launcher's notices with generated code without licensing user code.
notice_root="$skill_dir"
if [[ ! -f "$notice_root/LICENSE" ]]; then
  # Direct development invocation from the tracked vendored bundle.
  notice_root="$skill_dir/../../../.."
fi
install_if_absent_or_identical "$notice_root/LICENSE" code/.sandbox/VS-Code-Sandboxes-LICENSE.txt
install_if_absent_or_identical "$notice_root/THIRD-PARTY-NOTICES.txt" code/.sandbox/VS-Code-Sandboxes-THIRD-PARTY-NOTICES.txt
install_if_absent_or_identical "$notice_root/DISCLAIMER.txt" code/.sandbox/VS-Code-Sandboxes-DISCLAIMER.txt
setup_paths=(code/.sandbox/prepare-continue.py code/.sandbox/install-continue.js code/.sandbox/setup-continue.sh code/Dockerfile .Rprofile code/.sandbox/activate-project.R code/.sandbox/setup-renv.R code/.sandbox/verify-renv.R code/.sandbox/setup-vscode-r.R code/.sandbox/install-r-system-dependencies.sh .vscode/settings.json .vscode/extensions.json .gitignore .gitattributes .Renviron AGENTS.md .instructions.md code/.sandbox/devcontainer.json code/.sandbox/agent-sandbox-seccomp.json code/.sandbox/agent-sandbox-seccomp-LICENSE.txt code/.sandbox/agent-sandbox-seccomp-NOTICE.txt code/.sandbox/sandbox-runtime.json code/.sandbox/install-coding-agents.sh code/.sandbox/run-language-sandbox.sh code/.sandbox/start-development-container.sh code/.sandbox/development-container.js code/.sandbox/initialize-development-container.sh code/.sandbox/verify-development-container.sh code/.sandbox/run-r-sandbox.sh code/.sandbox/VS-Code-Sandboxes-LICENSE.txt code/.sandbox/VS-Code-Sandboxes-THIRD-PARTY-NOTICES.txt code/.sandbox/VS-Code-Sandboxes-DISCLAIMER.txt)
if [[ -n "$project_runner_source" ]]; then
  [[ -f "$project_runner_source" ]] || { echo "Error: bundled Windows project runner is unavailable: $project_runner_source" >&2; exit 1; }
  [[ "$project_runner_agent" == "$agent" ]] || { echo "Error: bundled Windows project runner agent does not match the setup selection." >&2; exit 1; }
  install_if_absent_or_identical "$project_runner_source" "code/.sandbox/Run R Sandbox.exe"
  install_if_absent_or_identical "$generated_agent" code/.sandbox/.r-sandbox-agent
  setup_paths+=("code/.sandbox/Run R Sandbox.exe" code/.sandbox/.r-sandbox-agent)
fi
if [[ -n "$project_app_template" ]]; then
  install_if_absent_or_identical "$generated_agent" code/.sandbox/.r-sandbox-agent
  setup_paths+=(code/.sandbox/.r-sandbox-agent)
  [[ -d "$project_app_template" && -f "$project_app_icon" ]] || { echo "Error: bundled macOS project launcher is incomplete." >&2; exit 1; }
  mkdir -p "code/.sandbox/Run R Sandbox.app/Contents/MacOS" "code/.sandbox/Run R Sandbox.app/Contents/Resources"
  install_if_absent_or_identical "$project_app_template/Contents/Info.plist" "code/.sandbox/Run R Sandbox.app/Contents/Info.plist"
  install_if_absent_or_identical "$project_app_template/Contents/MacOS/run-r-sandbox" "code/.sandbox/Run R Sandbox.app/Contents/MacOS/run-r-sandbox"
  install_if_absent_or_identical "$project_app_icon" "code/.sandbox/Run R Sandbox.app/Contents/Resources/RSandbox.icns"
  for notice in LICENSE THIRD-PARTY-NOTICES.txt DISCLAIMER.txt; do
    install_if_absent_or_identical "$notice_root/$notice" "code/.sandbox/Run R Sandbox.app/Contents/Resources/$notice"
  done
  for binary in "Sandbox Desktop" "Sandbox Window"; do
    install_if_absent_or_identical "${VSCODE_SANDBOX_DESKTOP_RESOURCES:-$project_app_template/Contents/Resources}/$binary" "code/.sandbox/Run R Sandbox.app/Contents/Resources/$binary"
    chmod 755 "code/.sandbox/Run R Sandbox.app/Contents/Resources/$binary"
  done
  chmod 755 "code/.sandbox/Run R Sandbox.app/Contents/MacOS/run-r-sandbox"
  setup_paths+=("code/.sandbox/Run R Sandbox.app")
fi
install_if_absent_or_identical "$skill_dir/assets/settings.json" .vscode/settings.json
install_if_absent_or_identical "$skill_dir/assets/extensions.json" .vscode/extensions.json
install_if_absent_or_identical "$skill_dir/assets/.instructions.md" .instructions.md
if [[ ! -e AGENTS.md ]]; then
  cp "$skill_dir/assets/AGENTS.md" AGENTS.md
elif ! grep -Fq '<!-- r-sandbox:scientific-libraries:start -->' AGENTS.md; then
  printf '\n' >> AGENTS.md
  sed -n '/<!-- r-sandbox:scientific-libraries:start -->/,/<!-- r-sandbox:scientific-libraries:end -->/p' \
    "$skill_dir/assets/AGENTS.md" >> AGENTS.md
fi
install_if_absent_or_identical "$skill_dir/assets/Renviron" .Renviron
install_if_absent_or_identical "$skill_dir/assets/Rprofile" .Rprofile
for script in activate-project.R setup-renv.R verify-renv.R setup-vscode-r.R install-r-system-dependencies.sh; do
  install_if_absent_or_identical "$skill_dir/assets/$script" "code/.sandbox/$script"
done

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
    echo "R Sandbox setup is already committed."
  else
    git commit --only -m "Add R sandbox setup" -- "${setup_paths[@]}"
  fi
else
  git add -f -A -- "${setup_paths[@]}"
  git commit --only -m "Initial R sandbox setup" -- "${setup_paths[@]}"
fi

git status --short
git show --stat --oneline HEAD
