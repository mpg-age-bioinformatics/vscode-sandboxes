package startui

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tailscale/hujson"
)

func TestContinuePlacementPreservesUserSettings(t *testing.T) {
	for _, original := range []string{"", "{}", `{
 // Keep this user's comment and unrelated extension placement.
 "remote.extensionKind": {"other.extension":["ui"], "Continue.continue":["ui"]},
 "editor.fontSize": 17,
 }`, `{"remote.extensionKind":{"continue.continue":["workspace"]}}`} {
		before := []byte(original)
		updated, err := continueWorkspaceSettings(before)
		if err != nil {
			t.Fatal(err)
		}
		if string(before) != original {
			t.Fatal("mutated original settings buffer")
		}
		value, err := hujson.Parse(bytes.Clone(updated))
		if err != nil {
			t.Fatal(err)
		}
		value.Standardize()
		var settings map[string]any
		if err := json.Unmarshal(value.Pack(), &settings); err != nil {
			t.Fatal(err)
		}
		kinds := settings["remote.extensionKind"].(map[string]any)
		var continuation any
		for key, kind := range kinds {
			if key == "Continue.continue" || key == "continue.continue" {
				continuation = kind
			}
		}
		got, _ := json.Marshal(continuation)
		if string(got) != `["workspace"]` {
			t.Fatal("incorrect execution location", kinds)
		}
		if original != "" && bytes.Contains(before, []byte("other.extension")) {
			if !bytes.Contains(updated, []byte("Keep this user's comment")) || settings["editor.fontSize"] != float64(17) || kinds["other.extension"] == nil {
				t.Fatal("lost unrelated user settings")
			}
		}
		again, err := continueWorkspaceSettings(updated)
		if err != nil || !bytes.Equal(updated, again) {
			t.Fatal("settings changed on repeated startup", err)
		}
	}
}

func TestContinuePlacementDoesNotOverwriteInvalidSettings(t *testing.T) {
	for _, original := range []string{`{bad`, `[]`, `null`, `{"remote.extensionKind":null}`, `{"remote.extensionKind":["ui"]}`} {
		path := filepath.Join(t.TempDir(), "settings.json")
		if err := os.WriteFile(path, []byte(original), 0600); err != nil {
			t.Fatal(err)
		}
		if err := ensureContinueWorkspaceFile(path); err == nil {
			t.Fatal("accepted invalid user settings", original)
		}
		after, _ := os.ReadFile(path)
		if string(after) != original {
			t.Fatal("overwrote invalid user settings")
		}
	}
}

func TestRemoteAutomaticExtensionsPreserveUserChoices(t *testing.T) {
	original := []byte(`{
 // Keep this comment and custom extensions.
 "editor.fontSize": 19,
 "remote.SSH.defaultExtensions": ["user.extension@1.2.3", "continue.continue@2.0.0"],
 "remote.extensionKind": {"Continue.continue":["ui"]},
 }`)
	updated, err := remoteEditorSettings(original, RemoteProfile{ImageSource: "python"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(updated, []byte("Keep this comment")) {
		t.Fatal("lost JSONC comment")
	}
	value, err := hujson.Parse(updated)
	if err != nil {
		t.Fatal(err)
	}
	value.Standardize()
	var settings map[string]any
	if err := json.Unmarshal(value.Pack(), &settings); err != nil {
		t.Fatal(err)
	}
	extensions := settings["remote.SSH.defaultExtensions"].([]any)
	found := map[string]int{}
	for _, entry := range extensions {
		found[entry.(string)]++
	}
	if found["Continue.continue"] != 1 || found["Continue.continue@2.0.0"] != 0 || found["user.extension@1.2.3"] != 1 || found["openai.chatgpt"] != 1 || settings["editor.fontSize"] != float64(19) {
		t.Fatal(settings)
	}
	again, err := remoteEditorSettings(updated, RemoteProfile{ImageSource: "python"})
	if err != nil || !bytes.Equal(updated, again) {
		t.Fatal("defaults changed on reconnect", err)
	}
	for _, invalid := range []string{`{"remote.SSH.defaultExtensions":"bad"}`, `{"remote.SSH.defaultExtensions":null}`} {
		if _, err := remoteEditorSettings([]byte(invalid), RemoteProfile{}); err == nil {
			t.Fatal("overwrote invalid defaults", invalid)
		}
	}
}
