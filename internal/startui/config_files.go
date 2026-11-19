package startui

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type lastConfig struct {
	Path string `json:"path"`
}

func existingConfig(path string) bool {
	info, err := os.Stat(path)
	return filepath.IsAbs(path) && err == nil && info.Mode().IsRegular()
}

func lastUsedConfig() string {
	var last lastConfig
	if readJSON(filepath.Join(settingsDir(), "last-config.json"), &last) == nil && automaticConfigAllowed(last.Path) {
		return last.Path
	}
	return ""
}

// Host defaults are never selected automatically, including paths remembered
// by older versions when discovery still ran, and symlink aliases of those files.
func automaticConfigAllowed(path string) bool {
	if path == "" {
		return false
	}
	same := func(a, b string) bool {
		a, b = resolveExistingParent(a), resolveExistingParent(b)
		if runtime.GOOS == "windows" {
			return strings.EqualFold(a, b)
		}
		return a == b
	}
	home, _ := os.UserHomeDir()
	if same(path, filepath.Join(home, ".codex", "config.toml")) {
		return false
	}
	if codexHome := os.Getenv("CODEX_HOME"); codexHome != "" && same(path, filepath.Join(codexHome, "config.toml")) {
		return false
	}
	return existingConfig(path)
}

func automaticConfigFile(saved string) string {
	if last := lastUsedConfig(); last != "" {
		return last
	}
	if automaticConfigAllowed(saved) {
		return saved
	}
	return ""
}

func rememberConfig(path string) error {
	if !existingConfig(path) {
		return errors.New("Choose an existing TOML file.")
	}
	return writeJSON(filepath.Join(settingsDir(), "last-config.json"), lastConfig{path})
}

// Create a settings-only TOML with exclusive creation: never overwrite a file.
func createConfigFile(path string, selection Selection) error {
	if !filepath.IsAbs(path) || !strings.EqualFold(filepath.Ext(path), ".toml") {
		return errors.New("Choose an absolute filename ending in .toml.")
	}
	if configInProject(selection.Project, path) {
		return errors.New("Save source TOML files outside the shared project folder so agents cannot read the original file.")
	}
	selection.ConfigFile, selection.Token = "", ""
	var data []byte
	var err error
	if selection.Endpoint == "private" {
		if selection.Agent == "" {
			selection.Agent = "both"
		}
		if selection.Project == "" {
			selection.Project = filepath.Dir(path)
		}
		if selection.Network == "" {
			selection.Network = "open"
		}
		if err = validateSettings(selection); err != nil {
			return err
		}
		data, err = endpointConfig(selection)
	} else if selection.Endpoint == "default" {
		settings := map[string]any{}
		if selection.Model != "" {
			settings["model"] = selection.Model
		}
		data, err = toml.Marshal(settings)
	} else {
		return errors.New("Choose a Codex connection type.")
	}
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("Create TOML file (existing files are preserved): %w", err)
	}
	complete := false
	defer func() {
		file.Close()
		if !complete {
			os.Remove(path)
		}
	}()
	if _, err = file.Write(append([]byte("# Connection settings only. Authenticate separately inside the sandbox.\n"), data...)); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	complete = true
	return rememberConfig(path)
}

func configInProject(project, path string) bool {
	if project == "" || path == "" {
		return false
	}
	within := func(root, file string) bool {
		if runtime.GOOS == "windows" {
			root, file = strings.ToLower(root), strings.ToLower(file)
		}
		relative, err := filepath.Rel(root, file)
		return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
	}
	return within(filepath.Clean(project), filepath.Clean(path)) || within(resolveExistingParent(project), resolveExistingParent(path))
}

func resolveExistingParent(path string) string {
	candidate := filepath.Clean(path)
	var tail []string
	for {
		if resolved, err := filepath.EvalSymlinks(candidate); err == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return resolved
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return filepath.Clean(path)
		}
		tail = append(tail, filepath.Base(candidate))
		candidate = parent
	}
}

// Merge explicit form edits without dropping unrelated host settings or changing
// the original file until the user chooses Save changes.
func applyConnectionEdits(config map[string]any, selection Selection) {
	if selection.Model != "" {
		config["model"] = selection.Model
	} else {
		delete(config, "model")
	}
	delete(config, "openai_base_url")
	if selection.Endpoint == "default" {
		config["model_provider"] = "openai"
		return
	}
	provider, _ := config["model_provider"].(string)
	if provider == "" || provider == "openai" {
		provider = "sandbox_private"
	}
	providers, _ := config["model_providers"].(map[string]any)
	if providers == nil {
		providers = map[string]any{}
		config["model_providers"] = providers
	}
	settings, _ := providers[provider].(map[string]any)
	if settings == nil {
		settings = map[string]any{"name": "Private endpoint", "wire_api": "responses"}
		providers[provider] = settings
	}
	settings["base_url"] = selection.BaseURL
	config["model_provider"] = provider
}

func updateConfigFile(path, revision string, selection Selection) error {
	if !existingConfig(path) {
		return errors.New("Choose an existing TOML file first.")
	}
	if configInProject(selection.Project, path) {
		return errors.New("Keep source TOMLs outside the shared project folder.")
	}
	selection.ConfigFile, selection.Token = "", ""
	if selection.Agent == "" {
		selection.Agent = "both"
	}
	if selection.Network == "" {
		selection.Network = "open"
	}
	if err := validateSettings(selection); err != nil {
		return err
	}
	original, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(original) > 1<<20 {
		return errors.New("The TOML file exceeds 1 MiB.")
	}
	if revision == "" || fmt.Sprintf("%x", sha256.Sum256(original)) != revision {
		return errors.New("The TOML changed since it was loaded. Browse to it again before saving.")
	}
	var config map[string]any
	if err = toml.Unmarshal(original, &config); err != nil {
		return errors.New("The selected file is not valid TOML.")
	}
	if config == nil {
		config = map[string]any{}
	}
	applyConnectionEdits(config, selection)
	data, err := toml.Marshal(config)
	if err != nil {
		return err
	}
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(target), ".sandbox-config-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	// Rename without removing the original first; a failure leaves it intact.
	if err = os.Rename(file.Name(), target); err != nil {
		return fmt.Errorf("Save configuration: %w", err)
	}
	return rememberConfig(path)
}
