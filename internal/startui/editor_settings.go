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
		if strings.EqualFold(key, "Continue.continue") {
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
	operations = append(operations, map[string]any{"op": "add", "path": "/remote.extensionKind/Continue.continue", "value": []string{"workspace"}})
	patch, _ := json.Marshal(operations)
	if err := value.Patch(patch); err != nil {
		return nil, fmt.Errorf("Set Continue execution location: %w", err)
	}
	value.Format()
	return value.Pack(), nil
}

func ensureContinueWorkspaceFile(path string) error {
	original, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("Read VS Code user settings: %w", err)
	}
	updated, err := continueWorkspaceSettings(original)
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
		return fmt.Errorf("Save VS Code Continue execution setting: %w", err)
	}
	return nil
}
