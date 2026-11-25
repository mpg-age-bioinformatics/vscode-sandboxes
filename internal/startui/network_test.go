package startui

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const denyDefault = "POLICY SOURCE APPLIES TO SUMMARY\ndefault-deny-all local all network: deny when no rule decides\n"
const ruleTable = `SOURCE APPLIES TO POLICY POLICY_ID RULE RULE_ID TYPE DECISION RESOURCES
kit sandbox:test-sandbox codex kit-id api api-id network allow api.openai.com
local all - local-policy - custom-id network allow example.com
local sandbox:test-sandbox - local-policy - guard-id network deny **
`

func mockSBX(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture runs on Unix")
	}
	isolatedSettings(t)
	dir := t.TempDir()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TRACE", filepath.Join(dir, "trace"))
	t.Setenv("GLOBAL", filepath.Join(dir, "global"))
	t.Setenv("TABLE", filepath.Join(dir, "table"))
	t.Setenv("AUTH_MARKER", filepath.Join(dir, "auth"))
	t.Setenv("DECISION", "")
	os.WriteFile(os.Getenv("GLOBAL"), []byte(denyDefault), 0600)
	os.WriteFile(os.Getenv("TABLE"), []byte(ruleTable), 0600)
	script := `#!/bin/bash
set -eu
printf '%s\n' "$*" >> "$TRACE"
case "$1 ${2:-}" in
 'daemon status') echo '{"status":"running"}';;
 'policy ls') if [[ "${3:-}" == test-sandbox ]];then cat "$TABLE";else cat "$GLOBAL";fi;;
 'policy check') if [[ -n "$DECISION" ]];then printf '%s' "$DECISION";exit 1;elif [[ "$(cat "$GLOBAL")" == *default-allow-all* ]];then echo '{"allowed":true,"context":"global","type":"network"}';else echo '{"allowed":false,"context":"global","type":"network","deny_kind":"implicit"}';exit 1;fi;;
 'policy init') printf 'POLICY SOURCE APPLIES TO SUMMARY\ndefault-deny-all local all network: deny when no rule decides\n' > "$GLOBAL";;
 'policy reset') ;;
 *) if [[ "$1" == ls ]];then echo 'other-project running';elif [[ "$1" == diagnose && "${AUTH_REQUIRED:-0}" == 1 && ! -e "$AUTH_MARKER" ]];then echo 'Authentication required: run sbx login';exit 1;elif [[ "$1" == login ]];then touch "$AUTH_MARKER";fi;;
esac
`
	// diagnose/login have no second argument.
	if err := os.WriteFile(filepath.Join(dir, "sbx"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	return os.Getenv("TRACE")
}
func trace(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
func TestRestrictedNetworkRemovesKitExtrasAndKeepsGuardUntilReady(t *testing.T) {
	path := mockSBX(t)
	s := selection()
	s.Network = "restricted"
	s.Sites = "example.com, models.example.org"
	// Saved lists for other types and bundled defaults must never add hidden access.
	s.SiteLists = map[string]string{"r-sandbox": "cloud.r-project.org", "python-sandbox": "pypi.org"}
	if err := applyNetwork("test-sandbox", s, io.Discard); err != nil {
		t.Fatal(err)
	}
	got := trace(t, path)
	for _, hidden := range []string{"pypi.org", "cloud.r-project.org", "registry-1.docker.io"} {
		if strings.Contains(got, "policy allow network --sandbox test-sandbox "+hidden) {
			t.Fatal("allowed a host absent from the displayed list:", hidden)
		}
	}
	for _, want := range []string{"policy deny network --sandbox test-sandbox api.openai.com", "policy allow network --sandbox test-sandbox models.example.org", "policy rm network --sandbox test-sandbox --resource **"} {
		if !strings.Contains(got, want) {
			t.Error("missing", want, "in", got)
		}
	}
	if !strings.HasSuffix(got, "policy rm network --sandbox test-sandbox --resource **\n") {
		t.Fatal("guard was not the final change:", got)
	}
}
func TestRestrictedNetworkRejectsBroadRulesAndUnknownOutput(t *testing.T) {
	for _, table := range []string{strings.Replace(ruleTable, "api.openai.com", "**", 1), "unrecognized output"} {
		t.Run(table, func(t *testing.T) {
			path := mockSBX(t)
			os.WriteFile(os.Getenv("TABLE"), []byte(table), 0600)
			s := selection()
			s.Network = "restricted"
			s.Sites = "example.com"
			if err := applyNetwork("test-sandbox", s, io.Discard); err == nil {
				t.Fatal("accepted unenforceable policy")
			}
			if strings.Contains(trace(t, path), "--resource **") {
				t.Fatal("removed the deny-all guard")
			}
		})
	}
}
func TestGlobalPolicyConfirmationAndCancel(t *testing.T) {
	for _, apply := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "apply"}[apply], func(t *testing.T) {
			path := mockSBX(t)
			os.WriteFile(os.Getenv("GLOBAL"), []byte("default-allow-all local all\n"), 0600)
			s := selection()
			s.Network = "restricted"
			s.Sites = "example.com"
			asked := false
			err := prepareGlobalPolicy(s, io.Discard, func(message string) bool {
				asked = true
				if !strings.Contains(message, "other-project running") || !strings.Contains(message, "ALL sandboxes") || !strings.Contains(message, "clear local custom policies") {
					t.Fatal(message)
				}
				return apply
			})
			if !asked {
				t.Fatal("did not ask")
			}
			if apply && err != nil {
				t.Fatal(err)
			}
			if !apply && err == nil {
				t.Fatal("cancel was ignored")
			}
			got := trace(t, path)
			if strings.Contains(got, "policy reset --force") != apply {
				t.Fatal(got)
			}
		})
	}
}

func TestImplicitDefaultDenyWithoutNamedRule(t *testing.T) {
	path := mockSBX(t)
	os.WriteFile(os.Getenv("GLOBAL"), []byte("POLICY SOURCE APPLIES TO SUMMARY\nlocal-policy local all filesystem read: 1 allow\n"), 0600)
	s := selection()
	s.Network, s.Sites = "restricted", "example.com"
	if err := prepareGlobalPolicy(s, io.Discard, func(string) bool {
		t.Fatal("implicit default deny must not prompt for a shared reset")
		return false
	}); err != nil {
		t.Fatal(err)
	}
	if err := applyNetwork("test-sandbox", s, io.Discard); err != nil {
		t.Fatal(err)
	}
	got := trace(t, path)
	if strings.Contains(got, "policy reset") || strings.Contains(got, "policy init") {
		t.Fatal("changed shared policy", got)
	}
	if strings.Count(got, "policy check network --json https://vscode-sandboxes-default-probe.invalid") != 2 || strings.Contains(got, "policy check network --sandbox") {
		t.Fatal("must verify global default at preparation and application", got)
	}
}

func TestInstalledDockerDefaultPolicy(t *testing.T) {
	if os.Getenv("VSCODE_TEST_INSTALLED_DOCKER_POLICY") != "1" {
		t.Skip("opt in to read-only installed Docker policy verification")
	}
	listing, err := output("policy", "ls")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := verifyDenyDefault(listing)
	if err != nil || !ok {
		t.Fatalf("default deny not verified: %v", err)
	}
}

func TestDefaultPolicyVerificationFailsClosed(t *testing.T) {
	for _, response := range []string{
		`{"allowed":true,"context":"global","type":"network"}`,
		`{"allowed":false,"context":"global","type":"network","deny_kind":"explicit"}`,
		`{"allowed":false,"context":"sandbox:test-sandbox","type":"network","deny_kind":"implicit"}`,
		`{"context":"global","type":"network","deny_kind":"implicit"}`,
		`{"allowed":false,"context":"global","type":"filesystem","deny_kind":"implicit"}`,
		`not JSON`,
	} {
		t.Run(response, func(t *testing.T) {
			path := mockSBX(t)
			os.WriteFile(os.Getenv("GLOBAL"), []byte("POLICY SOURCE APPLIES TO SUMMARY\n"), 0600)
			t.Setenv("DECISION", response)
			s := selection()
			s.Network, s.Sites = "restricted", "example.com"
			if err := applyNetwork("test-sandbox", s, io.Discard); err == nil {
				t.Fatal("accepted unverifiable default")
			}
			if got := trace(t, path); strings.Contains(got, "--resource **") || strings.Contains(got, "policy allow") {
				t.Fatal("opened network before verifying default", got)
			}
		})
	}
}
func TestOpenToOfflineTransition(t *testing.T) {
	path := mockSBX(t)
	s := selection()
	if err := applyNetwork("test-sandbox", s, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := guardNetwork("test-sandbox", io.Discard); err != nil {
		t.Fatal(err)
	}
	s.Network = "none"
	if err := applyNetwork("test-sandbox", s, io.Discard); err != nil {
		t.Fatal(err)
	}
	got := trace(t, path)
	if !strings.HasSuffix(got, "policy rm network --sandbox test-sandbox --resource **\npolicy deny network --sandbox test-sandbox **\n") {
		t.Fatal(got)
	}
}
func TestAutomaticDockerLogin(t *testing.T) {
	path := mockSBX(t)
	t.Setenv("AUTH_REQUIRED", "1")
	if err := EnsureHost(io.Discard); err != nil {
		t.Fatal(err)
	}
	if strings.Count(trace(t, path), "login\n") != 1 {
		t.Fatal("did not log in once")
	}
	if err := EnsureHost(io.Discard); err != nil {
		t.Fatal(err)
	}
	if strings.Count(trace(t, path), "login\n") != 1 {
		t.Fatal("unnecessarily repeated login")
	}
}
func TestPolicyParsingAndOverlap(t *testing.T) {
	resources, err := allowedResources(ruleTable)
	if err != nil || len(resources) != 2 {
		t.Fatal(resources, err)
	}
	if !overlap("**.example.com", "api.example.com:443") || overlap("*.example.com", "pypi.org") {
		t.Fatal("incorrect overlap")
	}
	if hasDenyDefault("not-default-deny-all local all") {
		t.Fatal("accepted wrong default")
	}
	resources, err = allowedResources(strings.Replace(ruleTable, "api.openai.com", "api.openai.com, auth.openai.com", 1))
	if err != nil || len(resources) != 3 {
		t.Fatal("dropped a multi-host allow:", resources, err)
	}
}

func TestEndpointInstallationRestoresSandboxConfiguration(t *testing.T) {
	mockSBX(t)
	remote := t.TempDir()
	t.Setenv("REMOTE_HOME", remote)
	os.MkdirAll(filepath.Join(remote, ".codex"), 0700)
	config := filepath.Join(remote, ".codex", "config.toml")
	os.WriteFile(config, []byte("model = 'original-model'\n"), 0600)
	originalContinue := "name: Existing Continue settings\nversion: 1.0.0\nschema: v1\nmodels: []\n"
	os.MkdirAll(filepath.Join(remote, ".continue"), 0700)
	os.WriteFile(filepath.Join(remote, ".continue", "config.yaml"), []byte(originalContinue), 0600)

	mock := filepath.Join(filepath.Dir(os.Getenv("TRACE")), "sbx")
	original, err := os.ReadFile(mock)
	if err != nil {
		t.Fatal(err)
	}
	// Run the exact installation script in a temporary simulated sandbox home.
	script := strings.Replace(string(original), "case \"$1 ${2:-}\" in", `if [[ "$1" == exec ]]; then
 shift
 [[ "$1" == -i ]] && shift
 shift
 export HOME="$REMOTE_HOME"
 exec "$@"
fi
case "$1 ${2:-}" in`, 1)
	os.WriteFile(mock, []byte(script), 0755)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[{"id":"private-model"},{"id":"other-private-model"}]}`)
	}))
	defer gateway.Close()
	private := filepath.Join(t.TempDir(), "private.toml")
	os.WriteFile(private, []byte("model = 'private-model'\nmodel_provider = 'company'\n[model_providers.company]\nbase_url = '"+gateway.URL+"/v1'\n"), 0600)
	s := selection()
	s.Endpoint = "private"
	s.ConfigFile = private
	s.Token = "sandbox-key"
	s.Network = "open"
	for i := 0; i < 2; i++ {
		if err := applyStartup("test-sandbox", "codex", s, io.Discard); err != nil {
			t.Fatal(err)
		}
	}
	data, _ := os.ReadFile(config)
	if strings.Contains(string(data), "private-model") || strings.Contains(string(data), "sandbox-key") {
		t.Fatal(string(data))
	}
	continuePath := filepath.Join(remote, ".continue", "config.yaml")
	continueData, err := os.ReadFile(continuePath)
	if err != nil || !strings.Contains(string(continueData), "other-private-model") || !strings.Contains(string(continueData), "sandbox-key") {
		t.Fatal("Continue was not configured", err)
	}
	if strings.Contains(string(data), "model_catalog_json") {
		t.Fatal("Codex retained a private catalog")
	}
	info, _ := os.Stat(config)
	if info.Mode().Perm() != 0600 {
		t.Fatal("sandbox config is not private")
	}
	s.Endpoint = "default"
	s.ConfigFile = ""
	if err := applyStartup("test-sandbox", "codex", s, io.Discard); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(config)
	restoredContinue, err := os.ReadFile(continuePath)
	if err != nil || string(restoredContinue) != originalContinue {
		t.Fatal("default connection did not restore Continue settings", err)
	}
	if string(data) != "model = 'original-model'\n" {
		t.Fatal("did not restore original:", string(data))
	}
}
