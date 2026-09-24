//go:build linux

package platform

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// On Linux one file covers terminals and GUI apps: /etc/environment, which
// pam_env reads at every login and hands to everything started in that session.
// It is not a shell script — plain KEY="value" lines, no export, no $expansion —
// and it takes effect at the next login.
//
// Only the text between our markers is ever written or removed.
const environmentPath = "/etc/environment"

type LinuxEnv struct{}

func Env() EnvWriter { return LinuxEnv{} }

func (LinuxEnv) WriteCommands(vars EnvVars) []string {
	return []string{
		fmt.Sprintf("sudo tee -a %s   # adds a marked AIUL block setting %s", environmentPath, strings.Join(sortedKeys(vars), ", ")),
		"# takes effect at the next login",
	}
}

func (LinuxEnv) RemoveCommands() []string {
	return []string{
		fmt.Sprintf("sudo sed -i '/AIUL BEGIN/,/AIUL END/d' %s", environmentPath),
	}
}

func (LinuxEnv) Write(vars EnvVars) error {
	existing, err := os.ReadFile(environmentPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", environmentPath, err)
	}

	// Keep a backup the first time we touch it.
	if len(existing) > 0 && !strings.Contains(string(existing), blockBegin) {
		backup := fmt.Sprintf("%s.aiul-backup.%s", environmentPath, time.Now().Format("20060102150405"))
		if err := os.WriteFile(backup, existing, 0o644); err != nil {
			return fmt.Errorf("back up %s: %w", environmentPath, err)
		}
	}

	return os.WriteFile(environmentPath, []byte(withEnvironmentBlock(string(existing), vars)), 0o644)
}

// withEnvironmentBlock replaces our block in /etc/environment's contents.
func withEnvironmentBlock(existing string, vars EnvVars) string {
	var b strings.Builder
	b.WriteString(stripBlock(existing))
	if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
		b.WriteString("\n")
	}
	b.WriteString(blockBegin + "\n")
	b.WriteString("# Written by aiul. Remove with 'sudo aiul uninstall' or scripts/killswitch.sh.\n")
	for _, k := range sortedKeys(vars) {
		fmt.Fprintf(&b, "%s=%q\n", k, vars[k])
	}
	b.WriteString(blockEnd + "\n")
	return b.String()
}

func (LinuxEnv) Remove() error {
	existing, err := os.ReadFile(environmentPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	cleaned := stripBlock(string(existing))
	if cleaned == string(existing) {
		return nil
	}
	return os.WriteFile(environmentPath, []byte(cleaned), 0o644)
}

func (LinuxEnv) Current() (EnvVars, error) {
	data, err := os.ReadFile(environmentPath)
	if err != nil {
		if os.IsNotExist(err) {
			return EnvVars{}, nil
		}
		return nil, err
	}
	return parseBlockVars(string(data)), nil
}
