package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These tests never change a system setting. They test the pure logic — parsing,
// block editing, and the command lists shown before anything runs.

func TestEveryInterfaceHasAnImplementation(t *testing.T) {
	// If this compiles and runs, the current GOOS has all six.
	if Trust() == nil || Proxy() == nil || Env() == nil || MDM() == nil || Tools() == nil || Service() == nil {
		t.Fatal("an interface has no implementation on this platform")
	}
}

// TestCommandsAreShownBeforeAnythingRuns is rule 1 in test form: every mutating
// operation can describe itself first.
func TestCommandsAreShownBeforeAnythingRuns(t *testing.T) {
	if got := Proxy().SetCommands("127.0.0.1:8899"); len(got) == 0 {
		t.Error("ProxyConfigurator.SetCommands returned nothing to show the user")
	}
	if got := Proxy().UnsetCommands(); len(got) == 0 {
		t.Error("ProxyConfigurator.UnsetCommands returned nothing")
	}
	if got := Env().WriteCommands(EnvVars{"HTTPS_PROXY": "http://127.0.0.1:8899"}); len(got) == 0 {
		t.Error("EnvWriter.WriteCommands returned nothing")
	}
	if got := Service().InstallCommands(); len(got) == 0 {
		t.Error("ServiceManager.InstallCommands returned nothing")
	}
	if got := Trust().InstallCommands("/tmp/root.crt"); len(got) == 0 {
		t.Error("TrustInstaller.InstallCommands returned nothing")
	}
}

func TestSplitHostPort(t *testing.T) {
	cases := map[string][2]string{
		"127.0.0.1:8899": {"127.0.0.1", "8899"},
		"127.0.0.1":      {"127.0.0.1", "8899"},
		"proxy.local:80": {"proxy.local", "80"},
	}
	for in, want := range cases {
		host, port := splitHostPort(in)
		if host != want[0] || port != want[1] {
			t.Errorf("splitHostPort(%q) = %q, %q; want %q, %q", in, host, port, want[0], want[1])
		}
	}
}

// TestStripBlockLeavesOtherContentAlone is the important one: /etc/zshenv may
// already hold something the user needs, and we must never eat it.
func TestStripBlockLeavesOtherContentAlone(t *testing.T) {
	theirs := "# their own settings\nexport EDITOR=vim\n"
	ours := blockBegin + "\nexport HTTPS_PROXY=\"http://127.0.0.1:8899\"\n" + blockEnd + "\n"

	cases := map[string]string{
		theirs + ours:          theirs,
		ours + theirs:          theirs,
		theirs:                 theirs,
		ours:                   "",
		theirs + ours + theirs: theirs + theirs,
	}
	for input, want := range cases {
		if got := stripBlock(input); got != want {
			t.Errorf("stripBlock(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestStripBlockHandlesATruncatedBlock(t *testing.T) {
	// A crash mid-write could leave a begin marker with no end. Removing from the
	// marker onwards is safer than leaving half a block that we would later fail
	// to recognise.
	input := "export EDITOR=vim\n" + blockBegin + "\nexport HTTPS_PROXY=\"x\"\n"
	got := stripBlock(input)
	if strings.Contains(got, "HTTPS_PROXY") || strings.Contains(got, blockBegin) {
		t.Errorf("a truncated block was not removed: %q", got)
	}
	if !strings.Contains(got, "EDITOR=vim") {
		t.Errorf("the user's own content was lost: %q", got)
	}
}

func TestCurrentReadsBackWhatWeWrote(t *testing.T) {
	// Parse the block format without touching the real /etc/zshenv.
	dir := t.TempDir()
	path := filepath.Join(dir, "zshenv")
	content := "# theirs\n" + blockBegin + "\n" +
		"export HTTPS_PROXY=\"http://127.0.0.1:8899\"\n" +
		"export NODE_USE_SYSTEM_CA=\"1\"\n" + blockEnd + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got := parseBlockVars(content)
	if got["HTTPS_PROXY"] != "http://127.0.0.1:8899" || got["NODE_USE_SYSTEM_CA"] != "1" {
		t.Errorf("parsed %v", got)
	}
	if _, ok := got["theirs"]; ok {
		t.Error("content outside our block was parsed as a variable")
	}
}

func TestAllManagedVarsCoversWhatWeSet(t *testing.T) {
	// Whatever the agent writes must also be in the removal list, or uninstall
	// leaves variables behind.
	written := DefaultEnvVars("http://127.0.0.1:8899", "/path/to/root.crt")
	for name := range written {
		found := false
		for _, managed := range AllManagedVars {
			if managed == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s is set by the agent but missing from AllManagedVars, so uninstall would leave it behind", name)
		}
	}
}

func TestDefaultEnvVarsBypassesLocalAddresses(t *testing.T) {
	vars := DefaultEnvVars("http://127.0.0.1:8899", "/path/root.crt")
	noProxy := vars["NO_PROXY"]
	for _, must := range []string{"localhost", "127.0.0.1", "::1"} {
		if !strings.Contains(noProxy, must) {
			t.Errorf("NO_PROXY must contain %q, got %q", must, noProxy)
		}
	}
	if vars["NODE_EXTRA_CA_CERTS"] != "/path/root.crt" {
		t.Errorf("NODE_EXTRA_CA_CERTS = %q", vars["NODE_EXTRA_CA_CERTS"])
	}
}

func TestDetectFindsToolsWithoutCrashing(t *testing.T) {
	// Whatever is installed on the machine running the tests, this must not panic
	// and every result must be complete enough to act on.
	for _, tool := range Tools().Detect() {
		if tool.Name == "" || tool.Path == "" || tool.Kind == "" {
			t.Errorf("incomplete tool record: %+v", tool)
		}
	}
}
