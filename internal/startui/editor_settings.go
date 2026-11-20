package startui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tailscale/hujson"
)

// EnsureContinueWorkspace sets the local preference VS Code reads before remote
// extension selection. Remote/container settings arrive too late when a local
// Continue installation prefers its UI extension host.
func EnsureContinueWorkspace() error {
	root, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	return ensureContinueWorkspaceFile(filepath.Join(root, "Code", "User", "settings.json"))
}

func continueWorkspaceSettings(original []byte) ([]byte, error) {
	return workspaceExtensionSettings(original, "Continue.continue")
}

func workspaceExtensionSettings(original []byte, extension string) ([]byte, error) {
	if len(bytes.TrimSpace(original)) == 0 {
		original = []byte("{}\n")
	}
	value, err := hujson.Parse(bytes.Clone(original))
	if err != nil {
		return nil, errors.New("VS Code user settings are invalid JSON with comments; repair settings.json before starting")
	}
	standard := value.Clone()
	standard.Standardize()
	var settings map[string]json.RawMessage
	if err := json.Unmarshal(standard.Pack(), &settings); err != nil || settings == nil {
		return nil, errors.New("VS Code user settings must be a JSON object")
	}
	kinds := map[string]json.RawMessage{}
	raw, exists := settings["remote.extensionKind"]
	if exists {
		if err := json.Unmarshal(raw, &kinds); err != nil || kinds == nil {
			return nil, errors.New("VS Code remote.extensionKind must be a JSON object")
		}
	}
	correct := false
	operations := []map[string]any{}
	if !exists {
		operations = append(operations, map[string]any{"op": "add", "path": "/remote.extensionKind", "value": map[string]any{}})
	}
	for key, raw := range kinds {
		if strings.EqualFold(key, extension) {
			var kind []string
			if json.Unmarshal(raw, &kind) == nil && len(kind) == 1 && kind[0] == "workspace" {
				correct = true
			}
			operations = append(operations, map[string]any{"op": "remove", "path": "/remote.extensionKind/" + key})
		}
	}
	if correct && len(operations) == 1 {
		return original, nil
	}
	operations = append(operations, map[string]any{"op": "add", "path": "/remote.extensionKind/" + extension, "value": []string{"workspace"}})
	patch, _ := json.Marshal(operations)
	if err := value.Patch(patch); err != nil {
		return nil, fmt.Errorf("Set remote extension execution location: %w", err)
	}
	value.Format()
	return value.Pack(), nil
}

func ensureContinueWorkspaceFile(path string) error {
	return ensureWorkspaceExtensionFile(path, "Continue.continue")
}

func EnsureRemoteCodexWorkspace() error {
	root, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	return ensureWorkspaceExtensionFile(filepath.Join(root, "Code", "User", "settings.json"), "openai.chatgpt")
}

func ensureWorkspaceExtensionFile(path, extension string) error {
	return ensureEditorSettingsFile(path, func(original []byte) ([]byte, error) { return workspaceExtensionSettings(original, extension) })
}

// EnsureRemoteEditorSettings configures the local defaults Remote SSH consumes
// when it creates the remote server.
// The ordinary desktop --install-extension CLI installs into the local host.
func EnsureRemoteEditorSettings(profile RemoteProfile) error {
	root, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	return ensureEditorSettingsFile(filepath.Join(root, "Code", "User", "settings.json"), func(original []byte) ([]byte, error) { return remoteEditorSettings(original, profile) })
}

func remoteEditorSettings(original []byte, profile RemoteProfile) ([]byte, error) {
	var err error
	for _, extension := range []string{"openai.chatgpt", "Continue.continue"} {
		original, err = workspaceExtensionSettings(original, extension)
		if err != nil {
			return nil, err
		}
	}
	value, err := hujson.Parse(bytes.Clone(original))
	if err != nil {
		return nil, err
	}
	standard := value.Clone()
	standard.Standardize()
	var settings map[string]json.RawMessage
	if err := json.Unmarshal(standard.Pack(), &settings); err != nil {
		return nil, err
	}
	var defaults []string
	if raw, exists := settings["remote.SSH.defaultExtensions"]; exists {
		if err := json.Unmarshal(raw, &defaults); err != nil || string(raw) == "null" {
			return nil, errors.New("VS Code remote.SSH.defaultExtensions must be a list of extension names")
		}
	}
	previous, _ := json.Marshal(defaults)
	for _, extension := range remoteExtensions(profile) {
		id := strings.ToLower(strings.Split(extension, "@")[0])
		if id == "ms-vscode-remote.remote-ssh" || id == "ms-vscode-remote.remote-containers" {
			continue
		}
		found := false
		for index, current := range defaults {
			if strings.EqualFold(strings.Split(current, "@")[0], id) {
				if id == "continue.continue" {
					defaults[index] = extension
				}
				found = true
				break
			}
		}
		if !found {
			defaults = append(defaults, extension)
		}
	}
	desired, _ := json.Marshal(defaults)
	operations := []map[string]any{}
	if !bytes.Equal(previous, desired) {
		operations = append(operations, map[string]any{"op": "add", "path": "/remote.SSH.defaultExtensions", "value": defaults})
	}
	if profile.Network == "restricted" || profile.Network == "none" {
		operations = append(operations, map[string]any{"op": "add", "path": "/remote.SSH.localServerDownload", "value": "always"})
	}
	if len(operations) == 0 {
		return original, nil
	}
	patch, _ := json.Marshal(operations)
	if err := value.Patch(patch); err != nil {
		return nil, err
	}
	value.Format()
	return value.Pack(), nil
}

func ensureEditorSettingsFile(path string, transform func([]byte) ([]byte, error)) error {
	original, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("Read VS Code user settings: %w", err)
	}
	updated, err := transform(original)
	if err != nil {
		return err
	}
	if bytes.Equal(original, updated) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	target := path
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		target = resolved
	}
	file, err := os.CreateTemp(filepath.Dir(target), ".sandbox-editor-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(updated); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	current, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if !bytes.Equal(current, original) {
		return errors.New("VS Code user settings changed during startup; retry")
	}
	if err := os.Rename(file.Name(), target); err != nil {
		return fmt.Errorf("Save VS Code remote settings: %w", err)
	}
	return nil
}
