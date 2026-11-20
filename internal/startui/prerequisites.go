package startui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// Prerequisite checks are deliberately independent of project settings and
// credentials. Never persist a dismissal: missing tools must gate every launch.
type Prerequisites struct {
	Docker     bool   `json:"docker"`
	VSCode     bool   `json:"vscode"`
	Ready      bool   `json:"ready"`
	Supported  bool   `json:"supported"`
	Installing bool   `json:"installing"`
	Log        string `json:"log"`
	Error      string `json:"error"`
}

type prerequisiteSetup struct {
	sync.Mutex
	installing bool
	log        string
	err        string
	detect     func() Prerequisites
	install    func(context.Context, io.Writer) error
}

func (p *prerequisiteSetup) Write(data []byte) (int, error) {
	p.Lock()
	defer p.Unlock()
	p.log += string(data)
	if len(p.log) > 128<<10 {
		p.log = p.log[len(p.log)-(128<<10):]
	}
	return len(data), nil
}

func (p *prerequisiteSetup) status() Prerequisites {
	status := p.detectStatus()
	p.Lock()
	defer p.Unlock()
	status.Installing, status.Log, status.Error = p.installing, p.log, p.err
	status.Ready = status.Ready && !p.installing
	return status
}

func (p *prerequisiteSetup) detectStatus() Prerequisites {
	if p.detect != nil {
		return p.detect()
	}
	return detectPrerequisites()
}

func detectPrerequisites() Prerequisites {
	return detectPrerequisitesForPlatform(runtime.GOOS, runtime.GOARCH)
}

func detectPrerequisitesForPlatform(platform, arch string) Prerequisites {
	_, dockerErr := findSandboxCLI(platform)
	_, editorErr := editorCommandForPlatform(platform, "--version")
	supported := (platform == "darwin" && arch == "arm64") || (platform == "windows" && arch == "amd64")
	return Prerequisites{Docker: dockerErr == nil, VSCode: editorErr == nil, Ready: dockerErr == nil && editorErr == nil, Supported: supported}
}

// SandboxCLI also discovers installs made after the launcher started, without
// requiring the user to restart their session to pick up a changed PATH.
func SandboxCLI() (string, error) {
	return findSandboxCLI(runtime.GOOS)
}

func findSandboxCLI(platform string) (string, error) {
	binary := "sbx"
	if platform == "windows" {
		binary = "sbx.exe"
	}
	if path, err := exec.LookPath(binary); err == nil {
		return path, nil
	}
	home, _ := os.UserHomeDir()
	candidates := []string{}
	if platform == "windows" {
		for _, root := range []string{os.Getenv("LOCALAPPDATA"), os.Getenv("ProgramFiles")} {
			candidates = append(candidates, filepath.Join(root, "DockerSandboxes", "bin", binary),
				filepath.Join(root, "Docker", "DockerSandboxes", "bin", binary),
				filepath.Join(root, "Docker Sandboxes", "bin", binary))
		}
	} else if platform == "darwin" {
		candidates = append(candidates,
			filepath.Join(settingsDir(), "tools", "docker-sandboxes-0.39.0", "bin", "sbx"),
			"/opt/homebrew/bin/sbx", "/usr/local/bin/sbx",
			"/Applications/Sbx.app/Contents/MacOS/sbx", filepath.Join(home, "Applications", "Sbx.app", "Contents", "MacOS", "sbx"),
			"/Applications/Docker Sandboxes.app/Contents/MacOS/sbx", filepath.Join(home, "Applications", "Docker Sandboxes.app", "Contents", "MacOS", "sbx"))
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() && (platform == "windows" || info.Mode()&0111 != 0) {
			return candidate, nil
		}
	}
	if platform == "windows" && runtime.GOOS == "windows" {
		// MSI installers update the registry PATH, not this running process.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", `$env:Path=[Environment]::GetEnvironmentVariable('Path','Machine')+';'+[Environment]::GetEnvironmentVariable('Path','User'); $c=Get-Command sbx.exe -CommandType Application -ErrorAction SilentlyContinue; if($c){[Console]::Write($c[0].Source)}`)
		hideWindow(cmd)
		data, err := cmd.Output()
		if err == nil {
			path := string(data)
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				return path, nil
			}
		}
	}
	return "", fmt.Errorf("Docker Sandboxes was not detected")
}

type prerequisiteDownload struct{ URL, SHA256, Filename string }

// Vendor downloads are pinned and hash-checked before extraction or execution.
// Docker 0.39.0 is the tested baseline; VS Code metadata comes from Microsoft's
// official update API for stable 1.147.0. No vendor binaries are redistributed.
func prerequisiteArtifact(component, platform string) (prerequisiteDownload, error) {
	switch component + ":" + platform {
	case "docker:darwin":
		return prerequisiteDownload{"https://github.com/docker/sbx-releases/releases/download/v0.39.0/DockerSandboxes-darwin.tar.gz", "d12fa06aeece298d96dae058101f256537770a00ef46e6584762e0f27b06a5a3", "DockerSandboxes-darwin.tar.gz"}, nil
	case "docker:windows":
		return prerequisiteDownload{"https://github.com/docker/sbx-releases/releases/download/v0.39.0/DockerSandboxes.msi", "f1189f3d2d80c60091eee9e1a852132c8e678ce06725fa2e029d42f45ea038e9", "DockerSandboxes.msi"}, nil
	case "vscode:darwin":
		return prerequisiteDownload{"https://update.code.visualstudio.com/1.147.0/darwin-arm64/stable", "149d944feb599b076b48406b174e1c3cd1d510e9d673854777a20a6f4116a59a", "VSCode-darwin-arm64.zip"}, nil
	case "vscode:windows":
		return prerequisiteDownload{"https://update.code.visualstudio.com/1.147.0/win32-x64-user/stable", "903bee51e0114b8e7383703c038176e21cbd0f4a666a76c07705453fe824aaa9", "VSCodeUserSetup-x64.exe"}, nil
	}
	return prerequisiteDownload{}, fmt.Errorf("Automatic installation is unavailable on this platform")
}

func fileMatchesHash(path, expected string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return false
	}
	return hex.EncodeToString(hash.Sum(nil)) == expected
}

func downloadPrerequisite(ctx context.Context, client *http.Client, artifact prerequisiteDownload, cache string, log io.Writer) (string, error) {
	if _, err := hex.DecodeString(artifact.SHA256); err != nil || len(artifact.SHA256) != 64 {
		return "", fmt.Errorf("Invalid installer checksum")
	}
	if err := os.MkdirAll(cache, 0700); err != nil {
		return "", err
	}
	target := filepath.Join(cache, artifact.SHA256+"-"+artifact.Filename)
	if fileMatchesHash(target, artifact.SHA256) {
		fmt.Fprintln(log, "Using verified cached download:", artifact.Filename)
		return target, nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, artifact.URL, nil)
	if err != nil {
		return "", err
	}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Download returned HTTP %d", response.StatusCode)
	}
	temporary, err := os.CreateTemp(cache, ".download-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(temporary.Name())
	fmt.Fprintln(log, "Downloading", artifact.Filename, "from its official publisher…")
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(temporary, hash), io.LimitReader(response.Body, (1<<30)+1))
	closeErr := temporary.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if size > 1<<30 {
		return "", fmt.Errorf("Installer exceeds the 1 GiB download limit")
	}
	if hex.EncodeToString(hash.Sum(nil)) != artifact.SHA256 {
		return "", fmt.Errorf("Installer checksum mismatch; nothing was installed. Retry or use the official download link.")
	}
	// Replace only this managed cached artifact after verification.
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err := os.Rename(temporary.Name(), target); err != nil {
		return "", err
	}
	fmt.Fprintln(log, "Verified download checksum.")
	return target, nil
}

// Caller holds the startup state lock, so installation and startup cannot race.
func (p *prerequisiteSetup) start() error {
	return p.startComponents([]string{"docker", "vscode"})
}

func (p *prerequisiteSetup) startComponents(components []string) error {
	if len(components) == 0 || len(components) > 2 {
		return fmt.Errorf("Choose missing components to install")
	}
	for _, component := range components {
		if component != "docker" && component != "vscode" {
			return fmt.Errorf("Unknown prerequisite component")
		}
	}

	p.Lock()
	defer p.Unlock()
	if p.installing {
		return fmt.Errorf("Installation is already running")
	}
	if !p.detectStatus().Supported {
		return fmt.Errorf("Automatic setup requires Apple silicon macOS or 64-bit Intel/AMD Windows 11")
	}
	p.installing, p.log, p.err = true, "", ""
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
		defer cancel()
		install := p.install
		if install == nil {
			install = func(ctx context.Context, log io.Writer) error {
				return installPrerequisiteComponents(ctx, components, log)
			}
		}
		err := install(ctx, p)
		p.Lock()
		defer p.Unlock()
		p.installing = false
		if err != nil {
			p.err = err.Error()
		}
	}()
	return nil
}

func installMissingPrerequisites(ctx context.Context, log io.Writer) error {
	return installPrerequisiteComponents(ctx, []string{"docker", "vscode"}, log)
}

func installPrerequisiteComponents(ctx context.Context, components []string, log io.Writer) error {
	cache, err := os.UserCacheDir()
	if err != nil {
		return err
	}
	cache = filepath.Join(cache, "VS Code Sandboxes", "downloads")
	client := &http.Client{Timeout: 30 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return fmt.Errorf("Refusing an insecure installer redirect")
		}
		if len(via) > 10 {
			return fmt.Errorf("Too many installer redirects")
		}
		return nil
	}}
	for _, component := range components {
		status := detectPrerequisites()
		if (component == "docker" && status.Docker) || (component == "vscode" && status.VSCode) {
			fmt.Fprintln(log, "Keeping existing", component, "installation.")
			continue
		}
		artifact, err := prerequisiteArtifact(component, runtime.GOOS)
		if err != nil {
			return err
		}
		file, err := downloadPrerequisite(ctx, client, artifact, cache, log)
		if err != nil {
			return err
		}
		if runtime.GOOS == "darwin" {
			err = installMacPrerequisite(ctx, component, file, log)
		} else {
			err = installWindowsPrerequisite(ctx, component, file, log)
		}
		if err != nil {
			return err
		}
		fmt.Fprintln(log, "Installed", component+".")
	}
	status := detectPrerequisites()
	for _, component := range components {
		if (component == "docker" && !status.Docker) || (component == "vscode" && !status.VSCode) {
			return fmt.Errorf("Installation finished but a requested tool was not detected. Complete any installer prompts, then click Check again.")
		}
	}
	fmt.Fprintln(log, "Requested components are detected. Continue with your workspace settings.")
	return nil
}

func installMacPrerequisite(ctx context.Context, component, archive string, log io.Writer) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	parent, destination := filepath.Join(home, "Applications"), "Visual Studio Code.app"
	if component == "docker" {
		parent = filepath.Join(settingsDir(), "tools")
		destination = "docker-sandboxes-0.39.0"
	}
	if err := os.MkdirAll(parent, 0755); err != nil {
		return err
	}
	target := filepath.Join(parent, destination)
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		return fmt.Errorf("Refusing to replace existing %s. Repair or move that installation, then check again.", target)
	}
	stage, err := os.MkdirTemp(parent, ".sandbox-install-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	var cmd *exec.Cmd
	if component == "docker" {
		cmd = exec.CommandContext(ctx, "/usr/bin/tar", "-xzf", archive, "-C", stage)
	} else {
		cmd = exec.CommandContext(ctx, "/usr/bin/ditto", "-x", "-k", archive, stage)
	}
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Extract %s: %w", component, err)
	}
	source := stage
	if component == "vscode" {
		source = filepath.Join(stage, "Visual Studio Code.app")
		// Validate Microsoft's signed app before making it available.
		verify := exec.CommandContext(ctx, "/usr/bin/codesign", "--verify", "--deep", "--strict", source)
		verify.Stdout, verify.Stderr = log, log
		if err := verify.Run(); err != nil {
			return fmt.Errorf("VS Code signature verification failed: %w", err)
		}
	} else if _, err := os.Stat(filepath.Join(stage, "bin", "sbx")); err != nil {
		return fmt.Errorf("Docker Sandboxes archive is incomplete: %w", err)
	}
	return os.Rename(source, target)
}

func installWindowsPrerequisite(ctx context.Context, component, file string, log io.Writer) error {
	// Authenticode checks use an environment variable so paths never become code.
	verify := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", `$s=Get-AuthenticodeSignature -LiteralPath $env:SANDBOX_INSTALLER; if($s.Status -ne 'Valid'){throw 'Installer signature is invalid'}; if($s.SignerCertificate.Subject -notmatch $env:SANDBOX_PUBLISHER){throw 'Unexpected installer publisher'}`)
	publisher := "Microsoft Corporation"
	if component == "docker" {
		publisher = "Docker Inc"
	}
	verify.Env = append(os.Environ(), "SANDBOX_INSTALLER="+file, "SANDBOX_PUBLISHER="+publisher)
	hideWindow(verify)
	verify.Stdout, verify.Stderr = log, log
	if err := verify.Run(); err != nil {
		return fmt.Errorf("Verify installer publisher: %w", err)
	}
	var cmd *exec.Cmd
	if component == "docker" {
		cmd = exec.CommandContext(ctx, "msiexec.exe", "/i", file, "/passive", "/norestart")
	} else {
		cmd = exec.CommandContext(ctx, file, "/SILENT", "/NORESTART", "/MERGETASKS=!runcode")
	}
	hideWindow(cmd)
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 3010 {
			return fmt.Errorf("Installation requires a Windows restart. Restart Windows, then reopen this app to continue.")
		}
		return fmt.Errorf("Install %s: %w. Complete any Windows installer or administrator prompt, then retry.", component, err)
	}
	return nil
}
