//go:build linux

package platform

import (
	"bufio"
	"net"
	"os"
	"strings"
	"testing"
)

func TestSocketInode(t *testing.T) {
	table := `  sl  local_address rem_address   st tx_queue:rx_queue tr:tm->when retrnsmt   uid  timeout inode
   0: 0100007F:22C3 00000000:0000 0A 00000000:00000000 00:00000000 00000000   998        0 1111 1 0 100 0 0 10 0
   1: 0100007F:22C3 0100007F:C350 01 00000000:00000000 00:00000000 00000000   998        0 2222 1 0 20 4 30 10 -1
   2: 0100007F:C350 0100007F:22C3 01 00000000:00000000 00:00000000 00000000  1000        0 3333 1 0 20 4 30 10 -1
`
	// 0xC350 = 50000, the client's port; 0x22C3 = 8899, the proxy's.
	if got := socketInode(table, 50000); got != "3333" {
		t.Errorf("client end: got %q, want 3333", got)
	}
	// The listening socket (state 0A) is never a match.
	if got := socketInode(table, 1); got != "" {
		t.Errorf("unknown port: got %q", got)
	}
}

// The real thing: connect to ourselves and find our own pid by the port.
func TestByLocalPortFindsThisProcess(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err == nil {
			defer c.Close()
			buf := make([]byte, 1)
			_, _ = c.Read(buf)
		}
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	port := conn.LocalAddr().(*net.TCPAddr).Port
	proc, err := LinuxProcess{}.ByLocalPort(port)
	if err != nil {
		t.Fatal(err)
	}
	if proc.PID != os.Getpid() || proc.Path == "" {
		t.Errorf("got %+v, want pid %d with an executable", proc, os.Getpid())
	}
	if dir, err := (LinuxProcess{}).WorkingDir(proc.PID); err != nil || dir == "" {
		t.Errorf("working dir: %q %v", dir, err)
	}
}

func TestParsePasswdKeepsOnlyDesktopUsers(t *testing.T) {
	passwd := `root:x:0:0:root:/root:/bin/bash
_aiul:x:998:998:AI Usage Logger service:/nonexistent:/usr/sbin/nologin
alice:x:1000:1000:Alice,,,:/home/alice:/bin/bash
nobody:x:65534:65534:nobody:/nonexistent:/usr/sbin/nologin
`
	got := parsePasswd(bufio.NewScanner(strings.NewReader(passwd)))
	if len(got) != 1 || got[0].Name != "alice" || got[0].UID != 1000 || got[0].Home != "/home/alice" {
		t.Errorf("got %+v, want only alice", got)
	}
}

func TestEnvironmentBlockRoundTrip(t *testing.T) {
	existing := "PATH=\"/usr/local/sbin:/usr/bin\"\n"
	vars := EnvVars{"HTTPS_PROXY": "http://127.0.0.1:8899", "NO_PROXY": "localhost,127.0.0.1"}

	once := withEnvironmentBlock(existing, vars)
	twice := withEnvironmentBlock(once, vars)
	if once != twice {
		t.Errorf("writing twice changed the file:\n%s\nvs\n%s", once, twice)
	}
	if strings.Contains(once, "export") {
		t.Error("/etc/environment is not a shell script; no export")
	}
	if got := parseBlockVars(once); got["HTTPS_PROXY"] != vars["HTTPS_PROXY"] || len(got) != 2 {
		t.Errorf("read back %v", got)
	}
	if stripBlock(once) != existing {
		t.Errorf("removing the block did not restore the file: %q", stripBlock(once))
	}
}

func TestUnitFileRunsWorkerAsServiceAccount(t *testing.T) {
	u := unitFile("x", ServiceUserName, helperUnit, []string{"/usr/local/bin/aiul", "run"},
		"/var/log/aiul/agent.log", "/var/log/aiul/agent.err.log",
		map[string]string{DevAllowUnmanagedVar: "1"})
	for _, want := range []string{
		"User=_aiul\n",
		"After=aiul-helper.service\n",
		"Environment=AIUL_STATE_DIR=/var/db/aiul\n",
		"Environment=AIUL_DEV_ALLOW_UNMANAGED=1\n",
		"ExecStart=/usr/local/bin/aiul run\n",
		"StandardError=append:/var/log/aiul/agent.err.log\n",
	} {
		if !strings.Contains(u, want) {
			t.Errorf("unit lacks %q:\n%s", want, u)
		}
	}
}
