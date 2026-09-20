//go:build darwin

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkisan/aiul/internal/paths"
)

// launchd is the macOS service manager. Two kinds of job matter here:
//
//   - a LaunchDaemon runs as root, at boot, with no user logged in. Ours runs
//     `aiul run`: the proxy and the agent loop in one process.
//   - a LaunchAgent runs as the logged-in user, at login. Ours only sets
//     environment variables for GUI applications (see env_darwin.go).
//
// The privilege split is deliberate and recorded in docs/DECISIONS.md: the daemon
// needs root only to change network settings and write /etc/zshenv.
const (
	// Two jobs, because the agent is two processes (see docs/DECISIONS.md, D6):
	//
	//   helper — root, tiny, answers four verbs on a unix socket
	//   worker — runs as _aiul, does the proxy, parsing, redaction and forwarding
	//
	// launchd starts the worker as the service account directly through the
	// plist's UserName key, so there is no privilege-dropping code to get wrong.
	helperLabel = "com.aiul.helper"
	helperPlist = "/Library/LaunchDaemons/com.aiul.helper.plist"

	daemonLabel = "com.aiul.agent"
	daemonPlist = "/Library/LaunchDaemons/com.aiul.agent.plist"

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

// DarwinService installs and removes our launchd jobs.
type DarwinService struct{}

func Service() ServiceManager { return DarwinService{} }

func (DarwinService) InstallCommands() []string {
	return []string{
		fmt.Sprintf("sudo cp <this binary> %s", InstalledBinaryPath),
		fmt.Sprintf("sudo mkdir -p %s %s", logDir, WorkerStateDir),
		fmt.Sprintf("sudo chown -R %s:%s %s", ServiceUserName, ServiceGroupName, WorkerStateDir),
		fmt.Sprintf("sudo tee %s   # root helper: '%s helper'", helperPlist, InstalledBinaryPath),
		fmt.Sprintf("sudo tee %s   # worker as %s: '%s run --manage-proxy'", daemonPlist, ServiceUserName, InstalledBinaryPath),
		fmt.Sprintf("sudo launchctl load -w %s   # or kickstart -k, if it is already running", helperPlist),
		fmt.Sprintf("sudo launchctl load -w %s", daemonPlist),
	}
}

func (DarwinService) UninstallCommands() []string {
	return []string{
		fmt.Sprintf("sudo launchctl unload -w %s", daemonPlist),
		fmt.Sprintf("sudo launchctl unload -w %s", helperPlist),
		fmt.Sprintf("sudo rm -f %s %s", daemonPlist, helperPlist),
		fmt.Sprintf("sudo rm -f %s", InstalledBinaryPath),
		fmt.Sprintf("sudo dscl . -delete /Users/%s", ServiceUserName),
		fmt.Sprintf("sudo dscl . -delete /Groups/%s", ServiceGroupName),
	}
}

func (s DarwinService) Install(binaryPath string, extraEnv map[string]string) error {
	// The service account the worker runs as. Created first: the directories it
	// owns and the socket group both refer to it.
	uid, gid, err := CreateServiceAccount()
	if err != nil {
		return err
	}

	// The config file holds the endpoint and the device token, and the WORKER
	// reads it — not root. A file written 0600 by root is invisible to the service
	// account, which then starts with no endpoint and silently forwards nothing.
	// Give it to root:_aiul 0640: the worker can read it, and no ordinary user on
	// the machine can see the token.
	if err := secureAgentConfig(gid); err != nil {
		return err
	}

	for _, dir := range []string{logDir, WorkerStateDir} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	// launchd opens a job's log files as the account that job runs as, so the
	// worker's two files must belong to that account. Without this, launchd cannot
	// start the worker at all — and because it never runs, it writes no log line
	// saying why. Create them here and hand them over.
	for _, name := range []string{"agent.log", "agent.err.log"} {
		path := filepath.Join(logDir, name)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
		if err != nil {
			return fmt.Errorf("create %s: %w", path, err)
		}
		_ = f.Close()

		if err := os.Chown(path, uid, gid); err != nil {
			return fmt.Errorf("give %s to %s: %w", path, ServiceUserName, err)
		}
	}

	// The worker writes its spool here and must own it. The log directory stays
	// root-owned; both processes append to files launchd opens for them.
	if err := chownTree(WorkerStateDir, uid, gid); err != nil {
		return fmt.Errorf("give %s to %s: %w", WorkerStateDir, ServiceUserName, err)
	}

	// The CA lives in the installing user's home, which the service account
	// cannot read. Copy it to the state directory and give it to that account:
	// without its key the worker cannot mint a certificate for anything.
	if err := copyCAForWorker(uid, gid); err != nil {
		return fmt.Errorf("give the CA to %s: %w", ServiceUserName, err)
	}

	// Copy the binary somewhere root-owned. Running a root daemon from a user's
	// home directory would let that user replace it and gain root.
	if binaryPath != InstalledBinaryPath {
		data, err := os.ReadFile(binaryPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", binaryPath, err)
		}
		if err := os.WriteFile(InstalledBinaryPath, data, 0o755); err != nil {
			return fmt.Errorf("write %s: %w", InstalledBinaryPath, err)
		}
	}

	// 1. the root helper
	helper := daemonPlistXML(helperLabel, "", []string{
		InstalledBinaryPath, "helper", "--group", strconv.Itoa(gid),
	}, logDir+"/helper.log", logDir+"/helper.err.log", extraEnv)

	if err := os.WriteFile(helperPlist, []byte(helper), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", helperPlist, err)
	}

	// 2. the worker, as the service account. UserName is what keeps the code that
	// parses network traffic out of root.
	worker := daemonPlistXML(daemonLabel, ServiceUserName, []string{
		InstalledBinaryPath, "run", "--manage-proxy",
	}, logDir+"/agent.log", logDir+"/agent.err.log", extraEnv)

	if err := os.WriteFile(daemonPlist, []byte(worker), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", daemonPlist, err)
	}

	// The helper first: the worker asks it for the system proxy as soon as it
	// starts.
	if err := loadOrRestart(helperLabel, helperPlist); err != nil {
		return err
	}
	if err := loadOrRestart(daemonLabel, daemonPlist); err != nil {
		return err
	}

	return nil
}

// loadOrRestart starts a job, or restarts it in place when it is already running.
//
// Installing over a running agent replaces the binary on disk and leaves the OLD
// process running with its old code and old settings, so an upgrade has to restart
// it. The obvious way — bootout, then load — does not work: bootout returns before
// launchd has finished tearing the job down, the load that follows fails, and
// NOTHING comes back. That is how an upgrade left this Mac with a system proxy
// setting and no proxy behind it.
//
// `kickstart -k` asks launchd to stop and start the job itself, which has no gap
// for a race to live in. It only works on a job launchd already knows about, so a
// first install still goes through load.
func loadOrRestart(label, plist string) error {
	if err := exec.Command("launchctl", "print", "system/"+label).Run(); err == nil {
		if err := run("launchctl", "kickstart", "-k", "system/"+label); err != nil {
			return fmt.Errorf("restart %s: %w", label, err)
		}

		return nil
	}

	if err := run("launchctl", "load", "-w", plist); err != nil {
		return fmt.Errorf("load %s: %w", label, err)
	}

	return nil
}

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

// daemonPlistXML builds a launchd job. An empty user means it runs as root.
func daemonPlistXML(label, user string, argv []string, stdout, stderr string, extraEnv map[string]string) string {
	var args strings.Builder
	for _, a := range argv {
		fmt.Fprintf(&args, "\t\t<string>%s</string>\n", a)
	}

	userKey := ""
	if user != "" {
		userKey = fmt.Sprintf("\t<key>UserName</key>\n\t<string>%s</string>\n", user)
	}

	// Both halves must agree where the CA and the spool are. The service account
	// has no home directory, so this cannot be left to a default.
	//
	// A variable set in the shell that ran `install` does NOT reach a launchd job:
	// anything the installed processes need has to be written in here. That
	// includes the development MDM override, without which the worker refuses to
	// start on an unmanaged Mac while the install has already pointed the system
	// proxy at it.
	vars := map[string]string{paths.StateDirEnv: WorkerStateDir}
	for key, value := range extraEnv {
		vars[key] = value
	}

	var entries strings.Builder
	for _, key := range sortedKeys(vars) {
		fmt.Fprintf(&entries, "\t\t<key>%s</key>\n\t\t<string>%s</string>\n", key, vars[key])
	}
	env := fmt.Sprintf("\t<key>EnvironmentVariables</key>\n\t<dict>\n%s\t</dict>\n", entries.String())

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
%s%s	<key>ProgramArguments</key>
	<array>
%s	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
	<key>ProcessType</key>
	<string>Background</string>
</dict>
</plist>
`, label, userKey, env, args.String(), stdout, stderr)
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

func (DarwinService) Uninstall() error {
	var firstErr error

	// The worker first, then the helper: the worker asks the helper to remove the
	// system proxy on its way out.
	for _, plist := range []string{daemonPlist, helperPlist} {
		if _, err := os.Stat(plist); err == nil {
			_ = run("launchctl", "unload", "-w", plist)
			if err := os.Remove(plist); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	// bootout catches a job loaded without a plist on disk.
	_ = run("launchctl", "bootout", "system/"+daemonLabel)
	_ = run("launchctl", "bootout", "system/"+helperLabel)

	if err := DeleteServiceAccount(); err != nil && firstErr == nil {
		firstErr = err
	}

	if err := os.Remove(InstalledBinaryPath); err != nil && !os.IsNotExist(err) && firstErr == nil {
		firstErr = err
	}

	// The public CA copies go too: they are only there for the environment
	// variables, which are being removed alongside them.
	if err := os.RemoveAll(PublicCADir); err != nil && !os.IsNotExist(err) && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

// Running reports whether BOTH halves are up.
//
// It asks about processes rather than about launchd: `launchctl list` run by an
// ordinary user shows only that user's own jobs, not system daemons, so it
// answered "no" while both halves were in fact running. `ps` shows every process
// on the machine to everyone, so this answer is the same whoever asks.
func (DarwinService) Running() (bool, error) {
	out, err := exec.Command("ps", "-axo", "command").Output()
	if err != nil {
		return false, fmt.Errorf("ps: %w", err)
	}
	listing := string(out)

	return strings.Contains(listing, InstalledBinaryPath+" run") &&
		strings.Contains(listing, InstalledBinaryPath+" helper"), nil
}
