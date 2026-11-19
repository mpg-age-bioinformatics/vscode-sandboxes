package startui

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

const privateOfflineMessage = "No internet cannot be used with a private endpoint. Choose Selected sites or Open."

// Private endpoints are a required, disclosed exception in restricted mode.
// Resolve a file-backed connection on the host; never include credentials/paths.
func withPrivateEndpointAccess(selection Selection) (Selection, string, error) {
	if selection.Endpoint != "private" {
		return selection, "", nil
	}
	if selection.Network == "none" {
		return selection, "", errors.New(privateOfflineMessage)
	}
	if selection.Network != "restricted" {
		return selection, "", nil
	}
	base := selection.BaseURL
	if selection.ConfigFile != "" && !selection.ConfigOverrides {
		preview, err := previewCodexConfig(selection.ConfigFile)
		if err != nil {
			return selection, "", err
		}
		base = preview.BaseURL
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return selection, "", errors.New("Enter a valid private endpoint URL before starting with Selected sites.")
	}
	resource := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	if port != "" && !(parsed.Scheme == "https" && port == "443") && !(parsed.Scheme == "http" && port == "80") {
		resource += ":" + port
	}
	if _, err := sites(resource); err != nil {
		return selection, "", errors.New("Use a DNS hostname or IPv4 address for the private endpoint in Selected sites, or choose Open.")
	}
	var requested []string
	if strings.TrimSpace(selection.Sites) != "" {
		requested, err = sites(selection.Sites)
		if err != nil {
			return selection, "", err
		}
	}
	found := false
	for _, host := range requested {
		if host == resource {
			found = true
			break
		}
	}
	if !found {
		if selection.Sites != "" {
			selection.Sites = strings.TrimSpace(selection.Sites) + "\n"
		}
		selection.Sites += resource
	}
	if selection.Sandbox != "" {
		lists := map[string]string{}
		for kind, list := range selection.SiteLists {
			lists[kind] = list
		}
		lists[selection.Sandbox] = selection.Sites
		selection.SiteLists = lists
	}
	return selection, resource, nil
}

func privateEndpointNotice(host string) string {
	return fmt.Sprintf("Warning: private endpoint host %s is automatically included in Selected sites.", host)
}
