package startui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// Deliberately separate from persisted profiles, sessions and image catalogs.
type remoteEndpoint struct {
	BaseURL      string `json:"baseURL"`
	APIKey       string `json:"apiKey"`
	PrivateAgent string `json:"privateAgent"`
	Reset        bool   `json:"reset,omitempty"`
}

func (e remoteEndpoint) normalized() remoteEndpoint {
	e.BaseURL = strings.TrimSpace(e.BaseURL)
	e.APIKey = strings.TrimSpace(e.APIKey)
	if e.PrivateAgent == "" {
		e.PrivateAgent = "continue"
	}
	return e
}

func (e remoteEndpoint) validate() error {
	if e.PrivateAgent != "" && e.PrivateAgent != "continue" && e.PrivateAgent != "codex" {
		return errors.New("Choose Codex or Continue for the private endpoint.")
	}
	if e.Reset {
		if e.BaseURL != "" || e.APIKey != "" {
			return errors.New("Clear endpoint credentials before restoring default connections.")
		}
		return nil
	}
	if e.BaseURL == "" && e.APIKey == "" {
		return nil
	}
	if strings.TrimSpace(e.BaseURL) == "" || strings.TrimSpace(e.APIKey) == "" {
		return errors.New("Enter both a private endpoint URL and an API key.")
	}
	if len(e.BaseURL) > 8192 || len(e.APIKey) > 16384 || strings.IndexFunc(e.APIKey, unicode.IsControl) >= 0 {
		return errors.New("Invalid endpoint URL or API key.")
	}
	_, err := modelsEndpoint(e.BaseURL)
	return err
}

func endpointPython(name string, payload any) string {
	data, _ := json.Marshal(payload)
	return "python3 -c " + remoteQuote(remoteAsset(name)) + " <<'VSCODE_ENDPOINT_PAYLOAD'\n" + string(data) + "\nVSCODE_ENDPOINT_PAYLOAD\n"
}

func (p *remoteController) configureEndpoint(ctx context.Context, alias string, e remoteEndpoint) error {
	e = e.normalized()
	if err := e.validate(); err != nil {
		return err
	}
	if !e.Reset && e.BaseURL == "" && e.APIKey == "" {
		return nil
	}
	payload := map[string]any{"continue": nil, "codex": nil}
	if !e.Reset {
		endpoint, _ := modelsEndpoint(e.BaseURL)
		out, err := p.ssh(ctx, alias, endpointPython("endpoint-models.py", map[string]string{"modelsURL": endpoint.String(), "apiKey": e.APIKey}))
		if err != nil {
			return errors.New("Private model discovery failed inside the remote container. Check the endpoint, key, certificate trust and HPC network access.")
		}
		var result struct {
			Models []string `json:"models"`
		}
		if json.Unmarshal([]byte(out), &result) != nil || len(result.Models) == 0 || len(result.Models) > 2000 {
			return errors.New("The remote endpoint returned an invalid model list.")
		}
		for _, model := range result.Models {
			if model == e.APIKey || model == "" || len(model) > 512 || strings.IndexFunc(model, unicode.IsControl) >= 0 {
				return errors.New("The remote endpoint returned an invalid model identifier.")
			}
		}
		selection := Selection{Endpoint: "private", BaseURL: e.BaseURL, Token: e.APIKey, PrivateAgent: e.PrivateAgent}
		data, err := privateContinueConfigForModels(selection, result.Models)
		if err != nil {
			return errors.New("Could not prepare the private Continue connection.")
		}
		payload["continue"] = string(data)
		if e.PrivateAgent == "codex" {
			codex, _, err := privateCodexConfig(selection, data)
			if err != nil {
				return errors.New("Could not prepare the private Codex connection.")
			}
			payload["codex"] = string(codex)
		}
	}
	out, err := p.ssh(ctx, alias, endpointPython("endpoint-config.py", payload))
	if err != nil || strings.TrimSpace(out) != "ENDPOINT_CONFIGURED" {
		return errors.New("Could not apply endpoint settings inside the remote container. Reconnect and retry.")
	}
	if e.Reset {
		fmt.Fprintln(p, "Restored the sandbox's original Continue and Codex connections.")
	} else if e.PrivateAgent == "codex" {
		fmt.Fprintln(p, "Private endpoint configured for Continue and Codex (Responses API) inside this allocation.")
	} else {
		fmt.Fprintln(p, "Private endpoint configured for Continue; Codex keeps its original connection.")
	}
	return nil
}

func (p *remoteController) applyEndpoint(e remoteEndpoint) error {
	e = e.normalized()
	if err := e.validate(); err != nil {
		return err
	}
	if !e.Reset && e.BaseURL == "" && e.APIKey == "" {
		return errors.New("Enter private endpoint credentials or choose Restore default connections.")
	}
	p.Lock()
	session := p.state.Session
	if err := validateExistingRemoteEndpoint(session.Profile, e); err != nil {
		p.Unlock()
		return err
	}
	if p.state.Running || p.stopping {
		p.Unlock()
		return errors.New("Wait for remote startup or cancellation to finish.")
	}
	if !session.Active || session.Port == 0 || session.Node == "" {
		p.Unlock()
		return errors.New("Attach or reconnect to a running sandbox first.")
	}
	if session.ProjectPath == "" {
		p.Unlock()
		return errors.New("Start a new allocation before applying credentials; this legacy allocation has a persistent container home.")
	}
	if p.cancel != nil {
		p.cancel()
	}
	p.state.Running = true
	p.state.Error = ""
	p.state.Phase = "Configuring remote agent connection"
	p.generation++
	generation := p.generation
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	p.cancel = cancel
	p.Unlock()
	go func() {
		defer cancel()
		err := p.configureEndpoint(ctx, "vscode-sandboxes-"+session.Token, e)
		p.Lock()
		defer p.Unlock()
		if p.generation != generation || p.stopping {
			return
		}
		p.state.Running = false
		if err != nil {
			p.state.Error = err.Error()
			p.state.Phase = "Agent connection setup stopped"
		} else {
			p.state.Phase = "Remote agent connection updated"
		}
	}()
	return nil
}
