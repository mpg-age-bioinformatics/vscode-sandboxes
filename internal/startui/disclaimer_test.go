package startui

import (
	"bytes"
	licensing "github.com/mpg-age-bioinformatics/vscode-sandboxes"
	"html"
	"strings"
	"testing"
)

func TestResponsibilityNoticeInUIAndRemoteProjects(t *testing.T) {
	page := string(startupPage())
	if strings.Contains(page, "__DISCLAIMER_TEXT__") || !strings.Contains(page, html.EscapeString(licensing.Disclaimer)) || !strings.Contains(page, "Use at your own risk.") {
		t.Fatal("startup page is missing the complete canonical responsibility notice")
	}
	for _, source := range []string{"python", "r", "bioinformatics", "registry", "dockerfile"} {
		profile := RemoteProfile{ImageSource: source, Version: "3.13", Architecture: "amd64"}
		if source == "r" {
			profile.Version = "4.5"
		}
		files, err := remoteProjectFiles(profile, "FROM ubuntu:24.04\n", nil)
		if err != nil {
			t.Fatal(source, err)
		}
		if !bytes.Equal(files["code/.sandbox/VS-Code-Sandboxes-DISCLAIMER.txt"], []byte(licensing.Disclaimer)) {
			t.Fatal("remote project lost responsibility notice", source)
		}
	}
}
