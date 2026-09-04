package windowslauncher

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

const skillsRepository = "https://github.com/mpg-age-bioinformatics/skills.git"

var reader = bufio.NewReader(os.Stdin)

type Config struct {
	AppName          string
	SandboxSlug      string
	SkillLauncher    string
	ProjectRunner    string
	VersionPrompt    string
	NeedsVersion     bool
	NeedsDocker      bool
	DefaultVersion   string
	EnvPrefix        string
	DefaultSkillsRef string
}

type setupSelection struct {
	version string
	agent   string
	project string
}

type dependencies struct {
	git  string
	bash string
}

func Main(config Config) {
	fmt.Println(config.AppName)
	fmt.Println(strings.Repeat("=", len(config.AppName)))
	fmt.Println()

	err := execute(config)
	fmt.Println()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Setup stopped:", err)
	} else {
		fmt.Println(config.AppName, "finished successfully.")
	}
	fmt.Print("Press Enter to close this window...")
	_, _ = reader.ReadString('\n')
	if err != nil {
		os.Exit(1)
	}
}

func execute(config Config) error {
	deps, err := checkDependencies(config.NeedsDocker)
	if err != nil {
		return err
	}
	selection, cancelled, err := showSetupForm(config)
	if cancelled || err != nil {
		return err
	}
	return run(config, deps, selection)
}

func checkDependencies(needsDocker bool) (dependencies, error) {
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

	git, err := requireCommand("git.exe", "Install Git for Windows: https://git-scm.com/download/win")
	if err != nil {
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
	if needsDocker {
		docker, err := requireCommand("docker.exe", "This sandbox requires Docker Desktop running Linux containers. Install Docker Desktop and start it.")
		if err != nil {
			return dependencies{}, err
		}
		if err := checkDockerDaemon(docker); err != nil {
			return dependencies{}, err
		}
	}
	if err := addVSCodeToPath(); err != nil {
		return dependencies{}, err
	}
	if err := configureVSCodeSSHPath(ssh); err != nil {
		return dependencies{}, err
	}
	return dependencies{git: git, bash: bash}, nil
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
	skillsDir := filepath.Join(tempRoot, "skills")
	repository := getenv(config.EnvPrefix+"_SKILLS_REPOSITORY", skillsRepository)
	ref := getenv(config.EnvPrefix+"_SKILLS_REF", config.DefaultSkillsRef)
	if ref == "" {
		return errors.New("this launcher was built without a pinned skills revision; download a correctly built release or set " + config.EnvPrefix + "_SKILLS_REF")
	}
	fmt.Println("Downloading", config.AppName, "setup files at revision", ref+"...")
	if err := runVisible(deps.git, "-c", "core.autocrlf=false", "clone", "--quiet", "--no-checkout", "--depth", "1", repository, skillsDir); err != nil {
		return fmt.Errorf("download setup files: %w", err)
	}
	if err := runVisible(deps.git, "-C", skillsDir, "-c", "core.autocrlf=false", "fetch", "--quiet", "--depth", "1", "origin", ref); err != nil {
		return fmt.Errorf("download pinned setup revision %s: %w", ref, err)
	}
	if err := runVisible(deps.git, "-C", skillsDir, "-c", "core.autocrlf=false", "checkout", "--quiet", "--detach", "FETCH_HEAD"); err != nil {
		return fmt.Errorf("select pinned setup revision %s: %w", ref, err)
	}

	launcher := filepath.Join(skillsDir, config.SandboxSlug, "assets", config.SkillLauncher)
	if info, err := os.Stat(launcher); err != nil || info.IsDir() {
		return fmt.Errorf("downloaded setup is incomplete: %s", launcher)
	}
	unixLauncher, err := cygpath(deps.bash, launcher)
	if err != nil {
		return err
	}
	unixProject, err := cygpath(deps.bash, project)
	if err != nil {
		return err
	}
	args := []string{unixLauncher}
	if config.NeedsVersion {
		args = append(args, selection.version)
	}
	args = append(args, selection.agent, unixProject)
	fmt.Println("Starting setup...")
	if err := os.Setenv(config.EnvPrefix+"_SKILLS_REF", ref); err != nil {
		return fmt.Errorf("configure pinned skills revision: %w", err)
	}
	if err := runVisible(deps.bash, args...); err != nil {
		return fmt.Errorf("sandbox setup failed: %w", err)
	}
	runner := filepath.Join(project, "code", config.ProjectRunner)
	if info, err := os.Stat(runner); err != nil || info.IsDir() {
		return fmt.Errorf("setup did not create the Windows project runner: %s", runner)
	}
	return nil
}

func showSetupForm(config Config) (setupSelection, bool, error) {
	resultDir, err := os.MkdirTemp("", config.SandboxSlug+"-selection-*")
	if err != nil {
		return setupSelection{}, false, err
	}
	defer os.RemoveAll(resultDir)
	resultPath := filepath.Join(resultDir, "selection.txt")
	cmd := exec.Command("powershell.exe", "-NoProfile", "-STA", "-Command", windowsFormScript)
	cmd.Env = append(os.Environ(),
		"VSCODE_SANDBOX_APP_NAME="+config.AppName,
		"VSCODE_SANDBOX_VERSION_LABEL="+config.VersionPrompt,
		"VSCODE_SANDBOX_DEFAULT_VERSION="+config.DefaultVersion,
		"VSCODE_SANDBOX_DEFAULT_PROJECT_NAME="+config.SandboxSlug+"-project",
		"VSCODE_SANDBOX_NEEDS_VERSION="+strconv.FormatBool(config.NeedsVersion),
		"VSCODE_SANDBOX_RESULT_PATH="+resultPath)
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
$appName = $env:VSCODE_SANDBOX_APP_NAME
$versionLabelText = $env:VSCODE_SANDBOX_VERSION_LABEL
$defaultVersion = $env:VSCODE_SANDBOX_DEFAULT_VERSION
$defaultProjectName = $env:VSCODE_SANDBOX_DEFAULT_PROJECT_NAME
$needsVersion = $env:VSCODE_SANDBOX_NEEDS_VERSION -eq 'true'
$resultPath = $env:VSCODE_SANDBOX_RESULT_PATH
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
$desktop = [Environment]::GetFolderPath('Desktop')
if ([String]::IsNullOrWhiteSpace($desktop)) { $desktop = $env:USERPROFILE }
$defaultProject = Join-Path $desktop $defaultProjectName
$form = New-Object Windows.Forms.Form
$form.Text = $appName
$form.StartPosition = 'CenterScreen'
$form.FormBorderStyle = 'FixedDialog'
$form.MaximizeBox = $false
$form.MinimizeBox = $false
$form.ClientSize = New-Object Drawing.Size(680, $(if ($needsVersion) { 205 } else { 167 }))
$top = 20
function Add-Label($text, $y) {
  $label = New-Object Windows.Forms.Label
  $label.Text = $text
  $label.Location = New-Object Drawing.Point(18, $y)
  $label.Size = New-Object Drawing.Size(130, 23)
  $form.Controls.Add($label)
}
if ($needsVersion) {
  Add-Label $versionLabelText $top
  $version = New-Object Windows.Forms.TextBox
  $version.Text = $defaultVersion
  $version.Location = New-Object Drawing.Point(155, $top)
  $version.Size = New-Object Drawing.Size(505, 23)
  $form.Controls.Add($version)
  $top += 38
}
Add-Label 'Agent' $top
$agent = New-Object Windows.Forms.ComboBox
$agent.DropDownStyle = 'DropDownList'
[void]$agent.Items.AddRange(@('codex', 'claude'))
$agent.SelectedIndex = 0
$agent.Location = New-Object Drawing.Point(155, $top)
$agent.Size = New-Object Drawing.Size(505, 23)
$form.Controls.Add($agent)
$top += 38
Add-Label 'Project folder' $top
$project = New-Object Windows.Forms.TextBox
$project.Text = $defaultProject
$project.Location = New-Object Drawing.Point(155, $top)
$project.Size = New-Object Drawing.Size(415, 23)
$form.Controls.Add($project)
$browse = New-Object Windows.Forms.Button
$browse.Text = 'Browse...'
$browse.Location = New-Object Drawing.Point(580, ($top - 1))
$browse.Size = New-Object Drawing.Size(80, 25)
$browse.Add_Click({
  $picker = New-Object Windows.Forms.FolderBrowserDialog
  if (Test-Path -LiteralPath $project.Text -PathType Container) { $picker.SelectedPath = $project.Text }
  if ($picker.ShowDialog() -eq 'OK') { $project.Text = $picker.SelectedPath }
})
$form.Controls.Add($browse)
$top += 42
$setup = New-Object Windows.Forms.Button
$setup.Text = 'Setup'
$setup.Location = New-Object Drawing.Point(495, $top)
$setup.Size = New-Object Drawing.Size(80, 28)
$setup.Add_Click({
  $selectedVersion = if ($needsVersion) { $version.Text.Trim() } else { '' }
  $selectedProject = $project.Text.Trim()
  if (($needsVersion -and $selectedVersion -notmatch '^[0-9]+\.[0-9]+(\.[0-9]+)?$') -or [String]::IsNullOrWhiteSpace($selectedProject)) {
    [void][Windows.Forms.MessageBox]::Show('Enter a valid version and project folder.', $appName, 'OK', 'Error')
    return
  }
  if ($selectedProject -notmatch '^[A-Za-z]:[\\/]' -or $selectedProject.StartsWith('\\')) {
    [void][Windows.Forms.MessageBox]::Show('Choose an absolute folder on a local Windows drive.', $appName, 'OK', 'Error')
    return
  }
  [IO.File]::WriteAllLines($resultPath, @($selectedVersion, $agent.SelectedItem.ToString(), $selectedProject), (New-Object Text.UTF8Encoding($false)))
  $form.DialogResult = 'OK'
  $form.Close()
})
$cancel = New-Object Windows.Forms.Button
$cancel.Text = 'Cancel'
$cancel.Location = New-Object Drawing.Point(580, $top)
$cancel.Size = New-Object Drawing.Size(80, 28)
$cancel.DialogResult = 'Cancel'
$form.AcceptButton = $setup
$form.CancelButton = $cancel
$form.Controls.Add($setup)
$form.Controls.Add($cancel)
[void]$form.ShowDialog()
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
	out, err := exec.Command(path, "-lc", `printf '%s' "$(uname -s)"; command -v cygpath >/dev/null`).CombinedOutput()
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
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s is required. %s", name, help)
	}
	return path, nil
}

func checkDockerDaemon(docker string) error {
	output, err := exec.Command(docker, "info", "--format", "{{.OSType}}").CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			detail = err.Error()
		}
		return fmt.Errorf("the Docker daemon is unavailable; start Docker Desktop and retry: %s", detail)
	}
	if strings.ToLower(strings.TrimSpace(string(output))) != "linux" {
		return fmt.Errorf("Docker Desktop must use Linux containers; detected %q", strings.TrimSpace(string(output)))
	}
	return nil
}

func isUNCPath(path string) bool {
	normalized := strings.ReplaceAll(strings.TrimSpace(path), "/", `\`)
	return strings.HasPrefix(normalized, `\\`)
}

func runVisible(name string, args ...string) error {
	cmd := exec.Command(name, args...)
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
