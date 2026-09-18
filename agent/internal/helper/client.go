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

// ProcessOnPort asks who owns a connection from this local port.
func (c *Client) ProcessOnPort(port int) (pid int, name, workingDir string, err error) {
	fields, err := c.send(VerbProcess + " " + strconv.Itoa(port))
	if err != nil {
		return 0, "", "", err
	}
	if len(fields) != 3 {
		return 0, "", "", fmt.Errorf("helper: expected three fields, got %d", len(fields))
	}

	pid, err = strconv.Atoi(fields[0])
	if err != nil {
		return 0, "", "", fmt.Errorf("helper: unreadable pid %q", fields[0])
	}

	return pid, fields[1], fields[2], nil
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
