package startui

import (
	"os/exec"
	"runtime"
	"testing"
)

func TestRemoteWatchdog(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("server-side watchdog tests require POSIX flock")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable")
	}
	if out, err := exec.Command(python, "../../scripts/test-remote-watchdog.py").CombinedOutput(); err != nil {
		t.Fatalf("watchdog regression tests: %v\n%s", err, out)
	}
}
