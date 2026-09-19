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
