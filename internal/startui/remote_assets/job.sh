#!/usr/bin/env bash
set -euo pipefail
umask 077
run="$1"; state="$2"; project="$3"; legacy_data="$4"; image="$5"; apptainer="$6"; gpus="$7"; stage="${8:-}"; cache_id="${9:-}"
# VSCODE_SECURITY_HELPERS
# Load the same requested modules in the compute-node batch shell.
mkdir -p "$run/runtime"
for name in ready startup-phase host_key host_key.pub sshd_config sshd.pid ssh-server.json ssh-activity.json ssh-activity.lock; do
 ln -sfn "runtime/$name" "$run/$name"
done
rm -f "$run/runtime/ready"
security_assets
# VSCODE_CONTINUE_ASSETS
# VSCODE_NETWORK_ASSETS
cat > "$run/bash-prompt.sh" <<'VSCODE_PROMPT_SCRIPT'
# VSCODE_BASH_PROMPT
VSCODE_PROMPT_SCRIPT
mount_folders="$legacy_data"
if [[ -f "$run/additional-folders" ]]; then mount_folders="$(paste -sd, - < "$run/additional-folders")";fi
security_mounts "$mount_folders"
printf '%s\n' 'Starting Apptainer container' > "$run/startup-phase"
source "$run/modules.sh"
command -v "$apptainer" >/dev/null || { echo 'Apptainer is unavailable after module setup.' >&2; exit 1; }
# Honor the compute-node environment (including site prologs/modules).
base="${SLURM_TMPDIR:-${TMPDIR:-${TMP:-${TEMP:-}}}}"
if [[ -n "$base" ]]; then
 [[ "$base" == /* && -d "$base" && -w "$base" ]] || { echo 'The cluster temporary directory is not an absolute writable directory.' >&2; exit 1; }
 temporary="$(mktemp -d "$base/vscode-sandboxes.$SLURM_JOB_ID.XXXXXX")"
else
 # mktemp chooses the server's OS default when the site sets no TMP variables.
 temporary="$(mktemp -d)"
fi
child=""
gateway=""
cleanup() {
 if [[ -n "$gateway" ]]; then kill "$gateway" 2>/dev/null || true; wait "$gateway" 2>/dev/null || true; fi
 # renv deliberately makes sandbox cache directories read-only. Restore owner
 # traversal/write rights only within this allocation's disposable tree.
 # find does not follow cached-library symlinks into the image or project.
 find "$temporary" -type d -user "$(id -un)" -exec chmod u+rwx {} + 2>/dev/null || true
 rm -rf -- "$temporary"
}
trap cleanup EXIT
trap 'if [[ -n "$child" ]]; then kill -TERM "$child" 2>/dev/null || true; wait "$child" 2>/dev/null || true; fi; exit 143' TERM HUP INT
case "$temporary" in
 *:*|*,*|*'"'*|*'\'*|*$'\n'*|*$'\r'*|*$'\t'*) echo 'Unsupported temporary-directory path.' >&2; exit 1;;
esac
mkdir -p "$temporary/files" "$temporary/runtime" "$temporary/build"
printf '%s\n' "$temporary" > "$run/temporary-directory"
export TMPDIR="$temporary/files" TMP="$temporary/files" TEMP="$temporary/files"
export APPTAINER_TMPDIR="${APPTAINER_TMPDIR:-$temporary/build}"
if [[ -n "$stage" ]]; then
 convert_image() {
  if [[ ! -s "$image" ]]; then
   printf '%s\n' 'Converting image into server cache' > "$run/startup-phase"
   echo 'Converting the image once into the shared server cache...'
   secure_apptainer build "$image.tmp.$SLURM_JOB_ID" "docker-archive:$stage/image.tar"
   mv "$image.tmp.$SLURM_JOB_ID" "$image"
  fi
  if [[ -n "$cache_id" ]]; then
   [[ "$cache_id" =~ ^[0-9a-f]{64}$ ]] || exit 1
   printf '%s\n' "$cache_id" > "$(dirname "$image")/ready.tmp.$SLURM_JOB_ID"
   mv "$(dirname "$image")/ready.tmp.$SLURM_JOB_ID" "$(dirname "$image")/ready"
  fi
  rm -f "$stage/image.tar"
 }
 if [[ -n "$cache_id" ]]; then
  (flock -x 9; convert_image) 9>"$image.lock"
 else
  convert_image
 fi
fi
[[ -d "$project" && -f "$image" ]] || { echo 'Project/image is not accessible on this compute node.' >&2; exit 1; }
node="${SLURMD_NODENAME:-allocated-node}"
user="$(id -un)"
passwd_home="$(getent passwd "$user" | cut -d: -f6)"
[[ "$passwd_home" == /* && "$passwd_home" != / && "$passwd_home" != *:* && "$passwd_home" != *,* && "$passwd_home" != *' '* ]] || { echo 'Unsupported account home path.' >&2; exit 1; }
# Container home is allocation-local; only the project is writable persistent storage.
[[ "$project" != "$passwd_home" && "$passwd_home" != "$project/"* ]] || { echo 'Choose a project subfolder rather than mounting the account home or its parent.' >&2; exit 1; }
mkdir -p "$temporary/home"
printf '%s\n' "$project" > "$run/project-path"
for startup in .profile .bashrc; do
 cat > "$temporary/home/$startup" <<'PROFILE'
if [ -r /run/vscode-launcher/project-path ]; then
 cd -- "$(cat /run/vscode-launcher/project-path)" || exit
fi
# Filename completion works even with older cached images. Rich command
# completion is enabled when bash-completion is included in the image.
if [ -n "${BASH_VERSION:-}" ]; then
 case $- in
  *i*)
   bind 'set disable-completion off'
   bind '"\C-i": complete'
   if [ -r /usr/share/bash-completion/bash_completion ]; then
    . /usr/share/bash-completion/bash_completion
   fi
   if [ -r /run/vscode-launcher/bash-prompt.sh ]; then
    . /run/vscode-launcher/bash-prompt.sh
   fi
   ;;
 esac
fi
PROFILE
done
if [[ -f "$run/watchdog.py" ]]; then
 mkdir -p "$temporary/home/.ssh"
 cat > "$temporary/home/.ssh/rc" <<'SSHRC'
# OpenSSH invokes this after authentication; never write to stdout.
python3 /run/vscode-launcher/watchdog.py connected /run/vscode-launcher >/dev/null
SSHRC
 chmod 700 "$temporary/home/.ssh"; chmod 600 "$temporary/home/.ssh/rc"
fi
# Apptainer backs its standard temporary locations with the site-selected space.
# R rejects spaces in R_TempDir. Only the container alias changes in that case;
# all temporary files still live in the directory selected by the server.
container_tmp="$TMPDIR"
if [[ "$container_tmp" == *' '* ]]; then
 container_tmp=/run/vscode-launcher/runtime/tmp
 mkdir -p "$run/runtime/tmp"
fi
# Restricted workspaces have no external interfaces. The gateway lives in the
# batch shell's network/PID namespace and sees only an immutable allowlist.
network="$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1])).get("network") or "open")' "$run/profile.json")"
[[ "$network" == open || "$network" == restricted || "$network" == none ]] || { echo 'Invalid internet policy.' >&2; exit 1; }
network_args=()
if [[ "$network" != open ]]; then
 printf '%s\n' 'Provisioning editor without project/data access' > "$run/startup-phase"
 mkdir -p "$temporary/provision" "$temporary/provision-runtime" "$temporary/ssh" "$temporary/network"
 for asset in prepare-editor.py prepare-continue.py install-continue.js; do cp "$run/$asset" "$temporary/provision/$asset"; done
 python3 - "$run/profile.json" "$temporary/provision/profile.json" <<'PYPROFILE'
import json,sys
profile=json.load(open(sys.argv[1]))
with open(sys.argv[2],'w') as output:json.dump({key:profile.get(key) for key in ('editorCommit','editorExtensions')},output)
PYPROFILE
 # No project, additional folders, keys, module scripts or run directory are
 # exposed to this trusted helper. It shares only the fresh editor home.
 secure_apptainer exec --cleanenv --containall --no-init --no-eval --no-privs --drop-caps ALL --no-mount hostfs,cwd,bind-paths,sys --security "seccomp:$run/seccomp-agents.json" --workdir "$temporary/provision-runtime" --home "$temporary/home:$passwd_home" --bind "$temporary/provision:/run/vscode-launcher:ro" --bind "$TMPDIR:$container_tmp:rw" --env "TMPDIR=$container_tmp,TMP=$container_tmp,TEMP=$container_tmp" --pwd / "$image" python3 /run/vscode-launcher/prepare-editor.py &
 child=$!
 wait "$child"
 child=""
 readlink /proc/self/ns/net > "$run/host-network-namespace"
 printf '%s\n' "$temporary/ssh" > "$run/ssh-socket-directory"
 network_args=(--net --network none --bind "$temporary/ssh:/run/vscode-ssh:rw")
 if [[ "$network" == restricted ]]; then
  python3 "$run/network.py" gateway "$run/profile.json" "$temporary/network" &
  gateway=$!
  for ((attempt=0;attempt<100;attempt++)); do
   kill -0 "$gateway" || { echo 'Allowlist gateway failed.' >&2; exit 1; }
   [[ ! -S "$temporary/network/proxy.sock" ]] || break
   sleep 0.1
  done
  [[ -S "$temporary/network/proxy.sock" ]] || { echo 'Allowlist gateway did not become ready.' >&2; exit 1; }
  network_args+=(--bind "$temporary/network:/run/vscode-network:ro")
 fi
fi
args=(exec --cleanenv --containall --no-init --no-eval --no-privs --drop-caps ALL --no-mount hostfs,cwd,bind-paths,sys --security "seccomp:$run/seccomp-agents.json" --workdir "$temporary/runtime" --home "$temporary/home:$passwd_home" --bind "$(cd "$project" && pwd -P):$project:rw" --bind "$run:/run/vscode-launcher:ro" --bind "$run/runtime:/run/vscode-launcher/runtime:rw" --bind "$TMPDIR:$container_tmp:rw" --env "TMPDIR=$container_tmp,TMP=$container_tmp,TEMP=$container_tmp" --pwd "$project")
if (( ${#network_args[@]} > 0 )); then args+=("${network_args[@]}"); fi
if [[ -f "$run/additional-folders" ]]; then
 while IFS= read -r folder || [[ -n "$folder" ]]; do
  [[ -n "$folder" ]] || continue
  [[ "$folder" != "$passwd_home" ]] || { echo 'Mount a subfolder rather than replacing the container home.' >&2; exit 1; }
  [[ -d "$folder" ]] || { echo "Additional folder is not accessible on this compute node: $folder" >&2; exit 1; }
  project_real="$(cd "$project" && pwd -P)"
  folder_real="$(cd "$folder" && pwd -P)"
  [[ "$folder_real" != "$project_real" && "$folder_real" != "$project_real/"* && "$project_real" != "$folder_real/"* ]] || { echo 'Additional read-only folders must not overlap the project directory.' >&2; exit 1; }
  args+=(--bind "$folder_real:$folder:ro")
 done < "$run/additional-folders"
elif [[ -n "$legacy_data" ]]; then
 # Compatibility for batch-script fixtures and older recorded allocations.
 args+=(--bind "$legacy_data:/data:ro")
fi
if (( gpus > 0 )); then
 args+=(--nv)
 for variable in CUDA_VISIBLE_DEVICES NVIDIA_VISIBLE_DEVICES ROCR_VISIBLE_DEVICES HIP_VISIBLE_DEVICES GPU_DEVICE_ORDINAL; do
  if gpu_value="$(printenv "$variable")"; then args+=(--env "$variable=$gpu_value");fi
 done
fi
printf '%s\n' "$image" > "$run/image"
printf '%s\n' "$apptainer" > "$run/apptainer"
# Keep the batch shell to forward termination and clean its temporary subfolder.
secure_apptainer "${args[@]}" "$image" /bin/bash /run/vscode-launcher/container.sh "$node" "$SLURM_JOB_ID" "$project" &
child=$!
wait "$child"
