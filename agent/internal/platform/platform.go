// Package platform holds every action that differs between operating systems.
//
// The rule: nothing outside this package may run networksetup, security,
// launchctl or any other OS-specific tool. Porting to Linux and Windows later
// then means writing new files in here and nothing else.
//
// Only the darwin implementation exists today; linux and windows have stub files
// that compile and return ErrUnsupported.
package platform

import "errors"

// AgentConfigPath is where the installed agent reads its endpoint, device token
// and debug setting. An MDM writes this file; `aiul install` makes sure the
// service account can read it, because the worker is not root.
const AgentConfigPath = "/etc/aiul/agent.conf"

// ErrUnsupported is returned by stub implementations on operating systems we have
// not built yet.
var ErrUnsupported = errors.New("not implemented on this operating system yet")

// ErrProxyStateHidden is Proxy().Current() saying it cannot see the setting from
// this account. On Linux the proxy is each person's GNOME setting, which only
// root can read, and the worker is not root. The worker then asks the helper to
// apply the setting again instead of comparing it — which also covers someone
// who logged in after the agent started.
var ErrProxyStateHidden = errors.New("only root can read other users' proxy settings")

// TrustInstaller adds and removes our development root CA from the machine's
// trust store, so software on this device accepts certificates we mint.
type TrustInstaller interface {
	// Commands returns the exact shell commands that Install (or Uninstall) will
	// run, so they can be shown to the user before anything happens. Rule 1.
	InstallCommands(certPath string) []string
	UninstallCommands(certPath string) []string

	// Install adds the certificate at certPath to the trust store. It usually
	// needs administrator rights and will prompt for a password.
	Install(certPath string) error

	// Uninstall removes it again. Removing something that is not there is not an
	// error, so this is safe to run repeatedly.
	Uninstall(certPath string) error

	// IsTrusted reports whether a certificate with this common name is currently
	// present and trusted.
	IsTrusted(commonName string) (bool, error)
}
