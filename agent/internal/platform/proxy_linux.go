//go:build linux

package platform

import (
	"fmt"
	"os"
	"strings"
)

// LinuxProxy sets GNOME's proxy (Settings > Network > Proxy), which Chrome
// follows as soon as it changes. It is stored per person, so it is set for each
// desktop user who is logged in. gsettings is GNOME's command-line tool for
// those settings.
//
// Only the HTTPS proxy is set, as on macOS: AI tools speak HTTPS, and plain
// HTTP keeps going direct.
type LinuxProxy struct{}

func Proxy() ProxyConfigurator { return LinuxProxy{} }

func (LinuxProxy) SetCommands(hostport string) []string {
	host, port := splitHostPort(hostport)
	return []string{
		"# as each logged-in desktop user, on their session bus:",
		fmt.Sprintf("gsettings set org.gnome.system.proxy.https host %q", host),
		fmt.Sprintf("gsettings set org.gnome.system.proxy.https port %s", port),
		fmt.Sprintf("gsettings set org.gnome.system.proxy ignore-hosts %q", ignoreHosts()),
		"gsettings set org.gnome.system.proxy mode 'manual'",
	}
}

func (LinuxProxy) UnsetCommands() []string {
	return []string{
		"gsettings set org.gnome.system.proxy mode 'none'   # as each desktop user whose proxy points at us",
	}
}

// ignoreHosts is NoProxyList in GSettings' list syntax: ['localhost', '127.0.0.1'].
func ignoreHosts() string {
	quoted := make([]string, len(NoProxyList))
	for i, h := range NoProxyList {
		quoted[i] = "'" + h + "'"
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// Set points each logged-in person's proxy at us, changing only what differs so
// that repeating it every health tick costs a few reads.
//
// Nobody logged in is not an error: there is nothing to point, and the next tick
// catches whoever logs in.
func (p LinuxProxy) Set(hostport string) error {
	host, port := splitHostPort(hostport)
	var firstErr error
	for _, u := range desktopUsers() {
		if u.sessionBus() == "" {
			continue
		}
		if u.proxy() == hostport {
			continue
		}
		for _, kv := range [][2]string{
			{"org.gnome.system.proxy.https host", host},
			{"org.gnome.system.proxy.https port", port},
			{"org.gnome.system.proxy ignore-hosts", ignoreHosts()},
			{"org.gnome.system.proxy mode", "manual"},
		} {
			schema, key, _ := strings.Cut(kv[0], " ")
			if out, err := u.command("gsettings", "set", schema, key, kv[1]).CombinedOutput(); err != nil && firstErr == nil {
				firstErr = fmt.Errorf("set %s for %s: %w: %s", kv[0], u.Name, err, strings.TrimSpace(string(out)))
			}
		}
	}
	return firstErr
}

// Unset turns the proxy off for every person whose proxy points at us, logged
// in or not: someone logging in later must not find a proxy that is not there.
// A proxy the person set up themselves is left alone. Rule 7.
func (LinuxProxy) Unset() error {
	var firstErr error
	for _, u := range desktopUsers() {
		current := u.proxy()
		if current == "" || !strings.HasSuffix(current, ":"+ourProxyPort) {
			continue
		}
		if out, err := u.command("gsettings", "set", "org.gnome.system.proxy", "mode", "none").CombinedOutput(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("unset the proxy for %s: %w: %s", u.Name, err, strings.TrimSpace(string(out)))
		}
	}
	return firstErr
}

// ourProxyPort marks a setting as ours; the kill switch uses the same test.
const ourProxyPort = "8899"

// Current reports each logged-in person's HTTPS proxy, "" for none.
func (LinuxProxy) Current() (map[string]string, error) {
	if os.Geteuid() != 0 {
		return nil, ErrProxyStateHidden
	}
	out := map[string]string{}
	for _, u := range desktopUsers() {
		if u.sessionBus() != "" {
			out[u.Name] = u.proxy()
		}
	}
	return out, nil
}

// proxy reads one person's HTTPS proxy as host:port, or "" when it is off.
func (u desktopUser) proxy() string {
	get := func(schema, key string) string {
		out, err := u.command("gsettings", "get", schema, key).Output()
		if err != nil {
			return ""
		}
		return strings.Trim(strings.TrimSpace(string(out)), "'")
	}
	if get("org.gnome.system.proxy", "mode") != "manual" {
		return ""
	}
	return get("org.gnome.system.proxy.https", "host") + ":" + get("org.gnome.system.proxy.https", "port")
}
