package main

import "github.com/mpg-age-bioinformatics/vscode-sandboxes/internal/windowslauncher"

var skillsRef string

func main() {
	windowslauncher.Main(windowslauncher.Config{
		AppName:          "Bioinformatics Sandbox",
		SandboxSlug:      "bioinformatics-sandbox",
		SkillLauncher:    "bioinformatics-sandox.sh",
		ProjectRunner:    "Run Bioinformatics Sandbox.exe",
		EnvPrefix:        "BIOINFORMATICS_SANDBOX",
		DefaultSkillsRef: skillsRef,
	})
}
