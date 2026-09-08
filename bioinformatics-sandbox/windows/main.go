package main

import "github.com/mpg-age-bioinformatics/vscode-sandboxes/internal/windowslauncher"

func main() {
	windowslauncher.Main(windowslauncher.Config{
		AppName:       "Bioinformatics Sandbox",
		SandboxSlug:   "bioinformatics-sandbox",
		ProjectRunner: "Run Bioinformatics Sandbox.exe",
		ShellLauncher: "run-bioinformatics-sandbox.sh",
	})
}
