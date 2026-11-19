package startui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectAccessCreatesAndChecksFolder(t *testing.T) {
	project := filepath.Join(t.TempDir(), "new project", "workspace")
	if err := CheckProjectAccess(project); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(project)
	if err != nil || len(entries) != 0 {
		t.Fatalf("access probe was not cleaned: %v, %v", entries, err)
	}
	if err := CheckProjectAccess("relative/path"); err == nil {
		t.Fatal("accepted relative project")
	}
	file := filepath.Join(project, "file")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := CheckProjectAccess(file); err == nil || !strings.Contains(err.Error(), file) || !strings.Contains(err.Error(), "Browse") {
		t.Fatalf("missing useful access error: %v", err)
	}
}

func TestProjectAccessDetectsUnreadableFolder(t *testing.T) {
	project := t.TempDir()
	if err := os.Chmod(project, 0000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(project, 0700)
	if _, err := os.ReadDir(project); err == nil {
		t.Skip("runner can bypass directory permissions")
	}
	if err := CheckProjectAccess(project); err == nil || !strings.Contains(err.Error(), project) {
		t.Fatalf("missing folder error: %v", err)
	}
}

func TestProjectAccessDoesNotRequireHostGit(t *testing.T) {
	project := t.TempDir()
	skills := filepath.Join(project, "skills", ".git")
	if err := os.MkdirAll(skills, 0755); err != nil {
		t.Fatal(err)
	}
	// Git configuration is checked later, inside sbx; no host tool is needed
	// while reviewing or selecting a project folder.
	if err := os.WriteFile(filepath.Join(skills, "config"), []byte("[broken\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	if err := CheckProjectAccess(project); err != nil {
		t.Fatal(err)
	}
}
