//go:build darwin

package platform

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// macOS has no single place to set a machine-wide environment variable that both
// terminals and GUI applications see. This is the "awkward" row in the build map,
// and the answer is two mechanisms:
//
//  1. Terminals: a marked block in /etc/zshenv. zsh reads that file for EVERY
//     shell, including non-interactive ones, so a CLI started from anywhere
//     inherits the variables.
//  2. GUI applications: a LaunchAgent that runs `launchctl setenv` at login.
//     Anything launched from the Dock or Spotlight after that inherits them.
//
// Both write only between our markers or under our own label, so removing them
// touches nothing else.

const (
	zshenvPath = "/etc/zshenv"

	// The LaunchAgent that sets the variables for GUI applications at login.
	sessionAgentLabel = "com.aiul.session"
	sessionAgentPlist = "/Library/LaunchAgents/com.aiul.session.plist"
)

// DarwinEnv writes environment variables for both terminals and GUI apps.
type DarwinEnv struct{}

func Env() EnvWriter { return DarwinEnv{} }

func (DarwinEnv) WriteCommands(vars EnvVars) []string {
	return []string{
		fmt.Sprintf("sudo tee -a %s   # adds a marked AIUL block setting %s", zshenvPath, strings.Join(sortedKeys(vars), ", ")),
		fmt.Sprintf("sudo tee %s      # a LaunchAgent running 'launchctl setenv' at login", sessionAgentPlist),
		fmt.Sprintf("sudo launchctl bootstrap gui/$(stat -f%%u /dev/console) %s", sessionAgentPlist),
		fmt.Sprintf("sudo launchctl asuser $(stat -f%%u /dev/console) launchctl setenv %s ...   # the running GUI session", sortedKeys(vars)[0]),
	}
}

func (DarwinEnv) RemoveCommands() []string {
	return []string{
		fmt.Sprintf("sudo sed -i '' '/AIUL BEGIN/,/AIUL END/d' %s", zshenvPath),
		fmt.Sprintf("sudo launchctl bootout gui/$(stat -f%%u /dev/console) %s", sessionAgentPlist),
		fmt.Sprintf("sudo rm -f %s", sessionAgentPlist),
		"launchctl unsetenv <each variable>",
	}
}

func (e DarwinEnv) Write(vars EnvVars) error {
	if err := e.writeZshenv(vars); err != nil {
		return err
	}
	if err := e.writeSessionAgent(vars); err != nil {
		return err
	}
	// Set them now as well, so the current GUI session picks them up without a
	// logout. Applications already running keep their old copy until restarted.
	for _, k := range sortedKeys(vars) {
		args := guiSetenvArgs(consoleUID(), k, vars[k])
		_ = run(args[0], args[1:]...)
	}
	return nil
}

// guiSetenvArgs builds the command that sets one variable for the GUI session.
//
// This is not the same as running `launchctl setenv` and hoping. `install` runs
// under sudo, and launchctl talks to the domain of whoever is asking — so a plain
// `launchctl setenv` from root sets it for ROOT and the person's Dock, Spotlight
// and apps see nothing. That is exactly why the Claude desktop app, which bundles
// its own Claude Code, never received NODE_EXTRA_CA_CERTS: it saw the proxy, could
// not verify our certificate, and had to be tunneled.
//
// `launchctl asuser <uid> launchctl setenv ...` runs it inside that user's GUI
// domain instead. uid 0 or an empty console (no one logged in) means there is no
// GUI session to write to, and the plain form is the best we can do.
func guiSetenvArgs(uid int, key, value string) []string {
	if uid <= 0 {
		return []string{"launchctl", "setenv", key, value}
	}
	return []string{"launchctl", "asuser", strconv.Itoa(uid), "launchctl", "setenv", key, value}
}

// consoleUID is the user logged in at the screen, read from the owner of
// /dev/console — the standard way to find the GUI session from a root daemon.
// Returns 0 when nobody is logged in, or when the owner cannot be read.
func consoleUID() int {
	info, err := os.Stat("/dev/console")
	if err != nil {
		return 0
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0
	}
	return int(st.Uid)
}

// writeZshenv replaces our marked block, leaving anything else in the file alone.
func (e DarwinEnv) writeZshenv(vars EnvVars) error {
	existing, err := os.ReadFile(zshenvPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", zshenvPath, err)
	}

	if len(existing) > 0 {
		// Keep a backup the first time we touch a file we did not create.
		backup := fmt.Sprintf("%s.aiul-backup.%s", zshenvPath, time.Now().Format("20060102150405"))
		if !strings.Contains(string(existing), blockBegin) {
			if err := os.WriteFile(backup, existing, 0o644); err != nil {
				return fmt.Errorf("back up %s: %w", zshenvPath, err)
			}
		}
	}

	var b strings.Builder
	b.WriteString(stripBlock(string(existing)))
	if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
		b.WriteString("\n")
	}
	b.WriteString(blockBegin + "\n")
	b.WriteString("# Written by aiul. Remove with 'sudo aiul uninstall' or scripts/killswitch.sh.\n")
	for _, k := range sortedKeys(vars) {
		// export, so child processes of the shell inherit it too.
		fmt.Fprintf(&b, "export %s=%q\n", k, vars[k])
	}
	b.WriteString(blockEnd + "\n")

	return os.WriteFile(zshenvPath, []byte(b.String()), 0o644)
}

// writeSessionAgent writes a LaunchAgent that runs `launchctl setenv` once at
// login, so GUI applications inherit the variables.
func (DarwinEnv) writeSessionAgent(vars EnvVars) error {
	var args strings.Builder
	args.WriteString("\t\t<string>/bin/sh</string>\n\t\t<string>-c</string>\n\t\t<string>")
	for _, k := range sortedKeys(vars) {
		fmt.Fprintf(&args, "launchctl setenv %s %s; ", k, shellQuote(vars[k]))
	}
	args.WriteString("</string>\n")

	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
%s	</array>
	<key>RunAtLoad</key>
	<true/>
</dict>
</plist>
`, sessionAgentLabel, args.String())

	if err := os.WriteFile(sessionAgentPlist, []byte(plist), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", sessionAgentPlist, err)
	}
	// Load it into the GUI session of whoever is at the screen, for the same
	// reason setenv needs asuser: a root `launchctl load` loads into root's
	// domain, where no application of theirs will ever look. Failure is fine —
	// there may be nobody logged in, as during an MDM install — because the
	// LaunchAgent runs at the next login regardless.
	if uid := consoleUID(); uid > 0 {
		_ = run("launchctl", "bootstrap", fmt.Sprintf("gui/%d", uid), sessionAgentPlist)
		return nil
	}
	_ = run("launchctl", "load", "-w", sessionAgentPlist)
	return nil
}

func (e DarwinEnv) Remove() error {
	var firstErr error

	// 1. the /etc/zshenv block
	if existing, err := os.ReadFile(zshenvPath); err == nil {
		cleaned := stripBlock(string(existing))
		if strings.TrimSpace(cleaned) == "" {
			// The file holds nothing but our block, so remove it entirely.
			if err := os.Remove(zshenvPath); err != nil && firstErr == nil {
				firstErr = err
			}
		} else if cleaned != string(existing) {
			if err := os.WriteFile(zshenvPath, []byte(cleaned), 0o644); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}

	// 2. the LaunchAgent
	if _, err := os.Stat(sessionAgentPlist); err == nil {
		if uid := consoleUID(); uid > 0 {
			_ = run("launchctl", "bootout", fmt.Sprintf("gui/%d", uid), sessionAgentPlist)
		}
		_ = run("launchctl", "unload", "-w", sessionAgentPlist)
		if err := os.Remove(sessionAgentPlist); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	// 3. the variables in the running GUI session, in that session's own domain
	// (see guiSetenvArgs) and in ours.
	uid := consoleUID()
	for _, k := range AllManagedVars {
		if uid > 0 {
			_ = run("launchctl", "asuser", strconv.Itoa(uid), "launchctl", "unsetenv", k)
		}
		_ = run("launchctl", "unsetenv", k)
	}
	return firstErr
}

func (DarwinEnv) Current() (EnvVars, error) {
	data, err := os.ReadFile(zshenvPath)
	if err != nil {
		if os.IsNotExist(err) {
			return EnvVars{}, nil
		}
		return nil, err
	}

	return parseBlockVars(string(data)), nil
}

// AllManagedVars is every variable this agent may set, so removal can clear them
// all even if the configuration has changed since they were written.
var AllManagedVars = []string{
	"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy",
	"NO_PROXY", "no_proxy",
	"NODE_EXTRA_CA_CERTS", "NODE_USE_SYSTEM_CA",
	"SSL_CERT_FILE", "REQUESTS_CA_BUNDLE",
	"CODEX_CA_CERTIFICATE", "CLAUDE_CODE_CERT_STORE",
}

// stripBlock removes our marked block from a file's contents.
func stripBlock(s string) string {
	begin := strings.Index(s, blockBegin)
	if begin < 0 {
		return s
	}
	end := strings.Index(s[begin:], blockEnd)
	if end < 0 {
		// A truncated block: drop everything from the marker on, rather than
		// leaving half a block behind.
		return s[:begin]
	}
	rest := s[begin+end+len(blockEnd):]
	return s[:begin] + strings.TrimPrefix(rest, "\n")
}

func sortedKeys(vars EnvVars) []string {
	out := make([]string, 0, len(vars))
	for k := range vars {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
