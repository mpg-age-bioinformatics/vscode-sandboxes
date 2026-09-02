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
	projectRunner    = "Run Bioinformatics Sandbox.exe"
	versionPrompt    = ""
	needsVersion     = false
	needsDocker      = false
	defaultVersion   = ""
	envPrefix        = "BIOINFORMATICS_SANDBOX"
	skillsRepository = "https://github.com/mpg-age-bioinformatics/skills.git"
)

var reader = bufio.NewReader(os.Stdin)

type setupSelection struct{ version, agent, project string }

func main() {
	fmt.Println(appName)
	fmt.Println(strings.Repeat("=", len(appName)))
	fmt.Println()

	selection, cancelled, err := showSetupForm()
	if cancelled {
		return
	}
	if err == nil {
		err = run(selection)
	}
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

func run(selection setupSelection) error {
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
		if err := checkDockerDaemon(docker); err != nil {
			return err
		}
	}
	if err := addVSCodeToPath(); err != nil {
		return err
	}

	version := selection.version
	agent := selection.agent
	if agent != "codex" && agent != "claude" {
		return errors.New("agent must be codex or claude")
	}

	projectInput := selection.project
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
	runner := filepath.Join(project, "code", projectRunner)
	if info, err := os.Stat(runner); err != nil || info.IsDir() {
		return fmt.Errorf("setup did not create the Windows project runner: %s", runner)
	}
	return nil
}

func showSetupForm() (setupSelection, bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return setupSelection{}, false, err
	}
	resultDir, err := os.MkdirTemp("", sandboxSlug+"-selection-*")
	if err != nil {
		return setupSelection{}, false, err
	}
	defer os.RemoveAll(resultDir)
	resultPath := filepath.Join(resultDir, "selection.txt")
	cmd := exec.Command("powershell.exe", "-NoProfile", "-STA", "-Command", windowsFormScript)
	cmd.Env = append(os.Environ(), "VSCODE_SANDBOX_APP_NAME="+appName, "VSCODE_SANDBOX_VERSION_LABEL="+versionPrompt, "VSCODE_SANDBOX_DEFAULT_VERSION="+defaultVersion, "VSCODE_SANDBOX_DEFAULT_PROJECT="+filepath.Join(home, "Desktop", sandboxSlug+"-project"), "VSCODE_SANDBOX_NEEDS_VERSION="+strconv.FormatBool(needsVersion), "VSCODE_SANDBOX_RESULT_PATH="+resultPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return setupSelection{}, false, fmt.Errorf("open setup form: %s", strings.TrimSpace(string(output)))
	}
	data, err := os.ReadFile(resultPath)
	if os.IsNotExist(err) {
		return setupSelection{}, true, nil
	}
	if err != nil {
		return setupSelection{}, false, fmt.Errorf("read setup form values: %w", err)
	}
	values := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(values) < 3 {
		return setupSelection{}, false, errors.New("the setup form returned incomplete values")
	}
	return setupSelection{strings.TrimSpace(values[0]), strings.TrimSpace(values[1]), strings.TrimSpace(values[2])}, false, nil
}

const windowsFormScript = `
$appName = $env:VSCODE_SANDBOX_APP_NAME; $versionLabelText = $env:VSCODE_SANDBOX_VERSION_LABEL
$defaultVersion = $env:VSCODE_SANDBOX_DEFAULT_VERSION; $defaultProject = $env:VSCODE_SANDBOX_DEFAULT_PROJECT
$needsVersionText = $env:VSCODE_SANDBOX_NEEDS_VERSION; $resultPath = $env:VSCODE_SANDBOX_RESULT_PATH
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
$needsVersion = $needsVersionText -eq 'true'
$form = New-Object Windows.Forms.Form
$form.Text = $appName; $form.StartPosition = 'CenterScreen'; $form.FormBorderStyle = 'FixedDialog'; $form.MaximizeBox = $false; $form.MinimizeBox = $false
$form.ClientSize = New-Object Drawing.Size(680, $(if ($needsVersion) { 205 } else { 167 })); $top = 20
function Add-Label($text, $y) { $label = New-Object Windows.Forms.Label; $label.Text = $text; $label.Location = New-Object Drawing.Point(18, $y); $label.Size = New-Object Drawing.Size(130, 23); $form.Controls.Add($label) }
if ($needsVersion) { Add-Label $versionLabelText $top; $version = New-Object Windows.Forms.TextBox; $version.Text = $defaultVersion; $version.Location = New-Object Drawing.Point(155, $top); $version.Size = New-Object Drawing.Size(505, 23); $form.Controls.Add($version); $top += 38 }
Add-Label 'Agent' $top; $agent = New-Object Windows.Forms.ComboBox; $agent.DropDownStyle = 'DropDownList'; [void]$agent.Items.AddRange(@('codex', 'claude')); $agent.SelectedIndex = 0; $agent.Location = New-Object Drawing.Point(155, $top); $agent.Size = New-Object Drawing.Size(505, 23); $form.Controls.Add($agent); $top += 38
Add-Label 'Project folder' $top; $project = New-Object Windows.Forms.TextBox; $project.Text = $defaultProject; $project.Location = New-Object Drawing.Point(155, $top); $project.Size = New-Object Drawing.Size(415, 23); $form.Controls.Add($project)
$browse = New-Object Windows.Forms.Button; $browse.Text = 'Browse...'; $browse.Location = New-Object Drawing.Point(580, ($top - 1)); $browse.Size = New-Object Drawing.Size(80, 25); $browse.Add_Click({ $picker = New-Object Windows.Forms.FolderBrowserDialog; if ($picker.ShowDialog() -eq 'OK') { $project.Text = $picker.SelectedPath } }); $form.Controls.Add($browse); $top += 42
$setup = New-Object Windows.Forms.Button; $setup.Text = 'Setup'; $setup.Location = New-Object Drawing.Point(495, $top); $setup.Size = New-Object Drawing.Size(80, 28); $setup.DialogResult = 'OK'
$cancel = New-Object Windows.Forms.Button; $cancel.Text = 'Cancel'; $cancel.Location = New-Object Drawing.Point(580, $top); $cancel.Size = New-Object Drawing.Size(80, 28); $cancel.DialogResult = 'Cancel'; $form.AcceptButton = $setup; $form.CancelButton = $cancel; $form.Controls.Add($setup); $form.Controls.Add($cancel)
if ($form.ShowDialog() -eq 'OK') { $selectedVersion = if ($needsVersion) { $version.Text.Trim() } else { '' }; if (($needsVersion -and $selectedVersion -eq '') -or $project.Text.Trim() -eq '') { [Windows.Forms.MessageBox]::Show('All fields are required.', $appName, 'OK', 'Error'); exit 2 }; [IO.File]::WriteAllLines($resultPath, @($selectedVersion, $agent.SelectedItem.ToString(), $project.Text.Trim()), (New-Object Text.UTF8Encoding($false))) }
`

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

func checkDockerDaemon(docker string) error {
	if output, err := exec.Command(docker, "info").CombinedOutput(); err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			detail = err.Error()
		}
		return fmt.Errorf("the Docker daemon is unavailable; start Docker Desktop and retry: %s", detail)
	}
	return nil
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
