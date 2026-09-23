//go:build windows

package platform

import "os"

// Windows stubs. Each has a note on what the real implementation will do, so the
// port is a matter of filling these in rather than rediscovering the design.

// WindowsProxy: env vars in /etc/environment plus profile.d, or the desktop
// environment's own proxy settings.
type WindowsProxy struct{}

func Proxy() ProxyConfigurator { return WindowsProxy{} }

func (WindowsProxy) SetCommands(string) []string         { return nil }
func (WindowsProxy) UnsetCommands() []string             { return nil }
func (WindowsProxy) Set(string) error                    { return ErrUnsupported }
func (WindowsProxy) Unset() error                        { return ErrUnsupported }
func (WindowsProxy) Current() (map[string]string, error) { return nil, ErrUnsupported }

// WindowsEnv: HKLM Environment in the registry.
type WindowsEnv struct{}

func Env() EnvWriter { return WindowsEnv{} }

func (WindowsEnv) WriteCommands(EnvVars) []string { return nil }
func (WindowsEnv) RemoveCommands() []string       { return nil }
func (WindowsEnv) Write(EnvVars) error            { return ErrUnsupported }
func (WindowsEnv) Remove() error                  { return ErrUnsupported }
func (WindowsEnv) Current() (EnvVars, error)      { return nil, ErrUnsupported }

// WindowsMDM: the registry and the MDM APIs report enrolment; this will use
// provisioned configuration file or a fleet marker.
type WindowsMDM struct{}

func MDM() MDMChecker { return WindowsMDM{} }

func (WindowsMDM) Enrolled() (bool, string, error) {
	if os.Getenv(DevAllowUnmanagedVar) == "1" {
		return true, "DEVELOPER OVERRIDE: " + DevAllowUnmanagedVar + "=1 is set, so the MDM check was skipped. This must never be set on a real device.", nil
	}
	return false, "MDM detection is not implemented on Windows yet", ErrUnsupported
}

// WindowsTools: look for the same commands on PATH.
type WindowsTools struct{}

func Tools() ToolDetector { return WindowsTools{} }

func (WindowsTools) Detect() []Tool { return nil }

// WindowsService: a Windows Service.
type WindowsService struct{}

// WorkerLogPath: the Windows event log, once there is a service to write to it.
const WorkerLogPath = "Event Viewer > Application > aiul"

func Service() ServiceManager { return WindowsService{} }

func (WindowsService) InstallCommands() []string   { return nil }
func (WindowsService) UninstallCommands() []string { return nil }
func (WindowsService) Install(string, map[string]string) error {
	return ErrUnsupported
}
func (WindowsService) Uninstall() error       { return ErrUnsupported }
func (WindowsService) Running() (bool, error) { return false, ErrUnsupported }

// AllManagedVars is shared with darwin so callers need no build tags.
var AllManagedVars = []string{
	"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy",
	"NO_PROXY", "no_proxy",
	"NODE_EXTRA_CA_CERTS", "NODE_USE_SYSTEM_CA",
	"SSL_CERT_FILE", "REQUESTS_CA_BUNDLE",
	"CODEX_CA_CERTIFICATE", "CLAUDE_CODE_CERT_STORE",
}

// InstalledBinaryPath is where the agent binary lives once installed.
const InstalledBinaryPath = "C:\\Program Files\\AIUL\\aiul.exe"

// DevAllowUnmanagedVar matches darwin.
const DevAllowUnmanagedVar = "AIUL_DEV_ALLOW_UNMANAGED"

// Device token storage. Windows will use the Credential Manager (DPAPI).
func SetDeviceToken(string) error  { return ErrUnsupported }
func DeviceToken() (string, error) { return "", nil }
func DeleteDeviceToken() error     { return nil }

// WindowsProcess: GetExtendedTcpTable maps a port to a process id; the working
// directory needs more work on Windows and may not be available at all.
type WindowsProcess struct{}

func Processes() ProcessFinder { return WindowsProcess{} }

func (WindowsProcess) ByLocalPort(int) (Process, error) { return Process{}, ErrUnsupported }
func (WindowsProcess) WorkingDir(int) (string, error)   { return "", ErrUnsupported }

// Service account. On Windows this is a virtual service account.
const (
	ServiceUserName  = "aiul"
	ServiceGroupName = "aiul"
)

func ServiceAccount() (int, int)              { return -1, -1 }
func CreateServiceAccountCommands() []string  { return nil }
func CreateServiceAccount() (int, int, error) { return -1, -1, ErrUnsupported }
func DeleteServiceAccount() error             { return ErrUnsupported }

// PublicCADir is where the certificates every user must be able to read will live.
// On Windows the trust store is the registry, so the port will add them there
// rather than to a file; this constant keeps the shared code compiling.
const PublicCADir = `C:\\ProgramData\\aiul`

// PublishCA will add the public certificates to the machine store.
func PublishCA(string, string) error { return ErrUnsupported }
