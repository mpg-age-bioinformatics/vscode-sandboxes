package startui

import (
	"fmt"
	"regexp"
)

// SandboxType is shared by the selector and both platform dispatchers.
type SandboxType struct {
	Slug         string `json:"slug"`
	Name         string `json:"name"`
	Version      string `json:"version"`
	VersionLabel string `json:"versionLabel"`
	NeedsPodman  bool   `json:"needsPodman"`
}

func Catalog() []SandboxType {
	return []SandboxType{
		{"python-sandbox", "Python", "3.13", "Python version", true},
		{"r-sandbox", "R", "4.5", "R version", true},
		{"bioinformatics-sandbox", "General bioinformatics", "", "", false},
	}
}

func FindSandbox(slug string) (SandboxType, error) {
	for _, item := range Catalog() {
		if item.Slug == slug {
			return item, nil
		}
	}
	return SandboxType{}, fmt.Errorf("Choose Python, R, or general bioinformatics.")
}

func resolveSelection(config Config, selected Selection) (Selection, error) {
	if len(config.Sandboxes) == 0 {
		if selected.Sandbox != "" && selected.Sandbox != config.Slug {
			return selected, fmt.Errorf("This project launcher is configured for %s.", config.Slug)
		}
		selected.Sandbox = config.Slug
	} else {
		found := false
		for _, item := range config.Sandboxes {
			if item.Slug == selected.Sandbox {
				found = true
			}
		}
		if !found {
			return selected, fmt.Errorf("Choose a sandbox type.")
		}
	}
	item, err := FindSandbox(selected.Sandbox)
	if err != nil {
		return selected, err
	}
	if !config.ProjectMode && item.VersionLabel != "" && !regexp.MustCompile(`^[0-9]+\.[0-9]+(\.[0-9]+)?$`).MatchString(selected.Version) {
		return selected, fmt.Errorf("Enter a %s in major.minor or major.minor.patch format.", item.VersionLabel)
	}
	if item.VersionLabel == "" {
		selected.Version = ""
	}
	return selected, nil
}
