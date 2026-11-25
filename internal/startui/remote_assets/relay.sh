#!/usr/bin/env bash
# Runs on the login node. Only Slurm transports bytes to the compute node.
set -euo pipefail
# VSCODE_SECURITY_HELPERS
run="$1"; token="$2"; job="$3"; port="$4"
[[ "$job" =~ ^[0-9]+$ && "$port" =~ ^[0-9]+$ && "$port" -ge 1024 && "$port" -le 65535 ]] || exit 1
[[ "$(cat "$run/owner-token")" == "$token" && "$(cat "$run/job-id")" == "$job" ]] || { echo 'Allocation identity mismatch.' >&2; exit 1; }
source "$run/modules.sh" >&2
identity="$(squeue --noheader --jobs "$job" --format='%i|%k')"
[[ "$identity" == "$job|vscode-sandboxes:$token" ]] || { echo 'Allocation ended or belongs to another job.' >&2; exit 1; }
[[ "$(sed -n 's/^PORT=//p' "$run/ready")" == "$port" ]] || exit 1
image="$(cat "$run/image")"; apptainer="$(cat "$run/apptainer")"
args=(--jobid="$job" --nodes=1 --ntasks=1 --cpus-per-task=1 --quiet --mpi=none --unbuffered)
help="$(srun --help 2>&1)"
# Old Slurm shares steps by default; newer Slurm requires explicit overlap.
[[ "$help" != *--overlap* ]] || args+=(--overlap)
[[ "$help" != *--exact* ]] || args+=(--exact)
# Slurm otherwise line-buffers task output. Its unbuffered mode uses a PTY;
# the relay switches it to raw mode before sending any SSH bytes.
unset SLURM_UNBUFFEREDIO SLURM_LABELIO
python="$(cat <<'PY'
import os,socket,sys,threading,termios,tty
for fd in (0,1):
    if os.isatty(fd): tty.setraw(fd,termios.TCSANOW)
if len(sys.argv)>2:
    os.chdir(sys.argv[2]);stream=socket.socket(socket.AF_UNIX);stream.settimeout(20);stream.connect('ssh.sock')
else: stream=socket.create_connection(('127.0.0.1',int(sys.argv[1])),timeout=20)
stream.settimeout(None)
def upload():
    try:
        while True:
            data=os.read(0,65536)
            if not data: break
            stream.sendall(data)
        stream.shutdown(socket.SHUT_WR)
    except OSError: pass
threading.Thread(target=upload,daemon=True).start()
try:
    while True:
        data=stream.recv(65536)
        if not data: break
        view=memoryview(data)
        while view:
            count=os.write(1,view)
            view=view[count:]
finally: stream.close()
PY
)"
if [[ -f "$run/ssh-socket-directory" ]]; then
 directory="$(cat "$run/ssh-socket-directory")"
 [[ "$directory" == /* ]] || exit 1
 # Trusted fixed byte-forwarder only, outside the workspace. The socket is
 # reachable across network namespaces; no host TCP listener is created.
 exec srun "${args[@]}" env -i PATH="$PATH" HOME="$HOME" USER="$(id -un)" python3 -I -u -c "$python" "$port" "$directory"
fi
# Same kernel policy on the transport helper; it receives no project/home binds.
# env -i prevents host bind/environment switches from reopening the host filesystem.
exec srun "${args[@]}" env -i PATH="$PATH" HOME="$HOME" USER="$(id -un)" \
 "$apptainer" exec --cleanenv --containall --no-init --no-eval --no-privs --drop-caps ALL \
 --no-mount hostfs,cwd,bind-paths,sys --security "seccomp:$run/seccomp-agents.json" "$image" python3 -I -u -c "$python" "$port"
