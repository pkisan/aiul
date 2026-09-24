//go:build darwin || linux

package platform

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkisan/aiul/internal/paths"
)

// What macOS and Linux install the same way: the same directories, the same
// files, the same ownership. Only the service manager differs (launchd there,
// systemd here), so every path in the instructions is the same on both.
const (
	// Where the worker keeps its state. It cannot use a home directory, because
	// the service account deliberately has none.
	WorkerStateDir = "/var/db/aiul"

	// Where the installed binary lives. /usr/local/bin is not writable by a
	// standard user, which is what we want for a binary that runs as root.
	InstalledBinaryPath = "/usr/local/bin/aiul"

	logDir = "/var/log/aiul"

	// PublicCADir holds the copies of the CA that EVERY user needs to read.
	//
	// The worker's own copy lives under /var/db/aiul, which only the service
	// account may enter — correct for a private key, wrong for a certificate.
	// SSL_CERT_FILE and its friends point at these copies instead: pointing them
	// into the worker's directory broke curl for every user on the machine, since
	// those variables REPLACE the trust store and curl cannot read the file.
	PublicCADir = "/usr/local/share/aiul"

	// WorkerLogPath is where the worker says why it would not start. Worth naming
	// in error messages: an install that fails is nearly always answered here.
	WorkerLogPath = logDir + "/agent.err.log"
)

// PublishCA copies the public half of the CA where every user can read it.
//
// Only certificates: the private key stays where it is, owned by the service
// account. A certificate is public by definition — it is what we ask the machine
// to trust — and the environment variables the agent writes are useless if the
// file they name cannot be opened.
func PublishCA(certPath, bundlePath string) error {
	if err := os.MkdirAll(PublicCADir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", PublicCADir, err)
	}

	for _, from := range []string{certPath, bundlePath} {
		if from == "" {
			continue
		}

		data, err := os.ReadFile(from)
		if err != nil {
			return fmt.Errorf("read %s: %w", from, err)
		}

		to := filepath.Join(PublicCADir, filepath.Base(from))
		if err := os.WriteFile(to, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", to, err)
		}
	}

	return nil
}

// secureAgentConfig makes the agent's config file readable by the worker and by
// nobody else. A missing file is normal: an agent with no backend configured
// captures into its spool and waits.
func secureAgentConfig(gid int) error {
	if _, err := os.Stat(AgentConfigPath); err != nil {
		return nil
	}

	if err := os.Chown(AgentConfigPath, 0, gid); err != nil {
		return fmt.Errorf("give %s to %s: %w", AgentConfigPath, ServiceGroupName, err)
	}
	if err := os.Chmod(AgentConfigPath, 0o640); err != nil {
		return fmt.Errorf("set permissions on %s: %w", AgentConfigPath, err)
	}

	return nil
}

// copyCAForWorker copies the development CA into the worker's own directory.
//
// The key keeps mode 0600 and changes owner rather than becoming readable by
// everyone: exactly one account on the machine can mint certificates with it.
func copyCAForWorker(uid, gid int) error {
	source, err := paths.CADir()
	if err != nil {
		return err
	}
	target := filepath.Join(WorkerStateDir, "dev-ca")

	if err := os.MkdirAll(target, 0o700); err != nil {
		return err
	}

	for name, mode := range map[string]os.FileMode{
		"root.crt":      0o644,
		"root.key":      0o600,
		"ca-bundle.pem": 0o644,
	} {
		data, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			if os.IsNotExist(err) && name == "ca-bundle.pem" {
				continue // written on demand; not fatal if absent
			}

			return fmt.Errorf("read %s: %w", name, err)
		}
		if err := os.WriteFile(filepath.Join(target, name), data, mode); err != nil {
			return err
		}
	}

	return chownTree(target, uid, gid)
}

// chownTree gives a directory and everything under it to the service account.
func chownTree(root string, uid, gid int) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		return os.Chown(path, uid, gid)
	})
}

// prepareInstall does everything before the service manager is involved: the
// service account, the config file's permissions, the log and state directories,
// the worker's copy of the CA and the root-owned binary. It returns the service
// account's uid and gid.
func prepareInstall(binaryPath string) (uid, gid int, err error) {
	// The service account the worker runs as. Created first: the directories it
	// owns and the socket group both refer to it.
	uid, gid, err = CreateServiceAccount()
	if err != nil {
		return -1, -1, err
	}

	// The config file holds the endpoint and the device token, and the WORKER
	// reads it — not root. A file written 0600 by root is invisible to the service
	// account, which then starts with no endpoint and silently forwards nothing.
	// Give it to root:_aiul 0640: the worker can read it, and no ordinary user on
	// the machine can see the token.
	if err := secureAgentConfig(gid); err != nil {
		return -1, -1, err
	}

	for _, dir := range []string{logDir, WorkerStateDir} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return -1, -1, fmt.Errorf("create %s: %w", dir, err)
		}
	}
	// The log directory stays root-owned but must be enterable by everyone:
	// launchd opens the worker's log files AS the service account, and a 0750
	// root:wheel directory made it fail before the worker ran — so it logged
	// nothing. Seen on a fresh Mac mini 2026-09-24; the dev Mac hid it with an
	// older 0755 directory. Chmod, not just MkdirAll, fixes one left behind.
	if err := os.Chmod(logDir, 0o755); err != nil {
		return -1, -1, fmt.Errorf("open up %s: %w", logDir, err)
	}
	// launchd opens a job's log files as the account that job runs as, so the
	// worker's two files must belong to that account. Without this, launchd cannot
	// start the worker at all — and because it never runs, it writes no log line
	// saying why. Create them here and hand them over.
	for _, name := range []string{"agent.log", "agent.err.log"} {
		path := filepath.Join(logDir, name)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
		if err != nil {
			return -1, -1, fmt.Errorf("create %s: %w", path, err)
		}
		_ = f.Close()

		if err := os.Chown(path, uid, gid); err != nil {
			return -1, -1, fmt.Errorf("give %s to %s: %w", path, ServiceUserName, err)
		}
	}

	// The worker writes its spool here and must own it. The log directory stays
	// root-owned; both processes append to files launchd opens for them.
	if err := chownTree(WorkerStateDir, uid, gid); err != nil {
		return -1, -1, fmt.Errorf("give %s to %s: %w", WorkerStateDir, ServiceUserName, err)
	}

	// The CA lives in the installing user's home, which the service account
	// cannot read. Copy it to the state directory and give it to that account:
	// without its key the worker cannot mint a certificate for anything.
	if err := copyCAForWorker(uid, gid); err != nil {
		return -1, -1, fmt.Errorf("give the CA to %s: %w", ServiceUserName, err)
	}

	// Copy the binary somewhere root-owned. Running a root daemon from a user's
	// home directory would let that user replace it and gain root.
	if binaryPath != InstalledBinaryPath {
		data, err := os.ReadFile(binaryPath)
		if err != nil {
			return -1, -1, fmt.Errorf("read %s: %w", binaryPath, err)
		}
		if err := os.WriteFile(InstalledBinaryPath, data, 0o755); err != nil {
			return -1, -1, fmt.Errorf("write %s: %w", InstalledBinaryPath, err)
		}
	}

	return uid, gid, nil
}
