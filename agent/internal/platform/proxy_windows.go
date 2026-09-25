//go:build windows

package platform

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// WindowsProxy sets each person's proxy ("Settings > Network & internet >
// Proxy"), which Chrome and Edge follow as soon as it changes.
//
// It is stored per person, in their part of the registry. When someone signs
// in, Windows loads their part under HKEY_USERS\<their SID> (a SID is Windows'
// ID for an account), and the SYSTEM helper writes there directly. People who
// sign in later are caught by the worker's 30-second health tick, which asks
// the helper to apply again.
type WindowsProxy struct{}

func Proxy() ProxyConfigurator { return WindowsProxy{} }

const (
	internetSettingsKey = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`
	connectionsKey      = internetSettingsKey + `\Connections`
	connectionsValue    = "DefaultConnectionSettings"
)

func (WindowsProxy) SetCommands(hostport string) []string {
	k := `HKU\<SID>\` + internetSettingsKey
	return []string{
		"# for each signed-in person, as SYSTEM:",
		fmt.Sprintf(`reg add "%s" /v ProxyEnable /t REG_DWORD /d 1 /f`, k),
		fmt.Sprintf(`reg add "%s" /v ProxyServer /d "%s" /f`, k, winProxyServer(hostport)),
		fmt.Sprintf(`reg add "%s" /v ProxyOverride /d "%s" /f`, k, winBypass()),
		fmt.Sprintf(`# and the same in "%s\Connections" %s (a binary record)`, k, connectionsValue),
	}
}

func (WindowsProxy) UnsetCommands() []string {
	return []string{
		`reg add "HKU\<SID>\` + internetSettingsKey + `" /v ProxyEnable /t REG_DWORD /d 0 /f   # for each person whose proxy points at us, signed in or not`,
	}
}

// Set points each signed-in person's proxy at us, writing only when something
// differs, so repeating it every health tick costs a few registry reads.
func (WindowsProxy) Set(hostport string) error {
	server := winProxyServer(hostport)
	var firstErr error
	for _, sid := range loadedUserSIDs() {
		if err := setProxyIn(sid, server); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("set the proxy for %s: %w", sid, err)
		}
	}
	return firstErr
}

func setProxyIn(root, server string) error {
	k, _, err := registry.CreateKey(registry.USERS, root+`\`+internetSettingsKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()

	c, _, err := registry.CreateKey(registry.USERS, root+`\`+connectionsKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer c.Close()

	enabled, _, _ := k.GetIntegerValue("ProxyEnable")
	current, _, _ := k.GetStringValue("ProxyServer")
	blob, _, _ := c.GetBinaryValue(connectionsValue)
	if cs, ok := decodeConnSettings(blob); ok && enabled == 1 && current == server &&
		cs.flags&connFlagProxy != 0 && cs.proxy == server {
		return nil // already ours
	}

	if err := k.SetDWordValue("ProxyEnable", 1); err != nil {
		return err
	}
	if err := k.SetStringValue("ProxyServer", server); err != nil {
		return err
	}
	if err := k.SetStringValue("ProxyOverride", winBypass()); err != nil {
		return err
	}
	return c.SetBinaryValue(connectionsValue, withProxy(blob, server, winBypass()))
}

// Unset turns our proxy off for every person, signed in or not: someone who
// signs in after the agent is gone must not find a proxy that is not there.
// Signed-out people's registry files are loaded for the moment it takes. A
// proxy someone set up themselves is left alone. Rule 7.
func (WindowsProxy) Unset() error {
	var firstErr error
	loaded := map[string]bool{}
	for _, sid := range loadedUserSIDs() {
		loaded[sid] = true
		if err := unsetProxyIn(sid); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("remove the proxy for %s: %w", sid, err)
		}
	}

	// Loading another person's registry file needs administrator rights; the
	// worker (not an administrator) handles only the signed-in people above.
	if !IsAdmin() {
		return firstErr
	}
	for sid, profile := range profiles() {
		if loaded[sid] {
			continue
		}
		hive := filepath.Join(profile, "NTUSER.DAT")
		if _, err := os.Stat(hive); err != nil {
			continue
		}
		mount := "AIUL-" + sid
		if out, err := exec.Command("reg", "load", `HKU\`+mount, hive).CombinedOutput(); err != nil {
			// In use by something else, most likely: not ours to force.
			if firstErr == nil {
				firstErr = fmt.Errorf("load %s: %w: %s", hive, err, strings.TrimSpace(string(out)))
			}
			continue
		}
		if err := unsetProxyIn(mount); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("remove the proxy for %s: %w", sid, err)
		}
		_ = exec.Command("reg", "unload", `HKU\`+mount).Run()
	}
	return firstErr
}

func unsetProxyIn(root string) error {
	k, err := registry.OpenKey(registry.USERS, root+`\`+internetSettingsKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return nil // no settings at all: nothing of ours
	}
	defer k.Close()

	if current, _, _ := k.GetStringValue("ProxyServer"); isOurProxyServer(current) {
		if err := k.SetDWordValue("ProxyEnable", 0); err != nil {
			return err
		}
		_ = k.DeleteValue("ProxyServer")
		_ = k.DeleteValue("ProxyOverride")
	}

	c, err := registry.OpenKey(registry.USERS, root+`\`+connectionsKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return nil
	}
	defer c.Close()
	blob, _, err := c.GetBinaryValue(connectionsValue)
	if err != nil {
		return nil
	}
	if cleaned, changed := withoutProxy(blob); changed {
		return c.SetBinaryValue(connectionsValue, cleaned)
	}
	return nil
}

// Current reports each signed-in person's HTTPS proxy, "" for none. Only an
// administrator or SYSTEM can read other people's settings; the worker gets
// ErrProxyStateHidden and asks the helper to re-apply instead.
func (WindowsProxy) Current() (map[string]string, error) {
	if !IsAdmin() {
		return nil, ErrProxyStateHidden
	}
	out := map[string]string{}
	for _, sid := range loadedUserSIDs() {
		out[sid] = ""
		k, err := registry.OpenKey(registry.USERS, sid+`\`+internetSettingsKey, registry.QUERY_VALUE)
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return nil, ErrProxyStateHidden // elevated, but still not allowed: let the helper look
		}
		if err != nil {
			continue
		}
		enabled, _, _ := k.GetIntegerValue("ProxyEnable")
		server, _, _ := k.GetStringValue("ProxyServer")
		k.Close()
		if enabled == 1 {
			out[sid] = winProxyHost(server)
		}
	}
	return out, nil
}

// loadedUserSIDs lists the people whose registry is loaded: those signed in,
// and briefly some who just signed out. Local accounts are S-1-5-21-…,
// Microsoft Entra (Azure AD) accounts S-1-12-1-…; the "_Classes" twins and the
// built-in service accounts are skipped.
func loadedUserSIDs() []string {
	names, err := registry.USERS.ReadSubKeyNames(-1)
	if err != nil {
		return nil
	}
	var out []string
	for _, n := range names {
		if isPersonSID(n) {
			out = append(out, n)
		}
	}
	return out
}

func isPersonSID(s string) bool {
	return (strings.HasPrefix(s, "S-1-5-21-") || strings.HasPrefix(s, "S-1-12-1-")) &&
		!strings.HasSuffix(s, "_Classes")
}

// profiles maps each person's SID to their profile folder (C:\Users\name).
func profiles() map[string]string {
	const list = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProfileList`
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, list, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil
	}
	defer k.Close()
	sids, _ := k.ReadSubKeyNames(-1)

	out := map[string]string{}
	for _, sid := range sids {
		if !isPersonSID(sid) {
			continue
		}
		p, err := registry.OpenKey(registry.LOCAL_MACHINE, list+`\`+sid, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		dir, _, err := p.GetStringValue("ProfileImagePath")
		p.Close()
		if err != nil {
			continue
		}
		// Stored as "%SystemDrive%\Users\name"; expand it Windows' way.
		if expanded, err := registry.ExpandString(dir); err == nil {
			dir = expanded
		}
		out[sid] = dir
	}
	return out
}
