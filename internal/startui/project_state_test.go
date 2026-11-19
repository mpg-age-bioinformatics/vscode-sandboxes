package startui

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func stateFixture(t *testing.T) Selection {
	t.Helper()
	isolatedSettings(t)
	project := t.TempDir()
	write := func(name, text string) {
		path := filepath.Join(project, name)
		os.MkdirAll(filepath.Dir(path), 0755)
		if err := os.WriteFile(path, []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("code/.sandbox/run-python-sandbox.sh", "#!/bin/sh\necho reused\n")
	write("code/.sandbox/sandbox-runtime.json", `{"language":"python","series":"3.13"}`)
	write("code/user.py", "print('initial')\n")
	for _, args := range [][]string{{"init"}, {"config", "user.name", "Test"}, {"config", "user.email", "test@localhost"}, {"add", "."}, {"commit", "-m", "Initial setup"}} {
		if _, err := projectGit(project, args...); err != nil {
			t.Fatal(err)
		}
	}
	s := selection()
	s.Project, s.Sandbox = project, "python-sandbox"
	return s
}

func TestInitialSetupFinalizedBeforeEditorWithSingleCleanCommit(t *testing.T) {
	s := stateFixture(t)
	s.FreshRepository = true
	lock := filepath.Join(s.Project, "code/.sandbox/container-image.lock.json")
	os.WriteFile(lock, []byte(`{"imageId":"pinned"}`), 0644)
	if err := finalizeProject(s, "python-fixture-dev", io.Discard); err != nil {
		t.Fatal(err)
	}
	count, _ := projectGit(s.Project, "rev-list", "--count", "HEAD")
	if count != "1" {
		t.Fatal("expected one initial commit", count)
	}
	dirty, _ := projectGit(s.Project, "status", "--porcelain")
	if dirty != "" {
		t.Fatal("setup left changed files", dirty)
	}
	state := ExistingProject(s.Project)
	if state == nil || !state.Ready || state.SandboxName != "python-fixture-dev" {
		t.Fatal("missing completed-project record", state)
	}
	// Later restarts preserve both staged work and untracked user code.
	os.WriteFile(filepath.Join(s.Project, "code/user.py"), []byte("user edit\n"), 0644)
	projectGit(s.Project, "add", "code/user.py")
	os.WriteFile(filepath.Join(s.Project, "code/new.py"), []byte("new user file\n"), 0644)
	before, _ := projectGit(s.Project, "status", "--porcelain")
	head, _ := projectGit(s.Project, "rev-parse", "HEAD")
	if err := finalizeProject(s, "python-fixture-dev", io.Discard); err != nil {
		t.Fatal(err)
	}
	after, _ := projectGit(s.Project, "status", "--porcelain")
	next, _ := projectGit(s.Project, "rev-parse", "HEAD")
	if before != after || head != next {
		t.Fatal("restart changed user work or history")
	}
	// Folder selection restores the recorded language/version, not UI defaults.
	chosen := s
	chosen.Sandbox, chosen.Version = "r-sandbox", "4.5"
	chosen, err := resolveSelection(Config{Sandboxes: Catalog()}, chosen)
	if err != nil || chosen.Sandbox != "python-sandbox" || chosen.Version != "3.13" {
		t.Fatal("did not recover project settings", chosen, err)
	}
}

func TestFinalizationPreservesExistingUserIndex(t *testing.T) {
	s := stateFixture(t)
	os.WriteFile(filepath.Join(s.Project, "code/user.py"), []byte("staged user edit\n"), 0644)
	projectGit(s.Project, "add", "code/user.py")
	os.WriteFile(filepath.Join(s.Project, "code/.sandbox/container-image.lock.json"), []byte(`{"imageId":"pinned"}`), 0644)
	if err := finalizeProject(s, "fixture", io.Discard); err != nil {
		t.Fatal(err)
	}
	staged, _ := projectGit(s.Project, "diff", "--cached", "--name-only")
	if staged != "code/user.py" {
		t.Fatal("changed unrelated staging", staged)
	}
	committed, _ := projectGit(s.Project, "show", "HEAD:code/user.py")
	if !strings.Contains(committed, "initial") {
		t.Fatal("committed user edits")
	}
}

func TestRecreatedProjectCannotReuseOldSuccess(t *testing.T) {
	s := stateFixture(t)
	if err := rememberProject(s, "old-name"); err != nil {
		t.Fatal(err)
	}
	// Simulate replacing the repository while keeping its generated scripts.
	os.RemoveAll(filepath.Join(s.Project, ".git"))
	if existing := ExistingProject(s.Project); existing != nil {
		t.Fatal("reused the old success record for a new folder")
	}
}

func TestExportActivityRedactsCompleteLogAndErrors(t *testing.T) {
	state := &logState{secrets: []string{"secret-key"}}
	state.Write([]byte("request secret-"))
	state.Write([]byte("key failed\n"))
	state.Error = "retry secret-key"
	text := state.activityText()
	if strings.Contains(text, "secret-key") || !strings.Contains(text, "[redacted]") || !strings.Contains(text, "Error: retry") {
		t.Fatal("bad exported activity", text)
	}
}

func TestNewBaselinesForAllSandboxTypes(t *testing.T) {
	for _, slug := range []string{"python-sandbox", "r-sandbox", "bioinformatics-sandbox"} {
		t.Run(slug, func(t *testing.T) {
			s := stateFixture(t)
			s.Sandbox = slug
			s.FreshRepository = true
			script := ProjectScript(s.Project, slug)
			if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0755); err != nil {
				t.Fatal(err)
			}
			if slug == "r-sandbox" {
				os.WriteFile(filepath.Join(s.Project, "renv.lock"), []byte(`{"R":{"Version":"4.5"}}`), 0644)
			}
			if err := finalizeProject(s, "test", io.Discard); err != nil {
				t.Fatal(err)
			}
			dirty, _ := projectGit(s.Project, "status", "--porcelain")
			if dirty != "" {
				t.Fatal("new environment is dirty", dirty)
			}
			count, _ := projectGit(s.Project, "rev-list", "--count", "HEAD")
			if count != "1" {
				t.Fatal("initial setup has extra commits", count)
			}
		})
	}
}

func TestSuccessRecordCannotReuseReplacedDirectoryIdentity(t *testing.T) {
	s := stateFixture(t)
	if err := rememberProject(s, "old-name"); err != nil {
		t.Fatal(err)
	}
	var state ProjectState
	if err := readJSON(projectStatePath(s.Project), &state); err != nil {
		t.Fatal(err)
	}
	state.DirectoryID = "replaced-directory"
	if err := writeJSON(projectStatePath(s.Project), state); err != nil {
		t.Fatal(err)
	}
	existing := ExistingProject(s.Project)
	if existing == nil || existing.Ready || existing.SandboxName != "" {
		t.Fatal("reused a stale sandbox after folder replacement", existing)
	}
}
