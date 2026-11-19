//go:build !windows

package startui

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}
