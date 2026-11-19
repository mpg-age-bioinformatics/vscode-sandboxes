package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mpg-age-bioinformatics/vscode-sandboxes/internal/startui"
)

func main() {
	if startui.HandleHelper() {
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "--unified" {
		home, _ := os.UserHomeDir()
		c := startui.Config{Name: "VS Code - Docker Sandboxes", Slug: "sandboxes", Version: "3.13", VersionLabel: "Python version", Agent: "both", Project: filepath.Join(home, "sandboxes", "python-sandbox-project"), Sandboxes: startui.Catalog()}
		err := startui.Run(c, func(s startui.Selection, log io.Writer) error {
			return runWorker(s, log)
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	// name slug version version-label legacy-command [project agent]
	// Keep the old argument layout for existing macOS app entry points.
	if len(os.Args) < 6 {
		fmt.Fprintln(os.Stderr, "Missing desktop launcher configuration")
		os.Exit(2)
	}
	c := startui.Config{Name: os.Args[1], Slug: os.Args[2], Version: os.Args[3], VersionLabel: os.Args[4], Agent: "both"}
	home, _ := os.UserHomeDir()
	c.Project = filepath.Join(home, "sandboxes", c.Slug+"-project")
	if len(os.Args) == 8 {
		c.ProjectMode = true
		c.Project = os.Args[6]
		c.Agent = os.Args[7]
	}
	err := startui.Run(c, func(s startui.Selection, log io.Writer) error {
		return runWorker(s, log)
	})
	if err != nil {
		data, _ := json.Marshal(err.Error())
		fmt.Fprintln(os.Stderr, string(data))
		os.Exit(1)
	}
}

func runWorker(s startui.Selection, log io.Writer) error {
	return startui.LaunchSandbox(s, log)
}
