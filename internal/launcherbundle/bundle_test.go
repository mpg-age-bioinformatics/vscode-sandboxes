package launcherbundle

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMaterializeSandboxBundles(t *testing.T) {
	tests := []struct {
		slug    string
		runtime string
	}{
		{"python-sandbox", "run-python-sandbox.sh"},
		{"r-sandbox", "run-r-sandbox.sh"},
		{"bioinformatics-sandbox", "run-bioinformatics-sandbox.sh"},
	}
	for _, test := range tests {
		t.Run(test.slug, func(t *testing.T) {
			destination := t.TempDir()
			if err := Materialize(test.slug, destination); err != nil {
				t.Fatal(err)
			}
			if test.slug != "bioinformatics-sandbox" {
				for _, file := range []string{"devcontainer.json.template", "run-language-sandbox.sh", "start-development-container.sh", "development-container.js", "initialize-development-container.sh", "verify-development-container.sh"} {
					if _, err := os.Stat(filepath.Join(destination, "assets", file)); err != nil {
						t.Fatalf("missing shared runtime %s: %v", file, err)
					}
				}
				if _, err := os.Stat(filepath.Join(destination, "assets", "Dockerfile.template")); err != nil {
					t.Fatal("missing development container Dockerfile")
				}
			}
			for _, relative := range []string{
				"LICENSE",
				filepath.Join("assets", "prepare-continue.py"),
				filepath.Join("assets", "install-continue.js"),
				filepath.Join("assets", "setup-continue.sh"),
				"THIRD-PARTY-NOTICES.txt",
				filepath.Join("scripts", "setup-project.sh"),
				filepath.Join("assets", test.runtime),
				filepath.Join("assets", "install-coding-agents.sh"),
				filepath.Join("assets", ".instructions.md"),
			} {
				info, err := os.Stat(filepath.Join(destination, relative))
				if err != nil {
					t.Fatalf("missing embedded file %s: %v", relative, err)
				}
				if filepath.Ext(relative) == ".sh" && info.Mode()&0111 == 0 {
					t.Fatalf("embedded script is not executable: %s", relative)
				}
			}
		})
	}
}
