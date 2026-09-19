package proxy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/pkisan/aiul/internal/ca"
	"github.com/pkisan/aiul/internal/platform"
	"github.com/pkisan/aiul/internal/redact"
	"github.com/pkisan/aiul/internal/tasks"
)

// How an explicit HTTPS proxy works, in short.
//
// A client told to use a proxy does not connect to the website. It connects to us
// and sends one line of plain text:
//
//	CONNECT api.openai.com:443 HTTP/1.1
//
// We answer "HTTP/1.1 200 Connection established" and from then on the socket is
// a plain pipe. The client starts its TLS handshake through that pipe, believing
// it is talking to api.openai.com.
//
// That is the moment everything hinges on. We already know the hostname — from the
// CONNECT line, before any certificate exists — so we can decide:
//
//   - pass or tunnel: open a socket to the real server and copy bytes both ways.
//     We never see inside; there is nothing to decrypt and nothing to log.
//   - capture: we complete the TLS handshake ourselves using a certificate we mint
//     for that hostname, and open our own verified TLS connection to the real
//     server. Two encrypted connections, with us in the middle.

// Config holds everything the proxy needs.
type Config struct {
	// Addr is where the proxy listens. Loopback only by default: nothing on the
	// network can use us as an open proxy.
	Addr string

	// Root signs the certificates we mint. Required only for capture.
	Root *ca.Root

	// Classifier decides capture / tunnel / pass.
	Classifier *Classifier

	// Logger receives operational messages. Never prompt content.
	Logger *slog.Logger

	// DialTimeout bounds how long we wait for the real server.
	DialTimeout time.Duration

	// Sink receives one Event per captured interaction. Nil means nothing is
	// recorded, which is what the tests of the sealed paths want.
	Sink Sink

	// UpstreamRootCAs is the set of certificate authorities we verify the REAL
	// provider against. Nil means the normal system roots, which is what
	// production always uses. It exists so tests can run against a local fake
	// provider, and so a company with its own internal root can add it.
	//
	// Rule 5: this only ever ADDS roots. There is no option to skip verification,
	// and tls.Config.InsecureSkipVerify appears nowhere in this package.
	UpstreamRootCAs *x509.CertPool

	// Tasks resolves a working directory to a git branch and task ID. Nil disables
	// task tagging, which is what most tests want.
	Tasks *tasks.Resolver

	// Processes finds which program opened a connection. Nil disables the lookup.
	Processes platform.ProcessFinder

	// Checkout reads the repository root and branch of a directory. Nil reads them
	// directly, which is right when the agent runs as the person using it. The
	// installed worker runs as a service account that cannot see into anyone's
	// home directory, so it passes a reader that asks the root helper instead.
	Checkout func(dir string) (repo, branch string)

	// Dial opens the connection to the real server. It exists so tests can point
	// every hostname at a local fake provider. Nil means an ordinary TCP dial.
	Dial func(network, addr string) (net.Conn, error)
}

// Proxy is the explicit HTTPS proxy.
type Proxy struct {
	cfg      Config
	certs    *CertCache
	log      *slog.Logger
	redactor *redact.Redactor
	server   *http.Server

	wg sync.WaitGroup
}

// New builds a proxy. It does not listen yet.
func New(cfg Config) (*Proxy, error) {
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:8899"
	}
	if cfg.Classifier == nil {
		cfg.Classifier = NewClassifier()
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = 30 * time.Second
	}
	if cfg.Root == nil {
		return nil, errors.New("proxy: no CA root; run 'aiul ca init' first")
	}

	p := &Proxy{
		cfg:   cfg,
		certs: NewCertCache(cfg.Root),
		log:   cfg.Logger,
		// Redaction is not optional and has no switch to turn it off. Rule 8.
		redactor: redact.New(),
	}
	p.server = &http.Server{
		Addr:    cfg.Addr,
		Handler: http.HandlerFunc(p.handle),
		// No ReadTimeout or WriteTimeout: a captured connection may be a long,
		// slowly streaming answer, and cutting it off would break the tool.
		ReadHeaderTimeout: 30 * time.Second,
	}
	return p, nil
}

// ListenAndServe starts the proxy and blocks until it is shut down.
func (p *Proxy) ListenAndServe() error {
	ln, err := net.Listen("tcp", p.cfg.Addr)
	if err != nil {
		return fmt.Errorf("proxy: listen on %s: %w", p.cfg.Addr, err)
	}
	p.log.Info("proxy listening", "addr", ln.Addr().String(), "allowlist_version", AllowListVersion)
	return p.server.Serve(ln)
}

// Serve runs the proxy on an existing listener. Tests use this.
func (p *Proxy) Serve(ln net.Listener) error { return p.server.Serve(ln) }

// Shutdown stops accepting new connections and waits briefly for existing ones.
func (p *Proxy) Shutdown(ctx context.Context) error {
	err := p.server.Shutdown(ctx)
	p.wg.Wait()
	return err
}

// handle is the entry point for everything the proxy receives.
func (p *Proxy) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		p.handleConnect(w, r)
		return
	}
	// A plain (non-CONNECT) proxy request is http:// traffic. AI tools use HTTPS,
	// so this is not our business; refuse clearly rather than half-supporting it.
	http.Error(w, "aiul proxy handles HTTPS (CONNECT) only", http.StatusNotImplemented)
}

func (p *Proxy) handleConnect(w http.ResponseWriter, r *http.Request) {
	hostport := r.Host // for CONNECT, this is exactly "host:port"
	if _, _, err := net.SplitHostPort(hostport); err != nil {
		hostport = net.JoinHostPort(hostport, "443")
	}

	decision := p.cfg.Classifier.Classify(hostport)

	// Hijacking takes the raw TCP connection away from net/http so we can use it as
	// a pipe. After this, net/http writes nothing to it and we own it entirely.
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "proxy: connection cannot be hijacked", http.StatusInternalServerError)
		return
	}
	clientConn, bufrw, err := hijacker.Hijack()
	if err != nil {
		p.log.Error("hijack failed", "host", hostport, "err", err)
		return
	}

	// Anything the client already sent after the CONNECT line is sitting in this
	// buffer and must be processed before we read from the socket again.
	buffered := bufrw.Reader.Buffered()

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		defer clientConn.Close()

		upstream, err := p.dial(hostport)
		if err != nil {
			p.log.Warn("upstream dial failed", "host", hostport, "err", err)
			writeConnectResponse(clientConn, http.StatusBadGateway)
			return
		}
		defer upstream.Close()

		if err := writeConnectResponse(clientConn, http.StatusOK); err != nil {
			return
		}

		// Give any already-buffered client bytes to whoever handles the connection.
		var clientReader io.Reader = clientConn
		if buffered > 0 {
			clientReader = io.MultiReader(io.LimitReader(bufrw.Reader, int64(buffered)), clientConn)
		}

		switch decision {
		case Capture:
			// The source port identifies the client process, and through it the
			// working directory and the task. Look it up now, while the connection
			// is open: ports are reused quickly.
			p.capture(clientConn, clientReader, upstream, hostport, p.contextOf(clientConn))
		default:
			// Pass and tunnel are byte-for-byte identical on the wire. The only
			// difference is that tunnel means "we know this is an AI host we chose
			// not to break", which will be logged as metadata.
			n1, n2 := pipe(clientConn, clientReader, upstream)
			p.log.Debug("passed through sealed",
				"host", hostport, "decision", decision.String(),
				"bytes_to_server", n1, "bytes_to_client", n2)
		}
	}()
}

// dial opens a plain TCP connection to the real server.
func (p *Proxy) dial(hostport string) (net.Conn, error) {
	if p.cfg.Dial != nil {
		return p.cfg.Dial("tcp", hostport)
	}
	d := &net.Dialer{Timeout: p.cfg.DialTimeout}
	return d.Dial("tcp", hostport)
}

// pipe copies bytes in both directions until either side closes, and returns how
// many bytes went each way. This is the sealed path: we never look at the content.
func pipe(client net.Conn, clientReader io.Reader, upstream net.Conn) (toServer, toClient int64) {
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		toServer, _ = io.Copy(upstream, clientReader)
		// Tell the server we are done sending; it may still have data for us.
		closeWrite(upstream)
	}()
	go func() {
		defer wg.Done()
		toClient, _ = io.Copy(client, upstream)
		closeWrite(client)
	}()

	wg.Wait()
	return toServer, toClient
}

// closeWrite half-closes a TCP connection: no more data from us, but we keep
// reading. Without it, a server waiting for the end of a request would hang.
func closeWrite(c net.Conn) {
	type closeWriter interface{ CloseWrite() error }
	if cw, ok := c.(closeWriter); ok {
		_ = cw.CloseWrite()
		return
	}
	if tc, ok := c.(*tls.Conn); ok {
		_ = tc.CloseWrite()
	}
}

func writeConnectResponse(c net.Conn, status int) error {
	var line string
	if status == http.StatusOK {
		line = "HTTP/1.1 200 Connection established\r\n\r\n"
	} else {
		line = fmt.Sprintf("HTTP/1.1 %d %s\r\n\r\n", status, http.StatusText(status))
	}
	_, err := io.WriteString(c, line)
	return err
}
