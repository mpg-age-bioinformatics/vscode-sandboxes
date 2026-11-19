package startui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPrivateContinueConfigUsesGatewayModels(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer sandbox-key" {
			t.Error("incorrect model discovery request")
		}
		fmt.Fprint(w, `{"data":[{"id":"qwen-private"},{"id":"gemma-private"}]}`)
	}))
	defer gateway.Close()
	s := selection()
	s.Endpoint, s.BaseURL, s.Token = "private", gateway.URL+"/v1", "sandbox-key"
	for _, chosen := range []string{"", "qwen-private", "missing-model"} {
		s.Model = chosen
		data, err := privateContinueConfig(context.Background(), s)
		if chosen == "missing-model" {
			if err == nil {
				t.Fatal("accepted unavailable model")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		var config struct {
			Name, Version, Schema string
			Models                []struct {
				Name, Provider, Model, APIBase, APIKey string
				Roles                                  []string
				UseResponsesAPI                        bool
			}
		}
		if err := json.Unmarshal(data, &config); err != nil {
			t.Fatal(err)
		}
		if config.Name != "Sandbox private endpoint" || config.Version != "1.0.0" || config.Schema != "v1" || len(config.Models) != 2 {
			t.Fatal("invalid Continue configuration")
		}
		want := chosen
		if want == "" {
			want = "gemma-private"
		}
		if config.Models[0].Model != want {
			t.Fatal("chosen model was not first")
		}
		for _, model := range config.Models {
			if model.Provider != "openai" || model.APIBase != s.BaseURL || model.APIKey != s.Token || model.UseResponsesAPI || strings.Join(model.Roles, ",") != "chat,edit,apply" {
				t.Fatal("incorrect Continue model connection")
			}
		}
	}
}
