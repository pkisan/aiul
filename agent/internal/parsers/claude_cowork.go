package parsers

import (
	"encoding/json"
	"strings"
)

// ClaudeCowork parses a Claude desktop session that runs in Anthropic's cloud
// (Cowork remote, `environment_kind: anthropic_cloud`). The model is never called
// from the device, so the conversation crosses the proxy in two other places.
// Captured on 2026-09-29.
//
// The person's message is POSTed to the session:
//
//	POST claude.ai/v1/code/sessions/<cse_id>/events
//	{"events":[{"payload":{"type":"control_request","request":{"subtype":"set_model","model":"..."}}},
//	           {"payload":{"type":"user","uuid":"...","message":{"role":"user","content":"Hey"}}}]}
//
// The answer comes back on the session's event stream, a long-lived SSE that
// replays the session's history when it connects and then stays open:
//
//	GET claude.ai/v1/code/sessions/<cse_id>/events/stream
//	data: {"event_type":"user","event_id":"<same uuid>","source":"client","payload":{...}}
//	data: {"event_type":"assistant","source":"worker","payload":{"message":{"model":"...","content":[{"type":"text","text":"Hi"}],"usage":{...}}}}
//	data: {"event_type":"result","source":"worker","payload":{...}}
//
// The stream can stay open for hours, so the proxy splits it into turns as it
// flows (CoworkTurns) and hands each finished turn over as its own exchange, with
// Method TurnMethod. Both the POST and the turn carry the person's message uuid,
// so both become the same event id: the POST gives the prompt at once, the turn
// fills in the answer, and a replayed turn is recognised as one already sent.
type ClaudeCowork struct{}

// TurnMethod marks an exchange the proxy assembled from one turn of the stream.
const TurnMethod = "TURN"

func (ClaudeCowork) Name() string { return "claude-cowork" }

func (ClaudeCowork) Handles(host, path string) bool {
	if strings.ToLower(host) != "claude.ai" {
		return false
	}
	path = strings.TrimSuffix(strings.SplitN(path, "?", 2)[0], "/")
	return strings.HasPrefix(path, "/v1/code/sessions/") &&
		(strings.HasSuffix(path, "/events") || strings.HasSuffix(path, "/events/stream"))
}

// IsCoworkStream reports whether a path is a session's event stream, which the
// proxy splits into turns instead of recording as one exchange.
func IsCoworkStream(host, path string) bool {
	return strings.ToLower(host) == "claude.ai" &&
		strings.HasPrefix(path, "/v1/code/sessions/") &&
		strings.HasSuffix(strings.SplitN(path, "?", 2)[0], "/events/stream")
}

// coworkEvent is the part of an event, POSTed or streamed, that we read.
type coworkEvent struct {
	EventType string `json:"event_type"`
	EventID   string `json:"event_id"`
	Source    string `json:"source"`
	Payload   struct {
		Type string `json:"type"`
		UUID string `json:"uuid"`
		// false on context the app adds for the model (timezone, which device
		// folders it can reach), sent as user messages next to the person's own.
		ShouldQuery *bool `json:"shouldQuery"`
		Request     struct {
			Subtype string `json:"subtype"`
			Model   string `json:"model"`
		} `json:"request"`
		Message struct {
			Model   string          `json:"model"`
			Content json.RawMessage `json:"content"`
			Usage   struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		} `json:"message"`
	} `json:"payload"`
}

// personText is the text of a message the person typed, or "" for anything
// else: the app's own context, and tool results the agent feeds itself.
func (e coworkEvent) personText() string {
	p := e.Payload
	if p.Type != "user" || (p.ShouldQuery != nil && !*p.ShouldQuery) {
		return ""
	}
	if containsBlockType(p.Message.Content, "tool_result") {
		return ""
	}
	return textFromContent(p.Message.Content)
}

func (e coworkEvent) uuid() string {
	if e.Payload.UUID != "" {
		return e.Payload.UUID
	}
	return e.EventID
}

func coworkEventID(uuid string) string {
	if uuid == "" {
		return ""
	}
	return "cowork-" + uuid
}

func (ClaudeCowork) Parse(ex Exchange) (Result, error) {
	res := Result{Tool: "claude-cowork", Kind: KindHuman}

	switch {
	case ex.Method == TurnMethod:
		return parseCoworkTurn(ex.SSE, res), nil
	case ex.Method != "" && ex.Method != "POST":
		// The whole stream, recorded when it closes, or a read of past events:
		// its turns were already recorded one by one.
		res.Skip = true
		return res, nil
	}

	var req struct {
		Events []struct {
			Payload json.RawMessage `json:"payload"`
		} `json:"events"`
	}
	if err := json.Unmarshal(ex.ReqBody, &req); err != nil {
		return res, err
	}

	for _, raw := range req.Events {
		var ev coworkEvent
		if json.Unmarshal(raw.Payload, &ev.Payload) != nil {
			continue
		}
		if ev.Payload.Type == "control_request" && ev.Payload.Request.Subtype == "set_model" {
			res.Model = ev.Payload.Request.Model
		}
		if text := ev.personText(); text != "" {
			res.Prompt = text
			res.EventID = coworkEventID(ev.uuid())
		}
	}

	if res.Prompt == "" {
		res.Skip = true
	}
	return res, nil
}

// parseCoworkTurn reads one turn: the person's message, then what the model said.
func parseCoworkTurn(events []string, res Result) Result {
	res.Streamed = true
	var answer []string

	for _, data := range events {
		var ev coworkEvent
		if json.Unmarshal([]byte(data), &ev) != nil {
			continue
		}
		switch ev.EventType {
		case "user":
			if text := ev.personText(); text != "" && res.Prompt == "" {
				res.Prompt = text
				res.EventID = coworkEventID(ev.uuid())
			}
		case "assistant":
			msg := ev.Payload.Message
			if msg.Model != "" {
				res.Model = msg.Model
			}
			if text := textFromContent(msg.Content); text != "" {
				answer = append(answer, text)
			}
			res.PromptTokens += msg.Usage.InputTokens
			res.ResponseTokens += msg.Usage.OutputTokens
		}
	}

	res.Answer = strings.Join(answer, "\n\n")
	if res.Prompt == "" {
		res.Skip = true
	}
	return res
}

// CoworkTurns splits a session's event stream into turns: a message the person
// typed, everything that follows, up to the result that ends the model's reply.
type CoworkTurns struct {
	turn         []string
	sawAssistant bool
}

// ponytail: a turn longer than this keeps its start only; a long agent run's
// tail is dropped from our copy, the answer text up to that point survives.
const maxTurnEvents = 5000

// Add takes one event's data and returns a finished turn, or nil.
func (t *CoworkTurns) Add(data string) []string {
	var ev coworkEvent
	if json.Unmarshal([]byte(data), &ev) != nil {
		return nil
	}

	switch {
	case ev.EventType == "user" && ev.personText() != "":
		// A new message. One that arrives before the last reply ended (the
		// person typed again) starts over rather than merging two turns.
		t.turn = []string{data}
		t.sawAssistant = false
		return nil
	case t.turn == nil, ev.EventType == "stream_event":
		// Nothing started yet, or text pieces the assistant event repeats whole.
		return nil
	}

	if len(t.turn) < maxTurnEvents {
		t.turn = append(t.turn, data)
	}
	if ev.EventType == "assistant" {
		t.sawAssistant = true
	}
	// The first result can arrive before the model has said anything (a no-op
	// "queued" result); only one after an answer ends the turn.
	if ev.EventType == "result" && t.sawAssistant {
		done := t.turn
		t.turn, t.sawAssistant = nil, false
		return done
	}
	return nil
}
