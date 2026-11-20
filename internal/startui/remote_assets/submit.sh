#!/usr/bin/env bash
set -euo pipefail
umask 077
# VSCODE_SECURITY_HELPERS
state="$1"; token="$2"; project="$3"; folders="$4"; image="$5"; partition="$6"; cpus="$7"; memory="$8"; time_limit="$9"; shift 9
gpus="$1"; apptainer="$2"; public_key="$3"; modules="$4"; job_script="$5"; container_script="$6"; stage="${7:-}"; initialize_script="${8:-}"; relay_script="${9:-}"; profile_metadata="${10:-}"; cache_id="${11:-}"; watchdog_script="${12:-}"
# Module initialization applies to login-node submission and is saved for the job.
module_file="$(mktemp)"
trap 'rm -f "$module_file"' EXIT
printf '%s' "$modules" | base64 -d > "$module_file"
source "$module_file"
case "$project" in '~/'*) project="$HOME/${project:2}";; esac
case "$state" in '~/'*) state="$HOME/${state:2}";; esac
for command in sbatch squeue scancel base64; do command -v "$command" >/dev/null || { echo "Missing cluster command: $command" >&2; exit 1; };done
run="$state/runs/$token"
security_mounts "$folders"
mkdir -p "$project"
project="$(cd "$project" && pwd -L)"
[[ -d "$project" && ( -f "$image" || ( -n "$stage" && -f "$stage/image.tar" ) ) ]] || { echo 'The project directory and image must already exist on the cluster.' >&2; exit 1; }
mkdir -p "$state/runs"
state="$(cd "$state" && pwd -P)"
run="$state/runs/$token"
mkdir "$run"
printf '%s' "$public_key" | base64 -d > "$run/authorized_keys"
cp "$module_file" "$run/modules.sh"
: > "$run/additional-folders"
IFS=',' read -r -a folder_list <<< "$folders"
for folder in ${folder_list[@]+"${folder_list[@]}"}; do
 folder="${folder#"${folder%%[![:space:]]*}"}"
 folder="${folder%"${folder##*[![:space:]]}"}"
 [[ -n "$folder" ]] || continue
 case "$folder" in '~/'*) folder="$HOME/${folder:2}";; esac
 [[ "$folder" == /* && -d "$folder" ]] || { echo "Additional folder does not exist on the cluster: $folder" >&2; exit 1; }
 project_real="$(cd "$project" && pwd -P)"
 folder_real="$(cd "$folder" && pwd -P)"
 [[ "$folder_real" != "$project_real" && "$folder_real" != "$project_real/"* && "$project_real" != "$folder_real/"* ]] || { echo 'Additional read-only folders must not overlap the project directory.' >&2; exit 1; }
 grep -Fxq -- "$folder" "$run/additional-folders" || printf '%s\n' "$folder" >> "$run/additional-folders"
done
printf '%s' "$job_script" | base64 -d > "$run/job.sh"
printf '%s' "$container_script" | base64 -d > "$run/container.sh"
printf '%s' "$relay_script" | base64 -d > "$run/relay.sh"
if [[ -n "$watchdog_script" ]]; then
 printf '%s' "$watchdog_script" | base64 -d > "$run/watchdog.py"
 chmod 600 "$run/watchdog.py"
fi
printf '%s' "$profile_metadata" | base64 -d > "$run/profile.json"
if [[ -n "$stage" ]]; then
 [[ -f "$stage/project.tar" && ( -s "$image" || -f "$stage/image.tar" ) ]] || { echo 'Staged image/project upload is incomplete.' >&2; exit 1; }
 cp "$stage/project.tar" "$run/project.tar"
 printf '%s' "$initialize_script" | base64 -d > "$run/initialize-project.sh"
fi
chmod 600 "$run/authorized_keys" "$run/modules.sh" "$run/job.sh" "$run/container.sh" "$run/relay.sh"
printf '%s\n' "$token" > "$run/owner-token"
printf '%s\n' "$project" > "$run/project-path"
printf '%s\n' "$image" > "$run/image"
printf '%s\n' "$apptainer" > "$run/apptainer"
args=(--parsable --nodes=1 --ntasks=1 --cpus-per-task="$cpus" --mem="$memory" --time="$time_limit" --job-name=vscode-sandboxes --comment="vscode-sandboxes:$token" --output="$run/slurm-%j.log" --chdir="$project")
[[ -z "$partition" ]] || args+=(--partition="$partition")
(( gpus == 0 )) || args+=(--gres="gpu:$gpus")
printf 'RUN=%s\n' "$run"
# Always use the logged-in user's Slurm default account.
unset SBATCH_ACCOUNT
job="$(sbatch "${args[@]}" "$run/job.sh" "$run" "$state" "$project" "" "$image" "$apptainer" "$gpus" "$stage" "$cache_id")"
job="${job%%;*}"
[[ "$job" =~ ^[0-9]+$ ]] || { echo 'Slurm did not return a numeric job ID; inspect your queue before retrying.' >&2; exit 1; }
printf '%s\n' "$job" > "$run/job-id"
printf 'JOB=%s\n' "$job"
# The singleton monitor lives on the login server, outside this SSH session.
if [[ -n "$watchdog_script" ]]; then
 if ! python3 "$run/watchdog.py" register "$run" "$token" "$job"; then
  current="$(squeue --noheader --jobs "$job" --format='%i|%k')"
  if [[ "$current" == "$job|vscode-sandboxes:$token" ]]; then scancel "$job"; fi
  echo 'Could not start the server-side idle watchdog; stopped the new allocation.' >&2
  exit 1
 fi
fi
