package windowslauncher

import "github.com/mpg-age-bioinformatics/vscode-sandboxes/internal/startui"

func configForSandbox(slug string) (Config, error) {
	item, err := startui.FindSandbox(slug)
	if err != nil {
		return Config{}, err
	}
	name := item.Name
	if slug == "bioinformatics-sandbox" {
		name = "Bioinformatics"
	}
	return Config{AppName: name + " Sandbox", SandboxSlug: slug, ProjectRunner: "Run " + name + " Sandbox.exe", ShellLauncher: "run-" + slug + ".sh", VersionPrompt: item.VersionLabel, NeedsVersion: item.VersionLabel != "", DefaultVersion: item.Version}, nil
}
