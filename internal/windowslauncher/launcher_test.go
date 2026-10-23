package windowslauncher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	licensing "github.com/mpg-age-bioinformatics/vscode-sandboxes"
)

func TestIsUNCPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{`C:\Users\alice\project`, false},
		{`D:/work/project`, false},
		{`\\server\share\project`, true},
		{`//wsl.localhost/Ubuntu/home/alice/project`, true},
	}
	for _, test := range tests {
		if got := isUNCPath(test.path); got != test.want {
			t.Errorf("isUNCPath(%q) = %v, want %v", test.path, got, test.want)
		}
	}
}

func TestInvokedAsProjectRunner(t *testing.T) {
	config := Config{ProjectRunner: "Run Python Sandbox.exe"}
	if !invokedAsProjectRunner(config, `C:\work\code\Run Python Sandbox.exe`) {
		t.Fatal("project runner filename was not detected")
	}
	if invokedAsProjectRunner(config, `C:\Downloads\Python-Sandbox.exe`) {
		t.Fatal("installer filename was incorrectly detected as a project runner")
	}
}

func TestEnsureJSONCStringSettingAddsSettingAndPreservesComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Code", "User", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	original := "{\n  // Keep this comment.\n  \"editor.fontSize\": 14\n}\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	changed, err := ensureJSONCStringSetting(path, "remote.SSH.path", `C:\Windows\System32\OpenSSH\ssh.exe`)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("setting was not added")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "// Keep this comment.") || !strings.Contains(text, `"remote.SSH.path": "C:\\Windows\\System32\\OpenSSH\\ssh.exe"`) {
		t.Fatalf("unexpected settings contents:\n%s", text)
	}
}

func TestEnsureJSONCStringSettingPreservesExplicitSetting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	original := "{\n  \"remote.SSH.path\": \"C:\\\\custom\\\\ssh.exe\"\n}\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	changed, err := ensureJSONCStringSetting(path, "remote.SSH.path", `C:\Windows\System32\OpenSSH\ssh.exe`)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("explicit user setting was overwritten")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Fatal("settings file changed")
	}
}

// Notices must remain available on unsupported hosts and before any setup UI.
func TestLicensesFlagBypassesSetup(t *testing.T) {
	output, err := os.CreateTemp(t.TempDir(), "licenses-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	previousArgs, previousStdout := os.Args, os.Stdout
	os.Args, os.Stdout = []string{"Run Python Sandbox.exe", "--licenses"}, output
	defer func() { os.Args, os.Stdout = previousArgs, previousStdout }()
	Main(Config{})
	got, err := os.ReadFile(output.Name())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != licensing.License+"\n"+licensing.ThirdPartyNotices {
		t.Fatal("--licenses did not print both complete license documents")
	}
}
