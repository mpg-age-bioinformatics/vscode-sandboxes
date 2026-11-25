#!/usr/bin/env bash
# Runs inside Apptainer, never on the login node.
set -euo pipefail
umask 077
node="$1"; job="$2"
run=/run/vscode-launcher
status="$run/runtime"
python3 -I - <<'PYSECURITY'
# VSCODE_VERIFY_SECURITY
PYSECURITY
network="$(python3 -I -c 'import json;print(json.load(open("/run/vscode-launcher/profile.json")).get("network") or "open")')"
bridge="";ssh_bridge=""
if [[ "$network" != open ]]; then
 python3 -I "$run/network.py" verify "$(cat "$run/host-network-namespace")"
 if [[ "$network" == restricted ]]; then
  proxy_port="$(python3 -I -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1])')"
  python3 -I "$run/network.py" proxy-bridge /run/vscode-network "$proxy_port" &
  bridge=$!
  export HTTP_PROXY="http://127.0.0.1:$proxy_port" HTTPS_PROXY="http://127.0.0.1:$proxy_port" ALL_PROXY="http://127.0.0.1:$proxy_port"
  export http_proxy="$HTTP_PROXY" https_proxy="$HTTPS_PROXY" all_proxy="$ALL_PROXY"
  export NO_PROXY=localhost,127.0.0.1,::1 no_proxy=localhost,127.0.0.1,::1 NODE_USE_ENV_PROXY=1
  # Readiness requires the actual host gateway; a failed bridge never enables
  # direct traffic. A deliberately denied request must return HTTP 403.
  python3 -I - "$proxy_port" <<'PYPROXY'
import socket,sys,time
for attempt in range(100):
 try:
  with socket.create_connection(('127.0.0.1',int(sys.argv[1])),timeout=2) as stream:
   stream.sendall(b'CONNECT denied.invalid:1 HTTP/1.1\r\nHost: denied.invalid:1\r\n\r\n')
   if not stream.recv(256).startswith(b'HTTP/1.1 403'):raise RuntimeError('Gateway policy check failed')
  break
 except OSError:
  if attempt==99:raise
  time.sleep(.1)
PYPROXY
 fi
fi
if [[ "$network" == none ]]; then
 unset HTTP_PROXY HTTPS_PROXY ALL_PROXY http_proxy https_proxy all_proxy NO_PROXY no_proxy NODE_USE_ENV_PROXY
fi
export VSCODE_SANDBOX_NETWORK="$network"
project="${3:-/workspace}"
cd -- "$project"
for tool in /usr/sbin/sshd ssh-keygen python3 node tar codex; do
  command -v "$tool" >/dev/null || { echo "Image is missing $tool" >&2; exit 1; }
done
printf '%s\n' 'Preparing project environment' > "$status/startup-phase"
if [[ -f "$run/initialize-project.sh" ]]; then bash "$run/initialize-project.sh" "$project";fi
printf '%s\n' 'Installing verified Continue extension inside the container' > "$status/startup-phase"
/usr/bin/python3 -I "$run/agent-network.py" "$run/profile.json"
echo 'Preparing pinned Continue extension in the remote VS Code extension home…'
if ! node "$run/install-continue.js" check; then
 bundle="$(python3 -I "$run/prepare-continue.py")"
 tar -C "$(dirname -- "$bundle")" -cf - "$(basename -- "$bundle")" | node "$run/install-continue.js" install
fi
printf '%s\n' 'Starting container SSH service' > "$status/startup-phase"
user="$(id -un)"
[[ "$user" != root ]] || { echo 'The remote service must run as your non-root HPC account.' >&2; exit 1; }
mkdir -p "$HOME/.vscode-server" "$HOME/.codex" "$HOME/.claude"
chmod 700 "$HOME/.codex" "$HOME/.claude"
# VS Code otherwise inherits the HPC passwd entry, which can be /bin/sh.
# Configure allocation-local remote settings, preserving explicit user choices.
python3 -I - <<'PYEDITOR'
import json,os,pathlib,tempfile
for name in ('.vscode-server','.vscode-server-insiders'):
    target=pathlib.Path.home()/name/'data/Machine/settings.json'
    target.parent.mkdir(parents=True,exist_ok=True)
    settings=json.loads(target.read_text()) if target.exists() else {}
    profiles=settings.setdefault('terminal.integrated.profiles.linux',{})
    profiles.setdefault('Sandbox Bash',{'path':'/bin/bash','args':['-l']})
    settings.setdefault('terminal.integrated.defaultProfile.linux','Sandbox Bash')
    settings.setdefault('terminal.integrated.cwd','${workspaceFolder}')
    if os.environ.get('HTTP_PROXY'):
        settings['http.proxy']=os.environ['HTTP_PROXY']
        settings['http.proxySupport']='override'
        settings['http.noProxy']=['localhost','127.0.0.1','::1']
    with tempfile.NamedTemporaryFile(mode='w',dir=target.parent,delete=False) as stream:
        json.dump(settings,stream,indent=2);stream.write('\n');temporary=stream.name
    os.replace(temporary,target)
PYEDITOR
if [[ ! -f "$status/host_key" ]]; then ssh-keygen -q -t ed25519 -N '' -f "$status/host_key";fi
port="$(python3 -I -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')"
network_env=""
if [[ "$network" != open ]]; then
 network_env="VSCODE_SANDBOX_NETWORK=$network"
 if [[ "$network" == restricted ]]; then
  network_env+=" HTTP_PROXY=$HTTP_PROXY HTTPS_PROXY=$HTTPS_PROXY ALL_PROXY=$ALL_PROXY http_proxy=$http_proxy https_proxy=$https_proxy all_proxy=$all_proxy NO_PROXY=localhost,127.0.0.1,::1 no_proxy=localhost,127.0.0.1,::1 NODE_USE_ENV_PROXY=1"
 fi
fi
cat > "$status/sshd_config" <<CONFIG
Port $port
ListenAddress 127.0.0.1
HostKey $status/host_key
PidFile $status/sshd.pid
AuthorizedKeysFile $run/authorized_keys
StrictModes yes
PasswordAuthentication no
KbdInteractiveAuthentication no
AuthenticationMethods publickey
PubkeyAuthentication yes
UsePAM no
PermitRootLogin no
PermitUserRC yes
ClientAliveInterval 60
ClientAliveCountMax 3
LoginGraceTime 60
AllowUsers $user
AllowTcpForwarding local
AllowAgentForwarding no
X11Forwarding no
PermitTunnel no
GatewayPorts no
SetEnv PATH=$HOME/.local/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin VSCODE_SANDBOX_REMOTE_CONTAINER=$job SHELL=/bin/bash TMPDIR="$TMPDIR" TMP="$TMPDIR" TEMP="$TMPDIR" $network_env
Subsystem sftp internal-sftp
CONFIG
/usr/sbin/sshd -t -f "$status/sshd_config"
/usr/sbin/sshd -D -e -f "$status/sshd_config" &
pid=$!
trap 'rm -f "$status/ready"; kill "$pid" ${bridge:+"$bridge"} ${ssh_bridge:+"$ssh_bridge"} 2>/dev/null || true' EXIT
trap 'exit 143' TERM HUP INT
ready=0
for ((attempt=0;attempt<30;attempt++)); do
  kill -0 "$pid" || { echo 'Container SSH service exited.' >&2; exit 1; }
  if python3 -I - "$port" <<'PY'
import socket,sys
with socket.create_connection(('127.0.0.1',int(sys.argv[1])),timeout=1) as stream:
    if not stream.recv(128).startswith(b'SSH-'): sys.exit(1)
PY
  then ready=1;break;fi
  sleep 1
done
[[ "$ready" == 1 ]] || { echo 'Container SSH service did not become ready.' >&2; exit 1; }
if [[ "$network" != open ]]; then
 python3 -I "$run/network.py" ssh-bridge /run/vscode-ssh "$port" &
 ssh_bridge=$!
 for ((attempt=0;attempt<100;attempt++)); do
  kill -0 "$ssh_bridge" || { echo 'SSH socket bridge failed.' >&2; exit 1; }
  [[ ! -S /run/vscode-ssh/ssh.sock ]] || break
  sleep 0.1
 done
 [[ -S /run/vscode-ssh/ssh.sock ]] || { echo 'SSH socket bridge did not become ready.' >&2; exit 1; }
fi
if [[ -f "$run/watchdog.py" ]]; then
 python3 -I "$run/watchdog.py" server "$run" "$pid"
fi
printf 'HOST=%s\nPORT=%s\nUSER=%s\nJOB=%s\nPROJECT=%s\n' "$node" "$port" "$user" "$job" "$project" > "$status/ready.tmp"
cat "$status/host_key.pub" >> "$status/ready.tmp"
mv "$status/ready.tmp" "$status/ready"
wait "$pid"
