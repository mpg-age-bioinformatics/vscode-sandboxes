package startui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func endpointTestController(t *testing.T) (*remoteController, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX container scripts")
	}
	home := remoteTestHome(t)
	p := &remoteController{}
	p.ssh = func(ctx context.Context, host, script string, args ...string) (string, error) {
		if host != "fixture-container" || len(args) != 0 {
			t.Fatal("credentials must not travel through process arguments", host, args)
		}
		command := exec.CommandContext(ctx, "bash", "-s")
		command.Stdin = strings.NewReader(script)
		var output, stderr bytes.Buffer
		command.Stdout = &output
		command.Stderr = &stderr
		err := command.Run()
		if err != nil {
			return output.String(), fmt.Errorf("%w: %s", err, stderr.String())
		}
		return output.String(), nil
	}
	return p, home
}

func TestRemoteEndpointUsesLocalConfigurationAndRestoresOriginals(t *testing.T) {
	p, home := endpointTestController(t)
	const key = "private-remote-fixture-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer "+key {
			t.Error("incorrect discovery authentication or path")
			w.WriteHeader(401)
			return
		}
		fmt.Fprint(w, `{"data":[{"id":"z-model"},{"id":"a-model"}]}`)
	}))
	defer server.Close()
	for folder, file := range map[string]string{".continue": "config.yaml", ".codex": "config.toml"} {
		os.MkdirAll(filepath.Join(home, folder), 0700)
		os.WriteFile(filepath.Join(home, folder, file), []byte("original "+folder+" configuration"), 0600)
	}
	connection := remoteEndpoint{BaseURL: server.URL + "/v1", APIKey: key, PrivateAgent: "continue"}
	check := func(agent string) {
		t.Helper()
		connection.PrivateAgent = agent
		if err := p.configureEndpoint(context.Background(), "fixture-container", connection); err != nil {
			t.Fatal(err)
		}
		selection := Selection{Endpoint: "private", BaseURL: connection.BaseURL, Token: key, PrivateAgent: agent}
		local, err := privateContinueConfig(context.Background(), selection)
		if err != nil {
			t.Fatal(err)
		}
		current, err := os.ReadFile(filepath.Join(home, ".continue/config.yaml"))
		if err != nil || !bytes.Equal(current, local) {
			t.Fatal("Remote and Local Continue configurations differ", err)
		}
		codex, _ := os.ReadFile(filepath.Join(home, ".codex/config.toml"))
		if agent == "codex" {
			expected, _, err := privateCodexConfig(selection, local)
			if err != nil || !bytes.Equal(codex, expected) {
				t.Fatal("Remote and Local Codex configurations differ", err)
			}
		} else if string(codex) != "original .codex configuration" {
			t.Fatal("Continue choice changed Codex", string(codex))
		}
		for _, file := range []string{".continue/config.yaml", ".codex/config.toml"} {
			info, err := os.Stat(filepath.Join(home, file))
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatal("configuration is not private", file, err)
			}
		}
		status, _ := json.Marshal(p.snapshot())
		if bytes.Contains(status, []byte(key)) || bytes.Contains(status, []byte(connection.BaseURL)) {
			t.Fatal("credentials escaped into public launcher state")
		}
	}
	check("continue")
	check("codex")
	check("continue")
	if err := p.configureEndpoint(context.Background(), "fixture-container", remoteEndpoint{Reset: true}); err != nil {
		t.Fatal(err)
	}
	for folder, file := range map[string]string{".continue": "config.yaml", ".codex": "config.toml"} {
		current, _ := os.ReadFile(filepath.Join(home, folder, file))
		if string(current) != "original "+folder+" configuration" {
			t.Fatal("did not restore original config", folder)
		}
	}
}

func TestRemoteEndpointRedirectsAndErrorsDoNotExposeKeys(t *testing.T) {
	p, home := endpointTestController(t)
	var received atomic.Bool
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received.Store(true) }))
	defer destination.Close()
	const key = "must-not-leak-fixture-key"
	for _, redirect := range []bool{false, true} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if redirect {
				http.Redirect(w, r, destination.URL+"/models", http.StatusFound)
				return
			}
			w.WriteHeader(401)
			fmt.Fprint(w, key)
		}))
		err := p.configureEndpoint(context.Background(), "fixture-container", remoteEndpoint{BaseURL: server.URL, APIKey: key})
		server.Close()
		if err == nil || strings.Contains(err.Error(), key) || strings.Contains(p.snapshot().Text, key) {
			t.Fatal("private discovery error leaked credentials", err)
		}
	}
	if received.Load() {
		t.Fatal("followed a cross-origin authenticated redirect")
	}
	if _, err := os.Stat(filepath.Join(home, ".continue/config.yaml")); !os.IsNotExist(err) {
		t.Fatal("wrote configuration after failed discovery", err)
	}
	p.ssh = func(context.Context, string, string, ...string) (string, error) { return "", errors.New(key) }
	if err := p.configureEndpoint(context.Background(), "fixture-container", remoteEndpoint{BaseURL: "https://endpoint.example/v1", APIKey: key}); err == nil || strings.Contains(err.Error(), key) {
		t.Fatal("SSH errors must be redacted", err)
	}
}

func TestRemoteEndpointValidationAndLegacyIsolation(t *testing.T) {
	for _, connection := range []remoteEndpoint{
		{BaseURL: "https://endpoint.example/v1"}, {APIKey: "secret"},
		{BaseURL: "https://user:secret@host/v1", APIKey: "secret"},
		{BaseURL: "https://endpoint.example/v1", APIKey: "key\nkey"},
		{PrivateAgent: "unknown"}, {Reset: true, APIKey: "secret"},
	} {
		if connection.validate() == nil {
			t.Fatal("accepted invalid endpoint connection")
		}
	}
	p := &remoteController{state: RemoteStatus{Session: RemoteSession{Active: true, JobID: "55", Node: "node", Port: 22022}}}
	if err := p.applyEndpoint(remoteEndpoint{BaseURL: "https://endpoint.example/v1", APIKey: "secret"}); err == nil {
		t.Fatal("allowed private credentials into a legacy persistent home")
	}
}

func TestRemoteEndpointApplyRouteKeepsCredentialsOutOfState(t *testing.T) {
	p, home := endpointTestController(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"data":[{"id":"fixture-model"}]}`) }))
	defer server.Close()
	originalSSH := p.ssh
	token := strings.Repeat("e", 32)
	p.ssh = func(ctx context.Context, host, script string, args ...string) (string, error) {
		if host != "vscode-sandboxes-"+token {
			t.Error("wrong target allocation")
		}
		return originalSSH(ctx, "fixture-container", script, args...)
	}
	p.state.Session = RemoteSession{Profile: remoteTestProfile(), Active: true, Token: token, JobID: "77", Node: "compute", Port: 22022, ProjectPath: "/projects/current"}
	payload, _ := json.Marshal(remoteEndpoint{BaseURL: server.URL, APIKey: "ephemeral-route-key", PrivateAgent: "codex"})
	request := httptest.NewRequest(http.MethodPost, "/remote/endpoint", bytes.NewReader(payload))
	response := httptest.NewRecorder()
	p.serve("remote/endpoint", response, request, &logState{}, &prerequisiteSetup{})
	if response.Code != http.StatusAccepted {
		t.Fatal("endpoint route rejected valid configuration", response.Code, response.Body.String())
	}
	for deadline := time.Now().Add(10 * time.Second); p.snapshot().Running; {
		if time.Now().After(deadline) {
			t.Fatal("endpoint configuration did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
	status := p.snapshot()
	if status.Error != "" {
		t.Fatal(status.Error)
	}
	state, _ := json.Marshal(status)
	if bytes.Contains(state, []byte("ephemeral-route-key")) || bytes.Contains(response.Body.Bytes(), []byte("ephemeral-route-key")) {
		t.Fatal("key exposed by endpoint route")
	}
	data, err := os.ReadFile(filepath.Join(home, ".codex/config.toml"))
	if err != nil || !bytes.Contains(data, []byte("ephemeral-route-key")) {
		t.Fatal("Codex did not receive the sandbox key", err)
	}
}

func TestRemoteEndpointProxyConfiguration(t *testing.T) {
	_, home := endpointTestController(t)
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:32123")
	config := `{"models":[{"name":"fixture","requestOptions":{"timeout":1000}}]}`
	command := exec.Command("bash", "-s")
	command.Stdin = strings.NewReader(endpointPython("endpoint-config.py", map[string]any{"continue": config}))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatal(string(output), err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".continue/config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var actual struct {
		Models []struct {
			RequestOptions struct {
				Proxy   string
				Timeout int
			}
		}
	}
	if err := json.Unmarshal(data, &actual); err != nil {
		t.Fatal(err)
	}
	if len(actual.Models) != 1 || actual.Models[0].RequestOptions.Proxy != "http://127.0.0.1:32123" || actual.Models[0].RequestOptions.Timeout != 1000 {
		t.Fatal(string(data))
	}
}
