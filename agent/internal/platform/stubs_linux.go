//go:build linux

package platform

// Linux stubs. Each has a note on what the real implementation will do, so the
// port is a matter of filling these in rather than rediscovering the design.

// LinuxProxy: env vars in /etc/environment plus profile.d, or the desktop
// environment's own proxy settings.
type LinuxProxy struct{}

func Proxy() ProxyConfigurator { return LinuxProxy{} }

func (LinuxProxy) SetCommands(string) []string         { return nil }
func (LinuxProxy) UnsetCommands() []string             { return nil }
func (LinuxProxy) Set(string) error                    { return ErrUnsupported }
func (LinuxProxy) Unset() error                        { return ErrUnsupported }
func (LinuxProxy) Current() (map[string]string, error) { return nil, ErrUnsupported }

// LinuxEnv: /etc/environment and /etc/profile.d/aiul.sh.
type LinuxEnv struct{}

func Env() EnvWriter { return LinuxEnv{} }

func (LinuxEnv) WriteCommands(EnvVars) []string { return nil }
func (LinuxEnv) RemoveCommands() []string       { return nil }
func (LinuxEnv) Write(EnvVars) error            { return ErrUnsupported }
func (LinuxEnv) Remove() error                  { return ErrUnsupported }
func (LinuxEnv) Current() (EnvVars, error)      { return nil, ErrUnsupported }

// LinuxMDM: Linux has no universal enrolment concept; this will rely on a
// provisioned configuration file or a fleet marker.
type LinuxMDM struct{}

func MDM() MDMChecker { return LinuxMDM{} }

func (LinuxMDM) Enrolled() (bool, string, error) {
	return false, "MDM detection is not implemented on Linux yet", ErrUnsupported
}

// LinuxTools: look for the same commands on PATH.
type LinuxTools struct{}

func Tools() ToolDetector { return LinuxTools{} }

func (LinuxTools) Detect() []Tool { return nil }

// LinuxService: a systemd unit.
type LinuxService struct{}

// WorkerLogPath: the journal, once there is a unit to read it from.
const WorkerLogPath = "journalctl -u aiul"

func Service() ServiceManager { return LinuxService{} }

func (LinuxService) InstallCommands() []string   { return nil }
func (LinuxService) UninstallCommands() []string { return nil }
func (LinuxService) Install(string, map[string]string) error {
	return ErrUnsupported
}
func (LinuxService) Uninstall() error       { return ErrUnsupported }
func (LinuxService) Running() (bool, error) { return false, ErrUnsupported }

// AllManagedVars is shared with darwin so callers need no build tags.
var AllManagedVars = []string{
	"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy",
	"NO_PROXY", "no_proxy",
	"NODE_EXTRA_CA_CERTS", "NODE_USE_SYSTEM_CA",
	"SSL_CERT_FILE", "REQUESTS_CA_BUNDLE",
	"CODEX_CA_CERTIFICATE", "CLAUDE_CODE_CERT_STORE",
}

// InstalledBinaryPath is where the agent binary lives once installed.
const InstalledBinaryPath = "/usr/local/bin/aiul"

// DevAllowUnmanagedVar matches darwin.
const DevAllowUnmanagedVar = "AIUL_DEV_ALLOW_UNMANAGED"

// Device token storage. Linux will use the Secret Service API (libsecret) or a
// root-only file with mode 0600 on a server without a desktop keyring.
func SetDeviceToken(string) error  { return ErrUnsupported }
func DeviceToken() (string, error) { return "", nil }
func DeleteDeviceToken() error     { return nil }

// LinuxProcess: read /proc/net/tcp to map a port to an inode, then find the
// process holding that inode in /proc/<pid>/fd, and read /proc/<pid>/cwd.
type LinuxProcess struct{}

func Processes() ProcessFinder { return LinuxProcess{} }

func (LinuxProcess) ByLocalPort(int) (Process, error) { return Process{}, ErrUnsupported }
func (LinuxProcess) WorkingDir(int) (string, error)   { return "", ErrUnsupported }

// Service account. On Linux this is `useradd --system aiul`.
const (
	ServiceUserName  = "aiul"
	ServiceGroupName = "aiul"
)

func ServiceAccount() (int, int)              { return -1, -1 }
func CreateServiceAccountCommands() []string  { return nil }
func CreateServiceAccount() (int, int, error) { return -1, -1, ErrUnsupported }
func DeleteServiceAccount() error             { return ErrUnsupported }
