package main

import "github.com/mpg-age-bioinformatics/vscode-sandboxes/internal/windowslauncher"

var skillsRef string

func main() {
	windowslauncher.Main(windowslauncher.Config{
		AppName:          "Python Sandbox",
		SandboxSlug:      "python-sandbox",
		SkillLauncher:    "python-sandox.sh",
		ProjectRunner:    "Run Python Sandbox.exe",
		VersionPrompt:    "Python version (major.minor or major.minor.patch)",
		NeedsVersion:     true,
		NeedsDocker:      true,
		DefaultVersion:   "3.13",
		EnvPrefix:        "PYTHON_SANDBOX",
		DefaultSkillsRef: skillsRef,
	})
}
