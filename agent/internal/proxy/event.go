package proxy

import (
	"net/http"
	"time"
)

// interaction is what the proxy observed on one request/response pair, before any
// parsing or redaction. It is deliberately raw: parsers turn it into an Event, and
// redaction runs before anything is stored. Rule 8.
type interaction struct {
	Host   string
	Method string
	Path   string
	Status int

	RequestBytes  int64
	ResponseBytes int64

	// Bounded copies taken on the side while streaming. Never the authority for
	// what was sent — the client and provider always got the originals.
	RequestCopy  []byte
	ResponseCopy []byte

	RequestHeader http.Header
	ResponseHead  http.Header

	Started  time.Time
	Duration time.Duration
}

// Sink receives one finished interaction. internal/forward implements it by
// writing a JSON event to the spool; tests implement it by collecting in memory.
// A nil sink means "drop everything", which is what the pass-through paths want.
type Sink interface {
	Record(Event)
}

// Event is the structured record of one AI interaction. Parsers fill in the model,
// prompt and response; the proxy fills in everything observable from the wire.
type Event struct {
	Schema   int       `json:"schema"`
	ID       string    `json:"id"`
	Time     time.Time `json:"time"`
	Host     string    `json:"host"`
	Method   string    `json:"method"`
	Path     string    `json:"path"`
	Status   int       `json:"status"`
	Tool     string    `json:"tool,omitempty"`
	Model    string    `json:"model,omitempty"`
	Parser   string    `json:"parser,omitempty"`
	Prompt   string    `json:"prompt,omitempty"`
	Answer   string    `json:"answer,omitempty"`
	Streamed bool      `json:"streamed,omitempty"`

	PromptTokens   int `json:"prompt_tokens,omitempty"`
	ResponseTokens int `json:"response_tokens,omitempty"`

	RequestBytes  int64 `json:"request_bytes"`
	ResponseBytes int64 `json:"response_bytes"`

	DurationMS int64 `json:"duration_ms"`

	// AllowListVersion records which version of the allow-list decided to capture
	// this, so an event can always be explained later.
	AllowListVersion int `json:"allowlist_version"`
}

// record turns a raw interaction into an Event and hands it to the sink.
// Parsing and redaction are added in the next steps of Phase 2 and in Phase 3;
// for now it captures the metadata every event needs.
func (p *Proxy) record(in interaction) {
	if p.cfg.Sink == nil {
		return
	}
	p.cfg.Sink.Record(Event{
		Schema:           1,
		Time:             in.Started,
		Host:             in.Host,
		Method:           in.Method,
		Path:             in.Path,
		Status:           in.Status,
		RequestBytes:     in.RequestBytes,
		ResponseBytes:    in.ResponseBytes,
		DurationMS:       in.Duration.Milliseconds(),
		AllowListVersion: AllowListVersion,
	})
}
