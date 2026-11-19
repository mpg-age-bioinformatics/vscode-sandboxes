package startui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/pelletier/go-toml/v2"
)

// Continue's YAML loader accepts JSON, a YAML 1.2 subset. Use JSON encoding so
// arbitrary model IDs, URLs, and sandbox credentials are escaped safely.
func privateContinueConfig(ctx context.Context, s Selection) ([]byte, error) {
	data, err := endpointConfig(s)
	if err != nil {
		return nil, err
	}
	var config map[string]any
	if err := toml.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	provider, _ := config["model_provider"].(string)
	providers, _ := config["model_providers"].(map[string]any)
	settings, _ := providers[provider].(map[string]any)
	base, _ := settings["base_url"].(string)
	models, err := listEndpointModels(ctx, base, s.Token)
	if err != nil {
		return nil, fmt.Errorf("Discover private models for Continue: %w", err)
	}
	if len(models) == 0 {
		return nil, errors.New("The private endpoint returned no models for this API key.")
	}
	selected, _ := config["model"].(string)
	if selected != "" {
		found := false
		for index, id := range models {
			if id == selected {
				models = append([]string{id}, append(models[:index], models[index+1:]...)...)
				found = true
				break
			}
		}
		if !found {
			return nil, errors.New("The selected model is not in the private endpoint's /models list for this key. Remove or update the model setting in the connection TOML, then retry. Models can be selected in Continue.")
		}
	}
	entries := make([]map[string]any, 0, len(models))
	for _, model := range models {
		entries = append(entries, map[string]any{"name": model, "provider": "openai", "model": model, "apiBase": base, "apiKey": s.Token, "roles": []string{"chat", "edit", "apply"}, "useResponsesApi": false})
	}
	return json.MarshalIndent(map[string]any{"name": "Sandbox private endpoint", "version": "1.0.0", "schema": "v1", "models": entries}, "", "  ")
}
