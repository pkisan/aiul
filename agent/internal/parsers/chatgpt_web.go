package parsers

import (
	"encoding/json"
	"strings"
)

// ChatGPTWeb parses a conversation typed into chatgpt.com in a browser.
//
// This is not the OpenAI API. The web application has its own private endpoint
// and its own streaming protocol, neither documented nor stable, so this parser
// was written against real traffic captured on 2026-09-21 and is pinned by a
// fixture built from it.
//
// The request is ordinary JSON: messages[].content.parts[] with an author role.
// The RESPONSE is the interesting half — a stream of JSON-patch style operations
// against a message being built up on the client:
//
//	{"p":"/message/content/parts/0","o":"append","v":"Hi!"}   <- sets the target
//	{"v":" I'm doing well, thanks"}                            <- continues it
//	{"p":"","o":"patch","v":[{"p":"/message/content/parts/0","o":"append","v":"..."}]}
//
// So the answer is assembled by following appends to a content part, including the
// bare {"v":"..."} continuations that inherit the previous target, and the batched
// ops inside a patch. Everything else in the stream — system messages, markers,
// title generation, server metadata — is skipped.
//
// If OpenAI changes this protocol the fixture test fails, which is the point: a
// parser that silently records nothing is worse than an honest gap.
type ChatGPTWeb struct{}

func (ChatGPTWeb) Name() string { return "chatgpt-web" }

func (ChatGPTWeb) Handles(host, path string) bool {
	switch strings.ToLower(host) {
	case "chatgpt.com", "chat.openai.com":
	default:
		return false
	}

	// Only the endpoint that carries a conversation. The web application calls a
	// dozen neighbours with similar names — /prepare, /conversations (the sidebar
	// list), /conversation/experimental/generate_autocompletions — and none of them
	// is somebody talking to the model.
	path, _, _ = strings.Cut(path, "?")
	path = strings.TrimSuffix(path, "/")

	return path == "/backend-api/f/conversation" || path == "/backend-api/conversation"
}

type chatGPTWebRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Author struct {
			Role string `json:"role"`
		} `json:"author"`
		Content struct {
			ContentType string   `json:"content_type"`
			Parts       []string `json:"parts"`
		} `json:"content"`
	} `json:"messages"`
}

func (p ChatGPTWeb) Parse(ex Exchange) (Result, error) {
	var res Result
	res.Tool = "chatgpt-web"
	// A browser conversation is always streamed, and there is no usage block:
	// the web application does not report token counts to itself.
	res.Streamed = true

	var req chatGPTWebRequest
	if err := json.Unmarshal(ex.ReqBody, &req); err != nil {
		return res, err
	}
	res.Model = req.Model

	for _, m := range req.Messages {
		if m.Author.Role != "user" {
			continue
		}
		// The newest user message is what this request asked.
		res.Prompt = strings.Join(m.Content.Parts, "\n")
	}

	answer, model := p.reassemble(ex.SSE)
	res.Answer = answer
	// The request usually says "auto"; the stream says which model actually ran.
	if model != "" {
		res.Model = model
	}

	return res, nil
}

// patchOp is one operation in the stream. Every field is optional, which is why
// they are all pointers or raw.
type patchOp struct {
	P *string         `json:"p"`
	O string          `json:"o"`
	V json.RawMessage `json:"v"`

	// Events that carry a type are markers, titles and metadata, never content.
	Type string `json:"type"`
}

// reassemble follows the appends and returns the answer, plus the model the
// stream named if it mentioned one.
func (p ChatGPTWeb) reassemble(events []string) (answer, model string) {
	var b strings.Builder

	// Whether the last targeted append was to a content part. A bare {"v":"..."}
	// continues wherever the previous one pointed, so continuations must only be
	// taken while that target is text the user can see.
	appendingText := false

	for _, raw := range events {
		if raw == "" || raw == `"v1"` || raw == "[DONE]" {
			continue
		}

		var op patchOp
		if err := json.Unmarshal([]byte(raw), &op); err != nil {
			continue // one unreadable event must not lose the others
		}
		if op.Type != "" {
			// A marker or title. Not content, but the metadata event names the
			// model that actually answered.
			if m := modelSlugIn(raw); m != "" {
				model = m
			}

			continue
		}

		switch op.O {
		case "append":
			if op.P != nil && isContentPart(*op.P) {
				b.WriteString(stringValue(op.V))
				appendingText = true
			} else if op.P != nil {
				appendingText = false
			}

		case "patch":
			// A batch of operations, each with its own path.
			var ops []patchOp
			if err := json.Unmarshal(op.V, &ops); err != nil {
				continue
			}
			for _, inner := range ops {
				if inner.O == "append" && inner.P != nil && isContentPart(*inner.P) {
					b.WriteString(stringValue(inner.V))
					appendingText = true
				}
			}

		case "add", "replace", "remove":
			// Messages being created or their status changing. The model the
			// stream chose appears here on the assistant's own message.
			if m := modelSlugIn(raw); m != "" {
				model = m
			}
			appendingText = false

		case "":
			// A continuation of the previous append: {"v":" more text"}.
			if op.P == nil && appendingText {
				b.WriteString(stringValue(op.V))
			}
		}
	}

	return b.String(), model
}

// isContentPart reports whether a patch path points at text the person reads.
func isContentPart(path string) bool {
	return strings.Contains(path, "/content/parts/")
}

// stringValue decodes a JSON value that should be a string, returning "" for
// anything else rather than failing the whole reassembly.
func stringValue(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}

	return s
}

// modelSlugIn pulls the model out of an event that mentions one. The field turns
// up in several shapes of event, so it is read from the raw text rather than by
// decoding each shape.
func modelSlugIn(raw string) string {
	const key = `"model_slug":"`

	i := strings.Index(raw, key)
	if i < 0 {
		return ""
	}
	rest := raw[i+len(key):]

	end := strings.IndexByte(rest, '"')
	if end < 0 {
		return ""
	}
	slug := rest[:end]

	// "auto" is what the client asked for, not what answered.
	if slug == "auto" || slug == "" {
		return ""
	}

	return slug
}
