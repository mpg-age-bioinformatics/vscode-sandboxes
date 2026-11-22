package startui

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// The startup card accepts only a URL and a separate key. Legacy saved file/model
// fields cannot override the displayed inputs.
func directConnection(s Selection) Selection {
	s.ConfigFile, s.Model = "", ""
	s.ConfigOverrides = false
	s.BaseURL, s.Token = strings.TrimSpace(s.BaseURL), strings.TrimSpace(s.Token)
	if s.PrivateAgent == "" {
		s.PrivateAgent = "continue"
	}
	s.Endpoint = "default"
	if s.BaseURL != "" || s.Token != "" {
		s.Endpoint = "private"
	}
	return s
}

func Validate(s Selection) error {
	if err := validateSettings(s); err != nil {
		return err
	}
	if s.Endpoint == "private" && strings.TrimSpace(s.Token) == "" {
		return errors.New("Enter an API key for the private endpoint.")
	}
	return nil
}

// Connection files contain settings only and can be saved without a startup key.
func validateSettings(s Selection) error {
	if !filepath.IsAbs(s.Project) {
		return errors.New("Choose an absolute project folder.")
	}
	if s.Agent != "both" && s.Agent != "codex" && s.Agent != "claude" {
		return errors.New("Invalid agent configuration.")
	}
	if s.PrivateAgent != "" && s.PrivateAgent != "continue" && s.PrivateAgent != "codex" {
		return errors.New("Choose Codex or Continue for the private endpoint.")
	}
	if s.Network != "none" && s.Network != "restricted" && s.Network != "open" {
		return errors.New("Choose an internet setting.")
	}
	if s.Endpoint == "private" && s.Network == "none" {
		return errors.New(privateOfflineMessage)
	}
	if s.Network == "restricted" {
		if _, err := sites(s.Sites); err != nil {
			return err
		}
	}
	if s.Endpoint != "default" && s.Endpoint != "private" {
		return errors.New("Choose an endpoint.")
	}
	if s.ConfigFile != "" && !filepath.IsAbs(s.ConfigFile) {
		return errors.New("Use an absolute TOML file path.")
	}
	if s.ConfigFile != "" && configInProject(s.Project, s.ConfigFile) {
		return errors.New("Keep the source TOML outside the shared project folder so agents cannot access its original authentication.")
	}
	if s.Endpoint == "private" {
		if s.ConfigFile != "" {
			if !filepath.IsAbs(s.ConfigFile) {
				return errors.New("Use an absolute TOML file path.")
			}
		} else {
			u, err := url.Parse(s.BaseURL)
			if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
				return errors.New("Enter an HTTP or HTTPS endpoint URL without embedded credentials.")
			}
		}
	}
	return nil
}
func sites(input string) ([]string, error) {
	items := strings.FieldsFunc(strings.ToLower(input), func(r rune) bool { return r == ',' || r == '\n' || r == '\r' || r == ' ' || r == '\t' })
	if len(items) == 0 {
		return nil, errors.New("Enter at least one allowed host.")
	}
	seen := map[string]bool{}
	result := []string{}
	for _, host := range items {
		if !regexp.MustCompile(`^(\*\*?\.)?[a-z0-9][a-z0-9.-]*(:[0-9]{1,5})?$`).MatchString(host) || strings.Contains(host, "..") {
			return nil, fmt.Errorf("Invalid host %q. Use hostnames, optional *. subdomains, and optional ports; omit https:// and URL paths.", host)
		}
		if !seen[host] {
			seen[host] = true
			result = append(result, host)
		}
	}
	return result, nil
}

type configPreview struct {
	Model    string `json:"model"`
	BaseURL  string `json:"baseURL"`
	Private  bool   `json:"private"`
	Revision string `json:"revision"`
}

func previewCodexConfig(path string) (configPreview, error) {
	var preview configPreview
	if !filepath.IsAbs(path) {
		return preview, errors.New("Use an absolute TOML file path.")
	}
	file, err := os.Open(path)
	if err != nil {
		return preview, fmt.Errorf("Read Codex TOML file: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil {
		return preview, err
	}
	if len(data) > 1<<20 {
		return preview, errors.New("The Codex TOML file exceeds 1 MiB.")
	}
	var config map[string]any
	if err := toml.Unmarshal(data, &config); err != nil {
		return preview, errors.New("The selected file is not valid TOML.")
	}
	preview.Revision = fmt.Sprintf("%x", sha256.Sum256(data))
	preview.Model, _ = config["model"].(string)
	provider, _ := config["model_provider"].(string)
	preview.BaseURL, _ = config["openai_base_url"].(string)
	if provider != "" && provider != "openai" {
		preview.Private = true
		providers, _ := config["model_providers"].(map[string]any)
		selected, _ := providers[provider].(map[string]any)
		preview.BaseURL, _ = selected["base_url"].(string)
	}
	preview.Private = preview.Private || preview.BaseURL != ""
	if preview.BaseURL != "" {
		u, err := url.Parse(preview.BaseURL)
		if err != nil || u.User != nil {
			return configPreview{}, errors.New("Use an endpoint URL without embedded credentials.")
		}
		preview.BaseURL = cleanConnectionURL(u)
	}
	return preview, nil
}

// endpointConfig imports portable connection settings, never source authentication.
// Only a separate key explicitly entered for this startup may be added at runtime.
func endpointConfig(s Selection) ([]byte, error) {
	var c map[string]any
	if s.ConfigFile != "" {
		data, err := os.ReadFile(s.ConfigFile)
		if err != nil {
			return nil, fmt.Errorf("Read endpoint file: %w", err)
		}
		if err := toml.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("Invalid endpoint TOML: %w", err)
		}
	} else {
		c = map[string]any{"model": s.Model, "model_provider": "sandbox_private", "model_providers": map[string]any{"sandbox_private": map[string]any{"name": "Private endpoint", "base_url": s.BaseURL, "wire_api": "responses"}}}
		if s.Token != "" {
			c["model_providers"].(map[string]any)["sandbox_private"].(map[string]any)["experimental_bearer_token"] = s.Token
		}
	}
	if s.Endpoint == "default" {
		// The endpoint selector chooses the built-in OpenAI connection even when
		// this same source file selects a private provider. Never copy its URL,
		// authentication, or private model/routing alias into default mode.
		connection := map[string]any{}
		provider, _ := c["model_provider"].(string)
		base, _ := c["openai_base_url"].(string)
		if (provider == "" || provider == "openai") && base == "" {
			if model, ok := c["model"]; ok {
				connection["model"] = model
			}
		}
		for _, key := range []string{"model_reasoning_effort", "model_reasoning_summary", "model_verbosity"} {
			if value, ok := c[key]; ok {
				connection[key] = value
			}
		}
		return toml.Marshal(connection)
	}
	if s.ConfigFile != "" && s.ConfigOverrides {
		if c == nil {
			c = map[string]any{}
		}
		applyConnectionEdits(c, s)
	}
	provider, _ := c["model_provider"].(string)
	if provider == "" || provider == "openai" {
		// Map built-in OpenAI URL overrides into an independent private provider.
		base, _ := c["openai_base_url"].(string)
		if base == "" {
			return nil, errors.New("The TOML file must select a custom model_provider or set openai_base_url.")
		}
		provider = "sandbox_private"
		c["model_provider"] = provider
		providers, _ := c["model_providers"].(map[string]any)
		if providers == nil {
			providers = map[string]any{}
			c["model_providers"] = providers
		}
		providers[provider] = map[string]any{"name": "Private endpoint", "base_url": base, "wire_api": "responses", "env_key": "OPENAI_API_KEY"}
	}
	providers, _ := c["model_providers"].(map[string]any)
	p, _ := providers[provider].(map[string]any)
	if p == nil {
		return nil, errors.New("The selected provider is missing from model_providers.")
	}
	base, _ := p["base_url"].(string)
	u, err := url.Parse(base)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return nil, errors.New("The selected provider needs an HTTP(S) base_url without embedded credentials.")
	}
	// Rebuild the provider from a connection-only allowlist. Do not copy bearer
	// tokens, headers, OAuth/helpers, or values from the host environment.
	clean := map[string]any{"base_url": cleanConnectionURL(u)}
	for _, key := range []string{"name", "wire_api", "requires_openai_auth"} {
		if value, ok := p[key]; ok {
			clean[key] = value
		}
	}
	if key, ok := p["env_key"].(string); ok && regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(key) {
		// This is a variable name only; it must be set separately in the sandbox.
		clean["env_key"] = key
	}
	if query, ok := p["query_params"].(map[string]any); ok {
		if version, ok := query["api-version"].(string); ok && apiVersion.MatchString(version) {
			clean["query_params"] = map[string]any{"api-version": version}
		}
	}
	if s.Token != "" {
		clean["requires_openai_auth"] = false
		delete(clean, "env_key")
		clean["experimental_bearer_token"] = s.Token
	}
	p = clean
	// Import connection settings only. Host hooks, MCP processes, and machine
	// paths in a user's full Codex config are not portable to the sandbox.
	connection := map[string]any{"model_provider": provider, "model_providers": map[string]any{provider: p}}
	if model, ok := c["model"].(string); ok && strings.TrimSpace(model) != "" {
		connection["model"] = model
	}
	for _, key := range []string{"model_reasoning_effort", "model_reasoning_summary", "model_verbosity"} {
		if value, ok := c[key]; ok {
			connection[key] = value
		}
	}
	return toml.Marshal(connection)
}

// Command starts a child process without opening an additional console on Windows.
func Command(name string, args ...string) *exec.Cmd {
	if name == "sbx" || name == "sbx.exe" {
		if path, err := SandboxCLI(); err == nil {
			name = path
		}
	}
	cmd := exec.Command(name, args...)
	hideWindow(cmd)
	return cmd
}

// PrepareEnvironment creates a temporary, private launch manifest, outside the
// workspace. It is inherited by setup and runtime scripts, never shell-sourced.
func PrepareEnvironment(s Selection) ([]string, func(), error) {
	if s.Endpoint == "default" {
		s.Token = ""
	}
	var networkErr error
	s, _, networkErr = withPrivateEndpointAccess(s)
	if networkErr != nil {
		return nil, nil, networkErr
	}
	if err := Validate(s); err != nil {
		return nil, nil, err
	}
	if s.Endpoint == "private" {
		if err := EnsureContinueWorkspace(); err != nil {
			return nil, nil, err
		}
	}
	hasConnection := s.ConfigFile != ""
	var connectionData []byte
	if hasConnection {
		connectionOnly := s
		connectionOnly.Token = ""
		data, err := endpointConfig(connectionOnly)
		if err != nil {
			return nil, nil, err
		}
		name := "default.toml"
		if s.Endpoint == "private" {
			name = "private.toml"
		}
		path := filepath.Join(settingsDir(), "endpoints", name)
		if err := privateWrite(path, data); err != nil {
			return nil, nil, err
		}
		connectionData = data
	}
	// Repository inspection happens after the workspace is mounted in sbx.
	s.FreshRepository, s.PreservePaths = false, nil
	dir, err := os.MkdirTemp("", "sandbox-startup-*")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { os.RemoveAll(dir) }
	if hasConnection {
		// Concurrent launches must not read each other's cached endpoint.
		s.ConfigFile = filepath.Join(dir, "connection.toml")
		if err := privateWrite(s.ConfigFile, connectionData); err != nil {
			cleanup()
			return nil, nil, err
		}
	}
	path := filepath.Join(dir, "selection.json")
	if err := writeJSON(path, s); err != nil {
		cleanup()
		return nil, nil, err
	}
	exe, err := os.Executable()
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	env := []string{"VSCODE_SANDBOX_STARTUP_HELPER=" + exe, "VSCODE_SANDBOX_STARTUP_SELECTION=" + path}
	if old := ExistingProject(s.Project); old != nil && old.Ready {
		env = append(env, "VSCODE_SANDBOX_RESUME=1")
		if old.SandboxName != "" {
			env = append(env, "VSCODE_SANDBOX_REUSE_NAME="+old.SandboxName)
		}
	}
	return env, cleanup, nil
}

func command(log io.Writer, args ...string) error {
	cmd := Command("sbx", args...)
	hideWindow(cmd)
	cmd.Stdout = log
	cmd.Stderr = log
	return cmd.Run()
}
func output(args ...string) (string, error) {
	cmd := Command("sbx", args...)
	hideWindow(cmd)
	data, err := cmd.CombinedOutput()
	return string(data), err
}

func EnsureHost(log io.Writer) error {
	if runtime.GOOS == "darwin" {
		status, err := output("daemon", "status", "--json")
		if err != nil || !regexp.MustCompile(`"status"\s*:\s*"running"`).MatchString(status) {
			fmt.Fprintln(log, "Starting Docker Sandboxes…")
			if err := command(log, "daemon", "start", "--detach"); err != nil {
				return err
			}
		}
	}
	diagnostic, err := output("diagnose")
	fmt.Fprint(log, diagnostic)
	if err != nil && regexp.MustCompile(`(?i)sbx login|not authenticated|not logged|sign.in|unauthorized|authentication`).MatchString(diagnostic) {
		fmt.Fprintln(log, "Docker sign-in is required. Complete authentication in your browser.")
		if err := command(log, "login"); err != nil {
			return fmt.Errorf("Docker sign-in failed: %w", err)
		}
		diagnostic, err = output("diagnose")
		fmt.Fprint(log, diagnostic)
	}
	if err != nil {
		return fmt.Errorf("Docker Sandboxes diagnostics failed. See startup activity for details: %w", err)
	}
	return nil
}

// HandleHelper is used by generated shell launchers, with networking still
// guarded by a per-sandbox deny-all rule while credentials are installed.
func HandleHelper() bool {
	if os.Getenv("VSCODE_SSH_ASKPASS_URL") != "" {
		if err := runSSHAskpass(os.Args[len(os.Args)-1], os.Stdout); err != nil {
			os.Exit(1)
		}
		return true
	}
	if len(os.Args) != 4 || (os.Args[1] != "--startup-apply" && os.Args[1] != "--startup-guard" && os.Args[1] != "--startup-ready") {
		return false
	}
	var s Selection
	if err := readJSON(os.Getenv("VSCODE_SANDBOX_STARTUP_SELECTION"), &s); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var err error
	if os.Args[1] == "--startup-ready" {
		err = finalizeProject(s, os.Args[2], os.Stdout)
	} else if os.Args[1] == "--startup-guard" {
		err = guardNetwork(os.Args[2], os.Stdout)
	} else {
		err = applyStartup(os.Args[2], os.Args[3], s, os.Stdout)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return true
}
func applyStartup(name, agent string, s Selection, log io.Writer) error {
	if s.PrivateAgent != "" && s.PrivateAgent != "continue" && s.PrivateAgent != "codex" {
		return errors.New("Choose Codex or Continue for the private endpoint.")
	}
	var networkErr error
	var endpointHost string
	s, endpointHost, networkErr = withPrivateEndpointAccess(s)
	if networkErr != nil {
		return networkErr
	}
	if endpointHost != "" {
		fmt.Fprintln(log, privateEndpointNotice(endpointHost))
	}
	var codexPrivateData []byte
	var codexPrivateModel string
	if s.Endpoint == "private" {
		data, err := privateContinueConfig(context.Background(), s)
		if err != nil {
			return err
		}
		if s.PrivateAgent == "codex" {
			codexPrivateData, codexPrivateModel, err = privateCodexConfig(s, data)
			if err != nil {
				return err
			}
		}
		script := `set -eu
umask 077
mkdir -p "$HOME/.continue"
cd "$HOME/.continue"
if [ ! -e .sandbox-managed ]; then
  if [ -e config.yaml ]; then cp -p config.yaml .sandbox-original-config.yaml; fi
  touch .sandbox-managed
fi
cat > .sandbox-config.tmp
mv .sandbox-config.tmp config.yaml`
		cmd := Command("sbx", "exec", "-i", name, "sh", "-c", script)
		hideWindow(cmd)
		cmd.Stdin = bytes.NewReader(data)
		cmd.Stdout, cmd.Stderr = log, log
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("Install Continue connection settings: %w", err)
		}
		var configured struct {
			Models []json.RawMessage `json:"models"`
		}
		_ = json.Unmarshal(data, &configured)
		fmt.Fprintf(log, "Continue configured with %d private models inside this sandbox.\n", len(configured.Models))
	} else {
		script := `set -eu
if [ -e "$HOME/.continue/.sandbox-managed" ]; then
  cd "$HOME/.continue"
  if [ -e .sandbox-original-config.yaml ]; then mv .sandbox-original-config.yaml config.yaml; else rm -f config.yaml; fi
  rm -f .sandbox-managed
fi`
		cmd := Command("sbx", "exec", name, "sh", "-c", script)
		hideWindow(cmd)
		cmd.Stdout, cmd.Stderr = log, log
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("Restore Continue connection settings: %w", err)
		}
	}
	if agent == "codex" || agent == "both" {
		data := codexPrivateData
		if len(data) == 0 && s.ConfigFile != "" {
			codexSelection := s
			codexSelection.Endpoint = "default"
			codexSelection.Token = ""
			var err error
			data, err = endpointConfig(codexSelection)
			if err != nil {
				return err
			}
		}
		// Back up the sandbox's original config once, and restore it on default mode.
		script := `set -eu
+umask 077
+mkdir -p "$HOME/.codex"
+chmod 700 "$HOME/.codex"
+cd "$HOME/.codex"
+rm -f .sandbox-models.json
+if [ "$1" = import ]; then
+  if [ ! -e .sandbox-managed ]; then
+    if [ -e config.toml ]; then cp -p config.toml .sandbox-original.toml; fi
+    touch .sandbox-managed
+  fi
+  umask 077
+  cat > .sandbox-config.tmp
+  mv .sandbox-config.tmp config.toml
+else
+  if [ -e .sandbox-managed ]; then
+    if [ -e .sandbox-original.toml ]; then mv .sandbox-original.toml config.toml; else rm -f config.toml; fi
+    rm -f .sandbox-managed
+  fi
+fi`
		script = strings.ReplaceAll(script, "\n+", "\n")
		mode := "default"
		if len(codexPrivateData) > 0 || s.ConfigFile != "" {
			mode = "import"
		}
		cmd := Command("sbx", "exec", "-i", name, "sh", "-c", script, "startup", mode, s.Endpoint)
		hideWindow(cmd)
		cmd.Stdin = bytes.NewReader(data)
		cmd.Stdout = log
		cmd.Stderr = log
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("Install Codex connection settings: %w", err)
		}
		if len(codexPrivateData) > 0 {
			fmt.Fprintf(log, "Codex connection: private endpoint (Responses API), model %s.\n", codexPrivateModel)
		} else {
			fmt.Fprintln(log, "Codex connection: default OpenAI")
		}
	}
	return applyNetwork(name, s, log)
}

var apiVersion = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}(-preview)?$`)

func cleanConnectionURL(source *url.URL) string {
	clean := *source
	query := url.Values{}
	if version := clean.Query().Get("api-version"); apiVersion.MatchString(version) {
		query.Set("api-version", version)
	}
	clean.RawQuery, clean.Fragment, clean.RawFragment = query.Encode(), "", ""
	return clean.String()
}
