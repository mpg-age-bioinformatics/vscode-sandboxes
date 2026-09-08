package main

import "github.com/mpg-age-bioinformatics/vscode-sandboxes/internal/windowslauncher"

func main() {
	windowslauncher.Main(windowslauncher.Config{
		AppName:        "Python Sandbox",
		SandboxSlug:    "python-sandbox",
		ProjectRunner:  "Run Python Sandbox.exe",
		ShellLauncher:  "run-python-sandbox.sh",
		VersionPrompt:  "Python version (major.minor or major.minor.patch)",
		NeedsVersion:   true,
		NeedsDocker:    true,
		DefaultVersion: "3.13",
	})
}
