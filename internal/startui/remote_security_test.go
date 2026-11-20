package startui

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRemoteSecurityMountPolicy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX shell")
	}
	root := remoteTestHome(t)
	project := filepath.Join(root, "sandboxes", "project")
	state := filepath.Join(root, "state")
	image := filepath.Join(root, "image.sif")
	run := filepath.Join(state, "runs", "test")
	os.MkdirAll(project, 0700)
	os.MkdirAll(filepath.Join(root, ".ssh"), 0700)
	os.Symlink(filepath.Join(root, ".ssh"), filepath.Join(root, "alias-to-credentials"))
	for _, tc := range []struct {
		name, project, folders string
		allowed                bool
	}{
		{"project", project, "", true},
		{"read-only-extra", project, filepath.Join(root, "datasets"), true},
		{"home", root, "", false},
		{"ancestor", filepath.Dir(root), "", false},
		{"credentials", filepath.Join(root, ".ssh"), "", false},
		{"credential-alias", filepath.Join(root, "alias-to-credentials"), "", false},
		{"extra-hidden", project, filepath.Join(root, ".config"), false},
		{"extra-home", project, root, false},
		{"bin", filepath.Join(root, "bin"), "", false},
		{"control", state, "", false},
		{"image-parent", root, "", false},
		{"runtime", project, "/run", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command("bash", "-s", "--", tc.project, root, state, image, run, tc.folders)
			cmd.Stdin = strings.NewReader(remoteAsset("security.sh") + "\nproject=$1; HOME=$2; state=$3; image=$4; run=$5; security_mounts \"$6\"\n")
			out, err := cmd.CombinedOutput()
			if (err == nil) != tc.allowed {
				t.Fatalf("allowed=%v: %s %v", tc.allowed, out, err)
			}
		})
	}
}

func TestRemoteSecurityPortableImageAssets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX shell and symlinks")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable")
	}
	recipe := remoteDockerfile(RemoteProfile{ImageSource: "python", Version: "3.13"})
	var installer []string
	for _, line := range strings.Split(recipe, "\n") {
		if strings.HasPrefix(line, "RUN [") {
			var argv []string
			if json.Unmarshal([]byte(strings.TrimPrefix(line, "RUN ")), &argv) == nil && len(argv) == 3 && strings.Contains(argv[2], "slurm-unavailable.sh") {
				installer = argv
				break
			}
		}
	}
	if len(installer) == 0 {
		t.Fatal("missing portable security installation")
	}
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "bin"), 0700)
	script := strings.ReplaceAll(strings.ReplaceAll(installer[2], "/usr/local/share/vscode-sandboxes/security", filepath.Join(root, "security")), "/usr/local/bin", filepath.Join(root, "bin"))
	if out, err := exec.Command(python, "-c", script).CombinedOutput(); err != nil {
		t.Fatal(string(out), err)
	}
	for name, expected := range map[string]string{"seccomp-agents.json": remoteAsset("seccomp-agents.json"), "verify-security.py": remoteAsset("verify-security.py")} {
		data, err := os.ReadFile(filepath.Join(root, "security", name))
		if err != nil || string(data) != expected {
			t.Fatal(name, err)
		}
	}
	if data, err := os.ReadFile(filepath.Join(root, "security", "APPTAINER.md")); err != nil || !strings.Contains(string(data), "--containall") {
		t.Fatal("missing security guide", err)
	}
	if out, err := exec.Command("bash", filepath.Join(root, "bin", "sbatch")).CombinedOutput(); err == nil || !strings.Contains(string(out), "Slurm") {
		t.Fatal("Slurm explanation stub", string(out), err)
	}
}

func verifyRemoteSecurityLive(t *testing.T, ctx context.Context, s RemoteSession) {
	t.Helper()
	hostInfo, err := loginSSH(ctx, s.Profile.Login, `printf '%s\n' "$HOME"; readlink /proc/self/ns/pid; readlink /proc/self/ns/ipc`)
	if err != nil {
		t.Fatal(err)
	}
	info := strings.Split(strings.TrimSpace(hostInfo), "\n")
	if len(info) != 3 {
		t.Fatal(hostInfo)
	}
	script := `set -eu
python3 - "$1" "$2" "$3" "$4" "$5" <<'PY'
import ctypes,errno,json,os,pathlib,tempfile,threading,sys
home=pathlib.Path(sys.argv[1]);token=sys.argv[2];project=pathlib.Path(sys.argv[3])
assert os.readlink('/proc/self/ns/pid')!=sys.argv[4]
assert os.readlink('/proc/self/ns/ipc')!=sys.argv[5]
assert not (home/('.vscode-security-fixture-'+token)/'marker').exists(),'Real home visible'
assert not pathlib.Path('/host-leak/marker').exists(),'Inherited bind injected'
assert not os.environ.get('VSCODE_SECURITY_TEST_SECRET'),'Host credential forwarded'
assert not os.environ.get('VSCODE_SECURITY_INJECTED'),'APPTAINERENV override forwarded'
assert not os.environ.get('SSH_AUTH_SOCK'),'Host SSH agent forwarded'
extra=home/'sandboxes'/('security-readonly-'+token)
assert (extra/'marker').read_text()==token
for path in [extra/'marker',pathlib.Path('/run/vscode-launcher/modules.sh'),pathlib.Path('/run/vscode-launcher/watchdog.py')]:
 try:
  with path.open('a'):pass
 except OSError as error:assert error.errno==errno.EROFS,(path,error)
 else:raise AssertionError('Writable protected path: '+str(path))
with tempfile.NamedTemporaryFile(dir=project):pass
with tempfile.NamedTemporaryFile(dir=pathlib.Path.home()):pass
with tempfile.NamedTemporaryFile(dir='/run/vscode-launcher/runtime'):pass
status=dict(line.split(':',1) for line in pathlib.Path('/proc/self/status').read_text().splitlines() if ':' in line)
assert all(int(status[k].strip(),16)==0 for k in ['CapInh','CapPrm','CapEff','CapBnd','CapAmb'])
assert status['NoNewPrivs'].strip()=='1' and status['Seccomp'].strip()=='2'
libc=ctypes.CDLL(None,use_errno=True)
for name,args in [('unshare',(0,)),('ptrace',(0x7fffffff,0,0,0))]:
 ctypes.set_errno(0);assert getattr(libc,name)(*args)==-1 and ctypes.get_errno()==errno.EPERM,name
done=[];worker=threading.Thread(target=lambda:done.append(True));worker.start();worker.join();assert done
child=os.fork()
if child==0:os._exit(0)
assert os.waitpid(child,0)[1]==0
print('SECURITY_MOUNTS_ENV_NAMESPACES_CAPABILITIES_SECCOMP_FORK_THREADS_OK')
PY
node -e 'const {Worker}=require("worker_threads"); new Worker("require(\"worker_threads\").parentPort.postMessage(1)",{eval:true}).on("message",n=>{if(n!==1)process.exitCode=1;else console.log("NODE_THREADS_OK")})'
codex --version
claude --version
if command -v Rscript >/dev/null;then Rscript --vanilla -e 'stopifnot(sum(1:10)==55);f<-tempfile();write.csv(data.frame(x=1:3),f);stopifnot(file.exists(f));unlink(f);cat("R_RUNTIME_OK\n")';fi
`
	out, err := loginSSH(ctx, "vscode-sandboxes-"+s.Token, script, info[0], s.Token, s.ProjectPath, info[1], info[2])
	if err != nil {
		t.Fatal(out, err)
	}
	t.Log("SECURITY_VERIFIED", s.JobID, out)
}
