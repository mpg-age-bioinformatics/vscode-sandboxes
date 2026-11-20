package startui

import (
	"reflect"
	"strings"
	"testing"
)

func TestRemoteDefaultsAndLegacyPreferences(t *testing.T) {
	p := defaultRemoteProfile()
	if p.Project != "~/sandboxes" || p.Partition != "cluster" || p.CPUs != 1 || p.Account != "" {
		t.Fatal(p)
	}
	old := normalizeRemoteProfile(RemoteProfile{Data: " /shared/data ", Account: "old-charge-account", Modules: "source /site/init.sh", CPUs: 4})
	if old.Data != "" || old.Account != "" || old.Folders != "/shared/data" || old.Modules != "source /site/init.sh" || old.CPUs != 4 {
		t.Fatal("lost saved preferences or retained account override", old)
	}
	p = remoteTestProfile()
	p.Project = "~/sandboxes"
	p.Account = "ignored even if invalid"
	p.Folders = " /shared/data , ~/references , , /shared/data "
	p.ModuleNames = "apptainer , gcc/13"
	p = normalizeRemoteProfile(p)
	if err := validateRemoteProfile(p); err != nil {
		t.Fatal(err)
	}
}

func TestRemoteFolderAndModuleLists(t *testing.T) {
	folders, err := remoteFolders(" /shared/data ,~/references, , /shared/data/ , /shared/my folder , ")
	if err != nil || !reflect.DeepEqual(folders, []string{"/shared/data", "~/references", "/shared/my folder"}) {
		t.Fatal(folders, err)
	}
	modules, err := remoteModuleNames(" apptainer , gcc/13.2 , , apptainer ")
	if err != nil || !reflect.DeepEqual(modules, []string{"apptainer", "gcc/13.2"}) {
		t.Fatal(modules, err)
	}
	for _, value := range []string{"relative", "/", "~/../workspace", "/data/../workspace", "/run/vscode-launcher", "/shared/a:rw", "/shared/a\nb"} {
		if _, err := remoteFolders(value); err == nil {
			t.Fatal("accepted invalid folder", value)
		}
	}
	for _, value := range []string{"module load apptainer", "apptainer;touch bad", "$(whoami)", "-x", "apptainer\ngcc"} {
		if _, err := remoteModuleNames(value); err == nil {
			t.Fatal("accepted a command as a module", value)
		}
	}
	p := normalizeRemoteProfile(RemoteProfile{ModuleNames: " apptainer , gcc/13 , apptainer ", Folders: " /a , /a/ "})
	if p.ModuleNames != "apptainer, gcc/13" || p.Folders != "/a" {
		t.Fatal(p)
	}
	script := remoteModuleScript(p)
	if !strings.Contains(script, "module load 'apptainer' 'gcc/13'") || !strings.Contains(script, "/etc/profile.d/modules.sh") {
		t.Fatal(script)
	}
}
