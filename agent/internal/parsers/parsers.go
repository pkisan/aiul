// Package parsers turns one captured HTTP exchange into a readable record of what
// was asked and what was answered.
//
// Every provider has its own request and response shape, so there is one parser
// per provider behind a small interface. A parser never fails the request: if it
// cannot make sense of a body it returns what it could, and the event still
// carries the metadata.
package parsers

import (
	"net/http"
	"time"
)

// Exchange is everything a parser is given. The bodies are OUR copies, already
// decompressed and possibly truncated. Nothing a parser does affects what the
// client or the provider received.
type Exchange struct {
	Host     string
	Path     string
	Method   string
	Status   int
	ReqBody  []byte
	RespBody []byte
	ReqHead  http.Header
	RespHead http.Header

	// SSE holds the reassembled "data:" payloads when the response was streamed,
	// in order. Empty for a normal JSON response.
	SSE []string

	Started  time.Time
	Duration time.Duration
}

// Result is what a parser extracted.
type Result struct {
	// Tool is the client software, when the request reveals it (from User-Agent or
	// a provider-specific header) — "claude-code", "gemini-cli", and so on.
	Tool string

	// Model is the model name the request asked for.
	Model string

	// Prompt is the user's message. For a multi-turn conversation this is the last
	// user message, which is the new thing this request added.
	Prompt string

	// Answer is the model's reply, with a streamed response already reassembled
	// into one piece of text.
	Answer string

	// System is the system prompt, when one was sent. Useful for telling a human
	// prompt from an automated follow-up.
	System string

	PromptTokens   int
	ResponseTokens int

	// Streamed records whether the answer arrived as a stream.
	Streamed bool

	// Automated marks a request that is not a fresh human prompt. Kept in step
	// with Kind: anything but KindHuman is automated.
	Automated bool

	// Kind says who caused this request, which a flat "automated" flag could not.
	// One message typed by a person produces one KindHuman request, a dozen
	// KindAgent ones as the agent works through its tools, and a scattering of
	// KindUtility calls the tool makes for itself.
	Kind string
}

// What caused a request. Stored on the event, so the dashboard can show a
// person's turn and everything the agent did on their behalf underneath it.
const (
	// KindHuman: the newest message is text a person typed.
	KindHuman = "human"

	// KindAgent: the newest message is a tool result — the agent feeding itself
	// the output of the last thing it ran, continuing work already asked for.
	KindAgent = "agent"

	// KindUtility: the tool talking to a model about its own housekeeping —
	// grading a prompt, naming a conversation, suggesting a next action. No
	// conversation, no tools offered, and nobody waiting on the answer.
	KindUtility = "utility"
)

// Parser reads one provider's format.
type Parser interface {
	// Name identifies the parser in the stored event, so a strange record can
	// always be traced back to the code that produced it.
	Name() string

	// Handles reports whether this parser understands the exchange. It is given
	// the host and path only, so the decision is cheap.
	Handles(host, path string) bool

	// Parse extracts what it can. An error means "this body was not what I
	// expected"; the caller still records the event's metadata.
	Parse(ex Exchange) (Result, error)
}

// All returns every parser, in the order they should be tried.
func All() []Parser {
	return []Parser{
		OpenAI{},
		Anthropic{},
		Gemini{},
		CloudCode{},
		ChatGPTWeb{},
		ClaudeWeb{},
		Cursor{},
	}
}

// Names lists the parsers in this build, so `aiul parsers` can report what a
// given binary actually understands rather than what a document claims.
func Names() []string {
	list := All()

	out := make([]string, 0, len(list))
	for _, p := range list {
		out = append(out, p.Name())
	}

	return out
}

// For returns the first parser that handles this host and path, or nil.
func For(host, path string) Parser {
	for _, p := range All() {
		if p.Handles(host, path) {
			return p
		}
	}
	return nil
}
