package startui

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestHostDefaultsAreNeverAutomaticallySelected(t *testing.T) {
	isolatedSettings(t)
	codexHome := filepath.Join(t.TempDir(), "Codex settings with spaces")
	t.Setenv("CODEX_HOME", codexHome)
	home, _ := os.UserHomeDir()
	for _, folder := range []string{codexHome, filepath.Join(home, ".codex")} {
		if err := os.MkdirAll(folder, 0700); err != nil {
			t.Fatal(err)
		}
		source := filepath.Join(folder, "config.toml")
		if err := os.WriteFile(source, []byte("model='host-model'\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if got := automaticConfigFile(""); got != "" {
			t.Fatalf("discovered host config: %q", got)
		}
		// Older versions could save the automatically discovered host file.
		if err := rememberConfig(source); err != nil {
			t.Fatal(err)
		}
		if got := automaticConfigFile(source); got != "" {
			t.Fatalf("restored legacy host config: %q", got)
		}
		alias := filepath.Join(t.TempDir(), "host-default-alias.toml")
		if err := os.Symlink(source, alias); err == nil {
			if got := automaticConfigFile(alias); got != "" {
				t.Fatalf("selected a host-default symlink: %q", got)
			}
		}
	}
	custom := filepath.Join(t.TempDir(), "sandbox.toml")
	os.WriteFile(custom, []byte("model='sandbox-model'\n"), 0600)
	if err := rememberConfig(custom); err != nil {
		t.Fatal(err)
	}
	if got := automaticConfigFile(filepath.Join(codexHome, "config.toml")); got != custom {
		t.Fatalf("lost chosen sandbox file: %q", got)
	}
}

func TestConfigPreviewDoesNotRequireOrExposeCredentials(t *testing.T) {
	source := filepath.Join(t.TempDir(), "private.toml")
	input := "model='private-model'\nmodel_provider='company'\n[model_providers.company]\nbase_url='https://models.example.org/v1'\nenv_key='UNSET_TEST_CREDENTIAL'\nexperimental_bearer_token='secret-test-token'\n"
	os.WriteFile(source, []byte(input), 0600)
	preview, err := previewCodexConfig(source)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.Private || preview.Model != "private-model" || preview.BaseURL != "https://models.example.org/v1" {
		t.Fatal(preview)
	}
	data, _ := json.Marshal(preview)
	if bytes.Contains(data, []byte("secret-test-token")) || bytes.Contains(data, []byte("UNSET_TEST_CREDENTIAL")) {
		t.Fatal("preview exposed credentials")
	}
	os.WriteFile(source, []byte("invalid = ["), 0600)
	if _, err := previewCodexConfig(source); err == nil {
		t.Fatal("accepted invalid TOML")
	}
}

func TestDefaultConfigMapsOnlyPortableSettings(t *testing.T) {
	isolatedSettings(t)
	source := filepath.Join(t.TempDir(), "config.toml")
	input := "model='test-model'\nmodel_reasoning_effort='high'\nnotify=['/host/script']\n[mcp_servers.host]\ncommand='/host/only'\n"
	os.WriteFile(source, []byte(input), 0600)
	s := selection()
	s.ConfigFile = source
	preview, err := previewCodexConfig(source)
	if err != nil || preview.Private || preview.Model != "test-model" {
		t.Fatal(preview, err)
	}
	env, cleanup, err := PrepareEnvironment(s)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if len(env) != 2 {
		t.Fatal("missing runtime manifest")
	}
	data, err := os.ReadFile(filepath.Join(settingsDir(), "endpoints", "default.toml"))
	if err != nil {
		t.Fatal(err)
	}
	var mapped map[string]any
	if err := toml.Unmarshal(data, &mapped); err != nil {
		t.Fatal(err)
	}
	if mapped["model"] != "test-model" || mapped["model_reasoning_effort"] != "high" || len(mapped) != 2 {
		t.Fatal("imported non-portable host settings", mapped)
	}
	original, _ := os.ReadFile(source)
	if string(original) != input {
		t.Fatal("modified source file")
	}
}

func TestSameTOMLSupportsPrivateAndDefaultConnections(t *testing.T) {
	for _, input := range []string{
		"model='private-alias'\nmodel_provider='company'\n[model_providers.company]\nbase_url='https://private.example/v1'\nexperimental_bearer_token='source-secret'\n",
		"model='private-alias'\nopenai_base_url='https://private.example/v1'\n",
	} {
		t.Run(input, func(t *testing.T) {
			isolatedSettings(t)
			source := filepath.Join(t.TempDir(), "connections.toml")
			if err := os.WriteFile(source, []byte(input), 0600); err != nil {
				t.Fatal(err)
			}
			s := selection()
			s.ConfigFile, s.Token = source, "sandbox-secret"
			for _, overrides := range []bool{false, true} {
				s.ConfigOverrides = overrides
				s.BaseURL, s.Model = "https://draft.example/v1", "draft-alias"
				data, err := endpointConfig(s)
				if err != nil {
					t.Fatal(err)
				}
				var config map[string]any
				if err := toml.Unmarshal(data, &config); err != nil {
					t.Fatal(err)
				}
				if len(config) != 0 {
					t.Fatalf("private settings leaked into default OpenAI: %s", data)
				}
			}
			env, cleanup, err := PrepareEnvironment(s)
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()
			manifest, err := os.ReadFile(strings.TrimPrefix(env[1], "VSCODE_SANDBOX_STARTUP_SELECTION="))
			if err != nil || bytes.Contains(manifest, []byte(s.Token)) {
				t.Fatal("default startup retained the private API key", err)
			}
			s.Endpoint, s.ConfigOverrides = "private", false
			data, err := endpointConfig(s)
			if err != nil || !bytes.Contains(data, []byte("private-alias")) || !bytes.Contains(data, []byte("https://private.example/v1")) || !bytes.Contains(data, []byte("sandbox-secret")) {
				t.Fatalf("switching back lost the private connection: %s, %v", data, err)
			}
			original, err := os.ReadFile(source)
			if err != nil || string(original) != input {
				t.Fatal("switching changed the source TOML", err)
			}
		})
	}
}

func TestPrivateModelCanBeChosenLater(t *testing.T) {
	for _, mode := range []string{"manual", "toml-without-model", "clear-toml-model"} {
		t.Run(mode, func(t *testing.T) {
			isolatedSettings(t)
			s := selection()
			s.Endpoint, s.BaseURL, s.Token = "private", "https://private.example/v1", "sandbox-key"
			if mode != "manual" {
				s.ConfigFile = filepath.Join(t.TempDir(), "connection.toml")
				input := "model_provider='company'\n[model_providers.company]\nbase_url='https://private.example/v1'\n"
				if mode == "clear-toml-model" {
					input = "model='old-alias'\n" + input
					s.ConfigOverrides = true
				}
				if err := os.WriteFile(s.ConfigFile, []byte(input), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "manual" {
				path := filepath.Join(t.TempDir(), "saved.toml")
				if err := createConfigFile(path, s); err != nil {
					t.Fatal("could not save a private connection without a model:", err)
				}
				preview, err := previewCodexConfig(path)
				if err != nil || !preview.Private || preview.Model != "" {
					t.Fatal("model-free connection did not round-trip:", preview, err)
				}
			}
			if err := Validate(s); err != nil {
				t.Fatal(err)
			}
			data, err := endpointConfig(s)
			if err != nil {
				t.Fatal(err)
			}
			var config map[string]any
			if err := toml.Unmarshal(data, &config); err != nil {
				t.Fatal(err)
			}
			if _, exists := config["model"]; exists {
				t.Fatalf("unset model must be omitted, not empty: %s", data)
			}
			if config["model_provider"] == nil || !bytes.Contains(data, []byte(s.BaseURL)) {
				t.Fatal("omitting the model removed the private connection")
			}
			_, cleanup, err := PrepareEnvironment(s)
			if err != nil {
				t.Fatal(err)
			}
			cleanup()
		})
	}
}

func TestImportedDefaultConfigIsInstalledAndCanBeCleared(t *testing.T) {
	mockSBX(t)
	// Let only the fixture's exec call run its supplied shell in a temporary HOME.
	bin := strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))[0]
	scriptPath := filepath.Join(bin, "sbx")
	script, _ := os.ReadFile(scriptPath)
	script = bytes.Replace(script, []byte("case \"$1 ${2:-}\" in"), []byte("if [[ \"$1 ${2:-}\" == 'exec -i' ]]; then shift 3; HOME=\"$TEST_SANDBOX_HOME\" \"$@\"; exit $?; fi\ncase \"$1 ${2:-}\" in"), 1)
	os.WriteFile(scriptPath, script, 0755)
	sandboxHome := t.TempDir()
	t.Setenv("TEST_SANDBOX_HOME", sandboxHome)
	os.MkdirAll(filepath.Join(sandboxHome, ".codex"), 0700)
	target := filepath.Join(sandboxHome, ".codex", "config.toml")
	os.WriteFile(target, []byte("model='original-model'\n"), 0600)
	source := filepath.Join(t.TempDir(), "mapped.toml")
	os.WriteFile(source, []byte("model='mapped-model'\n"), 0600)
	s := selection()
	s.Network, s.ConfigFile = "none", source
	if err := applyStartup("test-sandbox", "both", s, io.Discard); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(target)
	if !bytes.Contains(got, []byte("mapped-model")) {
		t.Fatal("did not install mapped config")
	}
	s.ConfigFile = ""
	if err := applyStartup("test-sandbox", "both", s, io.Discard); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(target)
	if !bytes.Contains(got, []byte("original-model")) {
		t.Fatal("did not restore prior config")
	}
}
