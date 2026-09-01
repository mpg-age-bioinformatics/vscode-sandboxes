package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

const (
	appName          = "Bioinformatics Sandbox"
	sandboxSlug      = "bioinformatics-sandbox"
	skillLauncher    = "bioinformatics-sandox.sh"
	versionPrompt    = ""
	needsVersion     = false
	needsDocker      = false
	envPrefix        = "BIOINFORMATICS_SANDBOX"
	skillsRepository = "https://github.com/mpg-age-bioinformatics/skills.git"
)

var reader = bufio.NewReader(os.Stdin)

func main() {
	fmt.Println(appName)
	fmt.Println(strings.Repeat("=", len(appName)))
	fmt.Println()

	err := run()
	fmt.Println()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Setup stopped:", err)
	} else {
		fmt.Println(appName, "finished successfully.")
	}
	fmt.Print("Press Enter to close this window...")
	_, _ = reader.ReadString('\n')
	if err != nil {
		os.Exit(1)
	}
}

func run() error {
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

	git, err := requireCommand("git.exe", "Install Git for Windows: https://git-scm.com/download/win")
	if err != nil {
		return err
	}
	bash, err := findGitBash()
	if err != nil {
		return err
	}
	if _, err := requireCommand("ssh.exe", "Install the Windows OpenSSH Client optional feature."); err != nil {
		return err
	}
	sbx, err := requireCommand("sbx.exe", "Install Docker Sandboxes with: winget install -h Docker.sbx")
	if err != nil {
		return err
	}
	if err := checkSBXVersion(sbx); err != nil {
		return err
	}
	if err := runVisible(sbx, "diagnose"); err != nil {
		return fmt.Errorf("Docker Sandboxes diagnostics failed; confirm virtualization is enabled and run 'sbx login': %w", err)
	}
	if needsDocker {
		docker, err := requireCommand("docker.exe", "This sandbox requires a Docker engine. Install a supported Docker Desktop release and start it.")
		if err != nil {
			return err
		}
		if err := runVisible(docker, "version"); err != nil {
			return errors.New("the Docker engine is unavailable; start Docker Desktop and confirm this Windows edition and build are supported")
		}
	}
	if err := addVSCodeToPath(); err != nil {
		return err
	}

	version := ""
	if needsVersion {
		version, err = promptRequired(versionPrompt + ": ")
		if err != nil {
			return err
		}
	}
	agent, err := promptRequired("Agent (codex or claude): ")
	if err != nil {
		return err
	}
	if agent != "codex" && agent != "claude" {
		return errors.New("agent must be codex or claude")
	}

	desktop, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	defaultProject := filepath.Join(desktop, "Desktop", sandboxSlug+"-project")
	projectInput, err := prompt("Project directory [" + defaultProject + "]: ")
	if err != nil {
		return err
	}
	if strings.TrimSpace(projectInput) == "" {
		projectInput = defaultProject
	}
	project, err := filepath.Abs(strings.TrimSpace(projectInput))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(project, 0755); err != nil {
		return fmt.Errorf("create project directory: %w", err)
	}

	tempRoot, err := os.MkdirTemp("", sandboxSlug+"-app-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempRoot)
	skillsDir := filepath.Join(tempRoot, "skills")
	repository := getenv(envPrefix+"_SKILLS_REPOSITORY", skillsRepository)
	fmt.Println("Downloading", appName, "setup files...")
	if err := runVisible(git, "clone", "--quiet", "--depth", "1", repository, skillsDir); err != nil {
		return fmt.Errorf("download setup files: %w", err)
	}
	if ref := strings.TrimSpace(os.Getenv(envPrefix + "_SKILLS_REF")); ref != "" {
		if err := runVisible(git, "-C", skillsDir, "fetch", "--quiet", "--depth", "1", "origin", ref); err != nil {
			return fmt.Errorf("download requested setup revision: %w", err)
		}
		if err := runVisible(git, "-C", skillsDir, "checkout", "--quiet", "--detach", "FETCH_HEAD"); err != nil {
			return fmt.Errorf("select requested setup revision: %w", err)
		}
	}

	launcher := filepath.Join(skillsDir, sandboxSlug, "assets", skillLauncher)
	if _, err := os.Stat(launcher); err != nil {
		return fmt.Errorf("downloaded setup is incomplete: %s", launcher)
	}
	unixLauncher, err := cygpath(bash, launcher)
	if err != nil {
		return err
	}
	unixProject, err := cygpath(bash, project)
	if err != nil {
		return err
	}
	args := []string{unixLauncher}
	if needsVersion {
		args = append(args, version)
	}
	args = append(args, agent, unixProject)
	fmt.Println("Starting setup...")
	if err := runVisible(bash, args...); err != nil {
		return fmt.Errorf("sandbox setup failed: %w", err)
	}
	return nil
}

func checkWindows11() error {
	script := "Write-Output ((Get-CimInstance Win32_OperatingSystem).ProductType); Write-Output ([Environment]::OSVersion.Version.Build)"
	out, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput()
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
	out, err := exec.Command(sbx, "version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("cannot run 'sbx version': %w", err)
	}
	re := regexp.MustCompile(`Client Version:\s+v?(\d+)\.(\d+)\.(\d+)`)
	m := re.FindStringSubmatch(string(out))
	if len(m) != 4 {
		return fmt.Errorf("cannot determine the Docker Sandboxes version from: %s", strings.TrimSpace(string(out)))
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	if major == 0 && minor < 39 {
		return fmt.Errorf("Docker Sandboxes 0.39.0 or newer is required; detected %s.%s.%s", m[1], m[2], m[3])
	}
	return nil
}

func findGitBash() (string, error) {
	if p, err := exec.LookPath("bash.exe"); err == nil {
		return p, nil
	}
	candidates := []string{
		filepath.Join(os.Getenv("ProgramFiles"), "Git", "bin", "bash.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Git", "usr", "bin", "bash.exe"),
		filepath.Join(os.Getenv("LocalAppData"), "Programs", "Git", "bin", "bash.exe"),
	}
	for _, p := range candidates {
		if p != "" {
			if info, err := os.Stat(p); err == nil && !info.IsDir() {
				return p, nil
			}
		}
	}
	return "", errors.New("Git Bash is required; install Git for Windows from https://git-scm.com/download/win")
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
	cmd := exec.Command(bash, "-lc", `cygpath -u "$1"`, "launcher", path)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("convert Windows path for Git Bash: %w", err)
	}
	return strings.TrimSpace(out.String()), nil
}

func requireCommand(name, help string) (string, error) {
	p, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s is required. %s", name, help)
	}
	return p, nil
}

func runVisible(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func prompt(label string) (string, error) {
	fmt.Print(label)
	value, err := reader.ReadString('\n')
	if err != nil && len(value) == 0 {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func promptRequired(label string) (string, error) {
	value, err := prompt(label)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", errors.New("a value is required")
	}
	return value, nil
}

func getenv(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
