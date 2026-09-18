package parsers

import (
	"encoding/json"
	"strings"
)

// Anthropic parses the Messages API at api.anthropic.com, which is what Claude
// Code and the Anthropic SDKs use.
type Anthropic struct{}

func (Anthropic) Name() string { return "anthropic" }

func (Anthropic) Handles(host, path string) bool {
	return host == "api.anthropic.com" && strings.HasPrefix(path, "/v1/messages")
}

func (p Anthropic) Parse(ex Exchange) (Result, error) {
	var res Result
	res.Tool = toolFromHeaders(ex.ReqHead)

	var req struct {
		Model    string          `json:"model"`
		Stream   bool            `json:"stream"`
		System   json.RawMessage `json:"system"`
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(ex.ReqBody, &req); err != nil {
		return res, err
	}
	res.Model = req.Model
	res.Streamed = req.Stream
	res.System = textFromContent(req.System)

	for _, m := range req.Messages {
		if m.Role != "user" {
			continue
		}
		// A user message whose content is a tool_result block is the agent feeding
		// itself the output of a tool, not a human typing a new prompt.
		if containsBlockType(m.Content, "tool_result") {
			res.Automated = true
		}
		if text := textFromContent(m.Content); text != "" {
			res.Prompt = text
		}
	}

	if len(ex.SSE) > 0 {
		res.Streamed = true
		res.Answer, res.PromptTokens, res.ResponseTokens = p.reassemble(ex.SSE)
		return res, nil
	}

	p.parseWholeResponse(ex.RespBody, &res)
	return res, nil
}

// reassemble joins a streamed Messages response.
//
// The stream is a sequence of typed events: message_start carries the input token
// count, each content_block_delta carries the next piece of text, and
// message_delta at the end carries the output token count.
func (Anthropic) reassemble(events []string) (answer string, promptTokens, responseTokens int) {
	var b strings.Builder
	for _, data := range events {
		if data == "" {
			continue
		}
		var ev struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
			Message struct {
				Usage struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			} `json:"message"`
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			continue
		}
		switch ev.Type {
		case "content_block_delta":
			// thinking_delta blocks also arrive here; only text is the answer.
			if ev.Delta.Type == "text_delta" || ev.Delta.Type == "" {
				b.WriteString(ev.Delta.Text)
			}
		case "message_start":
			promptTokens = ev.Message.Usage.InputTokens
			if ev.Message.Usage.OutputTokens > 0 {
				responseTokens = ev.Message.Usage.OutputTokens
			}
		case "message_delta":
			if ev.Usage.OutputTokens > 0 {
				responseTokens = ev.Usage.OutputTokens
			}
			if ev.Usage.InputTokens > 0 {
				promptTokens = ev.Usage.InputTokens
			}
		}
	}
	return b.String(), promptTokens, responseTokens
}

func (Anthropic) parseWholeResponse(body []byte, res *Result) {
	var resp struct {
		Model   string `json:"model"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return
	}
	if resp.Model != "" {
		res.Model = resp.Model
	}
	var b strings.Builder
	for _, c := range resp.Content {
		if c.Type == "text" || c.Type == "" {
			b.WriteString(c.Text)
		}
	}
	res.Answer = b.String()
	res.PromptTokens = resp.Usage.InputTokens
	res.ResponseTokens = resp.Usage.OutputTokens
}
