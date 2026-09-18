package forward

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func quiet() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

type spooledEvent struct {
	ID     string `json:"id"`
	Prompt string `json:"prompt"`
}

// fakeBackend records what it received and accepts whatever the test tells it to.
type fakeBackend struct {
	*httptest.Server
	received  [][]string // event ids per request
	authSeen  []string
	status    int
	acceptAll bool
	accept    []string
}

func newFakeBackend(t *testing.T) *fakeBackend {
	t.Helper()
	b := &fakeBackend{status: http.StatusOK, acceptAll: true}
	b.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.authSeen = append(b.authSeen, r.Header.Get("Authorization"))

		var in batch
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		var ids []string
		for _, raw := range in.Events {
			var e spooledEvent
			json.Unmarshal(raw, &e)
			ids = append(ids, e.ID)
		}
		b.received = append(b.received, ids)

		if b.status != http.StatusOK {
			w.WriteHeader(b.status)
			w.Write([]byte("backend is unhappy"))
			return
		}
		accepted := b.accept
		if b.acceptAll {
			accepted = ids
		}
		json.NewEncoder(w).Encode(response{Accepted: accepted})
	}))
	t.Cleanup(b.Close)
	return b
}

func newTestForwarder(t *testing.T, endpoint string) (*Forwarder, *Spool) {
	t.Helper()
	s, err := NewSpool(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return NewForwarder(s, Config{Endpoint: endpoint, DeviceToken: "device-token-123", Logger: quiet()}), s
}

func TestForwarderSendsAndDeletesConfirmedEvents(t *testing.T) {
	backend := newFakeBackend(t)
	f, spool := newTestForwarder(t, backend.URL)

	spool.Record(spooledEvent{ID: "e1", Prompt: "first"})
	spool.Record(spooledEvent{ID: "e2", Prompt: "second"})

	sent, err := f.DrainOnce(context.Background())
	if err != nil {
		t.Fatalf("DrainOnce: %v", err)
	}
	if sent != 2 {
		t.Errorf("sent %d events, want 2", sent)
	}

	left, _ := spool.Pending()
	if len(left) != 0 {
		t.Errorf("%d files left in the spool after confirmation", len(left))
	}
	if len(backend.received) != 1 || len(backend.received[0]) != 2 {
		t.Errorf("backend received %v", backend.received)
	}
	if got := backend.authSeen[0]; got != "Bearer device-token-123" {
		t.Errorf("Authorization header = %q", got)
	}
}

// The rule that matters most: nothing is deleted unless the backend confirmed it.
func TestForwarderKeepsEventsTheBackendRejected(t *testing.T) {
	backend := newFakeBackend(t)
	f, spool := newTestForwarder(t, backend.URL)

	spool.Record(spooledEvent{ID: "keep-me"})
	spool.Record(spooledEvent{ID: "take-me"})

	backend.acceptAll = false
	backend.accept = []string{"take-me"}

	sent, err := f.DrainOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if sent != 1 {
		t.Errorf("sent = %d, want 1", sent)
	}

	left, _ := spool.Pending()
	if len(left) != 1 {
		t.Fatalf("%d files left, want 1", len(left))
	}
	data, _ := os.ReadFile(left[0])
	if !strings.Contains(string(data), "keep-me") {
		t.Errorf("the wrong file was kept: %s", data)
	}
}

func TestForwarderKeepsEverythingWhenTheBackendFails(t *testing.T) {
	backend := newFakeBackend(t)
	f, spool := newTestForwarder(t, backend.URL)

	spool.Record(spooledEvent{ID: "e1"})
	spool.Record(spooledEvent{ID: "e2"})
	backend.status = http.StatusInternalServerError

	if _, err := f.DrainOnce(context.Background()); err == nil {
		t.Error("a 500 from the backend should be reported as an error")
	}

	left, _ := spool.Pending()
	if len(left) != 2 {
		t.Errorf("%d files left, want 2 — nothing may be lost when the backend fails", len(left))
	}
}

func TestForwarderKeepsEverythingWhenTheNetworkIsDown(t *testing.T) {
	// A port nothing is listening on stands in for "no network".
	f, spool := newTestForwarder(t, "http://127.0.0.1:1/events")
	spool.Record(spooledEvent{ID: "e1"})

	if _, err := f.DrainOnce(context.Background()); err == nil {
		t.Error("an unreachable backend should be reported as an error")
	}
	left, _ := spool.Pending()
	if len(left) != 1 {
		t.Errorf("%d files left, want 1", len(left))
	}
}

func TestBackoffGrowsOnFailureAndResetsOnSuccess(t *testing.T) {
	backend := newFakeBackend(t)
	f, spool := newTestForwarder(t, backend.URL)
	spool.Record(spooledEvent{ID: "e1"})

	backend.status = http.StatusBadGateway
	first := f.Backoff()
	f.DrainOnce(context.Background())
	if f.Backoff() <= first {
		t.Errorf("backoff did not grow: %v -> %v", first, f.Backoff())
	}
	f.DrainOnce(context.Background())
	grown := f.Backoff()

	backend.status = http.StatusOK
	if _, err := f.DrainOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.Backoff() >= grown {
		t.Errorf("backoff did not reset after success: %v", f.Backoff())
	}
	if f.Backoff() != minBackoff {
		t.Errorf("backoff = %v, want the minimum %v", f.Backoff(), minBackoff)
	}
}

func TestBackoffIsCapped(t *testing.T) {
	f, _ := newTestForwarder(t, "http://127.0.0.1:1/events")
	for i := 0; i < 50; i++ {
		f.growBackoff()
	}
	if f.Backoff() != maxBackoff {
		t.Errorf("backoff = %v, want it capped at %v", f.Backoff(), maxBackoff)
	}
}

// A corrupt file must not block everything behind it for ever.
func TestUnreadableSpoolFileIsSetAsideNotRetriedForEver(t *testing.T) {
	backend := newFakeBackend(t)
	f, spool := newTestForwarder(t, backend.URL)

	spool.Record(spooledEvent{ID: "good"})
	bad := spool.Dir() + "/20260101T000000.000-9999.json"
	if err := os.WriteFile(bad, []byte("{ truncated"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := f.DrainOnce(context.Background()); err != nil {
		t.Fatalf("a corrupt file must not fail the whole drain: %v", err)
	}

	left, _ := spool.Pending()
	if len(left) != 0 {
		t.Errorf("%d files still pending: %v", len(left), left)
	}
	if _, err := os.Stat(bad + ".bad"); err != nil {
		t.Error("the corrupt file should have been set aside as .bad for a person to look at")
	}
}

// With no endpoint configured — the normal state until Phase 6 — events simply
// stay in the spool and nothing is attempted.
func TestForwarderIsANoOpWithoutAnEndpoint(t *testing.T) {
	f, spool := newTestForwarder(t, "")
	spool.Record(spooledEvent{ID: "e1"})

	if f.Enabled() {
		t.Error("a forwarder with no endpoint should report itself as disabled")
	}
	sent, err := f.DrainOnce(context.Background())
	if err != nil || sent != 0 {
		t.Errorf("sent=%d err=%v", sent, err)
	}
	if left, _ := spool.Pending(); len(left) != 1 {
		t.Error("the event should still be waiting in the spool")
	}
}

// Our own traffic must never go through our own proxy.
func TestForwarderIgnoresTheProxyEnvironment(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:9")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:9")

	backend := newFakeBackend(t)
	f, spool := newTestForwarder(t, backend.URL)
	spool.Record(spooledEvent{ID: "e1"})

	if _, err := f.DrainOnce(context.Background()); err != nil {
		t.Fatalf("the forwarder used the proxy from the environment: %v", err)
	}
}

func TestLargeSpoolIsSentInBatches(t *testing.T) {
	backend := newFakeBackend(t)
	f, spool := newTestForwarder(t, backend.URL)

	const total = BatchSize + 10
	for i := 0; i < total; i++ {
		spool.Record(spooledEvent{ID: string(rune('a'+i%26)) + string(rune('0'+i/26))})
	}

	sent, err := f.DrainOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if sent == 0 {
		t.Fatal("nothing was sent")
	}
	if len(backend.received) < 2 {
		t.Errorf("expected more than one batch, got %d", len(backend.received))
	}
	if len(backend.received[0]) != BatchSize {
		t.Errorf("first batch had %d events, want %d", len(backend.received[0]), BatchSize)
	}
}
