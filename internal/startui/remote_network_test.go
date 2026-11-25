package startui

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

func TestRemoteNetworkInputs(t *testing.T) {
	for _, mode := range []string{"", "open", "none", "restricted"} {
		if err := validateRemoteNetwork(RemoteProfile{Network: mode, Sites: "pypi.org, *.example.org:8443"}); err != nil {
			t.Fatal(mode, err)
		}
	}
	for _, p := range []RemoteProfile{{Network: "typo"}, {Network: "restricted", Sites: "example.org:65536"}, {Network: "restricted", Sites: "example.org:0"}, {Network: "restricted", Sites: "https://example.org"}, {Network: "none", EditorCommit: "$(anything)"}, {EditorExtensions: []string{"--help"}}} {
		if validateRemoteNetwork(p) == nil {
			t.Fatal("accepted invalid policy", p)
		}
	}
	for _, test := range []struct {
		rule, endpoint string
		allowed        bool
	}{
		{"*.example.org", "https://models.example.org/v1", true},
		{"**.example.org", "https://example.org/v1", true},
		{"*.example.org", "https://example.org/v1", false},
		{"example.org", "https://example.org:8443/v1", false},
		{"example.org:8443", "https://example.org:8443/v1", true},
		{"example.org", "https://example.org.evil.org/v1", false},
	} {
		err := validateExistingRemoteEndpoint(RemoteProfile{Network: "restricted", Sites: test.rule}, remoteEndpoint{BaseURL: test.endpoint})
		if (err == nil) != test.allowed {
			t.Fatal(test, err)
		}
	}
	p := normalizeRemoteProfile(RemoteProfile{})
	if p.Network != "open" {
		t.Fatal("legacy profiles changed policy", p)
	}
	p = RemoteProfile{Network: "restricted", Sites: "pypi.org"}
	updated, host, err := remoteEndpointNetwork(p, remoteEndpoint{BaseURL: "https://models.example.org:8443/v1"})
	if err != nil || host != "models.example.org:8443" || !strings.Contains(updated.Sites, host) {
		t.Fatal(updated, host, err)
	}
	if _, _, err := remoteEndpointNetwork(RemoteProfile{Network: "none"}, remoteEndpoint{BaseURL: "https://models.example.org/v1"}); err == nil {
		t.Fatal("offline private endpoint allowed")
	}
}

func TestRemoteNetworkGateway(t *testing.T) {
	for _, file := range []string{"test-remote-network.py", "test-remote-editor.py"} {
		command := exec.Command("python3", "../../scripts/"+file)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatal(string(output), err)
		}
	}
}

func verifyRemoteNetworkLive(t *testing.T, ctx context.Context, s RemoteSession) {
	t.Helper()
	script := `set -eu
node /run/vscode-launcher/install-continue.js check
python3 - "$1" <<'PY'
import json,os,pathlib,socket,sys,urllib.request,urllib.error
mode=sys.argv[1]
assert os.environ['VSCODE_SANDBOX_NETWORK']==mode
assert {name for _,name in socket.if_nameindex()}=={'lo'}
assert socket.gethostbyname('localhost')=='127.0.0.1'
profile=json.loads(pathlib.Path('/run/vscode-launcher/profile.json').read_text())
server=pathlib.Path.home()/'.vscode-server/bin'/profile['editorCommit']
assert (server/'bin/code-server').is_file()
assert (pathlib.Path.home()/'.vscode-server/cli/servers'/('Stable-'+profile['editorCommit'])/'server').resolve()==server
assert list((pathlib.Path.home()/'.vscode-server/extensions').glob('continue.continue-*'))
for address in ('1.1.1.1','2606:4700:4700::1111'):
 family=socket.AF_INET6 if ':' in address else socket.AF_INET
 with socket.socket(family) as stream:
  stream.settimeout(2)
  assert stream.connect_ex((address,443))!=0
# Unsetting proxies must not restore direct internet.
try:urllib.request.build_opener(urllib.request.ProxyHandler({})).open('https://1.1.1.1',timeout=3)
except (OSError,urllib.error.URLError):pass
else:raise AssertionError('Direct proxy bypass worked')
if mode=='restricted':
 assert urllib.request.urlopen('https://pypi.org/simple/pip/',timeout=30).status==200
 for url in ('https://example.com','http://example.com','https://1.1.1.1'):
  try:urllib.request.urlopen(url,timeout=5)
  except (OSError,urllib.error.URLError) as error:assert '403' in str(error),str(error)
  else:raise AssertionError('Unlisted destination worked')
else:
 assert 'HTTPS_PROXY' not in os.environ
 try:urllib.request.urlopen('https://pypi.org',timeout=3)
 except (OSError,urllib.error.URLError):pass
 else:raise AssertionError('Offline internet worked')
print('NETWORK_POLICY_VERIFIED',mode)
PY
node -e 'require("node:dns").lookup("localhost",(error,address)=>{if(error||address!=="127.0.0.1")process.exit(1);console.log("NODE_LOCALHOST_VERIFIED")})'
if [ "$1" = restricted ]; then
 node -e 'fetch("https://pypi.org/simple/pip/").then(r=>{if(r.status!==200)process.exit(1);console.log("NODE_PROXY_VERIFIED")}).catch(e=>{console.error(e);process.exit(1)})'
fi
if command -v Rscript >/dev/null; then
 Rscript --vanilla -e 'stopifnot(capabilities("libcurl")); print(summary(cars)); cat("R_RUNTIME_VERIFIED\n")'
fi
# Actual preloaded server CLI must run offline too.
python3 - <<'PY'
import json,pathlib,subprocess
p=json.loads(pathlib.Path('/run/vscode-launcher/profile.json').read_text())
server=pathlib.Path.home()/'.vscode-server/bin'/p['editorCommit']/'bin/code-server'
listed=subprocess.check_output([str(server),'--list-extensions'],text=True,timeout=30)
assert 'continue.continue' in listed.lower().splitlines(),listed
print(listed,end='')
PY`
	out, err := loginSSH(ctx, "vscode-sandboxes-"+s.Token, script, s.Profile.Network)
	if err != nil {
		t.Fatal(out, err)
	}
	t.Log(strings.TrimSpace(out))
	// Ensure no credentials ever enter metadata (also covers helper profile scope).
	if strings.Contains(remoteProfileJSON(s.Profile), "apiKey") {
		t.Fatal("credentials in profile")
	}
}
