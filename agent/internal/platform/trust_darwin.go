//go:build darwin

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/pkisan/aiul/internal/ca"
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

func (DarwinTrust) UninstallCommands(string) []string {
	return []string{
		fmt.Sprintf("sudo security find-certificate -a -c %q -p %s > /tmp/aiul-roots.pem", ca.CommonNamePrefix, systemKeychain),
		"sudo security remove-trusted-cert -d /tmp/aiul-roots.pem",
		fmt.Sprintf("sudo security delete-certificate -c %q %s   # repeated until none remain", ca.CommonNamePrefix, systemKeychain),
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

// Uninstall removes every certificate of ours from the System keychain.
//
// The argument is ignored, deliberately. Removal used to be by file path, and the
// path was wrong in the case that matters: the installed agent trusts the CA under
// /var/db/aiul, while `sudo aiul uninstall` resolves paths for the person who
// typed it and found their own CA instead. `security` then reported
// "The specified item could not be found in the keychain" and the real one was
// untouched by that step.
//
// So this works from identity, not from a path: whatever is in the keychain under
// our common-name prefix is what gets removed, however it got there. A Mac can end
// up with more than one — a developer's own CA and the one the package provisioned
// — so it loops until none are left.
func (DarwinTrust) Uninstall(string) error {
	for attempt := 0; attempt < 10; attempt++ {
		trusted, err := DarwinTrust{}.IsTrusted(ca.CommonNamePrefix)
		if err != nil {
			return err
		}
		if !trusted {
			return nil
		}

		// remove-trusted-cert needs the certificate in a file, so export first.
		// Exporting them all at once means one call whatever the count.
		pem, err := os.CreateTemp("", "aiul-roots-*.pem")
		if err != nil {
			return fmt.Errorf("could not write the certificates out to remove them: %w", err)
		}
		exported, err := exec.Command("security", "find-certificate",
			"-a", "-c", ca.CommonNamePrefix, "-p", systemKeychain).Output()
		if err == nil && len(exported) > 0 {
			_, _ = pem.Write(exported)
		}
		_ = pem.Close()

		_ = runVisible(exec.Command("sudo", "security", "remove-trusted-cert", "-d", pem.Name()))
		_ = os.Remove(pem.Name())

		// delete-certificate removes one match per call, hence the loop.
		if err := runVisible(exec.Command("sudo", "security", "delete-certificate",
			"-c", ca.CommonNamePrefix, systemKeychain)); err != nil {
			break
		}
	}

	trusted, err := DarwinTrust{}.IsTrusted(ca.CommonNamePrefix)
	if err != nil {
		return err
	}
	if trusted {
		return fmt.Errorf("a certificate is still in %s; remove it by hand in Keychain Access, or run scripts/killswitch.sh", systemKeychain)
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
