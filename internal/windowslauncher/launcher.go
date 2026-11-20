package windowslauncher

import (
	"bytes"
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
	DefaultVersion string
}

func Main(config Config) {
	if len(os.Args) == 2 && os.Args[1] == "--licenses" {
		fmt.Print(licensing.License, "\n", licensing.ThirdPartyNotices, "\n", licensing.Disclaimer)
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

	home, _ := os.UserHomeDir()
	c := startui.Config{Name: config.AppName, Slug: config.SandboxSlug, Version: config.DefaultVersion, VersionLabel: config.VersionPrompt, Agent: "both", Project: filepath.Join(home, "sandboxes", config.SandboxSlug+"-project")}
	if !config.NeedsVersion {
		c.VersionLabel = ""
	}
	if config.Unified {
		c = startui.Config{Name: "VS Code - Sandboxes", Slug: "sandboxes", Version: "3.13", VersionLabel: "Python version", Agent: "both", Project: filepath.Join(home, "sandboxes", "python-sandbox-project"), Sandboxes: startui.Catalog()}
	}
	if invokedAsProjectRunner(config, executablePath()) {
		c.ProjectMode = true
		c.Project = filepath.Dir(filepath.Dir(filepath.Dir(executablePath())))
		agent, _ := os.ReadFile(filepath.Join(c.Project, "code", ".sandbox", "."+config.SandboxSlug+"-agent"))
		c.Agent = strings.TrimSpace(string(agent))
	}
	err := startui.Run(c, func(selected startui.Selection, log io.Writer) error {
		if err := checkDependencies(); err != nil {
			return err
		}
		return startui.LaunchSandbox(selected, log)
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func checkDependencies() error {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		return errors.New("this executable requires 64-bit Windows on an Intel or AMD processor")
	}
	nativeArch := strings.ToUpper(getenv("PROCESSOR_ARCHITEW6432", os.Getenv("PROCESSOR_ARCHITECTURE")))
	if nativeArch != "AMD64" && nativeArch != "X86_64" {
		return fmt.Errorf("Docker Sandboxes requires an Intel or AMD 64-bit processor; detected %s", nativeArch)
	}
	if err := checkWindows11(); err != nil {
		return err
	}

	ssh, err := requireCommand("ssh.exe", "Install the Windows OpenSSH Client optional feature.")
	if err != nil {
		return err
	}
	ssh = preferredWindowsSSHPath(ssh)
	if err := os.Setenv("PATH", filepath.Dir(ssh)+string(os.PathListSeparator)+os.Getenv("PATH")); err != nil {
		return fmt.Errorf("prioritize Windows OpenSSH: %w", err)
	}
	sbx, err := startui.SandboxCLI()
	if err != nil {
		return err
	}
	if err := checkSBXVersion(sbx); err != nil {
		return err
	}

	if err := addVSCodeToPath(); err != nil {
		return err
	}
	if err := configureVSCodeSSHPath(ssh); err != nil {
		return err
	}
	return nil
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

func executablePath() string {
	path, _ := os.Executable()
	return path
}

func invokedAsProjectRunner(config Config, executable string) bool {
	portablePath := strings.ReplaceAll(executable, `\`, "/")
	return strings.EqualFold(filepath.Base(portablePath), config.ProjectRunner)
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

func requireCommand(name, help string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s is required. %s", name, help)
	}
	return path, nil
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
