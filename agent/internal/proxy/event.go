package proxy

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pkisan/aiul/internal/parsers"
	"github.com/pkisan/aiul/internal/platform"
	"github.com/pkisan/aiul/internal/redact"
	"github.com/pkisan/aiul/internal/tasks"
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

	// How the client framed the request body. A chunked request is read
	// differently from one with a Content-Length, and telling them apart matters
	// when a body never reaches our copy.
	RequestTransferEncoding []string

	Started  time.Time
	Duration time.Duration

	// Task is what we worked out about where this interaction came from: the
	// process, its working directory, the branch and the task ID.
	Task tasks.Info
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

	// Where the work was happening. TaskID empty means the backend's "untagged"
	// bucket: better than attaching the event to a task we guessed at.
	TaskID  string `json:"task_id,omitempty"`
	Branch  string `json:"branch,omitempty"`
	Repo    string `json:"repo,omitempty"`
	WorkDir string `json:"work_dir,omitempty"`
	Process string `json:"process,omitempty"`
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

		TaskID:  in.Task.TaskID,
		Branch:  in.Task.Branch,
		Repo:    in.Task.Repo,
		WorkDir: in.Task.Dir,
		Process: in.Task.Process,
	}

	parser := parsers.For(in.Host, in.Path)

	// Research mode, off unless asked for: write down what an endpoint we cannot
	// parse actually sent, so a parser can be written against it.
	p.dumpForResearch(in, parser)

	if parser == nil {
		// An allow-listed host makes plenty of calls that are not conversations:
		// registry lookups, account settings, telemetry batches, and — for a web
		// application — hundreds of scripts and images. One Claude Code run
		// produced twelve of them and a single real exchange. Storing them would
		// mean holding data about the user for no benefit, so they are decrypted,
		// forwarded and forgotten.
		//
		// Static assets are not even worth a log line: a single page load buries
		// everything else under a hundred of them.
		if interesting(in.Path, in.ResponseHead.Get("Content-Type")) {
			p.log.Debug("no parser for this endpoint; nothing recorded",
				"host", in.Host, "method", in.Method, "path", in.Path, "status", in.Status)
		}

		return
	}

	// A copy that stopped exactly at the cap is a body we did not see all of.
	// Without saying so, a prompt parsed from a truncated request is
	// indistinguishable from a complete one, and an empty one looks like a tool
	// that sent nothing.
	truncated := len(in.RequestCopy) >= maxRequestCopyBytes

	var ex parsers.Exchange
	{
		ev.Parser = parser.Name()
		ex = p.exchange(in)
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

	// What the parser actually made of the exchange, and what it was given.
	//
	// An answer of zero has two very different causes and no other line separates
	// them: a stream that carried no prose (a turn that only calls a tool sends
	// input_json_delta, never text_delta) and a stream whose shape this parser does
	// not know (the Codex Responses API). The event types say which. The request
	// side is here for the other failure — a prompt of zero with the response
	// copied in full, which means the body never reached the tee.
	p.log.Debug("recorded", "host", in.Host, "path", in.Path, "status", in.Status,
		"parser", ev.Parser, "model", ev.Model, "streamed", ev.Streamed,
		"prompt_chars", len(ev.Prompt), "answer_chars", len(ev.Answer),
		"request_bytes", in.RequestBytes, "request_copy_bytes", len(in.RequestCopy),
		"request_encoding", in.RequestHeader.Get("Content-Encoding"),
		"transfer_encoding", strings.Join(in.RequestTransferEncoding, ","),
		"response_bytes", in.ResponseBytes, "response_copy_bytes", len(in.ResponseCopy),
		"content_type", in.ResponseHead.Get("Content-Type"),
		"sse_events", len(ex.SSE), "sse_types", sseTypes(ex.SSE),
		"body_shape", bodyShape(in.ResponseCopy))

	if truncated {
		p.log.Warn("the request body was larger than we copy, so the prompt may be incomplete",
			"host", in.Host, "path", in.Path, "request_bytes", in.RequestBytes,
			"copied_bytes", len(in.RequestCopy), "prompt_chars", len(ev.Prompt))
		ev.Prompt = strings.TrimRight(ev.Prompt, "\n") + truncationNote
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

// dumpForResearch writes an exchange to the research directory, when one is
// configured. Bodies are REDACTED first: masking replaces values and leaves the
// structure a parser is written against intact.
func (p *Proxy) dumpForResearch(in interaction, parser parsers.Parser) {
	if p.research == nil || !interesting(in.Path, in.ResponseHead.Get("Content-Type")) {
		return
	}

	ex := p.exchange(in)

	reqBody, respBody := string(ex.ReqBody), string(ex.RespBody)
	masked, _ := p.redactor.Strings(reqBody, respBody)
	reqBody, respBody = masked[0], masked[1]

	events := make([]string, 0, len(ex.SSE))
	for _, e := range ex.SSE {
		one, _ := p.redactor.Strings(e)
		events = append(events, one[0])
	}

	dumped := dumpedExchange{
		Time:     in.Started,
		Host:     in.Host,
		Method:   in.Method,
		Path:     in.Path,
		Status:   in.Status,
		ReqHead:  headerMap(in.RequestHeader),
		ReqBody:  reqBody,
		RespHead: headerMap(in.ResponseHead),
		RespBody: respBody,
		SSE:      events,
	}
	if parser != nil {
		dumped.Parser = parser.Name()
	}

	if err := p.research.write(dumped); err != nil {
		p.log.Warn("could not write the research dump", "err", err)
	}
}

// headerMap keeps the headers a parser might key on and drops the rest, so a dump
// carries no cookie and no authorization header.
func headerMap(h http.Header) map[string]string {
	out := map[string]string{}
	for _, name := range []string{
		"Content-Type", "Content-Encoding", "Accept", "User-Agent",
		"X-Stainless-Package-Version", "X-App", "Anthropic-Version", "Openai-Beta",
	} {
		if v := h.Get(name); v != "" {
			out[name] = v
		}
	}

	return out
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
		if isSSE(in.ResponseHead.Get("Content-Type")) || looksLikeSSE(respBody) {
			for _, e := range parseSSE(respBody) {
				ex.SSE = append(ex.SSE, e.Data)
			}
		} else {
			ex.RespBody = respBody
		}
	}
	return ex
}

// truncationNote is appended to a prompt parsed from a request we only partly
// copied. It goes in the stored text, where anyone reading the prompt sees it —
// a log line they will never look at is not honest enough.
const truncationNote = "\n\n[aiul: the request was larger than the 64 MiB we copy, so this prompt is incomplete]"

// looksLikeSSE decides by the bytes when the header does not say.
//
// Codex answers /backend-api/codex/responses with event-stream framing and NO
// Content-Type at all, so a header check alone sent 207 KB of answer to the JSON
// path, where it parsed as nothing. Framing is a property of the body, and the
// body is right here.
func looksLikeSSE(body []byte) bool {
	head := bytes.TrimLeft(body, " \r\n\t")
	return bytes.HasPrefix(head, []byte("event:")) || bytes.HasPrefix(head, []byte("data:"))
}

// bodyShape describes how a body is FRAMED, never what it says: the first few
// bytes as hex, and how many "data:" lines it contains. Codex answers 207 KB with
// no Content-Type at all, so isSSE() says no and the bytes go nowhere — and
// nothing in the log says whether they are event-stream lines, JSON, or
// something else entirely.
func bodyShape(body []byte) string {
	if len(body) == 0 {
		return "empty"
	}

	head := body
	if len(head) > 8 {
		head = head[:8]
	}

	return fmt.Sprintf("first=%x data_lines=%d json_start=%t",
		head, bytes.Count(body, []byte("\ndata:"))+boolToInt(bytes.HasPrefix(body, []byte("data:"))),
		bytes.HasPrefix(bytes.TrimLeft(body, " \r\n\t"), []byte("{")) ||
			bytes.HasPrefix(bytes.TrimLeft(body, " \r\n\t"), []byte("[")))
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// sseTypes summarises what a streamed response actually contained: the distinct
// event and delta types, in the order first seen, with how many of each. Types
// only — never the text they carry.
func sseTypes(events []string) string {
	order := make([]string, 0, 8)
	count := make(map[string]int, 8)

	for _, data := range events {
		var ev struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
			} `json:"delta"`
		}
		if json.Unmarshal([]byte(data), &ev) != nil {
			continue
		}
		name := ev.Type
		if name == "" {
			name = "(no type)"
		}
		if ev.Delta.Type != "" {
			name += ":" + ev.Delta.Type
		}
		if count[name] == 0 {
			order = append(order, name)
		}
		count[name]++
	}

	parts := make([]string, 0, len(order))
	for _, name := range order {
		parts = append(parts, fmt.Sprintf("%s×%d", name, count[name]))
	}
	return strings.Join(parts, " ")
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

// processOf finds the program that opened this connection, from the port it came
// from. The second result says whether the lookup worked; not knowing is normal
// (the finder is nil in most tests, and lsof can lose a race with a short
// connection).
func (p *Proxy) processOf(clientConn net.Conn) (platform.Process, bool) {
	if p.cfg.Processes == nil {
		return platform.Process{}, false
	}
	addr, ok := clientConn.RemoteAddr().(*net.TCPAddr)
	if !ok {
		return platform.Process{}, false
	}
	proc, err := p.cfg.Processes.ByLocalPort(addr.Port)
	if err != nil {
		p.log.Debug("could not identify the client process", "port", addr.Port, "err", err)
		return platform.Process{}, false
	}
	return proc, true
}

// contextOf works out which task a connection belongs to, from the program that
// opened it. Everything here is best-effort: not knowing is normal and never an
// error, because plenty of AI traffic comes from a browser or a directory that is
// not a checkout.
func (p *Proxy) contextOf(proc platform.Process, found bool) tasks.Info {
	if !found || p.cfg.Tasks == nil || p.cfg.Processes == nil {
		return tasks.Info{}
	}

	dir, err := p.cfg.Processes.WorkingDir(proc.PID)
	if err != nil {
		return tasks.Info{Process: proc.Name}
	}

	read := p.cfg.Checkout
	if read == nil {
		read = tasks.CheckoutAt
	}

	info := p.cfg.Tasks.ResolveWith(dir, read)
	info.Process = proc.Name

	if info.TaskID == "" {
		p.log.Debug("no task for this connection",
			"process", proc.Name, "dir", dir, "repo", info.Repo, "branch", info.Branch)
	}

	return info
}
