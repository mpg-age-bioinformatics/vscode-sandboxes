package startui

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRemoteSSHStreamsLargeArguments(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake SSH uses bash; native Windows end-to-end testing remains separate")
	}
	root := t.TempDir()
	binary := filepath.Join(root, "ssh")
	argsFile := filepath.Join(root, "arguments")
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Fatal(err)
	}
	fake := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + remoteQuote(argsFile) + "\nexec " + remoteQuote(bash) + " -s --\n"
	if err := os.WriteFile(binary, []byte(fake), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	payload := strings.Repeat("literal ' \" ` $(false) \\ newline\n", 32768)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, err := loginSSH(ctx, "fixture", `set -eu; printf '%s' "$1"`, payload)
	if err != nil || output != payload {
		t.Fatal("streamed arguments were changed or evaluated", len(output), len(payload), err)
	}
	arguments, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	socket, err := remoteControlPath("fixture")
	if err != nil || !strings.Contains(string(arguments), "ControlPath="+socket) {
		t.Fatal("SSH did not use shared login socket", err, string(arguments))
	}
	if len(arguments) > 768 || !strings.HasSuffix(string(arguments), "fixture\nbash -s --\n") {
		t.Fatal("script payload entered the OS command line", len(arguments))
	}
}

func TestRemoteSSHStreamLiveHPC(t *testing.T) {
	host := os.Getenv("VSCODE_REMOTE_WATCHDOG_HPC")
	if host == "" {
		t.Skip("set VSCODE_REMOTE_WATCHDOG_HPC")
	}
	payload := strings.Repeat("literal ' \" ` $(false) \\ newline\n", 32768)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	output, err := loginSSH(ctx, host, `set -eu; printf '%s' "$1" | sha256sum`, payload)
	expected := fmt.Sprintf("%x", sha256.Sum256([]byte(payload)))
	if err != nil || !strings.HasPrefix(output, expected+" ") {
		t.Fatal("live streamed argument mismatch", output, err)
	}
}
