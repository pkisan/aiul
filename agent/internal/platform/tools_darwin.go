//go:build darwin

package platform

import (
	"os"
	"os/exec"
	"path/filepath"
)

// DarwinTools finds the AI tools installed on this Mac.
type DarwinTools struct{}

func Tools() ToolDetector { return DarwinTools{} }

// knownCLIs maps a command name to what its runtime needs before it trusts our CA.
// The source for this table is the capture matrix in AI-Usage-Logger-Build-Map.md.
var knownCLIs = []Tool{
	{Name: "claude-code", Kind: "cli", TrustVars: []string{"NODE_USE_SYSTEM_CA", "NODE_EXTRA_CA_CERTS", "CLAUDE_CODE_CERT_STORE"}},
	{Name: "codex", Kind: "cli", TrustVars: []string{"CODEX_CA_CERTIFICATE", "SSL_CERT_FILE"}},
	{Name: "gemini", Kind: "cli", TrustVars: []string{"NODE_USE_SYSTEM_CA", "NODE_EXTRA_CA_CERTS"}},
	{Name: "opencode", Kind: "cli", TrustVars: []string{"NODE_EXTRA_CA_CERTS"}},
}

// commandFor maps our tool name to the command actually on PATH.
var commandFor = map[string]string{
	"claude-code": "claude",
	"codex":       "codex",
	"gemini":      "gemini",
	"opencode":    "opencode",
}

// knownIDEs are found by their application bundle rather than a command.
var knownIDEs = []struct {
	Tool
	bundle string
}{
	{Tool{Name: "cursor", Kind: "ide", TrustVars: []string{"NODE_EXTRA_CA_CERTS"}}, "/Applications/Cursor.app"},
	{Tool{Name: "vscode", Kind: "ide", TrustVars: []string{"NODE_EXTRA_CA_CERTS"}}, "/Applications/Visual Studio Code.app"},
	{Tool{Name: "antigravity", Kind: "ide", TrustVars: []string{"NODE_USE_SYSTEM_CA", "NODE_EXTRA_CA_CERTS"}}, "/Applications/Antigravity.app"},
}

func (DarwinTools) Detect() []Tool {
	var found []Tool

	for _, t := range knownCLIs {
		cmd := commandFor[t.Name]
		path, err := exec.LookPath(cmd)
		if err != nil {
			// LookPath uses this process's PATH, which a launch daemon barely has.
			// Check the usual places as well.
			path = findInUsualPlaces(cmd)
			if path == "" {
				continue
			}
		}
		t.Path = path
		found = append(found, t)
	}

	for _, ide := range knownIDEs {
		if _, err := os.Stat(ide.bundle); err == nil {
			t := ide.Tool
			t.Path = ide.bundle
			found = append(found, t)
		}
	}
	return found
}

// findInUsualPlaces looks where Homebrew, npm and the usual installers put things,
// because a process started by launchd has almost no PATH.
func findInUsualPlaces(cmd string) string {
	home, _ := os.UserHomeDir()
	dirs := []string{
		"/opt/homebrew/bin", "/usr/local/bin", "/usr/bin",
		filepath.Join(home, ".local", "bin"),
		filepath.Join(home, ".bun", "bin"),
		filepath.Join(home, ".cargo", "bin"),
	}
	for _, dir := range dirs {
		full := filepath.Join(dir, cmd)
		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			return full
		}
	}
	return ""
}
