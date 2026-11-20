package startui

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRemoteBuildRejectsLowSpaceBeforeSandboxCreation(t *testing.T) {
	remoteTestHome(t)
	previous := remoteStorageAvailable
	remoteStorageAvailable = func(string) (uint64, error) { return 857 << 20, nil }
	t.Cleanup(func() { remoteStorageAvailable = previous })
	p := remoteTestProfile()
	p.ImageSource = "python"
	p.Version = "3.12"
	_, err := prepareRemoteImage(context.Background(), p, strings.Repeat("a", 32), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "0.8 GiB") || !strings.Contains(err.Error(), "20 GiB") {
		t.Fatal(err)
	}
}

func TestRemoteBuildStorageErrorClassification(t *testing.T) {
	for _, line := range []string{"failed to perform sync: input/output error", "no space left on device", "disk quota exceeded"} {
		var log bytes.Buffer
		output := remoteBuildOutput{target: &log}
		output.Write([]byte(line))
		err := output.failure(fmt.Errorf("exit status 1"))
		if !strings.Contains(err.Error(), "storage failed") || strings.Contains(err.Error(), "emulation") {
			t.Fatal(err)
		}
	}
}

func TestRemoteBuildStopsWhenHostReserveIsConsumed(t *testing.T) {
	previous := remoteStorageAvailable
	remoteStorageAvailable = func(string) (uint64, error) { return 1 << 30, nil }
	defer func() { remoteStorageAvailable = previous }()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	report := make(chan error, 1)
	done := make(chan struct{})
	go func() { defer close(done); watchRemoteBuildSpace(ctx, cancel, ".", report) }()
	select {
	case err := <-report:
		if !strings.Contains(err.Error(), "2 GiB") {
			t.Fatal(err)
		}
	case <-time.After(8 * time.Second):
		cancel()
		<-done
		t.Fatal("storage monitor did not stop the build")
	}
	<-done
	if ctx.Err() == nil {
		t.Fatal("storage monitor did not cancel its build")
	}
}

// Exercise the whole shell pipeline with representative registry descriptors and
// an actual Docker-format tar, without downloading layers or using host Docker.
func TestRemoteBuilderExportsArchiveWithoutDaemonCopies(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX shell")
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node unavailable")
	}
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	os.Mkdir(bin, 0700)
	os.WriteFile(filepath.Join(root, "Dockerfile"), []byte("FROM python:3.12\nCOPY --from=node:22.22.1-bookworm-slim /usr/local/bin/node /usr/local/bin/node\n"), 0600)
	config := []byte(`{"architecture":"amd64","os":"linux","config":{},"rootfs":{"type":"layers","diff_ids":[]}}`)
	f, err := os.Create(filepath.Join(root, "fixture.tar"))
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(f)
	for name, data := range map[string][]byte{"manifest.json": []byte(`[{"Config":"config.json","RepoTags":["vscode-sandboxes-remote:workspace"],"Layers":[]}]`), "config.json": config} {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(data))})
		tw.Write(data)
	}
	tw.Close()
	f.Close()
	fixture := `#!/usr/bin/env python3
import sys,json,pathlib,shutil
root=pathlib.Path(__file__).parent.parent
args=sys.argv[1:]
with (root/'calls').open('a') as stream:stream.write(json.dumps(args)+'\n')
if args[:1]==['info']:
 print(str(root) if 'DockerRootDir' in args[-1] else 'amd64')
elif args[:3]==['buildx','imagetools','inspect']:
 print(json.dumps({'digest':'sha256:'+'a'*64}))
elif args[:2]==['buildx','build']:
 shutil.copyfile(root/'fixture.tar',root/'image.tar.partial')
elif args[:2] not in (['buildx','create'],['buildx','rm']):
 raise SystemExit('Unexpected Docker command: '+str(args))
`
	os.WriteFile(filepath.Join(bin, "docker"), []byte(fixture), 0700)
	// Fixture-only disk statistics let the test run on a nearly full host.
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	shim := "#!/bin/sh\nif [ \"$1\" = - ] && [ \"$#\" = 3 ]; then cat >/dev/null; exit 0; fi\nexec " + remoteQuote(node) + " \"$@\"\n"
	os.WriteFile(filepath.Join(bin, "node"), []byte(shim), 0700)
	cmd := exec.Command("bash", "-s", "--", "amd64", "python:3.12")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	cmd.Stdin = strings.NewReader(remoteAsset("local-build.sh"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out), err)
	}
	var lock map[string]string
	data, _ := os.ReadFile(filepath.Join(root, "image-lock.json"))
	if json.Unmarshal(data, &lock) != nil || lock["architecture"] != "amd64" || !strings.HasPrefix(lock["imageId"], "sha256:") {
		t.Fatal(string(data))
	}
	if _, err := os.Stat(filepath.Join(root, "image.tar")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "image.tar.partial")); !os.IsNotExist(err) {
		t.Fatal("partial archive retained", err)
	}
	calls, _ := os.ReadFile(filepath.Join(root, "calls"))
	for _, bad := range []string{`"pull"`, `"save"`, `"--load"`, `"image", "inspect"`} {
		if bytes.Contains(calls, []byte(bad)) {
			t.Fatal("duplicate daemon storage", string(calls))
		}
	}
	if !bytes.Contains(calls, []byte("type=docker,dest=image.tar.partial")) {
		t.Fatal(string(calls))
	}
}
