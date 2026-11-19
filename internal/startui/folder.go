package startui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// folderStart finds an existing directory even when the proposed project is new.
func folderStart(proposed string) string {
	for filepath.IsAbs(proposed) {
		if info, err := os.Stat(proposed); err == nil && info.IsDir() {
			return proposed
		}
		parent := filepath.Dir(proposed)
		if parent == proposed {
			break
		}
		proposed = parent
	}
	home, _ := os.UserHomeDir()
	return home
}

func pickFolder(ctx context.Context, proposed string) (string, error) {
	start := folderStart(proposed)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-STA", "-Command", windowsFolderPicker)
		cmd.Env = append(os.Environ(), "SANDBOX_FOLDER_START="+start)
		hideWindow(cmd)
	default:
		return "", fmt.Errorf("native folder selection is unavailable in this window")
	}
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("open folder picker: %w", err)
	}
	var selected string
	if err := json.Unmarshal([]byte(strings.TrimPrefix(string(output), "\ufeff")), &selected); err != nil {
		return "", fmt.Errorf("read selected folder: %w", err)
	}
	return selected, nil
}

const windowsFolderPicker = `
Add-Type -AssemblyName System.Windows.Forms
[Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false)
$dialog = New-Object Windows.Forms.FolderBrowserDialog
$dialog.Description = 'Choose or create your sandbox project folder'
$dialog.ShowNewFolderButton = $true
$dialog.SelectedPath = $env:SANDBOX_FOLDER_START
$owner = New-Object Windows.Forms.Form
$owner.TopMost = $true
try {
 $selected = ''
 if ($dialog.ShowDialog($owner) -eq [Windows.Forms.DialogResult]::OK) { $selected = $dialog.SelectedPath }
 [Console]::Write((ConvertTo-Json -InputObject $selected -Compress))
} finally { $dialog.Dispose(); $owner.Dispose() }
`
