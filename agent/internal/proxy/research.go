package proxy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Research mode: writing down what a tool actually sends, so a parser can be
// written for it.
//
// Supporting a new AI tool needs its real request and response shapes. The
// documented way is to run that one tool through mitmweb by hand. But the agent is
// already decrypting exactly this traffic and already knows which hosts are
// allow-listed, so it can write the exchange down itself — no second proxy, no
// second CA, and it works for a tool that is awkward to launch with a different
// proxy setting.
//
// Three rules this obeys:
//
//   - OFF unless AIUL_RESEARCH_DUMP names a directory, and loud in the log when on
//   - only allow-listed hosts, because only those are decrypted at all
//   - REDACTED bodies. Masking replaces values and leaves the JSON structure
//     intact, which is the part a parser is written against. A dump is still
//     someone's conversation, so the directory is 0700 and the files 0600.
type researchDumper struct {
	dir string
}

func newResearchDumper(dir string) (*researchDumper, error) {
	if dir == "" {
		return nil, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("research dump directory: %w", err)
	}

	return &researchDumper{dir: dir}, nil
}

// dumpedExchange is what gets written. Field names match what a parser test needs
// to build a fixture from it.
type dumpedExchange struct {
	Time     time.Time         `json:"time"`
	Host     string            `json:"host"`
	Method   string            `json:"method"`
	Path     string            `json:"path"`
	Status   int               `json:"status"`
	Parser   string            `json:"parser_that_claimed_it,omitempty"`
	ReqHead  map[string]string `json:"request_headers"`
	ReqBody  string            `json:"request_body"`
	RespHead map[string]string `json:"response_headers"`
	RespBody string            `json:"response_body,omitempty"`
	SSE      []string          `json:"sse_events,omitempty"`
}

// interesting reports whether an exchange is worth writing down.
//
// A web application fetches hundreds of scripts, stylesheets and images from the
// same allow-listed host. None of them is a conversation, and a directory with a
// thousand JavaScript files in it is not research material.
func interesting(path, contentType string) bool {
	lowerPath := strings.ToLower(path)
	for _, ext := range []string{".js", ".css", ".map", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".woff", ".woff2", ".ttf", ".ico", ".mp4", ".wasm"} {
		if strings.HasSuffix(lowerPath, ext) {
			return false
		}
	}
	if strings.Contains(lowerPath, "/cdn/assets/") || strings.Contains(lowerPath, "/static/") {
		return false
	}

	// Anything that is not text is not a conversation either.
	ct := strings.ToLower(contentType)
	if ct != "" &&
		!strings.Contains(ct, "json") &&
		!strings.Contains(ct, "text") &&
		!strings.Contains(ct, "event-stream") {
		return false
	}

	return true
}

func (d *researchDumper) write(ex dumpedExchange) error {
	if d == nil {
		return nil
	}

	// One file per exchange, named so a listing reads in order and says what it
	// holds: 20260921T105021.356-chatgpt.com-backend-api-f-conversation.json
	safe := strings.Trim(strings.ReplaceAll(strings.Trim(ex.Path, "/"), "/", "-"), "-")
	if len(safe) > 60 {
		safe = safe[:60]
	}
	name := fmt.Sprintf("%s-%s-%s.json",
		ex.Time.Format("20060102T150405.000"), ex.Host, safe)

	body, err := json.MarshalIndent(ex, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(d.dir, name), body, 0o600)
}
