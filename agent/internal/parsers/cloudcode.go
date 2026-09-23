package parsers

import (
	"encoding/json"
	"strings"
)

// CloudCode parses Google's Cloud Code endpoint, which Antigravity uses for every
// model call:
//
//	POST daily-cloudcode-pa.googleapis.com/v1internal:streamGenerateContent
//
// The body is Gemini's generateContent format inside an envelope — the request
// under "request" with the model beside it, and each streamed chunk under
// "response" — so this unwraps both and hands the rest to the Gemini parser.
// Fixtures: testdata/cloudcode, recorded 2026-09-23.
type CloudCode struct{}

func (CloudCode) Name() string { return "cloudcode" }

func (CloudCode) Handles(host, path string) bool {
	return (host == "cloudcode-pa.googleapis.com" || host == "daily-cloudcode-pa.googleapis.com") &&
		strings.Contains(strings.ToLower(path), "generatecontent")
}

func (CloudCode) Parse(ex Exchange) (Result, error) {
	var env struct {
		Model   string          `json:"model"`
		Request json.RawMessage `json:"request"`
	}
	if err := json.Unmarshal(ex.ReqBody, &env); err != nil {
		return Result{Tool: toolFromHeaders(ex.ReqHead)}, err
	}

	inner := ex
	inner.ReqBody = env.Request
	inner.SSE = make([]string, 0, len(ex.SSE))
	for _, data := range ex.SSE {
		inner.SSE = append(inner.SSE, unwrapResponse(data))
	}
	if len(ex.RespBody) > 0 {
		inner.RespBody = []byte(unwrapResponse(string(ex.RespBody)))
	}

	res, err := Gemini{}.Parse(inner)
	res.Model = env.Model
	if err != nil {
		return res, err
	}

	// Antigravity wraps what the person typed in <USER_REQUEST>, followed by
	// metadata blocks (open files, local time, settings changes). It also appends
	// <EPHEMERAL_MESSAGE> reminders as extra user messages, "not actually sent by
	// the user" in its own words; those are dropped before anything is decided,
	// or the reminder becomes the prompt and hides a tool result behind it.
	var req struct {
		Contents []struct {
			Role  string `json:"role"`
			Parts []struct {
				Text             string          `json:"text"`
				FunctionResponse json.RawMessage `json:"functionResponse"`
			} `json:"parts"`
		} `json:"contents"`
		Tools []json.RawMessage `json:"tools"`
	}
	_ = json.Unmarshal(env.Request, &req) // already decoded once by Gemini{}.Parse

	lastIsToolResult := false
	userRequest := ""
	for _, c := range req.Contents {
		real := false
		for _, p := range c.Parts {
			if strings.Contains(p.Text, "<EPHEMERAL_MESSAGE>") {
				continue
			}
			real = true
			if inside, ok := between(p.Text, "<USER_REQUEST>", "</USER_REQUEST>"); ok && c.Role != "model" {
				userRequest = strings.TrimSpace(inside)
			}
		}
		if !real {
			continue
		}
		lastIsToolResult = false
		for _, p := range c.Parts {
			if len(p.FunctionResponse) > 0 {
				lastIsToolResult = true
			}
		}
	}
	if userRequest != "" {
		res.Prompt = userRequest
	} else if strings.Contains(res.Prompt, "<EPHEMERAL_MESSAGE>") {
		res.Prompt = ""
	}

	// The title generator and other housekeeping calls offer no tools, which
	// kindOf reads as utility for an agent like this one.
	res.Kind = kindOf(lastIsToolResult, len(req.Tools) > 0, res.Tool)
	res.Automated = res.Kind != KindHuman
	return res, nil
}

// unwrapResponse returns the Gemini chunk inside {"response": {...}}, or the
// input unchanged when there is no envelope.
func unwrapResponse(data string) string {
	var env struct {
		Response json.RawMessage `json:"response"`
	}
	if json.Unmarshal([]byte(data), &env) == nil && len(env.Response) > 0 {
		return string(env.Response)
	}
	return data
}

func between(s, open, close string) (string, bool) {
	_, after, ok := strings.Cut(s, open)
	if !ok {
		return "", false
	}
	inside, _, ok := strings.Cut(after, close)
	return inside, ok
}
