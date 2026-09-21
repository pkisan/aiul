package parsers

import (
	"encoding/json"
	"strings"
)

// ClaudeWeb parses a conversation typed into claude.ai in a browser.
//
// Captured on 2026-09-21. The happy surprise is that the RESPONSE is the same
// stream the Messages API sends — `message_start`, `content_block_delta` with a
// `text_delta`, `message_delta` with the usage — so reassembly is the Anthropic
// parser's, reused rather than rewritten.
//
// The request is the part that differs. The API takes `messages[]`; the web
// application sends one `prompt` string for the new turn, because the rest of the
// conversation is already on the server:
//
//	{"prompt":"Hi how are you?","model":"claude-sonnet-5","effort":"medium",
//	 "thinking_mode":"auto","tools":[ ...MCP tools... ]}
type ClaudeWeb struct{}

func (ClaudeWeb) Name() string { return "claude-web" }

func (ClaudeWeb) Handles(host, path string) bool {
	if strings.ToLower(host) != "claude.ai" {
		return false
	}

	// Only the endpoint that carries a turn of conversation:
	//   /api/organizations/<org>/chat_conversations/<conversation>/completion
	//
	// Its neighbours — the conversation list, the title, composer notices,
	// event_logging batches — are not somebody talking to the model. Matching on
	// the ending keeps this robust to the two identifiers in the middle without
	// matching anything else.
	path, _, _ = strings.Cut(path, "?")

	return strings.HasPrefix(path, "/api/organizations/") &&
		strings.Contains(path, "/chat_conversations/") &&
		strings.HasSuffix(strings.TrimSuffix(path, "/"), "/completion")
}

func (p ClaudeWeb) Parse(ex Exchange) (Result, error) {
	var res Result
	res.Tool = "claude-web"
	res.Streamed = true // the browser client always streams

	var req struct {
		Prompt string `json:"prompt"`
		Model  string `json:"model"`

		// Present when the person attached a file or the assistant is continuing
		// with a tool result rather than the person typing.
		Attachments []json.RawMessage `json:"attachments"`
		Files       []json.RawMessage `json:"files"`
		ToolResults []json.RawMessage `json:"tool_results"`
	}
	if err := json.Unmarshal(ex.ReqBody, &req); err != nil {
		return res, err
	}

	res.Prompt = req.Prompt
	res.Model = req.Model

	// A turn carrying tool results is the assistant continuing its own work, the
	// same distinction the API parsers make. Counting those as prompts would make
	// one question look like several.
	res.Automated = len(req.ToolResults) > 0 && req.Prompt == ""

	// The response stream is the Messages API's, so the Anthropic parser already
	// knows how to read it.
	res.Answer, res.PromptTokens, res.ResponseTokens = Anthropic{}.reassemble(ex.SSE)

	// The stream names the model that actually answered, which is worth more than
	// what the client asked for.
	if model := modelInStream(ex.SSE); model != "" {
		res.Model = model
	}

	return res, nil
}

// modelInStream reads the model out of the message_start event.
func modelInStream(events []string) string {
	for _, data := range events {
		if !strings.Contains(data, `"message_start"`) {
			continue
		}

		var ev struct {
			Message struct {
				Model string `json:"model"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			continue
		}
		if ev.Message.Model != "" {
			return ev.Message.Model
		}
	}

	return ""
}
