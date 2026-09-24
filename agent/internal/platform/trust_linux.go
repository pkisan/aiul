//go:build linux

package platform

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Linux has two trust stores that matter:
//
//   - the system store: a .crt in /usr/local/share/ca-certificates, merged into
//     /etc/ssl/certs by update-ca-certificates. curl, Python, Go and most CLIs
//     read it.
//   - Chrome's: Chrome on Linux trusts a root you added only if it is in the
//     person's own NSS database, ~/.pki/nssdb. certutil (package libnss3-tools)
//     writes it. Chrome reads it at start, so a running Chrome needs a restart.
const (
	systemCAFile = "/usr/local/share/ca-certificates/aiul-dev-root.crt"
	nssNickname  = "AIUL Dev Root"
)

type LinuxTrust struct{}

func Trust() TrustInstaller { return LinuxTrust{} }

func (LinuxTrust) InstallCommands(certPath string) []string {
	return []string{
		fmt.Sprintf("sudo cp %s %s", certPath, systemCAFile),
		"sudo update-ca-certificates",
		fmt.Sprintf("certutil -d sql:$HOME/.pki/nssdb -A -t C,, -n %q -i %s   # as each desktop user, for Chrome", nssNickname, systemCAFile),
	}
}

func (LinuxTrust) UninstallCommands(string) []string {
	return []string{
		fmt.Sprintf("sudo rm -f %s", systemCAFile),
		"sudo update-ca-certificates --fresh",
		fmt.Sprintf("certutil -d sql:$HOME/.pki/nssdb -D -n %q   # as each desktop user", nssNickname),
	}
}

func (LinuxTrust) Install(certPath string) error {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("certificate not found: %w", err)
	}
	if _, err := exec.LookPath("certutil"); err != nil {
		return fmt.Errorf("certutil is missing, so Chrome cannot be told to trust the CA: sudo apt install libnss3-tools")
	}

	if err := os.WriteFile(systemCAFile, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", systemCAFile, err)
	}
	if err := run("update-ca-certificates"); err != nil {
		return err
	}

	// Each person's Chrome store. The file certutil imports must be readable by
	// that person, which the worker's own copy is not; the system copy is.
	for _, u := range desktopUsers() {
		if err := u.trustInNSS(systemCAFile); err != nil {
			return fmt.Errorf("trust the CA in %s's Chrome store: %w", u.Name, err)
		}
	}
	return nil
}

// trustInNSS replaces our root in this person's NSS database. The database is
// created if Chrome has never run; Chrome then uses the one we made.
func (u desktopUser) trustInNSS(certFile string) error {
	dir := filepath.Join(u.Home, ".pki", "nssdb")
	db := "sql:" + dir

	if _, err := os.Stat(filepath.Join(dir, "cert9.db")); err != nil {
		if err := u.plain("mkdir", "-p", dir).Run(); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
		if out, err := u.plain("certutil", "-d", db, "-N", "--empty-password").CombinedOutput(); err != nil {
			return fmt.Errorf("create the NSS database: %w: %s", err, strings.TrimSpace(string(out)))
		}
	}

	// An older root from a previous install has the same nickname; remove it so
	// only the current one is trusted.
	u.removeFromNSS()

	// C,, : a trusted CA for TLS servers, nothing else (not email, not code).
	out, err := u.plain("certutil", "-d", db, "-A", "-t", "C,,", "-n", nssNickname, "-i", certFile).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (u desktopUser) removeFromNSS() {
	db := "sql:" + filepath.Join(u.Home, ".pki", "nssdb")
	for i := 0; i < 10; i++ {
		if u.plain("certutil", "-d", db, "-D", "-n", nssNickname).Run() != nil {
			return
		}
	}
}

// plain runs a command as this person, without a session bus.
func (u desktopUser) plain(name string, args ...string) *exec.Cmd {
	cmd := exec.Command("runuser", append([]string{"-u", u.Name, "--", name}, args...)...)
	cmd.Env = append(os.Environ(), "HOME="+u.Home)
	return cmd
}

// Uninstall removes our root from both stores, by name rather than by path, so
// it works whichever CA was installed.
func (LinuxTrust) Uninstall(string) error {
	var firstErr error
	if _, err := os.Stat(systemCAFile); err == nil {
		if err := os.Remove(systemCAFile); err != nil {
			firstErr = err
		}
		if err := run("update-ca-certificates", "--fresh"); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if _, err := exec.LookPath("certutil"); err == nil {
		for _, u := range desktopUsers() {
			u.removeFromNSS()
		}
	}
	return firstErr
}

// IsTrusted reports whether the system store holds our root. Chrome's store is
// per person and not readable by everyone, so it is not part of the answer.
func (LinuxTrust) IsTrusted(commonName string) (bool, error) {
	data, err := os.ReadFile(systemCAFile)
	if err != nil {
		return false, nil
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return false, nil
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false, nil
	}
	return strings.HasPrefix(cert.Subject.CommonName, commonName), nil
}
