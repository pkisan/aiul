package proxy

import (
	"bufio"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"time"

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
func (p *Proxy) capture(clientConn net.Conn, clientReader io.Reader, upstream net.Conn, hostport, client string, ctx tasks.Info) {
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

	state := upstreamTLS.ConnectionState()
	sans := sanNames(state)

	// ---- 2. downstream, with a certificate we mint ------------------------------
	cert, err := p.certs.Get(host, sans)
	if err != nil {
		p.log.Error("minting failed", "host", host, "err", err)
		return
	}

	clientTLS := tls.Server(rewindConn{Conn: clientConn, reader: clientReader}, &tls.Config{
		Certificates: []tls.Certificate{*cert},
		NextProtos:   []string{"http/1.1"},
		MinVersion:   tls.VersionTLS12,
	})

	if err := clientTLS.HandshakeContext(handshakeContext()); err != nil {
		// Rule 4: the client refused our certificate. This is certificate pinning,
		// or a runtime that does not read the trust store. Either way we must never
		// break the tool: remember the host and pass it through sealed from now on,
		// including this very connection, which the client will retry.
		// Keyed by the program, not the host alone: a pinned app must not stop us
		// capturing another tool's traffic to the same provider.
		if p.cfg.Classifier.AddTunnel(host, client) {
			p.log.Warn("client rejected our certificate; tunneling this host for this program from now on",
				"host", host, "process", client, "err", err,
				"hint", "the tool may need its own CA environment variable, see the capture matrix")
		}
		return
	}
	defer clientTLS.Close()

	if proto := clientTLS.ConnectionState().NegotiatedProtocol; proto != "" && proto != "http/1.1" {
		p.log.Warn("client negotiated a protocol we do not parse", "host", host, "protocol", proto)
	}

	// ---- 3. request/response loop ----------------------------------------------
	// Both sides are now plaintext to us. A connection is usually reused for
	// several requests, so loop until it closes.
	clientBuf := bufio.NewReader(clientTLS)
	upstreamBuf := bufio.NewReader(upstreamTLS)

	for {
		req, err := http.ReadRequest(clientBuf)
		if err != nil {
			if !errors.Is(err, io.EOF) && !isClosed(err) {
				p.log.Debug("client connection ended", "host", host, "err", err)
			}
			return
		}

		if err := p.forward(req, clientTLS, upstreamTLS, upstreamBuf, host, started, ctx); err != nil {
			p.log.Debug("forwarding ended", "host", host, "err", err)
			return
		}
	}
}

// sanNames returns the names on the real server's certificate, so our minted copy
// claims the same ones. If the real certificate somehow has none, fall back to the
// server name we asked for.
func sanNames(state tls.ConnectionState) []string {
	if len(state.PeerCertificates) == 0 {
		return nil
	}
	leaf := state.PeerCertificates[0]
	names := append([]string{}, leaf.DNSNames...)
	for _, ip := range leaf.IPAddresses {
		names = append(names, ip.String())
	}
	if len(names) == 0 && state.ServerName != "" {
		names = []string{state.ServerName}
	}
	return names
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
