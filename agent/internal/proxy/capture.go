package proxy

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/pkisan/aiul/internal/platform"
	"github.com/pkisan/aiul/internal/tasks"
)

// capture is the interception path, used only for allow-listed AI hosts.
//
// Two TLS connections are made:
//
//  1. Upstream, to the real provider, verified normally against the system root
//     certificates. Rule 5: this side is never weakened. If the provider's
//     certificate is bad, the connection fails exactly as it would without us.
//  2. Downstream, to the client, using a certificate we mint that claims the same
//     names as the real one.
//
// We connect upstream FIRST, because that is where we learn which names the real
// certificate carries — so our copy claims exactly what the genuine server claims
// and nothing more.
// handshakeFailure explains why a client would not complete a handshake with us,
// as far as its ALPN offer can say. A client that never offered http/1.1 did not
// refuse our certificate — it asked for a protocol this proxy does not serve yet,
// and no certificate or trust store change will help it.
// clientObjected says whether the client actually refused our certificate.
//
// A client that distrusts a certificate SAYS SO: TLS has alerts for exactly this,
// and Go surfaces them as tls.AlertError or as "remote error: tls: ...". Cursor,
// which genuinely pins, produces "remote error: tls: unknown certificate".
//
// A bare EOF or a TCP reset is not an objection. It is the connection of a
// process that exited, and short-lived children do that constantly. Treating
// silence as refusal is what tunnelled the Claude desktop app's executable within
// seconds of every launch, so its prompts were never captured.
func clientObjected(err error) bool {
	var alert tls.AlertError
	if errors.As(err, &alert) {
		return true
	}
	// Go does not export the type it uses for an alert received from the peer, so
	// the text is the only handle on it. It is stable and specific.
	return strings.Contains(err.Error(), "remote error: tls:")
}

// helloFingerprint describes a ClientHello in one field: enough to tell one TLS
// stack from another (Node, Chromium, Go, curl all look different) without
// recording anything about the conversation inside it.
func helloFingerprint(hello *tls.ClientHelloInfo) string {
	versions := make([]string, 0, len(hello.SupportedVersions))
	for _, v := range hello.SupportedVersions {
		switch v {
		case tls.VersionTLS13:
			versions = append(versions, "1.3")
		case tls.VersionTLS12:
			versions = append(versions, "1.2")
		case tls.VersionTLS11:
			versions = append(versions, "1.1")
		case tls.VersionTLS10:
			versions = append(versions, "1.0")
		default:
			versions = append(versions, fmt.Sprintf("0x%04x", v))
		}
	}

	first := hello.CipherSuites
	if len(first) > 3 {
		first = first[:3]
	}
	top := make([]string, 0, len(first))
	for _, c := range first {
		top = append(top, fmt.Sprintf("0x%04x", c))
	}

	return fmt.Sprintf("tls=%s ciphers=%d(%s) curves=%d sigalgs=%d sni=%q alpn=%d",
		strings.Join(versions, "/"), len(hello.CipherSuites), strings.Join(top, ","),
		len(hello.SupportedCurves), len(hello.SignatureSchemes),
		hello.ServerName, len(hello.SupportedProtos))
}

// onlyOtherProtocols reports a client that offered ALPN but not http/1.1 — in
// practice one that speaks only HTTP/2. We cannot serve it, so the handshake
// fails on OUR side with no alert from the client.
func onlyOtherProtocols(offeredALPN []string) bool {
	return len(offeredALPN) > 0 && !slices.Contains(offeredALPN, "http/1.1")
}

// shouldTunnel decides whether a failed handshake means "pass this program
// through sealed from now on" (rule 4). Either the client objected with an alert,
// or it wants a protocol we do not serve. Without the second case a client like
// Cursor's h2-only call to api2direct.cursor.sh failed on every retry, forever.
func shouldTunnel(err error, offeredALPN []string) bool {
	return clientObjected(err) || onlyOtherProtocols(offeredALPN)
}

func handshakeFailure(offeredALPN []string) (reason, hint string) {
	if onlyOtherProtocols(offeredALPN) {
		return "the client speaks none of the protocols we serve (we serve http/1.1 only)",
			"this is not a certificate problem: the proxy has to speak HTTP/2 to capture this tool"
	}
	return "the client rejected our certificate",
		"the tool may need its own CA environment variable, see the capture matrix"
}

func (p *Proxy) capture(clientConn net.Conn, clientReader io.Reader, upstream net.Conn, hostport string, client platform.Process, ctx tasks.Info) {
	host := normalizeHost(hostport)
	started := time.Now()

	// ---- 1. upstream, fully verified -------------------------------------------
	upstreamTLS := tls.Client(upstream, &tls.Config{
		ServerName: host,
		// HTTP/1.1 only for now. Advertising h2 would mean speaking HTTP/2 to the
		// client as well, which we do not implement yet.
		NextProtos: []string{"http/1.1"},
		MinVersion: tls.VersionTLS12,
		// RootCAs nil means the system roots. No InsecureSkipVerify. Rule 5.
		RootCAs: p.cfg.UpstreamRootCAs,
	})
	if err := upstreamTLS.HandshakeContext(handshakeContext()); err != nil {
		p.log.Warn("upstream TLS handshake failed", "host", host, "err", err)
		return
	}
	defer upstreamTLS.Close()

	// ---- 2. downstream, with a certificate we mint ------------------------------
	cert, err := p.certs.Get(host)
	if err != nil {
		p.log.Error("minting failed", "host", host, "err", err)
		return
	}

	// What the client offered, recorded before the handshake can fail. A client
	// that never sent a ClientHello never saw our certificate, so it cannot have
	// refused it; and when one does refuse, its ClientHello is the only
	// description we have of what it is.
	var (
		offeredALPN []string
		helloSeen   bool
		fingerprint string
	)

	clientTLS := tls.Server(rewindConn{Conn: clientConn, reader: clientReader}, &tls.Config{
		Certificates: []tls.Certificate{*cert},
		NextProtos:   []string{"http/1.1"},
		MinVersion:   tls.VersionTLS12,
		GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
			helloSeen = true
			offeredALPN = hello.SupportedProtos
			fingerprint = helloFingerprint(hello)
			// nil means "carry on with the configuration you already have".
			return nil, nil
		},
	})

	if err := clientTLS.HandshakeContext(handshakeContext()); err != nil {
		if !shouldTunnel(err, offeredALPN) {
			// The connection died without the client ever saying it disliked
			// anything: a bare EOF or a TCP reset, which is what the kernel sends
			// for the sockets of a process that has exited. The Claude desktop app
			// spawns short-lived children that open a connection and are gone
			// before the handshake finishes, and reading that as pinning
			// condemned the executable for every later connection — including the
			// ones carrying the prompts.
			p.log.Debug("handshake ended without an objection from the client; nothing recorded",
				"host", host, "process", client.Name, "hello", fingerprint,
				"hello_seen", helloSeen, "err", err)
			return
		}

		// Rule 4: the client SAID it would not accept our certificate. It may be
		// certificate pinning or a runtime that does not read the trust store.
		// Either way we must never break the tool: remember it and pass it through
		// sealed from now on, including this very connection, which the client
		// will retry.
		//
		// Keyed by the program, not the host alone: a pinned app must not stop us
		// capturing another tool's traffic to the same provider. The key is the
		// executable, not the process name, because the Claude desktop app's
		// bundled Claude Code and the terminal CLI are both called "claude".
		atFailure, _ := p.processOf(clientConn)

		if p.cfg.Classifier.AddTunnel(host, client.Identity()) {
			reason, hint := handshakeFailure(offeredALPN)
			p.log.Warn("tunneling this host for this program from now on",
				"host", host, "reason", reason, "process", client.Name,
				"executable", client.Path, "process_at_failure", atFailure.Name,
				"executable_at_failure", atFailure.Path,
				"alpn", strings.Join(offeredALPN, ","), "hello", fingerprint,
				"err", err, "hint", hint)
		}
		return
	}

	defer clientTLS.Close()

	// The same description for a handshake that worked. Comparing a working
	// client with a refusing one is the whole point of recording it.
	p.log.Debug("client handshake succeeded", "host", host,
		"process", client.Name, "executable", client.Path, "hello", fingerprint)

	if proto := clientTLS.ConnectionState().NegotiatedProtocol; proto != "" && proto != "http/1.1" {
		p.log.Warn("client negotiated a protocol we do not parse", "host", host, "protocol", proto)
	}

	// ---- 3. request/response loop ----------------------------------------------
	// Both sides are now plaintext to us. A connection is usually reused for
	// several requests, so loop until it closes.
	clientBuf := bufio.NewReader(clientTLS)
	upstreamBuf := bufio.NewReader(upstreamTLS)

	// Which request this is on this connection. Connections are reused, and an
	// exchange that fails on the second or third request of a connection is a
	// different fault from one that fails on the first: it means our single
	// upstream connection went away while the client still had questions.
	requests := 0

	for {
		req, err := http.ReadRequest(clientBuf)
		if err != nil {
			if !errors.Is(err, io.EOF) && !isClosed(err) {
				p.log.Debug("client connection ended", "host", host, "err", err,
					"requests_served", requests)
			}
			return
		}
		requests++

		method, path := req.Method, req.URL.Path

		if err := p.forward(req, clientBuf, clientTLS, upstreamTLS, upstreamBuf, host, started, ctx); err != nil {
			p.log.Debug("forwarding ended", "host", host, "err", err,
				"method", method, "path", path, "request_on_connection", requests)
			return
		}
	}
}

// rewindConn lets the TLS server read bytes the client already sent before we
// hijacked the connection, then continue reading from the socket itself.
type rewindConn struct {
	net.Conn
	reader io.Reader
}

func (c rewindConn) Read(b []byte) (int, error) { return c.reader.Read(b) }

func isClosed(err error) bool {
	return errors.Is(err, net.ErrClosed) || errors.Is(err, io.ErrUnexpectedEOF)
}
