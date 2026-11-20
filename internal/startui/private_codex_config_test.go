package startui

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestPrivateCodexAlsoConfiguresContinueAndRestoresConnections(t *testing.T) {
	mockSBX(t)
	remote := t.TempDir()
	t.Setenv("REMOTE_HOME", remote)
	codex := filepath.Join(remote, ".codex", "config.toml")
	cont := filepath.Join(remote, ".continue", "config.yaml")
	os.MkdirAll(filepath.Dir(codex), 0700)
	os.MkdirAll(filepath.Dir(cont), 0700)
	originalCodex := "model = 'original-model'\n"
	originalContinue := "name: Original Continue\nmodels: []\n"
	os.WriteFile(codex, []byte(originalCodex), 0600)
	os.WriteFile(cont, []byte(originalContinue), 0600)
	mock := filepath.Join(filepath.Dir(os.Getenv("TRACE")), "sbx")
	script, _ := os.ReadFile(mock)
	script = []byte(strings.Replace(string(script), `case "$1 ${2:-}" in`, `if [[ "$1" == exec ]]; then
 shift
 [[ "$1" == -i ]] && shift
 shift
 export HOME="$REMOTE_HOME"
 exec "$@"
fi
case "$1 ${2:-}" in`, 1))
	os.WriteFile(mock, script, 0755)
	requests := 0
	key := "separate-sandbox-key"
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer "+key {
			t.Error("incorrect model discovery")
		}
		fmt.Fprint(w, `{"data":[{"id":"z-model"},{"id":"a-model"}]}`)
	}))
	defer gateway.Close()
	s := selection()
	s.Endpoint = "private"
	s.BaseURL = gateway.URL + "/v1"
	s.Token = key
	s.PrivateAgent = "codex"
	for _, step := range []string{"codex", "rotate-key", "continue", "codex-again", "default"} {
		switch step {
		case "rotate-key":
			key = "replacement-sandbox-key"
			s.Token = key
		case "continue":
			s.PrivateAgent = "continue"
		case "codex-again":
			s.PrivateAgent = "codex"
		case "default":
			s.Endpoint = "default"
			s.BaseURL = ""
			s.Token = ""
		}
		before := requests
		if err := applyStartup("test-sandbox", "both", s, io.Discard); err != nil {
			t.Fatalf("%s: %v", step, err)
		}
		codexData, _ := os.ReadFile(codex)
		continueData, _ := os.ReadFile(cont)
		if step == "default" {
			if string(codexData) != originalCodex || string(continueData) != originalContinue {
				t.Fatal("default did not restore both originals")
			}
			if requests != before {
				t.Fatal("default made a private model request")
			}
			continue
		}
		if requests != before+1 {
			t.Fatal("discovery was not shared between agents")
		}
		if !strings.Contains(string(continueData), key) || !strings.Contains(string(continueData), "z-model") {
			t.Fatal("Continue not configured for private endpoint")
		}
		if step == "continue" {
			if string(codexData) != originalCodex {
				t.Fatal("Continue-only did not restore Codex")
			}
			continue
		}
		var config map[string]any
		if err := toml.Unmarshal(codexData, &config); err != nil {
			t.Fatal(err)
		}
		if config["model"] != "a-model" || config["model_provider"] != "sandbox_private" {
			t.Fatal("incorrect private Codex model/provider", config["model"])
		}
		provider := config["model_providers"].(map[string]any)["sandbox_private"].(map[string]any)
		if provider["base_url"] != s.BaseURL || provider["wire_api"] != "responses" || provider["requires_openai_auth"] != false || provider["experimental_bearer_token"] != key {
			t.Fatal("incorrect private Codex connection")
		}
		if info, err := os.Stat(codex); err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("Codex key file is not private", err)
		}
		original, _ := os.ReadFile(filepath.Join(remote, ".codex", ".sandbox-original.toml"))
		if string(original) != originalCodex {
			t.Fatal("overwrote original Codex backup")
		}
	}
	// Credentials must never land in host preferences or logs.
	traceData, _ := os.ReadFile(os.Getenv("TRACE"))
	if strings.Contains(string(traceData), key) || strings.Contains(string(traceData), "separate-sandbox-key") {
		t.Fatal("key leaked through arguments/logs")
	}
}

func TestPrivateAgentValidationAndCompatibility(t *testing.T) {
	for _, agent := range []string{"", "continue", "codex", "claude", "both"} {
		s := selection()
		s.Endpoint = "private"
		s.BaseURL = "https://private.example/v1"
		s.Token = "key"
		s.PrivateAgent = agent
		err := Validate(s)
		valid := agent == "" || agent == "continue" || agent == "codex"
		if (err == nil) != valid {
			t.Fatal("private agent validation", agent, err)
		}
	}
	s := directConnection(selection())
	if s.PrivateAgent != "continue" {
		t.Fatal("legacy selections must default to Continue")
	}
}
