package startui

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRecommendedProjectLocations(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "sandboxes")
	for _, test := range []struct {
		path        string
		recommended bool
	}{
		{filepath.Join(root, "python-sandbox-project"), true},
		{root, true},
		{filepath.Join(home, "sandboxes-other", "project"), false},
		{filepath.Join(home, "Desktop", "project"), false},
		{filepath.Join(root, "..", "Documents", "project"), false},
	} {
		got, err := describeProjectLocation(root, test.path, "r-sandbox")
		if err != nil || got.Recommended != test.recommended || got.RecommendedProject != filepath.Join(root, "r-sandbox-project") {
			t.Fatalf("%s: %+v, %v", test.path, got, err)
		}
	}
	if _, err := describeProjectLocation(root, "relative", "python-sandbox"); err == nil {
		t.Fatal("accepted relative path")
	}
	if _, err := describeProjectLocation(root, home, "invalid"); err == nil {
		t.Fatal("accepted invalid type")
	}
}

func TestProjectLocationFollowsExistingSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs additional Windows privileges")
	}
	home := t.TempDir()
	root, outside := filepath.Join(home, "sandboxes"), filepath.Join(home, "Desktop")
	for _, folder := range []string{root, outside} {
		if err := os.Mkdir(folder, 0700); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(root, "linked-desktop")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	got, err := describeProjectLocation(root, filepath.Join(link, "new-project"), "python-sandbox")
	if err != nil || got.Recommended {
		t.Fatalf("symlink escape was recommended: %+v, %v", got, err)
	}
}
