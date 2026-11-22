package startui

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Background allocation probes must not repeatedly interrupt the user after cancellation.
type sshBackgroundProbe struct{}

type sshChallenge struct {
	ID     string `json:"id"`
	Host   string `json:"host"`
	Prompt string `json:"prompt"`
	answer chan sshAnswer
}
type sshAnswer struct {
	ID       string `json:"id"`
	Response string `json:"response"`
	Cancel   bool   `json:"cancel"`
}

var sshPrompts = struct {
	sync.Mutex
	pending []*sshChallenge
}{}

func sshNonce() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

// Every command has its own short-lived, capability-protected loopback bridge.
// Responses exist only in request memory and a single-use channel, never files,
// process arguments, environment variables, preferences or activity records.
func configureSSHAskpass(cmd *exec.Cmd, host string) (func(), error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	token := sshNonce()
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodPost || r.URL.Path != "/"+token || r.Header.Get("Origin") != "" {
			http.Error(w, "Forbidden", 403)
			return
		}
		var input struct {
			Prompt string `json:"prompt"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&input); err != nil {
			http.Error(w, "Invalid prompt", 400)
			return
		}
		challenge := &sshChallenge{ID: sshNonce(), Host: host, Prompt: input.Prompt, answer: make(chan sshAnswer, 1)}
		sshPrompts.Lock()
		sshPrompts.pending = append(sshPrompts.pending, challenge)
		sshPrompts.Unlock()
		defer removeSSHChallenge(challenge.ID)
		select {
		case answer := <-challenge.answer:
			if answer.Cancel {
				if cmd.Process != nil {
					_ = cmd.Process.Kill()
				}
				http.Error(w, "Authentication cancelled", 401)
				return
			}
			_, _ = io.WriteString(w, answer.Response)
		case <-ctx.Done():
			http.Error(w, "Authentication expired", 408)
		case <-r.Context().Done():
		}
	})}
	// Only helper routing information enters the environment, never a response.
	env := os.Environ()
	for _, key := range []string{"SSH_ASKPASS", "SSH_ASKPASS_REQUIRE", "VSCODE_SSH_ASKPASS_URL"} {
		clean := env[:0]
		for _, entry := range env {
			if !strings.HasPrefix(entry, key+"=") {
				clean = append(clean, entry)
			}
		}
		env = clean
	}
	cmd.Env = append(env, "SSH_ASKPASS="+executable, "SSH_ASKPASS_REQUIRE=force", "VSCODE_SSH_ASKPASS_URL=http://"+listener.Addr().String()+"/"+token)
	// Older Unix OpenSSH versions also require DISPLAY to invoke askpass.
	if os.Getenv("DISPLAY") == "" {
		cmd.Env = append(cmd.Env, "DISPLAY=vscode-sandboxes:0")
	}
	go server.Serve(listener)
	return func() { cancel(); _ = server.Close() }, nil
}

func removeSSHChallenge(id string) {
	sshPrompts.Lock()
	defer sshPrompts.Unlock()
	for i, p := range sshPrompts.pending {
		if p.ID == id {
			sshPrompts.pending = append(sshPrompts.pending[:i], sshPrompts.pending[i+1:]...)
			return
		}
	}
}

func serveSSHAuthentication(route string, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	if route == "remote/auth/prompt" {
		if r.Method != http.MethodGet {
			http.Error(w, "GET required", 405)
			return
		}
		sshPrompts.Lock()
		defer sshPrompts.Unlock()
		var prompt *sshChallenge
		if len(sshPrompts.pending) > 0 {
			prompt = sshPrompts.pending[0]
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"prompt": prompt})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", 405)
		return
	}
	var answer sshAnswer
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&answer); err != nil {
		http.Error(w, "Invalid response", 400)
		return
	}
	sshPrompts.Lock()
	defer sshPrompts.Unlock()
	for i, p := range sshPrompts.pending {
		if p.ID == answer.ID {
			sshPrompts.pending = append(sshPrompts.pending[:i], sshPrompts.pending[i+1:]...)
			p.answer <- answer
			_, _ = io.WriteString(w, `{"ok":true}`)
			return
		}
	}
	http.Error(w, "Authentication prompt expired", 409)
}

func runSSHAskpass(prompt string, output io.Writer) error {
	address := os.Getenv("VSCODE_SSH_ASKPASS_URL")
	// Never allow a malicious inherited environment to send a response elsewhere.
	parsed, parseErr := url.Parse(address)
	if parseErr != nil || parsed.Scheme != "http" || parsed.Hostname() != "127.0.0.1" || parsed.User != nil || parsed.Port() == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("Invalid SSH authentication bridge")
	}
	payload, err := json.Marshal(map[string]string{"prompt": prompt})
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 5 * time.Minute, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return errors.New("Redirect forbidden") }}
	defer client.CloseIdleConnections()
	response, err := client.Post(address, "application/json", bytes.NewReader(payload))
	if err != nil {
		return errors.New("SSH authentication unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return errors.New("SSH authentication cancelled or expired")
	}
	_, err = io.Copy(output, io.LimitReader(response.Body, 16384))
	return err
}
