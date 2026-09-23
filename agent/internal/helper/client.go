package helper

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"
)

// Client is the unprivileged side. Every privileged thing the worker needs goes
// through one of these three methods.
type Client struct {
	path string

	mu sync.Mutex // one request at a time: the protocol is a line each way
}

func NewClient(path string) *Client {
	if path == "" {
		path = SocketPath
	}

	return &Client{path: path}
}

// Available reports whether a helper is listening. When it is not — running the
// agent by hand in development — the worker does the unprivileged parts and skips
// the rest, rather than failing.
func (c *Client) Available() bool {
	conn, err := net.DialTimeout("unix", c.path, 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()

	return c.Ping() == nil
}

func (c *Client) Ping() error {
	_, err := c.send(VerbPing)

	return err
}

func (c *Client) ProxyOn() error {
	_, err := c.send(VerbProxyOn)

	return err
}

func (c *Client) ProxyOff() error {
	_, err := c.send(VerbProxyOff)

	return err
}

// ProcessOnPort asks who owns a connection from this local port, and what is
// checked out where that process is working. The repository and branch come back
// in the same answer because the worker cannot read them itself — see the note in
// the server.
func (c *Client) ProcessOnPort(port int) (pid int, name, workingDir, repo, branch, executable, account string, err error) {
	fields, err := c.send(VerbProcess + " " + strconv.Itoa(port))
	if err != nil {
		return 0, "", "", "", "", "", "", err
	}
	// Six fields since the executable was added. Five is a helper from an older
	// build, which happens for the moments of an upgrade before the helper
	// restarts: take what it sent rather than failing the lookup.
	// Seven since the Claude account was added.
	if len(fields) < 5 || len(fields) > 7 {
		return 0, "", "", "", "", "", "", fmt.Errorf("helper: expected five to seven fields, got %d", len(fields))
	}

	pid, err = strconv.Atoi(fields[0])
	if err != nil {
		return 0, "", "", "", "", "", "", fmt.Errorf("helper: unreadable pid %q", fields[0])
	}
	if len(fields) >= 6 {
		executable = fields[5]
	}
	if len(fields) == 7 {
		account = fields[6]
	}

	return pid, fields[1], fields[2], fields[3], fields[4], executable, account, nil
}

// send writes one line and reads one line. A new connection per request keeps this
// simple, and these calls are rare enough that it costs nothing worth saving.
func (c *Client) send(request string) ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	conn, err := net.DialTimeout("unix", c.path, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("helper unavailable: %w", err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))

	if _, err := fmt.Fprintln(conn, request); err != nil {
		return nil, fmt.Errorf("helper: %w", err)
	}

	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("helper: no reply: %w", err)
	}

	return ParseReply(line)
}
