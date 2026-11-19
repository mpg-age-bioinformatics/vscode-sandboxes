package startui

import (
	"bytes"
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

func Validate(s Selection) error {
	if !filepath.IsAbs(s.Project) {
		return errors.New("Choose an absolute project folder.")
	}
	if s.Agent != "codex" && s.Agent != "claude" {
		return errors.New("Choose Codex or Claude Code.")
	}
	if s.Network != "none" && s.Network != "restricted" && s.Network != "open" {
		return errors.New("Choose an internet setting.")
	}
	if s.Network == "restricted" {
		if _, err := sites(s.Sites); err != nil {
			return err
		}
	}
	if s.Endpoint != "default" && s.Endpoint != "private" {
		return errors.New("Choose an endpoint.")
	}
	if s.Endpoint == "private" {
		if s.Agent != "codex" {
			return errors.New("Private endpoints apply to Codex.")
		}
		if s.ConfigFile != "" {
			if !filepath.IsAbs(s.ConfigFile) {
				return errors.New("Use an absolute TOML file path.")
			}
		} else {
			u, err := url.Parse(s.BaseURL)
			if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
				return errors.New("Enter an HTTP or HTTPS endpoint URL without embedded credentials.")
			}
			if s.Model == "" {
				return errors.New("Enter the model ID served by your endpoint.")
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
func defaultCodexConfig() string {
	saved := filepath.Join(settingsDir(), "endpoints", "private.toml")
	if _, err := os.Stat(saved); err == nil {
		return saved
	}
	home := os.Getenv("CODEX_HOME")
	if home == "" {
		u, _ := os.UserHomeDir()
		home = filepath.Join(u, ".codex")
	}
	path := filepath.Join(home, "config.toml")
	if data, err := os.ReadFile(path); err == nil {
		var c map[string]any
		if toml.Unmarshal(data, &c) == nil {
			p, _ := c["model_provider"].(string)
			if p != "" && p != "openai" || c["openai_base_url"] != nil {
				return path
			}
		}
	}
	return ""
}

// endpointConfig imports the complete TOML document, preserving query parameters
// and custom headers. Environment credentials are resolved on the host and stored
// as user-only static settings so Remote-SSH and the Codex extension see them too.
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
	if model, _ := c["model"].(string); model == "" {
		return nil, errors.New("The TOML file must include a model ID.")
	}
	if p["auth"] != nil || p["gateway_oauth"] != nil {
		return nil, errors.New("This endpoint uses a host authentication helper or OAuth. Configure sandbox-compatible authentication in a separate TOML file first.")
	}
	if required, _ := p["requires_openai_auth"].(bool); required {
		return nil, errors.New("The private provider requires OpenAI authentication. Use a provider with independent endpoint authentication.")
	}
	if key, ok := p["env_key"].(string); ok && key != "" {
		value := s.Token
		if value == "" {
			value = os.Getenv(key)
		}
		if value == "" {
			return nil, fmt.Errorf("The endpoint needs %s. Enter the API key in the startup form, or export that variable before opening the app.", key)
		}
		delete(p, "env_key")
		p["experimental_bearer_token"] = value
	}
	if s.Token != "" {
		p["experimental_bearer_token"] = s.Token
	}
	if headers, ok := p["env_http_headers"].(map[string]any); ok {
		static, _ := p["http_headers"].(map[string]any)
		if static == nil {
			static = map[string]any{}
		}
		for header, key := range headers {
			env, ok := key.(string)
			if !ok {
				return nil, errors.New("Invalid env_http_headers.")
			}
			value := os.Getenv(env)
			if value == "" {
				return nil, fmt.Errorf("The endpoint header %s needs host environment variable %s.", header, env)
			}
			static[header] = value
		}
		p["http_headers"] = static
		delete(p, "env_http_headers")
	}
	// Import connection settings only. Host hooks, MCP processes, and machine
	// paths in a user's full Codex config are not portable to the sandbox.
	connection := map[string]any{"model": c["model"], "model_provider": provider, "model_providers": map[string]any{provider: p}}
	for _, key := range []string{"model_reasoning_effort", "model_reasoning_summary", "model_verbosity"} {
		if value, ok := c[key]; ok {
			connection[key] = value
		}
	}
	return toml.Marshal(connection)
}

// Command starts a child process without opening an additional console on Windows.
func Command(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	hideWindow(cmd)
	return cmd
}

// PrepareEnvironment creates a temporary, private launch manifest, outside the
// workspace. It is inherited by setup and runtime scripts, never shell-sourced.
func PrepareEnvironment(s Selection) ([]string, func(), error) {
	if err := Validate(s); err != nil {
		return nil, nil, err
	}
	if s.Endpoint == "private" {
		data, err := endpointConfig(s)
		if err != nil {
			return nil, nil, err
		}
		path := filepath.Join(settingsDir(), "endpoints", "private.toml")
		if err := privateWrite(path, data); err != nil {
			return nil, nil, err
		}
		s.ConfigFile = path
		s.Token = ""
	}
	dir, err := os.MkdirTemp("", "sandbox-startup-*")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { os.RemoveAll(dir) }
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
	return []string{"VSCODE_SANDBOX_STARTUP_HELPER=" + exe, "VSCODE_SANDBOX_STARTUP_SELECTION=" + path}, cleanup, nil
}

func command(log io.Writer, args ...string) error {
	cmd := exec.Command("sbx", args...)
	hideWindow(cmd)
	cmd.Stdout = log
	cmd.Stderr = log
	return cmd.Run()
}
func output(args ...string) (string, error) {
	cmd := exec.Command("sbx", args...)
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
	if len(os.Args) != 4 || (os.Args[1] != "--startup-apply" && os.Args[1] != "--startup-guard") {
		return false
	}
	var s Selection
	if err := readJSON(os.Getenv("VSCODE_SANDBOX_STARTUP_SELECTION"), &s); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var err error
	if os.Args[1] == "--startup-guard" {
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
	if agent == "codex" {
		var data []byte
		if s.Endpoint == "private" {
			var err error
			data, err = os.ReadFile(s.ConfigFile)
			if err != nil {
				return err
			}
		}
		// Back up the sandbox's original config once, and restore it on default mode.
		script := `set -eu
+mkdir -p "$HOME/.codex"
+cd "$HOME/.codex"
+if [ "$1" = private ]; then
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
		cmd := exec.Command("sbx", "exec", "-i", name, "sh", "-c", script, "startup", s.Endpoint)
		hideWindow(cmd)
		cmd.Stdin = bytes.NewReader(data)
		cmd.Stdout = log
		cmd.Stderr = log
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("Install Codex connection settings: %w", err)
		}
		fmt.Fprintln(log, "Codex connection:", s.Endpoint)
	}
	return applyNetwork(name, s, log)
}
