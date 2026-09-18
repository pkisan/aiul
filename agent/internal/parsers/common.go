package parsers

import (
	"encoding/json"
	"net/http"
	"strings"
)

// textFromContent reads a message body that may be either a plain string or a list
// of typed parts. Both shapes appear in every provider's API.
//
//	"hello"                                    -> "hello"
//	[{"type":"text","text":"hello"}]           -> "hello"
//	[{"type":"image",...},{"type":"text",...}] -> the text parts only
func textFromContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}

	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
		// Gemini and the OpenAI Responses API nest text one level deeper.
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return ""
	}

	var b strings.Builder
	for _, p := range parts {
		switch {
		case p.Text != "":
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(p.Text)
		case len(p.Content) > 0:
			if nested := textFromContent(p.Content); nested != "" {
				if b.Len() > 0 {
					b.WriteString("\n")
				}
				b.WriteString(nested)
			}
		}
	}
	return b.String()
}

// containsBlockType reports whether a content list holds a block of this type,
// used to spot tool results — the sign of an automated follow-up.
func containsBlockType(raw json.RawMessage, want string) bool {
	var parts []struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return false
	}
	for _, p := range parts {
		if p.Type == want {
			return true
		}
	}
	return false
}

// toolFromHeaders names the client software from the request headers. Tools
// identify themselves in the User-Agent or in a provider-specific header; this is
// a best guess and an empty result is fine.
func toolFromHeaders(h http.Header) string {
	if h == nil {
		return ""
	}

	// Anthropic SDKs send this, and Claude Code sets it to "claude-code".
	for _, key := range []string{"X-App", "X-Stainless-Package-Version", "X-Client-Name"} {
		if v := h.Get(key); v != "" && key == "X-App" {
			return strings.ToLower(v)
		}
	}

	ua := strings.ToLower(h.Get("User-Agent"))
	switch {
	case ua == "":
		return ""
	case strings.Contains(ua, "claude-code"):
		return "claude-code"
	case strings.Contains(ua, "gemini-cli") || strings.Contains(ua, "geminicli") || strings.Contains(ua, "genai-js"):
		return "gemini-cli"
	case strings.Contains(ua, "codex"):
		return "codex"
	case strings.Contains(ua, "opencode"):
		return "opencode"
	case strings.Contains(ua, "cursor"):
		return "cursor"
	case strings.Contains(ua, "vscode"):
		return "vscode"
	case strings.Contains(ua, "mozilla"):
		return "browser"
	}
	// Otherwise keep the first token of the User-Agent: "openai-python/1.2.3".
	if first, _, found := strings.Cut(ua, "/"); found {
		return first
	}
	return ua
}

func firstNonZero(values ...int) int {
	for _, v := range values {
		if v != 0 {
			return v
		}
	}
	return 0
}
