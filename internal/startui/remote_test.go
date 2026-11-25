package startui

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func remoteTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	return home
}
func remoteTestProfile() RemoteProfile {
	p := defaultRemoteProfile()
	p.Login = "researcher@hpc-login"
	p.Project = "/shared/project with 'quotes'"
	p.Image = "/shared/workspace.sif"
	return p
}
func TestRemoteProfileValidationAndQuoting(t *testing.T) {
	p := remoteTestProfile()
	if err := validateRemoteProfile(p); err != nil {
		t.Fatal(err)
	}
	for _, edit := range []func(*RemoteProfile){
		func(p *RemoteProfile) { p.Login = "-oProxyCommand=bad" }, func(p *RemoteProfile) { p.Project = "/data:/etc" }, func(p *RemoteProfile) { p.Project = "/var/tmp/project" },
		func(p *RemoteProfile) { p.StateDir = "~/../.." }, func(p *RemoteProfile) { p.CPUs = 0 }, func(p *RemoteProfile) { p.Memory = "8G;bad" },
		func(p *RemoteProfile) { p.Time = "1:99:00" }, func(p *RemoteProfile) { p.ModuleNames = "apptainer; touch bad" }, func(p *RemoteProfile) { p.Apptainer = "-bad" },
	} {
		bad := p
		edit(&bad)
		if validateRemoteProfile(bad) == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
	if runtime.GOOS == "windows" {
		return
	}
	for _, value := range []string{p.Project, "$(touch bad); `touch bad`", "a\nb", ""} {
		out, err := exec.Command("bash", "-c", "printf '%s' "+remoteQuote(value)).Output()
		if err != nil || string(out) != value {
			t.Fatalf("quote roundtrip %q: %s %v", value, out, err)
		}
	}
}
func TestRemoteCodexPlacementPreservesContinueAndComments(t *testing.T) {
	source := []byte("{\n// Preserve this comment\n\"remote.extensionKind\":{\"Continue.continue\":[\"workspace\"],\"openai.chatgpt\":[\"ui\"]},\"editor.fontSize\":17}")
	updated, err := workspaceExtensionSettings(source, "openai.chatgpt")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(updated, []byte("// Preserve this comment")) || !bytes.Contains(updated, []byte("Continue.continue")) || bytes.Contains(updated, []byte(`"ui"`)) {
		t.Fatal(string(updated))
	}
	again, err := workspaceExtensionSettings(updated, "openai.chatgpt")
	if err != nil || !bytes.Equal(updated, again) {
		t.Fatal("not idempotent", err)
	}
}
func TestRemoteLaunchAndReconnectThroughVerifiedContainer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX CLI fixture")
	}
	home := remoteTestHome(t)
	bin := t.TempDir()
	log := filepath.Join(home, "code-calls")
	if err := os.WriteFile(filepath.Join(bin, "code"), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$CODE_TEST_LOG\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODE_TEST_LOG", log)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	os.MkdirAll(filepath.Join(home, ".ssh"), 0700)
	original := "Host hpc-login\n  User researcher\n"
	os.WriteFile(filepath.Join(home, ".ssh", "config"), []byte(original), 0600)
	s := RemoteSession{Profile: remoteTestProfile(), Token: strings.Repeat("a", 32), Active: true}
	hostKey := base64.StdEncoding.EncodeToString(append([]byte{0, 0, 0, 11}, append([]byte("ssh-ed25519"), append([]byte{0, 0, 0, 32}, bytes.Repeat([]byte{1}, 32)...)...)...))
	submissions, probes, proofs, endpointConfigurations, webConfigurations := 0, 0, 0, 0, 0
	p := &remoteController{state: RemoteStatus{Session: s}, pollInterval: time.Millisecond}
	p.ssh = func(ctx context.Context, host, script string, args ...string) (string, error) {
		if strings.Contains(script, "VSCODE_AGENT_NETWORK") {
			if host != "vscode-sandboxes-"+s.Token || proofs != webConfigurations+1 {
				t.Fatal("web configuration did not follow proof of the exact container")
			}
			if _, err := os.Stat(log); err == nil {
				t.Fatal("editor opened before web policy")
			}
			webConfigurations++
			return "AGENT_WEB_POLICY_CONFIGURED\n", nil
		}
		if strings.HasPrefix(script, "python3 -I -c ") {
			if host != "vscode-sandboxes-"+s.Token || len(args) != 0 || proofs != 1 {
				t.Fatal("endpoint configuration did not target the verified container")
			}
			if _, err := os.Stat(log); err == nil {
				t.Fatal("VS Code opened before endpoint settings were applied")
			}
			if strings.Contains(script, "ENDPOINT_CONFIGURED") {
				endpointConfigurations++
				return "ENDPOINT_CONFIGURED\n", nil
			}
			return `{"models":["private-model"]}`, nil
		}
		switch script {
		case remoteAsset("submit.sh"):
			submissions++
			if host != s.Profile.Login || args[2] != s.Profile.Project {
				t.Fatal(host, args)
			}
			return "RUN=/shared/state/runs/" + s.Token + "\nJOB=1234\n", nil
		case remoteAsset("probe.sh"):
			probes++
			if probes == 1 {
				return "STATE=WAITING\nQUEUE=1234|RUNNING|compute-42|None\nSTARTUP_PHASE=Preparing Python environment\nLOG_OFFSET=17\nLOG64=" + base64.StdEncoding.EncodeToString([]byte("Installing tools\n")) + "\n", nil
			}
			if probes == 2 && (args[3] != "17" || p.snapshot().Phase != "Preparing Python environment" || !strings.Contains(p.snapshot().Text, "Installing tools\n")) {
				t.Fatal("startup progress was not forwarded", args, p.snapshot())
			}
			return "STATE=READY\nHOST=compute-42\nPORT=22022\nUSER=researcher\nJOB=1234\nPROJECT=" + s.Profile.Project + "\nssh-ed25519 " + hostKey + "\n", nil
		default:
			proofs++
			if host != "vscode-sandboxes-"+s.Token || !strings.Contains(script, "VSCODE_SANDBOX_REMOTE_CONTAINER") {
				t.Fatal("connection was not proved", host, script)
			}
			if _, err := os.Stat(log); err == nil {
				t.Fatal("editor opened before container verification")
			}
			return "CONTAINER_OK\n", nil
		}
	}
	if err := p.launch(context.Background(), s, false, remoteEndpoint{BaseURL: "https://models.example.invalid/v1", APIKey: "startup-private-key", PrivateAgent: "codex"}); err != nil {
		t.Fatal(err)
	}
	if submissions != 1 || proofs != 1 || endpointConfigurations != 1 || !p.snapshot().Session.Active {
		t.Fatal(p.snapshot())
	}
	profileJSON, _ := os.ReadFile(filepath.Join(settingsDir(), "remote-profile.json"))
	if bytes.Contains(profileJSON, []byte("startup-private-key")) || strings.Contains(p.snapshot().Text, "startup-private-key") {
		t.Fatal("startup credentials were persisted or logged")
	}
	config, _ := os.ReadFile(filepath.Join(home, ".ssh", "config"))
	if !bytes.HasSuffix(config, []byte(original)) {
		t.Fatal("lost SSH settings", string(config))
	}
	managed, _ := os.ReadFile(filepath.Join(settingsDir(), "remote-ssh", s.Token+".conf"))
	for _, part := range []string{"ProxyCommand", "PermitLocalCommand yes", "connected-host", "researcher@hpc-login", "/relay.sh", "HostName 127.0.0.1", "StrictHostKeyChecking yes", "IdentityAgent none", "ForwardAgent no"} {
		if !bytes.Contains(managed, []byte(part)) {
			t.Fatal("missing SSH protection", part)
		}
	}
	if bytes.Contains(managed, []byte("ProxyJump")) || bytes.Contains(managed, []byte("HostName compute-42")) {
		t.Fatal("direct compute-node transport configured", string(managed))
	}
	calls, _ := os.ReadFile(log)
	for _, part := range []string{"--install-extension ms-vscode-remote.remote-ssh", "--disable-workspace-trust --remote ssh-remote+vscode-sandboxes-" + s.Token + " " + s.Profile.Project} {
		if !bytes.Contains(calls, []byte(part)) {
			t.Fatal("wrong extension or workspace host", string(calls))
		}
	}
	if bytes.Contains(calls, []byte("--install-extension openai.chatgpt")) || bytes.Contains(calls, []byte("--install-extension Continue.continue")) {
		t.Fatal("remote extensions installed into the desktop host", string(calls))
	}
	os.Remove(log)
	if err := p.launch(context.Background(), p.snapshot().Session, true); err != nil {
		t.Fatal(err)
	}
	if submissions != 1 || proofs != 2 || endpointConfigurations != 1 || webConfigurations != 2 {
		t.Fatal("reconnect submitted another job")
	}
	p.ssh = func(_ context.Context, host, script string, args ...string) (string, error) {
		if script != remoteAsset("cancel.sh") || host != s.Profile.Login || args[2] != "1234" {
			t.Fatal("wrong cancellation target", host, args)
		}
		return "", nil
	}
	if err := p.stop(); err != nil {
		t.Fatal(err)
	}
	restored := newRemoteController()
	if restored.snapshot().Session.Active {
		t.Fatal("cancellation was not persisted")
	}
	if _, err := os.Stat(filepath.Join(remoteSessionDir(s.Token), "id_ed25519")); !os.IsNotExist(err) {
		t.Fatal("allocation key retained")
	}
}
func TestRemoteFailedCancellationKeepsAllocation(t *testing.T) {
	remoteTestHome(t)
	s := RemoteSession{Profile: remoteTestProfile(), Token: strings.Repeat("b", 32), RunDir: "/shared/state/run", JobID: "99", Active: true}
	p := &remoteController{state: RemoteStatus{Session: s}, ssh: func(context.Context, string, string, ...string) (string, error) {
		return "", errors.New("SSH unavailable")
	}}
	if p.stop() == nil || !p.snapshot().Session.Active {
		t.Fatal("forgot allocation after failed cancel")
	}
	s.JobID = ""
	p.state.Session = s
	if p.stop() == nil {
		t.Fatal("cancelled an unconfirmed job")
	}
}
func TestRemoteShellAssetsSyntax(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires bash")
	}
	for _, name := range []string{"submit.sh", "job.sh", "container.sh", "probe.sh", "cancel.sh", "recover.sh", "local-build.sh", "stage-image.sh", "initialize-project.sh", "relay.sh"} {
		cmd := exec.Command("bash", "-n")
		cmd.Stdin = strings.NewReader(remoteAsset(name))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatal(name, string(out), err)
		}
	}
}

func TestRemoteSchedulerScriptsWithIsolatedCommands(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires bash")
	}
	root := remoteTestHome(t)
	bin := filepath.Join(root, "bin")
	os.MkdirAll(bin, 0700)
	fixtures := map[string]string{
		"getent":   "printf 'researcher:x:1000:1000::/home/researcher:/bin/bash\\n'",
		"sbatch":   "test -z \"${SBATCH_ACCOUNT:-}\"; test \"$COMPUTE_MODULE\" = loaded; printf '%s\\n' \"$@\" > \"$FIXTURE_ARGS\"; printf '1234;cluster\\n'",
		"squeue":   "printf '%s' \"$FIXTURE_QUEUE\"",
		"scontrol": "printf 'NodeName=compute-1 NodeAddr=192.0.2.17 NodeHostName=compute-1\\n'",
		"scancel":  "printf '%s' \"$*\" > \"$FIXTURE_CANCEL\"",
		"apptainer": `printf '%s\n' "$@" > "$FIXTURE_CONTAINER"; test -z "${COMPUTE_MODULE:-}"
while (( $# )); do
 if [[ "$1" == --home ]]; then
  home_source="${2%:*}"; mkdir -p "$home_source/.cache/readonly"
  touch "$home_source/.cache/readonly/file"; chmod 500 "$home_source/.cache/readonly"
  ln -s "$FIXTURE_EXTERNAL_READONLY" "$home_source/external-library"; break
 fi
 shift
done`,
	}
	for name, body := range fixtures {
		if name == "apptainer" {
			body = strings.NewReplacer("$FIXTURE_CONTAINER", filepath.Join(root, "container"), "$FIXTURE_EXTERNAL_READONLY", filepath.Join(root, "external-readonly")).Replace(body)
		}
		os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/bash\nset -eu\n"+body+"\n"), 0700)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FIXTURE_ARGS", filepath.Join(root, "args"))
	t.Setenv("FIXTURE_CANCEL", filepath.Join(root, "cancel"))
	t.Setenv("FIXTURE_CONTAINER", filepath.Join(root, "container"))
	external := filepath.Join(root, "external-readonly")
	os.Mkdir(external, 0500)
	t.Setenv("FIXTURE_EXTERNAL_READONLY", external)
	t.Setenv("SLURM_JOB_ID", "1234")
	project := filepath.Join(root, "sandboxes")
	data := filepath.Join(root, "data with 'quotes'")
	references := filepath.Join(root, "references")
	os.MkdirAll(references, 0700)
	t.Setenv("FIXTURE_MODULES", filepath.Join(root, "modules-called"))
	t.Setenv("SBATCH_ACCOUNT", "must-not-be-used")
	clusterTemp := filepath.Join(root, "cluster temp")
	os.MkdirAll(clusterTemp, 0700)
	t.Setenv("SLURM_TMPDIR", clusterTemp)
	os.WriteFile(filepath.Join(clusterTemp, "keep"), []byte("existing cluster file"), 0600)
	image := filepath.Join(root, "workspace.sif")
	state := filepath.Join(root, "state")
	token := strings.Repeat("c", 32)
	os.MkdirAll(project, 0700)
	os.MkdirAll(data, 0700)
	os.WriteFile(image, []byte("fixture"), 0600)
	runScript := func(name string, args ...string) (string, error) {
		t.Helper()
		cmd := exec.Command("bash", append([]string{"-s", "--"}, args...)...)
		cmd.Stdin = strings.NewReader(remoteAsset(name))
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	encode := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	modules := remoteModuleScript(RemoteProfile{ModuleNames: "apptainer , gcc/13", Modules: `module() { printf '%s\n' "$*" >> "$FIXTURE_MODULES"; export COMPUTE_MODULE=loaded; }`})
	out, err := runScript("submit.sh", state, token, "~/sandboxes", data+" , ~/references , , "+data, image, "gpu", "4", "16G", "03:00:00", "2", "apptainer", encode("ssh-ed25519 fixture"), encode(modules), encode(remoteAsset("job.sh")), encode(remoteAsset("container.sh")))
	if err != nil {
		t.Fatal(out, err)
	}
	values := parseRemoteValues(out)
	run := values["RUN"]
	if values["JOB"] != "1234" {
		t.Fatal(out)
	}
	args, _ := os.ReadFile(filepath.Join(root, "args"))
	if bytes.Contains(args, []byte("--account")) {
		t.Fatal("submitted an account override", string(args))
	}
	folders, _ := os.ReadFile(filepath.Join(run, "additional-folders"))
	if string(folders) != data+"\n"+references+"\n" {
		t.Fatal("folder parsing/expansion", string(folders))
	}
	for _, want := range []string{"--cpus-per-task=4", "--mem=16G", "--time=03:00:00", "--partition=gpu", "--gres=gpu:2", project} {
		if !bytes.Contains(args, []byte(want+"\n")) {
			t.Fatal(want, string(args))
		}
	}
	t.Setenv("FIXTURE_QUEUE", "1234|PENDING|None|Resources|vscode-sandboxes:"+token+"\n")
	out, err = runScript("probe.sh", run, token, "1234")
	if err != nil || parseRemoteValues(out)["STATE"] != "WAITING" {
		t.Fatal(out, err)
	}
	// Package output must stream incrementally and never become protocol metadata.
	startupLog := "STATE=ENDED\n" + strings.Repeat("x", 40000)
	os.WriteFile(filepath.Join(run, "slurm-1234.log"), []byte(startupLog), 0600)
	os.WriteFile(filepath.Join(run, "startup-phase"), []byte("Installing locked Python packages\n"), 0600)
	out, err = runScript("probe.sh", run, token, "1234", "0")
	progress := parseRemoteValues(out)
	chunk, decodeErr := base64.StdEncoding.DecodeString(progress["LOG64"])
	if err != nil || decodeErr != nil || len(chunk) != 32768 || string(chunk) != startupLog[:32768] || progress["STATE"] != "WAITING" || progress["LOG_OFFSET"] != "32768" || progress["STARTUP_PHASE"] != "Installing locked Python packages" {
		t.Fatal("invalid startup progress", progress, err, decodeErr)
	}
	out, err = runScript("probe.sh", run, token, "1234", "32768")
	progress = parseRemoteValues(out)
	chunk, decodeErr = base64.StdEncoding.DecodeString(progress["LOG64"])
	if err != nil || decodeErr != nil || string(chunk) != startupLog[32768:] {
		t.Fatal("startup output repeated or omitted", err, decodeErr)
	}
	os.WriteFile(filepath.Join(run, "ready"), []byte("HOST=compute-1\nPORT=2222\nUSER=researcher\nJOB=1234\n"), 0600)
	out, err = runScript("probe.sh", run, token, "1234")
	if err != nil || parseRemoteValues(out)["STATE"] != "READY" || parseRemoteValues(out)["HOST"] != "compute-1" {
		t.Fatal(out, err)
	}
	os.WriteFile(filepath.Join(run, "profile.json"), []byte(`{"network":"open"}`), 0600)
	out, err = runScript("job.sh", run, state, project, data, image, "apptainer", "2")
	if err != nil {
		t.Fatal(out, err)
	}
	temporary, _ := os.ReadFile(filepath.Join(run, "temporary-directory"))
	tempPath := strings.TrimSpace(string(temporary))
	if !strings.HasPrefix(tempPath, clusterTemp+string(os.PathSeparator)) {
		t.Fatal("ignored cluster temp setting", tempPath)
	}
	if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
		t.Fatal("job temporary subfolder not cleaned", err)
	}
	if _, err := os.Stat(filepath.Join(clusterTemp, "keep")); err != nil {
		t.Fatal("changed site temporary root", err)
	}
	container, _ := os.ReadFile(filepath.Join(root, "container"))
	for _, want := range []string{"--cleanenv\n--containall", "--no-init", "--no-eval", "--no-privs", "--drop-caps\nALL", "--no-mount\nhostfs,cwd,bind-paths,sys", "seccomp:" + run + "/seccomp-agents.json", project + ":" + project + ":rw", data + ":" + data + ":ro", references + ":" + references + ":ro", run + ":/run/vscode-launcher:ro", run + "/runtime:/run/vscode-launcher/runtime:rw", "--workdir", "TMPDIR=", "--nv", "/run/vscode-launcher/container.sh", "1234"} {
		if !bytes.Contains(container, []byte(want)) {
			t.Fatal(want, string(container))
		}
	}
	if !bytes.Contains(container, []byte(tempPath+"/home:/home/researcher")) || !bytes.Contains(container, []byte("--pwd\n"+project+"\n")) {
		t.Fatal("missing ephemeral home or exact project working directory", string(container))
	}
	if info, err := os.Stat(external); err != nil || info.Mode().Perm() != 0500 {
		t.Fatal("cleanup followed a symlink outside the temporary home", err)
	}
	if _, err := os.Stat(filepath.Join(state, "home")); !os.IsNotExist(err) {
		t.Fatal("created a persistent container home", err)
	}
	moduleCalls, _ := os.ReadFile(filepath.Join(root, "modules-called"))
	if string(moduleCalls) != strings.Repeat("load apptainer gcc/13\n", 6) {
		t.Fatal("modules not loaded for submission, status and the compute job", string(moduleCalls))
	}
	if _, err = runScript("cancel.sh", run, "wrong-token", "1234"); err == nil {
		t.Fatal("cancelled mismatched allocation")
	}
	if _, err = os.Stat(filepath.Join(root, "cancel")); !os.IsNotExist(err) {
		t.Fatal("invoked scancel on mismatched allocation")
	}
	t.Setenv("FIXTURE_QUEUE", "1234|vscode-sandboxes:another-allocation")
	if _, err = runScript("cancel.sh", run, token, "1234"); err == nil {
		t.Fatal("cancelled a reused scheduler ID")
	}
	t.Setenv("FIXTURE_QUEUE", "1234|vscode-sandboxes:"+token)
	if out, err = runScript("cancel.sh", run, token, "1234"); err != nil {
		t.Fatal(out, err)
	}
	cancelled, _ := os.ReadFile(filepath.Join(root, "cancel"))
	if string(cancelled) != "1234" {
		t.Fatal(string(cancelled))
	}
	os.Remove(filepath.Join(run, "job-id"))
	t.Setenv("FIXTURE_QUEUE", "999|other\n1234|vscode-sandboxes:"+token+"\n")
	out, err = runScript("recover.sh", state, token)
	if err != nil || parseRemoteValues(out)["JOB"] != "1234" {
		t.Fatal(out, err)
	}
	t.Setenv("FIXTURE_QUEUE", "")
	out, err = runScript("probe.sh", run, token, "1234")
	if err != nil || parseRemoteValues(out)["STATE"] != "ENDED" {
		t.Fatal(out, err)
	}
	alias := filepath.Join(root, "project-alias")
	if err := os.Symlink(project, alias); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(run, "additional-folders"), []byte(alias+"\n"), 0600)
	out, err = runScript("job.sh", run, state, project, "", image, "apptainer", "0")
	if err == nil || !strings.Contains(out, "must not overlap") {
		t.Fatal("allowed a read-only alias of the writable project", out, err)
	}
	// Additional folders are optional, including on Bash versions with nounset
	// handling that treats an empty array as unbound.
	out, err = runScript("submit.sh", state, strings.Repeat("d", 32), "~/sandboxes", "", image, "cluster", "1", "8G", "02:00:00", "0", "apptainer", encode("ssh-ed25519 fixture"), encode(modules), encode(remoteAsset("job.sh")), encode(remoteAsset("container.sh")))
	if err != nil {
		t.Fatal("submission without additional folders", out, err)
	}
	folders, err = os.ReadFile(filepath.Join(parseRemoteValues(out)["RUN"], "additional-folders"))
	if err != nil || len(folders) != 0 {
		t.Fatal("expected no additional mounts", string(folders), err)
	}
}

func TestRemoteAuthenticationHookPreservesInputAndProtocolOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX SSH hook shell fixture")
	}
	home := remoteTestHome(t)
	bin := filepath.Join(home, "bin")
	os.MkdirAll(bin, 0700)
	fake := `#!/bin/sh
null_input=false
for argument in "$@";do [ "$argument" != -n ] || null_input=true;done
if [ "$null_input" = false ];then cat >/dev/null;fi
printf 'authentication-hook banner\n'
`
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(fake), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	s := RemoteSession{Profile: remoteTestProfile(), Token: strings.Repeat("c", 32), JobID: "42", RunDir: "/shared/run with spaces", Port: 2222, User: "researcher"}
	key := base64.StdEncoding.EncodeToString(append([]byte{0, 0, 0, 11}, append([]byte("ssh-ed25519"), append([]byte{0, 0, 0, 32}, bytes.Repeat([]byte{1}, 32)...)...)...))
	if err := prepareRemoteSSH(s, "ssh-ed25519 "+key); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(settingsDir(), "remote-ssh", s.Token+".conf"))
	if err != nil {
		t.Fatal(err)
	}
	hook := ""
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "  LocalCommand ") {
			hook = strings.TrimPrefix(line, "  LocalCommand ")
		}
	}
	if hook == "" {
		t.Fatal("missing authentication hook")
	}
	// The command after the hook represents the container shell receiving its
	// script; the fake SSH faithfully consumes stdin unless -n is present.
	script := "set -- '42'\nprintf 'CONTAINER_OK\\n'\n"
	command := exec.Command("sh", "-c", hook+"\ncat")
	command.Stdin = strings.NewReader(script)
	var output, diagnostics bytes.Buffer
	command.Stdout = &output
	command.Stderr = &diagnostics
	if err := command.Run(); err != nil {
		t.Fatal(err)
	}
	if output.String() != script {
		t.Fatal("authentication hook consumed input or polluted protocol output")
	}
	if !strings.Contains(diagnostics.String(), "authentication-hook banner") {
		t.Fatal("hook diagnostics were lost")
	}
}

func TestRemoteContainerProofFailureDiagnostics(t *testing.T) {
	failure := errors.New("transport failed")
	if err := verifyRemoteContainerProof("", failure); !errors.Is(err, failure) {
		t.Fatal("lost SSH error", err)
	}
	for _, proof := range []string{"", " \n", "login banner\nCONTAINER_OK\n", "unexpected-secret-value"} {
		err := verifyRemoteContainerProof(proof, nil)
		if err == nil || strings.Contains(err.Error(), "<nil>") {
			t.Fatal("misleading or missing error", err)
		}
		if strings.Contains(err.Error(), "unexpected-secret-value") {
			t.Fatal("unexpected response leaked into error")
		}
	}
	if err := verifyRemoteContainerProof("CONTAINER_OK\n", nil); err != nil {
		t.Fatal(err)
	}
}
