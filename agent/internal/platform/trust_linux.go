//go:build linux

package platform

// Linux stub. The real implementation copies the PEM into
// /usr/local/share/ca-certificates/ and runs update-ca-certificates (or
// /etc/pki/ca-trust/ and update-ca-trust on RHEL family). Not built yet.

type LinuxTrust struct{}

func Trust() TrustInstaller { return LinuxTrust{} }

func (LinuxTrust) InstallCommands(certPath string) []string   { return nil }
func (LinuxTrust) UninstallCommands(certPath string) []string { return nil }
func (LinuxTrust) Install(certPath string) error              { return ErrUnsupported }
func (LinuxTrust) Uninstall(certPath string) error            { return ErrUnsupported }
func (LinuxTrust) IsTrusted(commonName string) (bool, error)  { return false, ErrUnsupported }
