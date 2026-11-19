package startui

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func RecommendedProjectRoot() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "sandboxes")
}

type projectLocation struct {
	Existing           *ProjectState `json:"existing,omitempty"`
	Path               string        `json:"path"`
	Recommended        bool          `json:"recommended"`
	RecommendedProject string        `json:"recommendedProject"`
}

func describeProjectLocation(root, project, slug string) (projectLocation, error) {
	if !filepath.IsAbs(project) {
		return projectLocation{}, fmt.Errorf("Choose an absolute project folder.")
	}
	if _, err := FindSandbox(slug); err != nil {
		return projectLocation{}, err
	}
	root, project = filepath.Clean(root), filepath.Clean(project)
	resolvedRoot, resolvedProject := resolveExistingParent(root), resolveExistingParent(project)
	if runtime.GOOS == "windows" {
		resolvedRoot, resolvedProject = strings.ToLower(resolvedRoot), strings.ToLower(resolvedProject)
	}
	rel, err := filepath.Rel(resolvedRoot, resolvedProject)
	inside := err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	return projectLocation{ExistingProject(project), project, inside, filepath.Join(root, slug+"-project")}, nil
}
