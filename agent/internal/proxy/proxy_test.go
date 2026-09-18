package proxy

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pkisan/aiul/internal/ca"
)

// ---------------------------------------------------------------------------
// test helpers

// collector is a Sink that keeps events in memory.
type collector struct {
	mu     sync.Mutex
	events []Event
}

func (c *collector) Record(e Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
}

func (c *collector) all() []Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Event{}, c.events...)
}

// eventually polls until cond is true or the deadline passes.
func eventually(within time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

// newTestRoot creates a throwaway CA in a temporary HOME.
func newTestRoot(t *testing.T) *ca.Root {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	root, err := ca.Init(false)
	if err != nil {
		t.Fatalf("ca.Init: %v", err)
	}
	return root
}

// originServer is a TLS server standing in for a real AI provider. It uses its own
// CA, which the proxy must verify against — so the test proves rule 5 is honoured
// without ever touching a real host.
type originServer struct {
	addr     string
	hostname string
	rootPool *x509.CertPool
	close    func()
}

func newOriginServer(t *testing.T, hostname string, handler http.Handler) *originServer {
	t.Helper()

	// A separate CA that plays the part of a public certificate authority.
	providerCA, err := generateProviderCA(hostname)
	if err != nil {
		t.Fatalf("provider CA: %v", err)
	}
	cert, err := providerCA.MintLeaf(ca.LeafRequest{Hosts: []string{hostname, "127.0.0.1"}})
	if err != nil {
		t.Fatalf("provider leaf: %v", err)
	}

	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{*cert},
		NextProtos:   []string{"http/1.1"},
	})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	srv := &http.Server{Handler: handler}
	go srv.Serve(ln)

	pool := x509.NewCertPool()
	pool.AddCert(providerCA.Cert)

	return &originServer{
		addr:     ln.Addr().String(),
		hostname: hostname,
		rootPool: pool,
		close:    func() { srv.Close() },
	}
}

// generateProviderCA makes a second, unrelated CA for the fake provider.
func generateProviderCA(hostname string) (*ca.Root, error) {
	dir, err := os.MkdirTemp("", "provider-ca")
	if err != nil {
		return nil, err
	}
	old := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	defer os.Setenv("HOME", old)
	return ca.Init(true)
}

// startProxy runs a proxy on a random port, dialing every host at originAddr.
func startProxy(t *testing.T, cfg Config, originAddr string) (string, *Proxy) {
	t.Helper()
	if cfg.Logger == nil {
		cfg.Logger = quietLogger()
	}
	if originAddr != "" {
		cfg.Dial = func(network, _ string) (net.Conn, error) { return net.Dial(network, originAddr) }
	}
	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go p.Serve(ln)
	t.Cleanup(func() { ln.Close() })
	return ln.Addr().String(), p
}

// clientThrough builds an HTTP client that reaches everything through the proxy
// and trusts exactly the given roots. Never InsecureSkipVerify — rule 5 applies to
// tests too.
func clientThrough(proxyAddr string, roots *x509.CertPool) *http.Client {
	proxyURL, _ := url.Parse("http://" + proxyAddr)
	return &http.Client{
		Transport: &http.Transport{
			Proxy:           http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{RootCAs: roots},
		},
		Timeout: 10 * time.Second,
	}
}

// ---------------------------------------------------------------------------
// tests

// TestPassThroughIsSealed proves rule 3: a host that is not on the allow-list is
// never decrypted, so the client sees the ORIGIN's certificate, not ours.
func TestPassThroughIsSealed(t *testing.T) {
	root := newTestRoot(t)
	origin := newOriginServer(t, "not-an-ai-host.example", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "hello")
	}))
	defer origin.close()

	sink := &collector{}
	proxyAddr, _ := startProxy(t, Config{Root: root, Sink: sink}, origin.addr)

	// The client trusts ONLY the origin's CA. If the proxy had intercepted, the
	// handshake would fail — which is exactly the proof we want.
	resp, err := clientThrough(proxyAddr, origin.rootPool).Get("https://not-an-ai-host.example/x")
	if err != nil {
		t.Fatalf("sealed pass-through should work: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "hello" {
		t.Errorf("body = %q", body)
	}
	if len(sink.all()) != 0 {
		t.Error("a passed-through host must not produce any event")
	}
}

// TestCaptureReadsTheConversation proves the capture path works end to end and
// that the client ends up trusting OUR certificate, not the origin's.
func TestCaptureReadsTheConversation(t *testing.T) {
	root := newTestRoot(t)
	origin := newOriginServer(t, "api.openai.com", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "what is a proxy") {
			t.Errorf("the provider must receive the original request, got %q", body)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer origin.close()

	sink := &collector{}
	proxyAddr, _ := startProxy(t, Config{Root: root, Sink: sink, UpstreamRootCAs: origin.rootPool}, origin.addr)

	// Now the client trusts OUR root instead of the origin's: that is what an
	// endpoint with our CA installed looks like.
	ourPool := x509.NewCertPool()
	ourPool.AddCert(root.Cert)

	client := clientThrough(proxyAddr, ourPool)
	resp, err := client.Post("https://api.openai.com/v1/chat/completions", "application/json",
		strings.NewReader(`{"prompt":"what is a proxy"}`))
	if err != nil {
		t.Fatalf("capture request failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != `{"ok":true}` {
		t.Errorf("client body = %q, want the provider's unmodified answer", body)
	}

	// The event is written by the proxy's own goroutine, a moment after the client
	// has its answer, so wait for it rather than assuming an order.
	if !eventually(2*time.Second, func() bool { return len(sink.all()) == 1 }) {
		t.Fatalf("got %d events, want 1", len(sink.all()))
	}
	e := sink.all()[0]
	if e.Host != "api.openai.com" || e.Path != "/v1/chat/completions" || e.Status != 200 {
		t.Errorf("event = %+v", e)
	}
	if e.AllowListVersion != AllowListVersion {
		t.Errorf("event should record the allow-list version, got %d", e.AllowListVersion)
	}
}

// TestStreamingFlushesEachChunk is the rule 6 test: a chunk written by the
// provider must reach the client before the response is finished.
func TestStreamingFlushesEachChunk(t *testing.T) {
	root := newTestRoot(t)

	release := make(chan struct{})
	origin := newOriginServer(t, "api.anthropic.com", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		fmt.Fprint(w, "data: first\n\n")
		fl.Flush()
		// Hold the response open. If anything in the chain buffered, the client
		// below will block and the test fails on timeout.
		<-release
		fmt.Fprint(w, "data: second\n\n")
		fl.Flush()
	}))
	defer origin.close()

	proxyAddr, _ := startProxy(t, Config{Root: root, Sink: &collector{}, UpstreamRootCAs: origin.rootPool}, origin.addr)
	ourPool := x509.NewCertPool()
	ourPool.AddCert(root.Cert)

	client := clientThrough(proxyAddr, ourPool)
	resp, err := client.Get("https://api.anthropic.com/v1/messages")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// Read the first chunk. This must return while the handler is still blocked.
	first := make([]byte, 64)
	done := make(chan int, 1)
	go func() {
		n, _ := resp.Body.Read(first)
		done <- n
	}()

	select {
	case n := <-done:
		if !strings.Contains(string(first[:n]), "first") {
			t.Errorf("first chunk = %q", first[:n])
		}
	case <-time.After(3 * time.Second):
		close(release)
		t.Fatal("the first chunk did not arrive while the response was still open: something buffered it")
	}

	close(release)
	rest, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(rest), "second") {
		t.Errorf("rest = %q", rest)
	}
}

// TestClientRejectingOurCertIsTunneled is the rule 4 test: a client that refuses
// our certificate must not be broken — the host moves to the tunnel list and the
// next attempt passes through sealed.
func TestClientRejectingOurCertIsTunneled(t *testing.T) {
	root := newTestRoot(t)
	origin := newOriginServer(t, "api.openai.com", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "pinned-ok")
	}))
	defer origin.close()

	classifier := NewClassifier()
	proxyAddr, _ := startProxy(t, Config{Root: root, Classifier: classifier, Sink: &collector{}, UpstreamRootCAs: origin.rootPool}, origin.addr)

	// This client trusts only the ORIGIN's CA, so it rejects our minted
	// certificate — exactly what a pinning client does.
	pinned := clientThrough(proxyAddr, origin.rootPool)

	if _, err := pinned.Get("https://api.openai.com/v1/models"); err == nil {
		t.Fatal("the first attempt should fail: the client does not trust our certificate")
	}

	// The proxy must have learned from that failure. The client sees the error the
	// moment it rejects the certificate, a hair before the proxy's own goroutine
	// records it, so poll briefly rather than assuming an order.
	if !eventually(2*time.Second, func() bool { return classifier.Classify("api.openai.com") == Tunnel }) {
		t.Fatalf("after a rejected handshake, Classify = %v, want tunnel", classifier.Classify("api.openai.com"))
	}

	// And the retry must now succeed, sealed, with the origin's own certificate.
	resp, err := pinned.Get("https://api.openai.com/v1/models")
	if err != nil {
		t.Fatalf("after tunneling, the tool must work again: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "pinned-ok" {
		t.Errorf("body = %q", body)
	}
}

// TestUpstreamVerificationIsNotWeakened is the rule 5 test: if the provider's
// certificate does not verify, we fail rather than connect anyway.
func TestUpstreamVerificationIsNotWeakened(t *testing.T) {
	root := newTestRoot(t)
	// The origin's certificate is signed by a CA nothing trusts, and the proxy is
	// left with the system roots — so it must refuse.
	origin := newOriginServer(t, "api.openai.com", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "should never be reached")
	}))
	defer origin.close()

	proxyAddr, _ := startProxy(t, Config{Root: root, Sink: &collector{}}, origin.addr)
	ourPool := x509.NewCertPool()
	ourPool.AddCert(root.Cert)

	client := clientThrough(proxyAddr, ourPool)
	if _, err := client.Get("https://api.openai.com/v1/models"); err == nil {
		t.Fatal("the proxy must refuse an upstream certificate it cannot verify")
	}
}

func TestPlainHTTPIsRefused(t *testing.T) {
	root := newTestRoot(t)
	proxyAddr, _ := startProxy(t, Config{Root: root}, "")

	resp, err := http.Get("http://" + proxyAddr + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", resp.StatusCode)
	}
}

// TestStreamedResponseTerminatesPromptly guards the bug this test caught once
// already: a chunked response that never sends its terminator leaves the client
// hanging until its own timeout, which looks like "the tool is slow" in the field.
func TestStreamedResponseTerminatesPromptly(t *testing.T) {
	root := newTestRoot(t)
	origin := newOriginServer(t, "api.anthropic.com", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fl := w.(http.Flusher)
		for i := 0; i < 3; i++ {
			fmt.Fprintf(w, "data: chunk%d\n\n", i)
			fl.Flush()
		}
	}))
	defer origin.close()

	proxyAddr, _ := startProxy(t, Config{Root: root, Sink: &collector{}, UpstreamRootCAs: origin.rootPool}, origin.addr)
	ourPool := x509.NewCertPool()
	ourPool.AddCert(root.Cert)

	done := make(chan string, 1)
	go func() {
		resp, err := clientThrough(proxyAddr, ourPool).Get("https://api.anthropic.com/v1/messages")
		if err != nil {
			done <- "error: " + err.Error()
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		done <- string(body)
	}()

	select {
	case body := <-done:
		if !strings.Contains(body, "chunk2") {
			t.Errorf("body = %q", body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("reading the streamed response did not finish: the chunked terminator is missing")
	}
}

// TestCapturedStreamIsReassembledIntoOneAnswer is the Phase 2 milestone in test
// form: a streaming Anthropic conversation goes through the proxy, the client
// still receives it chunk by chunk, and the event holds the full prompt and the
// reassembled answer.
func TestCapturedStreamIsReassembledIntoOneAnswer(t *testing.T) {
	root := newTestRoot(t)

	sseBody, err := os.ReadFile(filepath.Join("..", "..", "testdata", "anthropic", "messages-stream.response.sse"))
	if err != nil {
		t.Fatal(err)
	}
	reqBody, err := os.ReadFile(filepath.Join("..", "..", "testdata", "anthropic", "messages-stream.request.json"))
	if err != nil {
		t.Fatal(err)
	}

	origin := newOriginServer(t, "api.anthropic.com", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ := io.ReadAll(r.Body)
		if len(got) != len(reqBody) {
			t.Errorf("the provider must receive the request unmodified: got %d bytes, want %d", len(got), len(reqBody))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		// Send it in small pieces, as a real provider does.
		for _, block := range strings.SplitAfter(string(sseBody), "\n\n") {
			fmt.Fprint(w, block)
			fl.Flush()
		}
	}))
	defer origin.close()

	sink := &collector{}
	proxyAddr, _ := startProxy(t, Config{Root: root, Sink: sink, UpstreamRootCAs: origin.rootPool}, origin.addr)
	ourPool := x509.NewCertPool()
	ourPool.AddCert(root.Cert)

	resp, err := clientThrough(proxyAddr, ourPool).Post(
		"https://api.anthropic.com/v1/messages", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	clientSaw, _ := io.ReadAll(resp.Body)

	// The client must receive the stream byte for byte.
	if !strings.Contains(string(clientSaw), "terminates the encrypted connection") {
		t.Errorf("the client did not receive the whole stream: %q", clientSaw)
	}

	if !eventually(2*time.Second, func() bool { return len(sink.all()) == 1 }) {
		t.Fatalf("got %d events, want 1", len(sink.all()))
	}
	e := sink.all()[0]

	if e.Parser != "anthropic" || e.Model != "claude-opus-5" {
		t.Errorf("parser=%q model=%q", e.Parser, e.Model)
	}
	if e.Prompt != "Explain what a TLS proxy does in two sentences." {
		t.Errorf("prompt = %q", e.Prompt)
	}
	want := "A TLS proxy sits between a client and a server and terminates the encrypted connection."
	if e.Answer != want {
		t.Errorf("answer = %q\nwant     %q", e.Answer, want)
	}
	if !e.Streamed {
		t.Error("the event should be marked streamed")
	}
	if e.PromptTokens != 24 || e.ResponseTokens != 37 {
		t.Errorf("tokens = %d/%d, want 24/37", e.PromptTokens, e.ResponseTokens)
	}
	if e.ID == "" {
		t.Error("every event needs an id")
	}
	if e.DurationMS < 0 {
		t.Error("duration should be recorded")
	}
}

// TestHousekeepingCallsAreNotStored: an allow-listed host makes many calls that
// are not conversations. They must not become events.
func TestHousekeepingCallsAreNotStored(t *testing.T) {
	root := newTestRoot(t)
	origin := newOriginServer(t, "api.anthropic.com", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"servers":[]}`)
	}))
	defer origin.close()

	sink := &collector{}
	proxyAddr, _ := startProxy(t, Config{Root: root, Sink: sink, UpstreamRootCAs: origin.rootPool}, origin.addr)
	ourPool := x509.NewCertPool()
	ourPool.AddCert(root.Cert)

	client := clientThrough(proxyAddr, ourPool)
	for _, path := range []string{"/mcp-registry/v0/servers", "/api/oauth/account/settings", "/api/event_logging/v2/batch"} {
		resp, err := client.Get("https://api.anthropic.com" + path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		resp.Body.Close()
	}

	// Give the proxy a moment to record anything it was going to record.
	time.Sleep(200 * time.Millisecond)
	if got := sink.all(); len(got) != 0 {
		t.Errorf("housekeeping calls produced %d events, want 0: %+v", len(got), got)
	}
}
