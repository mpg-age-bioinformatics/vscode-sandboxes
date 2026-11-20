package startui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func localCatalogFixture(t *testing.T) (*remoteController, RemoteProfile, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX Python and bash")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("Python unavailable")
	}
	root := remoteTestHome(t)
	bin := filepath.Join(root, "bin")
	os.MkdirAll(bin, 0700)
	os.WriteFile(filepath.Join(bin, "squeue"), []byte("#!/bin/bash\nprintf '%s' \"$CATALOG_QUEUE\"\n"), 0700)
	os.WriteFile(filepath.Join(bin, "scancel"), []byte("#!/bin/bash\nprintf '%s' \"$1\" > \"$CATALOG_CANCEL\"\n"), 0700)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CATALOG_CANCEL", filepath.Join(root, "cancelled"))
	t.Setenv("CATALOG_QUEUE", "")
	profile := defaultRemoteProfile()
	profile.Login = "fixture"
	profile.ImagesDir = filepath.Join(root, "images")
	profile.StateDir = filepath.Join(root, "state")
	profile.Project = filepath.Join(root, "project")
	profile.ImageSource = "python"
	profile.Version = "3.13"
	controller := &remoteController{ssh: func(ctx context.Context, host, script string, args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "bash", append([]string{"-s", "--"}, args...)...)
		cmd.Stdin = strings.NewReader(script)
		var stderr strings.Builder
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			return string(out), fmt.Errorf("%w: %s", err, stderr.String())
		}
		return string(out), nil
	}}
	return controller, profile, root
}

func TestRemoteServerCacheSkipsImageTransfer(t *testing.T) {
	p, profile, root := localCatalogFixture(t)
	ctx := context.Background()
	record := remoteImageRecord{ID: strings.Repeat("a", 64), RecipeKey: remoteImageIdentity(profile), Source: "python", Version: "3.13", Architecture: "amd64", Recipe: "FROM mcr.microsoft.com/devcontainers/python:3.13-bookworm\n", Lock: json.RawMessage(`{"imageId":"fixture"}`)}
	var reserved remoteImageRecord
	if err := p.catalog(ctx, profile, "reserve", record, &reserved); err != nil {
		t.Fatal(err)
	}
	var missing *remoteImageRecord
	if err := p.catalog(ctx, profile, "lookup", map[string]string{"recipeKey": record.RecipeKey}, &missing); err != nil || missing != nil {
		t.Fatal("pending image offered", missing, err)
	}
	os.WriteFile(reserved.Path, []byte("complete SIF fixture"), 0600)
	os.WriteFile(filepath.Join(filepath.Dir(reserved.Path), "ready"), []byte(record.ID+"\n"), 0600)
	s := RemoteSession{Profile: profile, Token: strings.Repeat("b", 32)}
	plan, hit, err := p.cachedRemotePlan(ctx, &s)
	if err != nil || !hit || plan.Archive != "" || plan.Image != reserved.Path || plan.Key != record.ID {
		t.Fatal("did not reuse remote SIF", plan, hit, err)
	}
	uploads := []string{}
	p.upload = func(_ context.Context, host, local, remote string) error {
		uploads = append(uploads, remote)
		data, err := os.ReadFile(local)
		if err != nil {
			return err
		}
		return os.WriteFile(remote, data, 0600)
	}
	if err := p.stageRemoteImage(ctx, s, &plan); err != nil {
		t.Fatal(err)
	}
	if len(uploads) != 1 || !strings.HasSuffix(uploads[0], "/project.tar") {
		t.Fatal("transferred an image for a cache hit", uploads)
	}
	var records []remoteImageRecord
	if err := p.catalog(ctx, profile, "images", nil, &records); err != nil || len(records) != 1 || records[0].State != "ready" {
		t.Fatal(records, err)
	}
	metadata, err := os.ReadFile(filepath.Join(root, "images", "metadata.json"))
	if err != nil || !strings.Contains(string(metadata), "imageId") {
		t.Fatal("metadata not retained", err)
	}
	// Explicit selection retains the preset and architecture from server metadata.
	s.Profile.ImageSource = "cached"
	s.Profile.CachedImage = record.ID
	s.Profile.Version = ""
	s.Token = strings.Repeat("c", 32)
	_, hit, err = p.cachedRemotePlan(ctx, &s)
	if err != nil || !hit || s.Profile.CachedSource != "python" || s.Profile.Version != "3.13" {
		t.Fatal(s.Profile, err)
	}
	os.Remove(filepath.Join(filepath.Dir(reserved.Path), "ready"))
	if _, _, err := p.cachedRemotePlan(ctx, &s); err == nil {
		t.Fatal("selected an incomplete cache entry")
	}
}

func TestRemoteAllocationInventoryOwnershipAndAuthorization(t *testing.T) {
	p, profile, root := localCatalogFixture(t)
	ctx := context.Background()
	token := strings.Repeat("d", 32)
	run := filepath.Join(profile.StateDir, "runs", token)
	os.MkdirAll(run, 0700)
	for name, data := range map[string]string{"owner-token": token, "job-id": "42", "profile.json": remoteProfileJSON(profile), "authorized_keys": "ssh-ed25519 AAAA original\n"} {
		os.WriteFile(filepath.Join(run, name), []byte(data), 0600)
	}
	t.Setenv("CATALOG_QUEUE", "42|RUNNING|node-display-only|vscode-sandboxes:"+token+"\n77|RUNNING|other|analysis-job\n")
	sessions, err := p.listRemoteSessions(ctx, profile)
	if err != nil || len(sessions) != 1 || sessions[0].JobID != "42" {
		t.Fatal(sessions, err)
	}
	var result RemoteSession
	if err := p.catalog(ctx, profile, "authorize", map[string]string{"token": token, "jobID": "42", "publicKey": "ssh-ed25519 AQID new-laptop"}, &result); err != nil {
		t.Fatal(err)
	}
	keys, _ := os.ReadFile(filepath.Join(run, "authorized_keys"))
	if !strings.Contains(string(keys), "original") || !strings.Contains(string(keys), "new-laptop") {
		t.Fatal(string(keys))
	}
	if err := p.catalog(ctx, profile, "stop", map[string]string{"token": token, "jobID": "77"}, &result); err == nil {
		t.Fatal("cancelled an unrelated job")
	}
	if err := p.catalog(ctx, profile, "stop", map[string]string{"token": token, "jobID": "42"}, &result); err != nil {
		t.Fatal(err)
	}
	cancelled, _ := os.ReadFile(filepath.Join(root, "cancelled"))
	if string(cancelled) != "42" {
		t.Fatal(string(cancelled))
	}
	t.Setenv("CATALOG_QUEUE", "42|RUNNING|node|vscode-sandboxes:"+strings.Repeat("e", 32)+"\n")
	if err := p.catalog(ctx, profile, "authorize", map[string]string{"token": token, "jobID": "42", "publicKey": "ssh-ed25519 AQID"}, &result); err == nil {
		t.Fatal("authorized a reused job ID")
	}
}

func TestRemoteConcurrentConversionsShareOneSIF(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX shell and flock fixture")
	}
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	os.MkdirAll(bin, 0700)
	os.WriteFile(filepath.Join(bin, "flock"), []byte("#!/bin/bash\nexec python3 -c 'import fcntl,sys;fcntl.flock(int(sys.argv[1]),fcntl.LOCK_EX)' \"$2\"\n"), 0700)
	os.WriteFile(filepath.Join(bin, "getent"), []byte("#!/bin/bash\necho researcher:x:1000:1000::/home/researcher:/bin/bash\n"), 0700)
	os.WriteFile(filepath.Join(bin, "apptainer"), []byte("#!/bin/bash\nset -eu\nif [[ $1 == build ]]; then echo build >> "+remoteQuote(filepath.Join(root, "calls"))+"; sleep 0.2; printf 'complete sif' > \"$2\"; fi\n"), 0700)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CONVERSION_CALLS", filepath.Join(root, "calls"))
	image := filepath.Join(root, "image.sif")
	key := strings.Repeat("f", 64)
	project := filepath.Join(root, "project")
	os.MkdirAll(project, 0700)
	results := make(chan error, 2)
	for _, job := range []string{"101", "102"} {
		run := filepath.Join(root, "run"+job)
		stage := filepath.Join(root, "stage"+job)
		os.MkdirAll(run, 0700)
		os.MkdirAll(stage, 0700)
		os.WriteFile(filepath.Join(run, "modules.sh"), nil, 0600)
		os.WriteFile(filepath.Join(run, "profile.json"), []byte(`{"network":"open"}`), 0600)
		os.WriteFile(filepath.Join(stage, "image.tar"), []byte("archive"), 0600)
		go func(job, run, stage string) {
			cmd := exec.Command("bash", "-s", "--", run, filepath.Join(root, "state"), project, "", image, "apptainer", "0", stage, key)
			cmd.Env = append(os.Environ(), "SLURM_JOB_ID="+job)
			cmd.Stdin = strings.NewReader(remoteAsset("job.sh"))
			out, err := cmd.CombinedOutput()
			if err != nil {
				err = fmt.Errorf("%w: %s", err, out)
			}
			results <- err
		}(job, run, stage)
	}
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	calls, _ := os.ReadFile(filepath.Join(root, "calls"))
	if string(calls) != "build\n" {
		t.Fatal("converted a shared image more than once", string(calls))
	}
	marker, _ := os.ReadFile(filepath.Join(root, "ready"))
	if strings.TrimSpace(string(marker)) != key {
		t.Fatal("missing atomic readiness marker", string(marker))
	}
	for _, job := range []string{"101", "102"} {
		if _, err := os.Stat(filepath.Join(root, "stage"+job, "image.tar")); !os.IsNotExist(err) {
			t.Fatal("retained uploaded archive", err)
		}
	}
}
