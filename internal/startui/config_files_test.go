package startui

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCreateConfigAndRememberLastOriginalFile(t *testing.T) {
	isolatedSettings(t)
	target := filepath.Join(t.TempDir(), "new sandbox config.toml")
	s := selection()
	s.Endpoint = "private"
	s.Model = "private-model"
	s.BaseURL = "https://models.example/v1?api_key=do-not-save"
	s.Token = "do-not-save-this-either"
	if err := createConfigFile(target, s); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("do-not-save")) || !bytes.Contains(data, []byte("private-model")) {
		t.Fatal("new file included authentication or lost settings")
	}
	if lastUsedConfig() != target {
		t.Fatal("did not remember created file immediately")
	}
	info, _ := os.Stat(target)
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatal("new file permissions", info.Mode())
	}
	original := append([]byte(nil), data...)
	if err := createConfigFile(target, s); err == nil {
		t.Fatal("overwrote an existing file")
	}
	after, _ := os.ReadFile(target)
	if !bytes.Equal(original, after) {
		t.Fatal("changed an existing file")
	}
	other := filepath.Join(t.TempDir(), "last selected.toml")
	os.WriteFile(other, []byte("model='another-model'\n"), 0600)
	if err := rememberConfig(other); err != nil {
		t.Fatal(err)
	}
	if lastUsedConfig() != other {
		t.Fatal("did not remember the last selected original file")
	}
	os.Remove(other)
	if lastUsedConfig() != "" {
		t.Fatal("returned a deleted file")
	}
}

func TestSourceTomlCannotBeExposedThroughWorkspace(t *testing.T) {
	isolatedSettings(t)
	project := t.TempDir()
	file := filepath.Join(project, "config.toml")
	os.WriteFile(file, []byte("model='test'\n"), 0600)
	s := selection()
	s.Project = project
	s.ConfigFile = file
	if err := Validate(s); err == nil {
		t.Fatal("accepted source file exposed by the workspace mount")
	}
	s.ConfigFile = ""
	if err := createConfigFile(filepath.Join(project, "new.toml"), s); err == nil {
		t.Fatal("created a source file in the shared workspace")
	}
	outside := filepath.Join(t.TempDir(), "outside.toml")
	os.WriteFile(outside, []byte("model='test'\n"), 0600)
	s.ConfigFile = outside
	if err := Validate(s); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		link := filepath.Join(t.TempDir(), "workspace-link")
		if err := os.Symlink(project, link); err != nil {
			t.Fatal(err)
		}
		s.ConfigFile = filepath.Join(link, "config.toml")
		if err := Validate(s); err == nil {
			t.Fatal("accepted a symlink into the workspace")
		}
	}
}

func TestEditSelectedTemplateAndSaveWithoutSharingAuthentication(t *testing.T) {
	isolatedSettings(t)
	source := filepath.Join(t.TempDir(), "template.toml")
	original := []byte("# empty template\n")
	os.WriteFile(source, original, 0600)
	preview, err := previewCodexConfig(source)
	if err != nil {
		t.Fatal(err)
	}
	s := selection()
	s.Endpoint = "private"
	s.ConfigFile = source
	s.BaseURL = "https://private.example/v1"
	s.Model = "private-model"
	s.ConfigOverrides = true
	draft, err := endpointConfig(s)
	if err != nil || !bytes.Contains(draft, []byte("https://private.example/v1")) {
		t.Fatal("draft settings were not applied", err)
	}
	unchanged, _ := os.ReadFile(source)
	if !bytes.Equal(unchanged, original) {
		t.Fatal("typing changed the source file")
	}
	s.Token = "sandbox-only-key"
	if err := updateConfigFile(source, preview.Revision, s); err != nil {
		t.Fatal(err)
	}
	saved, _ := os.ReadFile(source)
	if !bytes.Contains(saved, []byte("private-model")) || bytes.Contains(saved, []byte("sandbox-only-key")) {
		t.Fatal("saved wrong connection/authentication")
	}
	s.ConfigOverrides = false
	s.Token = ""
	imported, err := endpointConfig(s)
	if err != nil || !bytes.Contains(imported, []byte("private-model")) {
		t.Fatal("could not use saved file", err)
	}
	if err := updateConfigFile(source, preview.Revision, s); err == nil {
		t.Fatal("overwrote a file changed after preview")
	}
}

func TestSaveConnectionPreservesUnrelatedOriginalSettings(t *testing.T) {
	isolatedSettings(t)
	source := filepath.Join(t.TempDir(), "original.toml")
	os.WriteFile(source, []byte("model='old'\nmodel_provider='company'\nnotify=['/host/script']\n[model_providers.company]\nbase_url='https://old.example/v1'\nexperimental_bearer_token='host-secret'\n"), 0600)
	preview, err := previewCodexConfig(source)
	if err != nil {
		t.Fatal(err)
	}
	s := selection()
	s.Endpoint = "private"
	s.BaseURL = "https://new.example/v1"
	s.Model = "new-model"
	if err := updateConfigFile(source, preview.Revision, s); err != nil {
		t.Fatal(err)
	}
	saved, _ := os.ReadFile(source)
	if !bytes.Contains(saved, []byte("/host/script")) || !bytes.Contains(saved, []byte("host-secret")) {
		t.Fatal("removed unrelated original settings")
	}
	s.ConfigFile = source
	imported, err := endpointConfig(s)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(imported, []byte("host-secret")) || bytes.Contains(imported, []byte("/host/script")) {
		t.Fatal("imported source authentication/hooks")
	}
}
