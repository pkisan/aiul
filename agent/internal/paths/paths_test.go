package paths

import (
	"os/user"
	"strings"
	"testing"
)

// Under `sudo aiul install`, $HOME may be root's. The CA lives in the person's
// home, so SUDO_USER decides, not $HOME.
func TestSudoUserDecidesTheHomeDirectory(t *testing.T) {
	me, err := user.Current()
	if err != nil {
		t.Fatalf("who am I: %v", err)
	}

	t.Setenv(StateDirEnv, "")
	t.Setenv("HOME", "/var/root")
	t.Setenv("SUDO_USER", me.Username)

	dir, err := State()
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if !strings.HasPrefix(dir, me.HomeDir) {
		t.Fatalf("state dir is %q, wanted it under %q", dir, me.HomeDir)
	}
}

func TestWithoutSudoUserTheEnvironmentIsUsed(t *testing.T) {
	t.Setenv(StateDirEnv, "")
	t.Setenv("HOME", "/tmp/not-a-real-home")
	t.Setenv("SUDO_USER", "")

	dir, err := State()
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if !strings.HasPrefix(dir, "/tmp/not-a-real-home") {
		t.Fatalf("state dir is %q, wanted it under the HOME we set", dir)
	}
}

// The override beats everything, which is how the launchd jobs point both halves
// at /var/db/aiul.
func TestStateDirEnvWins(t *testing.T) {
	t.Setenv(StateDirEnv, "/var/db/aiul")
	t.Setenv("SUDO_USER", "nobody")

	if dir, _ := State(); dir != "/var/db/aiul" {
		t.Fatalf("state dir is %q, wanted /var/db/aiul", dir)
	}
}
