package startui

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Emulate the sbx boundary, executing only its sandbox-side commands against a
// temporary workspace. Host Git/Bash invocations are traps. Real Git operates
// on local fixture repositories only; no live sandbox or network is used.
func nativeFixture(t *testing.T, slug string) (Selection, string, string) {
	t.Helper()
	isolatedSettings(t)
	project := filepath.Join(t.TempDir(), "project with spaces & literal")
	if err := os.MkdirAll(project, 0755); err != nil {
		t.Fatal(err)
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	sandboxBin, hostBin := t.TempDir(), t.TempDir()
	// Keep separate search paths for the host controller and sandbox processes.
	if err := os.Symlink(git, filepath.Join(sandboxBin, "git")); err != nil {
		t.Fatal(err)
	}
	sandboxPath := sandboxBin + string(os.PathListSeparator) + os.Getenv("PATH")
	upstream := t.TempDir()
	os.MkdirAll(filepath.Join(upstream, "match-code-data"), 0755)
	os.MkdirAll(filepath.Join(upstream, "set-vscode-r-version"), 0755)
	os.WriteFile(filepath.Join(upstream, "match-code-data", "SKILL.md"), []byte("fixture"), 0644)
	os.WriteFile(filepath.Join(upstream, "set-vscode-r-version", "SKILL.md"), []byte("fixture"), 0644)
	for _, args := range [][]string{{"init"}, {"config", "user.name", "Fixture"}, {"config", "user.email", "fixture@localhost"}, {"add", "."}, {"commit", "-m", "Fixture"}} {
		if _, err := localGit(upstream, args...); err != nil {
			t.Fatal(err)
		}
	}
	log := filepath.Join(t.TempDir(), "commands.log")
	state := filepath.Join(t.TempDir(), "sandbox-name")
	for _, key := range []string{"PYTHON_SANDBOX_SKILLS_REPOSITORY", "R_SANDBOX_SKILLS_REPOSITORY", "BIOINFORMATICS_SANDBOX_SKILLS_REPOSITORY"} {
		t.Setenv(key, upstream)
	}
	t.Setenv("NATIVE_TEST_PROJECT", project)
	t.Setenv("NATIVE_TEST_LOG", log)
	t.Setenv("NATIVE_TEST_STATE", state)
	t.Setenv("NATIVE_TEST_SANDBOX_PATH", sandboxPath)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("HOME", t.TempDir())
	script := `
import os, sys, subprocess, json
args=sys.argv[1:]
with open(os.environ['NATIVE_TEST_LOG'],'a') as f: f.write(json.dumps(args)+'\n')
project=os.environ['NATIVE_TEST_PROJECT']
state=os.environ['NATIVE_TEST_STATE']
if args[0]=='version': print('Client Version: v0.39.0')
elif args[0]=='daemon': print('{"status":"running"}')
elif args[0] in ('diagnose','setup','stop'): pass
elif args[0]=='ls':
    if os.path.exists(state): print(open(state).read())
elif args[0]=='run':
    name=args[args.index('--name')+1]
    if not os.path.exists(state):
        assert args[-1]==project
        assert args[args.index('--deny-network')+1]=='**'
    open(state,'w').write(name)
elif args[0]=='policy':
    if os.environ.get('NATIVE_TEST_FAIL_POLICY')=='1': sys.exit(23)
elif args[0]=='exec':
    rest=args[1:]
    while rest[0].startswith('-'):
        flag=rest.pop(0)
        if flag in ('--workdir','--user'): rest.pop(0)
    name=rest.pop(0)
    if rest==['sh','-c','pwd -P']: print(os.path.realpath(project))
    elif rest[0]=='git' or (rest[:2]==['sh','-c'] and '/tmp/sandbox-setup.' in rest[2]):
        env=dict(os.environ,PATH=os.environ['NATIVE_TEST_SANDBOX_PATH'])
        result=subprocess.run(rest,cwd=project,env=env)
        sys.exit(result.returncode)
    elif rest[:2]==['sh','-c']: sys.stdin.buffer.read() # connection config, no project writes
    elif rest[:2]==['sh','-s']:
        content=sys.stdin.buffer.read().decode()
        assert 'podman --storage-driver=vfs' in content
    elif rest[0]=='bash':
        if os.environ.get('NATIVE_TEST_FAIL_RUNTIME')=='1': sys.exit(24)
        if rest[1].endswith('start-development-container.sh'):
            assert rest[2]=='both'
            lock=os.path.join(project,'code/.sandbox/container-image.lock.json')
            if not os.path.exists(lock): open(lock,'w').write('{"imageId":"fixture"}')
    else: raise AssertionError(rest)
else: raise AssertionError(args)
`
	if err := os.WriteFile(filepath.Join(hostBin, "sbx"), []byte("#!"+python+"\n"+script), 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"git", "bash", "cygpath", "docker", "podman"} {
		os.WriteFile(filepath.Join(hostBin, name), []byte("#!/bin/sh\necho 'Forbidden host tool: "+name+"' >&2\nexit 99\n"), 0755)
	}
	t.Setenv("PATH", hostBin)
	t.Setenv("PYTHON_SANDBOX_SKIP_VSCODE", "1")
	t.Setenv("R_SANDBOX_SKIP_VSCODE", "1")
	t.Setenv("BIOINFORMATICS_SANDBOX_SKIP_VSCODE", "1")
	resources := t.TempDir()
	for _, file := range []string{"Sandbox Desktop", "Sandbox Window", "PythonSandbox.icns", "RSandbox.icns", "BioinformaticsSandbox.icns"} {
		os.WriteFile(filepath.Join(resources, file), []byte("fixture native app resource"), 0755)
	}
	s := selection()
	s.Project = project
	s.Sandbox = slug
	s.Agent = "both"
	if slug == "r-sandbox" {
		s.Version = "4.5"
	}
	return s, filepath.Join(resources, "Sandbox Desktop"), log
}

func TestNativeStartupWithoutHostGitOrBash(t *testing.T) {
	for _, slug := range []string{"python-sandbox", "r-sandbox", "bioinformatics-sandbox"} {
		t.Run(slug, func(t *testing.T) {
			s, executable, audit := nativeFixture(t, slug)
			var log bytes.Buffer
			if err := launchSandbox(s, &log, executable); err != nil {
				t.Fatalf("%v\n%s", err, &log)
			}
			commands, _ := os.ReadFile(audit)
			if !strings.Contains(string(commands), "/tmp/sandbox-setup.") {
				t.Fatal("setup did not execute inside sbx")
			}
			state := ExistingProject(s.Project)
			if state == nil || !state.Ready || state.SandboxName == "" {
				t.Fatal("missing ready project", state)
			}
			// Use fixture Git directly, never the host controller's PATH.
			git := func(args ...string) string {
				cmd := exec.Command(filepath.Join(strings.Split(os.Getenv("NATIVE_TEST_SANDBOX_PATH"), string(os.PathListSeparator))[0], "git"), append([]string{"-C", s.Project}, args...)...)
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("%v: %s", err, out)
				}
				return strings.TrimSpace(string(out))
			}
			if count := git("rev-list", "--count", "HEAD"); count != "1" {
				t.Fatal("extra initial commits", count)
			}
			if dirty := git("status", "--porcelain"); dirty != "" {
				t.Fatal("dirty initial setup", dirty)
			}
			os.WriteFile(filepath.Join(s.Project, "code", "user.py"), []byte("user work"), 0644)
			git("add", "code/user.py")
			before, head := git("status", "--porcelain"), git("rev-parse", "HEAD")
			os.WriteFile(audit, nil, 0644)
			if err := launchSandbox(s, &log, executable); err != nil {
				t.Fatalf("restart: %v\n%s", err, &log)
			}
			if after := git("status", "--porcelain"); after != before {
				t.Fatal("restart changed staged work", after, before)
			}
			if git("rev-parse", "HEAD") != head {
				t.Fatal("restart changed history")
			}
			commands, _ = os.ReadFile(audit)
			if strings.Contains(string(commands), "/tmp/sandbox-setup.") {
				t.Fatal("restart reran scaffolding")
			}
		})
	}
}

func TestNativePolicyAndRuntimeFailuresPreventSetupSuccess(t *testing.T) {
	for _, failure := range []string{"NATIVE_TEST_FAIL_POLICY", "NATIVE_TEST_FAIL_RUNTIME"} {
		t.Run(failure, func(t *testing.T) {
			s, executable, audit := nativeFixture(t, "python-sandbox")
			t.Setenv(failure, "1")
			var log bytes.Buffer
			if err := launchSandbox(s, &log, executable); err == nil {
				t.Fatal("accepted failed startup")
			}
			if state := ExistingProject(s.Project); state != nil && state.Ready {
				t.Fatal("marked failed startup ready")
			}
			commands, _ := os.ReadFile(audit)
			if failure == "NATIVE_TEST_FAIL_POLICY" && strings.Contains(string(commands), "/tmp/sandbox-setup.") {
				t.Fatal("setup ran after policy failure")
			}
		})
	}
}

func TestWindowsSetupRunnerWithoutHostShell(t *testing.T) {
	s, executable, _ := nativeFixture(t, "python-sandbox")
	var log bytes.Buffer
	if err := setupInSandbox("fixture", s.Project, s, executable, "windows", &log); err != nil {
		t.Fatalf("%v\n%s", err, &log)
	}
	runner := filepath.Join(s.Project, "code/.sandbox/Run Python Sandbox.exe")
	got, err := os.ReadFile(runner)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := os.ReadFile(executable)
	if !bytes.Equal(got, want) {
		t.Fatal("Windows runner differs from bundled executable")
	}
}

func TestWindowsEditorBypassesCommandShell(t *testing.T) {
	root := t.TempDir()
	install := filepath.Join(root, "Programs", "Microsoft VS Code")
	cli := filepath.Join(install, "resources", "app", "out", "cli.js")
	os.MkdirAll(filepath.Dir(cli), 0755)
	os.WriteFile(cli, []byte("fixture"), 0644)
	os.WriteFile(filepath.Join(install, "Code.exe"), []byte("fixture"), 0755)
	t.Setenv("LocalAppData", root)
	t.Setenv("PATH", t.TempDir())
	literal := "vscode-remote://ssh-remote+fixture/workspace/literal & space"
	cmd, err := editorCommandForPlatform("windows", "--folder-uri", literal)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Path != filepath.Join(install, "Code.exe") || len(cmd.Args) != 4 || cmd.Args[3] != literal {
		t.Fatal(cmd.Path, cmd.Args)
	}
	if !strings.Contains(strings.Join(cmd.Env, "\n"), "ELECTRON_RUN_AS_NODE=1") {
		t.Fatal("missing native CLI mode")
	}
}

func TestNativeRejectsStaleSetupHistoryBeforeRuntime(t *testing.T) {
	s, executable, audit := nativeFixture(t, "python-sandbox")
	var log bytes.Buffer
	if err := launchSandbox(s, &log, executable); err != nil {
		t.Fatalf("%v\n%s", err, &log)
	}
	var state ProjectState
	if err := readJSON(projectStatePath(s.Project), &state); err != nil {
		t.Fatal(err)
	}
	// Simulate a saved setup revision that is absent from the repository.
	state.Commit = strings.Repeat("0", 40)
	if err := writeJSON(projectStatePath(s.Project), state); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(audit, nil, 0644)
	err := launchSandbox(s, &log, executable)
	if err == nil || !strings.Contains(err.Error(), "saved setup commit") {
		t.Fatal("accepted stale history", err)
	}
	commands, _ := os.ReadFile(audit)
	if strings.Contains(string(commands), "start-development-container.sh") {
		t.Fatal("initialized runtime before checking repository history")
	}
}
