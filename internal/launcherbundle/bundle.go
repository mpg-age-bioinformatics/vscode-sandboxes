package launcherbundle

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed all:bundles
var bundles embed.FS

// Materialize writes one sandbox's self-contained setup bundle to destination.
func Materialize(slug, destination string) error {
	bundle, err := fs.Sub(bundles, "bundles/"+slug)
	if err != nil {
		return fmt.Errorf("open embedded %s setup bundle: %w", slug, err)
	}
	return fs.WalkDir(bundle, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		target := filepath.Join(destination, filepath.FromSlash(path))
		if entry.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		data, err := fs.ReadFile(bundle, path)
		if err != nil {
			return err
		}
		mode := os.FileMode(0644)
		if strings.HasSuffix(path, ".sh") || strings.Contains(path, "/MacOS/") {
			mode = 0755
		}
		if err := os.WriteFile(target, data, mode); err != nil {
			return err
		}
		return nil
	})
}
