package forward

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"
)

// The forwarder reads spooled events and sends them to the backend in batches.
//
// Two rules shape it:
//
//   - A spooled file is deleted only after the backend has confirmed it. If the
//     network is down, the laptop sleeps, or the backend is being deployed, the
//     events simply wait on disk.
//   - Its own connection bypasses the proxy. The agent must be able to report even
//     when the proxy is broken — and sending our events through our own proxy
//     would be a loop.

const (
	// BatchSize is how many events go in one request. Small enough that a failure
	// costs little, large enough that a busy machine is not chatty.
	BatchSize = 50

	minBackoff = 5 * time.Second
	maxBackoff = 15 * time.Minute
)

// Forwarder sends spooled events to the backend.
type Forwarder struct {
	spool *Spool
	cfg   Config
	log   *slog.Logger

	client  *http.Client
	backoff time.Duration
}

// Config is what the forwarder needs to reach the backend.
type Config struct {
	// Endpoint is the ingestion URL, e.g. https://pm.example.com/api/aiul/events.
	// Empty means "do not forward": events stay in the spool, which is the normal
	// state until Phase 6 exists.
	Endpoint string

	// DeviceToken authenticates this device. It comes from the keychain, never
	// from a file in the repo.
	DeviceToken string

	// Timeout bounds one request.
	Timeout time.Duration

	Logger *slog.Logger
}

// batch is what we POST.
type batch struct {
	DeviceID string            `json:"device_id"`
	SentAt   time.Time         `json:"sent_at"`
	Events   []json.RawMessage `json:"events"`
}

// response is what the backend replies. Accepted lists the event ids it stored;
// only those files are deleted.
type response struct {
	Accepted []string `json:"accepted"`
}

// NewForwarder builds a forwarder over a spool.
func NewForwarder(spool *Spool, cfg Config) *Forwarder {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	return &Forwarder{
		spool: spool,
		cfg:   cfg,
		log:   cfg.Logger,
		client: &http.Client{
			Timeout: cfg.Timeout,
			Transport: &http.Transport{
				// Proxy: nil means "ignore HTTPS_PROXY". Our own traffic must not
				// go through our own proxy, and must keep working when it is down.
				Proxy:       nil,
				DialContext: (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
			},
		},
		backoff: minBackoff,
	}
}

// Enabled reports whether there is anywhere to forward to.
func (f *Forwarder) Enabled() bool { return f.cfg.Endpoint != "" }

// Backoff is the current wait after a failure, for `aiul status`.
func (f *Forwarder) Backoff() time.Duration { return f.backoff }

// DrainOnce sends as many batches as are waiting, and returns how many events the
// backend accepted. A failure stops the drain and grows the backoff.
func (f *Forwarder) DrainOnce(ctx context.Context) (int, error) {
	if !f.Enabled() {
		return 0, nil
	}

	sent := 0
	for {
		files, err := f.spool.Pending()
		if err != nil {
			return sent, fmt.Errorf("read the spool: %w", err)
		}
		if len(files) == 0 {
			return sent, nil
		}
		if len(files) > BatchSize {
			files = files[:BatchSize]
		}

		accepted, err := f.sendBatch(ctx, files)
		if err != nil {
			f.growBackoff()
			return sent, err
		}
		f.backoff = minBackoff
		sent += accepted

		// Fewer files than a full batch means the spool is drained.
		if len(files) < BatchSize {
			return sent, nil
		}
	}
}

// sendBatch posts one batch and deletes only what the backend confirmed.
func (f *Forwarder) sendBatch(ctx context.Context, files []string) (int, error) {
	b := batch{DeviceID: deviceID(), SentAt: time.Now().UTC()}
	byID := map[string]string{} // event id -> file path

	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			continue // a file that vanished is not a failure
		}
		var probe struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(data, &probe); err != nil || probe.ID == "" {
			// An unreadable spool file would block the queue for ever. Move it
			// aside so a person can look at it and the rest keeps flowing.
			f.log.Warn("spooled event is not valid JSON; setting it aside", "file", path)
			_ = os.Rename(path, path+".bad")
			continue
		}
		byID[probe.ID] = path
		b.Events = append(b.Events, json.RawMessage(data))
	}

	if len(b.Events) == 0 {
		return 0, nil
	}

	body, err := json.Marshal(b)
	if err != nil {
		return 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+f.cfg.DeviceToken)

	resp, err := f.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("send to the backend: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Read a little of the body for the log, then discard the rest.
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return 0, fmt.Errorf("the backend returned %d: %s", resp.StatusCode, bytes.TrimSpace(snippet))
	}

	var r response
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&r); err != nil {
		// The backend took them but we cannot read its reply. Keep the files: a
		// duplicate is recoverable, a lost prompt is not.
		return 0, fmt.Errorf("read the backend's reply: %w", err)
	}

	deleted := 0
	for _, id := range r.Accepted {
		path, ok := byID[id]
		if !ok {
			continue
		}
		if err := os.Remove(path); err == nil {
			deleted++
		}
	}
	return deleted, nil
}

func (f *Forwarder) growBackoff() {
	f.backoff *= 2
	if f.backoff > maxBackoff {
		f.backoff = maxBackoff
	}
}

// deviceID identifies this machine in a batch. The hostname is enough for now;
// Phase 6 replaces it with the identity the device token was issued against.
func deviceID() string {
	name, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return name
}
