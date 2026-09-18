// Package paths decides where the agent keeps its files.
//
// There are two answers, because there are two ways the agent runs:
//
//   - by hand, as you: everything lives under
//     ~/Library/Application Support/AIUL/, which needs no privileges and is easy
//     to inspect and delete.
//   - installed: the worker runs as the _aiul service account, which deliberately
//     has no home directory, so everything lives under /var/db/aiul/ which that
//     account owns.
//
// Keeping this decision in one place means the CA code and the spool code cannot
// disagree about where things are.
package paths

import (
	"os"
	"os/user"
	"path/filepath"
)

// StateDirEnv overrides the location entirely. The installed launchd jobs set it,
// and it is useful for tests.
const StateDirEnv = "AIUL_STATE_DIR"

// SystemStateDir is where the installed worker keeps everything.
const SystemStateDir = "/var/db/aiul"

// serviceUserName must match platform.ServiceUserName. It is repeated rather than
// imported to keep this package free of build tags.
const serviceUserName = "_aiul"

// State returns the base directory for this process.
func State() (string, error) {
	if dir := os.Getenv(StateDirEnv); dir != "" {
		return dir, nil
	}

	// Running as the service account: it has no usable home by design.
	if current, err := user.Current(); err == nil && current.Username == serviceUserName {
		return SystemStateDir, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		// No home either: fall back to the system location rather than failing.
		return SystemStateDir, nil
	}

	return filepath.Join(home, "Library", "Application Support", "AIUL"), nil
}

// CADir is where the development CA lives.
func CADir() (string, error) {
	base, err := State()
	if err != nil {
		return "", err
	}

	return filepath.Join(base, "dev-ca"), nil
}

// SpoolDir is where captured events wait to be forwarded.
func SpoolDir() (string, error) {
	base, err := State()
	if err != nil {
		return "", err
	}

	return filepath.Join(base, "spool"), nil
}
