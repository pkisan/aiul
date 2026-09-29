package parsers

import (
	"encoding/json"
	"strings"
)

// ClaudeCowork parses a message typed into a Claude desktop session that runs in
// Anthropic's cloud (Cowork remote, `environment_kind: anthropic_cloud`).
//
// Captured on 2026-09-29. The model is never called from the device: the app
// posts the person's message as an event to the session and the cloud worker
// does the rest, so this POST is the only place the prompt crosses the proxy.
//
//	POST claude.ai/v1/code/sessions/<cse_id>/events
//	{"events":[{"payload":{"type":"control_request","request":{"subtype":"set_model","model":"..."}}},
//	           {"payload":{"type":"user","message":{"role":"user","content":"How are you?"}}}]}
//
// The same endpoint also carries control requests alone (model and thinking
// changes); those are skipped. The answer does not come back on this exchange,
// so it is not recorded yet.
type ClaudeCowork struct{}

func (ClaudeCowork) Name() string { return "claude-cowork" }

func (ClaudeCowork) Handles(host, path string) bool {
	if strings.ToLower(host) != "claude.ai" {
		return false
	}
	path, _, _ = strings.Cut(path, "?")
	return strings.HasPrefix(path, "/v1/code/sessions/") &&
		strings.HasSuffix(strings.TrimSuffix(path, "/"), "/events")
}

func (ClaudeCowork) Parse(ex Exchange) (Result, error) {
	res := Result{Tool: "claude-cowork", Kind: KindHuman}

	// GET on the same path reads the session's events back; only a POST is the
	// person sending something.
	if ex.Method != "" && ex.Method != "POST" {
		res.Skip = true
		return res, nil
	}

	var req struct {
		Events []struct {
			Payload struct {
				Type string `json:"type"`
				// false on context the app adds for the model (timezone, which
				// device folders it can reach), sent as user messages in the same
				// POST as the person's own.
				ShouldQuery *bool `json:"shouldQuery"`
				Request     struct {
					Subtype string `json:"subtype"`
					Model   string `json:"model"`
				} `json:"request"`
				Message struct {
					Content json.RawMessage `json:"content"`
				} `json:"message"`
			} `json:"payload"`
		} `json:"events"`
	}
	if err := json.Unmarshal(ex.ReqBody, &req); err != nil {
		return res, err
	}

	for _, ev := range req.Events {
		switch {
		case ev.Payload.Type == "control_request" && ev.Payload.Request.Subtype == "set_model":
			res.Model = ev.Payload.Request.Model
		case ev.Payload.Type == "user" && (ev.Payload.ShouldQuery == nil || *ev.Payload.ShouldQuery):
			if text := textFromContent(ev.Payload.Message.Content); text != "" {
				res.Prompt = text
			}
		}
	}

	if res.Prompt == "" {
		res.Skip = true
	}
	return res, nil
}
