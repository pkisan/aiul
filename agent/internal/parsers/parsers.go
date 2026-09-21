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

	// Automated marks a request the format shows is not a fresh human prompt —
	// a tool-result follow-up, for instance, where the agent is continuing on its
	// own. Only set when the format makes it visible.
	Automated bool
}

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
		ChatGPTWeb{},
	}
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
