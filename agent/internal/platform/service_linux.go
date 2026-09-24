//go:build linux

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/pkisan/aiul/internal/paths"
)

// systemd is Linux's service manager, and a "unit" file is its job definition:
// what to run, as whom, and to restart it if it dies. Two units, as on macOS:
//
//	aiul-helper.service — root, tiny, answers four verbs on a unix socket
//	aiul.service        — the worker as _aiul: proxy, parsing, redaction, forwarding
const (
	unitDir     = "/etc/systemd/system"
	helperUnit  = "aiul-helper.service"
	workerUnit  = "aiul.service"
	helperUnitF = unitDir + "/" + helperUnit
	workerUnitF = unitDir + "/" + workerUnit
)

// LinuxService installs and removes our systemd units.
type LinuxService struct{}

func Service() ServiceManager { return LinuxService{} }

func (LinuxService) InstallCommands() []string {
	return []string{
		fmt.Sprintf("sudo cp <this binary> %s", InstalledBinaryPath),
		fmt.Sprintf("sudo mkdir -p %s %s", logDir, WorkerStateDir),
		fmt.Sprintf("sudo chown -R %s:%s %s", ServiceUserName, ServiceGroupName, WorkerStateDir),
		fmt.Sprintf("sudo tee %s   # root helper: '%s helper'", helperUnitF, InstalledBinaryPath),
		fmt.Sprintf("sudo tee %s   # worker as %s: '%s run --manage-proxy'", workerUnitF, ServiceUserName, InstalledBinaryPath),
		"sudo systemctl daemon-reload",
		fmt.Sprintf("sudo systemctl enable %s %s", helperUnit, workerUnit),
		fmt.Sprintf("sudo systemctl restart %s %s", helperUnit, workerUnit),
	}
}

func (LinuxService) UninstallCommands() []string {
	return []string{
		fmt.Sprintf("sudo systemctl disable --now %s %s", workerUnit, helperUnit),
		fmt.Sprintf("sudo rm -f %s %s", workerUnitF, helperUnitF),
		"sudo systemctl daemon-reload",
		fmt.Sprintf("sudo rm -f %s", InstalledBinaryPath),
		fmt.Sprintf("sudo userdel %s", ServiceUserName),
	}
}

func (LinuxService) Install(binaryPath string, extraEnv map[string]string) error {
	_, gid, err := prepareInstall(binaryPath)
	if err != nil {
		return err
	}

	helper := unitFile("AI Usage Logger privileged helper", "", "",
		[]string{InstalledBinaryPath, "helper", "--group", strconv.Itoa(gid)},
		logDir+"/helper.log", logDir+"/helper.err.log", extraEnv)
	if err := os.WriteFile(helperUnitF, []byte(helper), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", helperUnitF, err)
	}

	// The worker starts after the helper: it asks for the system proxy at once.
	worker := unitFile("AI Usage Logger agent", ServiceUserName, helperUnit,
		[]string{InstalledBinaryPath, "run", "--manage-proxy"},
		logDir+"/agent.log", logDir+"/agent.err.log", extraEnv)
	if err := os.WriteFile(workerUnitF, []byte(worker), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", workerUnitF, err)
	}

	// restart, not start: on an upgrade the old processes must go, or they keep
	// running the old binary. restart starts a unit that is not running.
	for _, args := range [][]string{
		{"daemon-reload"},
		{"enable", helperUnit, workerUnit},
		{"restart", helperUnit},
		{"restart", workerUnit},
	} {
		if err := run("systemctl", args...); err != nil {
			return err
		}
	}
	return nil
}

// unitFile builds a systemd unit. An empty user means root.
//
// Output is appended to the same files as on macOS, /var/log/aiul/*.log, so
// every "look at the log" instruction is the same on both. systemd opens them
// as root before starting the process, so ownership cannot stop the worker the
// way it once stopped launchd.
func unitFile(description, user, after string, argv []string, stdout, stderr string, extraEnv map[string]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Written by aiul. Remove with 'sudo aiul uninstall' or scripts/killswitch.sh.\n")
	fmt.Fprintf(&b, "[Unit]\nDescription=%s\n", description)
	if after != "" {
		fmt.Fprintf(&b, "After=%s\nWants=%s\n", after, after)
	}
	b.WriteString("\n[Service]\n")
	if user != "" {
		fmt.Fprintf(&b, "User=%s\nGroup=%s\n", user, user)
	}

	vars := map[string]string{paths.StateDirEnv: WorkerStateDir}
	for k, v := range extraEnv {
		vars[k] = v
	}
	for _, k := range sortedKeys(vars) {
		fmt.Fprintf(&b, "Environment=%s=%s\n", k, vars[k])
	}

	fmt.Fprintf(&b, "ExecStart=%s\n", strings.Join(argv, " "))
	b.WriteString("Restart=always\nRestartSec=2\n")
	fmt.Fprintf(&b, "StandardOutput=append:%s\nStandardError=append:%s\n", stdout, stderr)
	b.WriteString("\n[Install]\nWantedBy=multi-user.target\n")
	return b.String()
}

func (LinuxService) Uninstall() error {
	var firstErr error

	// The worker first: it asks the helper to remove the proxy on its way out.
	_ = run("systemctl", "disable", "--now", workerUnit)
	_ = run("systemctl", "disable", "--now", helperUnit)
	for _, f := range []string{workerUnitF, helperUnitF} {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) && firstErr == nil {
			firstErr = err
		}
	}
	_ = run("systemctl", "daemon-reload")

	if err := DeleteServiceAccount(); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := os.Remove(InstalledBinaryPath); err != nil && !os.IsNotExist(err) && firstErr == nil {
		firstErr = err
	}
	if err := os.RemoveAll(PublicCADir); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

// Running reports whether BOTH units are up. is-active needs no privileges.
func (LinuxService) Running() (bool, error) {
	// One unit per call: with several, is-active succeeds if ANY is active.
	for _, unit := range []string{helperUnit, workerUnit} {
		if exec.Command("systemctl", "is-active", "--quiet", unit).Run() != nil {
			return false, nil
		}
	}
	return true, nil
}
