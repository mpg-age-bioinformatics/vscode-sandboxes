package startui

import (
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestRemoteActivityDownload(t *testing.T) {
	p := &remoteController{state: RemoteStatus{Text: "\x1b[32mJob submitted\x1b[0m\nSlurm startup output\n", Phase: "Waiting for SSH", Error: "Connection failed", Session: RemoteSession{Profile: RemoteProfile{Modules: "private configuration excluded"}}}}
	prerequisites := &prerequisiteSetup{log: "Installed editor\n", err: "Tool setup failed"}
	for _, download := range []bool{false, true} {
		route := "/remote/activity"
		if download {
			route += "?download"
		}
		w := httptest.NewRecorder()
		p.serve("remote/activity", w, httptest.NewRequest("GET", route, nil), nil, prerequisites)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		text := w.Body.String()
		for _, expected := range []string{"Job submitted", "Slurm startup output", "Waiting for SSH", "Connection failed", "Installed editor", "Tool setup failed"} {
			if !strings.Contains(text, expected) {
				t.Fatal("missing activity", expected, text)
			}
		}
		if strings.Contains(text, "\x1b") || strings.Contains(text, "private configuration") {
			t.Fatal("export included formatting or configuration", text)
		}
		disposition := w.Header().Get("Content-Disposition")
		if download && !regexp.MustCompile(`^attachment; filename="vscode-sandox\.[0-9]{8}\.[0-9]{6}\.log"$`).MatchString(disposition) {
			t.Fatal(disposition)
		}
		if !download && disposition != "" {
			t.Fatal(disposition)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("activity must not be cached")
		}
	}
	w := httptest.NewRecorder()
	p.serve("remote/activity", w, httptest.NewRequest("POST", "/remote/activity", nil), nil, nil)
	if w.Code != 405 {
		t.Fatal(w.Code)
	}
}
