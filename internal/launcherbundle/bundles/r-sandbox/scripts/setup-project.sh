#!/usr/bin/env bash
set -euo pipefail

target_dir="${1:-}"
r_version="${2:-}"
agent="${3:-}"
script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
skill_dir="$(dirname -- "$script_dir")"
skills_repository="${R_SANDBOX_SKILLS_REPOSITORY:-https://github.com/mpg-age-bioinformatics/skills.git}"
project_runner_source="${VSCODE_SANDBOX_PROJECT_RUNNER_SOURCE:-}"
project_runner_agent="${VSCODE_SANDBOX_PROJECT_RUNNER_AGENT:-}"

if [[ -z "$target_dir" || -z "$r_version" || -z "$agent" ]]; then
  echo "Usage: setup-project.sh <absolute-project-directory> <R-version> <codex|claude>" >&2
  exit 2
fi
[[ "$target_dir" == /* && -d "$target_dir" ]] || { echo "Error: project directory must be an absolute existing path." >&2; exit 2; }
[[ "$r_version" =~ ^[0-9]+\.[0-9]+(\.[0-9]+)?$ ]] || { echo "Error: R version must be major.minor or major.minor.patch." >&2; exit 2; }
case "$agent" in
  codex) agent_template="codex" ;;
  claude) agent_template="claude-code" ;;
  *) echo "Error: agent must be codex or claude." >&2; exit 2 ;;
esac

IFS=. read -r r_major r_minor _ <<< "$r_version"
image_version="$r_major.$r_minor"
cd "$target_dir"
mkdir -p code data .vscode .r-library

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

generated_dockerfile="$(mktemp)"
generated_agent="$(mktemp)"
trap 'rm -f "$generated_dockerfile" "$generated_agent"' EXIT
sed -e "s/__R_VERSION__/${image_version}/g" -e "s/__AGENT_TEMPLATE__/${agent_template}/g" "$skill_dir/assets/Dockerfile.template" > "$generated_dockerfile"
printf '%s\n' "$agent" > "$generated_agent"
install_if_absent_or_identical "$generated_dockerfile" code/Dockerfile
install_if_absent_or_identical "$skill_dir/assets/run-r-sandbox.sh" code/run-r-sandbox.sh
chmod +x code/run-r-sandbox.sh
setup_paths=(.vscode/settings.json .vscode/extensions.json .gitignore .gitattributes .Renviron AGENTS.md .instructions.md code/Dockerfile code/run-r-sandbox.sh)
if [[ -n "$project_runner_source" ]]; then
  [[ -f "$project_runner_source" ]] || { echo "Error: bundled Windows project runner is unavailable: $project_runner_source" >&2; exit 1; }
  [[ "$project_runner_agent" == "$agent" ]] || { echo "Error: bundled Windows project runner agent does not match the setup selection." >&2; exit 1; }
  install_if_absent_or_identical "$project_runner_source" "code/Run R Sandbox.exe"
  install_if_absent_or_identical "$generated_agent" code/.r-sandbox-agent
  setup_paths+=("code/Run R Sandbox.exe" code/.r-sandbox-agent)
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
