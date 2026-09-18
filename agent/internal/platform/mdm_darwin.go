//go:build darwin

package platform

import (
	"os"
	"os/exec"
	"strings"
)

// DevAllowUnmanagedVar lets a developer run the agent on a machine that is not
// managed by an MDM. It exists so this project can be built on a personal laptop;
// it logs loudly every time it is used and must never be set in production.
const DevAllowUnmanagedVar = "AIUL_DEV_ALLOW_UNMANAGED"

// DarwinMDM checks enrollment with `profiles status -type enrollment`.
//
// Why this gate exists: decrypting someone's traffic is lawful on a company-owned,
// managed device and not on a personal one. Checking enrollment in software means
// a personal laptop that somehow receives this agent never starts reading traffic,
// regardless of what the configuration says.
type DarwinMDM struct{}

func MDM() MDMChecker { return DarwinMDM{} }

func (DarwinMDM) Enrolled() (bool, string, error) {
	if os.Getenv(DevAllowUnmanagedVar) == "1" {
		return true, "DEVELOPER OVERRIDE: " + DevAllowUnmanagedVar + "=1 is set, so the MDM check was skipped. This must never be set on a real device.", nil
	}

	out, err := exec.Command("profiles", "status", "-type", "enrollment").CombinedOutput()
	if err != nil {
		// The command needs no privileges, so a failure means something unusual.
		// Treat that as not enrolled: refusing to capture is the safe direction.
		return false, "could not run 'profiles status -type enrollment': " + strings.TrimSpace(string(out)), err
	}

	text := string(out)
	// The output looks like:
	//   Enrolled via DEP: No
	//   MDM enrollment: Yes (User Approved)
	enrolled := false
	for _, line := range strings.Split(text, "\n") {
		key, value, found := strings.Cut(line, ":")
		if !found || !strings.Contains(strings.ToLower(key), "mdm enrollment") {
			continue
		}
		if strings.Contains(strings.ToLower(value), "yes") {
			enrolled = true
		}
	}
	return enrolled, strings.TrimSpace(text), nil
}
