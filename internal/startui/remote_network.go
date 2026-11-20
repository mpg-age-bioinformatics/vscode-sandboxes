package startui

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

func validateRemoteNetwork(p RemoteProfile) error {
	switch p.Network {
	case "", "open", "none", "restricted":
	default:
		return errors.New("Choose No internet, Selected sites or Open.")
	}
	if len(p.Sites) > 32768 {
		return errors.New("Allowed hosts exceeds 32 KiB.")
	}
	if p.Network == "restricted" {
		rules, err := sites(p.Sites)
		if err != nil {
			return err
		}
		for _, rule := range rules {
			if at := strings.LastIndex(rule, ":"); at >= 0 {
				port, err := strconv.Atoi(rule[at+1:])
				if err != nil || port < 1 || port > 65535 {
					return errors.New("Allowed host ports must be between 1 and 65535.")
				}
			}
		}
	}
	if p.EditorCommit != "" && !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(p.EditorCommit) {
		return errors.New("Invalid VS Code build identifier.")
	}
	for _, extension := range p.EditorExtensions {
		if !regexp.MustCompile(`^[A-Za-z0-9-]+\.[A-Za-z0-9-]+$`).MatchString(extension) {
			return errors.New("Invalid editor extension identifier.")
		}
	}
	return nil
}

func remoteEndpointNetwork(p RemoteProfile, e remoteEndpoint) (RemoteProfile, string, error) {
	if e.BaseURL == "" || e.Reset {
		return p, "", nil
	}
	selection, host, err := withPrivateEndpointAccess(Selection{Network: p.Network, Sites: p.Sites, Endpoint: "private", BaseURL: e.BaseURL})
	if err != nil {
		return p, "", err
	}
	p.Sites = selection.Sites
	return p, host, nil
}

func validateExistingRemoteEndpoint(p RemoteProfile, e remoteEndpoint) error {
	if e.BaseURL == "" || e.Reset || p.Network == "" || p.Network == "open" {
		return nil
	}
	if p.Network == "none" {
		return errors.New(privateOfflineMessage)
	}
	parsed, err := url.Parse(e.BaseURL)
	if err != nil {
		return errors.New("Invalid private endpoint URL.")
	}
	host, port := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), "."), parsed.Port()
	if port == "" {
		port = "443"
		if parsed.Scheme == "http" {
			port = "80"
		}
	}
	rules, err := sites(p.Sites)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		name, targetPort := rule, ""
		if at := strings.LastIndex(rule, ":"); at >= 0 {
			name, targetPort = rule[:at], rule[at+1:]
		}
		if targetPort != "" && targetPort != port || targetPort == "" && port != "80" && port != "443" {
			continue
		}
		name = strings.TrimSuffix(name, ".")
		if name == host || strings.HasPrefix(name, "*.") && strings.HasSuffix(host, name[1:]) || strings.HasPrefix(name, "**.") && (host == name[3:] || strings.HasSuffix(host, name[2:])) {
			return nil
		}
	}
	return errors.New("This endpoint is outside the running allocation's allowlist. Start a new allocation with this endpoint or use an already permitted destination.")
}

// Version comes from the editor actually installed on this laptop. The helper
// downloads this exact server before any project directory enters a container.
func prepareRemoteEditorProfile(ctx context.Context, p *RemoteProfile) error {
	command, err := editorCommand("--version")
	if err != nil {
		return err
	}
	bounded := exec.CommandContext(ctx, command.Path, command.Args[1:]...)
	bounded.Env = command.Env
	hideWindow(bounded)
	output, err := bounded.Output()
	if err != nil {
		return fmt.Errorf("Read installed VS Code version: %w", err)
	}
	match := regexp.MustCompile(`(?m)^[a-f0-9]{40}\r?$`).FindString(string(output))
	if match == "" {
		return errors.New("Could not identify the installed VS Code commit for isolated editor provisioning.")
	}
	p.EditorCommit = strings.TrimSpace(match)
	p.EditorExtensions = nil
	for _, extension := range remoteExtensions(*p) {
		if strings.HasPrefix(strings.ToLower(extension), "ms-vscode-remote.") || strings.EqualFold(extension, "Continue.continue") {
			continue
		}
		p.EditorExtensions = append(p.EditorExtensions, strings.Split(extension, "@")[0])
	}
	return validateRemoteNetwork(*p)
}
