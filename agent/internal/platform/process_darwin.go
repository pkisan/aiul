//go:build darwin

package platform

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// DarwinProcess finds processes with `lsof`, the macOS tool that lists open files
// — and on Unix a network connection is a kind of open file.
//
// lsof is not fast, so callers cache what they learn. It also needs no special
// privileges for the current user's own processes, and runs as root in the daemon,
// where it sees everything.
type DarwinProcess struct{}

func Processes() ProcessFinder { return DarwinProcess{} }

func (DarwinProcess) ByLocalPort(port int) (Process, error) {
	// -i        internet connections only
	// -n -P     do not resolve host names or port names, which is much faster
	// -sTCP:... only established connections
	// -F pcn    machine-readable output: p=pid, c=command, n=name
	out, err := exec.Command("lsof",
		"-i", fmt.Sprintf("tcp@127.0.0.1:%d", port),
		"-n", "-P", "-sTCP:ESTABLISHED", "-F", "pcn").Output()
	if err != nil {
		// lsof exits non-zero when nothing matches, which is a normal "not found".
		return Process{}, fmt.Errorf("no process found on port %d", port)
	}
	return parseLsof(string(out), port)
}

// parseLsof reads lsof's field output. Each line begins with a one-letter tag:
//
//	p1234        the process id
//	cclaude      the command name
//	n127.0.0.1:52345->127.0.0.1:8899   the connection
//
// Fields belong to the process whose "p" line came before them.
func parseLsof(out string, port int) (Process, error) {
	var current Process
	want := fmt.Sprintf(":%d->", port)

	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		tag, value := line[0], line[1:]
		switch tag {
		case 'p':
			pid, err := strconv.Atoi(value)
			if err != nil {
				continue
			}
			current = Process{PID: pid}
		case 'c':
			current.Name = value
		case 'n':
			// Only the end of the connection whose SOURCE is our port. The proxy's
			// own listening socket also appears, and must not be mistaken for the
			// client.
			if strings.Contains(value, want) && current.PID != 0 {
				return current, nil
			}
		}
	}
	return Process{}, fmt.Errorf("no process found on port %d", port)
}

func (DarwinProcess) WorkingDir(pid int) (string, error) {
	// -d cwd    the working directory entry only
	// -a        combine the filters rather than OR them
	out, err := exec.Command("lsof", "-a", "-p", strconv.Itoa(pid), "-d", "cwd", "-F", "n").Output()
	if err != nil {
		return "", fmt.Errorf("no working directory for pid %d", pid)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if after, ok := strings.CutPrefix(line, "n"); ok && after != "" {
			return after, nil
		}
	}
	return "", fmt.Errorf("no working directory for pid %d", pid)
}
