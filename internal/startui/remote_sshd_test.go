package startui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoteSSHEffectiveEnvironment(t *testing.T) {
	sshd, err := exec.LookPath("sshd")
	if err != nil {
		t.Skip("OpenSSH server not installed")
	}
	keygen, err := exec.LookPath("ssh-keygen")
	if err != nil {
		t.Skip("ssh-keygen not installed")
	}
	dir := t.TempDir()
	key := filepath.Join(dir, "host_key")
	if out, err := exec.Command(keygen, "-q", "-t", "ed25519", "-N", "", "-f", key).CombinedOutput(); err != nil {
		t.Fatal(string(out), err)
	}
	_, config, ok := strings.Cut(remoteAsset("container.sh"), "<<CONFIG\n")
	if !ok {
		t.Fatal("missing SSH configuration")
	}
	config, _, ok = strings.Cut(config, "\nCONFIG")
	if !ok {
		t.Fatal("missing configuration terminator")
	}
	config = strings.NewReplacer("$network_env", "VSCODE_SANDBOX_NETWORK=restricted HTTP_PROXY=http://127.0.0.1:3128 HTTPS_PROXY=http://127.0.0.1:3128 http_proxy=http://127.0.0.1:3128 https_proxy=http://127.0.0.1:3128 NODE_USE_ENV_PROXY=1", "$TMPDIR", dir, "$status", dir, "$run", dir, "$HOME", dir, "$job", "12345", "$port", "22222", "$user", "researcher").Replace(config)
	file := filepath.Join(dir, "sshd_config")
	if err := os.WriteFile(file, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(sshd, "-T", "-f", file).CombinedOutput()
	if err != nil {
		t.Fatal(string(out), err)
	}
	// OpenSSH accepts repeated SetEnv directives but silently uses only the first.
	// Validate its effective environment, not just the generated file contents.
	if !strings.Contains(string(out), "VSCODE_SANDBOX_REMOTE_CONTAINER=12345") || !strings.Contains(string(out), "PATH="+dir+"/.local/bin:") || !strings.Contains(string(out), "SHELL=/bin/bash") {
		t.Fatal("container identity or tool PATH missing from effective SSH environment", string(out))
	}
	for _, variable := range []string{"VSCODE_SANDBOX_NETWORK=restricted", "HTTP_PROXY=http://127.0.0.1:3128", "HTTPS_PROXY=http://127.0.0.1:3128", "NODE_USE_ENV_PROXY=1"} {
		if !strings.Contains(string(out), variable) {
			t.Fatal("network environment missing from effective SSH configuration", variable, string(out))
		}
	}
	for _, setting := range []string{"permituserrc yes", "clientaliveinterval 60", "clientalivecountmax 3", "logingracetime 60"} {
		if !strings.Contains(string(out), setting+"\n") {
			t.Fatalf("watchdog SSH setting missing: %s\n%s", setting, out)
		}
	}
}
