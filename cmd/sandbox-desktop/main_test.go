package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mpg-age-bioinformatics/vscode-sandboxes/internal/startui"
)

func TestUnifiedMacWorkerArguments(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("macOS shell dispatcher")
	}
	worker := filepath.Join(t.TempDir(), "Selected Sandbox.command")
	script := "#!/bin/bash\nset -eu\n[[ -f \"$VSCODE_SANDBOX_STARTUP_SELECTION\" ]]\n[[ \"$PWD\" == \"$(dirname -- \"$0\")\" ]]\nprintf '<%s>\\n' \"$@\"\n"
	if err := os.WriteFile(worker, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	for _, item := range startui.Catalog() {
		s := startui.Selection{Sandbox: item.Slug, Version: item.Version, Project: filepath.Join(t.TempDir(), "project with spaces"), Agent: "codex", Network: "open", Endpoint: "default"}
		var out bytes.Buffer
		if err := runWorker(worker, s, item.VersionLabel != "", false, &out); err != nil {
			t.Fatal(item.Slug, err)
		}
		want := "<codex>\n<" + s.Project + ">\n"
		if item.VersionLabel != "" {
			want = "<" + item.Version + ">\n" + want
		}
		if out.String() != want {
			t.Fatalf("%s: %q, want %q", item.Slug, out.String(), want)
		}
		out.Reset()
		if err := runWorker(worker, s, false, true, &out); err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(out.String()) != "<codex>" {
			t.Fatal("project mode arguments:", out.String())
		}
	}
}

func TestExistingProjectSkipsScaffolding(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("macOS dispatcher")
	}
	project := t.TempDir()
	folder := filepath.Join(project, "code", ".sandbox")
	os.MkdirAll(folder, 0755)
	os.WriteFile(filepath.Join(folder, "sandbox-runtime.json"), []byte(`{"language":"python","series":"3.13"}`), 0644)
	os.WriteFile(filepath.Join(folder, "run-python-sandbox.sh"), []byte("#!/bin/bash\necho reused-project\n"), 0755)
	for _, args := range [][]string{{"init"}, {"config", "user.name", "Test"}, {"config", "user.email", "test@localhost"}, {"add", "."}, {"commit", "-m", "Setup"}} {
		cmd := exec.Command("git", append([]string{"-C", project}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatal(err, string(out))
		}
	}
	s := startui.Selection{Sandbox: "python-sandbox", Version: "3.13", Project: project, Agent: "both", Network: "open", Endpoint: "default"}
	var log bytes.Buffer
	if err := runWorker("/never-run-scaffolding.command", s, true, false, &log); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(log.String(), "reused-project") {
		t.Fatal("did not run the existing project launcher", log.String())
	}
}
