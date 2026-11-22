package startui

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func pendingSSHChallenge(t *testing.T) *sshChallenge {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		sshPrompts.Lock()
		if len(sshPrompts.pending) > 0 {
			p := sshPrompts.pending[0]
			sshPrompts.Unlock()
			return p
		}
		sshPrompts.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("SSH challenge did not reach the UI")
	return nil
}
func answerSSHFixture(t *testing.T, id, response string, cancel bool) int {
	t.Helper()
	data, _ := json.Marshal(sshAnswer{ID: id, Response: response, Cancel: cancel})
	request := httptest.NewRequest("POST", "/remote/auth/answer", bytes.NewReader(data))
	recorder := httptest.NewRecorder()
	serveSSHAuthentication("remote/auth/answer", recorder, request)
	if strings.Contains(recorder.Body.String(), response) && response != "" {
		t.Fatal("response leaked into HTTP acknowledgement")
	}
	return recorder.Code
}
func TestSSHAskpassSingleUseChallengesAndNoPersistence(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	cmd := exec.Command("unused")
	closeBridge, err := configureSSHAskpass(cmd, "raven.example.org")
	if err != nil {
		t.Fatal(err)
	}
	defer closeBridge()
	var address string
	for _, entry := range cmd.Env {
		if strings.HasPrefix(entry, "VSCODE_SSH_ASKPASS_URL=") {
			address = strings.TrimPrefix(entry, "VSCODE_SSH_ASKPASS_URL=")
		}
	}
	if address == "" {
		t.Fatal("no helper bridge")
	}
	t.Setenv("VSCODE_SSH_ASKPASS_URL", address)
	for _, fixture := range []struct {
		prompt, response string
		cancel           bool
	}{{"Password:", "fixture-password", false}, {"Verification code:", "123456-fixture", false}, {"Password:", "", true}} {
		var output bytes.Buffer
		done := make(chan error, 1)
		go func() { done <- runSSHAskpass(fixture.prompt, &output) }()
		challenge := pendingSSHChallenge(t)
		recorder := httptest.NewRecorder()
		serveSSHAuthentication("remote/auth/prompt", recorder, httptest.NewRequest("GET", "/remote/auth/prompt", nil))
		if !strings.Contains(recorder.Body.String(), fixture.prompt) || strings.Contains(recorder.Body.String(), "fixture-password") {
			t.Fatal("bad prompt payload")
		}
		if recorder.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("prompt may be cached")
		}
		if answerSSHFixture(t, challenge.ID, fixture.response, fixture.cancel) != 200 {
			t.Fatal("answer rejected")
		}
		select {
		case err := <-done:
			if (err != nil) != fixture.cancel || output.String() != fixture.response {
				t.Fatal("helper response mismatch or cancellation ignored", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("helper did not finish")
		}
		if answerSSHFixture(t, challenge.ID, "replay", false) != 409 {
			t.Fatal("response was reusable")
		}
		for _, entry := range cmd.Env {
			if strings.Contains(entry, "fixture-password") || strings.Contains(entry, "123456-fixture") {
				t.Fatal("credential in environment")
			}
		}
	}
	files, err := os.ReadDir(root)
	if err != nil || len(files) != 0 {
		t.Fatal("authentication persisted files", err)
	}
	response, err := http.Post(address+"wrong", "application/json", strings.NewReader(`{"prompt":"Password:"}`))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("bridge capability not enforced")
	}
	var output bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- runSSHAskpass("Expired prompt:", &output) }()
	id := pendingSSHChallenge(t).ID
	closeBridge()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("closed bridge accepted authentication")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("closed bridge stranded helper")
	}
	if answerSSHFixture(t, id, "stale", false) != 409 {
		t.Fatal("closed bridge retained prompt")
	}
}

// Exercise the same executable helper entry point as Sandbox Desktop while
// substituting only the SSH server transport. No account/real secret is needed.
func init() {
	if os.Getenv("VSCODE_ASKPASS_TEST_HELPER") == "1" && os.Getenv("VSCODE_SSH_ASKPASS_URL") != "" {
		if HandleHelper() {
			os.Exit(0)
		}
		os.Exit(1)
	}
}
func TestSSHAskpassCommandPasswordThenMFA(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake SSH fixture uses a POSIX shell")
	}
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("VSCODE_ASKPASS_TEST_HELPER", "1")
	fixture := `#!/bin/sh
password=$("$SSH_ASKPASS" 'Password:') || exit 1
[ "$password" = fixture-password ] || exit 1
unset password
code=$("$SSH_ASKPASS" 'Verification code:') || exit 1
[ "$code" = fixture-code ] || exit 1
unset code
exec /bin/sh -s --
`
	if err := os.WriteFile(filepath.Join(root, "ssh"), []byte(fixture), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var output string
	var commandErr error
	done := make(chan struct{})
	go func() { output, commandErr = loginSSH(ctx, "fixture", "printf authenticated"); close(done) }()
	for _, response := range []string{"fixture-password", "fixture-code"} {
		prompt := pendingSSHChallenge(t)
		if answerSSHFixture(t, prompt.ID, response, false) != 200 {
			t.Fatal("answer failed")
		}
	}
	select {
	case <-done:
		if commandErr != nil || output != "authenticated" {
			t.Fatal("SSH helper exchange failed", commandErr)
		}
	case <-ctx.Done():
		t.Fatal("SSH command did not finish")
	}
	done = make(chan struct{})
	go func() { _, commandErr = loginSSH(ctx, "fixture", "true"); close(done) }()
	prompt := pendingSSHChallenge(t)
	answerSSHFixture(t, prompt.ID, "", true)
	select {
	case <-done:
		if commandErr == nil {
			t.Fatal("cancellation did not stop SSH")
		}
	case <-ctx.Done():
		t.Fatal("cancelled SSH remained running")
	}
}
