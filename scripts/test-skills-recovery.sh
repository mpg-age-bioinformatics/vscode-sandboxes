#!/usr/bin/env bash
set -euo pipefail
repository_root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
test_root="$(mktemp -d "${TMPDIR:-/tmp}/sandbox-skills-recovery.XXXXXX")"
trap 'rm -rf -- "$test_root"' EXIT
source "$repository_root/internal/launcherbundle/bundles/common/assets/setup-skills.sh"
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
export GIT_AUTHOR_NAME=Test GIT_AUTHOR_EMAIL=test@example.org GIT_COMMITTER_NAME=Test GIT_COMMITTER_EMAIL=test@example.org
upstream="$test_root/upstream"
mkdir -p "$upstream/match-code-data" "$upstream/set-vscode-r-version" "$upstream/other-skill"
git -C "$upstream" init -q
printf 'keep\n' > "$upstream/match-code-data/SKILL.md"
printf 'keep\n' > "$upstream/set-vscode-r-version/SKILL.md"
printf 'omit\n' > "$upstream/other-skill/SKILL.md"
git -C "$upstream" add .
git -C "$upstream" commit -qm 'Fixture'
mkdir "$test_root/project"
cd "$test_root/project"

# Reproduce the reported originless repository, retaining user files and commits.
git init -q skills
printf 'user content\n' > skills/custom.txt
git -C skills add .
git -C skills commit -qm 'User content'
original_head="$(git -C skills rev-parse HEAD)"
setup_sandbox_skills "$upstream" > "$test_root/recovery.log" 2>&1
grep -q 'Preserved it in skills.backup.' "$test_root/recovery.log"
backup=(skills.backup.*/skills)
[[ "$(git -C "${backup[0]}" rev-parse HEAD)" == "$original_head" ]]
[[ "$(cat "${backup[0]}/custom.txt")" == 'user content' ]]
[[ "$(git -C skills config --local --get remote.origin.url)" == "$upstream" ]]
[[ -f skills/match-code-data/SKILL.md && -f skills/set-vscode-r-version/SKILL.md && ! -e skills/other-skill ]]
setup_sandbox_skills "$upstream" >/dev/null
[[ "${#backup[@]}" == 1 ]]

# A wrong origin still fails without changing the repository.
if setup_sandbox_skills "$test_root/wrong-origin" > "$test_root/wrong.log" 2>&1; then exit 1; fi
grep -q 'different origin' "$test_root/wrong.log"
[[ "$(git -C skills config --local --get remote.origin.url)" == "$upstream" ]]

# Download failure leaves the originless repository and user data in place.
git -C skills remote remove origin
printf 'do not lose\n' > skills/local.txt
if setup_sandbox_skills "$test_root/missing" > "$test_root/failure.log" 2>&1; then exit 1; fi
[[ "$(cat skills/local.txt)" == 'do not lose' ]]
[[ -z "$(find . -maxdepth 1 -name '.sandbox-skills.*' -print)" ]]

# A corrupt Git configuration must be reported, not treated as a missing origin.
printf '\n[invalid\n' >> skills/.git/config
if setup_sandbox_skills "$upstream" > "$test_root/corrupt.log" 2>&1; then exit 1; fi
grep -q 'bad config' "$test_root/corrupt.log"
[[ "$(cat skills/local.txt)" == 'do not lose' ]]
echo 'Skills recovery, preservation, origin checks, and Git-error checks passed.'
