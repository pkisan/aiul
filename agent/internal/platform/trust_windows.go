//go:build windows

package platform

// Windows stub. The real implementation imports the certificate into the
// LocalMachine\Root store (certutil -addstore Root, or the CertStore API), pushed
// by Intune/MDM in production. Not built yet.

type WindowsTrust struct{}

func Trust() TrustInstaller { return WindowsTrust{} }

func (WindowsTrust) InstallCommands(certPath string) []string   { return nil }
func (WindowsTrust) UninstallCommands(certPath string) []string { return nil }
func (WindowsTrust) Install(certPath string) error              { return ErrUnsupported }
func (WindowsTrust) Uninstall(certPath string) error            { return ErrUnsupported }
func (WindowsTrust) IsTrusted(commonName string) (bool, error)  { return false, ErrUnsupported }
