// Package helper is the small privileged surface of the agent.
//
// Why it exists. The proxy parses bytes that arrive from the network: TLS
// records, HTTP headers, JSON bodies, compressed streams. That is the code an
// attacker gets to influence, and it must not be running as root. But three
// things the agent does DO need root: changing the system proxy, and asking which
// process owns a connection (lsof cannot see another user's processes otherwise).
//
// So the agent is two processes. A tiny root helper answers a fixed list of verbs
// over a unix socket and never touches network traffic. Everything else runs as an
// unprivileged user and asks the helper when it needs one of those three things.
//
// The protocol is deliberately dull: one line in, one line out, a closed list of
// verbs, and the only argument anywhere is a port number that must parse as an
// integer in range. There is no verb that takes a path, a command, or a hostname,
// because a privileged helper that accepts a string someone else chose is a
// privilege escalation waiting to be found.
package helper

import (
	"fmt"
	"strconv"
	"strings"
)

// SocketPath is where the helper listens. /var/run is root-owned, so nobody
// unprivileged can replace the socket with their own and impersonate the helper.
const SocketPath = "/var/run/aiul-helper.sock"

// The complete list of verbs. Adding one is a security decision, not a detail.
const (
	// VerbPing checks the helper is alive.
	VerbPing = "PING"

	// VerbProxyOn points the system proxy at our listener. It takes NO argument:
	// the address is compiled in, so nothing the worker says can redirect the
	// machine's traffic somewhere else.
	VerbProxyOn = "PROXY-ON"

	// VerbProxyOff removes the system proxy. The fail-open path, so it must work
	// even when everything else is broken.
	VerbProxyOff = "PROXY-OFF"

	// VerbProcess asks which process owns a connection from a local port, and
	// where it is working. The argument is a port number and nothing else.
	VerbProcess = "PROCESS"
)

// Replies. A reply is one line: "OK", "OK <payload>", or "ERR <reason>".
const (
	replyOK  = "OK"
	replyErr = "ERR"
)

// Request is one parsed command.
type Request struct {
	Verb string
	Port int // only meaningful for VerbProcess
}

// ParseRequest reads one line from the worker.
//
// This function is the whole attack surface of the privileged process, so it is
// strict: an unknown verb, a missing argument, an extra argument, or a port
// outside the valid range is rejected outright rather than interpreted
// generously.
func ParseRequest(line string) (Request, error) {
	fields := strings.Fields(strings.TrimSpace(line))

	if len(fields) == 0 {
		return Request{}, fmt.Errorf("empty request")
	}

	verb := strings.ToUpper(fields[0])
	args := fields[1:]

	switch verb {
	case VerbPing, VerbProxyOn, VerbProxyOff:
		if len(args) != 0 {
			return Request{}, fmt.Errorf("%s takes no arguments", verb)
		}
		return Request{Verb: verb}, nil

	case VerbProcess:
		if len(args) != 1 {
			return Request{}, fmt.Errorf("%s takes exactly one argument", verb)
		}
		port, err := strconv.Atoi(args[0])
		if err != nil {
			return Request{}, fmt.Errorf("%s: the argument must be a port number", verb)
		}
		// A port outside this range cannot be a real connection, so it is either a
		// mistake or someone probing.
		if port < 1 || port > 65535 {
			return Request{}, fmt.Errorf("%s: port %d is out of range", verb, port)
		}
		return Request{Verb: verb, Port: port}, nil

	default:
		// Deliberately does not echo the verb back: no reason to reflect whatever
		// someone sent into a root process's logs.
		return Request{}, fmt.Errorf("unknown verb")
	}
}

// FormatOK builds a success reply. Payload fields are joined with tabs, so a value
// containing spaces (a path, a process name) survives the round trip.
func FormatOK(fields ...string) string {
	if len(fields) == 0 {
		return replyOK
	}
	return replyOK + " " + strings.Join(fields, "\t")
}

// FormatErr builds a failure reply. Newlines are stripped so an error can never
// turn into two lines and desynchronise the protocol.
func FormatErr(err error) string {
	message := strings.ReplaceAll(err.Error(), "\n", " ")
	return replyErr + " " + strings.TrimSpace(message)
}

// ParseReply reads the helper's answer.
func ParseReply(line string) ([]string, error) {
	line = strings.TrimRight(line, "\r\n")

	status, rest, _ := strings.Cut(line, " ")

	switch status {
	case replyOK:
		if rest == "" {
			return nil, nil
		}
		return strings.Split(rest, "\t"), nil
	case replyErr:
		return nil, fmt.Errorf("helper: %s", rest)
	default:
		return nil, fmt.Errorf("helper: unreadable reply %q", line)
	}
}
