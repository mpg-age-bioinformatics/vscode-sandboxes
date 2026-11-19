package startui

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrivateEndpointNetworkAccess(t *testing.T) {
	for _, base := range []string{"https://Lite.Example:443/v1", "https://lite.example:8443/v1?route=secret"} {
		s := selection()
		s.Endpoint, s.Network, s.BaseURL, s.Sandbox = "private", "restricted", base, "python-sandbox"
		s.Sites = "pypi.org"
		want := "lite.example"
		if strings.Contains(base, "8443") {
			want += ":8443"
		}
		got, host, err := withPrivateEndpointAccess(s)
		if err != nil || host != want || got.Sites != "pypi.org\n"+want || got.SiteLists[s.Sandbox] != got.Sites {
			t.Fatalf("unexpected access: %+v, %s, %v", got, host, err)
		}
		again, _, err := withPrivateEndpointAccess(got)
		if err != nil || again.Sites != got.Sites {
			t.Fatal("endpoint duplicated", err)
		}
	}
}

func TestPrivateEndpointAccessUsesEffectiveTOML(t *testing.T) {
	file := filepath.Join(t.TempDir(), "endpoint.toml")
	if err := os.WriteFile(file, []byte("model='alias'\nmodel_provider='private'\n[model_providers.private]\nbase_url='https://file.example/v1'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s := selection()
	s.Endpoint, s.Network, s.ConfigFile, s.BaseURL = "private", "restricted", file, "https://draft.example/v1"
	for _, override := range []bool{false, true} {
		s.ConfigOverrides = override
		_, host, err := withPrivateEndpointAccess(s)
		want := "file.example"
		if override {
			want = "draft.example"
		}
		if err != nil || host != want {
			t.Fatalf("host %q, want %q: %v", host, want, err)
		}
	}
}

func TestPrivateEndpointCannotStartOffline(t *testing.T) {
	s := selection()
	s.Endpoint, s.Network = "private", "none"
	for name, check := range map[string]func() error{
		"validate": func() error { return Validate(s) },
		"prepare": func() error {
			_, cleanup, err := PrepareEnvironment(s)
			if cleanup != nil {
				cleanup()
			}
			return err
		},
		"startup": func() error { return applyStartup("unused", "codex", s, io.Discard) },
	} {
		t.Run(name, func(t *testing.T) {
			if err := check(); err == nil || err.Error() != privateOfflineMessage {
				t.Fatalf("expected offline rejection, got %v", err)
			}
		})
	}
}

func TestEndpointDoesNotChangeOtherNetworkModes(t *testing.T) {
	for _, mode := range []string{"none", "open", "restricted"} {
		s := selection()
		s.Network, s.Sites = mode, "pypi.org"
		got, host, err := withPrivateEndpointAccess(s)
		if err != nil || host != "" || got.Sites != s.Sites {
			t.Fatal("default connection changed", err)
		}
	}
}
