package startui

import (
	"bytes"
	"encoding/json"
	"errors"
	"os/exec"
)

// A capability must never be placed in process arguments or the environment.
func startupWindowCommand(platform, helper, address, title string) (*exec.Cmd, error) {
	var command *exec.Cmd
	switch platform {
	case "darwin":
		command = exec.Command(helper)
	case "windows":
		command = exec.Command("powershell.exe", "-NoProfile", "-STA", "-Command", windowsWindow)
		hideWindow(command)
	default:
		return nil, errors.New("the desktop window supports macOS and Windows")
	}
	bootstrap, err := json.Marshal(map[string]string{"url": address, "title": title})
	if err != nil {
		return nil, err
	}
	command.Stdin = bytes.NewReader(bootstrap)
	return command, nil
}
