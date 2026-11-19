package startui

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestDetectMissingAndNewlyInstalledPrerequisitesWithoutPATHRestart(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PATH", t.TempDir())
	t.Setenv("LOCALAPPDATA", root)
	t.Setenv("LocalAppData", root)
	t.Setenv("ProgramFiles", t.TempDir())
	status := detectPrerequisitesForPlatform("windows", "amd64")
	if status.Docker || status.VSCode || status.Ready || !status.Supported {
		t.Fatal(status)
	}
	docker := filepath.Join(root, "DockerSandboxes", "bin", "sbx.exe")
	os.MkdirAll(filepath.Dir(docker), 0755)
	os.WriteFile(docker, []byte("fixture"), 0755)
	status = detectPrerequisitesForPlatform("windows", "amd64")
	if !status.Docker || status.VSCode || status.Ready {
		t.Fatal(status)
	}
	vscode := filepath.Join(root, "Programs", "Microsoft VS Code")
	os.MkdirAll(filepath.Join(vscode, "resources", "app", "out"), 0755)
	os.WriteFile(filepath.Join(vscode, "Code.exe"), []byte("fixture"), 0755)
	os.WriteFile(filepath.Join(vscode, "resources", "app", "out", "cli.js"), []byte("fixture"), 0644)
	if status = detectPrerequisitesForPlatform("windows", "amd64"); !status.Ready {
		t.Fatal(status)
	}
	os.Remove(docker)
	if status = detectPrerequisitesForPlatform("windows", "amd64"); status.Ready || status.Docker {
		t.Fatal("removal was not detected", status)
	}
}

func TestVerifiedInstallerDownloadAndOfflineCache(t *testing.T) {
	body := "vendor installer fixture"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, body) }))
	checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(body)))
	artifact := prerequisiteDownload{server.URL, checksum, "fixture-installer"}
	cache := t.TempDir()
	path, err := downloadPrerequisite(context.Background(), server.Client(), artifact, cache, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	server.Close()
	cached, err := downloadPrerequisite(context.Background(), server.Client(), artifact, cache, io.Discard)
	if err != nil || cached != path {
		t.Fatal("valid offline cache was not reused", cached, err)
	}
	os.WriteFile(path, []byte("tampered"), 0600)
	if _, err = downloadPrerequisite(context.Background(), server.Client(), artifact, cache, io.Discard); err == nil {
		t.Fatal("trusted a corrupted cache")
	}
}

func TestInstallerChecksumFailureDoesNotPublishDownload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "wrong bytes") }))
	defer server.Close()
	cache := t.TempDir()
	artifact := prerequisiteDownload{server.URL, strings.Repeat("0", 64), "fixture.exe"}
	if _, err := downloadPrerequisite(context.Background(), server.Client(), artifact, cache, io.Discard); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatal(err)
	}
	files, _ := os.ReadDir(cache)
	if len(files) != 0 {
		t.Fatal("retained unverified installer", files)
	}
}

func TestPinnedInstallerDefinitions(t *testing.T) {
	for _, component := range []string{"docker", "vscode"} {
		for _, platform := range []string{"darwin", "windows"} {
			artifact, err := prerequisiteArtifact(component, platform)
			if err != nil || !strings.HasPrefix(artifact.URL, "https://") || len(artifact.SHA256) != 64 {
				t.Fatal(artifact, err)
			}
		}
	}
	if _, err := prerequisiteArtifact("arbitrary-url", "windows"); err == nil {
		t.Fatal("accepted arbitrary installer source")
	}
}

func TestMacDockerExtractionKeepsRuntimeLayoutAndPreservesExistingInstall(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS extraction tools")
	}
	isolatedSettings(t)
	archive := filepath.Join(t.TempDir(), "fixture.tar.gz")
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	gzipWriter := gzip.NewWriter(file)
	writer := tar.NewWriter(gzipWriter)
	for _, name := range []string{"bin/sbx", "libexec/nerdbox-kernel-arm64"} {
		data := []byte("fixture")
		writer.WriteHeader(&tar.Header{Name: name, Mode: 0755, Size: int64(len(data))})
		writer.Write(data)
	}
	writer.Close()
	gzipWriter.Close()
	file.Close()
	if err := installMacPrerequisite(context.Background(), "docker", archive, io.Discard); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(settingsDir(), "tools", "docker-sandboxes-0.39.0")
	if _, err := os.Stat(filepath.Join(target, "libexec", "nerdbox-kernel-arm64")); err != nil {
		t.Fatal("runtime files were not kept beside CLI", err)
	}
	if err := installMacPrerequisite(context.Background(), "docker", archive, io.Discard); err == nil {
		t.Fatal("replaced an existing installation")
	}
}

func TestPrerequisiteInstallerSerializesAndReportsFailure(t *testing.T) {
	finish := make(chan struct{})
	setup := &prerequisiteSetup{
		detect: func() Prerequisites { return Prerequisites{Supported: true} },
		install: func(ctx context.Context, log io.Writer) error {
			fmt.Fprintln(log, "Downloading fixture")
			<-finish
			return fmt.Errorf("fixture install failed")
		},
	}
	if err := setup.start(); err != nil {
		t.Fatal(err)
	}
	if !setup.status().Installing || setup.status().Ready {
		t.Fatal("installation did not gate startup")
	}
	if err := setup.start(); err == nil {
		t.Fatal("allowed concurrent installers")
	}
	close(finish)
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		status := setup.status()
		if !status.Installing {
			if !strings.Contains(status.Error, "fixture install failed") || !strings.Contains(status.Log, "Downloading fixture") {
				t.Fatal(status)
			}
			return
		}
	}
	t.Fatal("installer did not finish")
}
