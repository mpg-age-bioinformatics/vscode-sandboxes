package startui

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestNativeWindowCapabilityOnlyUsesPipe(t *testing.T) {
	address := "http://127.0.0.1:12345/" + strings.Repeat("a", 64) + "/"
	for _, platform := range []string{"darwin", "windows"} {
		command, err := startupWindowCommand(platform, "fixture-window", address, "Fixture")
		if err != nil {
			t.Fatal(err)
		}
		for _, value := range append(command.Args, command.Env...) {
			if strings.Contains(value, address) || strings.Contains(value, strings.Repeat("a", 64)) {
				t.Fatal("capability in argv or environment", platform)
			}
		}
		data, err := io.ReadAll(command.Stdin)
		if err != nil {
			t.Fatal(err)
		}
		var bootstrap map[string]string
		if json.Unmarshal(data, &bootstrap) != nil || bootstrap["url"] != address || bootstrap["title"] != "Fixture" {
			t.Fatal("private bootstrap failed", platform)
		}
	}
}
