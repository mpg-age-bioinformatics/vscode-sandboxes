package main

import "github.com/mpg-age-bioinformatics/vscode-sandboxes/internal/windowslauncher"

func main() {
	windowslauncher.Main(windowslauncher.Config{
		AppName:        "R Sandbox",
		SandboxSlug:    "r-sandbox",
		ProjectRunner:  "Run R Sandbox.exe",
		ShellLauncher:  "run-r-sandbox.sh",
		VersionPrompt:  "R version (major.minor or major.minor.patch)",
		NeedsVersion:   true,
		NeedsDocker:    true,
		DefaultVersion: "4.5",
	})
}
