package parsers

import (
	"encoding/json"
	"strings"
)

// OpenAI parses the Chat Completions and Responses APIs — at api.openai.com and at
// the many providers that copied that wire format exactly.
//
// Reusing this parser for them is not laziness about correctness: they accept the
// same request fields and return the same `choices[].message.content`,
// `choices[].delta.content` and `usage` shapes, because being a drop-in for
// OpenAI's client libraries is the point of their APIs. Anything they add on top is
// ignored, the same way an unknown OpenAI field is.
//
// Each of these hosts is on the proxy allow-list, so until now their traffic was
// decrypted and then dropped with "no parser for this endpoint".
//
// NOT here, because their formats genuinely differ and need real captures first:
// api.cohere.com (a single `message` field, not `messages`), and the web
// applications chatgpt.com, claude.ai, gemini.google.com and aistudio.google.com,
// whose internal endpoints are private and change without notice.
type OpenAI struct{}

func (OpenAI) Name() string { return "openai" }

// openAICompatibleHosts are the hosts that speak the Chat Completions format.
// Exact hostnames, matched whole, never as a suffix — the same rule the allow-list
// follows.
var openAICompatibleHosts = map[string]bool{
	"api.openai.com": true,

	// OpenAI-compatible providers, all documented as drop-in for OpenAI clients.
	"api.deepseek.com":  true,
	"api.groq.com":      true,
	"api.mistral.ai":    true,
	"api.perplexity.ai": true,
	"api.together.xyz":  true,
	"api.x.ai":          true,
	"openrouter.ai":     true,

	// GitHub Copilot's chat endpoint is the same format.
	"api.githubcopilot.com": true,
}

func (OpenAI) Handles(host, path string) bool {
	if !openAICompatibleHosts[strings.ToLower(host)] {
		return false
	}

	// Some of these providers serve the endpoint without the /v1 prefix, and
	// OpenRouter serves it under /api/v1.
	for _, prefix := range []string{
		"/v1/chat/completions",
		"/v1/responses",
		"/v1/completions",
		"/api/v1/chat/completions",
		"/chat/completions",
	} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}

	return false
}

// openAIRequest is the part of the request we care about. Unknown fields are
// ignored, so a new API field never breaks parsing.
type openAIRequest struct {
	Model    string `json:"model"`
	Stream   bool   `json:"stream"`
	Messages []struct {
		Role string `json:"role"`
		// Content is a string in the simple case and a list of parts when the
		// message carries images or files, so it is decoded by hand below.
		Content json.RawMessage `json:"content"`
	} `json:"messages"`
	// The Responses API uses "input" and "instructions" instead of "messages".
	Input        json.RawMessage `json:"input"`
	Instructions string          `json:"instructions"`
}

func (p OpenAI) Parse(ex Exchange) (Result, error) {
	var res Result
	res.Tool = toolFromHeaders(ex.ReqHead)

	var req openAIRequest
	if err := json.Unmarshal(ex.ReqBody, &req); err != nil {
		return res, err
	}
	res.Model = req.Model
	res.Streamed = req.Stream
	res.System = req.Instructions

	// The last user message is what this request newly asked.
	for _, m := range req.Messages {
		text := textFromContent(m.Content)
		switch m.Role {
		case "user":
			res.Prompt = text
		case "system", "developer":
			if res.System == "" {
				res.System = text
			}
		case "tool":
			// A tool result means the agent is continuing by itself rather than
			// the human typing something new.
			res.Automated = true
		}
	}
	if res.Prompt == "" && len(req.Input) > 0 {
		res.Prompt = textFromContent(req.Input)
	}

	if len(ex.SSE) > 0 {
		res.Streamed = true
		res.Answer, res.PromptTokens, res.ResponseTokens = p.reassemble(ex.SSE)
		return res, nil
	}

	p.parseWholeResponse(ex.RespBody, &res)
	return res, nil
}

// reassemble joins a streamed Chat Completions response back into one answer.
//
// Each event carries a "delta" holding the next few characters. The final event
// is the literal text [DONE], which is not JSON.
func (OpenAI) reassemble(events []string) (answer string, promptTokens, responseTokens int) {
	var b strings.Builder
	for _, data := range events {
		if data == "[DONE]" || data == "" {
			continue
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
			// The Responses API streams a different shape.
			Type  string `json:"type"`
			Delta string `json:"delta"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue // a chunk we cannot read must not lose the ones we can
		}
		for _, c := range chunk.Choices {
			b.WriteString(c.Delta.Content)
		}
		if chunk.Type == "response.output_text.delta" {
			b.WriteString(chunk.Delta)
		}
		if chunk.Usage != nil {
			promptTokens = chunk.Usage.PromptTokens
			responseTokens = chunk.Usage.CompletionTokens
		}
	}
	return b.String(), promptTokens, responseTokens
}

func (OpenAI) parseWholeResponse(body []byte, res *Result) {
	var resp struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Output []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			InputTokens      int `json:"input_tokens"`
			OutputTokens     int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return
	}
	if resp.Model != "" {
		res.Model = resp.Model
	}

	var b strings.Builder
	for _, c := range resp.Choices {
		b.WriteString(c.Message.Content)
	}
	for _, o := range resp.Output {
		for _, c := range o.Content {
			b.WriteString(c.Text)
		}
	}
	res.Answer = b.String()

	res.PromptTokens = firstNonZero(resp.Usage.PromptTokens, resp.Usage.InputTokens)
	res.ResponseTokens = firstNonZero(resp.Usage.CompletionTokens, resp.Usage.OutputTokens)
}
