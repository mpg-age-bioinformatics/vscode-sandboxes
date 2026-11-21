package startui

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	licensing "github.com/mpg-age-bioinformatics/vscode-sandboxes"
	"github.com/mpg-age-bioinformatics/vscode-sandboxes/internal/launcherbundle"
)

type remoteImagePlan struct{ Image, Stage, Archive, Project, Key string }

func remoteSource(p RemoteProfile) string {
	if p.ImageSource == "" {
		return "sif"
	}
	return p.ImageSource
}
func remotePreset(p RemoteProfile) string {
	switch remoteSource(p) {
	case "python", "r", "bioinformatics":
		return p.ImageSource + "-sandbox"
	}
	return ""
}
func remoteArchitecture(p RemoteProfile) string {
	if p.Architecture == "" {
		return "amd64"
	}
	return p.Architecture
}
func validateRemoteImage(p RemoteProfile) error {
	if arch := remoteArchitecture(p); arch != "amd64" && arch != "arm64" {
		return fmt.Errorf("Choose amd64 or arm64 for the HPC compute node architecture.")
	}
	switch remoteSource(p) {
	case "cached":
		if !remoteImageIDPattern.MatchString(p.CachedImage) {
			return fmt.Errorf("Choose a cached server image.")
		}
	case "sif":
		if !safeRemotePath(p.Image, false) {
			return fmt.Errorf("SIF image must be an absolute path on the HPC.")
		}
	case "registry":
		if !regexp.MustCompile(`^(?:docker://)?[a-zA-Z0-9][a-zA-Z0-9._/:@-]*$`).MatchString(p.Image) || strings.Contains(p.Image, "://") && !strings.HasPrefix(p.Image, "docker://") {
			return fmt.Errorf("Enter an OCI registry image reference, such as docker.io/library/ubuntu:24.04; do not include credentials or a web page URL.")
		}
		if strings.Contains(p.Image, "@") && !regexp.MustCompile(`@sha256:[a-fA-F0-9]{64}$`).MatchString(p.Image) {
			return fmt.Errorf("Use @sha256:<digest> for an immutable image; credentials must not be embedded in image references.")
		}
	case "dockerfile":
		if len(p.Dockerfile) == 0 || len(p.Dockerfile) > 512*1024 || strings.ContainsRune(p.Dockerfile, 0) || !regexp.MustCompile(`(?im)^\s*FROM\s+`).MatchString(p.Dockerfile) {
			return fmt.Errorf("Upload a Dockerfile with a FROM instruction (maximum 512 KiB).")
		}
	case "python", "r":
		if !regexp.MustCompile(`^[0-9]+\.[0-9]+(?:\.[0-9]+)?$`).MatchString(p.Version) {
			return fmt.Errorf("Enter a Python or R version in major.minor or major.minor.patch format.")
		}
		if remoteSource(p) == "python" && strings.Count(p.Version, ".") != 1 {
			return fmt.Errorf("Python uses Local's major.minor image series; enter a version such as 3.13.")
		}
	case "bioinformatics":
	default:
		return fmt.Errorf("Choose a remote image source.")
	}
	return nil
}
func remoteBaseImage(p RemoteProfile) string {
	switch remoteSource(p) {
	case "python", "r":
		data, err := launcherbundle.ProjectAsset(remotePreset(p), "Dockerfile.template")
		if err != nil {
			return ""
		}
		version := p.Version
		if p.ImageSource == "python" {
			version = strings.Join(strings.Split(version, ".")[:2], ".")
		}
		for _, line := range strings.Split(string(data), "\n") {
			if image, ok := strings.CutPrefix(line, "ARG BASE_IMAGE="); ok {
				return strings.NewReplacer("__PYTHON_VERSION__", version, "__R_VERSION__", version).Replace(strings.TrimSpace(image))
			}
		}
		return ""
	case "bioinformatics":
		return "docker.io/docker/sandbox-templates:shell-docker"
	case "registry":
		return strings.TrimPrefix(p.Image, "docker://")
	}
	return ""
}
func remoteDockerfile(p RemoteProfile) string {
	source := "FROM " + remoteBaseImage(p) + "\n"
	if remoteSource(p) == "dockerfile" {
		source = p.Dockerfile + "\n"
	}
	// COPY from an external image works with single- and multi-stage uploads,
	// without needing to parse or rename their final stage. No downloaded script
	// is run on the laptop: all RUN instructions execute in sandbox-local Docker.
	recipe := source + `
# VS Code Sandboxes: portable remote workspace dependencies.
USER root
COPY --from=node:22.22.1-bookworm-slim /usr/local/bin/node /usr/local/bin/node
COPY --from=node:22.22.1-bookworm-slim /usr/local/lib/node_modules /usr/local/lib/node_modules
RUN command -v apt-get >/dev/null || (echo 'Remote support requires a Debian/Ubuntu based final image' >&2; exit 1)
# Create the locked SSH privilege-separation account before package setup. This
# also avoids useradd's audit-library mapping failure under cross-build emulation.
RUN if ! getent passwd sshd >/dev/null; then uid=60000; while getent passwd "$uid" >/dev/null; do uid=$((uid+1)); done; printf 'sshd:x:%s:65534:SSH privilege separation:/run/sshd:/usr/sbin/nologin\n' "$uid" >> /etc/passwd; printf 'sshd:!:0:0:99999:7:::\n' >> /etc/shadow; fi
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends bash bash-completion openssh-server python3 python3-venv python3-pip git curl ca-certificates tar gzip bzip2 xz-utils procps sudo build-essential cmake gfortran pkg-config libbz2-dev libcurl4-openssl-dev libfontconfig1-dev libfreetype-dev libfribidi-dev libgit2-dev libharfbuzz-dev libjpeg-dev liblzma-dev libpcre2-dev libpng-dev libssl-dev libtiff-dev libuv1-dev libxml2-dev zlib1g-dev && rm -rf /var/lib/apt/lists/*
RUN ln -sf ../lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm && ln -sf ../lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx && NODE_OPTIONS=--jitless npm install -g --no-audit --no-fund @openai/codex@0.163.0 @anthropic-ai/claude-code@2.1.328
RUN mkdir -p /run/sshd /workspace /data /run/vscode-launcher && chmod 755 /run/sshd
ENV SHELL=/bin/bash
ENV PATH="/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
WORKDIR /workspace
`
	profileHash := sha256.Sum256([]byte(remoteAsset("seccomp-agents.json")))
	securityFiles := map[string]string{
		"seccomp-agents.json":     remoteAsset("seccomp-agents.json"),
		"verify-security.py":      remoteAsset("verify-security.py"),
		"slurm-unavailable.sh":    remoteAsset("slurm-unavailable.sh"),
		"APPTAINER.md":            licensing.ApptainerGuide,
		"THIRD-PARTY-NOTICES.txt": licensing.ThirdPartyNotices,
	}
	filesJSON, _ := json.Marshal(securityFiles)
	install := `import json,pathlib,os;root=pathlib.Path('/usr/local/share/vscode-sandboxes/security');root.mkdir(parents=True,exist_ok=True);files=json.loads(` + strconv.Quote(string(filesJSON)) + `);[(root/name).write_text(value) for name,value in files.items()];stub=root/'slurm-unavailable.sh';stub.chmod(0o755);commands='sacct sacctmgr salloc sattach sbatch sbcast scancel scontrol scrontab sdiag seff sinfo sprio squeue sreport srun sshare sstat strigger sview smap sjstat sh5util scrun'.split();[(pathlib.Path('/usr/local/bin')/name).unlink(missing_ok=True) for name in commands];[os.symlink(str(stub),'/usr/local/bin/'+name) for name in commands]`
	installArgv, _ := json.Marshal([]string{"python3", "-c", install})
	recipe += fmt.Sprintf("\nLABEL studio.bioinformatics.apptainer.policy=\"mpcdf-strict-v1\" studio.bioinformatics.apptainer.seccomp-sha256=\"%x\"\nRUN %s\n", profileHash, installArgv)
	if remoteSource(p) == "r" {
		recipe = strings.Replace(recipe, "procps sudo build-essential", "procps sudo patchelf build-essential", 1)
		align, _ := json.Marshal([]string{"python3", "-c", remoteAsset("align-r-libraries.py")})
		smoke, _ := json.Marshal([]string{"Rscript", "--vanilla", "-e", remoteAsset("smoke-r.R")})
		recipe += "\n# Align affected R dependencies for 16 KiB emulation; retain code/ABI and record hashes.\nRUN " + string(align) + "\n# Validate the runtime, not only successful package installation.\nRUN " + string(smoke) + "\n"
	}
	return recipe

}

func prepareRemoteImage(ctx context.Context, p RemoteProfile, token string, log io.Writer) (remoteImagePlan, error) {
	if remoteSource(p) == "sif" {
		return remoteImagePlan{Image: p.Image}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	if err := validateRemoteImage(p); err != nil {
		return remoteImagePlan{}, err
	}
	release, err := lockRemoteCache(false)
	if err != nil {
		return remoteImagePlan{}, err
	}
	defer release()
	dir := filepath.Join(settingsDir(), "remote-images", remoteImageIdentity(p))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return remoteImagePlan{}, err
	}
	plan := remoteImagePlan{Archive: filepath.Join(dir, "image.tar"), Project: filepath.Join(dir, "project.tar")}
	var cached map[string]string
	if readJSON(filepath.Join(dir, "cache.json"), &cached) == nil && len(cached) == 4 {
		valid := true
		for name, expected := range cached {
			if name != "Dockerfile" && name != "image.tar" && name != "image-lock.json" && name != "project.tar" {
				valid = false
				break
			}
			actual, err := remoteFileHash(filepath.Join(dir, name))
			if err != nil || actual != expected {
				valid = false
				break
			}
		}
		if valid {
			if err := refreshRemoteProjectArchive(p, dir, &cached); err != nil {
				return remoteImagePlan{}, err
			}
			fmt.Fprintln(log, "Reusing the cached image and pinned recipe:", filepath.Join(dir, "Dockerfile"))
			return plan, nil
		}
	}
	if legacy := matchingRemoteImageCache(p, dir); legacy != "" {
		var hashes map[string]string
		if err := readJSON(filepath.Join(legacy, "cache.json"), &hashes); err != nil {
			return remoteImagePlan{}, err
		}
		if err := refreshRemoteProjectArchive(p, legacy, &hashes); err != nil {
			return remoteImagePlan{}, err
		}
		fmt.Fprintln(log, "Reusing a verified previous image build:", legacy)
		return remoteImagePlan{Archive: filepath.Join(legacy, "image.tar"), Project: filepath.Join(legacy, "project.tar")}, nil
	}
	if err := checkRemoteBuildSpace(dir, remoteBuildMinimumFree); err != nil {
		return remoteImagePlan{}, err
	}
	buildCtx, stopBuild := context.WithCancel(ctx)
	defer os.Remove(filepath.Join(dir, "image.tar.partial"))
	storageFailure := make(chan error, 1)
	monitorDone := make(chan struct{})
	go func() { defer close(monitorDone); watchRemoteBuildSpace(buildCtx, stopBuild, dir, storageFailure) }()
	defer func() { stopBuild(); <-monitorDone }()
	buildFailure := func(err error) error {
		select {
		case storageErr := <-storageFailure:
			return storageErr
		default:
			return err
		}
	}
	if _, err := SandboxCLI(); err != nil {
		return remoteImagePlan{}, fmt.Errorf("Building an image needs local Docker Sandboxes: %w", err)
	}
	lockDir := filepath.Join(dir, "building")
	if err := os.Mkdir(lockDir, 0700); err != nil {
		return remoteImagePlan{}, fmt.Errorf("This image is already building, or an interrupted build needs its %s directory removed after checking the builder: %w", lockDir, err)
	}
	defer os.Remove(lockDir)
	if err := privateWrite(filepath.Join(dir, "Dockerfile"), []byte(remoteDockerfile(p))); err != nil {
		return remoteImagePlan{}, err
	}
	if err := privateWrite(filepath.Join(dir, ".dockerignore"), []byte("**\n!Dockerfile\n")); err != nil {
		return remoteImagePlan{}, err
	}
	// This folder contains the uploaded recipe only, never the user's project or
	// home. COPY/ADD local paths require a self-contained recipe (or online ADD).
	name := "remote-build-" + token
	sbx, err := SandboxCLI()
	if err != nil {
		return remoteImagePlan{}, err
	}
	run := func(args ...string) error {
		cmd := exec.CommandContext(buildCtx, sbx, args...)
		hideWindow(cmd)
		cmd.Stdout, cmd.Stderr = log, log
		return cmd.Run()
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(cleanup, sbx, "rm", "--force", name)
		hideWindow(cmd)
		cmd.Stdout, cmd.Stderr = log, log
		_ = cmd.Run()
	}()
	if err := run("run", "--detached", "--name", name, "shell", dir); err != nil {
		return remoteImagePlan{}, buildFailure(fmt.Errorf("Start local image builder: %w", err))
	}
	// Scope build egress to this temporary VM; do not reset shared policy.
	if err := run("policy", "allow", "network", "--sandbox", name, "**"); err != nil {
		return remoteImagePlan{}, buildFailure(err)
	}
	cmd := exec.CommandContext(buildCtx, sbx, "exec", "-i", name, "bash", "-s", "--", remoteArchitecture(p), remoteBaseImage(p))
	hideWindow(cmd)
	cmd.Stdin = strings.NewReader(remoteAsset("local-build.sh"))
	output := &remoteBuildOutput{target: log}
	cmd.Stdout, cmd.Stderr = output, output
	if err := cmd.Run(); err != nil {
		return remoteImagePlan{}, buildFailure(output.failure(err))
	}
	dockerfile, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	if err != nil {
		return remoteImagePlan{}, err
	}
	lock, err := os.ReadFile(filepath.Join(dir, "image-lock.json"))
	if err != nil {
		return remoteImagePlan{}, err
	}
	files, err := remoteProjectFiles(p, string(dockerfile), lock)
	if err != nil {
		return remoteImagePlan{}, err
	}
	project := filepath.Join(dir, "project.tar")
	if err := writeRemoteProjectTar(project, files); err != nil {
		return remoteImagePlan{}, err
	}
	fmt.Fprintln(log, "Built image and retained reusable Dockerfile:", filepath.Join(dir, "Dockerfile"))
	checksums := map[string]string{}
	for _, name := range []string{"Dockerfile", "image.tar", "image-lock.json", "project.tar"} {
		digest, err := remoteFileHash(filepath.Join(dir, name))
		if err != nil {
			return remoteImagePlan{}, err
		}
		checksums[name] = digest
	}
	if err := writeJSON(filepath.Join(dir, "cache.json"), checksums); err != nil {
		return remoteImagePlan{}, err
	}
	return plan, nil
}

func remoteFileHash(filename string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer f.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func (p *remoteController) stageRemoteImage(ctx context.Context, s RemoteSession, plan *remoteImagePlan) error {
	if plan.Archive == "" && plan.Project == "" {
		return nil
	}
	p.phase("Uploading image archive and reusable project recipe")
	upload := p.upload
	if upload == nil {
		upload = func(ctx context.Context, host, local, remote string) error {
			return uploadRemoteFileWithProgress(ctx, host, local, remote, p)
		}
	}
	transferImage := plan.Archive != ""
	if transferImage {
		recipe, err := os.ReadFile(filepath.Join(filepath.Dir(plan.Archive), "Dockerfile"))
		if err != nil {
			return err
		}
		lock, err := os.ReadFile(filepath.Join(filepath.Dir(plan.Archive), "image-lock.json"))
		if err != nil {
			return err
		}
		record := describeRemoteImage(s.Profile, string(recipe), lock)
		var reserved remoteImageRecord
		if err := p.catalog(ctx, s.Profile, "reserve", record, &reserved); err != nil {
			return err
		}
		if reserved.ID != record.ID || !safeRemotePath(reserved.Path, false) {
			return fmt.Errorf("Invalid reserved image path")
		}
		plan.Image, plan.Key = reserved.Path, record.ID
		if reserved.State == "ready" {
			transferImage = false
			fmt.Fprintln(p, "The server image is already ready; skipping image transfer.")
		}
	}
	result, err := p.ssh(ctx, s.Profile.Login, remoteAsset("stage-image.sh"), s.Profile.StateDir, s.Token)
	if err != nil {
		return err
	}
	stage := parseRemoteValues(result)["STAGE"]
	if !safeRemotePath(stage, false) || !strings.HasSuffix(stage, "/uploads/"+s.Token) {
		return fmt.Errorf("Invalid remote image staging directory")
	}
	if !transferImage {
		p.phase("Uploading project template for cached server image")
	}
	if transferImage {
		if err := upload(ctx, s.Profile.Login, plan.Archive, stage+"/image.tar"); err != nil {
			return err
		}
	}
	if err := upload(ctx, s.Profile.Login, plan.Project, stage+"/project.tar"); err != nil {
		return err
	}
	plan.Stage = stage
	return nil
}

func uploadRemoteFile(ctx context.Context, host, local, remote string) error {
	return uploadRemoteFileWithProgress(ctx, host, local, remote, io.Discard)
}

func uploadRemoteFileWithProgress(ctx context.Context, host, local, remote string, log io.Writer) error {
	binary, err := hostSSHBinary("ssh")
	if err != nil {
		return err
	}
	source, err := os.Open(local)
	if err != nil {
		return err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return err
	}
	fmt.Fprintf(log, "Preparing upload %s (%.1f MiB); calculating checksum locally…\n", filepath.Base(local), float64(info.Size())/(1<<20))
	digest := sha256.New()
	if _, err := io.Copy(digest, source); err != nil {
		return err
	}
	if _, err := source.Seek(0, 0); err != nil {
		return err
	}
	script := `set -eu; umask 077; target=` + remoteQuote(remote) + `; temporary="$target.part"; trap 'rm -f "$temporary"' EXIT; cat > "$temporary"; printf '%s  %s\n' ` + remoteQuote(hex.EncodeToString(digest.Sum(nil))) + ` "$temporary" | sha256sum -c - >/dev/null; mv "$temporary" "$target"`
	cmd := exec.CommandContext(ctx, binary, "-T", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "ForwardAgent=no", "-o", "ConnectTimeout=15", "-o", "ServerAliveInterval=30", host, "bash -c "+remoteQuote(script))
	hideWindow(cmd)
	started := time.Now()
	reader := &remoteTransferReader{source: source, log: log, name: filepath.Base(local), total: info.Size(), started: started, last: started, now: time.Now}
	cmd.Stdin = reader
	fmt.Fprintf(log, "Uploading %s over the existing SSH stream…\n", reader.name)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Upload image/project archive: %w: %s", err, stderr.String())
	}
	fmt.Fprintf(log, "Upload %s complete: %.1f MiB transferred and remote SHA-256 verified.\n", reader.name, float64(reader.sent)/(1<<20))
	return nil
}

func remoteProjectFiles(p RemoteProfile, dockerfile string, lock []byte) (map[string][]byte, error) {
	files := map[string][]byte{"code/Dockerfile": []byte(dockerfile), "code/.sandbox/remote-image.lock.json": lock,
		"code/.sandbox/VS-Code-Sandboxes-DISCLAIMER.txt":          []byte(licensing.Disclaimer),
		"code/.sandbox/VS-Code-Sandboxes-LICENSE.txt":             []byte(licensing.License),
		"code/.sandbox/VS-Code-Sandboxes-THIRD-PARTY-NOTICES.txt": []byte(licensing.ThirdPartyNotices)}
	files["code/.sandbox/APPTAINER.md"] = []byte(licensing.ApptainerGuide)
	slug := remotePreset(p)
	if slug != "" {
		for source, target := range map[string]string{"settings.json": ".vscode/settings.json", "extensions.json": ".vscode/extensions.json", "AGENTS.md": "AGENTS.md", "gitignore": ".gitignore", "gitattributes": ".gitattributes", "pre-commit": "code/.sandbox/pre-commit"} {
			data, err := launcherbundle.ProjectAsset(slug, source)
			if err != nil {
				return nil, err
			}
			if source == "AGENTS.md" {
				data = []byte(remoteAgentInstructions(string(data)))
			}
			files[target] = data
		}
		// Retain the same language files as Local; the remote entry point replaces
		// the privileged/nested-container runtime, not the project conventions.
		if p.ImageSource == "r" {
			for _, name := range []string{"activate-project.R", "setup-renv.R", "verify-renv.R", "setup-vscode-r.R", "install-r-system-dependencies.sh", "Renviron", "Rprofile"} {
				data, err := launcherbundle.ProjectAsset(slug, name)
				if err != nil {
					return nil, err
				}
				dest := "code/.sandbox/" + name
				if name == "Renviron" || name == "Rprofile" {
					dest = "." + name
				}
				files[dest] = data
			}
		}
		if p.ImageSource == "r" {
			files["code/.sandbox/verify-remote-renv.R"] = []byte(remoteAsset("verify-r-environment.R"))
			files["code/.sandbox/setup-remote-renv.R"] = []byte(strings.ReplaceAll(string(files["code/.sandbox/setup-renv.R"]), "code/.sandbox/verify-renv.R", "code/.sandbox/verify-remote-renv.R"))
		}
		if p.ImageSource == "python" || p.ImageSource == "r" {
			data, err := launcherbundle.ProjectAsset(slug, "devcontainer.json.template")
			if err != nil {
				return nil, err
			}
			version := p.Version
			if p.ImageSource == "python" {
				version = strings.Join(strings.Split(version, ".")[:2], ".")
			}
			text := strings.NewReplacer("__PYTHON_VERSION__", version, "__R_VERSION__", version, "__AGENT__", "both").Replace(string(data))
			files["code/.sandbox/devcontainer.json"] = []byte(text)
			manifest, _ := json.Marshal(map[string]string{"language": p.ImageSource, "series": version, "baseImage": remoteBaseImage(p)})
			files["code/.sandbox/sandbox-runtime.json"] = manifest
		}
		for _, name := range []string{"setup-skills.sh", "install-coding-agents.sh", "prepare-continue.py", "install-continue.js", "setup-continue.sh", "agent-sandbox-seccomp.json", "agent-sandbox-seccomp-LICENSE.txt", "agent-sandbox-seccomp-NOTICE.txt", "development-container.js", "initialize-development-container.sh", "start-development-container.sh", "verify-development-container.sh"} {
			data, err := launcherbundle.CommonAsset(name)
			if err != nil {
				return nil, err
			}
			files["code/.sandbox/"+name] = data
		}
		files[".instructions.md"] = []byte("Follow AGENTS.md. Execute inside the remote Apptainer workspace; store implementation in code/ and datasets/results in data/.\n")
	}
	manifest, _ := json.Marshal(map[string]string{"source": remoteSource(p), "version": p.Version, "architecture": remoteArchitecture(p)})
	files["code/.sandbox/remote-project.json"] = manifest
	files["code/.sandbox/initialize-remote.sh"] = []byte(remoteAsset("initialize-project.sh"))
	return files, nil
}
func remoteAgentInstructions(local string) string {
	var lines []string
	for _, line := range strings.Split(local, "\n") {
		if strings.Contains(line, "Docker ") || strings.Contains(line, "Podman") || strings.Contains(line, "docker ") || strings.Contains(line, "podman ") || strings.Contains(line, "sbx ") {
			continue
		}
		line = strings.NewReplacer("development container", "Apptainer container", "Dev Containers", "Remote SSH").Replace(line)
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n") + "\n## Remote execution\n\nExecute all code, terminals and agents inside this allocated Apptainer container. Keep implementation and dependency manifests under code/, data/results under data/, Python packages in the project .venv and R packages in .r-library with renv.lock. Retain code/Dockerfile (or code/Dockerfile.remote if an existing recipe was preserved) and code/.sandbox/remote-image.lock.json for image reproduction. Rebuild image-level packages through the launcher from the retained Dockerfile; do not use sudo or attempt nested Docker/Podman inside the HPC container. The project is writable at its exact expanded HPC path; additional folders are read-only. Save every result and installed project package inside the project directory: it survives the allocation. Container home, editor/agent state and temporary files are allocation-local and disposable. Project write/chmod operations remain subject to HPC ownership and ACLs. Slurm expiry stops processes.\n"
}
func writeRemoteProjectTar(filename string, files map[string][]byte) error {
	var buffer bytes.Buffer
	tw := tar.NewWriter(&buffer)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		data := files[name]
		mode := int64(0644)
		if strings.HasSuffix(name, ".sh") || strings.HasSuffix(name, "pre-commit") {
			mode = 0755
		}
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(data))}); err != nil {
			return err
		}
		if _, err := tw.Write(data); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return privateWrite(filename, buffer.Bytes())
}
func remoteExtensions(p RemoteProfile) []string {
	if p.ImageSource == "cached" {
		p.ImageSource = p.CachedSource
	}
	// Remote SSH accepts extension IDs, not version-qualified installation specs.
	// The shared container bootstrap installs and pins Continue 2.0.0 explicitly.
	result := []string{"openai.chatgpt", "anthropic.claude-code", "Continue.continue"}
	slug := remotePreset(p)
	if slug != "" {
		data, err := launcherbundle.ProjectAsset(slug, "extensions.json")
		if err == nil {
			var config struct {
				Recommendations []string `json:"recommendations"`
			}
			if json.Unmarshal(data, &config) == nil {
				for _, ext := range config.Recommendations {
					found := false
					for _, old := range result {
						if strings.Split(old, "@")[0] == strings.Split(ext, "@")[0] {
							found = true
						}
					}
					if !found {
						result = append(result, ext)
					}
				}
			}
		}
	}
	if slug == "" {
		result = append(result, "ms-python.python", "ms-toolsai.jupyter", "REditorSupport.r")
	}
	return result
}
