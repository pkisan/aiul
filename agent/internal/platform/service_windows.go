//go:build windows

package platform

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/pkisan/aiul/internal/paths"
)

// A Windows service is a program the Service Control Manager (SCM) starts at
// boot and restarts when it fails — launchd's and systemd's job elsewhere. Two,
// as on the other systems:
//
//	aiul-helper — LocalSystem (Windows' root), tiny, answers four verbs
//	aiul        — the worker as NT SERVICE\aiul: proxy, parsing, forwarding
type WindowsService struct{}

func Service() ServiceManager { return WindowsService{} }

// The access list the worker's folders get. icacls is Windows' tool for
// file permissions; *S-1-5-18 is SYSTEM and *S-1-5-32-544 is Administrators,
// named by their fixed IDs so a German or Japanese Windows reads them too.
// (OI)(CI) makes files and folders created inside inherit the entry; F is full
// control and M is modify (read, write, delete — not change permissions).
func workerDirACL(dir string) []string {
	return []string{dir, "/inheritance:r",
		"/grant:r", "*S-1-5-18:(OI)(CI)F",
		"/grant:r", "*S-1-5-32-544:(OI)(CI)F",
		"/grant:r", ServiceUserName + ":(OI)(CI)M"}
}

func configACL() []string {
	return []string{AgentConfigPath, "/inheritance:r",
		"/grant:r", "*S-1-5-18:F",
		"/grant:r", "*S-1-5-32-544:F",
		"/grant:r", ServiceUserName + ":R"}
}

func (WindowsService) InstallCommands() []string {
	exe := `"` + InstalledBinaryPath + `"`
	return []string{
		fmt.Sprintf(`copy <this binary> %s`, exe),
		fmt.Sprintf(`mkdir %s %s %s`, WorkerStateDir, LogDir, PublicCADir),
		fmt.Sprintf(`sc.exe create %s binPath= "%s helper" start= auto obj= LocalSystem`, HelperServiceName, exe),
		fmt.Sprintf(`sc.exe create %s binPath= "%s run --manage-proxy" start= auto obj= "%s" depend= %s`, WorkerServiceName, exe, ServiceUserName, HelperServiceName),
		fmt.Sprintf(`sc.exe failure %s reset= 86400 actions= restart/2000/restart/2000/restart/2000   # and for %s`, WorkerServiceName, HelperServiceName),
		"icacls " + strings.Join(workerDirACL(WorkerStateDir), " ") + "   # and the same for " + LogDir,
		"icacls " + strings.Join(configACL(), " "),
		fmt.Sprintf(`copy the CA to %s\dev-ca`, WorkerStateDir),
		fmt.Sprintf(`sc.exe start %s; sc.exe start %s`, HelperServiceName, WorkerServiceName),
	}
}

func (WindowsService) UninstallCommands() []string {
	return []string{
		fmt.Sprintf(`sc.exe stop %s; sc.exe stop %s`, WorkerServiceName, HelperServiceName),
		fmt.Sprintf(`sc.exe delete %s; sc.exe delete %s`, WorkerServiceName, HelperServiceName),
		fmt.Sprintf(`rmdir /s "%s" %s`, filepath.Dir(InstalledBinaryPath), PublicCADir),
	}
}

func (WindowsService) Install(binaryPath string, extraEnv map[string]string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("open the service manager: %w", err)
	}
	defer m.Disconnect()

	// An upgrade: a running service holds its .exe open, so it cannot be
	// replaced. Stopping the helper also removes the proxy, so traffic flows
	// directly for the moment the upgrade takes.
	stopService(m, WorkerServiceName)
	stopService(m, HelperServiceName)

	if err := lockDataDir(); err != nil {
		return err
	}
	for _, dir := range []string{WorkerStateDir, LogDir, PublicCADir, filepath.Dir(InstalledBinaryPath)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}

	if !strings.EqualFold(filepath.Clean(binaryPath), InstalledBinaryPath) {
		data, err := os.ReadFile(binaryPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", binaryPath, err)
		}
		if err := os.WriteFile(InstalledBinaryPath, data, 0o755); err != nil {
			return fmt.Errorf("write %s: %w", InstalledBinaryPath, err)
		}
	}

	// A service does not inherit the installing window's environment; what it
	// needs goes into the service's own registry entry.
	env := map[string]string{paths.StateDirEnv: WorkerStateDir}
	for k, v := range extraEnv {
		env[k] = v
	}

	if err := ensureService(m, HelperServiceName, mgr.Config{
		DisplayName: "AI Usage Logger helper",
		Description: "Sets the proxy for signed-in users and says which program opened a connection. Remove with 'aiul uninstall' or scripts\\killswitch.ps1.",
		StartType:   mgr.StartAutomatic,
	}, env, "helper"); err != nil {
		return err
	}
	if err := ensureService(m, WorkerServiceName, mgr.Config{
		DisplayName:      "AI Usage Logger",
		Description:      "Records AI tool usage on this managed device. Remove with 'aiul uninstall' or scripts\\killswitch.ps1.",
		StartType:        mgr.StartAutomatic,
		ServiceStartName: ServiceUserName,
		Dependencies:     []string{HelperServiceName},
	}, env, "run", "--manage-proxy"); err != nil {
		return err
	}

	// Only now: the worker's virtual account exists once its service does, and
	// icacls cannot name it before.
	for _, dir := range []string{WorkerStateDir, LogDir} {
		if err := run("icacls", workerDirACL(dir)...); err != nil {
			return fmt.Errorf("restrict %s: %w", dir, err)
		}
	}
	if _, err := os.Stat(AgentConfigPath); err == nil {
		if err := run("icacls", configACL()...); err != nil {
			return fmt.Errorf("restrict %s: %w", AgentConfigPath, err)
		}
	}

	// The CA lives in the installing person's %LOCALAPPDATA%, which the worker
	// cannot read. Copied files inherit the state folder's access list.
	if err := copyCAForWindowsWorker(); err != nil {
		return fmt.Errorf("give the CA to the worker: %w", err)
	}

	for _, name := range []string{HelperServiceName, WorkerServiceName} {
		s, err := m.OpenService(name)
		if err != nil {
			return fmt.Errorf("open %s: %w", name, err)
		}
		err = s.Start()
		s.Close()
		if err != nil && !errors.Is(err, windows.ERROR_SERVICE_ALREADY_RUNNING) {
			return fmt.Errorf("start %s: %w", name, err)
		}
	}
	return nil
}

// lockDataDir closes C:\ProgramData\AIUL to everyone but SYSTEM and
// Administrators before anything secret goes in it.
//
// ProgramData lets every user create files and folders, and whoever creates
// something owns it and may change who can read it. An ordinary user could
// create C:\ProgramData\AIUL\state before the install and later read the CA's
// private key from it, and with that key make any website look genuine to
// every program on the PC. So: first shut the door (users may read, only
// SYSTEM and Administrators may write), then refuse if anything already inside
// belongs to someone else. In that order, nothing can slip in between.
func lockDataDir() error {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dataDir, err)
	}
	if err := run("icacls", dataDir, "/inheritance:r",
		"/grant:r", "*S-1-5-18:(OI)(CI)F",
		"/grant:r", "*S-1-5-32-544:(OI)(CI)F",
		"/grant:r", "*S-1-5-32-545:(OI)(CI)RX"); err != nil {
		return fmt.Errorf("restrict %s: %w", dataDir, err)
	}
	return checkOwners(dataDir)
}

// checkOwners fails on the first file or folder under root owned by anyone but
// SYSTEM, Administrators, the worker or the person running the install.
func checkOwners(root string) error {
	trusted := map[string]bool{"S-1-5-18": true, "S-1-5-32-544": true}
	// The worker's account exists only once its service does (an upgrade).
	if sid, _, _, err := windows.LookupSID("", ServiceUserName); err == nil {
		trusted[sid.String()] = true
	}
	// Some PCs make the administrator themself, not the group, the owner of
	// what they create.
	if u, err := windows.GetCurrentProcessToken().GetTokenUser(); err == nil {
		trusted[u.User.Sid.String()] = true
	}

	return filepath.WalkDir(root, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("check %s: %w", path, err)
		}
		sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
		if err != nil {
			return fmt.Errorf("read the owner of %s: %w", path, err)
		}
		owner, _, err := sd.Owner()
		if err != nil {
			return fmt.Errorf("read the owner of %s: %w", path, err)
		}
		if trusted[owner.String()] {
			return nil
		}
		name := owner.String()
		if account, domain, _, err := owner.LookupAccount(""); err == nil {
			name = domain + `\` + account
		}
		return fmt.Errorf("%s belongs to %s, not to an administrator, so that account could read what the agent keeps there. Delete %s and install again", path, name, dataDir)
	})
}

// ensureService creates the service, or updates one left by an earlier install.
func ensureService(m *mgr.Mgr, name string, cfg mgr.Config, env map[string]string, args ...string) error {
	s, err := m.OpenService(name)
	if err == nil {
		current, cerr := s.Config()
		if cerr != nil {
			s.Close()
			return fmt.Errorf("read %s: %w", name, cerr)
		}
		current.BinaryPathName = commandLine(InstalledBinaryPath, args...)
		current.DisplayName = cfg.DisplayName
		current.Description = cfg.Description
		current.StartType = cfg.StartType
		current.ServiceStartName = cfg.ServiceStartName
		current.Dependencies = cfg.Dependencies
		err = s.UpdateConfig(current)
		if err != nil {
			s.Close()
			return fmt.Errorf("update %s: %w", name, err)
		}
	} else {
		s, err = m.CreateService(name, InstalledBinaryPath, cfg, args...)
		if err != nil {
			return fmt.Errorf("create %s: %w", name, err)
		}
	}
	defer s.Close()

	// Restart two seconds after any failure, including a clean exit with an
	// error code (a worker that refuses to start is retried, as with launchd).
	restart := mgr.RecoveryAction{Type: mgr.ServiceRestart, Delay: 2 * time.Second}
	if err := s.SetRecoveryActions([]mgr.RecoveryAction{restart, restart, restart}, 86400); err != nil {
		return fmt.Errorf("set %s to restart on failure: %w", name, err)
	}
	if err := s.SetRecoveryActionsOnNonCrashFailures(true); err != nil {
		return fmt.Errorf("set %s to restart on failure: %w", name, err)
	}

	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Services\`+name, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open %s's registry entry: %w", name, err)
	}
	defer key.Close()
	lines := make([]string, 0, len(env))
	for k, v := range env {
		lines = append(lines, k+"="+v)
	}
	sort.Strings(lines)
	if err := key.SetStringsValue("Environment", lines); err != nil {
		return fmt.Errorf("set %s's environment: %w", name, err)
	}
	return nil
}

func commandLine(exe string, args ...string) string {
	out := syscall.EscapeArg(exe)
	for _, a := range args {
		out += " " + syscall.EscapeArg(a)
	}
	return out
}

// stopService stops a service if it exists and waits up to 30 seconds for it.
func stopService(m *mgr.Mgr, name string) {
	s, err := m.OpenService(name)
	if err != nil {
		return
	}
	defer s.Close()

	if _, err := s.Control(svc.Stop); err != nil {
		return // not running
	}
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(300 * time.Millisecond) {
		if st, err := s.Query(); err != nil || st.State == svc.Stopped {
			return
		}
	}
}

func copyCAForWindowsWorker() error {
	source, err := paths.CADir()
	if err != nil {
		return err
	}
	target := filepath.Join(WorkerStateDir, "dev-ca")
	if err := os.MkdirAll(target, 0o700); err != nil {
		return err
	}
	for _, name := range []string{"root.crt", "root.key", "ca-bundle.pem"} {
		data, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			if os.IsNotExist(err) && name == "ca-bundle.pem" {
				continue
			}
			return fmt.Errorf("read %s: %w", name, err)
		}
		if err := os.WriteFile(filepath.Join(target, name), data, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func (WindowsService) Uninstall() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("open the service manager: %w", err)
	}
	defer m.Disconnect()

	// The worker first: it asks the helper to remove the proxy on its way out,
	// and the helper removes it again as it stops.
	stopService(m, WorkerServiceName)
	stopService(m, HelperServiceName)

	var firstErr error
	for _, name := range []string{WorkerServiceName, HelperServiceName} {
		s, err := m.OpenService(name)
		if err != nil {
			continue // not installed
		}
		if err := s.Delete(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("delete %s: %w", name, err)
		}
		s.Close()
	}
	for _, dir := range []string{filepath.Dir(InstalledBinaryPath), PublicCADir} {
		if err := os.RemoveAll(dir); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Running reports whether BOTH services are running. It asks only for status,
// which any user may, so `aiul status` works from an ordinary window.
func (WindowsService) Running() (bool, error) {
	for _, name := range []string{HelperServiceName, WorkerServiceName} {
		state, err := serviceState(name)
		if err != nil || state != windows.SERVICE_RUNNING {
			return false, nil
		}
	}
	return true, nil
}

func serviceExists(name string) bool {
	_, err := serviceState(name)
	return err == nil
}

func serviceState(name string) (uint32, error) {
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return 0, err
	}
	defer windows.CloseServiceHandle(scm)

	namePtr, _ := windows.UTF16PtrFromString(name)
	h, err := windows.OpenService(scm, namePtr, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return 0, err
	}
	defer windows.CloseServiceHandle(h)

	var st windows.SERVICE_STATUS
	if err := windows.QueryServiceStatus(h, &st); err != nil {
		return 0, err
	}
	return st.CurrentState, nil
}
