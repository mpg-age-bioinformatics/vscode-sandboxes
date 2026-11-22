package startui

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mpg-age-bioinformatics/vscode-sandboxes/internal/launcherbundle"
)

//go:embed remote_assets/*
var remoteAssets embed.FS

type RemoteProfile struct {
	Network          string   `json:"network,omitempty"`
	Sites            string   `json:"sites,omitempty"`
	EditorCommit     string   `json:"editorCommit,omitempty"`
	EditorExtensions []string `json:"editorExtensions,omitempty"`
	Login            string   `json:"login"`
	Project          string   `json:"project"`
	Data             string   `json:"data,omitempty"` // Legacy preferences only.
	Folders          string   `json:"folders,omitempty"`
	StateDir         string   `json:"stateDir"`
	ImagesDir        string   `json:"imagesDir"`
	CachedImage      string   `json:"cachedImage,omitempty"`
	CachedSource     string   `json:"cachedSource,omitempty"`
	Image            string   `json:"image"`
	ImageSource      string   `json:"imageSource,omitempty"`
	Version          string   `json:"version,omitempty"`
	Dockerfile       string   `json:"dockerfile,omitempty"`
	Architecture     string   `json:"architecture,omitempty"`
	Partition        string   `json:"partition"`
	Account          string   `json:"account,omitempty"` // Legacy preferences, never submitted.
	CPUs             int      `json:"cpus"`
	Memory           string   `json:"memory"`
	Time             string   `json:"time"`
	GPUs             int      `json:"gpus"`
	Apptainer        string   `json:"apptainer"`
	Modules          string   `json:"modules"` // Optional advanced shell initialization.
	ModuleNames      string   `json:"moduleNames,omitempty"`
	WaitMinutes      int      `json:"waitMinutes"`
}

type RemoteSession struct {
	ProjectPath string        `json:"projectPath,omitempty"`
	Profile     RemoteProfile `json:"profile"`
	Token       string        `json:"token"`
	JobID       string        `json:"jobID"`
	RunDir      string        `json:"runDir"`
	Node        string        `json:"node"`
	Port        int           `json:"port"`
	User        string        `json:"user"`
	Active      bool          `json:"active"`
	QueueState  string        `json:"queueState,omitempty"`
}

type RemoteStatus struct {
	Running bool          `json:"running"`
	Phase   string        `json:"phase"`
	Error   string        `json:"error"`
	Text    string        `json:"text"`
	Session RemoteSession `json:"session"`
}

type remoteController struct {
	sync.Mutex
	state         RemoteStatus
	cancel        context.CancelFunc
	stopping      bool
	cacheClearing bool
	ssh           func(context.Context, string, string, ...string) (string, error)
	pollInterval  time.Duration
	lastCheck     time.Time
	checking      bool
	generation    uint64
	prepareImage  func(context.Context, RemoteProfile, string, io.Writer) (remoteImagePlan, error)
	upload        func(context.Context, string, string, string) error
}

func defaultRemoteProfile() RemoteProfile {
	return RemoteProfile{Network: "open", Project: "~/sandboxes", Partition: "cluster", StateDir: "~/.local/share/vscode-sandboxes/remote", ImagesDir: "~/.local/share/vscode-sandboxes/images", CPUs: 1, Memory: "8G", Time: "2-00:00:00", Apptainer: "apptainer", WaitMinutes: 30}
}

var remoteHostPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*(?:@[A-Za-z0-9][A-Za-z0-9_.-]*)?$`)
var remoteNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
var remoteImageIDPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var remoteJobPattern = regexp.MustCompile(`^[0-9]+$`)
var remoteTokenPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

func validateRemoteProfile(p RemoteProfile) error {
	if err := validateRemoteNetwork(p); err != nil {
		return err
	}
	if err := validateRemoteImage(p); err != nil {
		return err
	}
	if !remoteHostPattern.MatchString(p.Login) {
		return errors.New("Enter an existing SSH host alias or user@hostname, without spaces or command options.")
	}
	if !safeRemotePath(p.Project, true) {
		return errors.New("Project must be an absolute cluster path or start with ~/.")
	}
	if !strings.HasPrefix(p.Project, "~/") {
		cleaned := path.Clean(p.Project)
		for _, reserved := range []string{"/run", "/proc", "/sys", "/dev", "/tmp", "/var/tmp", "/etc", "/usr", "/bin", "/sbin", "/lib", "/lib64"} {
			if cleaned == reserved || strings.HasPrefix(cleaned, reserved+"/") {
				return errors.New("Choose a project outside container runtime and system directories.")
			}
		}
	}
	if _, err := remoteFolders(p.Folders); err != nil {
		return err
	}
	if _, err := remoteModuleNames(p.ModuleNames); err != nil {
		return err
	}
	if !safeRemotePath(p.ImagesDir, true) {
		return errors.New("Image storage must be an absolute cluster path or start with ~/.")
	}
	if !safeRemotePath(p.StateDir, true) {
		return errors.New("Persistent state must be an absolute cluster path or start with ~/.")
	}
	if p.Partition != "" && !remoteNamePattern.MatchString(p.Partition) {
		return errors.New("Partition may contain letters, numbers, dots, underscores, and hyphens.")
	}
	if p.CPUs < 1 || p.CPUs > 1024 || p.GPUs < 0 || p.GPUs > 64 {
		return errors.New("Choose 1–1024 CPUs and 0–64 GPUs.")
	}
	if !regexp.MustCompile(`^[1-9][0-9]{0,6}[MG]$`).MatchString(p.Memory) {
		return errors.New("Enter memory as a positive amount with M or G, for example 8G.")
	}
	if !regexp.MustCompile(`^(?:[0-9]+-)?[0-9]{1,3}:[0-5][0-9]:[0-5][0-9]$`).MatchString(p.Time) {
		return errors.New("Enter the Slurm time limit as HH:MM:SS or days-HH:MM:SS.")
	}
	if p.WaitMinutes < 1 || p.WaitMinutes > 1440 {
		return errors.New("Queue wait must be between 1 and 1440 minutes.")
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_./-]+$`).MatchString(p.Apptainer) || strings.HasPrefix(p.Apptainer, "-") {
		return errors.New("Enter the Apptainer executable name or path; put module commands in Module setup.")
	}
	if strings.ContainsAny(p.Modules, "\x00\r") || len(p.Modules) > 16384 {
		return errors.New("Module setup contains unsupported characters or exceeds 16 KiB.")
	}
	return nil
}

func safeRemotePath(value string, home bool) bool {
	if home && strings.HasPrefix(value, "~/") {
		value = "/" + value[2:]
	}
	if !strings.HasPrefix(value, "/") || path.Clean(value) == "/" {
		return false
	}
	for _, r := range value {
		if r < 32 || r == 127 || r == ':' || r == ',' {
			return false
		}
	}
	return true
}

func remoteAsset(name string) string {
	data, err := remoteAssets.ReadFile("remote_assets/" + name)
	if err != nil {
		panic(err)
	}
	result := string(data)
	if strings.Contains(result, "# VSCODE_SECURITY_HELPERS") {
		result = strings.ReplaceAll(result, "# VSCODE_SECURITY_HELPERS", remoteAsset("security.sh"))
	}
	if strings.Contains(result, "__VSCODE_SECCOMP_JSON__") {
		result = strings.ReplaceAll(result, "__VSCODE_SECCOMP_JSON__", remoteAsset("seccomp-agents.json"))
	}
	if strings.Contains(result, "# VSCODE_VERIFY_SECURITY") {
		result = strings.ReplaceAll(result, "# VSCODE_VERIFY_SECURITY", remoteAsset("verify-security.py"))
	}
	if strings.Contains(result, "# VSCODE_BASH_PROMPT") {
		result = strings.ReplaceAll(result, "# VSCODE_BASH_PROMPT", remoteAsset("bash-prompt.sh"))
	}
	if strings.Contains(result, "# VSCODE_NETWORK_ASSETS") {
		var scripts strings.Builder
		for _, file := range []string{"network.py", "prepare-editor.py"} {
			fmt.Fprintf(&scripts, "cat > \"$run/%s\" <<'VSCODE_NETWORK_ASSET'\n%s\nVSCODE_NETWORK_ASSET\n", file, remoteAsset(file))
		}
		result = strings.ReplaceAll(result, "# VSCODE_NETWORK_ASSETS", scripts.String())
	}
	if strings.Contains(result, "# VSCODE_CONTINUE_ASSETS") {
		var scripts strings.Builder
		for _, file := range []string{"prepare-continue.py", "install-continue.js"} {
			data, err := launcherbundle.CommonAsset(file)
			if err != nil {
				panic(err)
			}
			fmt.Fprintf(&scripts, "cat > \"$run/%s\" <<'VSCODE_CONTINUE_ASSET'\n%s\nVSCODE_CONTINUE_ASSET\n", file, data)
		}
		result = strings.ReplaceAll(result, "# VSCODE_CONTINUE_ASSETS", scripts.String())
	}
	return result
}
func remoteQuote(value string) string      { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
func remoteSessionDir(token string) string { return filepath.Join(settingsDir(), "remote", token) }
func remoteSessionFile() string            { return filepath.Join(settingsDir(), "remote-session.json") }

func hostSSHBinary(name string) (string, error) {
	if runtime.GOOS == "windows" {
		candidate := filepath.Join(os.Getenv("WINDIR"), "System32", "OpenSSH", name+".exe")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	binary, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("OpenSSH %s is required on your laptop. Configure and test your cluster SSH alias in a terminal first.", name)
	}
	return binary, nil
}

func loginSSH(ctx context.Context, host, script string, args ...string) (string, error) {
	binary, err := hostSSHBinary("ssh")
	if err != nil {
		return "", err
	}
	var input strings.Builder
	input.WriteString("set --")
	for _, arg := range args {
		if strings.ContainsRune(arg, 0) {
			return "", errors.New("Remote script argument contains a NUL byte.")
		}
		input.WriteString(" " + remoteQuote(arg))
	}
	input.WriteString("\n")
	input.WriteString(script)
	options, err := loginSSHOptions(host)
	if err != nil {
		return "", err
	}
	background, _ := ctx.Value(sshBackgroundProbe{}).(bool)
	if background {
		for i, option := range options {
			if option == "BatchMode=no" {
				options[i] = "BatchMode=yes"
			}
		}
	}
	call := exec.CommandContext(ctx, binary, append(options, host, "bash -s --")...)
	if !background {
		closePrompts, err := configureSSHAskpass(call, host)
		if err != nil {
			return "", err
		}
		defer closePrompts()
	}
	hideWindow(call)
	call.Stdin = strings.NewReader(input.String())
	var stdout, stderr strings.Builder
	call.Stdout, call.Stderr = &stdout, &stderr
	err = call.Run()
	if err != nil {
		return stdout.String(), fmt.Errorf("SSH to %s failed: %w. %s Complete any password or 2FA prompt in the launcher. Verify your SSH alias and known host key if authentication fails.", host, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func newRemoteController() *remoteController {
	p := &remoteController{ssh: loginSSH, pollInterval: 3 * time.Second}
	var saved RemoteSession
	if readJSON(remoteSessionFile(), &saved) == nil {
		saved.Profile = normalizeRemoteProfile(saved.Profile)
		if validRemoteSession(saved) {
			p.state.Session = saved
			p.state.Phase = "Saved allocation — reconnect to check its state"
		}
	}
	return p
}
func validRemoteSession(s RemoteSession) bool {
	return remoteTokenPattern.MatchString(s.Token) && validateRemoteProfile(s.Profile) == nil && (s.JobID == "" || remoteJobPattern.MatchString(s.JobID)) && (s.RunDir == "" || safeRemotePath(s.RunDir, false)) && (s.ProjectPath == "" || safeRemotePath(s.ProjectPath, false))
}
func (p *remoteController) snapshot() RemoteStatus { p.Lock(); defer p.Unlock(); return p.state }
func (p *remoteController) busy() bool {
	p.Lock()
	defer p.Unlock()
	return p.state.Running || p.cacheClearing
}
func (p *remoteController) Write(data []byte) (int, error) {
	p.Lock()
	defer p.Unlock()
	p.state.Text += string(data)
	if len(p.state.Text) > 128<<10 {
		p.state.Text = p.state.Text[len(p.state.Text)-(128<<10):]
	}
	return len(data), nil
}
func (p *remoteController) phase(phase string) { p.Lock(); p.state.Phase = phase; p.Unlock() }
func (p *remoteController) saveSession(s RemoteSession) error {
	p.Lock()
	defer p.Unlock()
	if p.state.Session.Token != "" && p.state.Session.Token != s.Token {
		return context.Canceled
	}
	if s.Active && (p.stopping || !p.state.Session.Active) {
		return context.Canceled
	}
	p.state.Session = s
	return writeJSON(remoteSessionFile(), s)
}

func (p *remoteController) start(profile RemoteProfile, reconnect bool, endpoints ...remoteEndpoint) error {
	connection := remoteEndpoint{}
	if len(endpoints) > 0 {
		connection = endpoints[0].normalized()
	}
	if err := connection.validate(); err != nil {
		return err
	}
	profile = normalizeRemoteProfile(profile)
	if !reconnect {
		var err error
		profile, _, err = remoteEndpointNetwork(profile, connection)
		if err != nil {
			return err
		}
	}
	if !reconnect {
		if err := validateRemoteProfile(profile); err != nil {
			return err
		}
	}
	if _, err := hostSSHBinary("ssh"); err != nil {
		return err
	}
	if _, err := hostSSHBinary("ssh-keygen"); err != nil {
		return err
	}
	if _, err := editorCommand("--version"); err != nil {
		return err
	}
	p.Lock()
	defer p.Unlock()
	if p.state.Running || p.cacheClearing {
		return errors.New("Wait for remote startup or cache cleanup to finish.")
	}
	if !reconnect && p.state.Session.Active {
		return errors.New("Reconnect to or cancel the existing allocation before starting another.")
	}
	session := p.state.Session
	if reconnect {
		if !session.Active || !validRemoteSession(session) {
			return errors.New("There is no saved allocation to reconnect to.")
		}
		profile = session.Profile
		if err := validateExistingRemoteEndpoint(profile, connection); err != nil {
			return err
		}
	} else {
		random := make([]byte, 16)
		if _, err := rand.Read(random); err != nil {
			return err
		}
		session = RemoteSession{Profile: profile, Token: hex.EncodeToString(random), Active: true}
	}
	if p.cancel != nil {
		p.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.stopping = false
	p.generation++
	generation := p.generation
	p.state = RemoteStatus{Running: true, Phase: "Connecting to HPC", Session: session}
	go func() {
		err := p.launch(ctx, session, reconnect, connection)
		p.Lock()
		defer p.Unlock()
		if p.generation != generation {
			return
		}
		p.state.Running = false
		if p.stopping {
			return
		}
		if err != nil {
			p.state.Error = err.Error()
			p.state.Phase = "Remote startup stopped"
		}
	}()
	return nil
}

func parseRemoteValues(text string) map[string]string {
	result := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		if key, value, ok := strings.Cut(line, "="); ok {
			result[key] = value
		}
	}
	return result
}

func (p *remoteController) launch(ctx context.Context, session RemoteSession, reconnect bool, endpoints ...remoteEndpoint) error {
	connection := remoteEndpoint{}
	if len(endpoints) > 0 {
		connection = endpoints[0]
	}
	if !reconnect {
		release, err := lockRemoteCache(false)
		if err != nil {
			session.Active = false
			_ = p.saveSession(session)
			return err
		}
		defer release()
	}
	if err := writeJSON(filepath.Join(settingsDir(), "remote-profile.json"), session.Profile); err != nil {
		return err
	}
	if !reconnect && session.Profile.Network != "" && session.Profile.Network != "open" {
		if err := prepareRemoteEditorProfile(ctx, &session.Profile); err != nil {
			return err
		}
		fmt.Fprintf(p, "Remote internet: %s. Editor provisioning downloads run separately without project or data access.\n", session.Profile.Network)
	}
	if !reconnect {
		// Build before recording a possible allocation. A build failure has not
		// submitted anything and must not leave a phantom active job behind.
		p.phase("Preparing remote image and project files")
		prepare := p.prepareImage
		if prepare == nil {
			prepare = prepareRemoteImage
		}
		plan, hit, err := p.cachedRemotePlan(ctx, &session)
		if err == nil && !hit {
			plan, err = prepare(ctx, session.Profile, session.Token, p)
		}
		if err != nil {
			session.Active = false
			_ = p.saveSession(session)
			return err
		}
		if err := p.stageRemoteImage(ctx, session, &plan); err != nil {
			session.Active = false
			_ = p.saveSession(session)
			return err
		}
		dir := remoteSessionDir(session.Token)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		binary, err := hostSSHBinary("ssh-keygen")
		if err != nil {
			return err
		}
		key := filepath.Join(dir, "id_ed25519")
		call := exec.CommandContext(ctx, binary, "-q", "-t", "ed25519", "-N", "", "-C", "vscode-sandboxes-remote", "-f", key)
		hideWindow(call)
		call.Stdout, call.Stderr = p, p
		if err := call.Run(); err != nil {
			return fmt.Errorf("Generate allocation SSH key: %w", err)
		}
		if err := p.saveSession(session); err != nil {
			return err
		}
		publicKey, err := os.ReadFile(key + ".pub")
		if err != nil {
			return err
		}
		encode := func(text string) string { return base64.StdEncoding.EncodeToString([]byte(text)) }
		f := session.Profile
		if connection.BaseURL != "" && f.Network == "restricted" {
			_, host, _ := remoteEndpointNetwork(f, connection)
			fmt.Fprintln(p, privateEndpointNotice(host))
		}
		p.phase("Submitting Slurm allocation")
		result, submitErr := p.ssh(ctx, f.Login, remoteAsset("submit.sh"), f.StateDir, session.Token, f.Project, f.Folders, plan.Image, f.Partition, strconv.Itoa(f.CPUs), f.Memory, f.Time, strconv.Itoa(f.GPUs), f.Apptainer, encode(string(publicKey)), encode(remoteModuleScript(f)), encode(remoteAsset("job.sh")), encode(remoteAsset("container.sh")), plan.Stage, encode(remoteAsset("initialize-project.sh")), encode(remoteAsset("relay.sh")), encode(remoteProfileJSON(session.Profile)), plan.Key, encode(remoteAsset("watchdog.py")))
		values := parseRemoteValues(result)
		session.RunDir, session.JobID = values["RUN"], values["JOB"]
		if submitErr != nil || !remoteJobPattern.MatchString(session.JobID) || !safeRemotePath(session.RunDir, false) {
			_ = p.saveSession(RemoteSession{Profile: f, Token: session.Token, Active: true})
			return fmt.Errorf("Submission could not be confirmed. Use Reconnect to recover this allocation before submitting again. %v", submitErr)
		}
		if err := p.saveSession(session); err != nil {
			return err
		}
		fmt.Fprintf(p, "Submitted Slurm job %s. Closing this window does not cancel the allocation.\n", session.JobID)
	}
	if session.JobID == "" {
		result, err := p.ssh(ctx, session.Profile.Login, remoteAsset("recover.sh"), session.Profile.StateDir, session.Token)
		if err != nil {
			return err
		}
		values := parseRemoteValues(result)
		if values["STATE"] == "ENDED" {
			session.Active = false
			_ = p.saveSession(session)
			return errors.New("No live allocation was found. You can start a new job.")
		}
		session.RunDir, session.JobID = values["RUN"], values["JOB"]
		if !validRemoteSession(session) || session.JobID == "" || session.RunDir == "" {
			return errors.New("Recovered allocation metadata is invalid.")
		}
		if err := p.saveSession(session); err != nil {
			return err
		}
	}
	waitCtx, cancel := context.WithTimeout(ctx, time.Duration(session.Profile.WaitMinutes)*time.Minute)
	defer cancel()
	lastQueue := ""
	logOffset := "0"
	for {
		queryCtx, stop := context.WithTimeout(waitCtx, 45*time.Second)
		output, err := p.ssh(queryCtx, session.Profile.Login, remoteAsset("probe.sh"), session.RunDir, session.Token, session.JobID, logOffset)
		stop()
		if err != nil {
			return err
		}
		values := parseRemoteValues(output)
		if next, err := strconv.ParseInt(values["LOG_OFFSET"], 10, 64); err == nil && next >= 0 {
			if chunk, err := base64.StdEncoding.DecodeString(values["LOG64"]); err == nil && len(chunk) <= 32768 {
				_, _ = p.Write(chunk)
				logOffset = strconv.FormatInt(next, 10)
			}
		}
		if values["STATE"] == "ENDED" {
			session.Active = false
			_ = p.saveSession(session)
			fmt.Fprintln(p, output)
			return errors.New("The Slurm job ended before connecting. Check its output above and the cluster queue.")
		}
		if queue := values["QUEUE"]; queue != lastQueue {
			fmt.Fprintln(p, "Slurm:", queue)
			lastQueue = queue
		}
		if values["STATE"] == "READY" {
			session.Node, session.User = values["HOST"], values["USER"]
			session.Port, _ = strconv.Atoi(values["PORT"])
			session.ProjectPath = values["PROJECT"]
			if session.ProjectPath != "" && !safeRemotePath(session.ProjectPath, false) {
				return errors.New("The container project path is invalid.")
			}
			if values["JOB"] != session.JobID || (len(session.Node) > 255 || strings.ContainsAny(session.Node, "\x00\r\n")) || !regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`).MatchString(session.User) || session.User == "root" || session.Port < 1024 || session.Port > 65535 {
				return errors.New("The container endpoint metadata is invalid.")
			}
			hostKey := ""
			for _, line := range strings.Split(output, "\n") {
				if strings.HasPrefix(line, "ssh-ed25519 ") {
					hostKey = line
					break
				}
			}
			if err := prepareRemoteSSH(session, hostKey); err != nil {
				return err
			}
			if err := p.saveSession(session); err != nil {
				return err
			}
			p.phase("Checking the container SSH endpoint")
			alias := "vscode-sandboxes-" + session.Token
			proof, err := p.ssh(waitCtx, alias, `set -eu; test -r /run/vscode-launcher/sshd_config; test "$VSCODE_SANDBOX_REMOTE_CONTAINER" = "$1"; command -v codex >/dev/null; test -w "$HOME"; printf 'CONTAINER_OK\n'`, session.JobID)
			if err != nil || strings.TrimSpace(proof) != "CONTAINER_OK" {
				return fmt.Errorf("Could not verify that SSH terminates inside the allocated Apptainer container: %v", err)
			}
			if connection.Reset || connection.BaseURL != "" || connection.APIKey != "" {
				if session.ProjectPath == "" {
					return errors.New("Start a new allocation before configuring private credentials; this legacy allocation has a persistent container home.")
				}
				if err := validateExistingRemoteEndpoint(session.Profile, connection); err != nil {
					return err
				}
				p.phase("Configuring remote agent connection")
				if err := p.configureEndpoint(waitCtx, alias, connection); err != nil {
					return err
				}
			}
			p.phase("Opening VS Code inside Apptainer")
			if err := openRemoteEditor(waitCtx, alias, p, session.Profile, session.ProjectPath); err != nil {
				return err
			}
			p.phase("Connected to job " + session.JobID + " on " + session.Node)
			fmt.Fprintln(p, "VS Code Server, extensions, Codex, terminals and notebooks run inside Apptainer. Persistent files remain on the cluster.")
			return nil
		}
		if phase := values["STARTUP_PHASE"]; phase != "" {
			p.phase(phase)
		} else if strings.Contains(values["QUEUE"], "|RUNNING|") {
			p.phase("Job " + session.JobID + " running · preparing container and project")
		} else {
			p.phase("Waiting for job " + session.JobID)
		}
		timer := time.NewTimer(p.pollInterval)
		select {
		case <-waitCtx.Done():
			timer.Stop()
			return errors.New("Queue/startup wait ended. The allocation may still exist; reconnect or cancel it explicitly.")
		case <-timer.C:
		}
	}
}

func sshConfigQuote(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "%", "%%").Replace(value) + `"`
}

func prepareRemoteSSH(s RemoteSession, hostKey string) error {
	fields := strings.Fields(hostKey)
	if len(fields) < 2 || fields[0] != "ssh-ed25519" {
		return errors.New("The container did not provide an Ed25519 host key.")
	}
	key, err := base64.StdEncoding.DecodeString(fields[1])
	if err != nil || len(key) != 51 || string(key[:15]) != "\x00\x00\x00\x0bssh-ed25519" || string(key[15:19]) != "\x00\x00\x00\x20" {
		return errors.New("The container host key is invalid.")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := remoteSessionDir(s.Token)
	alias := "vscode-sandboxes-" + s.Token
	known := filepath.Join(dir, "known_hosts")
	if err := privateWrite(known, []byte(alias+" "+fields[0]+" "+fields[1]+"\n")); err != nil {
		return err
	}
	binary, err := hostSSHBinary("ssh")
	if err != nil {
		return err
	}
	command := "exec bash " + remoteQuote(s.RunDir+"/relay.sh") + " " + remoteQuote(s.RunDir) + " " + remoteQuote(s.Token) + " " + remoteQuote(s.JobID) + " " + strconv.Itoa(s.Port)
	remoteCommand := "bash -c " + remoteQuote(command)
	quote := remoteQuote
	if runtime.GOOS == "windows" {
		quote = strconv.Quote
	}
	proxyOptions, err := loginSSHProxyOptions(s.Profile.Login, quote)
	if err != nil {
		return err
	}
	proxy := quote(filepath.ToSlash(binary)) + " " + proxyOptions + " " + s.Profile.Login + " " + quote(remoteCommand)
	// OpenSSH expands percent tokens even inside quoted ProxyCommand arguments.
	proxy = strings.ReplaceAll(proxy, "%", "%%")
	text := fmt.Sprintf("Host %s\n  HostName 127.0.0.1\n  Port %d\n  User %s\n  ProxyCommand %s\n  IdentityFile %s\n  IdentitiesOnly yes\n  IdentityAgent none\n  HostKeyAlias %s\n  UserKnownHostsFile %s\n  StrictHostKeyChecking yes\n  ForwardAgent no\n  ServerAliveInterval 30\n  ServerAliveCountMax 3\n", alias, s.Port, s.User, proxy, sshConfigQuote(filepath.ToSlash(filepath.Join(dir, "id_ed25519"))), alias, sshConfigQuote(filepath.ToSlash(known)))
	managed := filepath.Join(settingsDir(), "remote-ssh")
	if err := privateWrite(filepath.Join(managed, s.Token+".conf"), []byte(text)); err != nil {
		return err
	}
	config := filepath.Join(home, ".ssh", "config")
	original, err := os.ReadFile(config)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	line := "Include " + sshConfigQuote(filepath.ToSlash(filepath.Join(managed, "*.conf")))
	if !strings.Contains("\n"+string(original)+"\n", "\n"+line+"\n") {
		// Prepend at global scope; an Include appended below a Host block can
		// accidentally apply only to that block. Preserve every existing byte.
		updated := []byte("# VS Code Sandboxes remote allocations\n" + line + "\n\n" + string(original))
		target := config
		if resolved, err := filepath.EvalSymlinks(config); err == nil {
			target = resolved
		}
		current, readErr := os.ReadFile(config)
		if readErr != nil && !os.IsNotExist(readErr) {
			return readErr
		}
		if string(current) != string(original) {
			return errors.New("SSH config changed during startup; reconnect to retry.")
		}
		if err := privateWrite(target, updated); err != nil {
			return err
		}
	}
	return nil
}

func openRemoteEditor(ctx context.Context, alias string, log io.Writer, profile RemoteProfile, projectPath string) error {
	// Old allocations retain their /workspace mount until replaced.
	if projectPath == "" {
		projectPath = "/workspace"
	}
	if !safeRemotePath(projectPath, false) {
		return errors.New("Invalid remote workspace path.")
	}
	if err := EnsureRemoteEditorSettings(profile); err != nil {
		return err
	}
	commands := [][]string{{"--install-extension", "ms-vscode-remote.remote-ssh"}}
	commands = append(commands, []string{"--remote", "ssh-remote+" + alias, projectPath})
	for _, args := range commands {
		if err := ctx.Err(); err != nil {
			return err
		}
		cmd, err := editorCommand(args...)
		if err != nil {
			return err
		}
		child := exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)
		child.Env = cmd.Env
		hideWindow(child)
		child.Stdout, child.Stderr = log, log
		if err := child.Run(); err != nil {
			return fmt.Errorf("Open remote VS Code: %w", err)
		}
	}
	return nil
}

func (p *remoteController) stop() error {
	p.Lock()
	s := p.state.Session
	if p.state.Running && s.JobID == "" && (p.state.Phase == "Preparing remote image and project files" || p.state.Phase == "Uploading image archive and reusable project recipe" || p.state.Phase == "Uploading project template for cached server image") {
		if p.cancel != nil {
			p.cancel()
		}
		p.generation++
		p.state.Running = false
		p.state.Session.Active = false
		p.state.Phase = "Image preparation cancelled"
		s = p.state.Session
		p.Unlock()
		return p.saveSession(s)
	}
	if !s.Active || s.JobID == "" || !validRemoteSession(s) {
		p.Unlock()
		return errors.New("No confirmed allocation is available to cancel; reconnect first if submission was interrupted.")
	}
	if p.stopping {
		p.Unlock()
		return errors.New("Allocation cancellation is already running.")
	}
	p.stopping = true
	p.generation++
	if p.cancel != nil {
		p.cancel()
	}
	p.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	output, err := p.ssh(ctx, s.Profile.Login, remoteAsset("cancel.sh"), s.RunDir, s.Token, s.JobID)
	if err != nil {
		p.Lock()
		p.stopping = false
		p.state.Error = err.Error()
		p.state.Running = false
		p.Unlock()
		return err
	}
	fmt.Fprint(p, output)
	s.Active = false
	if err := p.saveSession(s); err != nil {
		return err
	}
	p.Lock()
	p.state.Running = false
	p.state.Phase = "Allocation " + s.JobID + " cancelled"
	p.state.Error = ""
	p.stopping = false
	p.Unlock()
	_ = os.Remove(filepath.Join(remoteSessionDir(s.Token), "id_ed25519"))
	_ = os.Remove(filepath.Join(settingsDir(), "remote-ssh", s.Token+".conf"))
	return nil
}

// Check active saved allocations sparingly, without querying the scheduler on
// every UI poll. Closing the launcher stops checks, never the Slurm allocation.
func (p *remoteController) refreshStatus() {
	p.Lock()
	if p.state.Running || p.checking || !p.state.Session.Active || p.state.Session.JobID == "" || time.Since(p.lastCheck) < 15*time.Second {
		p.Unlock()
		return
	}
	p.lastCheck = time.Now()
	p.checking = true
	s := p.state.Session
	p.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		ctx = context.WithValue(ctx, sshBackgroundProbe{}, true)
		result, err := p.ssh(ctx, s.Profile.Login, remoteAsset("probe.sh"), s.RunDir, s.Token, s.JobID)
		p.Lock()
		p.checking = false
		if p.state.Running || p.stopping || p.state.Session.Token != s.Token || !p.state.Session.Active {
			p.Unlock()
			return
		}
		if err != nil {
			p.state.Error = "Could not check allocation status; reconnect to retry. " + err.Error()
			p.Unlock()
			return
		}
		if parseRemoteValues(result)["STATE"] == "ENDED" {
			p.state.Session.Active = false
			p.state.Phase = "Allocation " + s.JobID + " ended"
			s = p.state.Session
			p.Unlock()
			_ = p.saveSession(s)
			return
		}
		p.Unlock()
	}()
}
func (p *remoteController) close() {
	p.Lock()
	defer p.Unlock()
	if p.cancel != nil {
		p.cancel()
	}
}
