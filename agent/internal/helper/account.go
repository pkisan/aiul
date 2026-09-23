package helper

import (
	"encoding/json"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ClaudeCodeAccount names the Claude account a Claude Code process is signed in
// with, or "" for any other program.
//
// Claude's traffic never names the person — only an account id — but Claude Code
// keeps the signed-in profile in ~/.claude.json of the user running it:
//
//	"oauthAccount": {"fullName": "...", "displayName": "...", "emailAddress": "..."}
//
// Only those three fields are read. The file belongs to the person, not the
// worker, so on an installed Mac only the root helper can read it — the same
// reason the helper reads .git/HEAD.
//
// ponytail: ignores CLAUDE_CONFIG_DIR; a person who moved their config gets no
// name, and the dashboard falls back to the device's person.
func ClaudeCodeAccount(pid int, name, executable string) string {
	if name != "claude" && filepath.Base(executable) != "claude" {
		return ""
	}

	home := homeOfProcess(pid)
	if home == "" {
		return ""
	}

	accountMu.Lock()
	defer accountMu.Unlock()

	// The file is rewritten constantly and can run to megabytes, while a busy
	// session opens a connection every few seconds: read it at most once a minute.
	if cached, ok := accounts[home]; ok && time.Since(cached.at) < time.Minute {
		return cached.name
	}

	account := readClaudeAccount(filepath.Join(home, ".claude.json"))
	accounts[home] = cachedAccount{name: account, at: time.Now()}

	return account
}

type cachedAccount struct {
	name string
	at   time.Time
}

var (
	accountMu sync.Mutex
	accounts  = map[string]cachedAccount{}
)

func readClaudeAccount(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	var config struct {
		OAuthAccount struct {
			FullName     string `json:"fullName"`
			DisplayName  string `json:"displayName"`
			EmailAddress string `json:"emailAddress"`
		} `json:"oauthAccount"`
	}
	if json.Unmarshal(data, &config) != nil {
		return ""
	}

	a := config.OAuthAccount
	for _, v := range []string{a.FullName, a.DisplayName, a.EmailAddress} {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

// homeOfProcess finds the home directory of the user a process runs as.
func homeOfProcess(pid int) string {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "uid=").Output()
	if err != nil {
		return ""
	}
	u, err := user.LookupId(strings.TrimSpace(string(out)))
	if err != nil {
		return ""
	}
	return u.HomeDir
}
