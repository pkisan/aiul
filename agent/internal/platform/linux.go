//go:build linux

package platform

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Linux (Ubuntu with GNOME) differs from macOS in one way that shapes most of
// this port: several settings belong to each PERSON rather than to the machine.
// The proxy lives in each user's GNOME settings and Chrome's trusted roots live
// in each user's ~/.pki/nssdb. So the root helper walks the desktop users and
// acts as each of them.

// DevAllowUnmanagedVar lets a developer run the agent on a machine no MDM
// manages. Linux has no MDM enrollment to check, so on Linux this override is the
// only way the agent installs at all: a test setup, never a real deployment.
const DevAllowUnmanagedVar = "AIUL_DEV_ALLOW_UNMANAGED"

// desktopUser is a person who can log in to the desktop.
type desktopUser struct {
	Name string
	UID  int
	Home string
}

// desktopUsers lists the people on this machine: uid 1000 and up with a home
// under /home. System accounts (uid below 1000) are never touched.
func desktopUsers() []desktopUser {
	f, err := os.Open("/etc/passwd")
	if err != nil {
		return nil
	}
	defer f.Close()

	return parsePasswd(bufio.NewScanner(f))
}

func parsePasswd(sc *bufio.Scanner) []desktopUser {
	var out []desktopUser
	for sc.Scan() {
		// name:password:uid:gid:comment:home:shell
		fields := strings.Split(sc.Text(), ":")
		if len(fields) < 7 {
			continue
		}
		uid, err := strconv.Atoi(fields[2])
		if err != nil || uid < 1000 || uid >= 60000 || !strings.HasPrefix(fields[5], "/home/") {
			continue
		}
		out = append(out, desktopUser{Name: fields[0], UID: uid, Home: fields[5]})
	}
	return out
}

// sessionBus is the D-Bus socket of a person's running desktop session, or "" if
// they are not logged in. gsettings talks to dconf over this bus.
func (u desktopUser) sessionBus() string {
	path := fmt.Sprintf("/run/user/%d/bus", u.UID)
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}

// command builds a command that runs as this person. runuser is the root-only
// sibling of su: no password, no PAM login session, just a different user.
//
// With a live session it joins that session's bus, so a running Chrome sees the
// change at once. Without one it starts a private bus (dbus-run-session), which
// still writes the person's saved settings for their next login.
func (u desktopUser) command(name string, args ...string) *exec.Cmd {
	argv := []string{"-u", u.Name, "--"}
	if bus := u.sessionBus(); bus != "" {
		argv = append(argv, "env", "DBUS_SESSION_BUS_ADDRESS=unix:path="+bus)
	} else {
		argv = append(argv, "dbus-run-session", "--")
	}
	argv = append(argv, name)
	argv = append(argv, args...)

	cmd := exec.Command("runuser", argv...)
	// Some tools read $HOME rather than asking the system, and runuser keeps
	// root's.
	cmd.Env = append(os.Environ(), "HOME="+u.Home)
	return cmd
}

// LinuxMDM: there is no MDM enrollment to ask about on Linux, so only the
// developer override lets the agent install.
type LinuxMDM struct{}

func MDM() MDMChecker { return LinuxMDM{} }

func (LinuxMDM) Enrolled() (bool, string, error) {
	if os.Getenv(DevAllowUnmanagedVar) == "1" {
		return true, "DEVELOPER OVERRIDE: " + DevAllowUnmanagedVar + "=1 is set, so the MDM check was skipped. This must never be set on a real device.", nil
	}
	return false, "Linux has no MDM enrollment this agent can check. Only the developer override installs it.", nil
}

// LinuxTools: not built yet; `aiul doctor` then lists no tools.
type LinuxTools struct{}

func Tools() ToolDetector { return LinuxTools{} }

func (LinuxTools) Detect() []Tool { return nil }

// The device token lives in /etc/aiul/agent.conf on Linux (root:_aiul 0640),
// written by the enrol script. There is no keychain to hold a second copy.
func SetDeviceToken(string) error  { return ErrUnsupported }
func DeviceToken() (string, error) { return "", nil }
func DeleteDeviceToken() error     { return nil }

// AllManagedVars is every variable this agent may set, so removal can clear them
// all even if the configuration has changed since they were written.
var AllManagedVars = []string{
	"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy",
	"NO_PROXY", "no_proxy",
	"NODE_EXTRA_CA_CERTS", "NODE_USE_SYSTEM_CA",
	"SSL_CERT_FILE", "REQUESTS_CA_BUNDLE",
	"CODEX_CA_CERTIFICATE", "CLAUDE_CODE_CERT_STORE",
}
