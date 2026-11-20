package startui

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// Explicit opt-in: this test pulls images and builds inside a temporary Docker
// Sandbox. It never connects to an HPC or submits a Slurm allocation.
func TestRemoteLiveImageBuild(t *testing.T) {
	if os.Getenv("VSCODE_REMOTE_LIVE_BUILD") != "1" {
		t.Skip("set VSCODE_REMOTE_LIVE_BUILD=1 to run an actual local image build")
	}
	p := remoteTestProfile()
	p.ImageSource = "python"
	p.Version = "3.13"
	p.Architecture = "amd64"
	if source := os.Getenv("VSCODE_REMOTE_LIVE_PRESET"); source != "" {
		p.ImageSource = source
	}
	if p.ImageSource == "r" {
		p.Version = "4.5"
	}
	if p.ImageSource == "bioinformatics" {
		p.Version = ""
	}
	if version := os.Getenv("VSCODE_REMOTE_LIVE_VERSION"); version != "" {
		p.Version = version
	}
	if arch := os.Getenv("VSCODE_REMOTE_LIVE_ARCH"); arch != "" {
		p.Architecture = arch
	}
	token := fmt.Sprintf("%032x", time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	plan, err := prepareRemoteImage(ctx, p, token, os.Stdout)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Built local archive: %+v", plan)
}
