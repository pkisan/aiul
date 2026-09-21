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
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pkisan/aiul/internal/ca"
	"github.com/pkisan/aiul/internal/platform"
	"github.com/pkisan/aiul/internal/tasks"
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
	proxyAddr, _ := startProxy(t, Config{Issuer: root, Sink: sink}, origin.addr)

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
	proxyAddr, _ := startProxy(t, Config{Issuer: root, Sink: sink, UpstreamRootCAs: origin.rootPool}, origin.addr)

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

	proxyAddr, _ := startProxy(t, Config{Issuer: root, Sink: &collector{}, UpstreamRootCAs: origin.rootPool}, origin.addr)
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
	proxyAddr, _ := startProxy(t, Config{Issuer: root, Classifier: classifier, Sink: &collector{}, UpstreamRootCAs: origin.rootPool}, origin.addr)

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

	proxyAddr, _ := startProxy(t, Config{Issuer: root, Sink: &collector{}}, origin.addr)
	ourPool := x509.NewCertPool()
	ourPool.AddCert(root.Cert)

	client := clientThrough(proxyAddr, ourPool)
	if _, err := client.Get("https://api.openai.com/v1/models"); err == nil {
		t.Fatal("the proxy must refuse an upstream certificate it cannot verify")
	}
}

func TestPlainHTTPIsRefused(t *testing.T) {
	root := newTestRoot(t)
	proxyAddr, _ := startProxy(t, Config{Issuer: root}, "")

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

	proxyAddr, _ := startProxy(t, Config{Issuer: root, Sink: &collector{}, UpstreamRootCAs: origin.rootPool}, origin.addr)
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
	proxyAddr, _ := startProxy(t, Config{Issuer: root, Sink: sink, UpstreamRootCAs: origin.rootPool}, origin.addr)
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
	proxyAddr, _ := startProxy(t, Config{Issuer: root, Sink: sink, UpstreamRootCAs: origin.rootPool}, origin.addr)
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

// TestSecretsAreMaskedButTheProviderGetsTheOriginal is the Phase 3 milestone: a
// fake API key in a prompt is masked in the stored event, while the provider
// receives the request exactly as the client sent it. Rule 8.
func TestSecretsAreMaskedButTheProviderGetsTheOriginal(t *testing.T) {
	root := newTestRoot(t)

	const fakeKey = "sk-proj-abcdefGHIJKL0123456789mnopqrstuvwxyz"
	const email = "punit@example.com"
	reqBody := fmt.Sprintf(`{"model":"claude-opus-5","messages":[{"role":"user","content":"my key is %s and my email is %s, is that safe to commit?"}]}`, fakeKey, email)

	var providerSaw string
	origin := newOriginServer(t, "api.anthropic.com", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ := io.ReadAll(r.Body)
		providerSaw = string(got)
		fmt.Fprintf(w, `{"model":"claude-opus-5","content":[{"type":"text","text":"No. Rotate %s immediately."}],"usage":{"input_tokens":20,"output_tokens":8}}`, fakeKey)
	}))
	defer origin.close()

	sink := &collector{}
	proxyAddr, _ := startProxy(t, Config{Issuer: root, Sink: sink, UpstreamRootCAs: origin.rootPool}, origin.addr)
	ourPool := x509.NewCertPool()
	ourPool.AddCert(root.Cert)

	resp, err := clientThrough(proxyAddr, ourPool).Post(
		"https://api.anthropic.com/v1/messages", "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	clientSaw, _ := io.ReadAll(resp.Body)

	// 1. The provider must have received the ORIGINAL request, key and all.
	if providerSaw != reqBody {
		t.Errorf("the provider did not receive the request unmodified:\n  got:  %s\n  want: %s", providerSaw, reqBody)
	}

	// 2. The client must have received the provider's ORIGINAL answer.
	if !strings.Contains(string(clientSaw), fakeKey) {
		t.Error("the client's answer was modified; only our copy may be redacted")
	}

	// 3. Our stored event must have neither the key nor the email.
	if !eventually(2*time.Second, func() bool { return len(sink.all()) == 1 }) {
		t.Fatalf("got %d events, want 1", len(sink.all()))
	}
	e := sink.all()[0]

	if strings.Contains(e.Prompt, fakeKey) {
		t.Errorf("the API key survived into the stored prompt: %q", e.Prompt)
	}
	if strings.Contains(e.Prompt, email) {
		t.Errorf("the email survived into the stored prompt: %q", e.Prompt)
	}
	if strings.Contains(e.Answer, fakeKey) {
		t.Errorf("the API key survived into the stored answer: %q", e.Answer)
	}

	// 4. The event records that something was masked, without the values.
	if len(e.Redacted) == 0 {
		t.Error("the event should list the rules that matched")
	}
	for _, name := range e.Redacted {
		if strings.Contains(name, "sk-") || strings.Contains(name, "@") {
			t.Errorf("a rule name leaked a value: %q", name)
		}
	}
	if e.RedactionRulesVersion == 0 {
		t.Error("the event should record which rule list was used")
	}

	// 5. The rest of the prompt must still be readable, or the record is useless.
	if !strings.Contains(e.Prompt, "is that safe to commit?") {
		t.Errorf("redaction destroyed the useful part of the prompt: %q", e.Prompt)
	}
}

// fakeProcesses stands in for lsof so the task-tagging path can be tested without
// depending on what happens to be running on the machine.
type fakeProcesses struct {
	name string
	dir  string
	err  error
}

func (f fakeProcesses) ByLocalPort(int) (platform.Process, error) {
	if f.err != nil {
		return platform.Process{}, f.err
	}
	return platform.Process{PID: 4242, Name: f.name}, nil
}

func (f fakeProcesses) WorkingDir(int) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.dir, nil
}

// TestCapturedEventCarriesTheTaskID is the Phase 5 milestone: an interaction
// started from a checkout on branch ABC-123 is stored against task ABC-123.
func TestCapturedEventCarriesTheTaskID(t *testing.T) {
	root := newTestRoot(t)

	// A real repository on a branch that names a ticket.
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test")
	os.WriteFile(filepath.Join(repo, "README.md"), []byte("hi\n"), 0o644)
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-qm", "first")
	runGit(t, repo, "checkout", "-qb", "feature/ABC-123-add-login")

	origin := newOriginServer(t, "api.anthropic.com", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body)
		fmt.Fprint(w, `{"model":"claude-opus-5","content":[{"type":"text","text":"done"}],"usage":{"input_tokens":5,"output_tokens":2}}`)
	}))
	defer origin.close()

	sink := &collector{}
	proxyAddr, _ := startProxy(t, Config{
		Issuer:          root,
		Sink:            sink,
		UpstreamRootCAs: origin.rootPool,
		Tasks:           tasks.NewResolver(),
		Processes:       fakeProcesses{name: "claude", dir: repo},
	}, origin.addr)

	ourPool := x509.NewCertPool()
	ourPool.AddCert(root.Cert)
	resp, err := clientThrough(proxyAddr, ourPool).Post("https://api.anthropic.com/v1/messages",
		"application/json", strings.NewReader(`{"model":"claude-opus-5","messages":[{"role":"user","content":"hello"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if !eventually(2*time.Second, func() bool { return len(sink.all()) == 1 }) {
		t.Fatalf("got %d events, want 1", len(sink.all()))
	}
	e := sink.all()[0]

	if e.TaskID != "ABC-123" {
		t.Errorf("task = %q, want ABC-123", e.TaskID)
	}
	if e.Branch != "feature/ABC-123-add-login" {
		t.Errorf("branch = %q", e.Branch)
	}
	if e.Repo != repo {
		t.Errorf("repo = %q, want %q", e.Repo, repo)
	}
	if e.Process != "claude" {
		t.Errorf("process = %q", e.Process)
	}
}

// Not knowing the task is normal — a browser, or a directory that is not a
// checkout — and must never break the capture.
func TestCaptureWorksWhenTheTaskIsUnknown(t *testing.T) {
	root := newTestRoot(t)
	origin := newOriginServer(t, "api.anthropic.com", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"model":"m","content":[{"type":"text","text":"ok"}]}`)
	}))
	defer origin.close()

	sink := &collector{}
	proxyAddr, _ := startProxy(t, Config{
		Issuer:          root,
		Sink:            sink,
		UpstreamRootCAs: origin.rootPool,
		Tasks:           tasks.NewResolver(),
		Processes:       fakeProcesses{err: fmt.Errorf("no process found")},
	}, origin.addr)

	ourPool := x509.NewCertPool()
	ourPool.AddCert(root.Cert)
	resp, err := clientThrough(proxyAddr, ourPool).Post("https://api.anthropic.com/v1/messages",
		"application/json", strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatalf("a failed process lookup must not break the request: %v", err)
	}
	resp.Body.Close()

	if !eventually(2*time.Second, func() bool { return len(sink.all()) == 1 }) {
		t.Fatal("the event should still be recorded, just untagged")
	}
	if got := sink.all()[0].TaskID; got != "" {
		t.Errorf("task = %q, want empty (the untagged bucket)", got)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

// TestCaptureThroughTheDeviceIntermediate is the D3 test at the proxy level: a real
// client, a real handshake, and a certificate minted by this device's short-lived,
// name-constrained intermediate rather than by the root.
//
// The client trusts only the ROOT. It has never seen the intermediate, so this also
// proves the chain we serve is complete enough for a client to build a path.
func TestCaptureThroughTheDeviceIntermediate(t *testing.T) {
	root := newTestRoot(t)
	origin := newOriginServer(t, "api.openai.com", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"a proxy sits in the middle"}}]}`)
	}))
	defer origin.close()

	key, err := ca.NewDeviceKey()
	if err != nil {
		t.Fatal(err)
	}
	cert, err := root.SignIntermediate(ca.IntermediateRequest{
		PublicKey:           &key.PublicKey,
		DeviceName:          "test-mac",
		PermittedDNSDomains: ca.PermittedDomainsFrom(AllowListEntries()),
	})
	if err != nil {
		t.Fatalf("sign intermediate: %v", err)
	}
	intermediate := &ca.Intermediate{Cert: cert, Key: key, RootDER: root.Cert.Raw}

	sink := &collector{}
	proxyAddr, _ := startProxy(t, Config{
		Issuer:          intermediate,
		Sink:            sink,
		UpstreamRootCAs: origin.rootPool,
	}, origin.addr)

	ourPool := x509.NewCertPool()
	ourPool.AddCert(root.Cert) // the root only: the intermediate must arrive in the chain

	// A real conversation, not a housekeeping call: only parsed conversations are
	// recorded, so /v1/models would produce no event and prove nothing here.
	resp, err := clientThrough(proxyAddr, ourPool).Post(
		"https://api.openai.com/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"gpt-4","messages":[{"role":"user","content":"what is a proxy"}]}`))
	if err != nil {
		t.Fatalf("a request for an allow-listed host must work: %v", err)
	}
	defer resp.Body.Close()

	// The certificate the client accepted was signed by the intermediate, not by
	// the root, and it carried the name constraints with it.
	served := resp.TLS.PeerCertificates
	if len(served) < 2 {
		t.Fatalf("the served chain has %d certificates, want at least leaf and intermediate", len(served))
	}
	if served[0].Issuer.CommonName != cert.Subject.CommonName {
		t.Errorf("the leaf was issued by %q, want the device intermediate %q",
			served[0].Issuer.CommonName, cert.Subject.CommonName)
	}
	if len(served[1].PermittedDNSDomains) == 0 {
		t.Error("the intermediate in the served chain carries no name constraints")
	}

	if !eventually(2*time.Second, func() bool { return len(sink.all()) == 1 }) {
		t.Fatalf("got %d events, want 1", len(sink.all()))
	}
}

// Research mode writes down what an unsupported endpoint actually sent, so a
// parser can be written for it. Two things must hold: the secrets are masked
// first, and a page full of JavaScript does not drown the one file that matters.
func TestResearchModeWritesRedactedExchanges(t *testing.T) {
	root := newTestRoot(t)
	origin := newOriginServer(t, "chatgpt.com", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"message":{"content":{"parts":["an answer"]}}}`)
	}))
	defer origin.close()

	dir := t.TempDir()
	proxyAddr, _ := startProxy(t, Config{
		Issuer:          root,
		Sink:            &collector{},
		UpstreamRootCAs: origin.rootPool,
		ResearchDir:     dir,
	}, origin.addr)

	ourPool := x509.NewCertPool()
	ourPool.AddCert(root.Cert)
	client := clientThrough(proxyAddr, ourPool)

	// A conversation, carrying something that must never reach the dump.
	resp, err := client.Post("https://chatgpt.com/backend-api/f/conversation", "application/json",
		strings.NewReader(`{"messages":[{"content":"my key is sk-ant-api03-SECRETVALUE1234567890"}]}`))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	// And a static asset, which is not research material.
	resp, err = client.Get("https://chatgpt.com/cdn/assets/abc-123.js")
	if err != nil {
		t.Fatalf("asset request failed: %v", err)
	}
	resp.Body.Close()

	var files []string
	if !eventually(2*time.Second, func() bool {
		entries, _ := os.ReadDir(dir)
		files = files[:0]
		for _, e := range entries {
			files = append(files, e.Name())
		}

		return len(files) >= 1
	}) {
		t.Fatalf("nothing was written to the research directory")
	}

	if len(files) != 1 {
		t.Errorf("got %d files, want 1 — the static asset must not be dumped: %v", len(files), files)
	}
	if !strings.Contains(files[0], "conversation") {
		t.Errorf("the file written was %q, want the conversation endpoint", files[0])
	}

	body, err := os.ReadFile(filepath.Join(dir, files[0]))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "SECRETVALUE1234567890") {
		t.Error("SECURITY: the research dump contains an unmasked secret")
	}
	// The shape a parser is written against has to survive the masking. The body
	// is a JSON string inside the dump, so its own quotes are escaped.
	if !strings.Contains(string(body), `\"messages\"`) {
		t.Errorf("the dump lost the request structure: %s", body)
	}
	if !strings.Contains(string(body), "REDACTED") {
		t.Errorf("the secret was neither masked nor present — what happened? %s", body)
	}
}

// Off unless asked for.
func TestResearchModeIsOffByDefault(t *testing.T) {
	root := newTestRoot(t)
	origin := newOriginServer(t, "api.openai.com", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer origin.close()

	dir := t.TempDir()
	proxyAddr, _ := startProxy(t, Config{
		Issuer: root, Sink: &collector{}, UpstreamRootCAs: origin.rootPool,
	}, origin.addr)

	ourPool := x509.NewCertPool()
	ourPool.AddCert(root.Cert)
	resp, err := clientThrough(proxyAddr, ourPool).Get("https://api.openai.com/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("research mode wrote %d files without being asked", len(entries))
	}
}
