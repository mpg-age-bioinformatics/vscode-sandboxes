package startui

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Explicit opt-in: uploads an image, submits a real short Slurm job, verifies
// container SSH and the language runtime, and cancels only its own allocation.
func TestRemoteLiveHPC(t *testing.T) {
	host := os.Getenv("VSCODE_REMOTE_LIVE_HPC")
	if host == "" {
		t.Skip("set VSCODE_REMOTE_LIVE_HPC to an authorized SSH host")
	}
	preset := os.Getenv("VSCODE_REMOTE_LIVE_PRESET")
	if preset == "" {
		preset = "python"
	}
	p := defaultRemoteProfile()
	p.Login, p.ImageSource, p.Architecture = host, preset, "amd64"
	p.Version = "3.13"
	if preset == "r" {
		p.Version = "4.5"
	}
	if preset == "bioinformatics" {
		p.Version = ""
	}
	token := fmt.Sprintf("%032x", time.Now().UnixNano())
	p.Project = "~/sandboxes/launcher-smoke-" + preset + "-" + token[16:]
	if project := os.Getenv("VSCODE_REMOTE_LIVE_PROJECT"); project != "" {
		p.Project = project
	}
	if os.Getenv("VSCODE_REMOTE_LIVE_CLUSTER_TMP") == "1" {
		p.Modules = `export SLURM_TMPDIR="$HOME/sandboxes/cluster temporary"`
	}
	p.Time = "00:20:00"
	p.Folders = " ~/sandboxes/launcher-readonly-fixture , , ~/sandboxes/launcher-readonly-fixture "
	p = normalizeRemoteProfile(p)
	if err := validateRemoteProfile(p); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	if _, err := loginSSH(ctx, host, `set -eu; mkdir -p "$HOME/sandboxes/launcher-readonly-fixture" "$HOME/sandboxes/cluster temporary"`); err != nil {
		t.Fatal(err)
	}
	controller := &remoteController{ssh: loginSSH}
	s := RemoteSession{Profile: p, Token: token, Active: true}
	var plan remoteImagePlan
	if source := os.Getenv("VSCODE_REMOTE_LIVE_CACHE_SIF"); source != "" {
		cache := os.Getenv("VSCODE_REMOTE_LIVE_CACHE")
		recipe, err := os.ReadFile(filepath.Join(cache, "Dockerfile"))
		if err != nil {
			t.Fatal(err)
		}
		lock, err := os.ReadFile(filepath.Join(cache, "image-lock.json"))
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]string
		if err := json.Unmarshal(lock, &fields); err != nil {
			t.Fatal(err)
		}
		expected := strings.ReplaceAll(remoteDockerfile(p), remoteBaseImage(p), fields["resolvedImage"])
		expected = strings.ReplaceAll(expected, "node:22.22.1-bookworm-slim", fields["nodeImage"])
		if string(recipe) != expected {
			t.Fatal("seed SIF recipe does not match the current image recipe")
		}
		record := describeRemoteImage(p, string(recipe), lock)
		var reserved remoteImageRecord
		if err := controller.catalog(ctx, p, "reserve", record, &reserved); err != nil {
			t.Fatal(err)
		}
		if reserved.State != "ready" {
			out, err := loginSSH(ctx, host, `set -eu; test -s "$1"; if ! test -s "$2"; then ln "$1" "$2"; fi; printf '%s\n' "$3" > "$(dirname "$2")/ready"`, source, reserved.Path, record.ID)
			if err != nil {
				t.Fatal(out, err)
			}
		}
		if os.Getenv("VSCODE_REMOTE_LIVE_CACHED") == "1" {
			s.Profile.ImageSource = "cached"
			s.Profile.CachedImage = record.ID
			s.Profile.Version = ""
		}
		var hit bool
		plan, hit, err = controller.cachedRemotePlan(ctx, &s)
		if err != nil || !hit {
			t.Fatal("remote cache miss", err)
		}
		p = s.Profile
		if err := controller.stageRemoteImage(ctx, s, &plan); err != nil {
			t.Fatal(err)
		}
		t.Log("SERVER_CACHE_HIT: no local build, no image transfer, no conversion")
	} else if stage := os.Getenv("VSCODE_REMOTE_LIVE_STAGE"); stage != "" {
		// Retry startup changes against an explicitly selected previous test SIF,
		// keeping a fresh project and allocation but avoiding another conversion.
		cache := os.Getenv("VSCODE_REMOTE_LIVE_CACHE")
		recipe, err := os.ReadFile(filepath.Join(cache, "Dockerfile"))
		if err != nil {
			t.Fatal(err)
		}
		lock, err := os.ReadFile(filepath.Join(cache, "image-lock.json"))
		if err != nil {
			t.Fatal(err)
		}
		files, err := remoteProjectFiles(p, string(recipe), lock)
		if err != nil {
			t.Fatal(err)
		}
		archive := filepath.Join(t.TempDir(), "project.tar")
		if err := writeRemoteProjectTar(archive, files); err != nil {
			t.Fatal(err)
		}
		plan = remoteImagePlan{Stage: stage, Image: stage + "/workspace.sif"}
		if !safeRemotePath(stage, false) {
			t.Fatal("invalid previous test stage")
		}
		if err := uploadRemoteFile(ctx, host, archive, stage+"/project.tar"); err != nil {
			t.Fatal(err)
		}
	} else {
		var err error
		plan, err = prepareRemoteImage(ctx, p, token, os.Stdout)
		if err != nil {
			t.Fatal(err)
		}
		if err := controller.stageRemoteImage(ctx, s, &plan); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("Prepared %s workspace staging at %s", preset, plan.Stage)
	dir := remoteSessionDir(token)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	defer os.Remove(filepath.Join(settingsDir(), "remote-ssh", token+".conf"))
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
	encode := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	// Recover by token as well if submission returns ambiguously.
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), time.Minute)
		defer done()
		if s.JobID == "" {
			out, err := loginSSH(cleanup, host, remoteAsset("recover.sh"), p.StateDir, token)
			if err != nil {
				t.Errorf("recover test allocation: %v", err)
				return
			}
			v := parseRemoteValues(out)
			s.JobID, s.RunDir = v["JOB"], v["RUN"]
		}
		if s.JobID != "" {
			out, err := loginSSH(cleanup, host, remoteAsset("cancel.sh"), s.RunDir, token, s.JobID)
			if err != nil {
				t.Errorf("cancel test allocation %s: %s %v", s.JobID, out, err)
			} else {
				t.Logf("Cancelled test allocation %s", s.JobID)
			}
		}
	}()
	out, err := loginSSH(ctx, host, remoteAsset("submit.sh"), p.StateDir, token, p.Project, p.Folders, plan.Image, p.Partition, strconv.Itoa(p.CPUs), p.Memory, p.Time, "0", p.Apptainer, encode(string(key)), encode(remoteModuleScript(p)), encode(remoteAsset("job.sh")), encode(remoteAsset("container.sh")), plan.Stage, encode(remoteAsset("initialize-project.sh")), encode(remoteAsset("relay.sh")), encode(remoteProfileJSON(s.Profile)), plan.Key, encode(remoteAsset("watchdog.py")))
	v := parseRemoteValues(out)
	s.JobID, s.RunDir = v["JOB"], v["RUN"]
	if err != nil || !validRemoteSession(s) || s.JobID == "" {
		t.Fatalf("submit: %s %v", out, err)
	}
	t.Logf("Job %s; project %s; run %s", s.JobID, p.Project, s.RunDir)
	sessions, err := controller.listRemoteSessions(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range sessions {
		if entry.Token == token && entry.JobID == s.JobID {
			found = true
		}
	}
	if !found {
		t.Fatal("submitted sandbox missing from server inventory")
	}
	var authorized RemoteSession
	if err := controller.catalog(ctx, p, "authorize", map[string]string{"token": token, "jobID": s.JobID, "publicKey": strings.TrimSpace(string(key))}, &authorized); err != nil {
		t.Fatal(err)
	}
	t.Log("ALLOCATION_DISCOVERED_AND_AUTHORIZED")

	last := ""
	for {
		out, err = loginSSH(ctx, host, remoteAsset("probe.sh"), s.RunDir, token, s.JobID)
		if err != nil {
			t.Fatal(out, err)
		}
		v = parseRemoteValues(out)
		if v["QUEUE"] != last {
			t.Log(out)
			last = v["QUEUE"]
		}
		if v["STATE"] == "ENDED" {
			t.Fatal("allocation ended before SSH startup: ", out)
		}
		if v["STATE"] == "READY" {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(5 * time.Second):
		}
	}
	s.Node, s.User, s.ProjectPath = v["HOST"], v["USER"], v["PROJECT"]
	if !safeRemotePath(s.ProjectPath, false) {
		t.Fatal("missing exact project path", s.ProjectPath)
	}
	s.Port, _ = strconv.Atoi(v["PORT"])
	hostkey := ""
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "ssh-ed25519 ") {
			hostkey = line
			break
		}
	}
	if err := prepareRemoteSSH(s, hostkey); err != nil {
		t.Fatal(err)
	}
	alias := "vscode-sandboxes-" + token
	script := `set -eu
 test "$VSCODE_SANDBOX_REMOTE_CONTAINER" = "$1"
 test -r /run/vscode-launcher/sshd_config
 test -w "$HOME"
 test -n "${TMPDIR:-}"
 test -w "$TMPDIR"
 python3 -c 'import tempfile;print("TMPDIR="+tempfile.gettempdir())'
 test -d /workspace/code
 test -d /workspace/data
 test ! -e "$HOME/.vscode-server/cli"
 test -r "$HOME/.profile"
 test "$(bash -lc pwd)" = "$2"
 test "$PWD" != /workspace
 python3 - "$2" "$HOME/sandboxes/launcher-readonly-fixture" <<'PYMOUNTS'
import os,sys,tempfile,pathlib
project=pathlib.Path(sys.argv[1])
# ~ is the isolated container home; expand against the project’s HPC account path instead.
extra=project.parent/'launcher-readonly-fixture'
with tempfile.NamedTemporaryFile(dir=project): pass
try:
    with tempfile.NamedTemporaryFile(dir=extra): pass
except OSError as error:
    if error.errno != 30: raise
else: raise RuntimeError('Additional folder allowed writes')
with tempfile.NamedTemporaryFile(dir=project) as output:
    os.chmod(output.name,0o600)
(project/'data'/('.launcher-persistence-'+os.environ['VSCODE_SANDBOX_REMOTE_CONTAINER'])).write_text('persistent result')
print('EXACT_PROJECT_RW_ADDITIONAL_RO_OK')
PYMOUNTS
 command -v codex
 codex --version
 command -v node
 git -C /workspace log -1 --format=%s
 printf 'CONTAINER_OK\n'
 `
	if preset == "python" {
		script += `cd /workspace; .venv/bin/python -c 'import sys,ipykernel,jupytext; print(sys.version); print("PYTHON_OK")'; .venv/bin/python -m pip check; .venv/bin/jupytext --version`
	}
	if preset == "r" {
		script += "cd /workspace; Rscript --vanilla -e " + remoteQuote(remoteAsset("smoke-r.R")) + "; Rscript --vanilla code/.sandbox/verify-remote-renv.R"
	}
	script = strings.ReplaceAll(script, "/workspace", remoteQuote(s.ProjectPath))
	out, err = loginSSH(ctx, alias, script, s.JobID, s.ProjectPath)
	t.Log(out)
	if err != nil || !strings.Contains(out, "CONTAINER_OK") {
		diagnostic, diagnosticErr := loginSSH(ctx, alias, `id; printf 'HOME=%s MARKER=%s\n' "$HOME" "${VSCODE_SANDBOX_REMOTE_CONTAINER:-missing}"; command -v codex || true; cat /run/vscode-launcher/sshd_config; df -h /tmp; pwd`)
		t.Log(diagnostic, diagnosticErr)
		t.Fatal("container runtime verification: ", err)
	}
	// Simulate attaching from another laptop: authorize a different key, then
	// connect using it without submitting another allocation.
	replacement := filepath.Join(dir, "replacement_key")
	if out, err := exec.CommandContext(ctx, keygen, "-q", "-t", "ed25519", "-N", "", "-f", replacement).CombinedOutput(); err != nil {
		t.Fatal(string(out), err)
	}
	replacementPublic, err := os.ReadFile(replacement + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.catalog(ctx, p, "authorize", map[string]string{"token": token, "jobID": s.JobID, "publicKey": strings.TrimSpace(string(replacementPublic))}, &authorized); err != nil {
		t.Fatal(err)
	}
	private, err := os.ReadFile(replacement)
	if err != nil {
		t.Fatal(err)
	}
	if err := privateWrite(filepath.Join(dir, "id_ed25519"), private); err != nil {
		t.Fatal(err)
	}
	if err := privateWrite(filepath.Join(dir, "id_ed25519.pub"), replacementPublic); err != nil {
		t.Fatal(err)
	}
	t.Log("NEW_CLIENT_KEY_AUTHORIZED")
	// Repeat through a fresh SSH connection to exercise reconnect and persisted state.
	out, err = loginSSH(ctx, alias, `test "$VSCODE_SANDBOX_REMOTE_CONTAINER" = "$1"; test -d "$2/.git"; echo RECONNECT_OK`, s.JobID, s.ProjectPath)
	if err != nil || !strings.Contains(out, "RECONNECT_OK") {
		t.Fatal(out, err)
	}
	t.Log("RECONNECT_OK")
	// VS Code can open concurrent connections. Each must attach to the same job,
	// including on one-CPU allocations and older Slurm without --overlap.
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			check, done := context.WithTimeout(ctx, 45*time.Second)
			defer done()
			out, err := loginSSH(check, alias, `sleep 2; test "$VSCODE_SANDBOX_REMOTE_CONTAINER" = "$1"; echo PARALLEL_OK`, s.JobID)
			if err == nil && !strings.Contains(out, "PARALLEL_OK") {
				err = fmt.Errorf("parallel SSH failed")
			}
			results <- err
		}()
	}
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	out, err = loginSSH(ctx, alias, `python3 -c 'import os;os.write(1,bytes(range(256))*4096)'`)
	pattern := make([]byte, 256)
	for i := range pattern {
		pattern[i] = byte(i)
	}
	if err != nil || out != strings.Repeat(string(pattern), 4096) {
		t.Fatalf("binary Slurm relay failed: %d bytes, %v", len(out), err)
	}
	t.Log("PARALLEL_AND_BINARY_OK")
	if err := controller.stopRemoteSession(ctx, p, token, s.JobID); err != nil {
		t.Fatal(err)
	}
	t.Log("STOP_SELECTED_OK")
	out, err = loginSSH(ctx, host, `set -eu; marker="$1/data/.launcher-persistence-$2"; test "$(cat "$marker")" = 'persistent result'; rm "$marker"; printf 'PROJECT_SURVIVED_ALLOCATION_OK\n'`, s.ProjectPath, s.JobID)
	if err != nil || !strings.Contains(out, "PROJECT_SURVIVED_ALLOCATION_OK") {
		t.Fatal(out, err)
	}
	t.Log("PROJECT_SURVIVED_ALLOCATION_OK")

}
