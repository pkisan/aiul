package parsers

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixture reads a recorded, anonymised body from testdata.
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return b
}

// sseData splits a recorded SSE body into its data payloads, the same way the
// proxy does before handing an exchange to a parser.
func sseData(t *testing.T, name string) []string {
	t.Helper()
	var out []string
	for _, line := range strings.Split(string(fixture(t, name)), "\n") {
		if after, ok := strings.CutPrefix(line, "data: "); ok {
			out = append(out, after)
		}
	}
	return out
}

func TestForPicksTheRightParser(t *testing.T) {
	cases := []struct {
		host, path, want string
	}{
		{"api.openai.com", "/v1/chat/completions", "openai"},
		{"api.openai.com", "/v1/responses", "openai"},
		{"api.anthropic.com", "/v1/messages", "anthropic"},
		{"api.anthropic.com", "/v1/messages?beta=true", "anthropic"},
		{"generativelanguage.googleapis.com", "/v1beta/models/gemini-2.5-pro:streamGenerateContent", "gemini"},
		{"api.openai.com", "/v1/models", ""}, // not a conversation

		// The OpenAI-compatible providers: same wire format, same parser.
		{"api.mistral.ai", "/v1/chat/completions", "openai"},
		{"api.groq.com", "/v1/chat/completions", "openai"},
		{"api.deepseek.com", "/v1/chat/completions", "openai"},
		{"api.x.ai", "/v1/chat/completions", "openai"},
		{"api.together.xyz", "/v1/chat/completions", "openai"},
		{"api.perplexity.ai", "/chat/completions", "openai"},
		{"openrouter.ai", "/api/v1/chat/completions", "openai"},
		{"api.githubcopilot.com", "/chat/completions", "openai"},

		// Still no parser: a different format, and one nobody has captured yet.
		{"api.cohere.com", "/v1/chat", ""},
		{"chatgpt.com", "/backend-api/conversation", ""},

		// A host must match whole, never as a suffix. Rule 3 in the allow-list, and
		// the same rule here, since a parser decides what gets stored.
		{"api.groq.com.evil.net", "/v1/chat/completions", ""},
		{"notapi.x.ai", "/v1/chat/completions", ""},
	}
	for _, c := range cases {
		p := For(c.host, c.path)
		got := ""
		if p != nil {
			got = p.Name()
		}
		if got != c.want {
			t.Errorf("For(%q, %q) = %q, want %q", c.host, c.path, got, c.want)
		}
	}
}

func TestAnthropicStreamedConversation(t *testing.T) {
	res, err := Anthropic{}.Parse(Exchange{
		Host:    "api.anthropic.com",
		Path:    "/v1/messages",
		ReqBody: fixture(t, "anthropic/messages-stream.request.json"),
		SSE:     sseData(t, "anthropic/messages-stream.response.sse"),
		ReqHead: http.Header{"User-Agent": {"claude-cli/1.0 (external, claude-code)"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if res.Model != "claude-opus-5" {
		t.Errorf("model = %q", res.Model)
	}
	if res.Prompt != "Explain what a TLS proxy does in two sentences." {
		t.Errorf("prompt = %q", res.Prompt)
	}
	if res.System != "You are a helpful assistant for a PHP team." {
		t.Errorf("system = %q", res.System)
	}
	// The whole point of reassembly: three deltas become one answer.
	want := "A TLS proxy sits between a client and a server and terminates the encrypted connection."
	if res.Answer != want {
		t.Errorf("answer = %q\nwant     %q", res.Answer, want)
	}
	if res.PromptTokens != 24 || res.ResponseTokens != 37 {
		t.Errorf("tokens = %d/%d, want 24/37", res.PromptTokens, res.ResponseTokens)
	}
	if !res.Streamed {
		t.Error("should be marked as streamed")
	}
	if res.Automated {
		t.Error("a typed question is not an automated follow-up")
	}
	if res.Tool != "claude-code" {
		t.Errorf("tool = %q, want claude-code", res.Tool)
	}
}

func TestAnthropicToolResultIsAutomated(t *testing.T) {
	res, err := Anthropic{}.Parse(Exchange{
		Host:     "api.anthropic.com",
		Path:     "/v1/messages",
		ReqBody:  fixture(t, "anthropic/tool-result.request.json"),
		RespBody: fixture(t, "anthropic/messages.response.json"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Automated {
		t.Error("a tool_result follow-up must be marked automated, not counted as a human prompt")
	}
	if res.Answer != "Two files: README.md and main.go." {
		t.Errorf("answer = %q", res.Answer)
	}
	if res.PromptTokens != 61 || res.ResponseTokens != 12 {
		t.Errorf("tokens = %d/%d", res.PromptTokens, res.ResponseTokens)
	}
}

func TestOpenAIStreamedConversation(t *testing.T) {
	res, err := OpenAI{}.Parse(Exchange{
		Host:    "api.openai.com",
		Path:    "/v1/chat/completions",
		ReqBody: fixture(t, "openai/chat-stream.request.json"),
		SSE:     sseData(t, "openai/chat-stream.response.sse"),
		ReqHead: http.Header{"User-Agent": {"OpenAI/Python 1.40.0"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Model != "gpt-5" {
		t.Errorf("model = %q", res.Model)
	}
	if res.Prompt != "Name one benefit of streaming responses." {
		t.Errorf("prompt = %q", res.Prompt)
	}
	if res.System != "You are a terse assistant." {
		t.Errorf("system = %q", res.System)
	}
	if res.Answer != "The user sees the answer as it is written." {
		t.Errorf("answer = %q", res.Answer)
	}
	if res.PromptTokens != 23 || res.ResponseTokens != 11 {
		t.Errorf("tokens = %d/%d, want 23/11", res.PromptTokens, res.ResponseTokens)
	}
	// [DONE] is not JSON and must not break anything or end up in the answer.
	if strings.Contains(res.Answer, "DONE") {
		t.Error("the [DONE] sentinel leaked into the answer")
	}
}

func TestOpenAIWholeResponseAndListContent(t *testing.T) {
	res, err := OpenAI{}.Parse(Exchange{
		Host:     "api.openai.com",
		Path:     "/v1/chat/completions",
		ReqBody:  fixture(t, "openai/chat.request.json"),
		RespBody: fixture(t, "openai/chat.response.json"),
	})
	if err != nil {
		t.Fatal(err)
	}
	// The prompt here is a list of content parts, not a plain string.
	if res.Prompt != "Why is streaming better?" {
		t.Errorf("prompt = %q", res.Prompt)
	}
	if res.Answer != "Because the reply appears while it is generated." {
		t.Errorf("answer = %q", res.Answer)
	}
	if res.Streamed {
		t.Error("this response was not streamed")
	}
}

func TestGeminiStreamedConversation(t *testing.T) {
	res, err := Gemini{}.Parse(Exchange{
		Host:    "generativelanguage.googleapis.com",
		Path:    "/v1beta/models/gemini-2.5-pro:streamGenerateContent",
		ReqBody: fixture(t, "gemini/generate-stream.request.json"),
		SSE:     sseData(t, "gemini/generate-stream.response.sse"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Model != "gemini-2.5-pro" {
		t.Errorf("model = %q (it comes from the path, not the body)", res.Model)
	}
	if res.Prompt != "Give one sentence about certificate pinning." {
		t.Errorf("prompt = %q", res.Prompt)
	}
	if res.System != "Answer briefly." {
		t.Errorf("system = %q", res.System)
	}
	if res.Answer != "Certificate pinning means a client accepts only one specific certificate." {
		t.Errorf("answer = %q", res.Answer)
	}
	// Usage repeats on each chunk and grows; the last value is the total.
	if res.PromptTokens != 14 || res.ResponseTokens != 12 {
		t.Errorf("tokens = %d/%d, want 14/12", res.PromptTokens, res.ResponseTokens)
	}
}

func TestGeminiWholeResponse(t *testing.T) {
	res, err := Gemini{}.Parse(Exchange{
		Host:     "generativelanguage.googleapis.com",
		Path:     "/v1beta/models/gemini-2.5-flash:generateContent",
		ReqBody:  fixture(t, "gemini/generate-stream.request.json"),
		RespBody: fixture(t, "gemini/generate.response.json"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Model != "gemini-2.5-flash" {
		t.Errorf("model = %q", res.Model)
	}
	if res.Streamed {
		t.Error("generateContent is not the streaming endpoint")
	}
	if res.Answer != "It accepts only a certificate it already knows." {
		t.Errorf("answer = %q", res.Answer)
	}
}

// A parser must never panic or lose everything because one part of a body is
// unexpected. Truncated copies are normal: we cap what we keep.
func TestParsersSurviveRubbish(t *testing.T) {
	bad := []Exchange{
		{ReqBody: []byte("not json at all")},
		{ReqBody: []byte(`{"model":"x","messages":[{"role":"user","content":{"weird":true}}]}`)},
		{ReqBody: []byte(`{"model":"x"}`), SSE: []string{"{oops", "", "[DONE]"}},
		{ReqBody: nil, RespBody: []byte(`{"truncated":`)},
	}
	for _, p := range All() {
		for i, ex := range bad {
			// An error is fine. A panic is not.
			_, _ = p.Parse(ex)
			_ = i
		}
	}
}

func TestToolFromHeaders(t *testing.T) {
	cases := map[string]string{
		"claude-cli/1.0 (external, claude-code)": "claude-code",
		"GeminiCLI/0.1.2 (darwin; arm64)":        "gemini-cli",
		"codex_cli_rs/0.5.0":                     "codex",
		"opencode/1.2.3":                         "opencode",
		"Mozilla/5.0 (Macintosh)":                "browser",
		"OpenAI/Python 1.40.0":                   "openai",
		"":                                       "",
	}
	for ua, want := range cases {
		h := http.Header{}
		if ua != "" {
			h.Set("User-Agent", ua)
		}
		if got := toolFromHeaders(h); got != want {
			t.Errorf("User-Agent %q -> %q, want %q", ua, got, want)
		}
	}

	// Anthropic clients name themselves in X-App.
	h := http.Header{"X-App": {"cli"}, "User-Agent": {"anthropic-sdk-go"}}
	if got := toolFromHeaders(h); got != "cli" {
		t.Errorf("X-App should win, got %q", got)
	}
}

// The OpenAI-compatible providers, parsed end to end.
//
// These fixtures are written from each provider's published API shape rather than
// captured from live traffic, which is an honest limitation: the owner has no
// account on most of them. What they prove is that the format those providers
// document is parsed correctly, including the streamed form. A live capture through
// mitmweb should still be taken before anyone claims a provider is supported —
// that is the research workflow in CLAUDE.md.
func TestOpenAICompatibleProviders(t *testing.T) {
	// Groq, not streamed. The shape is Chat Completions exactly, plus timing fields
	// of its own that we ignore.
	res, err := OpenAI{}.Parse(Exchange{
		Host: "api.groq.com",
		Path: "/v1/chat/completions",
		ReqBody: []byte(`{
			"model": "llama-3.3-70b-versatile",
			"messages": [
				{"role": "system", "content": "You are terse."},
				{"role": "user", "content": "name three sorting algorithms"}
			]
		}`),
		RespBody: []byte(`{
			"model": "llama-3.3-70b-versatile",
			"choices": [{"message": {"role": "assistant", "content": "quicksort, mergesort, heapsort"}}],
			"usage": {"prompt_tokens": 18, "completion_tokens": 9, "queue_time": 0.02}
		}`),
	})
	if err != nil {
		t.Fatalf("groq: %v", err)
	}
	if res.Prompt != "name three sorting algorithms" {
		t.Errorf("prompt = %q", res.Prompt)
	}
	if res.System != "You are terse." {
		t.Errorf("system = %q", res.System)
	}
	if res.Answer != "quicksort, mergesort, heapsort" {
		t.Errorf("answer = %q", res.Answer)
	}
	if res.Model != "llama-3.3-70b-versatile" {
		t.Errorf("model = %q", res.Model)
	}
	if res.PromptTokens != 18 || res.ResponseTokens != 9 {
		t.Errorf("tokens = %d/%d, want 18/9", res.PromptTokens, res.ResponseTokens)
	}

	// DeepSeek, streamed. Reassembly has to join the deltas and pick up the usage
	// block that arrives with the final chunk.
	res, err = OpenAI{}.Parse(Exchange{
		Host:    "api.deepseek.com",
		Path:    "/v1/chat/completions",
		ReqBody: []byte(`{"model":"deepseek-chat","stream":true,"messages":[{"role":"user","content":"why is the sky blue"}]}`),
		SSE: []string{
			`{"choices":[{"delta":{"content":"Rayleigh "}}]}`,
			`{"choices":[{"delta":{"content":"scattering"}}]}`,
			`{"choices":[{"delta":{}}],"usage":{"prompt_tokens":7,"completion_tokens":3}}`,
			`[DONE]`,
		},
	})
	if err != nil {
		t.Fatalf("deepseek: %v", err)
	}
	if res.Answer != "Rayleigh scattering" {
		t.Errorf("streamed answer = %q", res.Answer)
	}
	if !res.Streamed {
		t.Error("the event must record that it was streamed")
	}
	if res.PromptTokens != 7 || res.ResponseTokens != 3 {
		t.Errorf("tokens = %d/%d, want 7/3", res.PromptTokens, res.ResponseTokens)
	}

	// OpenRouter, under its own path prefix, and an agent's own follow-up rather
	// than a person typing: a tool result must not count as a new prompt.
	res, err = OpenAI{}.Parse(Exchange{
		Host:    "openrouter.ai",
		Path:    "/api/v1/chat/completions",
		ReqBody: []byte(`{"model":"anthropic/claude-3.5-sonnet","messages":[{"role":"user","content":"run the tests"},{"role":"tool","content":"3 passed"}]}`),
		RespBody: []byte(`{"choices":[{"message":{"content":"All green."}}],
			"usage":{"prompt_tokens":40,"completion_tokens":3}}`),
	})
	if err != nil {
		t.Fatalf("openrouter: %v", err)
	}
	if !res.Automated {
		t.Error("a tool result means the agent continued by itself; this must be marked automated")
	}
	if res.Answer != "All green." {
		t.Errorf("answer = %q", res.Answer)
	}
}

// chatgpt.com in a browser. The fixture below reproduces the protocol captured on
// 2026-09-21 — the same event shapes and the same order — with invented content:
// the real capture carried the person's own conversation and profile, so it is not
// in this repository.
func TestChatGPTWebConversation(t *testing.T) {
	res, err := ChatGPTWeb{}.Parse(Exchange{
		Host:   "chatgpt.com",
		Path:   "/backend-api/f/conversation",
		Method: "POST",
		Status: 200,
		ReqBody: []byte(`{
			"action": "next",
			"messages": [{
				"id": "eb27ed69-ad0c-4902-94dd-439c4427bbcd",
				"author": {"role": "user"},
				"content": {"content_type": "text", "parts": ["how do I reverse a list in python"]},
				"metadata": {"submission_mode": "manual_send"}
			}],
			"parent_message_id": "client-created-root",
			"model": "auto",
			"conversation_mode": {"kind": "primary_assistant"}
		}`),
		SSE: []string{
			`"v1"`,
			`{"type":"resume_conversation_token","kind":"topic","token":"eyJ...","conversation_id":"6ab0c34f"}`,
			// The user's own message echoed back, and system context messages. None
			// of this is the answer.
			`{"p":"","o":"add","v":{"message":{"id":"52df8e9d","author":{"role":"user"},"content":{"content_type":"user_editable_context","user_profile":"about the user"}}}}`,
			`{"v":{"message":{"id":"d4f4a7e5","author":{"role":"system"},"content":{"content_type":"text","parts":[""]}}}}`,
			`{"p":"","o":"add","v":{"message":{"id":"c8c08fde","author":{"role":"assistant"},"content":{"content_type":"text","parts":[""]},"metadata":{"model_slug":"gpt-5-6","default_model_slug":"auto"}}}}`,
			`{"type":"message_marker","message_id":"c8c08fde","marker":"user_visible_token","event":"first"}`,
			// The answer: one targeted append, then bare continuations, then a
			// batched patch that finishes the sentence and closes the turn.
			`{"p":"/message/content/parts/0","o":"append","v":"Use "}`,
			`{"v":"reversed(), "}`,
			`{"v":"or a [::-1] slice"}`,
			`{"p":"","o":"patch","v":[{"p":"/message/content/parts/0","o":"append","v":" for a copy."},{"p":"/message/status","o":"replace","v":"finished_successfully"},{"p":"/message/end_turn","o":"replace","v":true}]}`,
			`{"type":"message_marker","message_id":"c8c08fde","marker":"last_token","event":"last"}`,
			`{"type":"title_generation","title":"Reversing a list"}`,
		},
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if res.Prompt != "how do I reverse a list in python" {
		t.Errorf("prompt = %q", res.Prompt)
	}
	if res.Answer != "Use reversed(), or a [::-1] slice for a copy." {
		t.Errorf("answer = %q\nthe stream has to be reassembled from appends, bare continuations and a batched patch", res.Answer)
	}
	// The request said "auto"; the stream said what actually answered.
	if res.Model != "gpt-5-6" {
		t.Errorf("model = %q, want gpt-5-6", res.Model)
	}
	if !res.Streamed {
		t.Error("a browser conversation is always streamed")
	}
	if res.Tool != "chatgpt-web" {
		t.Errorf("tool = %q", res.Tool)
	}
}

// The web application calls a dozen endpoints with similar names. Only one of them
// is a person talking to the model, and recording the others would mean holding
// data about somebody for no benefit.
func TestOnlyTheChatGPTConversationEndpointIsParsed(t *testing.T) {
	cases := map[string]string{
		"/backend-api/f/conversation":                                     "chatgpt-web",
		"/backend-api/conversation":                                       "chatgpt-web",
		"/backend-api/f/conversation/prepare":                             "",
		"/backend-api/conversations":                                      "", // the sidebar list
		"/backend-api/conversation/experimental/generate_autocompletions": "",
		"/backend-api/sentinel/chat-requirements/prepare":                 "",
		"/ces/v1/telemetry/intake":                                        "",
		"/backend-api/codex/models":                                       "",
	}
	for path, want := range cases {
		got := ""
		if p := For("chatgpt.com", path); p != nil {
			got = p.Name()
		}
		if got != want {
			t.Errorf("For(chatgpt.com, %q) = %q, want %q", path, got, want)
		}
	}
}
