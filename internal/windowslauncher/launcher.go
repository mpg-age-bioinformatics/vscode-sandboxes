package windowslauncher

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	licensing "github.com/mpg-age-bioinformatics/vscode-sandboxes"
	"github.com/mpg-age-bioinformatics/vscode-sandboxes/internal/launcherbundle"
	"github.com/mpg-age-bioinformatics/vscode-sandboxes/internal/startui"
)

type Config struct {
	Unified        bool
	AppName        string
	SandboxSlug    string
	ProjectRunner  string
	ShellLauncher  string
	VersionPrompt  string
	NeedsVersion   bool
	NeedsPodman    bool
	DefaultVersion string
}

type setupSelection struct {
	version string
	agent   string
	project string
}

type dependencies struct {
	bash string
}

func Main(config Config) {
	if len(os.Args) == 2 && os.Args[1] == "--licenses" {
		fmt.Print(licensing.License, "\n", licensing.ThirdPartyNotices)
		return
	}
	if startui.HandleHelper() {
		return
	}
	// A unified executable is also copied to flavor-specific project-runner
	// names. Resolve those before dispatching either the UI or its worker.
	if config.Unified {
		for _, item := range startui.Catalog() {
			variant, _ := configForSandbox(item.Slug)
			if invokedAsProjectRunner(variant, executablePath()) {
				config = variant
				break
			}
		}
	}
	if len(os.Args) == 2 && os.Args[1] == "--startup-worker" {
		var selected startui.Selection
		data, err := os.ReadFile(os.Getenv("VSCODE_SANDBOX_STARTUP_SELECTION"))
		if err == nil {
			err = json.Unmarshal(data, &selected)
		}
		if err == nil && config.Unified {
			config, err = configForSandbox(selected.Sandbox)
		}
		if err == nil {
			if invokedAsProjectRunner(config, executablePath()) {
				err = runProject(config)
			} else {
				var deps dependencies
				deps, err = checkDependencies(config.NeedsPodman)
				if err == nil {
					err = run(config, deps, setupSelection{selected.Version, selected.Agent, selected.Project})
				}
				if err == nil {
					var launcher string
					launcher, err = cygpath(deps.bash, filepath.Join(selected.Project, "code", config.ShellLauncher))
					if err == nil {
						err = runVisible(deps.bash, launcher, selected.Agent)
					}
				}
			}
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	home, _ := os.UserHomeDir()
	c := startui.Config{Name: config.AppName, Slug: config.SandboxSlug, Version: config.DefaultVersion, VersionLabel: config.VersionPrompt, Agent: "codex", Project: filepath.Join(home, "Desktop", config.SandboxSlug+"-project")}
	if !config.NeedsVersion {
		c.VersionLabel = ""
	}
	if config.Unified {
		c = startui.Config{Name: "VS Code Sandboxes", Slug: "sandboxes", Version: "3.13", VersionLabel: "Python version", Agent: "codex", Project: filepath.Join(home, "Desktop", "python-sandbox-project"), Sandboxes: startui.Catalog()}
	}
	if invokedAsProjectRunner(config, executablePath()) {
		c.ProjectMode = true
		c.Project = filepath.Dir(filepath.Dir(executablePath()))
		agent, _ := os.ReadFile(filepath.Join(c.Project, "code", "."+config.SandboxSlug+"-agent"))
		c.Agent = strings.TrimSpace(string(agent))
	}
	err := startui.Run(c, func(selected startui.Selection, log io.Writer) error {
		env, cleanup, err := startui.PrepareEnvironment(selected)
		if err != nil {
			return err
		}
		defer cleanup()
		cmd := startui.Command(executablePath(), "--startup-worker")
		cmd.Env = append(os.Environ(), env...)
		cmd.Stdout = log
		cmd.Stderr = log
		return cmd.Run()
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func checkDependencies(needsPodman bool) (dependencies, error) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		return dependencies{}, errors.New("this executable requires 64-bit Windows on an Intel or AMD processor")
	}
	nativeArch := strings.ToUpper(getenv("PROCESSOR_ARCHITEW6432", os.Getenv("PROCESSOR_ARCHITECTURE")))
	if nativeArch != "AMD64" && nativeArch != "X86_64" {
		return dependencies{}, fmt.Errorf("Docker Sandboxes requires an Intel or AMD 64-bit processor; detected %s", nativeArch)
	}
	if err := checkWindows11(); err != nil {
		return dependencies{}, err
	}

	if _, err := requireCommand("git.exe", "Install Git for Windows: https://git-scm.com/download/win"); err != nil {
		return dependencies{}, err
	}
	bash, err := findGitBash()
	if err != nil {
		return dependencies{}, err
	}
	ssh, err := requireCommand("ssh.exe", "Install the Windows OpenSSH Client optional feature.")
	if err != nil {
		return dependencies{}, err
	}
	ssh = preferredWindowsSSHPath(ssh)
	if err := os.Setenv("PATH", filepath.Dir(ssh)+string(os.PathListSeparator)+os.Getenv("PATH")); err != nil {
		return dependencies{}, fmt.Errorf("prioritize Windows OpenSSH: %w", err)
	}
	sbx, err := requireCommand("sbx.exe", "Install Docker Sandboxes with: winget install -h Docker.sbx")
	if err != nil {
		return dependencies{}, err
	}
	if err := checkSBXVersion(sbx); err != nil {
		return dependencies{}, err
	}
	if err := runVisible(sbx, "setup", "ssh"); err != nil {
		return dependencies{}, fmt.Errorf("configure Docker Sandboxes SSH access: %w", err)
	}
	if err := runVisible(sbx, "diagnose"); err != nil {
		return dependencies{}, fmt.Errorf("Docker Sandboxes diagnostics failed after SSH setup; confirm virtualization is enabled and run 'sbx login': %w", err)
	}
	if needsPodman {
		podman, err := requireCommand("podman.exe", "Install Podman, then run podman machine init (once) and podman machine start.")
		if err != nil {
			return dependencies{}, err
		}
		if err := checkPodmanConnection(podman); err != nil {
			return dependencies{}, err
		}
	}
	if err := addVSCodeToPath(); err != nil {
		return dependencies{}, err
	}
	if err := configureVSCodeSSHPath(ssh); err != nil {
		return dependencies{}, err
	}
	return dependencies{bash: bash}, nil
}

func configureVSCodeSSHPath(discoveredSSH string) error {
	sshPath := preferredWindowsSSHPath(discoveredSSH)
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return errors.New("APPDATA is unavailable; cannot configure VS Code to use Windows OpenSSH")
	}
	settingsPath := filepath.Join(appData, "Code", "User", "settings.json")
	changed, err := ensureJSONCStringSetting(settingsPath, "remote.SSH.path", sshPath)
	if err != nil {
		return fmt.Errorf("configure VS Code Remote-SSH: %w", err)
	}
	if changed {
		fmt.Println("Configured VS Code to use Windows OpenSSH:", sshPath)
	}
	return nil
}

func preferredWindowsSSHPath(discoveredSSH string) string {
	if windowsDirectory := os.Getenv("WINDIR"); windowsDirectory != "" {
		windowsSSH := filepath.Join(windowsDirectory, "System32", "OpenSSH", "ssh.exe")
		if info, err := os.Stat(windowsSSH); err == nil && !info.IsDir() {
			return windowsSSH
		}
	}
	return discoveredSSH
}

func ensureJSONCStringSetting(path, key, value string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if regexp.MustCompile(`(?m)^[\t ]*"` + regexp.QuoteMeta(key) + `"[\t ]*:`).Match(data) {
		return false, nil
	}
	if len(bytes.TrimSpace(data)) == 0 {
		data = []byte("{}\n")
	}
	openingBrace := bytes.IndexByte(data, '{')
	if openingBrace < 0 {
		return false, fmt.Errorf("%s is not a JSON object", path)
	}
	insertion := []byte("\n  " + strconv.Quote(key) + ": " + strconv.Quote(value) + ",")
	updated := make([]byte, 0, len(data)+len(insertion))
	updated = append(updated, data[:openingBrace+1]...)
	updated = append(updated, insertion...)
	updated = append(updated, data[openingBrace+1:]...)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return false, err
	}
	if err := os.WriteFile(path, updated, 0644); err != nil {
		return false, err
	}
	return true, nil
}

func run(config Config, deps dependencies, selection setupSelection) error {
	if selection.agent != "codex" && selection.agent != "claude" {
		return errors.New("agent must be codex or claude")
	}
	if config.NeedsVersion && !regexp.MustCompile(`^[0-9]+\.[0-9]+(\.[0-9]+)?$`).MatchString(selection.version) {
		return errors.New("version must use major.minor or major.minor.patch format")
	}

	projectInput := strings.TrimSpace(selection.project)
	if !filepath.IsAbs(projectInput) {
		return errors.New("project folder must be an absolute path on a local Windows drive")
	}
	project := filepath.Clean(projectInput)
	if isUNCPath(project) {
		return errors.New("network and WSL project folders are not supported; choose a folder on a local Windows drive")
	}
	if err := os.MkdirAll(project, 0755); err != nil {
		return fmt.Errorf("create project directory: %w", err)
	}

	tempRoot, err := os.MkdirTemp("", config.SandboxSlug+"-app-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempRoot)
	setupDir := filepath.Join(tempRoot, "setup")
	if err := launcherbundle.Materialize(config.SandboxSlug, setupDir); err != nil {
		return fmt.Errorf("extract embedded setup files: %w", err)
	}
	setupScript := filepath.Join(setupDir, "scripts", "setup-project.sh")
	if info, err := os.Stat(setupScript); err != nil || info.IsDir() {
		return fmt.Errorf("embedded setup is incomplete: %s", setupScript)
	}
	unixSetupScript, err := cygpath(deps.bash, setupScript)
	if err != nil {
		return err
	}
	unixProject, err := cygpath(deps.bash, project)
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate launcher executable: %w", err)
	}
	unixExecutable, err := cygpath(deps.bash, executable)
	if err != nil {
		return err
	}
	args := []string{unixSetupScript, unixProject}
	if config.NeedsVersion {
		args = append(args, selection.version)
	}
	args = append(args, selection.agent)
	fmt.Println("Starting setup...")
	if err := runVisibleEnv(deps.bash, []string{
		"VSCODE_SANDBOX_PROJECT_RUNNER_SOURCE=" + unixExecutable,
		"VSCODE_SANDBOX_PROJECT_RUNNER_AGENT=" + selection.agent,
	}, args...); err != nil {
		return fmt.Errorf("sandbox setup failed: %w", err)
	}
	runner := filepath.Join(project, "code", config.ProjectRunner)
	if info, err := os.Stat(runner); err != nil || info.IsDir() {
		return fmt.Errorf("setup did not create the Windows project runner: %s", runner)
	}
	return nil
}

func executablePath() string {
	path, _ := os.Executable()
	return path
}

func invokedAsProjectRunner(config Config, executable string) bool {
	portablePath := strings.ReplaceAll(executable, `\`, "/")
	return strings.EqualFold(filepath.Base(portablePath), config.ProjectRunner)
}

func runProject(config Config) error {
	if config.ShellLauncher == "" {
		return errors.New("project shell launcher is not configured")
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate project runner: %w", err)
	}
	codeDir := filepath.Dir(executable)
	agentPath := filepath.Join(codeDir, "."+config.SandboxSlug+"-agent")
	agentData, err := os.ReadFile(agentPath)
	if err != nil {
		return fmt.Errorf("read project runner configuration: %w", err)
	}
	agent := strings.TrimSpace(string(agentData))
	if agent != "codex" && agent != "claude" {
		return errors.New("the project runner has an invalid configured agent")
	}
	launcher := filepath.Join(codeDir, config.ShellLauncher)
	if info, err := os.Stat(launcher); err != nil || info.IsDir() {
		return fmt.Errorf("project launcher is unavailable: %s", launcher)
	}
	bash, err := findGitBash()
	if err != nil {
		return err
	}
	if err := addVSCodeToPath(); err != nil {
		return err
	}
	unixLauncher, err := cygpath(bash, launcher)
	if err != nil {
		return err
	}
	cmd := startui.Command(bash, unixLauncher, agent)
	cmd.Dir = filepath.Dir(codeDir)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sandbox launcher failed: %w", err)
	}
	return nil
}

func checkWindows11() error {
	script := "Write-Output ((Get-CimInstance Win32_OperatingSystem).ProductType); Write-Output ([Environment]::OSVersion.Version.Build)"
	out, err := startui.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput()
	if err != nil {
		return fmt.Errorf("cannot determine the Windows edition and build: %w", err)
	}
	fields := strings.Fields(string(out))
	if len(fields) != 2 {
		return fmt.Errorf("cannot read the Windows edition and build from: %s", strings.TrimSpace(string(out)))
	}
	productType, productErr := strconv.Atoi(fields[0])
	build, buildErr := strconv.Atoi(fields[1])
	if productErr != nil || buildErr != nil {
		return errors.New("Windows returned invalid edition or build information")
	}
	if productType != 1 {
		return errors.New("Docker Sandboxes does not support Windows Server")
	}
	if build < 22000 {
		return fmt.Errorf("Docker Sandboxes requires Windows 11; detected Windows build %d", build)
	}
	return nil
}

func checkSBXVersion(sbx string) error {
	out, err := startui.Command(sbx, "version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("cannot run 'sbx version': %w", err)
	}
	re := regexp.MustCompile(`(?i)(Client Version:|sbx version:)\s+v?(\d+)\.(\d+)\.(\d+)`)
	m := re.FindStringSubmatch(string(out))
	if len(m) != 5 {
		return fmt.Errorf("cannot determine the Docker Sandboxes version from: %s", strings.TrimSpace(string(out)))
	}
	major, _ := strconv.Atoi(m[2])
	minor, _ := strconv.Atoi(m[3])
	if major == 0 && minor < 39 {
		return fmt.Errorf("Docker Sandboxes 0.39.0 or newer is required; detected %s.%s.%s", m[2], m[3], m[4])
	}
	return nil
}

func findGitBash() (string, error) {
	candidates := []string{
		filepath.Join(os.Getenv("ProgramFiles"), "Git", "bin", "bash.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Git", "usr", "bin", "bash.exe"),
		filepath.Join(os.Getenv("LocalAppData"), "Programs", "Git", "bin", "bash.exe"),
	}
	if path, err := exec.LookPath("bash.exe"); err == nil {
		candidates = append(candidates, path)
	}
	seen := map[string]bool{}
	for _, path := range candidates {
		if path == "" || seen[strings.ToLower(path)] {
			continue
		}
		seen[strings.ToLower(path)] = true
		if info, err := os.Stat(path); err == nil && !info.IsDir() && isGitBash(path) {
			return path, nil
		}
	}
	return "", errors.New("Git Bash is required; install Git for Windows from https://git-scm.com/download/win")
}

func isGitBash(path string) bool {
	out, err := startui.Command(path, "-lc", `printf '%s' "$(uname -s)"; command -v cygpath >/dev/null`).CombinedOutput()
	return err == nil && strings.HasPrefix(strings.ToUpper(strings.TrimSpace(string(out))), "MINGW")
}

func addVSCodeToPath() error {
	if _, err := exec.LookPath("code.cmd"); err == nil {
		return nil
	}
	candidates := []string{
		filepath.Join(os.Getenv("LocalAppData"), "Programs", "Microsoft VS Code", "bin"),
		filepath.Join(os.Getenv("ProgramFiles"), "Microsoft VS Code", "bin"),
	}
	for _, dir := range candidates {
		if _, err := os.Stat(filepath.Join(dir, "code.cmd")); err == nil {
			return os.Setenv("PATH", os.Getenv("PATH")+";"+dir)
		}
	}
	return errors.New("Visual Studio Code is required; install it from https://code.visualstudio.com/")
}

func cygpath(bash, path string) (string, error) {
	cmd := startui.Command(bash, "-lc", `cygpath -u "$1"`, "launcher", path)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("convert Windows path for Git Bash: %w", err)
	}
	return strings.TrimSpace(out.String()), nil
}

func requireCommand(name, help string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s is required. %s", name, help)
	}
	return path, nil
}

func checkPodmanConnection(podman string) error {
	output, err := startui.Command(podman, "info", "--format", "{{.Host.OS}}").CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			detail = err.Error()
		}
		return fmt.Errorf("Podman is unavailable; run podman machine init (once) and podman machine start, then retry: %s", detail)
	}
	if strings.ToLower(strings.TrimSpace(string(output))) != "linux" {
		return fmt.Errorf("Podman must use Linux containers; detected %q", strings.TrimSpace(string(output)))
	}
	return nil
}

func isUNCPath(path string) bool {
	normalized := strings.ReplaceAll(strings.TrimSpace(path), "/", `\`)
	return strings.HasPrefix(normalized, `\\`)
}

func runVisible(name string, args ...string) error {
	return runVisibleEnv(name, nil, args...)
}

func runVisibleEnv(name string, extraEnv []string, args ...string) error {
	cmd := startui.Command(name, args...)
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func getenv(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
