//go:build darwin

package platform

import (
	"fmt"
	"os/exec"
	"strings"
)

// DarwinProxy sets the system proxy with `networksetup`, the macOS command-line
// tool for network settings.
//
// A "network service" is one entry in System Settings > Network: Wi-Fi, Ethernet,
// a VPN. Each keeps its own proxy setting, so the agent sets all of them and the
// kill switch clears all of them. A service whose name starts with "*" is
// disabled; those are skipped.
type DarwinProxy struct{}

func Proxy() ProxyConfigurator { return DarwinProxy{} }

func (p DarwinProxy) SetCommands(hostport string) []string {
	host, port := splitHostPort(hostport)
	var cmds []string
	services, _ := p.services()
	for _, s := range services {
		cmds = append(cmds,
			fmt.Sprintf("sudo networksetup -setsecurewebproxy %q %s %s", s, host, port),
			fmt.Sprintf("sudo networksetup -setproxybypassdomains %q %s", s, strings.Join(bypassDomains, " ")),
		)
	}
	return cmds
}

func (p DarwinProxy) UnsetCommands() []string {
	var cmds []string
	services, _ := p.services()
	for _, s := range services {
		cmds = append(cmds,
			fmt.Sprintf("sudo networksetup -setsecurewebproxystate %q off", s),
			fmt.Sprintf("sudo networksetup -setwebproxystate %q off", s),
		)
	}
	return cmds
}

// bypassDomains never go through the proxy. Local addresses must not, or a
// broken proxy takes the whole machine's local development with it.
var bypassDomains = []string{
	"localhost", "127.0.0.1", "::1",
	"*.local", "*.test",
	"169.254/16", // link-local, including the macOS metadata range
	"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
}

func (p DarwinProxy) Set(hostport string) error {
	host, port := splitHostPort(hostport)
	services, err := p.services()
	if err != nil {
		return err
	}
	if len(services) == 0 {
		return fmt.Errorf("no active network services found")
	}

	for _, s := range services {
		// -setsecurewebproxy sets the HTTPS proxy and enables it in one step.
		if err := run("networksetup", "-setsecurewebproxy", s, host, port); err != nil {
			return fmt.Errorf("set proxy on %q: %w", s, err)
		}
		args := append([]string{"-setproxybypassdomains", s}, bypassDomains...)
		if err := run("networksetup", args...); err != nil {
			return fmt.Errorf("set bypass domains on %q: %w", s, err)
		}
	}
	return nil
}

func (p DarwinProxy) Unset() error {
	services, err := p.services()
	if err != nil {
		return err
	}
	// Keep going after a failure: turning the proxy off on as many services as
	// possible matters more than reporting the first error. Rule 7.
	var firstErr error
	for _, s := range services {
		for _, flag := range []string{"-setsecurewebproxystate", "-setwebproxystate"} {
			if err := run("networksetup", flag, s, "off"); err != nil && firstErr == nil {
				firstErr = fmt.Errorf("unset proxy on %q: %w", s, err)
			}
		}
	}
	return firstErr
}

func (p DarwinProxy) Current() (map[string]string, error) {
	services, err := p.services()
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(services))
	for _, s := range services {
		raw, err := exec.Command("networksetup", "-getsecurewebproxy", s).Output()
		if err != nil {
			continue
		}
		var enabled, host, port string
		for _, line := range strings.Split(string(raw), "\n") {
			key, value, found := strings.Cut(line, ": ")
			if !found {
				continue
			}
			switch strings.TrimSpace(key) {
			case "Enabled":
				enabled = strings.TrimSpace(value)
			case "Server":
				host = strings.TrimSpace(value)
			case "Port":
				port = strings.TrimSpace(value)
			}
		}
		if enabled == "Yes" && host != "" {
			out[s] = host + ":" + port
		} else {
			out[s] = "" // no proxy on this service
		}
	}
	return out, nil
}

// services lists the enabled network services. The first line of the output is a
// heading, and a leading "*" marks a disabled service.
func (DarwinProxy) services() ([]string, error) {
	raw, err := exec.Command("networksetup", "-listallnetworkservices").Output()
	if err != nil {
		return nil, fmt.Errorf("list network services: %w", err)
	}
	var out []string
	for i, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if i == 0 || line == "" || strings.HasPrefix(line, "*") {
			continue
		}
		out = append(out, line)
	}
	return out, nil
}


