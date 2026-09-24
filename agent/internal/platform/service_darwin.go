//go:build darwin

package platform

import (
	"fmt"
	"os"
	"os/exec"
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
	_, gid, err := prepareInstall(binaryPath)
	if err != nil {
		return err
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
