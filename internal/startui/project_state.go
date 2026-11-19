package startui

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type ProjectState struct {
	DirectoryID string `json:"directoryID"`
	Project     string `json:"project"`
	Sandbox     string `json:"sandbox"`
	Version     string `json:"version"`
	SandboxName string `json:"sandboxName,omitempty"`
	Commit      string `json:"commit"`
	Ready       bool   `json:"ready"`
}

func projectKey(project string) string {
	project = resolveExistingParent(project)
	if runtime.GOOS == "windows" {
		project = strings.ToLower(project)
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(project)))
}
func projectStatePath(project string) string {
	return filepath.Join(settingsDir(), "projects", projectKey(project)+".json")
}
func projectGit(project string, args ...string) (string, error) {
	executable := "git"
	if runtime.GOOS == "windows" {
		if _, err := exec.LookPath(executable); err != nil {
			for _, root := range []string{filepath.Join(os.Getenv("ProgramFiles"), "Git"), filepath.Join(os.Getenv("LocalAppData"), "Programs", "Git")} {
				candidate := filepath.Join(root, "cmd", "git.exe")
				if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
					executable = candidate
					break
				}
			}
		}
	}
	cmd := Command(executable, append([]string{"-C", project}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s", args[0], strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
func ProjectScript(project, slug string) string {
	name := strings.TrimSuffix(slug, "-sandbox")
	return filepath.Join(project, "code", ".sandbox", "run-"+name+"-sandbox.sh")
}

// A saved success record is bound to the directory identity and setup commit, so deleting
// and recreating a folder cannot accidentally select a different old sandbox.
func ExistingProject(project string) *ProjectState {
	if !filepath.IsAbs(project) {
		return nil
	}
	top, err := projectGit(project, "rev-parse", "--show-toplevel")
	if err != nil || projectKey(top) != projectKey(project) {
		return nil
	}
	var state ProjectState
	if readJSON(projectStatePath(project), &state) == nil && projectKey(state.Project) == projectKey(project) && state.Ready && state.Commit != "" && state.DirectoryID != "" && state.DirectoryID == projectDirectoryID(project) {
		if _, err := FindSandbox(state.Sandbox); err == nil {
			if _, err := os.Stat(ProjectScript(project, state.Sandbox)); err == nil {
				if _, err := projectGit(project, "merge-base", "--is-ancestor", state.Commit, "HEAD"); err == nil {
					return &state
				}
			}
		}
	}
	// Older launchers have generated projects but no success record. Reuse their
	// scripts without overwriting them, and run verification before recording success.
	var metadata struct {
		Language string `json:"language"`
		Series   string `json:"series"`
	}
	data, err := os.ReadFile(filepath.Join(project, "code", ".sandbox", "sandbox-runtime.json"))
	if err == nil && json.Unmarshal(data, &metadata) == nil && (metadata.Language == "python" || metadata.Language == "r") {
		state = ProjectState{Project: project, Sandbox: metadata.Language + "-sandbox", Version: metadata.Series}
	} else {
		state = ProjectState{Project: project, Sandbox: "bioinformatics-sandbox"}
	}
	if _, err := os.Stat(ProjectScript(project, state.Sandbox)); err != nil {
		return nil
	}
	head, err := projectGit(project, "rev-parse", "HEAD")
	if err != nil {
		return nil
	}
	state.Commit = head
	return &state
}

func rememberProject(s Selection, name string) error {
	head, err := projectGit(s.Project, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	return writeJSON(projectStatePath(s.Project), ProjectState{DirectoryID: projectDirectoryID(s.Project), Project: s.Project, Sandbox: s.Sandbox, Version: s.Version, SandboxName: name, Commit: head, Ready: true})
}

var runtimeBaselinePaths = []string{"code/.sandbox/devcontainer.json", "code/.sandbox/container-image.lock.json", "code/.sandbox/bioconductor-config.yaml", ".Rprofile", ".Renviron", "renv.lock", "renv/activate.R", "renv/settings.json", "renv/.gitignore"}

// Finalize only the first setup. Later starts must leave user edits and the
// staging area alone. A new repository retains one initial setup commit.
func finalizeProject(s Selection, name string, log io.Writer) error {
	if old := ExistingProject(s.Project); old != nil && old.Ready {
		return nil
	}
	var paths []string
	if s.FreshRepository {
		paths = []string{"."}
	} else {
		for _, path := range runtimeBaselinePaths {
			preserve := false
			for _, dirty := range s.PreservePaths {
				if path == dirty {
					preserve = true
				}
			}
			if !preserve {
				if _, err := os.Stat(filepath.Join(s.Project, path)); err == nil {
					paths = append(paths, path)
				}
			}
		}
	}
	if len(paths) > 0 {
		if _, err := projectGit(s.Project, append([]string{"add", "-A", "--"}, paths...)...); err != nil {
			return err
		}
		changed, err := projectGit(s.Project, append([]string{"diff", "--cached", "--name-only", "--"}, paths...)...)
		if err != nil {
			return err
		}
		if changed != "" {
			args := []string{"commit", "--only", "-m", "Finalize sandbox setup"}
			if s.FreshRepository {
				args = []string{"commit", "--amend", "--no-edit"}
			}
			args = append(args, "--")
			args = append(args, paths...)
			if _, err := projectGit(s.Project, args...); err != nil {
				return err
			}
		}
	}
	fmt.Fprintln(log, "Sandbox setup committed. The workspace is ready.")
	return rememberProject(s, name)
}
