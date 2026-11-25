package startui

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/mpg-age-bioinformatics/vscode-sandboxes/internal/launcherbundle"
)

// LaunchSandbox dispatches native host processes only. Shell scripts, Git,
// package managers and container engines execute inside the sandbox.
func LaunchSandbox(s Selection, log io.Writer) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return launchSandbox(s, log, executable)
}

func launchSandbox(s Selection, log io.Writer, executable string) error {
	if old := ExistingProject(s.Project); old != nil {
		s.Sandbox, s.Version = old.Sandbox, old.Version
	}
	item, err := FindSandbox(s.Sandbox)
	if err != nil {
		return err
	}
	s.Agent = "both"
	if item.VersionLabel != "" && !regexp.MustCompile(`^[0-9]+\.[0-9]+(\.[0-9]+)?$`).MatchString(s.Version) {
		return fmt.Errorf("Enter a %s in major.minor or major.minor.patch format.", item.VersionLabel)
	}
	if err := CheckProjectAccess(s.Project); err != nil {
		return err
	}
	if runtime.GOOS == "windows" && strings.HasPrefix(strings.ReplaceAll(s.Project, "/", `\`), `\\`) {
		return fmt.Errorf("Choose a project folder on a local Windows drive; network and WSL folders are not supported.")
	}
	env, cleanup, err := PrepareEnvironment(s)
	if err != nil {
		return err
	}
	defer cleanup()
	// Read the normalized, private selection without exporting it to a shell.
	for _, entry := range env {
		if path, ok := strings.CutPrefix(entry, "VSCODE_SANDBOX_STARTUP_SELECTION="); ok {
			if err := readJSON(path, &s); err != nil {
				return err
			}
		}
	}
	version, err := output("version")
	if err != nil {
		return fmt.Errorf("Docker Sandboxes is required: %w", err)
	}
	match := regexp.MustCompile(`(?i)(?:Client Version:|sbx version:)\s+v?(\d+)\.(\d+)\.(\d+)`).FindStringSubmatch(version)
	supported := len(match) == 4
	if supported {
		major, _ := strconv.Atoi(match[1])
		minor, _ := strconv.Atoi(match[2])
		supported = major > 0 || minor >= 39
	}
	if !supported {
		return fmt.Errorf("Docker Sandboxes 0.39.0 or newer is required. Detected: %s", strings.TrimSpace(version))
	}
	if err := EnsureHost(log); err != nil {
		return err
	}
	if err := command(log, "setup", "ssh"); err != nil {
		return err
	}

	old := ExistingProject(s.Project)
	prefix := strings.TrimSuffix(s.Sandbox, "-sandbox")
	name := prefix + "-" + projectKey(s.Project)[:16] + "-" + directoryKey(s.Project)[:8]
	if old != nil && old.Ready && old.SandboxName != "" {
		name = old.SandboxName
	}
	fmt.Fprintln(log, "Sandbox:", name)
	listing, err := output("ls", "--quiet")
	if err != nil {
		return err
	}
	exists := false
	for _, line := range strings.Split(listing, "\n") {
		if strings.TrimSpace(line) == name {
			exists = true
		}
	}
	if exists {
		if err := command(log, "stop", name); err != nil {
			return err
		}
		if err := guardNetwork(name, log); err != nil {
			return err
		}
		if err := command(log, "run", "--detached", "--name", name); err != nil {
			return err
		}
	} else if err := command(log, "run", "--detached", "--deny-network", "**", "--name", name, "shell", s.Project); err != nil {
		return err
	}
	if err := applyStartup(name, "both", s, log); err != nil {
		return err
	}
	workspace, err := sandboxWorkspace(name)
	if err != nil {
		return err
	}
	if _, err := sandboxGit(name, "--version"); err != nil {
		return fmt.Errorf("Git must be available inside the shell sandbox: %w", err)
	}
	if old != nil && old.Ready {
		if _, err := sandboxGit(name, "merge-base", "--is-ancestor", old.Commit, "HEAD"); err != nil {
			return fmt.Errorf("The saved setup commit is missing from this repository. Verify the project history before restarting: %w", err)
		}
	}
	// Capture repository state inside sbx before scaffolding or initialization.
	s.FreshRepository, s.PreservePaths = false, nil
	top, topErr := sandboxGit(name, "rev-parse", "--show-toplevel")
	_, headErr := sandboxGit(name, "rev-parse", "HEAD")
	if topErr == nil && top != workspace {
		return fmt.Errorf("Choose the repository root as the project folder.")
	}
	if _, statErr := os.Stat(filepath.Join(s.Project, ".git")); statErr == nil && topErr != nil {
		return fmt.Errorf("Cannot read the project repository inside the sandbox. Repair its Git configuration before retrying.")
	}
	s.FreshRepository = topErr != nil || headErr != nil

	if !s.FreshRepository && (old == nil || !old.Ready) {
		for _, path := range runtimeBaselinePaths {
			dirty, err := sandboxGit(name, "status", "--porcelain", "--", path)
			if err != nil {
				return err
			}
			if dirty != "" {
				s.PreservePaths = append(s.PreservePaths, path)
			}
		}
	}
	if old == nil {
		fmt.Fprintln(log, "Preparing project files and cloning skills inside the sandbox…")
		if err := setupInSandbox(name, workspace, s, executable, runtime.GOOS, log); err != nil {
			return err
		}
	} else {
		fmt.Fprintln(log, "Reopening the existing sandbox project; skipping project scaffolding.")
	}
	if s.Sandbox == "bioinformatics-sandbox" {
		if err := runSandboxAsset(name, workspace, "initialize-bioinformatics.sh", true, log); err != nil {
			return err
		}
		for _, script := range []string{"install-coding-agents.sh", "setup-continue.sh"} {
			if err := command(log, "exec", "--workdir", workspace, name, "bash", "code/.sandbox/"+script); err != nil {
				return err
			}
		}
	} else {
		resume := "0"
		if exists && old != nil && old.Ready {
			resume = "1"
		}
		if err := command(log, "exec", "--workdir", workspace, name, "bash", "code/.sandbox/start-development-container.sh", "both", name, resume); err != nil {
			return err
		}
	}
	if err := applyLocalAgentNetwork(name, workspace, s, log); err != nil {
		return err
	}
	if err := finalizeProject(s, name, log); err != nil {
		return err
	}
	return openSandboxEditor(name, workspace, s.Sandbox, log)
}

func directoryKey(project string) string {
	// Directory identity keeps a recreated folder from reusing a removed project's VM.
	return fmt.Sprintf("%x", sha256.Sum256([]byte(projectDirectoryID(project))))
}

func sandboxWorkspace(name string) (string, error) {
	out, err := output("exec", name, "sh", "-c", "pwd -P")
	workspace := strings.TrimSpace(out)
	if err != nil || !strings.HasPrefix(workspace, "/") || strings.ContainsAny(workspace, "\r\n") {
		return "", fmt.Errorf("Cannot determine the sandbox workspace path: %s", workspace)
	}
	return workspace, nil
}

func runSandboxAsset(name, workspace, filename string, root bool, log io.Writer) error {
	data, err := launcherbundle.CommonAsset(filename)
	if err != nil {
		return err
	}
	args := []string{"exec", "-i", "--workdir", workspace}
	if root {
		args = append(args, "--user", "root")
	}
	cmd := Command("sbx", append(args, name, "sh", "-s")...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = bytes.NewReader(data), log, log
	return cmd.Run()
}

func setupInSandbox(name, workspace string, s Selection, executable, platform string, log io.Writer) error {
	dir, err := os.MkdirTemp("", "sandbox-payload-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	setup := filepath.Join(dir, "setup")
	if err := launcherbundle.Materialize(s.Sandbox, setup); err != nil {
		return err
	}
	label := map[string]string{"python-sandbox": "Python", "r-sandbox": "R", "bioinformatics-sandbox": "Bioinformatics"}[s.Sandbox]

	if platform == "windows" {
		if err := copyLaunchFile(executable, filepath.Join(dir, "runner.exe")); err != nil {
			return err
		}

	} else if platform == "darwin" {
		template := filepath.Join(setup, "assets", "Run "+label+" Sandbox.app.template")
		resources := filepath.Dir(executable)
		// Both primary and project apps place the desktop helpers in Resources.
		for _, binary := range []string{"Sandbox Desktop", "Sandbox Window"} {
			if err := copyLaunchFile(filepath.Join(resources, binary), filepath.Join(template, "Contents", "Resources", binary)); err != nil {
				return err
			}
		}
		icon := filepath.Join(resources, label+"Sandbox.icns")
		if _, err := os.Stat(icon); err != nil {
			icon = filepath.Join(resources, "Launchers", s.Sandbox, label+"Sandbox.icns")
		}
		if err := copyLaunchFile(icon, filepath.Join(dir, "icon.icns")); err != nil {
			return err
		}
		platform = "macos"
	}
	var archive bytes.Buffer
	tw := tar.NewWriter(&archive)
	err = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == dir {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relative)
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(tw, file)
		return err
	})
	if err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	// Extract into sandbox-local /tmp. No host path, shell, tar, Git, or shared
	// staging directory is needed, and temporary setup inputs are never committed.
	script := `set -eu
payload="$(mktemp -d /tmp/sandbox-setup.XXXXXX)"
trap 'rm -rf -- "$payload"' EXIT
tar -xf - -C "$payload"
platform="$1"; label="$2"; version="$3"; slug="$4"; shift 4
if [ "$platform" = windows ]; then
  export VSCODE_SANDBOX_PROJECT_RUNNER_SOURCE="$payload/runner.exe"
  export VSCODE_SANDBOX_PROJECT_RUNNER_AGENT=both
elif [ "$platform" = macos ]; then
  export VSCODE_SANDBOX_PROJECT_APP_TEMPLATE="$payload/setup/assets/Run $label Sandbox.app.template"
  export VSCODE_SANDBOX_PROJECT_APP_ICON="$payload/icon.icns"
  export VSCODE_SANDBOX_DESKTOP_RESOURCES="$VSCODE_SANDBOX_PROJECT_APP_TEMPLATE/Contents/Resources"
fi
if [ "$slug" = bioinformatics-sandbox ]; then
  bash "$payload/setup/scripts/setup-project.sh" "$PWD" both
else
  bash "$payload/setup/scripts/setup-project.sh" "$PWD" "$version" both
fi`
	for attempt := 0; attempt < 2; attempt++ {
		// Re-create stdin and the process: exec.Cmd and an exhausted tar reader are
		// single-use. The setup script preserves existing/nonidentical files.
		var directoryRace setupDirectoryRaceWriter
		cmd := Command("sbx", "exec", "-i", "--workdir", workspace, name, "sh", "-c", script, "setup", platform, label, s.Version, s.Sandbox)
		cmd.Stdin = bytes.NewReader(archive.Bytes())
		cmd.Stdout, cmd.Stderr = log, io.MultiWriter(log, &directoryRace)
		if err := cmd.Run(); err != nil {
			// The workspace bridge can fail the RPC before mkdir's shell-side recovery
			// runs. Retry once for this exact error, never general setup/Git failures.
			if attempt == 0 && directoryRace.found {
				fmt.Fprintln(log, "Retrying project setup after a temporary shared-folder directory conflict…")
				continue
			}
			return fmt.Errorf("Project setup inside the sandbox failed: %w", err)
		}
		return nil
	}
	return errors.New("Project setup retry exhausted")
}

// Detect the bridge's narrow retryable error without retaining arbitrary setup
// stderr. Keep only enough tail bytes to match across pipe-read boundaries.
type setupDirectoryRaceWriter struct {
	tail  string
	found bool
}

func (w *setupDirectoryRaceWriter) Write(data []byte) (int, error) {
	const message = "mkdir: Already exists"
	text := w.tail + string(data)
	w.found = w.found || strings.Contains(text, message)
	if len(text) >= len(message) {
		text = text[len(text)-len(message)+1:]
	}
	w.tail = text
	return len(data), nil
}

func copyLaunchFile(source, destination string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("Read launcher component %s: %w", source, err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return err
	}
	return os.WriteFile(destination, data, 0755)
}

func editorCommand(args ...string) (*exec.Cmd, error) {
	return editorCommandForPlatform(runtime.GOOS, args...)
}

func editorCommandForPlatform(platform string, args ...string) (*exec.Cmd, error) {
	if platform == "windows" {
		// Run VS Code's JavaScript CLI via its native executable, bypassing
		// code.cmd and cmd.exe so shell metacharacters remain literal arguments.
		roots := []string{filepath.Join(os.Getenv("LocalAppData"), "Programs", "Microsoft VS Code"), filepath.Join(os.Getenv("ProgramFiles"), "Microsoft VS Code")}
		if path, err := exec.LookPath("code.cmd"); err == nil {
			roots = append([]string{filepath.Dir(filepath.Dir(path))}, roots...)
		}
		for _, root := range roots {
			binary, cli := filepath.Join(root, "Code.exe"), filepath.Join(root, "resources", "app", "out", "cli.js")
			if _, err := os.Stat(cli); err != nil {
				continue
			}
			if _, err := os.Stat(binary); err != nil {
				continue
			}
			cmd := Command(binary, append([]string{cli}, args...)...)
			cmd.Env = append(os.Environ(), "ELECTRON_RUN_AS_NODE=1")
			return cmd, nil
		}
	} else {
		candidates := []string{"code"}
		home, _ := os.UserHomeDir()
		for _, base := range []string{"/Applications", filepath.Join(home, "Applications"), filepath.Join(home, "Desktop")} {
			candidates = append(candidates, filepath.Join(base, "Visual Studio Code.app", "Contents", "Resources", "app", "bin", "code"))
		}
		for _, candidate := range candidates {
			if path, err := exec.LookPath(candidate); err == nil {
				return Command(path, args...), nil
			}
		}
	}
	return nil, fmt.Errorf("Visual Studio Code is required; install it from https://code.visualstudio.com/")
}

func openSandboxEditor(name, workspace, slug string, log io.Writer) error {
	skip := map[string]string{"python-sandbox": "PYTHON_SANDBOX_SKIP_VSCODE", "r-sandbox": "R_SANDBOX_SKIP_VSCODE", "bioinformatics-sandbox": "BIOINFORMATICS_SANDBOX_SKIP_VSCODE"}[slug]
	if os.Getenv(skip) == "1" {
		return nil
	}
	run := func(args ...string) error {
		cmd, err := editorCommand(args...)
		if err != nil {
			return err
		}
		cmd.Stdout, cmd.Stderr = log, log
		return cmd.Run()
	}
	if err := run("--install-extension", "ms-vscode-remote.remote-ssh"); err != nil {
		return err
	}
	if slug != "bioinformatics-sandbox" {
		if err := run("--install-extension", "ms-vscode-remote.remote-containers"); err != nil {
			return err
		}
		uri, err := output("exec", "--workdir", workspace, name, "node", "code/.sandbox/development-container.js", "uri", name)
		uri = strings.TrimSpace(uri)
		if err != nil || !strings.HasPrefix(uri, "vscode-remote://dev-container+") || strings.ContainsAny(uri, "\r\n") {
			return fmt.Errorf("Invalid development-container URI: %s", uri)
		}
		return run("--folder-uri", uri)
	}
	authority := "ssh-remote+" + name + ".sbx"
	for _, extension := range []string{"ms-python.python", "ms-python.vscode-pylance", "ms-toolsai.jupyter", "REditorSupport.r", "openai.chatgpt", "anthropic.claude-code"} {
		if err := run("--remote", authority, "--install-extension", extension); err != nil {
			return err
		}
	}
	list, err := editorCommand("--remote", authority, "--list-extensions")
	if err != nil {
		return err
	}
	list.Stderr = log
	installed, err := list.Output()
	if err != nil {
		return fmt.Errorf("List sandbox VS Code extensions: %w", err)
	}
	extensions := map[string]bool{}
	for _, line := range strings.Split(strings.ToLower(string(installed)), "\n") {
		extensions[strings.TrimSpace(line)] = true
	}
	for _, extension := range []string{"ms-python.python", "ms-python.vscode-pylance", "ms-toolsai.jupyter", "reditorsupport.r", "openai.chatgpt", "anthropic.claude-code", "continue.continue"} {
		if !extensions[extension] {
			return fmt.Errorf("VS Code extension was not installed in %s: %s", authority, extension)
		}
	}
	return run("--remote", authority, workspace)
}
