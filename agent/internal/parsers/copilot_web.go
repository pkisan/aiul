package parsers

import (
	"encoding/json"
	"strings"
)

// CopilotWeb parses a conversation typed into github.com/copilot in a browser.
//
// Captured on 2026-09-23. The page talks to the Copilot API host directly — not
// to github.com, which is never allow-listed (rule 3):
//
//	POST api.individual.githubcopilot.com/github/chat/threads/<thread>/messages
//	{"content":"Hi there","model":"auto","intent":"conversation", ...}
//
// The answer streams back as SSE events of our own kind, not OpenAI's:
//
//	{"type":"routedModel","model":"mai-code-1.1-flash"}   which model "auto" chose
//	{"type":"content","body":"Hi"}                        a piece of the answer
//	{"type":"complete", ..., "usage":{"inputTokens":..,"outputTokens":..}}
//
// The browser also sends an OPTIONS preflight to the same path; that is skipped.
type CopilotWeb struct{}

func (CopilotWeb) Name() string { return "copilot-web" }

func (CopilotWeb) Handles(host, path string) bool {
	if !strings.HasSuffix(strings.ToLower(host), ".githubcopilot.com") {
		return false
	}
	path, _, _ = strings.Cut(path, "?")

	// /github/chat/threads/<thread>/messages, and nothing else under /github/chat:
	// the thread list, /name (the generated title) and /models are not a turn.
	rest, ok := strings.CutPrefix(path, "/github/chat/threads/")
	if !ok {
		return false
	}
	thread, tail, _ := strings.Cut(rest, "/")
	return thread != "" && tail == "messages"
}

func (CopilotWeb) Parse(ex Exchange) (Result, error) {
	if ex.Method == "OPTIONS" {
		return Result{Skip: true}, nil
	}

	res := Result{Tool: "copilot-web", Kind: KindHuman, Streamed: len(ex.SSE) > 0}

	var req struct {
		Content string `json:"content"`
		Model   string `json:"model"`
	}
	if err := json.Unmarshal(ex.ReqBody, &req); err != nil {
		return res, err
	}
	res.Prompt = req.Content
	res.Model = req.Model // "auto" until the stream says which model answered

	var answer strings.Builder
	for _, data := range ex.SSE {
		var ev struct {
			Type  string `json:"type"`
			Body  string `json:"body"`
			Model string `json:"model"`
			Usage struct {
				InputTokens  int `json:"inputTokens"`
				OutputTokens int `json:"outputTokens"`
			} `json:"usage"`
		}
		if json.Unmarshal([]byte(data), &ev) != nil {
			continue
		}
		switch ev.Type {
		case "content":
			answer.WriteString(ev.Body)
		case "routedModel", "complete":
			if ev.Model != "" {
				res.Model = ev.Model
			}
			if ev.Type == "complete" {
				res.PromptTokens = ev.Usage.InputTokens
				res.ResponseTokens = ev.Usage.OutputTokens
			}
		}
	}
	res.Answer = answer.String()
	return res, nil
}
