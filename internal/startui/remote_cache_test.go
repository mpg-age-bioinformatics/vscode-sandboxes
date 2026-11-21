package startui

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func cacheFixture(t *testing.T) string {
	t.Helper()
	remoteTestHome(t)
	root := filepath.Join(settingsDir(), "remote-images")
	for _, name := range []string{strings.Repeat("a", 64), strings.Repeat("b", 64)} {
		if err := os.MkdirAll(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for name, data := range map[string][]byte{
		filepath.Join(strings.Repeat("a", 64), "image.tar"):         bytes.Repeat([]byte("cache archive bytes"), 8192),
		filepath.Join(strings.Repeat("a", 64), "Dockerfile"):        []byte("FROM test"),
		filepath.Join(strings.Repeat("b", 64), "image.tar.partial"): []byte("incomplete build"),
	} {
		if err := os.WriteFile(filepath.Join(root, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestRemoteCacheUsageAndClearPreserveUnrelatedData(t *testing.T) {
	root := cacheFixture(t)
	outside := t.TempDir()
	precious := filepath.Join(outside, "project.txt")
	os.WriteFile(precious, []byte("keep project data"), 0600)
	unknown := filepath.Join(root, "notes")
	os.WriteFile(unknown, []byte("keep unrelated root entry"), 0600)
	key := filepath.Join(settingsDir(), "remote", "session-key")
	os.MkdirAll(filepath.Dir(key), 0700)
	os.WriteFile(key, []byte("keep allocation key"), 0600)
	if err := os.Symlink(outside, filepath.Join(root, strings.Repeat("a", 64), "outside")); err != nil && runtime.GOOS != "windows" {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, strings.Repeat("c", 64))); err != nil && runtime.GOOS != "windows" {
		t.Fatal(err)
	}
	before, err := localRemoteCacheUsage(context.Background())
	if err != nil || !before.Measured || before.Entries != 2 || before.Files != 3 || before.InUse {
		t.Fatal(before, err)
	}
	after, err := clearLocalRemoteCache(context.Background())
	if err != nil || !after.Measured || after.Bytes != 0 || after.Entries != 0 || after.Files != 0 || after.RemovedBytes != before.Bytes {
		t.Fatal(before, after, err)
	}
	for file, want := range map[string]string{precious: "keep project data", unknown: "keep unrelated root entry", key: "keep allocation key"} {
		data, err := os.ReadFile(file)
		if err != nil || string(data) != want {
			t.Fatal("changed unrelated data", file, err)
		}
	}
	if _, err := os.Stat(filepath.Join(settingsDir(), "remote-image-cache.lock")); err != nil {
		t.Fatal("deleted maintenance lock", err)
	}
}

func TestRemoteCacheLeasesProtectBuildsAndUploads(t *testing.T) {
	root := cacheFixture(t)
	release, err := lockRemoteCache(false)
	if err != nil {
		t.Fatal(err)
	}
	released := false
	defer func() {
		if !released {
			release()
		}
	}()
	usage, err := localRemoteCacheUsage(context.Background())
	if err != nil || !usage.InUse || !usage.Measured {
		t.Fatal(usage, err)
	}
	if _, err := clearLocalRemoteCache(context.Background()); !errors.Is(err, errRemoteCacheInUse) {
		t.Fatal("removed in-use cache", err)
	}
	if _, err := os.Stat(filepath.Join(root, strings.Repeat("a", 64), "image.tar")); err != nil {
		t.Fatal(err)
	}
	release()
	released = true
	if _, err := clearLocalRemoteCache(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Cleanup also excludes new image preparation in another launcher.
	release, err = lockRemoteCache(true)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	usage, err = localRemoteCacheUsage(context.Background())
	if err != nil || !usage.InUse || usage.Measured {
		t.Fatal(usage, err)
	}
	profile := remoteTestProfile()
	profile.ImageSource = "python"
	profile.Version = "3.13"
	if _, err := prepareRemoteImage(context.Background(), profile, strings.Repeat("a", 32), &bytes.Buffer{}); !errors.Is(err, errRemoteCacheInUse) {
		t.Fatal("preparation ignored maintenance lock", err)
	}
}

func TestRemoteCacheRejectsSymlinkRoot(t *testing.T) {
	remoteTestHome(t)
	os.MkdirAll(settingsDir(), 0700)
	outside := t.TempDir()
	target := filepath.Join(outside, strings.Repeat("a", 64))
	os.MkdirAll(target, 0700)
	os.WriteFile(filepath.Join(target, "keep"), []byte("project"), 0600)
	if err := os.Symlink(outside, filepath.Join(settingsDir(), "remote-images")); err != nil {
		t.Skip(err)
	}
	if _, err := localRemoteCacheUsage(context.Background()); err == nil {
		t.Fatal("followed linked cache root")
	}
	if _, err := clearLocalRemoteCache(context.Background()); err == nil {
		t.Fatal("cleared linked directory")
	}
	if data, err := os.ReadFile(filepath.Join(target, "keep")); err != nil || string(data) != "project" {
		t.Fatal("deleted symlink target", err)
	}
}

func TestRemoteCacheEmptyAndSparseStorage(t *testing.T) {
	remoteTestHome(t)
	usage, err := localRemoteCacheUsage(context.Background())
	if err != nil || !usage.Measured || usage.Bytes != 0 {
		t.Fatal(usage, err)
	}
	if _, err := clearLocalRemoteCache(context.Background()); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		return
	} // Native Windows allocation semantics need a Windows runner.
	root := filepath.Join(settingsDir(), "remote-images", strings.Repeat("a", 64))
	os.MkdirAll(root, 0700)
	file, err := os.Create(filepath.Join(root, "image.tar.partial"))
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(1 << 30); err != nil {
		file.Close()
		t.Fatal(err)
	}
	file.Close()
	usage, err = localRemoteCacheUsage(context.Background())
	if err != nil || usage.Files != 1 || usage.Bytes >= 1<<30 {
		t.Fatal("reported sparse logical size as disk usage", usage, err)
	}
}

func TestRemoteCacheHTTPGuardsAndActiveAllocation(t *testing.T) {
	cacheFixture(t)
	p := &remoteController{state: RemoteStatus{Session: RemoteSession{Active: true, JobID: "77", Token: strings.Repeat("a", 32)}}}
	p.ssh = func(context.Context, string, string, ...string) (string, error) {
		t.Fatal("cache cleanup contacted HPC")
		return "", nil
	}
	before := p.state.Session
	call := func(route, method string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		p.serve(route, w, httptest.NewRequest(method, "/"+route, nil), &logState{}, &prerequisiteSetup{})
		return w
	}
	if w := call("remote/cache/clear", http.MethodGet); w.Code != 405 {
		t.Fatal(w.Code)
	}
	p.state.Running = true
	if w := call("remote/cache/clear", http.MethodPost); w.Code != 409 {
		t.Fatal(w.Code)
	}
	p.state.Running = false
	w := call("remote/cache", http.MethodGet)
	var usage remoteCacheStats
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &usage) != nil || usage.Entries != 2 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, w.Body.String())
	}
	w = call("remote/cache/clear", http.MethodPost)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if !reflect.DeepEqual(before, p.state.Session) || p.busy() {
		t.Fatal("cleanup changed allocation or left launcher busy")
	}
}

func TestRemoteCacheLeaseAcrossProcesses(t *testing.T) {
	if os.Getenv("VSCODE_TEST_CACHE_CHILD") == "1" {
		release, err := lockRemoteCache(false)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		fmt.Println("LOCKED")
		_, _ = io.Copy(io.Discard, os.Stdin)
		return
	}
	cacheFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRemoteCacheLeaseAcrossProcesses$")
	command.Env = append(os.Environ(), "VSCODE_TEST_CACHE_CHILD=1")
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	}()
	line, err := bufio.NewReader(output).ReadString('\n')
	if err != nil || line != "LOCKED\n" {
		t.Fatal("child did not lock cache", line, err, stderr.String())
	}
	if _, err := clearLocalRemoteCache(context.Background()); !errors.Is(err, errRemoteCacheInUse) {
		t.Fatal("ignored another process's image-cache lease", err)
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = command.Wait()
	waited = true
	if _, err := clearLocalRemoteCache(context.Background()); err != nil {
		t.Fatal("crashed launcher left cache locked", err)
	}
}
