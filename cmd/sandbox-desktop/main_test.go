package main

import (
	"bytes"
	"os"
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
	script := "#!/bin/bash\nset -eu\n[[ -f \"$VSCODE_SANDBOX_STARTUP_SELECTION\" ]]\nprintf '<%s>\\n' \"$@\"\n"
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
