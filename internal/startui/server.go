// Package startui provides the shared desktop startup window. The HTTP listener
// is loopback-only and every request is protected by an unguessable capability.
package startui

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

//go:embed window.html
var content embed.FS

type Config struct {
	Name         string        `json:"name"`
	Slug         string        `json:"slug"`
	Version      string        `json:"version"`
	VersionLabel string        `json:"versionLabel"`
	Project      string        `json:"project"`
	Agent        string        `json:"agent"`
	ProjectMode  bool          `json:"projectMode"`
	Sandboxes    []SandboxType `json:"sandboxes,omitempty"`
}

type Selection struct {
	Sandbox    string `json:"sandbox"`
	Project    string `json:"project"`
	Version    string `json:"version"`
	Agent      string `json:"agent"`
	Network    string `json:"network"`
	Sites      string `json:"sites"`
	Endpoint   string `json:"endpoint"`
	ConfigFile string `json:"configFile"`
	BaseURL    string `json:"baseURL"`
	Model      string `json:"model"`
	Token      string `json:"token"`
}

type logState struct {
	sync.Mutex
	Text          string `json:"text"`
	Running       bool   `json:"running"`
	Done          bool   `json:"done"`
	Error         string `json:"error"`
	Confirmation  string `json:"confirmation"`
	answer        chan bool
	secrets       []string
	pendingEscape string
}

var ansiEscape = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
var partialEscape = regexp.MustCompile(`^\x1b\[[0-?]*[ -/]*$`)

func (s *logState) Write(p []byte) (int, error) {
	s.Lock()
	defer s.Unlock()
	t := s.pendingEscape + string(p)
	s.pendingEscape = ""
	if offset := strings.LastIndexByte(t, '\x1b'); offset >= 0 {
		tail := t[offset:]
		if tail == "\x1b" || partialEscape.MatchString(tail) {
			s.pendingEscape, t = tail, t[:offset]
		}
	}
	t = ansiEscape.ReplaceAllString(t, "")
	for _, secret := range s.secrets {
		if secret != "" {
			t = strings.ReplaceAll(t, secret, "[redacted]")
		}
	}
	s.Text += t
	if len(s.Text) > 2<<20 {
		s.Text = s.Text[len(s.Text)-(2<<20):]
	}
	return len(p), nil
}

// Run keeps the window and its live log open until the user closes it.
func Run(config Config, execute func(Selection, io.Writer) error) error {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer ln.Close()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	path := "/" + hex.EncodeToString(key) + "/"
	state := &logState{answer: make(chan bool, 1)}
	closed := make(chan struct{}, 1)
	mux := http.NewServeMux()
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; frame-ancestors 'none'")
		if r.Host != ln.Addr().String() {
			http.Error(w, "invalid host", 403)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+ln.Addr().String() {
			http.Error(w, "invalid origin", 403)
			return
		}
		switch strings.TrimPrefix(r.URL.Path, path) {
		case "":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			data, _ := content.ReadFile("window.html")
			_, _ = w.Write(data)
		case "defaults":
			c := struct {
				Config
				Saved  Selection `json:"saved"`
				Import string    `json:"import"`
			}{Config: config}
			_ = readJSON(filepath.Join(settingsDir(), config.Slug+".json"), &c.Saved)
			c.Import = defaultCodexConfig()
			_ = json.NewEncoder(w).Encode(c)
		case "status":
			state.Lock()
			defer state.Unlock()
			_ = json.NewEncoder(w).Encode(state)
		case "start":
			if r.Method != "POST" {
				http.Error(w, "POST required", 405)
				return
			}
			var selection Selection
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&selection); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			var resolveErr error
			selection, resolveErr = resolveSelection(config, selection)
			if resolveErr != nil {
				http.Error(w, resolveErr.Error(), 400)
				return
			}
			if err := Validate(selection); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			if config.ProjectMode && (selection.Project != config.Project || selection.Agent != config.Agent) {
				http.Error(w, "The project launcher must use its configured workspace and agent.", 400)
				return
			}
			if selection.Endpoint == "private" {
				if _, err := endpointConfig(selection); err != nil {
					http.Error(w, err.Error(), 400)
					return
				}
			}
			state.Lock()
			if state.Running {
				state.Unlock()
				http.Error(w, "already starting", 409)
				return
			}
			state.Running, state.Done, state.Error, state.Text = true, false, "", ""
			state.secrets = []string{selection.Token}
			state.Unlock()
			go func() {
				err := EnsureHost(state)
				if err == nil {
					err = prepareGlobalPolicy(selection, state, func(message string) bool {
						state.Lock()
						state.Confirmation = message
						state.Unlock()
						answer := <-state.answer
						state.Lock()
						state.Confirmation = ""
						state.Unlock()
						return answer
					})
				}
				if err == nil {
					err = execute(selection, state)
				}
				state.Lock()
				defer state.Unlock()
				state.Running, state.Done = false, true
				if err != nil {
					state.Error = err.Error()
				} else {
					saved := selection
					saved.Token = ""
					if saved.Endpoint == "private" {
						saved.ConfigFile = filepath.Join(settingsDir(), "endpoints", "private.toml")
						saved.BaseURL = ""
						saved.Model = ""
					}
					if e := writeJSON(filepath.Join(settingsDir(), config.Slug+".json"), saved); e != nil {
						state.Error = "Started, but could not save settings: " + e.Error()
					}
				}
			}()
			w.WriteHeader(http.StatusAccepted)
		case "confirm":
			if r.Method != "POST" {
				http.Error(w, "POST required", 405)
				return
			}
			var answer struct {
				Apply bool `json:"apply"`
			}
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&answer); err != nil {
				http.Error(w, "Invalid answer", 400)
				return
			}
			state.Lock()
			pending := state.Confirmation != ""
			state.Confirmation = ""
			state.Unlock()
			if !pending {
				http.Error(w, "No pending policy change", 409)
				return
			}
			select {
			case state.answer <- answer.Apply:
			default:
			}
		case "close":
			if r.Method != "POST" {
				http.Error(w, "POST required", 405)
				return
			}
			state.Lock()
			running := state.Running
			state.Unlock()
			if running {
				http.Error(w, "Wait for startup to finish before closing.", 409)
				return
			}
			select {
			case closed <- struct{}{}:
			default:
			}
		default:
			http.NotFound(w, r)
		}
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	defer server.Close()
	go server.Serve(ln)
	url := "http://" + ln.Addr().String() + path
	var window *exec.Cmd
	if runtime.GOOS == "darwin" {
		exe, _ := os.Executable()
		window = exec.Command(filepath.Join(filepath.Dir(exe), "Sandbox Window"), url, config.Name)
	} else if runtime.GOOS == "windows" {
		window = exec.Command("powershell.exe", "-NoProfile", "-STA", "-Command", windowsWindow)
		window.Env = append(os.Environ(), "SANDBOX_WINDOW_URL="+url, "SANDBOX_WINDOW_TITLE="+config.Name)
		hideWindow(window)
	} else {
		return errors.New("the desktop window supports macOS and Windows")
	}
	if err := window.Start(); err != nil {
		return fmt.Errorf("open startup window: %w", err)
	}
	exited := make(chan error, 1)
	go func() { exited <- window.Wait() }()
	select {
	case err := <-exited:
		return err
	case <-closed:
		_ = window.Process.Kill()
		<-exited
		return nil
	}
}

func settingsDir() string {
	root, err := os.UserConfigDir()
	if err != nil {
		root = os.TempDir()
	}
	return filepath.Join(root, "VS Code Sandboxes")
}
func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}
func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return privateWrite(path, data)
}
func privateWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".settings-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	// Windows cannot replace an existing file using Rename.
	if runtime.GOOS == "windows" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return os.Rename(f.Name(), path)
}

const windowsWindow = `
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
$edge = @("${env:ProgramFiles(x86)}\Microsoft\Edge\Application\msedge.exe", "$env:ProgramFiles\Microsoft\Edge\Application\msedge.exe") | Where-Object { Test-Path -LiteralPath $_ } | Select-Object -First 1
if (!$edge) { [Windows.Forms.MessageBox]::Show('Microsoft Edge is required for the sandbox startup window.'); exit 1 }
# A dedicated profile keeps the window independent of existing Edge sessions.
$profile = Join-Path ([IO.Path]::GetTempPath()) ('sandbox-window-' + [Guid]::NewGuid().ToString())
$p = Start-Process -FilePath $edge -ArgumentList @('--app=' + $env:SANDBOX_WINDOW_URL, '--user-data-dir="' + $profile + '"', '--no-first-run', '--no-default-browser-check') -PassThru
$p.WaitForExit()
Remove-Item -LiteralPath $profile -Recurse -Force -ErrorAction SilentlyContinue
`
