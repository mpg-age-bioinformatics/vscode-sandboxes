package startui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirectConnectionIgnoresLegacyFilesAndModels(t *testing.T) {
	isolatedSettings(t)
	for _, test := range []struct {
		url, key, endpoint string
		valid              bool
	}{
		{"", "", "default", true},
		{"https://private.example/v1", "sandbox-key", "private", true},
		{"https://private.example/v1", "", "private", false},
		{"", "sandbox-key", "private", false},
	} {
		s := selection()
		s.BaseURL, s.Token = test.url, test.key
		s.ConfigFile, s.Model, s.ConfigOverrides = "/removed/config.toml", "stale-model", true
		s = directConnection(s)
		if s.Endpoint != test.endpoint || s.ConfigFile != "" || s.Model != "" || s.ConfigOverrides {
			t.Fatal("retained legacy connection selection")
		}
		if (Validate(s) == nil) != test.valid {
			t.Fatal("incorrect direct connection validation", test.endpoint)
		}
		if !test.valid {
			continue
		}
		env, cleanup, err := PrepareEnvironment(s)
		if err != nil {
			t.Fatal(err)
		}
		path := strings.TrimPrefix(env[1], "VSCODE_SANDBOX_STARTUP_SELECTION=")
		var manifest Selection
		if err := readJSON(path, &manifest); err != nil {
			t.Fatal(err)
		}
		if manifest.ConfigFile != "" || manifest.Model != "" || manifest.BaseURL != test.url || manifest.Token != test.key {
			t.Fatal("incorrect runtime selection")
		}
		entries, err := os.ReadDir(filepath.Dir(path))
		if err != nil || len(entries) != 1 || entries[0].Name() != "selection.json" {
			t.Fatal("created an unnecessary runtime connection file", err)
		}
		if _, err := os.Stat(filepath.Join(settingsDir(), "endpoints")); !os.IsNotExist(err) {
			t.Fatal("created a persistent endpoint cache")
		}
		cleanup()
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("runtime manifest was not removed")
		}
	}
}
