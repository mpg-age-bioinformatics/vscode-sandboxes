package startui

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestModelsEndpointPaths(t *testing.T) {
	for _, test := range []struct{ base, path string }{
		{"https://gateway.example", "/models"},
		{"https://gateway.example/v1/", "/v1/models"},
		{"https://gateway.example/team/api", "/team/api/models"},
		{"https://gateway.example/v1/models", "/v1/models"},
		{"https://gateway.example/team%2Fname/v1", "/team/name/v1/models"},
	} {
		endpoint, err := modelsEndpoint(test.base)
		if err != nil || endpoint.Path != test.path {
			t.Fatal(endpoint, err)
		}
	}
	for _, base := range []string{"", "ftp://gateway.example", "https://user:key@gateway.example", "https://gateway.example/#secret"} {
		if _, err := modelsEndpoint(base); err == nil {
			t.Fatal("accepted invalid model endpoint")
		}
	}
}

func TestModelListingUsesBearerKeyAndReturnsAliases(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer fake-model-key" {
			t.Error("incorrect model request")
		}
		fmt.Fprint(w, `{"data":[{"id":"router-alias"},{"id":"model-b"},{"id":"router-alias"},{"id":"fake-model-key"},{"id":""}]}`)
	}))
	defer server.Close()
	models, err := listEndpointModels(context.Background(), server.URL+"/v1", "fake-model-key")
	if err != nil || strings.Join(models, ",") != "model-b,router-alias" {
		t.Fatal(models, err)
	}
}

func TestModelListingDoesNotExposeErrorBodiesOrKeys(t *testing.T) {
	for _, status := range []int{401, 403, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				fmt.Fprint(w, "sensitive-response-body fake-model-key")
			}))
			defer server.Close()
			_, err := listEndpointModels(context.Background(), server.URL, "fake-model-key")
			if err == nil || strings.Contains(err.Error(), "fake-model-key") || strings.Contains(err.Error(), "sensitive-response-body") {
				t.Fatal("leaked response or ignored error")
			}
		})
	}
}

func TestModelListingNeverForwardsCredentialsToAnotherOrigin(t *testing.T) {
	var calls atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, `{"data":[]}`) }))
	defer other.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other.URL+"/models", 302) }))
	defer source.Close()
	if _, err := listEndpointModels(context.Background(), source.URL, "fake-model-key"); err == nil {
		t.Fatal("followed cross-origin redirect")
	}
	if calls.Load() != 0 {
		t.Fatal("sent a request to a different origin")
	}
}

func TestModelListingFollowsSameOriginRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			http.Redirect(w, r, "/listed-models", 307)
			return
		}
		if r.Header.Get("Authorization") != "Bearer fake-model-key" {
			t.Error("lost key on same-origin redirect")
		}
		fmt.Fprint(w, `{"data":[{"id":"router"}]}`)
	}))
	defer server.Close()
	models, err := listEndpointModels(context.Background(), server.URL, "fake-model-key")
	if err != nil || len(models) != 1 || models[0] != "router" {
		t.Fatal(models, err)
	}
}

func TestModelListingHandlesEmptyAndMalformedResponses(t *testing.T) {
	for _, body := range []string{`not json`, `{}`, `{"data":null}`, `{"data":[{"id":123}]}`, strings.Repeat("x", (2<<20)+1)} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		_, err := listEndpointModels(context.Background(), server.URL, "fake-model-key")
		server.Close()
		if err == nil {
			t.Fatal("accepted invalid response")
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"data":[]}`) }))
	defer server.Close()
	models, err := listEndpointModels(context.Background(), server.URL, "fake-model-key")
	if err != nil || models == nil || len(models) != 0 {
		t.Fatal(models, err)
	}
	if _, err := listEndpointModels(context.Background(), server.URL, ""); err == nil {
		t.Fatal("allowed an empty API key")
	}
}
