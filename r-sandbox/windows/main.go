package main

import "github.com/mpg-age-bioinformatics/vscode-sandboxes/internal/windowslauncher"

var skillsRef string

func main() {
	windowslauncher.Main(windowslauncher.Config{
		AppName:          "R Sandbox",
		SandboxSlug:      "r-sandbox",
		SkillLauncher:    "r-sandox.sh",
		ProjectRunner:    "Run R Sandbox.exe",
		VersionPrompt:    "R version (major.minor or major.minor.patch)",
		NeedsVersion:     true,
		NeedsDocker:      true,
		DefaultVersion:   "4.5",
		EnvPrefix:        "R_SANDBOX",
		DefaultSkillsRef: skillsRef,
	})
}
