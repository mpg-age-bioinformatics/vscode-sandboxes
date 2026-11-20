package startui

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mpg-age-bioinformatics/vscode-sandboxes/internal/launcherbundle"
)

func TestRemoteImageSourcesAndPortableRecipes(t *testing.T) {
	for _, source := range []string{"python", "r", "bioinformatics", "registry", "dockerfile", "sif"} {
		p := remoteTestProfile()
		p.ImageSource = source
		p.Version = "3.13"
		p.Architecture = "amd64"
		p.Dockerfile = "FROM ubuntu:24.04\nRUN printf 'custom source'\n"
		if source == "r" {
			p.Version = "4.5"
		}
		if source == "registry" {
			p.Image = "docker://docker.io/library/ubuntu:24.04"
		}
		if err := validateRemoteProfile(p); err != nil {
			t.Fatal(source, err)
		}
		if source == "sif" {
			plan, err := prepareRemoteImage(context.Background(), p, "", io.Discard)
			if err != nil || plan.Image != p.Image || plan.Archive != "" {
				t.Fatal(plan, err)
			}
			continue
		}
		recipe := remoteDockerfile(p)
		for _, required := range []string{"openssh-server", "@openai/codex@0.163.0", "COPY --from=node:", "USER root", "/run/sshd", "NODE_OPTIONS=--jitless", "getent passwd sshd"} {
			if !strings.Contains(recipe, required) {
				t.Fatal(source, required)
			}
		}
		if source == "dockerfile" && !strings.HasPrefix(recipe, p.Dockerfile) {
			t.Fatal("lost uploaded source")
		}
		files, err := remoteProjectFiles(p, recipe, []byte(`{"imageId":"fixture"}`))
		if err != nil {
			t.Fatal(err)
		}
		if string(files["code/Dockerfile"]) != recipe {
			t.Fatal("did not retain full recipe")
		}
		if source == "python" || source == "r" || source == "bioinformatics" {
			slug := remotePreset(p)
			for src, dst := range map[string]string{"settings.json": ".vscode/settings.json", "extensions.json": ".vscode/extensions.json", "gitignore": ".gitignore"} {
				local, _ := launcherbundle.ProjectAsset(slug, src)
				if !bytes.Equal(local, files[dst]) {
					t.Fatal("local/remote asset mismatch", source, src)
				}
			}
			if strings.Contains(string(files["AGENTS.md"]), "sbx exec") || !bytes.Contains(files["AGENTS.md"], []byte("Remote execution")) {
				t.Fatal("wrong execution policy")
			}
			if source == "python" && !strings.Contains(recipe, "FROM mcr.microsoft.com/devcontainers/python:3.13-bookworm") {
				t.Fatal("different Python base")
			}
			if source == "r" {
				local, _ := launcherbundle.ProjectAsset(slug, "setup-renv.R")
				if !bytes.Equal(local, files["code/.sandbox/setup-renv.R"]) {
					t.Fatal("different renv setup")
				}
			}
		}
		file := filepath.Join(t.TempDir(), "project.tar")
		if err := writeRemoteProjectTar(file, files); err != nil {
			t.Fatal(err)
		}
		f, _ := os.Open(file)
		reader := tar.NewReader(f)
		count := 0
		for {
			entry, err := reader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			if filepath.IsAbs(entry.Name) || strings.Contains(entry.Name, "..") {
				t.Fatal(entry.Name)
			}
			count++
		}
		f.Close()
		if count != len(files) {
			t.Fatal("missing files")
		}
	}
	for _, p := range []RemoteProfile{{ImageSource: "dockerfile", Dockerfile: "RUN whoami"}, {ImageSource: "registry", Image: "https://hub.docker.com/r/foo"}, {ImageSource: "python", Version: "3;touch bad"}, {ImageSource: "sif", Image: "docker://image"}, {ImageSource: "unknown"}, {ImageSource: "python", Version: "3.13", Architecture: "sparc"}} {
		if validateRemoteImage(p) == nil {
			t.Fatal("accepted invalid image source", p)
		}
	}
}

func TestRemoteImageStagingUploadsBeforeSubmit(t *testing.T) {
	remoteTestHome(t)
	s := RemoteSession{Profile: remoteTestProfile(), Token: strings.Repeat("d", 32)}
	localDir := t.TempDir()
	os.WriteFile(filepath.Join(localDir, "Dockerfile"), []byte("FROM ubuntu:24.04\n"), 0600)
	os.WriteFile(filepath.Join(localDir, "image-lock.json"), []byte(`{"imageId":"fixture"}`), 0600)
	plan := remoteImagePlan{Archive: filepath.Join(localDir, "image.tar"), Project: filepath.Join(localDir, "project.tar")}
	calls := []string{}
	p := &remoteController{ssh: func(_ context.Context, host, script string, args ...string) (string, error) {
		if strings.Contains(script, "VSCODE_CATALOG_PY") {
			return func() string {
				var record remoteImageRecord
				json.Unmarshal([]byte(args[3]), &record)
				record.Path = "/shared/images/fixture/image.sif"
				record.State = "preparing"
				data, _ := json.Marshal(record)
				return string(data)
			}(), nil
		}
		if script != remoteAsset("stage-image.sh") {
			t.Fatal("not staging")
		}
		return "STAGE=/shared/state/uploads/" + s.Token + "\n", nil
	}, upload: func(_ context.Context, host, local, remote string) error {
		calls = append(calls, local+":"+remote)
		return nil
	}}
	if err := p.stageRemoteImage(context.Background(), s, &plan); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || plan.Image != "/shared/images/fixture/image.sif" {
		t.Fatal(calls, plan)
	}
}

func TestRemoteLocalBuilderUsesSandboxDockerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX isolated command fixture")
	}
	remoteTestHome(t)
	previousStorage := remoteStorageAvailable
	remoteStorageAvailable = func(string) (uint64, error) { return 100 << 30, nil }
	t.Cleanup(func() { remoteStorageAvailable = previousStorage })
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "calls")
	t.Setenv("BUILDER_CALLS", log)
	// The sbx fixture executes no uploaded recipe; it creates representative
	// build outputs in the directory the launcher explicitly mounted.
	fixture := `#!/usr/bin/env python3
import json,os,sys
from pathlib import Path
args=sys.argv[1:]
with open(os.environ['BUILDER_CALLS'],'a') as f:f.write(json.dumps(args)+'\n')
if args[0]=='run':Path(os.environ['BUILDER_CALLS']+'.dir').write_text(args[-1])
if args[0]=='exec':
 sys.stdin.read()
 root=Path(Path(os.environ['BUILDER_CALLS']+'.dir').read_text())
 (root/'image.tar').write_bytes(b'image archive fixture')
 (root/'image-lock.json').write_text('{"imageId":"sha256:fixture","architecture":"amd64"}')
`
	os.WriteFile(filepath.Join(bin, "sbx"), []byte(fixture), 0700)
	// A host Docker invocation would fail the test immediately.
	os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\nexit 99\n"), 0700)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	p := remoteTestProfile()
	p.ImageSource = "python"
	p.Version = "3.13"
	plan, err := prepareRemoteImage(context.Background(), p, strings.Repeat("e", 32), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(plan.Archive); err != nil {
		t.Fatal(err)
	}
	calls, _ := os.ReadFile(log)
	remoteStorageAvailable = func(string) (uint64, error) { return 0, nil }
	cached, err := prepareRemoteImage(context.Background(), p, strings.Repeat("f", 32), io.Discard)
	if err != nil || cached != plan {
		t.Fatal("cache not reused", cached, err)
	}
	afterCache, _ := os.ReadFile(log)
	if !bytes.Equal(calls, afterCache) {
		t.Fatal("cache reuse invoked the builder")
	}
	remoteStorageAvailable = func(string) (uint64, error) { return 100 << 30, nil }
	os.WriteFile(plan.Archive, []byte("corrupted archive"), 0600)
	if _, err := prepareRemoteImage(context.Background(), p, strings.Repeat("f", 32), io.Discard); err != nil {
		t.Fatal(err)
	}
	afterRepair, _ := os.ReadFile(log)
	if bytes.Equal(afterCache, afterRepair) {
		t.Fatal("trusted corrupt cache")
	}
	if !bytes.Contains(calls, []byte(`"--sandbox", "remote-build-`)) || !bytes.Contains(calls, []byte(`"rm", "--force"`)) {
		t.Fatal("unscoped network or builder not cleaned", string(calls))
	}
}

func TestRemoteComputeConversionAndProjectInitialization(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires bash and python3")
	}
	root := remoteTestHome(t)
	bin := filepath.Join(root, "bin")
	os.MkdirAll(bin, 0700)
	run := filepath.Join(root, "run")
	state := filepath.Join(root, "state")
	project := filepath.Join(root, "project")
	stage := filepath.Join(root, "uploads")
	for _, dir := range []string{run, state, project, stage} {
		os.MkdirAll(dir, 0700)
	}
	os.WriteFile(filepath.Join(run, "modules.sh"), nil, 0600)
	os.WriteFile(filepath.Join(run, "profile.json"), []byte(`{"network":"open"}`), 0600)
	os.WriteFile(filepath.Join(stage, "image.tar"), []byte("archive"), 0600)
	fixture := "#!/bin/bash\nset -eu\nprintf '%s\\n' \"$*\" >> \"$COMPUTE_CALLS\"\nif [[ \"$1\" == build ]];then printf 'sif' > \"$2\";fi\n"
	os.WriteFile(filepath.Join(bin, "apptainer"), []byte(strings.ReplaceAll(fixture, "$COMPUTE_CALLS", filepath.Join(root, "calls"))), 0700)
	os.WriteFile(filepath.Join(bin, "getent"), []byte("#!/bin/sh\nprintf 'researcher:x:1000:1000::/home/researcher:/bin/bash\\n'\n"), 0700)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("COMPUTE_CALLS", filepath.Join(root, "calls"))
	t.Setenv("SLURM_JOB_ID", "123")
	image := filepath.Join(stage, "workspace.sif")
	cmd := exec.Command("bash", "-s", "--", run, state, project, "", image, "apptainer", "0", stage)
	cmd.Stdin = strings.NewReader(remoteAsset("job.sh"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out), err)
	}
	calls, _ := os.ReadFile(filepath.Join(root, "calls"))
	if !bytes.Contains(calls, []byte("build "+image+".tmp.123 docker-archive:")) {
		t.Fatal(string(calls))
	}
	// Exercise the actual initialization script against a disposable project,
	// with paths relocated, and a custom source (no network/package operations).
	p := RemoteProfile{ImageSource: "dockerfile", Architecture: "amd64"}
	files, _ := remoteProjectFiles(p, "FROM ubuntu:24.04\n", []byte(`{"imageId":"fixture"}`))
	tarPath := filepath.Join(run, "project.tar")
	writeRemoteProjectTar(tarPath, files)
	script := strings.ReplaceAll(strings.Replace(remoteAsset("initialize-project.sh"), `cd -- "${1:-/workspace}"`, "cd "+remoteQuote(project), 1), "run=/run/vscode-launcher", "run="+remoteQuote(run))
	init := func() error {
		cmd := exec.Command("bash", "-s")
		cmd.Stdin = strings.NewReader(script)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s: %w", out, err)
		}
		return nil
	}
	if err := init(); err != nil {
		t.Fatal(err)
	}
	if err := init(); err != nil {
		t.Fatal("not idempotent", err)
	}
	// Catalog serialization can change JSON whitespace without changing image identity.
	files["code/.sandbox/remote-image.lock.json"] = []byte("{\n  \"imageId\": \"fixture\"\n}\n")
	writeRemoteProjectTar(tarPath, files)
	if err := init(); err != nil {
		t.Fatal("equivalent JSON image lock was rejected", err)
	}
	for _, dir := range []string{"code/.sandbox", "data", ".vscode"} {
		if info, err := os.Stat(filepath.Join(project, dir)); err != nil || !info.IsDir() {
			t.Fatal(dir, err)
		}
	}
	original := []byte("FROM custom:existing\n")
	os.WriteFile(filepath.Join(project, "code/Dockerfile"), original, 0600)
	if err := init(); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(project, "code/Dockerfile"))
	if !bytes.Equal(after, original) {
		t.Fatal("overwrote user's Dockerfile")
	}
	if _, err := os.Stat(filepath.Join(project, "code/Dockerfile.remote")); err != nil {
		t.Fatal("lost actual build recipe", err)
	}
	var metadata map[string]string
	json.Unmarshal(files["code/.sandbox/remote-project.json"], &metadata)
	metadata["version"] = "changed"
	files["code/.sandbox/remote-project.json"], _ = json.Marshal(metadata)
	writeRemoteProjectTar(tarPath, files)
	if err := init(); err == nil {
		t.Fatal("mixed different environment versions")
	}
}

func TestRemoteFailedPreparationDoesNotRetainAllocation(t *testing.T) {
	remoteTestHome(t)
	s := RemoteSession{Profile: remoteTestProfile(), Token: strings.Repeat("9", 32), Active: true}
	p := &remoteController{state: RemoteStatus{Session: s}, prepareImage: func(context.Context, RemoteProfile, string, io.Writer) (remoteImagePlan, error) {
		return remoteImagePlan{}, fmt.Errorf("build failed")
	}, ssh: func(context.Context, string, string, ...string) (string, error) {
		t.Fatal("contacted HPC after failed local build")
		return "", nil
	}}
	if err := p.launch(context.Background(), s, false); err == nil || p.snapshot().Session.Active {
		t.Fatal("retained a phantom job", err)
	}
	p.state = RemoteStatus{Running: true, Phase: "Preparing remote image and project files", Session: s}
	cancelled := false
	p.cancel = func() { cancelled = true }
	if err := p.stop(); err != nil || !cancelled || p.snapshot().Running || p.snapshot().Session.Active {
		t.Fatal("preparation did not cancel", err)
	}
}

func TestRemoteDockerfileDownloadEndpoint(t *testing.T) {
	p := &remoteController{}
	for _, test := range []struct {
		method, body string
		code         int
	}{
		{"GET", "", 405}, {"POST", `{"imageSource":"python","version":"3.13"}`, 200},
		{"POST", `{"imageSource":"registry","image":"https://hub.docker.com/foo"}`, 400},
		{"POST", `{"imageSource":"dockerfile","dockerfile":"RUN echo invalid"}`, 400},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(test.method, "/remote/dockerfile", strings.NewReader(test.body))
		p.serve("remote/dockerfile", w, r, &logState{}, &prerequisiteSetup{})
		if w.Code != test.code {
			t.Fatal(test, w.Code, w.Body.String())
		}
		if w.Code == 200 && (!strings.Contains(w.Header().Get("Content-Disposition"), "Dockerfile") || !strings.Contains(w.Body.String(), "mcr.microsoft.com/devcontainers/python:3.13-bookworm")) {
			t.Fatal("wrong recipe download", w.Body.String())
		}
	}
}

func TestRemoteRAlignmentScopeAndAtomicFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires Python test fixture")
	}
	helper := filepath.Join(t.TempDir(), "align.py")
	if err := os.WriteFile(helper, []byte(remoteAsset("align-r-libraries.py")), 0600); err != nil {
		t.Fatal(err)
	}
	script := `
import pathlib,runpy,struct,sys,tempfile
api=runpy.run_path(sys.argv[1]); inspect=api['inspect_elf']; needs=api['needs_alignment']
root=pathlib.Path(tempfile.mkdtemp()).resolve()
def fixture(filename,kind=3,machine=62,alignment=4096,vaddr=8192,needed=None):
 data=bytearray(0x3000);ident=b'\x7fELF\x02\x01\x01'+bytes(9)
 count=3 if needed else 2
 struct.pack_into('<16sHHIQQQIHHHHHH',data,0,ident,kind,machine,1,0,64,0,0,64,56,count,64,0,0)
 struct.pack_into('<IIQQQQQQ',data,64,1,5,0,0,0,4096,4096,4096)
 struct.pack_into('<IIQQQQQQ',data,120,1,6,4096,vaddr,vaddr,4096,4096,alignment)
 if needed:
  strings=needed.encode()+b'\0';data[0x400:0x400+len(strings)]=strings
  struct.pack_into('<IIQQQQQQ',data,176,2,6,0x300,0x300,0x300,64,64,8)
  for i,pair in enumerate([(1,0),(5,0x400),(10,len(strings)),(0,0)]):struct.pack_into('<qQ',data,0x300+i*16,*pair)
 filename.write_bytes(data);return bytes(data)
lib=root/'libicu.so';original=fixture(lib)
assert needs(inspect(lib))
fixture(lib,alignment=16384);assert not needs(inspect(lib))
fixture(lib,vaddr=4096);assert not needs(inspect(lib))
fixture(lib,machine=183);assert inspect(lib) is None
original=fixture(lib)
exe=root/'R';fixture(exe,kind=2,needed='libicu.so')
unrelated=root/'unused.so';fixture(unrelated)
graph=api['dependency_paths']([exe],{'libicu.so':[lib],'unused.so':[unrelated]})
assert set(graph)=={exe,lib},graph
hidden=root/'hidden.so';lib.rename(hidden)
try:
 try:api['dependency_paths']([exe],{})
 except RuntimeError:pass
 else:raise AssertionError('accepted an unresolved R dependency')
finally:hidden.rename(lib)
# A failed rewrite must leave the original file, permissions and directory intact.
process=api['align_library'].__globals__['subprocess']
process.check_output=lambda *args,**kwargs:''
def corrupt(args,**kwargs):pathlib.Path(args[-1]).write_bytes(b'invalid ELF')
process.run=corrupt
try:api['align_library'](lib,inspect(lib))
except RuntimeError:pass
else:raise AssertionError('accepted a corrupt rewrite')
assert lib.read_bytes()==original
assert not list(root.glob('.vscode-elf-*'))
print('ELF layout, native/compatible skips, R dependency scope, and atomic preservation passed')
`
	cmd := exec.Command("python3", "-c", script, helper)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out), err)
	}
}

func TestRemoteRRecipeContainsPortableAlignmentAndRuntimeCheck(t *testing.T) {
	p := remoteTestProfile()
	p.ImageSource = "r"
	p.Version = "4.5"
	recipe := remoteDockerfile(p)
	alignment, smoke := -1, -1
	for index, line := range strings.Split(recipe, "\n") {
		if !strings.HasPrefix(line, "RUN [") {
			continue
		}
		var command []string
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "RUN ")), &command); err != nil {
			t.Fatal(err)
		}
		if len(command) == 3 && command[0] == "python3" && command[2] == remoteAsset("align-r-libraries.py") {
			alignment = index
		}
		if len(command) == 4 && command[0] == "Rscript" && command[3] == remoteAsset("smoke-r.R") {
			smoke = index
		}
	}
	if alignment < 0 || smoke <= alignment {
		t.Fatal("R must be aligned then executed before image export")
	}
	p.ImageSource = "python"
	p.Version = "3.13"
	if strings.Contains(remoteDockerfile(p), "align_library") {
		t.Fatal("rewrote Python's libraries")
	}
}
