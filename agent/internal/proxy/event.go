package proxy

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"time"

	"github.com/pkisan/aiul/internal/parsers"
	"github.com/pkisan/aiul/internal/redact"
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
	System   string    `json:"system,omitempty"`
	Answer   string    `json:"answer,omitempty"`
	Streamed bool      `json:"streamed,omitempty"`

	// Automated marks a request the wire format shows is an agent's own follow-up
	// (a tool result, say) rather than something a person typed.
	Automated bool `json:"automated,omitempty"`

	PromptTokens   int `json:"prompt_tokens,omitempty"`
	ResponseTokens int `json:"response_tokens,omitempty"`

	RequestBytes  int64 `json:"request_bytes"`
	ResponseBytes int64 `json:"response_bytes"`

	DurationMS int64 `json:"duration_ms"`

	// AllowListVersion records which version of the allow-list decided to capture
	// this, so an event can always be explained later.
	AllowListVersion int `json:"allowlist_version"`

	// Redacted names the redaction rules that matched, so a stored record shows
	// that a secret was present without keeping it. Never holds a value.
	Redacted []string `json:"redacted,omitempty"`

	// RedactionRulesVersion records which rule list produced this record.
	RedactionRulesVersion int `json:"redaction_rules_version"`
}

// record turns a raw interaction into an Event and hands it to the sink.
//
// Redaction (Phase 3) will run on the Event before it leaves this function. The
// bodies here are OUR copies; the client and provider already have the originals.
func (p *Proxy) record(in interaction) {
	if p.cfg.Sink == nil {
		return
	}

	ev := Event{
		Schema:           1,
		ID:               newEventID(),
		Time:             in.Started,
		Host:             in.Host,
		Method:           in.Method,
		Path:             in.Path,
		Status:           in.Status,
		RequestBytes:     in.RequestBytes,
		ResponseBytes:    in.ResponseBytes,
		DurationMS:       in.Duration.Milliseconds(),
		AllowListVersion: AllowListVersion,
	}

	parser := parsers.For(in.Host, in.Path)
	if parser == nil {
		// An allow-listed host makes plenty of calls that are not conversations:
		// registry lookups, account settings, telemetry batches. One Claude Code
		// run produced twelve of them and a single real exchange. Storing them
		// would mean holding data about the user for no benefit, so they are
		// decrypted, forwarded and forgotten.
		p.log.Debug("no parser for this endpoint; nothing recorded",
			"host", in.Host, "method", in.Method, "path", in.Path, "status", in.Status)
		return
	}

	{
		ev.Parser = parser.Name()
		ex := p.exchange(in)
		res, err := parser.Parse(ex)
		if err != nil {
			// A body we could not read is still worth recording as metadata.
			p.log.Debug("parse failed", "host", in.Host, "parser", parser.Name(), "err", err)
		}
		ev.Tool = res.Tool
		ev.Model = res.Model
		ev.Prompt = res.Prompt
		ev.System = res.System
		ev.Answer = res.Answer
		ev.Streamed = res.Streamed
		ev.Automated = res.Automated
		ev.PromptTokens = res.PromptTokens
		ev.ResponseTokens = res.ResponseTokens
	}

	// Rule 8: mask before anything is stored or sent. The provider already has the
	// original request; this only touches our copy.
	texts, found := p.redactor.Strings(ev.Prompt, ev.System, ev.Answer)
	ev.Prompt, ev.System, ev.Answer = texts[0], texts[1], texts[2]
	ev.Redacted = found.Names()
	ev.RedactionRulesVersion = found.RulesVersion
	if found.Any() {
		// The names of the rules, never the values.
		p.log.Info("masked secrets before storing", "host", in.Host, "found", redact.Describe(found))
	}

	p.cfg.Sink.Record(ev)
}

// exchange prepares our copies for a parser: decompress, and split a streamed
// response into its SSE payloads.
func (p *Proxy) exchange(in interaction) parsers.Exchange {
	reqBody, reqOK := decompress(in.RequestCopy, in.RequestHeader.Get("Content-Encoding"))
	respBody, respOK := decompress(in.ResponseCopy, in.ResponseHead.Get("Content-Encoding"))

	if !reqOK || !respOK {
		// br and zstd are not decoded yet; say so once rather than storing rubbish.
		p.log.Debug("body was compressed in a format we do not decode yet",
			"host", in.Host,
			"request_encoding", in.RequestHeader.Get("Content-Encoding"),
			"response_encoding", in.ResponseHead.Get("Content-Encoding"))
	}

	ex := parsers.Exchange{
		Host:     in.Host,
		Path:     in.Path,
		Method:   in.Method,
		Status:   in.Status,
		ReqHead:  in.RequestHeader,
		RespHead: in.ResponseHead,
		Started:  in.Started,
		Duration: in.Duration,
	}
	if reqOK {
		ex.ReqBody = reqBody
	}
	if respOK {
		if isSSE(in.ResponseHead.Get("Content-Type")) {
			for _, e := range parseSSE(respBody) {
				ex.SSE = append(ex.SSE, e.Data)
			}
		} else {
			ex.RespBody = respBody
		}
	}
	return ex
}

// newEventID returns a random identifier for one event, so the backend can
// deduplicate a batch that was sent twice after a retry.
func newEventID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b[:])
}
