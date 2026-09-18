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
		{"api.openai.com", "/v1/models", ""},        // not a conversation
		{"api.mistral.ai", "/v1/chat/completions", ""}, // no parser yet: metadata only
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
