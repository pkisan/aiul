//go:build darwin

package platform

import (
	"strings"
	"testing"
)

// A variable set in the shell that runs `install` does not reach a launchd job.
// The worker refuses to start on an unmanaged Mac, so on a development machine
// the override has to be written into the job definition — otherwise install
// points the system proxy at a worker that will never come up.
func TestTheDaemonPlistCarriesTheEnvironmentTheWorkerNeeds(t *testing.T) {
	plist := daemonPlistXML(daemonLabel, ServiceUserName,
		[]string{InstalledBinaryPath, "run", "--manage-proxy"},
		"/var/log/aiul/agent.log", "/var/log/aiul/agent.err.log",
		map[string]string{DevAllowUnmanagedVar: "1"})

	for _, want := range []string{
		"<key>" + DevAllowUnmanagedVar + "</key>",
		"<key>AIUL_STATE_DIR</key>",
		"<string>" + WorkerStateDir + "</string>",
		// UserName is what keeps the code that parses traffic out of root.
		"<key>UserName</key>\n\t<string>" + ServiceUserName + "</string>",
	} {
		if !strings.Contains(plist, want) {
			t.Errorf("the worker's job definition is missing %q:\n%s", want, plist)
		}
	}
}

// The helper is the half that must stay root, and it must not silently gain a
// UserName key from a copy-paste.
func TestTheHelperJobRunsAsRoot(t *testing.T) {
	plist := daemonPlistXML(helperLabel, "",
		[]string{InstalledBinaryPath, "helper"},
		"/var/log/aiul/helper.log", "/var/log/aiul/helper.err.log", nil)

	if strings.Contains(plist, "UserName") {
		t.Errorf("the helper must run as root, but its job names a user:\n%s", plist)
	}
	if !strings.Contains(plist, "<key>AIUL_STATE_DIR</key>") {
		t.Errorf("both halves must agree where the state directory is:\n%s", plist)
	}
}

// The environment variables the agent writes REPLACE the trust store, so the file
// they name has to be readable by whoever runs the tool. Pointing them at the
// worker's own copy — in a directory only the service account may enter — broke
// curl for every user on the Mac with "error setting certificate verify
// locations".
func TestTheCAIsPublishedWhereEveryUserCanReadIt(t *testing.T) {
	vars := DefaultEnvVars("http://127.0.0.1:8899",
		PublicCADir+"/root.crt", PublicCADir+"/ca-bundle.pem")

	for _, key := range []string{"SSL_CERT_FILE", "REQUESTS_CA_BUNDLE", "NODE_EXTRA_CA_CERTS"} {
		value := vars[key]
		if value == "" {
			t.Errorf("%s is not set", key)

			continue
		}
		if strings.HasPrefix(value, WorkerStateDir) {
			t.Errorf("%s points at %s, which only the service account can read", key, value)
		}
		if !strings.HasPrefix(value, PublicCADir) {
			t.Errorf("%s = %q, want it under %s", key, value, PublicCADir)
		}
	}
}

// The Claude desktop app bundles its own copy of Claude Code, and macOS reports
// the process name of both it and the terminal CLI as "claude". Anything keyed by
// the name alone treats the two as one program, which is how a pinned desktop app
// switched off capture for the terminal CLI.
func TestProcessIdentityPrefersTheExecutable(t *testing.T) {
	desktop := Process{PID: 1, Name: "claude", Path: "/Users/x/Library/Application Support/Claude/claude-code/2.1.275/claude.app/Contents/MacOS/claude"}
	cli := Process{PID: 2, Name: "claude", Path: "/opt/homebrew/bin/claude"}

	if desktop.Identity() == cli.Identity() {
		t.Errorf("two different programs share the identity %q", desktop.Identity())
	}
	// With no path — lsof lost the race, the process is gone — the name is all
	// there is, and is still better than nothing.
	unknown := Process{PID: 3, Name: "claude"}
	if unknown.Identity() != "claude" {
		t.Errorf("Identity() = %q, want the process name", unknown.Identity())
	}
}

// `install` runs under sudo. A plain `launchctl setenv` from root writes root's
// own domain, so the person's GUI applications never see the variable — which is
// why Claude Desktop had no NODE_EXTRA_CA_CERTS and rejected our certificate.
func TestGUIVariablesAreSetInTheLoggedInUsersDomain(t *testing.T) {
	args := guiSetenvArgs(501, "NODE_EXTRA_CA_CERTS", "/usr/local/share/aiul/root.crt")
	want := []string{"launchctl", "asuser", "501", "launchctl", "setenv", "NODE_EXTRA_CA_CERTS", "/usr/local/share/aiul/root.crt"}
	if strings.Join(args, " ") != strings.Join(want, " ") {
		t.Errorf("guiSetenvArgs = %v, want %v", args, want)
	}

	// Nobody at the screen: there is no GUI session to write to, so the plain
	// form is all that is left, and it must not become "asuser 0".
	for _, uid := range []int{0, -1} {
		plain := guiSetenvArgs(uid, "HTTPS_PROXY", "http://127.0.0.1:8899")
		if len(plain) != 4 || plain[1] != "setenv" {
			t.Errorf("guiSetenvArgs(%d) = %v, want a plain setenv", uid, plain)
		}
	}
}
