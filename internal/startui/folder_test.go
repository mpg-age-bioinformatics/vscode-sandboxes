package startui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFolderPickerStartsAtExistingAncestor(t *testing.T) {
	root := t.TempDir()
	existing := filepath.Join(root, "projects with spaces and ü")
	if err := os.Mkdir(existing, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(existing, "file.txt")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, proposed := range []string{existing, filepath.Join(existing, "new", "project"), file} {
		if got := folderStart(proposed); got != existing {
			t.Fatalf("folderStart(%q) = %q, want %q", proposed, got, existing)
		}
	}
	home, _ := os.UserHomeDir()
	if got := folderStart(""); got != home {
		t.Fatalf("empty path starts at %q, want home %q", got, home)
	}
}
