package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/mpg-age-bioinformatics/vscode-sandboxes/internal/startui"
)

func main() {
	if startui.HandleHelper() {
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "--unified" {
		exe, err := os.Executable()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		home, _ := os.UserHomeDir()
		c := startui.Config{Name: "VS Code Sandboxes", Slug: "sandboxes", Version: "3.13", VersionLabel: "Python version", Agent: "codex", Project: filepath.Join(home, "Desktop", "python-sandbox-project"), Sandboxes: startui.Catalog()}
		err = startui.Run(c, func(s startui.Selection, log io.Writer) error {
			item, err := startui.FindSandbox(s.Sandbox)
			if err != nil {
				return err
			}
			name := item.Name
			if item.Slug == "bioinformatics-sandbox" {
				name = "Bioinformatics"
			}
			worker := filepath.Join(filepath.Dir(exe), "Launchers", item.Slug, name+" Sandbox.command")
			return runWorker(worker, s, item.VersionLabel != "", false, log)
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	// name slug version version-label command [project agent]
	if len(os.Args) < 6 {
		fmt.Fprintln(os.Stderr, "Missing desktop launcher configuration")
		os.Exit(2)
	}
	c := startui.Config{Name: os.Args[1], Slug: os.Args[2], Version: os.Args[3], VersionLabel: os.Args[4], Agent: "codex"}
	commandPath := os.Args[5]
	home, _ := os.UserHomeDir()
	c.Project = filepath.Join(home, "Desktop", c.Slug+"-project")
	if len(os.Args) == 8 {
		c.ProjectMode = true
		c.Project = os.Args[6]
		c.Agent = os.Args[7]
	}
	err := startui.Run(c, func(s startui.Selection, log io.Writer) error {
		return runWorker(commandPath, s, c.VersionLabel != "", c.ProjectMode, log)
	})
	if err != nil {
		data, _ := json.Marshal(err.Error())
		fmt.Fprintln(os.Stderr, string(data))
		os.Exit(1)
	}
}

func runWorker(path string, s startui.Selection, needsVersion, projectMode bool, log io.Writer) error {
	env, cleanup, err := startui.PrepareEnvironment(s)
	if err != nil {
		return err
	}
	defer cleanup()
	args := []string{path}
	if projectMode {
		args = append(args, s.Agent)
	} else {
		if needsVersion {
			args = append(args, s.Version)
		}
		args = append(args, s.Agent, s.Project)
	}
	cmd := exec.Command("/bin/bash", args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = log
	cmd.Stderr = log
	return cmd.Run()
}
