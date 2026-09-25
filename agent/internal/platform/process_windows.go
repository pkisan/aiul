//go:build windows

package platform

import (
	"fmt"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// WindowsProcess finds the program behind a connection with
// GetExtendedTcpTable, Windows' list of TCP connections with the process ID
// that owns each one. Seeing other users' programs' paths needs SYSTEM, which
// is why the installed worker asks the helper.
type WindowsProcess struct{}

func Processes() ProcessFinder { return WindowsProcess{} }

var procGetExtendedTcpTable = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("GetExtendedTcpTable")

func (WindowsProcess) ByLocalPort(port int) (Process, error) {
	table, err := tcpTable()
	if err != nil {
		return Process{}, err
	}
	pid := findPIDInTCPTable(table, port, 8899)
	if pid == 0 {
		return Process{}, fmt.Errorf("no process found on port %d", port)
	}

	proc := Process{PID: pid}
	if path, err := imagePath(pid); err == nil {
		proc.Path = path
		// "claude.exe" becomes "claude", the name the same tool has on macOS
		// and Linux, so the backend sees one tool rather than two.
		base := filepath.Base(path)
		if strings.EqualFold(filepath.Ext(base), ".exe") {
			base = base[:len(base)-4]
		}
		proc.Name = base
	}
	return proc, nil
}

// tcpTable returns the IPv4 connection table with owning process IDs. The
// first call learns the size; connections come and go, so it may take a retry.
func tcpTable() ([]byte, error) {
	const (
		afInet                      = 2
		tcpTableOwnerPIDConnections = 4
		errInsufficientBuffer       = 122
	)
	size := uint32(0)
	var buf []byte
	for attempt := 0; attempt < 5; attempt++ {
		var ptr uintptr
		if size > 0 {
			buf = make([]byte, size)
			ptr = uintptr(unsafe.Pointer(&buf[0]))
		}
		r, _, _ := procGetExtendedTcpTable.Call(ptr, uintptr(unsafe.Pointer(&size)), 0,
			afInet, tcpTableOwnerPIDConnections, 0)
		switch r {
		case 0:
			return buf[:size], nil
		case errInsufficientBuffer:
			continue
		default:
			return nil, fmt.Errorf("GetExtendedTcpTable: error %d", r)
		}
	}
	return nil, fmt.Errorf("GetExtendedTcpTable: the table kept growing")
}

func imagePath(pid int) (string, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)

	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return "", err
	}
	return windows.UTF16ToString(buf[:n]), nil
}

// WorkingDir is not read on Windows: it lives inside the other process's
// memory (its PEB), and reading that is fragile. Tasks then come only from
// what a parser reports, as Cursor's does.
//
// ponytail: no working directory on Windows, so no branch-based task tags;
// read the PEB (NtQueryInformationProcess + ReadProcessMemory) if tagging
// terminal tools on Windows matters.
func (WindowsProcess) WorkingDir(int) (string, error) {
	return "", fmt.Errorf("the working directory of another process is not read on Windows")
}
