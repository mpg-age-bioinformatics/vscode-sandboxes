package startui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type remoteImageRecord struct {
	RecipeKey    string          `json:"recipeKey"`
	ID           string          `json:"id"`
	Source       string          `json:"source"`
	Version      string          `json:"version"`
	Architecture string          `json:"architecture"`
	Reference    string          `json:"reference"`
	Recipe       string          `json:"recipe"`
	Lock         json.RawMessage `json:"lock"`
	Path         string          `json:"path"`
	Size         int64           `json:"size"`
	State        string          `json:"state"`
	CreatedAt    string          `json:"createdAt"`
}

func remoteImageIdentity(p RemoteProfile) string {
	data, _ := json.Marshal([]string{p.ImageSource, p.Version, remoteArchitecture(p), remoteDockerfile(p), remoteAsset("local-build.sh")})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func validateRemoteCatalogProfile(p RemoteProfile) error {
	if !remoteHostPattern.MatchString(p.Login) || !safeRemotePath(p.StateDir, true) || !safeRemotePath(p.ImagesDir, true) {
		return fmt.Errorf("Enter a valid SSH login, persistent configuration path and image storage path.")
	}
	if _, err := remoteModuleNames(p.ModuleNames); err != nil {
		return err
	}
	if strings.ContainsAny(p.Modules, "\x00\r") || len(p.Modules) > 16384 {
		return fmt.Errorf("Invalid module initialization.")
	}
	return nil
}

func (p *remoteController) catalog(ctx context.Context, profile RemoteProfile, operation string, payload any, result any) error {
	profile = normalizeRemoteProfile(profile)
	if err := validateRemoteCatalogProfile(profile); err != nil {
		return err
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	script := "{\n" + remoteModuleScript(profile) + "\n} >&2\nexec python3 - \"$@\" <<'VSCODE_CATALOG_PY'\n" + remoteAsset("catalog.py") + "\nVSCODE_CATALOG_PY\n"
	output, err := p.ssh(ctx, profile.Login, script, operation, profile.ImagesDir, profile.StateDir, string(data))
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(output), result); err != nil {
		return fmt.Errorf("Invalid remote catalog response: %w", err)
	}
	return nil
}

func validImageRecord(r *remoteImageRecord) bool {
	if r == nil || !remoteImageIDPattern.MatchString(r.ID) || !safeRemotePath(r.Path, false) || r.State != "ready" || r.Size <= 0 || r.Recipe == "" || !json.Valid(r.Lock) {
		return false
	}
	switch r.Source {
	case "python", "r", "bioinformatics", "registry", "dockerfile":
	default:
		return false
	}
	check := RemoteProfile{ImageSource: r.Source, Version: r.Version, Architecture: r.Architecture, Image: r.Reference, Dockerfile: "FROM ubuntu:24.04\n"}
	return validateRemoteImage(check) == nil
}

func describeRemoteImage(p RemoteProfile, recipe string, lock []byte) remoteImageRecord {
	sum := sha256.Sum256([]byte(remoteImageIdentity(p) + "\x00" + string(lock)))
	return remoteImageRecord{ID: hex.EncodeToString(sum[:]), RecipeKey: remoteImageIdentity(p), Source: p.ImageSource, Version: p.Version, Architecture: remoteArchitecture(p), Reference: p.Image, Recipe: recipe, Lock: lock}
}

func (p *remoteController) cachedRemotePlan(ctx context.Context, s *RemoteSession) (remoteImagePlan, bool, error) {
	f := s.Profile
	if remoteSource(f) == "sif" {
		return remoteImagePlan{}, false, nil
	}
	key := f.CachedImage
	query := map[string]string{"id": key}
	if key == "" {
		query = map[string]string{"recipeKey": remoteImageIdentity(f)}
	}
	var record *remoteImageRecord
	if err := p.catalog(ctx, f, "lookup", query, &record); err != nil {
		return remoteImagePlan{}, false, err
	}
	if record == nil {
		if f.CachedImage != "" || f.ImageSource == "cached" {
			return remoteImagePlan{}, false, fmt.Errorf("The selected server image is unavailable. List images again.")
		}
		return remoteImagePlan{}, false, nil
	}
	if !validImageRecord(record) || (key != "" && record.ID != key) || (key == "" && record.RecipeKey != remoteImageIdentity(f)) {
		return remoteImagePlan{}, false, fmt.Errorf("Invalid cached image metadata.")
	}
	template := f
	template.ImageSource = record.Source
	template.Version = record.Version
	template.Architecture = record.Architecture
	template.Image = record.Reference
	files, err := remoteProjectFiles(template, record.Recipe, record.Lock)
	if err != nil {
		return remoteImagePlan{}, false, err
	}
	dir := remoteSessionDir(s.Token)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return remoteImagePlan{}, false, err
	}
	project := filepath.Join(dir, "project.tar")
	if err := writeRemoteProjectTar(project, files); err != nil {
		return remoteImagePlan{}, false, err
	}
	if s.Profile.CachedImage != "" {
		s.Profile.ImageSource = "cached"
	}
	s.Profile.CachedSource = record.Source
	s.Profile.Version = record.Version
	s.Profile.Architecture = record.Architecture
	fmt.Fprintf(p, "Reusing server image %s (%s %s); no local build, image transfer or SIF conversion.\n", record.ID[:12], record.Source, record.Version)
	return remoteImagePlan{Image: record.Path, Project: project, Key: record.ID}, true, nil
}

func (p *remoteController) listRemoteSessions(ctx context.Context, profile RemoteProfile) ([]RemoteSession, error) {
	var sessions []RemoteSession
	if err := p.catalog(ctx, profile, "jobs", nil, &sessions); err != nil {
		return nil, err
	}
	result := []RemoteSession{}
	for _, s := range sessions {
		s.Profile = normalizeRemoteProfile(s.Profile)
		s.Profile.Login = profile.Login
		if validRemoteSession(s) && s.JobID != "" && s.RunDir != "" {
			result = append(result, s)
		}
	}
	return result, nil
}

func (p *remoteController) selectedRemoteSession(ctx context.Context, profile RemoteProfile, token, job string) (RemoteSession, error) {
	sessions, err := p.listRemoteSessions(ctx, profile)
	if err != nil {
		return RemoteSession{}, err
	}
	for _, s := range sessions {
		if s.Token == token && s.JobID == job {
			return s, nil
		}
	}
	return RemoteSession{}, fmt.Errorf("The selected sandbox is no longer running. Refresh the list.")
}

func (p *remoteController) attachRemoteSession(ctx context.Context, profile RemoteProfile, token, job string, endpoints ...remoteEndpoint) error {
	for _, endpoint := range endpoints {
		if err := endpoint.validate(); err != nil {
			return err
		}
	}
	s, err := p.selectedRemoteSession(ctx, profile, token, job)
	if err != nil {
		return err
	}
	p.Lock()
	busy := p.state.Running || p.stopping
	p.Unlock()
	if busy {
		return fmt.Errorf("Wait for current startup or cancellation to finish.")
	}
	dir := remoteSessionDir(s.Token)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	key := filepath.Join(dir, "id_ed25519")
	if _, err := os.Stat(key); os.IsNotExist(err) {
		binary, err := hostSSHBinary("ssh-keygen")
		if err != nil {
			return err
		}
		call := exec.CommandContext(ctx, binary, "-q", "-t", "ed25519", "-N", "", "-f", key)
		hideWindow(call)
		if out, err := call.CombinedOutput(); err != nil {
			return fmt.Errorf("Generate attachment key: %s %w", out, err)
		}
	}
	public, err := os.ReadFile(key + ".pub")
	if err != nil {
		return err
	}
	var authorized RemoteSession
	if err := p.catalog(ctx, profile, "authorize", map[string]string{"token": token, "jobID": job, "publicKey": strings.TrimSpace(string(public))}, &authorized); err != nil {
		return err
	}
	p.Lock()
	if p.state.Running || p.stopping {
		p.Unlock()
		return fmt.Errorf("Remote startup is busy.")
	}
	p.state.Session = s
	p.Unlock()
	return p.start(s.Profile, true, endpoints...)
}

func (p *remoteController) stopRemoteSession(ctx context.Context, profile RemoteProfile, token, job string) error {
	s, err := p.selectedRemoteSession(ctx, profile, token, job)
	if err != nil {
		return err
	}
	var stopped RemoteSession
	if err := p.catalog(ctx, profile, "stop", map[string]string{"token": token, "jobID": job}, &stopped); err != nil {
		return err
	}
	p.Lock()
	tracked := p.state.Session.Token == s.Token
	p.Unlock()
	if tracked {
		p.Lock()
		if p.cancel != nil {
			p.cancel()
		}
		p.generation++
		p.state.Session.Active = false
		p.state.Running = false
		p.state.Phase = "Allocation " + job + " cancelled"
		saved := p.state.Session
		p.Unlock()
		_ = writeJSON(remoteSessionFile(), saved)
	}
	_ = os.Remove(filepath.Join(settingsDir(), "remote-ssh", token+".conf"))
	_ = os.RemoveAll(remoteSessionDir(token))
	return nil
}

func (p *remoteController) detachRemoteSession() error {
	p.Lock()
	defer p.Unlock()
	if p.state.Running || p.stopping {
		return fmt.Errorf("Wait for startup to finish.")
	}
	p.state = RemoteStatus{Phase: "Allocation left running; choose or create a sandbox"}
	return writeJSON(remoteSessionFile(), RemoteSession{})
}

func remoteProfileJSON(p RemoteProfile) string { data, _ := json.Marshal(p); return string(data) }

func refreshRemoteProjectArchive(p RemoteProfile, dir string, hashes *map[string]string) error {
	recipe, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	if err != nil {
		return err
	}
	lock, err := os.ReadFile(filepath.Join(dir, "image-lock.json"))
	if err != nil {
		return err
	}
	files, err := remoteProjectFiles(p, string(recipe), lock)
	if err != nil {
		return err
	}
	if err := writeRemoteProjectTar(filepath.Join(dir, "project.tar"), files); err != nil {
		return err
	}
	digest, err := remoteFileHash(filepath.Join(dir, "project.tar"))
	if err != nil {
		return err
	}
	(*hashes)["project.tar"] = digest
	return writeJSON(filepath.Join(dir, "cache.json"), hashes)
}

func matchingRemoteImageCache(p RemoteProfile, exclude string) string {
	entries, _ := os.ReadDir(filepath.Join(settingsDir(), "remote-images"))
	for _, entry := range entries {
		if !entry.IsDir() || !remoteImageIDPattern.MatchString(entry.Name()) {
			continue
		}
		dir := filepath.Join(settingsDir(), "remote-images", entry.Name())
		if dir == exclude {
			continue
		}
		var lock map[string]string
		if readJSON(filepath.Join(dir, "image-lock.json"), &lock) != nil || lock["architecture"] != remoteArchitecture(p) {
			continue
		}
		expected := remoteDockerfile(p)
		if base := remoteBaseImage(p); base != "" {
			expected = strings.ReplaceAll(expected, base, lock["resolvedImage"])
		}
		expected = strings.ReplaceAll(expected, "node:22.22.1-bookworm-slim", lock["nodeImage"])
		recipe, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
		if err != nil || string(recipe) != expected {
			continue
		}
		var hashes map[string]string
		if readJSON(filepath.Join(dir, "cache.json"), &hashes) != nil || len(hashes) != 4 {
			continue
		}
		valid := true
		for name, want := range hashes {
			if name != "Dockerfile" && name != "image.tar" && name != "image-lock.json" && name != "project.tar" {
				valid = false
				break
			}
			got, err := remoteFileHash(filepath.Join(dir, name))
			if err != nil || got != want {
				valid = false
				break
			}
		}
		if valid {
			return dir
		}
	}
	return ""
}
