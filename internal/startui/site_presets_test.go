package startui

import (
	"strings"
	"testing"
)

func TestSitePresetsFollowSandboxLanguage(t *testing.T) {
	presets := sitePresets()
	contains := func(list []string, host string) bool {
		for _, item := range list {
			if item == host {
				return true
			}
		}
		return false
	}
	for _, sandbox := range Catalog() {
		hosts := presets[sandbox.Slug]
		for _, host := range []string{"auth.openai.com", "api.openai.com", "chatgpt.com", "chat.openai.com", "ab.chatgpt.com"} {
			if !contains(hosts, host) {
				t.Errorf("%s is missing Codex authentication/backend host %s", sandbox.Slug, host)
			}
		}
		parsed, err := sites(strings.Join(hosts, "\n"))
		if err != nil || len(parsed) != len(hosts) {
			t.Fatalf("%s contains invalid or duplicate hosts: %v", sandbox.Slug, err)
		}
		for _, host := range []string{"registry-1.docker.io", "auth.docker.io", "ghcr.io", "pkg-containers.githubusercontent.com", "mcr.microsoft.com", "*.data.mcr.microsoft.com", "registry.npmjs.org", "nodejs.org", "deb.debian.org", "ports.ubuntu.com", "update.code.visualstudio.com"} {
			if !contains(hosts, host) {
				t.Errorf("%s is missing startup host %s", sandbox.Slug, host)
			}
		}
		for _, host := range hosts {
			if host == "**" || host == "*.cloudflarestorage.com" || host == "*.amazonaws.com" {
				t.Errorf("overbroad default: %s", host)
			}
		}
	}
	python, r, general := presets["python-sandbox"], presets["r-sandbox"], presets["bioinformatics-sandbox"]
	if !contains(python, "pypi.org") || contains(python, "bioconductor.org") {
		t.Fatal("Python preset has the wrong package hosts")
	}
	if !contains(r, "cloud.r-project.org") || !contains(r, "bioconductor.org") || contains(r, "pypi.org") {
		t.Fatal("R preset has the wrong package hosts")
	}
	union := mergeSites(python, r)
	if len(union) != len(general) {
		t.Fatal("General must contain exactly the union of Python and R defaults")
	}
	for _, host := range union {
		if !contains(general, host) {
			t.Errorf("General is missing %s", host)
		}
	}
}
