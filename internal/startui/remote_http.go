package startui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"time"
)

func remotePrerequisites(prerequisites *prerequisiteSetup) map[string]any {
	_, sshErr := hostSSHBinary("ssh")
	_, keyErr := hostSSHBinary("ssh-keygen")
	_, editorErr := editorCommand("--version")
	_, dockerErr := SandboxCLI()
	prerequisites.Lock()
	installing := prerequisites.installing
	log, installError := prerequisites.log, prerequisites.err
	prerequisites.Unlock()
	return map[string]any{"log": log, "error": installError, "ssh": sshErr == nil && keyErr == nil, "vscode": editorErr == nil, "docker": dockerErr == nil, "installing": installing, "supported": (runtime.GOOS == "darwin" && runtime.GOARCH == "arm64") || (runtime.GOOS == "windows" && runtime.GOARCH == "amd64")}
}

func (p *remoteController) serve(route string, w http.ResponseWriter, r *http.Request, local *logState, prerequisites *prerequisiteSetup) {
	switch route {
	case "remote/cache", "remote/cache/clear":
		p.serveLocalRemoteCache(w, r, route == "remote/cache/clear", local, prerequisites)

	case "remote/activity":
		if r.Method != http.MethodGet {
			http.Error(w, "GET required", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		if r.URL.Query().Has("download") {
			w.Header().Set("Content-Disposition", `attachment; filename="vscode-sandox.`+time.Now().Format("20060102.150405")+`.log"`)
		}
		status := p.snapshot()
		text := status.Text
		if status.Phase != "" {
			text += "\nStatus: " + status.Phase + "\n"
		}
		if status.Error != "" {
			text += "\nError: " + status.Error + "\n"
		}
		if prerequisites != nil {
			prerequisites.Lock()
			if prerequisites.log != "" {
				text += "\nLocal tool setup:\n" + prerequisites.log
			}
			if prerequisites.err != "" {
				text += "\nLocal tool setup error: " + prerequisites.err + "\n"
			}
			prerequisites.Unlock()
		}
		_, _ = w.Write([]byte(ansiEscape.ReplaceAllString(text, "")))
	case "remote.js":
		if r.Method != "GET" {
			http.Error(w, "GET required", 405)
			return
		}
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		data, _ := content.ReadFile("remote.js")
		_, _ = w.Write(data)
	case "remote/recipe":
		if r.Method != "GET" {
			http.Error(w, "GET required", 405)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(remoteAsset("remote.def")))
	case "remote/dockerfile":
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var profile RemoteProfile
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1048576))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&profile); err != nil {
			http.Error(w, "Invalid profile", 400)
			return
		}
		if err := validateRemoteImage(profile); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if remoteSource(profile) == "sif" {
			http.Error(w, "An existing SIF has no generated Dockerfile", 400)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="Dockerfile"`)
		_, _ = w.Write([]byte(remoteDockerfile(profile)))
	case "remote/defaults":
		if r.Method != "GET" {
			http.Error(w, "GET required", 405)
			return
		}
		profile := defaultRemoteProfile()
		_ = readJSON(settingsDir()+"/remote-profile.json", &profile)
		profile = normalizeRemoteProfile(profile)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"sitePresets": sitePresets(), "sandboxes": Catalog(), "profile": profile, "status": p.snapshot(), "prerequisites": remotePrerequisites(prerequisites)})
	case "remote/status":
		if r.Method != "GET" {
			http.Error(w, "GET required", 405)
			return
		}
		p.refreshStatus()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": p.snapshot(), "prerequisites": remotePrerequisites(prerequisites)})
	case "remote/images", "remote/sandboxes", "remote/attach", "remote/stop-selected", "remote/detach":
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var input struct {
			Profile  RemoteProfile  `json:"profile"`
			Token    string         `json:"token"`
			JobID    string         `json:"jobID"`
			Endpoint remoteEndpoint `json:"endpoint"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1048576))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			http.Error(w, "Invalid catalog request", 400)
			return
		}
		input.Profile = normalizeRemoteProfile(input.Profile)
		if route != "remote/detach" {
			if err := validateRemoteCatalogProfile(input.Profile); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer cancel()
		var result any
		var err error
		switch route {
		case "remote/images":
			var records []remoteImageRecord
			err = p.catalog(ctx, input.Profile, "images", nil, &records)
			filtered := []remoteImageRecord{}
			for _, record := range records {
				if validImageRecord(&record) {
					filtered = append(filtered, record)
				}
			}
			result = filtered
		case "remote/sandboxes":
			result, err = p.listRemoteSessions(ctx, input.Profile)
		case "remote/attach":
			local.Lock()
			prerequisites.Lock()
			installing := prerequisites.installing
			prerequisites.Unlock()
			if local.Running || installing {
				err = fmt.Errorf("Wait for local startup or installation to finish.")
			} else {
				err = p.attachRemoteSession(ctx, input.Profile, input.Token, input.JobID, input.Endpoint.normalized())
			}
			local.Unlock()
		case "remote/stop-selected":
			err = p.stopRemoteSession(ctx, input.Profile, input.Token, input.JobID)
		case "remote/detach":
			err = p.detachRemoteSession()
		}
		if err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if result == nil {
			w.WriteHeader(http.StatusAccepted)
		} else {
			_ = json.NewEncoder(w).Encode(result)
		}
	case "remote/start", "remote/reconnect":
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var input struct {
			RemoteProfile
			Endpoint remoteEndpoint `json:"endpoint"`
		}
		if r.Body != nil && r.ContentLength != 0 {
			decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1048576))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&input); err != nil {
				http.Error(w, "Invalid remote start request", 400)
				return
			}
		}
		local.Lock()
		prerequisites.Lock()
		installing := prerequisites.installing
		prerequisites.Unlock()
		if local.Running || installing {
			local.Unlock()
			http.Error(w, "Wait for local startup or installation to finish.", 409)
			return
		}
		err := p.start(input.RemoteProfile, route == "remote/reconnect", input.Endpoint.normalized())
		local.Unlock()
		if err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	case "remote/endpoint":
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var endpoint remoteEndpoint
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&endpoint); err != nil {
			http.Error(w, "Invalid endpoint request", 400)
			return
		}
		if err := p.applyEndpoint(endpoint); err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	case "remote/cancel":
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		if err := p.stop(); err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	default:
		http.NotFound(w, r)
	}
}
