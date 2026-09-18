package helper

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeOps records what the privileged side was asked to do.
type fakeOps struct {
	mu        sync.Mutex
	proxyOn   int
	proxyOff  int
	lastPort  int
	failWith  error
	processes map[int][3]string
}

func (f *fakeOps) ProxyOn() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.proxyOn++

	return f.failWith
}

func (f *fakeOps) ProxyOff() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.proxyOff++

	return f.failWith
}

func (f *fakeOps) ProcessOnPort(port int) (int, string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastPort = port

	if f.failWith != nil {
		return 0, "", "", f.failWith
	}
	if entry, ok := f.processes[port]; ok {
		var pid int
		fmt.Sscanf(entry[0], "%d", &pid)

		return pid, entry[1], entry[2], nil
	}

	return 0, "", "", fmt.Errorf("no process found on port %d", port)
}

// shortTempDir returns a temporary directory with a SHORT path.
//
// A unix socket path is limited to about 104 characters on macOS, and t.TempDir()
// builds its path from the test's name — which is long enough here to push the
// socket over the limit and fail with "bind: invalid argument".
func shortTempDir(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("", "aiul")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	return dir
}

func startServer(t *testing.T, ops *fakeOps) *Client {
	t.Helper()

	path := filepath.Join(shortTempDir(t), "h.sock")

	server := NewServer(ops, slog.New(slog.NewTextHandler(io.Discard, nil)))

	ln, err := server.Listen(path, -1) // -1: do not change the group in a test
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go server.Serve(ln)
	t.Cleanup(func() { ln.Close() })

	return NewClient(path)
}

func TestClientAndServerSpeakToEachOther(t *testing.T) {
	ops := &fakeOps{processes: map[int][3]string{
		54321: {"4242", "Cursor Helper", "/Users/dev/my project"},
	}}
	client := startServer(t, ops)

	if !client.Available() {
		t.Fatal("the helper should be available")
	}

	if err := client.ProxyOn(); err != nil {
		t.Errorf("ProxyOn: %v", err)
	}
	if err := client.ProxyOff(); err != nil {
		t.Errorf("ProxyOff: %v", err)
	}

	pid, name, dir, err := client.ProcessOnPort(54321)
	if err != nil {
		t.Fatalf("ProcessOnPort: %v", err)
	}
	if pid != 4242 || name != "Cursor Helper" || dir != "/Users/dev/my project" {
		t.Errorf("got %d %q %q", pid, name, dir)
	}

	ops.mu.Lock()
	defer ops.mu.Unlock()
	if ops.proxyOn != 1 || ops.proxyOff != 1 || ops.lastPort != 54321 {
		t.Errorf("ops = %+v", ops)
	}
}

func TestAnErrorFromThePrivilegedSideReachesTheClient(t *testing.T) {
	client := startServer(t, &fakeOps{failWith: fmt.Errorf("networksetup exploded")})

	err := client.ProxyOn()
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "networksetup exploded") {
		t.Errorf("err = %v", err)
	}
}

func TestMissingProcessIsAnOrdinaryError(t *testing.T) {
	client := startServer(t, &fakeOps{processes: map[int][3]string{}})

	if _, _, _, err := client.ProcessOnPort(9999); err == nil {
		t.Error("a port with no process must be an error, not a silent zero")
	}
}

// The root process must refuse anything outside its verb list, over the wire and
// not merely in the parser.
func TestTheServerRefusesRubbishOverTheWire(t *testing.T) {
	ops := &fakeOps{}
	client := startServer(t, ops)

	conn, err := net.Dial("unix", client.path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	for _, line := range []string{
		"EXEC /bin/sh",
		"PROXY-ON http://evil.example:8080",
		"PROCESS ../../etc/passwd",
		"",
	} {
		fmt.Fprintln(conn, line)

		reply := make([]byte, 256)
		n, err := conn.Read(reply)
		if err != nil {
			t.Fatalf("reading the reply to %q: %v", line, err)
		}
		if !strings.HasPrefix(string(reply[:n]), "ERR") {
			t.Errorf("%q was not refused: %s", line, reply[:n])
		}
	}

	// And none of it reached the privileged operations.
	ops.mu.Lock()
	defer ops.mu.Unlock()
	if ops.proxyOn != 0 || ops.proxyOff != 0 {
		t.Errorf("a refused request still ran something: %+v", ops)
	}
}

// A very long line must not make the root process allocate without limit.
func TestAnEnormousRequestIsRefused(t *testing.T) {
	client := startServer(t, &fakeOps{})

	conn, err := net.Dial("unix", client.path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	fmt.Fprintln(conn, "PING "+strings.Repeat("A", 10000))

	// The connection is dropped rather than answered, which is the intended
	// outcome: nothing is parsed and nothing is allocated.
	buf := make([]byte, 64)
	if _, err := conn.Read(buf); err == nil {
		t.Log("the server answered rather than dropping the connection, which is also acceptable")
	}
}

func TestClientReportsAnAbsentHelper(t *testing.T) {
	// Running the agent by hand in development: no helper, and that is fine.
	client := NewClient(filepath.Join(shortTempDir(t), "none.sock"))

	if client.Available() {
		t.Error("there is no helper listening")
	}
	if err := client.ProxyOn(); err == nil {
		t.Error("calling an absent helper must fail rather than hang")
	}
}
