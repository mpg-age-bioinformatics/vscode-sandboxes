package startui

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRemoteAuthenticationSocketIsolation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if runtime.GOOS == "windows" {
		t.Skip("Unix OpenSSH multiplexing")
	}
	first, err := remoteControlPath("alice@raven.example.org")
	if err != nil {
		t.Fatal(err)
	}
	same, _ := remoteControlPath("alice@raven.example.org")
	other, _ := remoteControlPath("bob@raven.example.org")
	if first != same || first == other {
		t.Fatal("socket not isolated per login")
	}
	info, _ := os.Stat(filepath.Dir(first))
	if info.Mode().Perm() != 0700 {
		t.Fatal("connection directory is not private")
	}
	for _, host := range []string{"-oProxyCommand=bad", "raven;touch bad", "raven\nother"} {
		if _, err := remoteControlPath(host); err == nil {
			t.Fatal("accepted SSH options or shell input", host)
		}
	}
	options, err := loginSSHOptions("alice@raven.example.org")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(options, " ")
	for _, expected := range []string{"ControlPath=" + first, "BatchMode=no", "StrictHostKeyChecking=yes", "ForwardAgent=no", "ControlPersist=600"} {
		if !strings.Contains(joined, expected) {
			t.Fatal("missing", expected, joined)
		}
	}
	proxy, err := loginSSHProxyOptions("alice@raven.example.org", remoteQuote)
	if err != nil || !strings.Contains(proxy, remoteQuote("ControlPath="+first)) {
		t.Fatal("relay uses different login transport", proxy, err)
	}
}
