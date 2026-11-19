package windowslauncher

import (
	"github.com/mpg-age-bioinformatics/vscode-sandboxes/internal/startui"
	"testing"
)

func TestUnifiedDispatchConfigurations(t *testing.T) {
	for _, item := range startui.Catalog() {
		c, err := configForSandbox(item.Slug)
		if err != nil {
			t.Fatal(err)
		}
		if c.SandboxSlug != item.Slug || c.ShellLauncher != "run-"+item.Slug+".sh" {
			t.Fatal(c)
		}
		if !invokedAsProjectRunner(c, `C:\project\code\`+c.ProjectRunner) {
			t.Fatal("unified project runner cannot resolve", c)
		}
		if item.Slug == "bioinformatics-sandbox" && (c.NeedsVersion || c.NeedsPodman) {
			t.Fatal("general sandbox acquired language dependencies")
		}
	}
	if _, err := configForSandbox("bad"); err == nil {
		t.Fatal("accepted unknown sandbox")
	}
}
