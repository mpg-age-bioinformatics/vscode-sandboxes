package startui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/mpg-age-bioinformatics/vscode-sandboxes/internal/launcherbundle"
)

func agentNetworkScript() string {
	script, err := launcherbundle.CommonAsset("agent-network.py")
	if err != nil {
		panic(err)
	}
	return string(script)
}

func agentNetworkInstallScript() string {
	prepare, err := launcherbundle.CommonAsset("prepare-continue.py")
	if err != nil {
		panic(err)
	}
	install, err := launcherbundle.CommonAsset("install-continue.js")
	if err != nil {
		panic(err)
	}
	return "\nif ! node -e " + remoteQuote(string(install)) + " sandbox-install check; then\nbundle=$(python3 -I -c " + remoteQuote(string(prepare)) + ")\ntar -C \"$(dirname \"$bundle\")\" -cf - \"$(basename \"$bundle\")\" | node -e " + remoteQuote(string(install)) + " sandbox-install install\nfi\n"
}

func applyLocalAgentNetwork(name, workspace string, s Selection, log io.Writer) error {
	data, err := json.Marshal(map[string]string{"network": s.Network, "sites": s.Sites})
	if err != nil {
		return err
	}
	script := "set -eu\n/usr/bin/python3 -I -c " + remoteQuote(agentNetworkScript()) + agentNetworkInstallScript()
	cmd := Command("sbx", "exec", "-i", name, "sh", "-c", script)
	cmd.Stdin = bytes.NewReader(data)
	cmd.Stdout = log
	cmd.Stderr = log
	hideWindow(cmd)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Configure sandbox web tools: %w", err)
	}
	if s.Sandbox != "bioinformatics-sandbox" {
		script = "set -eu\nunset DOCKER_CONTEXT\nexport DOCKER_HOST=unix:///var/run/docker.sock\ncontainer=$(docker ps -q --filter " + remoteQuote("label=devcontainer.local_folder="+workspace) + ")\n[ -n \"$container\" ] && [ \"$(printf '%s\\n' \"$container\" | wc -l)\" -eq 1 ] || { echo 'Could not identify the exact development container.' >&2; exit 1; }\ndocker exec -i \"$container\" sh -c " + remoteQuote("set -eu\n/usr/bin/python3 -I -c "+remoteQuote(agentNetworkScript())+agentNetworkInstallScript())
		cmd = Command("sbx", "exec", "-i", name, "sh", "-c", script)
		cmd.Stdin = bytes.NewReader(data)
		cmd.Stdout = log
		cmd.Stderr = log
		hideWindow(cmd)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("Configure development-container web tools: %w", err)
		}
	}
	fmt.Fprintln(log, "Agent web policy applied. Hosted search uses domain filters including subdomains; port/IP rules are omitted. Continue hosted search is disabled in restricted/offline modes. VS Code Chat Anthropic BYOK search is configured; broader Chat fetch/browser filtering requires desktop-wide policy. Other providers/MCP tools need separate controls.")
	return nil
}
