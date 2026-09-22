package helper

import (
	"bufio"
	"fmt"
	"log/slog"
	"net"
	"os"
	"time"

	"github.com/pkisan/aiul/internal/tasks"
)

// Privileged is the small set of operations the helper is willing to perform.
// Nothing else in the root process does any work.
type Privileged interface {
	ProxyOn() error
	ProxyOff() error
	ProcessOnPort(port int) (pid int, name, executable, workingDir string, err error)
}

// Server is the root side. It answers verbs and nothing more: it never opens a
// network socket, never reads a request body, never parses anything an AI provider
// sent.
type Server struct {
	ops Privileged
	log *slog.Logger
}

func NewServer(ops Privileged, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}

	return &Server{ops: ops, log: log}
}

// Listen opens the unix socket with restrictive permissions.
//
// 0660 with the socket's group set to the worker's group means: root writes it,
// the worker may use it, and no other account on the machine can reach it.
func (s *Server) Listen(path string, gid int) (net.Listener, error) {
	// A socket left behind by a crash would stop us binding.
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove the stale socket %s: %w", path, err)
	}

	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", path, err)
	}

	if gid >= 0 {
		if err := os.Chown(path, 0, gid); err != nil {
			ln.Close()

			return nil, fmt.Errorf("set the socket group: %w", err)
		}
	}
	if err := os.Chmod(path, 0o660); err != nil {
		ln.Close()

		return nil, fmt.Errorf("set the socket permissions: %w", err)
	}

	return ln, nil
}

// Serve answers requests until the listener is closed.
func (s *Server) Serve(ln net.Listener) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}

		go s.handle(conn)
	}
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()

	// A caller that opens the socket and says nothing must not hold a root
	// goroutine for ever.
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))

	reader := bufio.NewReader(conn)

	for {
		// A bounded read: nobody gets to make the root process allocate without
		// limit by sending a very long line.
		line, err := readLine(reader, 256)
		if err != nil {
			return
		}

		reply := s.answer(line)

		if _, err := fmt.Fprintln(conn, reply); err != nil {
			return
		}

		_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	}
}

func (s *Server) answer(line string) string {
	req, err := ParseRequest(line)
	if err != nil {
		s.log.Warn("refused a request", "err", err)

		return FormatErr(err)
	}

	switch req.Verb {
	case VerbPing:
		return FormatOK()

	case VerbProxyOn:
		if err := s.ops.ProxyOn(); err != nil {
			return FormatErr(err)
		}
		s.log.Info("system proxy on")

		return FormatOK()

	case VerbProxyOff:
		if err := s.ops.ProxyOff(); err != nil {
			return FormatErr(err)
		}
		s.log.Info("system proxy off")

		return FormatOK()

	case VerbProcess:
		pid, name, exe, dir, err := s.ops.ProcessOnPort(req.Port)
		if err != nil {
			return FormatErr(err)
		}

		// The checkout is read here because only root can: the worker's account
		// cannot traverse into a person's home directory, so it cannot read
		// .git/HEAD itself. This is a file read of a path the worker did not
		// choose — the directory came from the process that opened the connection
		// — and the task ID is still worked out in the worker.
		repo, branch, readErr := tasks.CheckoutAtVerbose(dir)
		if repo != "" && branch == "" && readErr != nil {
			// Worth a warning, not silence: on macOS ~/Desktop, ~/Documents and
			// ~/Downloads are protected by TCC, and a daemon without Full Disk
			// Access can stat .git but not open .git/HEAD. Every interaction in
			// such a checkout is then recorded with no branch at all.
			s.log.Warn("found the repository but could not read its branch",
				"repo", repo, "err", readErr,
				"hint", "grant Full Disk Access to /usr/local/bin/aiul (PPPC profile), see docs/SETUP-MAC.md")
		}

		// The executable, not just the process name: the Claude desktop app
		// bundles its own Claude Code, and both are called "claude".
		return FormatOK(fmt.Sprintf("%d", pid), name, dir, repo, branch, exe)

	default:
		// Unreachable: ParseRequest only returns verbs listed above. Kept so that
		// adding a verb without handling it fails closed rather than open.
		return FormatErr(fmt.Errorf("unhandled verb"))
	}
}

// readLine reads one line, refusing anything longer than max bytes.
func readLine(r *bufio.Reader, max int) (string, error) {
	var out []byte

	for {
		b, err := r.ReadByte()
		if err != nil {
			return "", err
		}
		if b == '\n' {
			return string(out), nil
		}
		if len(out) >= max {
			return "", fmt.Errorf("request too long")
		}
		out = append(out, b)
	}
}
