package startui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// A deny-all preset is necessary because an allow rule alone cannot narrow an
// allow-all preset, and a wildcard deny would also block selected sites.
func prepareGlobalPolicy(s Selection, log io.Writer, confirm func(string) bool) error {
	listing, err := output("policy", "ls")
	if err != nil {
		if !strings.Contains(strings.ToLower(listing), "not been initialized") && !strings.Contains(strings.ToLower(listing), "not initialized") {
			return fmt.Errorf("Inspect network policy: %s", listing)
		}
		preset := "allow-all"
		if s.Network == "restricted" || s.Network == "none" {
			preset = "deny-all"
		}
		fmt.Fprintln(log, "Initializing Docker network policy:", preset)
		return command(log, "policy", "init", preset)
	}
	if s.Network != "restricted" {
		return nil
	}
	denyDefault, err := verifyDenyDefault(listing)
	if err != nil {
		return err
	}
	if denyDefault {
		return nil
	}
	other, err := output("ls")
	if err != nil {
		return fmt.Errorf("List sandboxes before shared policy change: %w", err)
	}
	message := "Restricted mode needs Docker's shared deny-by-default policy. This change applies to ALL sandboxes. Docker will clear local custom policies and stop running sandboxes when it restarts the daemon. Other sandboxes will need to be started again, and may need their allowed sites reconfigured.\n\nCurrent sandboxes:\n" + other + "\nApply this shared change, or cancel this startup?"
	if !confirm(message) {
		return errors.New("Startup cancelled. Shared network policy was unchanged.")
	}
	// A new sandbox may have started while the confirmation was open. Show the
	// changed inventory before performing a reset which would stop it too.
	latest, err := output("ls")
	if err != nil {
		return err
	}
	if latest != other {
		return prepareGlobalPolicy(s, log, confirm)
	}
	fmt.Fprintln(log, "Applying the confirmed shared policy change…")
	if err := command(log, "policy", "reset", "--force"); err != nil {
		return err
	}
	if err := command(log, "policy", "init", "deny-all"); err != nil {
		return err
	}
	// Reset removed all launcher-owned rules too; stale manifests must not
	// make the next startup try to remove rules which no longer exist.
	return os.RemoveAll(filepath.Join(settingsDir(), "network"))
}
func hasDenyDefault(table string) bool {
	for _, line := range strings.Split(table, "\n") {
		f := strings.Fields(line)
		if len(f) > 1 && f[0] == "default-deny-all" && strings.Contains(line, "local") {
			return true
		}
	}
	return false
}

func verifyDenyDefault(table string) (bool, error) {
	if hasDenyDefault(table) {
		return true, nil
	}
	// Docker 0.39 omits the implicit default from policy ls. Ask the daemon's
	// authorizer without a sandbox context: its startup guard would otherwise
	// make an allow-by-default installation appear safe. No network request runs.
	result, err := output("policy", "check", "network", "--json", "https://vscode-sandboxes-default-probe.invalid")
	var denied *exec.ExitError
	if err != nil && (!errors.As(err, &denied) || denied.ExitCode() != 1) {
		return false, fmt.Errorf("Verify Docker's default network decision: %w", err)
	}
	var decision struct {
		Allowed *bool  `json:"allowed"`
		Context string `json:"context"`
		Kind    string `json:"deny_kind"`
		Type    string `json:"type"`
	}
	if err := json.Unmarshal([]byte(result), &decision); err != nil || decision.Allowed == nil || decision.Context != "global" || decision.Type != "network" {
		return false, errors.New("Docker returned an unsupported default network decision; startup remains blocked.")
	}
	if err != nil && *decision.Allowed {
		return false, errors.New("Docker returned a contradictory default network decision; startup remains blocked.")
	}
	return !*decision.Allowed && decision.Kind == "implicit", nil
}

// allowedResources reads the CLI's documented rule-level table, locating
// columns by header instead of hard-coded offsets. Unknown shapes fail closed.
func allowedResources(table string) ([]string, error) {
	var header []string
	var result []string
	for _, line := range strings.Split(table, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if strings.Contains(line, "DECISION") && strings.Contains(line, "RESOURCES") {
			header = fields
			continue
		}
		if header == nil {
			continue
		}
		decision, resource, kind := -1, -1, -1
		for i, h := range header {
			switch h {
			case "DECISION":
				decision = i
			case "RESOURCES":
				resource = i
			case "TYPE":
				kind = i
			}
		}
		if decision < 0 || resource < 0 || kind < 0 {
			return nil, errors.New("Unsupported Docker policy table; cannot verify restricted mode.")
		}
		// APPLIES TO occupies one logical column but two whitespace header tokens.
		applies := -1
		for i := 0; i+1 < len(header); i++ {
			if header[i] == "APPLIES" && header[i+1] == "TO" {
				applies = i
				break
			}
		}
		// Data scope 'all' has one token, 'sandbox:name' has one as well.
		if applies >= 0 {
			if decision > applies {
				decision--
			}
			if resource > applies {
				resource--
			}
			if kind > applies {
				kind--
			}
		}
		if len(fields) <= resource || len(fields) <= kind || len(fields) <= decision {
			return nil, fmt.Errorf("Cannot verify Docker policy row: %s", line)
		}
		if fields[decision] != "allow" && fields[decision] != "deny" {
			return nil, errors.New("Unrecognized policy decision; refusing restricted startup.")
		}
		if strings.HasPrefix(fields[kind], "filesystem:") {
			continue
		}
		if fields[decision] == "allow" {
			if !strings.HasPrefix(fields[kind], "network") && !strings.HasPrefix(fields[kind], "connect:") && !strings.HasPrefix(fields[kind], "http") {
				return nil, fmt.Errorf("Unsupported network rule type %s", fields[kind])
			}
			// RESOURCES can contain several comma-separated hosts with spaces.
			// Never silently drop trailing hosts from an allow rule.
			expected := len(header)
			if applies >= 0 {
				expected--
			}
			resourceText := fields[resource]
			if header[len(header)-1] == "RESOURCES" {
				resourceText = strings.Join(fields[resource:], " ")
			} else if len(fields) != expected {
				return nil, errors.New("Ambiguous Docker policy columns; refusing restricted startup.")
			}
			result = append(result, strings.FieldsFunc(resourceText, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })...)
		}
	}
	if header == nil {
		return nil, errors.New("Docker did not return a rule table; cannot enforce the site list.")
	}
	unique := []string{}
	seen := map[string]bool{}
	for _, resource := range result {
		if !seen[resource] {
			seen[resource] = true
			unique = append(unique, resource)
		}
	}
	return unique, nil
}
func overlap(a, b string) bool {
	// Conservative overlap check: ports and wildcard suffixes must never allow
	// us to install a deny which also covers a requested destination.
	host := func(s string) string { return strings.Split(s, ":")[0] }
	a, b = host(a), host(b)
	if a == "**" || b == "**" || a == b {
		return true
	}
	suffix := func(p, h string) bool {
		if strings.HasPrefix(p, "*.") || strings.HasPrefix(p, "**.") {
			tail := strings.TrimLeft(p, "*")
			return strings.HasSuffix(h, tail)
		}
		return false
	}
	return suffix(a, b) || suffix(b, a)
}
func applyNetwork(name string, s Selection, log io.Writer) error {
	manifest := filepath.Join(settingsDir(), "network", name+".json")
	var previous []string
	if err := readJSON(manifest, &previous); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("Read previous network settings: %w", err)
	}
	// Runtime installs this guard before starting or resuming the sandbox.
	// Only our own previous allow/deny resources are removed.
	for _, resource := range previous {
		if resource == "**" {
			continue
		}
		if err := command(log, "policy", "rm", "network", "--sandbox", name, "--resource", resource); err != nil {
			return fmt.Errorf("Remove previous launcher rule %s: %w", resource, err)
		}
		remaining := []string{}
		for _, p := range previous {
			if p != resource {
				remaining = append(remaining, p)
			}
		}
		if err := writeJSON(manifest, remaining); err != nil {
			return err
		}
		previous = remaining
	}
	if s.Network == "none" {
		fmt.Fprintln(log, "Sandbox internet: blocked (no outbound access).")
		return writeJSON(manifest, []string{})
	}
	var owned []string
	if s.Network == "restricted" {
		listing, err := output("policy", "ls")
		if err != nil {
			return fmt.Errorf("Inspect Docker network policy: %w", err)
		}
		denyDefault, err := verifyDenyDefault(listing)
		if err != nil {
			return err
		}
		if !denyDefault {
			return errors.New("Restricted mode requires the confirmed shared deny-by-default policy. Startup remains blocked.")
		}
		requested, err := sites(s.Sites)
		if err != nil {
			return err
		}
		table, err := output("policy", "ls", name, "--wide", "--type", "all")
		if err != nil {
			return err
		}
		current, err := allowedResources(table)
		if err != nil {
			return err
		}
		set := map[string]bool{}
		for _, site := range requested {
			set[site] = true
		}
		for _, resource := range current {
			covered := set[resource]
			for _, site := range requested {
				if covers(site, resource) {
					covered = true
				}
			}
			if covered {
				continue
			}
			for _, site := range requested {
				if overlap(resource, site) {
					return fmt.Errorf("Existing allow rule %q is broader than requested site %q. Startup stays blocked. Remove or narrow that rule in Docker policy before retrying.", resource, site)
				}
			}
			if resource == "**" {
				return errors.New("An existing allow-all rule prevents enforcing the site list. Startup stays blocked.")
			}
			if err := command(log, "policy", "deny", "network", "--sandbox", name, resource); err != nil {
				return err
			}
			owned = append(owned, resource)
			// Persist after each change so an interrupted startup can be retried.
			if err := writeJSON(manifest, owned); err != nil {
				return err
			}
		}
		for _, site := range requested {
			if !setContains(current, site) {
				if err := command(log, "policy", "allow", "network", "--sandbox", name, site); err != nil {
					return err
				}
				owned = append(owned, site)
				if err := writeJSON(manifest, owned); err != nil {
					return err
				}
			}
		}
		fmt.Fprintln(log, "Sandbox internet: restricted to", strings.Join(requested, ", "))
	} else {
		// Remove our guard first; adding the opposite decision in the same scope is
		// rejected by sbx. External organization/global denies remain authoritative.
		if err := command(log, "policy", "rm", "network", "--sandbox", name, "--resource", "**"); err != nil {
			return err
		}
		if err := command(log, "policy", "allow", "network", "--sandbox", name, "**"); err != nil {
			return err
		}
		// The wildcard allow must be removed before the next startup's guard.
		owned = append(owned, "**")
		fmt.Fprintln(log, "Sandbox internet: open, subject to organization policy.")
		return writeJSON(manifest, owned)
	}
	if err := writeJSON(manifest, owned); err != nil {
		return err
	}
	return command(log, "policy", "rm", "network", "--sandbox", name, "--resource", "**")
}
func setContains(items []string, s string) bool {
	for _, item := range items {
		if item == s {
			return true
		}
	}
	return false
}

func guardNetwork(name string, log io.Writer) error {
	var previous []string
	path := filepath.Join(settingsDir(), "network", name+".json")
	_ = readJSON(path, &previous)
	if setContains(previous, "**") {
		if err := command(log, "policy", "rm", "network", "--sandbox", name, "--resource", "**"); err != nil {
			return err
		}
		kept := []string{}
		for _, p := range previous {
			if p != "**" {
				kept = append(kept, p)
			}
		}
		if err := writeJSON(path, kept); err != nil {
			return err
		}
	}
	return command(log, "policy", "deny", "network", "--sandbox", name, "**")
}

// covers determines whether an existing rule is wholly inside a requested rule.
func covers(allowed, existing string) bool {
	split := func(s string) (string, string) {
		parts := strings.SplitN(s, ":", 2)
		if len(parts) == 2 {
			return parts[0], parts[1]
		}
		return s, ""
	}
	ah, ap := split(allowed)
	eh, ep := split(existing)
	if ap != "" && ap != ep {
		return false
	}
	if ah == eh {
		return true
	}
	if strings.HasPrefix(ah, "**.") {
		return strings.HasSuffix(eh, strings.TrimPrefix(ah, "**"))
	}
	if strings.HasPrefix(ah, "*.") && !strings.Contains(eh, "*") {
		suffix := strings.TrimPrefix(ah, "*")
		return strings.HasSuffix(eh, suffix) && !strings.Contains(strings.TrimSuffix(eh, suffix), ".")
	}
	return false
}
