package startui

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// The socket grants access to an authenticated login, so keep it in a private
// directory. Hash the literal alias: different users/aliases must not share it.
func remoteControlPath(host string) (string, error) {
	if !remoteHostPattern.MatchString(host) {
		return "", errors.New("Enter an SSH alias or user@hostname first.")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".ssh", "vscode-sandboxes")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("SSH connection directory must be a real private directory.")
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(host))
	return filepath.Join(dir, fmt.Sprintf("%x", digest[:12])), nil
}

func loginSSHOptions(host string) ([]string, error) {
	options := []string{"-T", "-o", "BatchMode=no", "-o", "StrictHostKeyChecking=yes", "-o", "ForwardAgent=no", "-o", "ConnectTimeout=15", "-o", "ServerAliveInterval=30"}
	if runtime.GOOS != "windows" {
		socket, err := remoteControlPath(host)
		if err != nil {
			return nil, err
		}
		options = append(options, "-o", "ControlMaster=auto", "-o", "ControlPersist=600", "-o", "ControlPath="+socket)
	}
	return options, nil
}

func loginSSHProxyOptions(host string, quote func(string) string) (string, error) {
	options, err := loginSSHOptions(host)
	if err != nil {
		return "", err
	}
	// VS Code uses the already authenticated master; it cannot reach the UI broker.
	for i, option := range options {
		if option == "BatchMode=no" {
			options[i] = "BatchMode=yes"
		}
	}
	parts := make([]string, len(options))
	for i, option := range options {
		parts[i] = quote(option)
	}
	return strings.Join(parts, " "), nil
}
