//go:build darwin

package platform

import (
	"fmt"
	"os/exec"
	"strings"
)

// The device token authenticates this machine to the backend. It belongs in the
// macOS keychain rather than a file: the keychain is encrypted at rest, tied to
// the machine, and does not end up in a backup of the repository by accident.
//
// We use the `security` command rather than the C API to keep this readable and
// dependency-free.
const (
	keychainService = "com.aiul.agent"
	keychainAccount = "device-token"
)

// SetDeviceToken stores the token, replacing any existing one.
func SetDeviceToken(token string) error {
	// -U updates the entry if it already exists instead of failing.
	// -w takes the value; it is passed as an argument, which is acceptable for a
	// command the user runs themselves, and noted in DECISIONS.md.
	return run("security", "add-generic-password",
		"-U", "-s", keychainService, "-a", keychainAccount, "-w", token)
}

// DeviceToken reads the token. An empty string with no error means "none set",
// which is the normal state until the backend exists.
func DeviceToken() (string, error) {
	out, err := exec.Command("security", "find-generic-password",
		"-s", keychainService, "-a", keychainAccount, "-w").Output()
	if err != nil {
		// Not found is not a failure.
		return "", nil
	}
	return strings.TrimSpace(string(out)), nil
}

// DeleteDeviceToken removes the token, for uninstall.
func DeleteDeviceToken() error {
	out, err := exec.Command("security", "delete-generic-password",
		"-s", keychainService, "-a", keychainAccount).CombinedOutput()
	if err != nil && !strings.Contains(string(out), "could not be found") {
		return fmt.Errorf("delete the device token: %w", err)
	}
	return nil
}
