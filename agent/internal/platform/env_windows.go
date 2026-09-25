//go:build windows

package platform

import (
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Machine environment variables, the ones in "System Properties > Environment
// Variables > System variables". Every program started afterwards sees them —
// new terminals, and apps started from a fresh Explorer after sign-in.
//
// Windows cannot mark a block of variables as ours the way /etc/environment
// can, so the names we set are recorded in one more variable, AIUL_MANAGED_VARS,
// and removal takes exactly those.
const (
	machineEnvKey = `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`
	managedVarsName = "AIUL_MANAGED_VARS"
)

type WindowsEnv struct{}

func Env() EnvWriter { return WindowsEnv{} }

func (WindowsEnv) WriteCommands(vars EnvVars) []string {
	var out []string
	for _, k := range sortedKeys(vars) {
		out = append(out, fmt.Sprintf(`setx /M %s "%s"`, k, vars[k]))
	}
	out = append(out, fmt.Sprintf(`setx /M %s "%s"`, managedVarsName, strings.Join(sortedKeys(vars), ",")))
	return append(out, "# new terminals see them; open programs keep their old copy")
}

func (WindowsEnv) RemoveCommands() []string {
	return []string{fmt.Sprintf(`reg delete "HKLM\%s" /v <each name in %%%s%%> /f`, machineEnvKey, managedVarsName)}
}

func (e WindowsEnv) Write(vars EnvVars) error {
	// Take away what an earlier install set first, so a variable dropped from
	// the list does not linger.
	if err := e.Remove(); err != nil {
		return err
	}
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, machineEnvKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open the machine environment: %w", err)
	}
	defer k.Close()

	for _, name := range sortedKeys(vars) {
		if err := k.SetStringValue(name, vars[name]); err != nil {
			return fmt.Errorf("set %s: %w", name, err)
		}
	}
	if err := k.SetStringValue(managedVarsName, strings.Join(sortedKeys(vars), ",")); err != nil {
		return err
	}
	announceEnvironmentChange()
	return nil
}

func (WindowsEnv) Remove() error {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, machineEnvKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open the machine environment: %w", err)
	}
	defer k.Close()

	list, _, err := k.GetStringValue(managedVarsName)
	if err != nil {
		return nil // nothing of ours
	}
	for _, name := range strings.Split(list, ",") {
		if name != "" {
			_ = k.DeleteValue(name)
		}
	}
	_ = k.DeleteValue(managedVarsName)
	announceEnvironmentChange()
	return nil
}

func (WindowsEnv) Current() (EnvVars, error) {
	out := EnvVars{}
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, machineEnvKey, registry.QUERY_VALUE)
	if err != nil {
		return out, nil
	}
	defer k.Close()

	list, _, err := k.GetStringValue(managedVarsName)
	if err != nil {
		return out, nil
	}
	for _, name := range strings.Split(list, ",") {
		if v, _, err := k.GetStringValue(name); err == nil {
			out[name] = v
		}
	}
	return out, nil
}

var procSendMessageTimeout = windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageTimeoutW")

// announceEnvironmentChange tells running programs (Explorer above all) that
// the environment changed, so programs started from it get the new variables
// without signing out. The same broadcast setx and the Control Panel send.
func announceEnvironmentChange() {
	const (
		hwndBroadcast   = 0xffff
		wmSettingChange = 0x001A
		smtoAbortIfHung = 0x0002
	)
	env, _ := windows.UTF16PtrFromString("Environment")
	var result uintptr
	_, _, _ = procSendMessageTimeout.Call(hwndBroadcast, wmSettingChange, 0,
		uintptr(unsafe.Pointer(env)), smtoAbortIfHung, 5000, uintptr(unsafe.Pointer(&result)))
}
