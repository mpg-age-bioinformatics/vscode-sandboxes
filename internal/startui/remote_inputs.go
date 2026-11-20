package startui

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

func remoteCSV(value string) []string {
	var result []string
	seen := map[string]bool{}
	for _, part := range strings.Split(value, ",") {
		item := strings.TrimSpace(part)
		if item != "" && !seen[item] {
			result = append(result, item)
			seen[item] = true
		}
	}
	return result
}

func remoteFolders(value string) ([]string, error) {
	folders := remoteCSV(value)
	if len(folders) > 64 {
		return nil, fmt.Errorf("Choose at most 64 additional folders.")
	}
	cleanedFolders := []string{}
	seen := map[string]bool{}
	for _, folder := range folders {
		if !safeRemotePath(folder, true) {
			return nil, fmt.Errorf("Additional folder %q must be an absolute cluster path or start with ~/; colons and control characters are not supported.", folder)
		}
		if strings.HasPrefix(folder, "~/") {
			relative := path.Clean(folder[2:])
			if relative == ".." || strings.HasPrefix(relative, "../") {
				return nil, fmt.Errorf("Use an absolute path for additional folders outside your home.")
			}
			folder = "~/" + relative
		} else {
			folder = path.Clean(folder)
		}
		cleaned := folder
		for _, reserved := range []string{"/workspace", "/run", "/proc", "/sys", "/dev", "/tmp", "/var/tmp"} {
			if cleaned == reserved || strings.HasPrefix(cleaned, reserved+"/") {
				return nil, fmt.Errorf("Additional folder %q would replace a container runtime path.", folder)
			}
		}
		if !seen[folder] {
			cleanedFolders = append(cleanedFolders, folder)
			seen[folder] = true
		}
	}
	return cleanedFolders, nil
}

func remoteModuleNames(value string) ([]string, error) {
	names := remoteCSV(value)
	if len(names) > 64 {
		return nil, fmt.Errorf("Choose at most 64 modules.")
	}
	pattern := regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.+/@-]*$`)
	for _, name := range names {
		if len(name) > 256 || !pattern.MatchString(name) {
			return nil, fmt.Errorf("Enter module names separated by commas, for example apptainer, gcc/13. Do not include 'module load' or shell commands in this field.")
		}
	}
	return names, nil
}

func normalizeRemoteProfile(p RemoteProfile) RemoteProfile {
	p.Account = ""
	if p.Network == "" {
		p.Network = "open"
	}
	if strings.TrimSpace(p.ImagesDir) == "" {
		p.ImagesDir = "~/.local/share/vscode-sandboxes/images"
	}
	if p.Folders == "" && p.Data != "" {
		p.Folders = p.Data
	}
	p.Data = ""
	if folders, err := remoteFolders(p.Folders); err == nil {
		p.Folders = strings.Join(folders, ", ")
	} else {
		p.Folders = strings.Join(remoteCSV(p.Folders), ", ")
	}
	p.ModuleNames = strings.Join(remoteCSV(p.ModuleNames), ", ")
	p.Project = strings.TrimSpace(p.Project)
	if p.Project == "" {
		p.Project = "~/sandboxes"
	}
	p.Partition = strings.TrimSpace(p.Partition)
	if p.Partition == "" {
		p.Partition = "cluster"
	}
	return p
}

func remoteModuleScript(p RemoteProfile) string {
	script := "#!/usr/bin/env bash\n# VS Code Sandboxes login and compute modules\nset -e\n" + p.Modules + "\n"
	names, _ := remoteModuleNames(p.ModuleNames)
	if len(names) == 0 {
		return script
	}
	script += `if ! type module >/dev/null 2>&1; then
  for initialization in /etc/profile.d/modules.sh /etc/profile.d/lmod.sh /usr/share/Modules/init/bash; do
    if [[ -r "$initialization" ]]; then source "$initialization"; break; fi
  done
fi
type module >/dev/null 2>&1 || { echo 'The module command is unavailable; provide site initialization in the advanced options.' >&2; exit 1; }
module load`
	for _, name := range names {
		script += " " + remoteQuote(name)
	}
	return script + "\n"
}
