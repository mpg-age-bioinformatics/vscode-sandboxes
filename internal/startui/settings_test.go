package startui

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func isolatedSettings(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("APPDATA", filepath.Join(root, "config"))
}
func selection() Selection {
	return Selection{Project: filepath.Join(os.TempDir(), "sandbox-project"), Version: "3.13", Agent: "codex", Network: "open", Endpoint: "default"}
}

func TestEndpointRoundTripAndCredentialMapping(t *testing.T) {
	isolatedSettings(t)
	t.Setenv("PRIVATE_TOKEN", "a-secret-key")
	source := filepath.Join(t.TempDir(), "config.toml")
	input := `model = "internal-model"
model_provider = "company"
notify = ["/host/only/script"]
[model_providers.company]
name = "Company gateway"
base_url = "https://models.example.org/v1"
wire_api = "responses"
env_key = "PRIVATE_TOKEN"
query_params = { "api-version" = "2026-01-01" }
http_headers = { "x-project" = "biology" }
`
	if err := os.WriteFile(source, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	s := selection()
	s.Endpoint = "private"
	s.ConfigFile = source
	data, err := endpointConfig(s)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := toml.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	provider := parsed["model_providers"].(map[string]any)["company"].(map[string]any)
	if provider["experimental_bearer_token"] != "a-secret-key" || provider["env_key"] != nil {
		t.Fatal("credential was not mapped")
	}
	if parsed["notify"] != nil || provider["query_params"] == nil || provider["http_headers"] == nil {
		t.Fatal("connection import did not preserve the right settings")
	}
	env, cleanup, err := PrepareEnvironment(s)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if len(env) != 2 {
		t.Fatal(env)
	}
	saved := filepath.Join(settingsDir(), "endpoints", "private.toml")
	info, err := os.Stat(saved)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("credentials mode: %o", info.Mode().Perm())
	}
	if defaultCodexConfig() != saved {
		t.Fatal("saved endpoint was not discovered")
	}
	original, _ := os.ReadFile(source)
	if string(original) != input {
		t.Fatal("import changed host config")
	}
	s.ConfigFile = saved
	if _, err := endpointConfig(s); err != nil {
		t.Fatal("saved config cannot be reused:", err)
	}
}
func TestEndpointMissingCredentialsFailsWithoutWriting(t *testing.T) {
	isolatedSettings(t)
	t.Setenv("MISSING_TEST_KEY", "")
	source := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(source, []byte("model='internal'\nmodel_provider='private'\n[model_providers.private]\nbase_url='https://private.example/v1'\nenv_key='MISSING_TEST_KEY'\n"), 0600)
	s := selection()
	s.Endpoint = "private"
	s.ConfigFile = source
	if _, _, err := PrepareEnvironment(s); err == nil || !strings.Contains(err.Error(), "MISSING_TEST_KEY") {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(settingsDir(), "endpoints", "private.toml")); !os.IsNotExist(err) {
		t.Fatal("wrote invalid endpoint")
	}
	s.Token = "key-from-form"
	if _, err := endpointConfig(s); err != nil {
		t.Fatal(err)
	}
}
func TestNewEndpointUsesIndependentProvider(t *testing.T) {
	s := selection()
	s.Endpoint = "private"
	s.BaseURL = "https://internal.example/v1"
	s.Model = "company-model"
	s.Token = "provided-key"
	if err := Validate(s); err != nil {
		t.Fatal(err)
	}
	data, err := endpointConfig(s)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("wire_api = 'responses'")) && !bytes.Contains(data, []byte(`wire_api = "responses"`)) {
		t.Fatalf("missing Responses API: %s", data)
	}
	if !bytes.Contains(data, []byte("provided-key")) {
		t.Fatal("key missing")
	}
}
func TestValidateRejectsUnsafeDestinations(t *testing.T) {
	for _, input := range []string{"", "**", "https://example.com", "example.com/path", "example.com;touch /tmp/bad", "*."} {
		s := selection()
		s.Network = "restricted"
		s.Sites = input
		if err := Validate(s); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
	for _, input := range []string{"example.com", "*.example.com, files.pythonhosted.org", "**.example.com:443"} {
		s := selection()
		s.Network = "restricted"
		s.Sites = input
		if err := Validate(s); err != nil {
			t.Errorf("rejected %q: %v", input, err)
		}
	}
}
