package parsers

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
)

// AccountFromHeaders names the AI account a request was made with, when the
// credential itself says so. ChatGPT in the browser and Codex both send a JWT —
// a signed token whose middle part is plain JSON anyone can read — carrying the
// signed-in person's email. Only that claim is read; the token is never stored
// and its signature is not our business.
//
// Opaque tokens (Claude, Cursor's session keys) return "": those accounts are
// named by the tool's own profile endpoints, which need research first.
func AccountFromHeaders(h http.Header) string {
	token, found := strings.CutPrefix(h.Get("Authorization"), "Bearer ")
	if !found {
		return ""
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}

	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return ""
	}

	var claims struct {
		Name   string `json:"name"`
		Email  string `json:"email"`
		OpenAI struct {
			Email string `json:"email"`
		} `json:"https://api.openai.com/profile"`
	}
	if json.Unmarshal(payload, &claims) != nil {
		return ""
	}

	// A person's name reads best; the email is what OpenAI's tokens carry.
	return firstNonEmpty(claims.Name, claims.OpenAI.Email, claims.Email)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
