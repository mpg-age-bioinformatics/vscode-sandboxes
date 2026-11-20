package startui

import (
	"encoding/json"
	"errors"

	"github.com/pelletier/go-toml/v2"
)

// Reuse the authenticated discovery already performed for Continue. Codex needs
// an explicit private model rather than its built-in OpenAI model default.
func privateCodexConfig(s Selection, continueData []byte) ([]byte, string, error) {
	var discovered struct {
		Models []struct {
			Model string `json:"model"`
		} `json:"models"`
	}
	if err := json.Unmarshal(continueData, &discovered); err != nil {
		return nil, "", err
	}
	if len(discovered.Models) == 0 || discovered.Models[0].Model == "" {
		return nil, "", errors.New("The private endpoint returned no model for Codex.")
	}
	data, err := endpointConfig(s)
	if err != nil {
		return nil, "", err
	}
	var config map[string]any
	if err := toml.Unmarshal(data, &config); err != nil {
		return nil, "", err
	}
	provider, _ := config["model_provider"].(string)
	providers, _ := config["model_providers"].(map[string]any)
	settings, _ := providers[provider].(map[string]any)
	if settings == nil {
		return nil, "", errors.New("The private Codex provider is missing.")
	}
	model := discovered.Models[0].Model
	config["model"] = model
	settings["wire_api"] = "responses"
	settings["requires_openai_auth"] = false
	settings["experimental_bearer_token"] = s.Token
	delete(settings, "env_key")
	data, err = toml.Marshal(config)
	return data, model, err
}
