#!/usr/bin/env bash
set -euo pipefail

target_dir="${1:-}"
python_version="${2:-}"
agent="${3:-}"
script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
skill_dir="$(dirname -- "$script_dir")"
skills_repository="${PYTHON_SANDBOX_SKILLS_REPOSITORY:-https://github.com/mpg-age-bioinformatics/skills.git}"
project_runner_source="${VSCODE_SANDBOX_PROJECT_RUNNER_SOURCE:-}"
project_runner_agent="${VSCODE_SANDBOX_PROJECT_RUNNER_AGENT:-}"
project_app_template="${VSCODE_SANDBOX_PROJECT_APP_TEMPLATE:-}"
project_app_icon="${VSCODE_SANDBOX_PROJECT_APP_ICON:-}"

if [[ -z "$target_dir" || -z "$python_version" || -z "$agent" ]]; then
  echo "Usage: setup-project.sh <absolute-project-directory> <Python-version> <codex|claude>" >&2
  exit 2
fi
[[ "$target_dir" == /* && -d "$target_dir" ]] || { echo "Error: project directory must be an absolute existing path." >&2; exit 2; }
[[ "$python_version" =~ ^[0-9]+\.[0-9]+(\.[0-9]+)?$ ]] || { echo "Error: Python version must be major.minor or major.minor.patch." >&2; exit 2; }
case "$agent" in
  codex) agent_template="codex" ;;
  claude) agent_template="claude-code" ;;
  *) echo "Error: agent must be codex or claude." >&2; exit 2 ;;
esac

IFS=. read -r python_major python_minor _ <<< "$python_version"
image_version="$python_major.$python_minor"
cd "$target_dir"
mkdir -p code data .vscode

if [[ -e skills ]]; then
  [[ -d skills/.git ]] || { echo "Error: skills/ exists and is not a Git clone." >&2; exit 1; }
  existing_origin="$(git -C skills remote get-url origin 2>/dev/null || true)"
  [[ "$existing_origin" == "$skills_repository" ]] || { echo "Error: skills/ has a different origin: ${existing_origin:-<none>}" >&2; exit 1; }
else
  git -c core.autocrlf=false clone "$skills_repository" skills
fi

install_if_absent_or_identical() {
  local source_path="$1" destination_path="$2"
  if [[ -e "$destination_path" ]]; then
    cmp -s "$source_path" "$destination_path" || { echo "Error: refusing to overwrite existing $destination_path" >&2; exit 1; }
  else
    cp "$source_path" "$destination_path"
  fi
}

install_dir_if_absent_or_identical() {
  local source_path="$1" destination_path="$2"
  if [[ -e "$destination_path" ]]; then
    diff -qr "$source_path" "$destination_path" >/dev/null || { echo "Error: refusing to overwrite existing $destination_path" >&2; exit 1; }
  else
    cp -R "$source_path" "$destination_path"
  fi
}

generated_dockerfile="$(mktemp)"
generated_command="$(mktemp)"
generated_agent="$(mktemp)"
trap 'rm -f "$generated_dockerfile" "$generated_command" "$generated_agent"' EXIT
sed -e "s/__PYTHON_VERSION__/${image_version}/g" -e "s/__AGENT_TEMPLATE__/${agent_template}/g" "$skill_dir/assets/Dockerfile.template" > "$generated_dockerfile"
sed -e "s/__AGENT__/${agent}/g" "$skill_dir/assets/run-python-sandbox.command.template" > "$generated_command"
printf '%s\n' "$agent" > "$generated_agent"
install_if_absent_or_identical "$generated_dockerfile" code/Dockerfile
install_if_absent_or_identical "$skill_dir/assets/run-python-sandbox.sh" code/run-python-sandbox.sh
install_if_absent_or_identical "$skill_dir/assets/windows-venv-sitecustomize.py" code/windows-venv-sitecustomize.py
install_if_absent_or_identical "$generated_command" code/run-python-sandbox.command
chmod +x code/run-python-sandbox.sh
chmod 755 code/run-python-sandbox.command
setup_paths=(.vscode/settings.json .vscode/extensions.json .gitignore .gitattributes AGENTS.md .instructions.md code/Dockerfile code/run-python-sandbox.sh code/windows-venv-sitecustomize.py code/run-python-sandbox.command)
if [[ -n "$project_runner_source" ]]; then
  [[ -f "$project_runner_source" ]] || { echo "Error: bundled Windows project runner is unavailable: $project_runner_source" >&2; exit 1; }
  [[ "$project_runner_agent" == "$agent" ]] || { echo "Error: bundled Windows project runner agent does not match the setup selection." >&2; exit 1; }
  install_if_absent_or_identical "$project_runner_source" "code/Run Python Sandbox.exe"
  install_if_absent_or_identical "$generated_agent" code/.python-sandbox-agent
  setup_paths+=("code/Run Python Sandbox.exe" code/.python-sandbox-agent)
fi
if [[ -n "$project_app_template" ]]; then
  [[ -d "$project_app_template" && -f "$project_app_icon" ]] || { echo "Error: bundled macOS project launcher is incomplete." >&2; exit 1; }
  mkdir -p "code/Run Python Sandbox.app/Contents/MacOS" "code/Run Python Sandbox.app/Contents/Resources"
  install_if_absent_or_identical "$project_app_template/Contents/Info.plist" "code/Run Python Sandbox.app/Contents/Info.plist"
  install_if_absent_or_identical "$project_app_template/Contents/MacOS/run-python-sandbox" "code/Run Python Sandbox.app/Contents/MacOS/run-python-sandbox"
  install_if_absent_or_identical "$project_app_icon" "code/Run Python Sandbox.app/Contents/Resources/PythonSandbox.icns"
  chmod 755 "code/Run Python Sandbox.app/Contents/MacOS/run-python-sandbox"
  setup_paths+=("code/Run Python Sandbox.app")
fi
install_if_absent_or_identical "$skill_dir/assets/settings.json" .vscode/settings.json
install_if_absent_or_identical "$skill_dir/assets/extensions.json" .vscode/extensions.json
install_if_absent_or_identical "$skill_dir/assets/.instructions.md" .instructions.md
install_if_absent_or_identical "$skill_dir/assets/AGENTS.md" AGENTS.md

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

git init
if git rev-parse --verify HEAD >/dev/null 2>&1; then
  git add -f -A -- "${setup_paths[@]}"
  if git diff --cached --quiet -- "${setup_paths[@]}"; then
    echo "Python Sandbox setup is already committed."
  else
    git commit --only -m "Add Python sandbox setup" -- "${setup_paths[@]}"
  fi
else
  git add -f -A -- "${setup_paths[@]}"
  git commit --only -m "Initial Python sandbox setup" -- "${setup_paths[@]}"
fi

git status --short
git show --stat --oneline HEAD
