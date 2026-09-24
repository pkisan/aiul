//go:build linux

package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// LinuxProcess reads /proc, the kernel's view of every process as files. No
// tool to run and no parsing of a tool's output:
//
//  1. /proc/net/tcp lists every TCP connection with the "inode" of its socket.
//     Find the one whose LOCAL port is the client's port.
//  2. /proc/<pid>/fd/* are that process's open files; a socket reads back as
//     "socket:[<inode>]". The process holding our inode opened the connection.
//  3. /proc/<pid>/exe and /proc/<pid>/cwd are its program and working directory.
//
// Another user's fd, exe and cwd are readable only by root, which is why the
// installed worker asks the root helper.
type LinuxProcess struct{}

func Processes() ProcessFinder { return LinuxProcess{} }

func (LinuxProcess) ByLocalPort(port int) (Process, error) {
	inode := ""
	// tcp6 too: a client may reach 127.0.0.1 through an IPv6 socket
	// (::ffff:127.0.0.1).
	for _, table := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		data, err := os.ReadFile(table)
		if err != nil {
			continue
		}
		if inode = socketInode(string(data), port); inode != "" {
			break
		}
	}
	if inode == "" {
		return Process{}, fmt.Errorf("no process found on port %d", port)
	}

	pid := pidHoldingSocket(inode)
	if pid == 0 {
		return Process{}, fmt.Errorf("no process found on port %d", port)
	}

	proc := Process{PID: pid}
	if comm, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid)); err == nil {
		proc.Name = strings.TrimSpace(string(comm))
	}
	// The executable tells two programs with the same name apart (see Process).
	if exe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid)); err == nil {
		proc.Path = exe
	}
	return proc, nil
}

// socketInode finds, in a /proc/net/tcp table, the ESTABLISHED connection whose
// local port is port, and returns its socket inode. A line looks like:
//
//	sl  local_address rem_address   st tx_queue:rx_queue tr:tm->when retrnsmt   uid  timeout inode
//	 0: 0100007F:C350 0100007F:22C3 01 00000000:00000000 00:00000000 00000000  1000        0 123456
//
// Addresses are hex ip:port; state 01 is ESTABLISHED. Matching the LOCAL port
// is what separates the client's end from the proxy's own end of the same
// connection, whose local port is 8899.
func socketInode(table string, port int) string {
	want := fmt.Sprintf(":%04X", port)
	for _, line := range strings.Split(table, "\n") {
		f := strings.Fields(line)
		if len(f) < 10 || f[3] != "01" {
			continue
		}
		if strings.HasSuffix(f[1], want) && f[9] != "0" {
			return f[9]
		}
	}
	return ""
}

// pidHoldingSocket walks /proc/<pid>/fd looking for the socket. 0 if none.
func pidHoldingSocket(inode string) int {
	want := "socket:[" + inode + "]"
	dirs, _ := filepath.Glob("/proc/[0-9]*/fd")
	for _, dir := range dirs {
		fds, err := os.ReadDir(dir)
		if err != nil {
			continue // gone, or not ours to read
		}
		for _, fd := range fds {
			if link, err := os.Readlink(filepath.Join(dir, fd.Name())); err == nil && link == want {
				pid, _ := strconv.Atoi(filepath.Base(filepath.Dir(dir)))
				return pid
			}
		}
	}
	return 0
}

func (LinuxProcess) WorkingDir(pid int) (string, error) {
	dir, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", pid))
	if err != nil {
		return "", fmt.Errorf("no working directory for pid %d", pid)
	}
	return dir, nil
}
