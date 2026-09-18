//go:build darwin

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// systemKeychain is the machine-wide keychain. A certificate trusted here is
// trusted by Safari, Chrome, curl and most macOS software for every user on the
// Mac. The login keychain would only cover one user and would not be seen by
// processes such as a launch daemon.
const systemKeychain = "/Library/Keychains/System.keychain"

// DarwinTrust implements TrustInstaller using the macOS `security` command.
type DarwinTrust struct{}

// Trust returns the TrustInstaller for this operating system.
func Trust() TrustInstaller { return DarwinTrust{} }

func (DarwinTrust) InstallCommands(certPath string) []string {
	return []string{
		// -d          add to the admin (machine-wide) trust domain
		// -r trustRoot treat it as a trusted root certificate
		// -p ssl      trust it for TLS only, not for signing code or anything else
		// -k          the keychain to put it in
		fmt.Sprintf("sudo security add-trusted-cert -d -r trustRoot -p ssl -k %s %s", systemKeychain, shellQuote(certPath)),
	}
}

func (DarwinTrust) UninstallCommands(certPath string) []string {
	return []string{
		fmt.Sprintf("sudo security remove-trusted-cert -d %s", shellQuote(certPath)),
		fmt.Sprintf("sudo security delete-certificate -c %q %s", "AIUL Dev Root", systemKeychain),
	}
}

func (DarwinTrust) Install(certPath string) error {
	if _, err := os.Stat(certPath); err != nil {
		return fmt.Errorf("certificate not found: %w", err)
	}
	// sudo is used rather than requesting rights in-process so the user sees the
	// familiar password prompt and can cancel it.
	cmd := exec.Command("sudo", "security", "add-trusted-cert",
		"-d", "-r", "trustRoot", "-p", "ssl", "-k", systemKeychain, certPath)
	return runVisible(cmd)
}

func (DarwinTrust) Uninstall(certPath string) error {
	// remove-trusted-cert drops the trust setting; delete-certificate removes the
	// certificate itself. Either may legitimately fail if it was never installed,
	// so a failure here is reported but not fatal to the caller's intent.
	if _, err := os.Stat(certPath); err == nil {
		_ = runVisible(exec.Command("sudo", "security", "remove-trusted-cert", "-d", certPath))
	}
	_ = runVisible(exec.Command("sudo", "security", "delete-certificate", "-c", "AIUL Dev Root", systemKeychain))

	trusted, err := DarwinTrust{}.IsTrusted("AIUL Dev Root")
	if err != nil {
		return err
	}
	if trusted {
		return fmt.Errorf("the certificate is still in %s; remove it by hand in Keychain Access, or run scripts/killswitch.sh", systemKeychain)
	}
	return nil
}

func (DarwinTrust) IsTrusted(commonName string) (bool, error) {
	// find-certificate exits non-zero when nothing matches, which is a normal
	// "no" rather than a failure.
	out, err := exec.Command("security", "find-certificate", "-c", commonName, systemKeychain).CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "could not be found") || len(out) == 0 {
			return false, nil
		}
		return false, nil
	}
	return len(out) > 0, nil
}

// runVisible runs a command with the user's terminal attached, so sudo can ask for
// a password and the user sees exactly what happened.
func runVisible(cmd *exec.Cmd) error {
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// shellQuote wraps a path in single quotes when it contains anything awkward, so
// the command we print can be copied and pasted safely.
func shellQuote(s string) string {
	if !strings.ContainsAny(s, " \t'\"$&|;()<>*?[]{}!#~`\\") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
