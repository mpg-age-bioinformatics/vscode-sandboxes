package main

import (
	"bytes"
	"github.com/mpg-age-bioinformatics/vscode-sandboxes/internal/startui"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMacDispatcherUsesNativeSandboxCommands(t *testing.T) {
	bin := t.TempDir()
	// No Git or Git Bash is on PATH. An unsupported sbx version stops before
	// any VM action, proving the old shell worker is never executed.
	os.WriteFile(filepath.Join(bin, "sbx"), []byte("#!/bin/sh\necho 'Client Version: v0.1.0'\n"), 0755)
	t.Setenv("PATH", bin)
	for _, item := range startui.Catalog() {
		s := startui.Selection{Sandbox: item.Slug, Version: item.Version, Project: t.TempDir(), Agent: "both", Network: "open", Endpoint: "default"}
		var log bytes.Buffer
		err := runWorker(s, &log)
		if err == nil || !strings.Contains(err.Error(), "0.39.0") {
			t.Fatalf("%s: %v", item.Slug, err)
		}
	}
}
