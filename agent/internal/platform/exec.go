package platform

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// Small helpers every operating system's implementation uses.

// run executes a command that changes a system setting. It is separated so every
// such call in this package goes through one place.
func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func splitHostPort(hostport string) (host, port string) {
	if i := strings.LastIndex(hostport, ":"); i > 0 {
		return hostport[:i], hostport[i+1:]
	}
	return hostport, "8899"
}

func sortedKeys(vars EnvVars) []string {
	out := make([]string, 0, len(vars))
	for k := range vars {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
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
