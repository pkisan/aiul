//go:build windows

package platform

import (
	"os"

	"golang.org/x/sys/windows"
)

// Windows layout. Everything the installed agent owns sits under one folder in
// ProgramData (the machine-wide application data folder), so the kill switch
// and a person looking for it have one place to go.
//
// ponytail: C:\ProgramData is assumed rather than read from %ProgramData%;
// these are constants the shared code compiles against. Change here if a
// machine ever moves it.
const (
	dataDir = `C:\ProgramData\AIUL`

	// WorkerStateDir holds the worker's CA copy, spool and the helper's socket.
	// Its access list admits SYSTEM, Administrators and the worker only.
	WorkerStateDir = dataDir + `\state`

	// LogDir holds both services' logs: a service has no console.
	LogDir        = dataDir + `\logs`
	WorkerLogPath = LogDir + `\agent.err.log`

	// PublicCADir holds the certificates every user must read (SSL_CERT_FILE and
	// friends point here). ProgramData lets every user read by default.
	PublicCADir = dataDir + `\public`

	// AgentConfigPath holds the endpoint and device token; readable by SYSTEM,
	// Administrators and the worker.
	AgentConfigPath = dataDir + `\agent.conf`

	// InstalledBinaryPath: Program Files is writable only by administrators,
	// which is what a binary that runs as SYSTEM needs.
	InstalledBinaryPath = `C:\Program Files\AIUL\aiul.exe`

	// The two services, as launchd's two jobs on macOS.
	HelperServiceName = "aiul-helper"
	WorkerServiceName = "aiul"

	// DevAllowUnmanagedVar matches darwin and linux.
	DevAllowUnmanagedVar = "AIUL_DEV_ALLOW_UNMANAGED"
)

// The worker runs as a "virtual account": an identity Windows creates for one
// service, named after it, with no password and no rights beyond what we grant.
// Unlike the shared LocalService account, access lists can name it alone.
const (
	ServiceUserName  = `NT SERVICE\` + WorkerServiceName
	ServiceGroupName = ServiceUserName
)

// IsAdmin reports whether this process runs elevated ("Run as administrator",
// or SYSTEM). Being in the Administrators group is not enough on Windows: an
// unelevated window runs with those rights switched off.
func IsAdmin() bool { return windows.GetCurrentProcessToken().IsElevated() }

// The virtual account comes and goes with the worker service, so there is no
// separate account to create or delete. uid/gid have no meaning here.
func ServiceAccount() (int, int) {
	if serviceExists(WorkerServiceName) {
		return 0, 0
	}
	return -1, -1
}

func CreateServiceAccountCommands() []string {
	return []string{"# none: Windows creates " + ServiceUserName + " with the service in step 2"}
}
func CreateServiceAccount() (int, int, error) { return 0, 0, nil }
func DeleteServiceAccount() error             { return nil }

// WindowsMDM: dev override only for now. Real detection (W5) reads
// HKLM\SOFTWARE\Microsoft\Enrollments or `dsregcmd /status`.
type WindowsMDM struct{}

func MDM() MDMChecker { return WindowsMDM{} }

func (WindowsMDM) Enrolled() (bool, string, error) {
	if os.Getenv(DevAllowUnmanagedVar) == "1" {
		return true, "DEVELOPER OVERRIDE: " + DevAllowUnmanagedVar + "=1 is set, so the MDM check was skipped. This must never be set on a real device.", nil
	}
	return false, "MDM detection is not built for Windows yet. Only the developer override installs it.", nil
}

// WindowsTools: not built yet; `aiul doctor` then lists no tools.
type WindowsTools struct{}

func Tools() ToolDetector { return WindowsTools{} }

func (WindowsTools) Detect() []Tool { return nil }

// The device token lives in agent.conf, as on Linux.
func SetDeviceToken(string) error  { return ErrUnsupported }
func DeviceToken() (string, error) { return "", nil }
func DeleteDeviceToken() error     { return nil }

// AllManagedVars is every variable this agent may set.
var AllManagedVars = []string{
	"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy",
	"NO_PROXY", "no_proxy",
	"NODE_EXTRA_CA_CERTS", "NODE_USE_SYSTEM_CA",
	"SSL_CERT_FILE", "REQUESTS_CA_BUNDLE",
	"CODEX_CA_CERTIFICATE", "CLAUDE_CODE_CERT_STORE",
}
