package startui

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// CheckProjectAccess runs before starting the daemon or changing network policy.
// A successful stat or chdir alone does not establish access on macOS.
func CheckProjectAccess(project string) error {
	if !filepath.IsAbs(project) {
		return fmt.Errorf("Choose an absolute project folder.")
	}
	if err := os.MkdirAll(project, 0755); err != nil {
		return projectAccessError(project, err)
	}
	directory, err := os.Open(project)
	if err != nil {
		return projectAccessError(project, err)
	}
	_, err = directory.Readdirnames(1)
	directory.Close()
	if err != nil && err != io.EOF {
		return projectAccessError(project, err)
	}
	if runtime.GOOS == "darwin" {
		// Exercise getcwd in a child, as Git does during setup.
		cmd := exec.Command("/bin/pwd", "-P")
		cmd.Dir = project
		if _, err := cmd.Output(); err != nil {
			return projectAccessError(project, err)
		}
	}
	if _, err := os.Stat(filepath.Join(project, "skills", ".git")); err == nil {
		cmd := exec.Command("git", "-C", filepath.Join(project, "skills"), "rev-parse", "--git-dir")
		cmd.Dir = project
		hideWindow(cmd)
		if output, err := cmd.CombinedOutput(); err != nil {
			return projectAccessError(filepath.Join(project, "skills"), fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output))))
		}
	} else if !os.IsNotExist(err) {
		return projectAccessError(filepath.Join(project, "skills"), err)
	}
	probe, err := os.CreateTemp(project, ".sandbox-access-*")
	if err != nil {
		return projectAccessError(project, err)
	}
	closeErr := probe.Close()
	removeErr := os.Remove(probe.Name())
	if closeErr != nil {
		return projectAccessError(project, closeErr)
	}
	if removeErr != nil {
		return projectAccessError(project, removeErr)
	}
	return nil
}

func projectAccessError(project string, err error) error {
	help := "Choose an accessible, writable folder using Browse."
	if runtime.GOOS == "darwin" {
		help = "Recommended: choose a project under ~/sandboxes using Browse. If macOS denied access to your chosen location, allow folder access in System Settings → Privacy & Security → Files and Folders. If the app is absent and access remains blocked, you may need to add the installed launcher under Full Disk Access using +, enable it, then quit and reopen it. Full Disk Access grants broader access than the project needs."
	}
	return fmt.Errorf("Cannot prepare project folder %q: %w. %s", project, err, help)
}
