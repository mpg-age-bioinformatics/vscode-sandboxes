package startui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"
)

func modelsEndpoint(base string) (*url.URL, error) {
	endpoint, err := url.Parse(strings.TrimSpace(base))
	if err != nil || endpoint.Hostname() == "" || (endpoint.Scheme != "https" && endpoint.Scheme != "http") || endpoint.User != nil || endpoint.Fragment != "" {
		return nil, errors.New("Enter an HTTP(S) endpoint URL without embedded credentials or a fragment.")
	}
	if endpoint.Scheme == "http" {
		host := strings.ToLower(endpoint.Hostname())
		ip := net.ParseIP(host)
		if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return nil, errors.New("Private endpoints require HTTPS. HTTP is permitted only for a local loopback endpoint or an SSH tunnel on localhost.")
		}
	}
	escapedPath := strings.TrimRight(endpoint.EscapedPath(), "/")
	if !strings.HasSuffix(escapedPath, "/models") {
		escapedPath += "/models"
	}
	endpoint.Path, err = url.PathUnescape(escapedPath)
	if err != nil {
		return nil, errors.New("The endpoint contains an invalid path.")
	}
	endpoint.RawPath = escapedPath
	return endpoint, nil
}

// Model discovery runs on the host for List and private connection startup. The key is
// used for this request, never persisted or included in error/response bodies.
func listEndpointModels(ctx context.Context, base, key string) ([]string, error) {
	endpoint, err := modelsEndpoint(base)
	if err != nil {
		return nil, err
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, errors.New("Enter an API key to list models.")
	}
	if strings.ContainsAny(key, "\r\n") {
		return nil, errors.New("The API key must not contain newlines.")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, errors.New("Could not prepare the model-list request.")
	}
	request.Header.Set("Authorization", "Bearer "+key)
	request.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(next *http.Request, previous []*http.Request) error {
		if len(previous) > 3 || next.URL.Scheme != endpoint.Scheme || !strings.EqualFold(next.URL.Host, endpoint.Host) {
			return http.ErrUseLastResponse
		}
		return nil
	}}
	response, err := client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, errors.New("Model listing was canceled.")
		}
		return nil, errors.New("Could not reach the model endpoint. Check its URL, network access, and certificate trust.")
	}
	defer response.Body.Close()
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return nil, fmt.Errorf("Model listing was denied (HTTP %d). Check the key and its permissions.", response.StatusCode)
	}
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		return nil, errors.New("The model endpoint redirected to another origin or too many times. Enter its final base URL.")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("Model endpoint returned HTTP %d.", response.StatusCode)
	}
	const limit = 2 << 20
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, errors.New("Could not read the model list.")
	}
	if len(data) > limit {
		return nil, errors.New("The model-list response is too large.")
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err = json.Unmarshal(data, &envelope); err != nil || len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return nil, errors.New("The endpoint did not return an OpenAI-compatible model list (data[].id).")
	}
	var entries []struct {
		ID string `json:"id"`
	}
	if err = json.Unmarshal(envelope.Data, &entries); err != nil {
		return nil, errors.New("The endpoint returned an invalid model list.")
	}
	models := []string{}
	seen := map[string]bool{}
	for _, entry := range entries {
		id := strings.TrimSpace(entry.ID)
		if id == "" || id == key || len(id) > 512 || strings.IndexFunc(id, unicode.IsControl) >= 0 || seen[id] {
			continue
		}
		models = append(models, id)
		seen[id] = true
		if len(models) > 2000 {
			return nil, errors.New("The endpoint returned too many models.")
		}
	}
	sort.Strings(models)
	return models, nil
}
