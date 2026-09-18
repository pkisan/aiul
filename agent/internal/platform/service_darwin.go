//go:build darwin

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
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
	daemonLabel = "com.aiul.agent"
	daemonPlist = "/Library/LaunchDaemons/com.aiul.agent.plist"

	// Where the installed binary lives. /usr/local/bin is not writable by a
	// standard user, which is what we want for a binary that runs as root.
	InstalledBinaryPath = "/usr/local/bin/aiul"

	logDir = "/var/log/aiul"
)

// DarwinService installs and removes our launchd jobs.
type DarwinService struct{}

func Service() ServiceManager { return DarwinService{} }

func (DarwinService) InstallCommands() []string {
	return []string{
		fmt.Sprintf("sudo cp <this binary> %s", InstalledBinaryPath),
		fmt.Sprintf("sudo mkdir -p %s", logDir),
		fmt.Sprintf("sudo tee %s   # a LaunchDaemon running '%s run'", daemonPlist, InstalledBinaryPath),
		fmt.Sprintf("sudo launchctl load -w %s", daemonPlist),
	}
}

func (DarwinService) UninstallCommands() []string {
	return []string{
		fmt.Sprintf("sudo launchctl unload -w %s", daemonPlist),
		fmt.Sprintf("sudo rm -f %s", daemonPlist),
		fmt.Sprintf("sudo rm -f %s", InstalledBinaryPath),
	}
}

func (s DarwinService) Install(binaryPath string) error {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", logDir, err)
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

	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>run</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>%s/agent.log</string>
	<key>StandardErrorPath</key>
	<string>%s/agent.err.log</string>
	<key>ProcessType</key>
	<string>Background</string>
</dict>
</plist>
`, daemonLabel, InstalledBinaryPath, logDir, logDir)

	if err := os.WriteFile(daemonPlist, []byte(plist), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", daemonPlist, err)
	}
	if err := run("launchctl", "load", "-w", daemonPlist); err != nil {
		return fmt.Errorf("load the launch daemon: %w", err)
	}
	return nil
}

func (DarwinService) Uninstall() error {
	var firstErr error
	if _, err := os.Stat(daemonPlist); err == nil {
		_ = run("launchctl", "unload", "-w", daemonPlist)
		if err := os.Remove(daemonPlist); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	// bootout catches a job loaded without a plist on disk.
	_ = run("launchctl", "bootout", "system/"+daemonLabel)

	if err := os.Remove(InstalledBinaryPath); err != nil && !os.IsNotExist(err) && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

func (DarwinService) Running() (bool, error) {
	out, err := exec.Command("launchctl", "list").Output()
	if err != nil {
		return false, fmt.Errorf("launchctl list: %w", err)
	}
	return strings.Contains(string(out), daemonLabel), nil
}
