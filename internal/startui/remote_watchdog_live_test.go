package startui

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Run the deterministic suite with the actual HPC login node's Python and flock.
// Its fake scheduler and process fixtures live in an isolated temporary tree.
func TestRemoteWatchdogLinuxHPC(t *testing.T) {
	host := os.Getenv("VSCODE_REMOTE_WATCHDOG_HPC")
	if host == "" {
		t.Skip("set VSCODE_REMOTE_WATCHDOG_HPC")
	}
	tests, err := os.ReadFile("../../scripts/test-remote-watchdog.py")
	if err != nil {
		t.Fatal(err)
	}
	ctx, done := context.WithTimeout(context.Background(), time.Minute)
	defer done()
	script := `python3 - "$1" "$2" <<'PY'
import base64,pathlib,subprocess,sys,tempfile
with tempfile.TemporaryDirectory(prefix='vscode-watchdog-tests-') as temporary:
 root=pathlib.Path(temporary)
 for name,encoded in [('scripts/test-remote-watchdog.py',sys.argv[1]),('internal/startui/remote_assets/watchdog.py',sys.argv[2])]:
  path=root/name;path.parent.mkdir(parents=True,exist_ok=True);path.write_bytes(base64.b64decode(encoded))
 result=subprocess.run([sys.executable,str(root/'scripts/test-remote-watchdog.py')],stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True,timeout=45)
 print(result.stdout,end='')
 sys.exit(result.returncode)
PY`
	out, err := loginSSH(ctx, host, script, base64.StdEncoding.EncodeToString(tests), base64.StdEncoding.EncodeToString([]byte(remoteAsset("watchdog.py"))))
	if err != nil {
		t.Fatal(out, err)
	}
	t.Log(out)
}

// Explicitly opted-in, disposable allocations only. Accelerate their idle clocks,
// never the production timeout, to test the real singleton and Slurm cancellation.
func TestRemoteWatchdogLiveHPC(t *testing.T) {
	testRemoteAllocationsLiveHPC(t, false)
}

func TestRemoteContinueBootstrapLiveHPC(t *testing.T) {
	testRemoteAllocationsLiveHPC(t, true)
}

func TestRemoteNetworkLiveHPC(t *testing.T) {
	t.Setenv("VSCODE_REMOTE_NETWORK_TEST", "1")
	testRemoteAllocationsLiveHPC(t, true)
}

func testRemoteAllocationsLiveHPC(t *testing.T, bootstrapOnly bool) {
	t.Helper()
	host := os.Getenv("VSCODE_REMOTE_WATCHDOG_HPC")
	image := os.Getenv("VSCODE_REMOTE_WATCHDOG_SIF")
	if host == "" || image == "" {
		t.Skip("set VSCODE_REMOTE_WATCHDOG_HPC and VSCODE_REMOTE_WATCHDOG_SIF")
	}
	ctx, done := context.WithTimeout(context.Background(), 20*time.Minute)
	defer done()
	enc := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	controller := &remoteController{ssh: loginSSH}
	var sessions []RemoteSession
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		for _, s := range sessions {
			if s.JobID != "" {
				_, _ = loginSSH(cleanup, host, remoteAsset("cancel.sh"), s.RunDir, s.Token, s.JobID)
			}
			os.RemoveAll(remoteSessionDir(s.Token))
			os.Remove(filepath.Join(settingsDir(), "remote-ssh", s.Token+".conf"))
			if os.Getenv("VSCODE_REMOTE_SECURITY_TEST") == "1" {
				_, _ = loginSSH(cleanup, host, `python3 - "$1" <<'PYCLEAN'
import pathlib,shutil,sys
for path in [pathlib.Path.home()/('.vscode-security-fixture-'+sys.argv[1]),pathlib.Path.home()/'sandboxes'/('security-readonly-'+sys.argv[1])]:
 if path.is_dir():shutil.rmtree(path)
PYCLEAN`, s.Token)
			}
		}
	}()
	for n := 0; n < 2; n++ {
		selectedImage := image
		if n == 1 && os.Getenv("VSCODE_REMOTE_WATCHDOG_SECOND_SIF") != "" {
			selectedImage = os.Getenv("VSCODE_REMOTE_WATCHDOG_SECOND_SIF")
		}
		p := defaultRemoteProfile()
		p.Login = host
		p.ImageSource = "sif"
		p.Image = selectedImage
		p.Memory = "2G"
		p.Time = "00:20:00"
		if os.Getenv("VSCODE_REMOTE_NETWORK_TEST") == "1" {
			p.Memory = "4G"
			p.Network = "restricted"
			if n == 1 {
				p.Network = "none"
			}
			p.Sites = "pypi.org, files.pythonhosted.org"
			if err := prepareRemoteEditorProfile(ctx, &p); err != nil {
				t.Fatal(err)
			}
		}
		token := fmt.Sprintf("%032x", time.Now().UnixNano())
		p.Project = "~/sandboxes/watchdog-test-" + token[16:]
		if os.Getenv("VSCODE_REMOTE_SECURITY_TEST") == "1" {
			p.Folders = "~/sandboxes/security-readonly-" + token
			p.Modules = `export VSCODE_SECURITY_TEST_SECRET=not-a-real-credential
export APPTAINERENV_VSCODE_SECURITY_INJECTED=not-a-real-credential
export APPTAINER_BINDPATH="$HOME/.vscode-security-fixture-` + token + `:/host-leak:ro"
export SINGULARITY_BINDPATH="$APPTAINER_BINDPATH"`
			if out, err := loginSSH(ctx, host, `set -eu; umask 077; mkdir -p "$HOME/.vscode-security-fixture-$1" "$HOME/sandboxes/security-readonly-$1"; printf '%s' "$1" > "$HOME/.vscode-security-fixture-$1/marker"; printf '%s' "$1" > "$HOME/sandboxes/security-readonly-$1/marker"`, token); err != nil {
				t.Fatal(out, err)
			}
		}
		s := RemoteSession{Profile: p, Token: token, Active: true}
		dir := remoteSessionDir(token)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		keygen, err := hostSSHBinary("ssh-keygen")
		if err != nil {
			t.Fatal(err)
		}
		if out, err := exec.CommandContext(ctx, keygen, "-q", "-t", "ed25519", "-N", "", "-f", filepath.Join(dir, "id_ed25519")).CombinedOutput(); err != nil {
			t.Fatal(string(out), err)
		}
		key, err := os.ReadFile(filepath.Join(dir, "id_ed25519.pub"))
		if err != nil {
			t.Fatal(err)
		}
		out, err := loginSSH(ctx, host, remoteAsset("submit.sh"), p.StateDir, token, p.Project, p.Folders, selectedImage, p.Partition, "1", p.Memory, p.Time, "0", p.Apptainer, enc(string(key)), enc(remoteModuleScript(p)), enc(remoteAsset("job.sh")), enc(remoteAsset("container.sh")), "", "", enc(remoteAsset("relay.sh")), enc(remoteProfileJSON(p)), "", enc(remoteAsset("watchdog.py")))
		v := parseRemoteValues(out)
		s.RunDir = v["RUN"]
		s.JobID = v["JOB"]
		sessions = append(sessions, s)
		if err != nil || s.JobID == "" {
			t.Fatal("submit", out, err)
		}
		for {
			out, err = loginSSH(ctx, host, remoteAsset("probe.sh"), s.RunDir, token, s.JobID)
			if err != nil {
				t.Fatal(out, err)
			}
			v = parseRemoteValues(out)
			if v["STATE"] == "ENDED" {
				t.Fatal(out)
			}
			if v["STATE"] == "READY" {
				break
			}
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-time.After(3 * time.Second):
			}
		}
		anchored, anchorErr := loginSSH(ctx, host, `python3 -I - "$1" "$2" "$3" <<'PYANCHOR'
import json,pathlib,subprocess,sys,time
run=pathlib.Path(sys.argv[1])
for attempt in range(100):
 if (run/'server-identity.json').exists():break
 time.sleep(.1)
identity=json.loads((run/'server-identity.json').read_text())
assert identity['token']==sys.argv[2] and identity['jobID']==sys.argv[3]
assert identity['readyAt']<=time.time()
# The actual host probe must accept the anchored PID/namespace/executable.
help_text=subprocess.check_output(['srun','--help'],text=True)
options=[flag for flag in ('--overlap','--exact') if flag in help_text]
observation=json.loads(subprocess.check_output(['srun','--jobid='+sys.argv[3],'--nodes=1','--ntasks=1','--cpus-per-task=1','--mpi=none']+options+['python3','-I',str(run/'watchdog.py'),'probe',str(run),sys.argv[2],sys.argv[3]],text=True))
assert observation['hostAnchored'] is True
print('HOST_ANCHORED_SSH_IDENTITY_VERIFIED')
PYANCHOR`, s.RunDir, token, s.JobID)
		if anchorErr != nil {
			t.Fatal("host anchor", anchored, anchorErr)
		}
		t.Log(anchored)
		s.User = v["USER"]
		s.Node = v["HOST"]
		s.ProjectPath = v["PROJECT"]
		s.Port, _ = strconv.Atoi(v["PORT"])
		for _, line := range strings.Split(out, "\n") {
			if strings.HasPrefix(line, "ssh-ed25519 ") {
				if err := prepareRemoteSSH(s, line); err != nil {
					t.Fatal(err)
				}
				break
			}
		}
		sessions[n] = s
		if _, err := loginSSH(ctx, "vscode-sandboxes-"+s.Token, "true"); err != nil {
			t.Fatal("authenticated connection", err)
		}
		event, err := loginSSH(ctx, host, `python3 -I - "$1" "$2" "$3" <<'PYCLIENT'
import json,pathlib,sys,time
record=json.loads((pathlib.Path(sys.argv[1])/'host-activity.json').read_text())
assert record['token']==sys.argv[2] and record['jobID']==sys.argv[3]
assert 0<=time.time()-record['lastAuthenticatedAt']<60
print('HOST_AUTHENTICATION_EVENT_VERIFIED')
PYCLIENT`, s.RunDir, s.Token, s.JobID)
		if err != nil {
			t.Fatal("host authentication event", event, err)
		}
		t.Log(event)
		if bootstrapOnly && os.Getenv("VSCODE_REMOTE_NETWORK_TEST") != "1" {
			out, err := loginSSH(ctx, "vscode-sandboxes-"+s.Token, `set -eu
node /run/vscode-launcher/install-continue.js check
python3 - <<'PY'
import json,pathlib
root=pathlib.Path.home()/'.vscode-server/extensions/continue.continue-2.0.0-linux-x64'
manifest=json.loads((root/'package.json').read_text())
assert manifest['version']=='2.0.0'
assert manifest['__metadata']['pinned'] is True
assert (root/'.sandbox-continue-policy.json').is_file()
assert not (pathlib.Path.home()/'.vscode-server/cli').exists(),'VS Code already bootstrapped before extension check'
print('PINNED_CONTINUE_INSTALLED_BEFORE_VSCODE_SERVER')
PY`)
			if err != nil {
				t.Fatal(out, err)
			}
			t.Log(strings.TrimSpace(out))
		}
		if os.Getenv("VSCODE_REMOTE_SECURITY_TEST") == "1" {
			verifyRemoteSecurityLive(t, ctx, s)
		}
		if os.Getenv("VSCODE_REMOTE_NETWORK_TEST") == "1" {
			verifyRemoteNetworkLive(t, ctx, s)
		}
		t.Log("READY", s.JobID)
	}
	if bootstrapOnly {
		return
	}
	checkSingleton := `python3 - "$1" "$2" <<'PY'
import pathlib,os,json,sys,fcntl
root=pathlib.Path.home()/'.local/share/vscode-sandboxes/watchdog'
with (root/'registry.lock').open('a') as lock:
 fcntl.flock(lock,fcntl.LOCK_EX)
 entries=json.loads((root/'allocations.json').read_text())
 assert all(t in entries for t in sys.argv[1:]),entries
workers=[]
for path in pathlib.Path('/proc').iterdir():
 try:
  if path.name.isdigit() and path.stat().st_uid==os.getuid():
   args=(path/'cmdline').read_bytes().split(b'\0')
   if any(a.endswith(b'/watchdog.py') for a in args) and b'daemon' in args:workers.append(int(path.name))
 except OSError:pass
assert len(workers)==1,workers
with (root/'daemon.lock').open('a') as lock:
 try:fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
 except BlockingIOError:pass
 else:raise AssertionError('No daemon owns the singleton lock')
print('ONE_WATCHDOG',workers[0], 'ALLOCATIONS',len(entries))
PY`
	if out, err := loginSSH(ctx, host, checkSingleton, sessions[0].Token, sessions[1].Token); err != nil {
		t.Fatal(out, err)
	} else {
		t.Log(strings.TrimSpace(out))
	}
	// Keep A connected; B's short authenticated connection must also be recorded.
	live, disconnect := context.WithCancel(ctx)
	defer disconnect()
	connection := make(chan error, 1)
	go func() {
		_, err := loginSSH(live, "vscode-sandboxes-"+sessions[0].Token, "sleep 420")
		connection <- err
	}()
	if out, err := loginSSH(ctx, "vscode-sandboxes-"+sessions[1].Token, "echo AUTHENTICATED_B"); err != nil {
		t.Fatal(out, err)
	}
	inspect := `python3 - "$1" "$2" "$3" <<'PY'
import importlib.util,pathlib,sys,json
run=pathlib.Path(sys.argv[1]);spec=importlib.util.spec_from_file_location('w',run/'watchdog.py');w=importlib.util.module_from_spec(spec);spec.loader.exec_module(w)
entry=dict(token=sys.argv[2],jobID=sys.argv[3],runDir=str(run))
print(json.dumps(w.observe(entry,{})))
PY`
	for i, s := range sessions {
		out, err := loginSSH(ctx, host, inspect, s.RunDir, s.Token, s.JobID)
		if err != nil {
			t.Fatal("inspect", out, err)
		}
		t.Logf("OBSERVATION_%d %s", i, out)
		if !strings.Contains(out, `"lastAuthenticatedAt": `) || strings.Contains(out, `"lastAuthenticatedAt": 0`) {
			t.Fatal("authentication hook missing", out)
		}
		if i == 0 && strings.Contains(out, `"connections": 0`) {
			t.Fatal("active SSH connection was not detected", out)
		}
	}
	backdate := `python3 - "$1" <<'PY'
import fcntl,json,pathlib,sys,time,os,tempfile
root=pathlib.Path.home()/'.local/share/vscode-sandboxes/watchdog'
with (root/'registry.lock').open('a') as lock:
 fcntl.flock(lock,fcntl.LOCK_EX)
 entries=json.loads((root/'allocations.json').read_text())
 for token in sys.argv[1].split(','):
  e=entries[token];e.update(connected=False,idleSince=time.time()-86410)
  event=pathlib.Path(e['runDir'])/'ssh-activity.json'
  if event.exists():e['lastAuthenticatedAt']=json.loads(event.read_text())['lastAuthenticatedAt']
 with tempfile.NamedTemporaryFile(mode='w',dir=root,delete=False) as out:
  json.dump(entries,out);name=out.name
 os.replace(name,root/'allocations.json')
 print((root/'status.json').read_text())
PY`
	if out, err := loginSSH(ctx, host, backdate, sessions[0].Token+","+sessions[1].Token); err != nil {
		t.Fatal(out, err)
	} else {
		t.Log("SINGLETON", out)
	}
	waitEnded := func(s RemoteSession) {
		for {
			out, err := loginSSH(ctx, host, `squeue --noheader --user "$USER" --format='%i|%k'`)
			if err != nil {
				t.Fatal(out, err)
			}
			if !strings.Contains(out, s.JobID+"|vscode-sandboxes:"+s.Token) {
				return
			}
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-time.After(5 * time.Second):
			}
		}
	}
	waitEnded(sessions[1])
	t.Log("IDLE_B_CANCELLED")
	inventory, err := controller.listRemoteSessions(ctx, sessions[0].Profile)
	if err != nil {
		t.Fatal(err)
	}
	alive := false
	for _, s := range inventory {
		if s.Token == sessions[0].Token && s.JobID == sessions[0].JobID {
			alive = true
		}
	}
	if !alive {
		t.Fatal("connection to A did not keep A alive")
	}
	t.Log("CONNECTED_A_RETAINED")
	disconnect()
	select {
	case <-connection:
	case <-time.After(10 * time.Second):
		t.Fatal("SSH did not disconnect")
	}
	// Allow the transport to close before accelerating only A's own clock.
	time.Sleep(5 * time.Second)
	if out, err := loginSSH(ctx, host, backdate, sessions[0].Token); err != nil {
		t.Fatal(out, err)
	}
	waitEnded(sessions[0])
	t.Log("DISCONNECTED_A_CANCELLED")
	for {
		out, err := loginSSH(ctx, host, `cat "$HOME/.local/share/vscode-sandboxes/watchdog/status.json"`)
		if err != nil {
			t.Fatal(out, err)
		}
		if strings.Contains(out, `"running": false`) {
			t.Log("WATCHDOG_EXITED", out)
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(3 * time.Second):
		}
	}
	verifyExit := `python3 - "$1" "$2" <<'PY'
import pathlib,json,fcntl,sys
root=pathlib.Path.home()/'.local/share/vscode-sandboxes/watchdog'
with (root/'daemon.lock').open('a') as lock:
 fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
 assert not json.loads((root/'allocations.json').read_text())
 status=json.loads((root/'status.json').read_text())
 assert not status['running'] and status['count']==0,status
 path=pathlib.Path('/proc')/str(status['pid'])/'cmdline'
 assert not path.exists() or b'watchdog.py' not in path.read_bytes(),'Watchdog process still running'
 log=(root/'watchdog.log').read_text()
 for job in sys.argv[1:]:
  assert 'job='+job+' Stopped after 24 hours without an authenticated connection' in log,job
print('WATCHDOG_PROCESS_EXITED_LOCK_RELEASED_AND_CANCELLATIONS_CONFIRMED')
PY`
	if out, err := loginSSH(ctx, host, verifyExit, sessions[0].JobID, sessions[1].JobID); err != nil {
		t.Fatal(out, err)
	} else {
		t.Log(strings.TrimSpace(out))
	}
}
