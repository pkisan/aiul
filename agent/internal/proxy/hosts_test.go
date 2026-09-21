package proxy

import (
	"strings"
	"sync"
	"testing"
)

// TestMatchHostIsAnchored is the rule 3 test: patterns match whole hostnames, so a
// lookalike domain can never trick us into decrypting traffic.
func TestMatchHostIsAnchored(t *testing.T) {
	cases := []struct {
		pattern string
		host    string
		want    bool
	}{
		// exact patterns
		{"api.openai.com", "api.openai.com", true},
		{"api.openai.com", "API.OPENAI.COM", false}, // callers lower-case first
		{"api.openai.com", "openai.com", false},
		{"api.openai.com", "xapi.openai.com", false},
		{"api.openai.com", "api.openai.com.evil.net", false},
		{"api.openai.com", "api.openai.computer", false},

		// wildcard patterns
		{"*.claude.ai", "claude.ai", true},
		{"*.claude.ai", "api.claude.ai", true},
		{"*.claude.ai", "a.b.claude.ai", true},
		{"*.claude.ai", "notclaude.ai", false},
		{"*.claude.ai", "xclaude.ai", false},
		{"*.claude.ai", "claude.ai.evil.net", false},
		{"*.claude.ai", "evilclaude.ai.attacker.com", false},

		// nothing matches nothing
		{"", "api.openai.com", false},
		{"api.openai.com", "", false},
	}

	for _, c := range cases {
		if got := matchHost(c.pattern, c.host); got != c.want {
			t.Errorf("matchHost(%q, %q) = %v, want %v", c.pattern, c.host, got, c.want)
		}
	}
}

func TestNormalizeHost(t *testing.T) {
	cases := map[string]string{
		"api.openai.com:443":  "api.openai.com",
		"API.OpenAI.com":      "api.openai.com",
		"api.openai.com.":     "api.openai.com",
		"  claude.ai:443  ":   "claude.ai",
		"[::1]:8443":          "::1",
		"127.0.0.1:8899":      "127.0.0.1",
		"":                    "",
		"bad host:443":        "",
		"api..openai.com":     "",
		"api.openai.com:http": "api.openai.com:http", // non-numeric port is not stripped
	}
	for in, want := range cases {
		if got := normalizeHost(in); got != want {
			t.Errorf("normalizeHost(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestClassifyDefaultsToPass(t *testing.T) {
	c := NewClassifier()

	// Everything that is not an AI host must pass through sealed. These are the
	// ones it would be most damaging to decrypt.
	for _, host := range []string{
		"www.google.com", "accounts.google.com", "googleapis.com",
		"storage.googleapis.com", "github.com", "api.github.com",
		"login.microsoftonline.com", "hdfcbank.com", "icloud.com",
		"notopenai.com", "api.openai.com.evil.net", "openai.com.attacker.io",
		"", "localhost", "127.0.0.1",
	} {
		if got := c.Classify(host, ""); got != Pass {
			t.Errorf("Classify(%q) = %v, want pass", host, got)
		}
	}
}

func TestClassifyCapturesAIHosts(t *testing.T) {
	c := NewClassifier()
	for _, host := range []string{
		"api.openai.com", "api.openai.com:443", "API.OPENAI.COM:443",
		"api.anthropic.com", "claude.ai", "api.claude.ai",
		"generativelanguage.googleapis.com", "chatgpt.com", "cdn.chatgpt.com",
	} {
		if got := c.Classify(host, ""); got != Capture {
			t.Errorf("Classify(%q) = %v, want capture", host, got)
		}
	}
}

// TestAllowListHasNoBroadDomains guards rule 3 against a future careless edit.
func TestAllowListHasNoBroadDomains(t *testing.T) {
	forbidden := []string{
		"google.com", "*.google.com", "googleapis.com", "*.googleapis.com",
		"github.com", "*.github.com", "githubusercontent.com", "*.githubusercontent.com",
		"amazonaws.com", "*.amazonaws.com", "microsoft.com", "*.microsoft.com",
		"cloudflare.com", "*.cloudflare.com", "apple.com", "*.apple.com",
	}
	for _, entry := range allowList {
		for _, bad := range forbidden {
			if entry == bad {
				t.Errorf("allow-list contains the broad shared domain %q; rule 3 forbids it", entry)
			}
		}
		if entry == "*" || strings.HasPrefix(entry, "*.com") || !strings.Contains(entry, ".") {
			t.Errorf("allow-list entry %q is not a specific hostname", entry)
		}
	}
}

func TestTunnelListWinsOverCapture(t *testing.T) {
	c := NewClassifier()

	if got := c.Classify("api.anthropic.com", "Claude"); got != Capture {
		t.Fatalf("precondition: want capture, got %v", got)
	}

	// Claude Desktop rejected our certificate — rule 4 says never try again for
	// that program.
	if !c.AddTunnel("api.anthropic.com:443", "Claude") {
		t.Error("AddTunnel should report the pair as newly added")
	}
	if c.AddTunnel("api.anthropic.com", "Claude") {
		t.Error("AddTunnel should report false the second time")
	}

	if got := c.Classify("api.anthropic.com", "Claude"); got != Tunnel {
		t.Errorf("after AddTunnel, Classify = %v, want tunnel", got)
	}
	// The bug this keys the list by program to fix: another program talking to the
	// same host must still be captured.
	if got := c.Classify("api.anthropic.com", "claude"); got != Capture {
		t.Errorf("the CLI on the same host changed to %v, want capture", got)
	}
	// Nor may an unidentified program inherit another program's rejection.
	if got := c.Classify("api.anthropic.com", ""); got != Capture {
		t.Errorf("unidentified program changed to %v, want capture", got)
	}
	// Tunneling one host must not affect any other.
	if got := c.Classify("api.openai.com", "Claude"); got != Capture {
		t.Errorf("unrelated host changed to %v", got)
	}
	if hosts := c.TunnelHosts(); len(hosts) != 1 || hosts[0] != "api.anthropic.com (Claude)" {
		t.Errorf("TunnelHosts = %v", hosts)
	}
}

// The bug behind this test: the Claude desktop app ships its own copy of Claude
// Code, so macOS reports "claude" as the process name for both it and the
// terminal CLI. Keyed by name, the desktop app's pinning silenced the CLI too.
// The key is the executable path, which differs.
func TestTunnelListSeparatesTwoProgramsWithTheSameName(t *testing.T) {
	const (
		desktop = "/Users/x/Library/Application Support/Claude/claude-code/2.1.275/claude.app/Contents/MacOS/claude"
		cli     = "/opt/homebrew/bin/claude"
	)
	c := NewClassifier()

	if !c.AddTunnel("api.anthropic.com", desktop) {
		t.Fatal("AddTunnel should report the pair as newly added")
	}

	if got := c.Classify("api.anthropic.com", desktop); got != Tunnel {
		t.Errorf("the program that rejected us = %v, want tunnel", got)
	}
	if got := c.Classify("api.anthropic.com", cli); got != Capture {
		t.Errorf("the terminal CLI = %v, want capture", got)
	}
}

// TestClassifierIsConcurrencySafe: run with -race. The proxy classifies on every
// connection while other connections may be adding tunnel entries.
func TestClassifierIsConcurrencySafe(t *testing.T) {
	c := NewClassifier()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); c.Classify("api.openai.com", "") }()
		go func() { defer wg.Done(); c.AddTunnel("api.anthropic.com", "Claude") }()
	}
	wg.Wait()
}

// The hosts observed on a real Mac on 2026-09-21, and the ones deliberately left
// off. Copilot on a personal plan talks to api.individual.githubcopilot.com, not
// api.githubcopilot.com, which is why Copilot usage was invisible until then.
func TestObservedIDEHostsAreClassified(t *testing.T) {
	c := NewClassifier()

	capture := []string{
		"api.individual.githubcopilot.com:443",
		"api2.cursor.sh:443",
		"api3.cursor.sh:443",
		"api2direct.cursor.sh:443",
		"api.origin.cursor.com:443",
	}
	for _, host := range capture {
		if got := c.Classify(host, ""); got != Capture {
			t.Errorf("%s should be captured, got %v", host, got)
		}
	}

	// Updates, telemetry and the extension marketplace are not AI usage, and
	// decrypting them would be reading traffic we have no business reading.
	pass := []string{
		"downloads.cursor.com:443",
		"metrics.cursor.sh:443",
		"marketplace.cursorapi.com:443",
		// And the anchoring rule still holds for the new entries.
		"api2.cursor.sh.evil.net:443",
		"notapi2.cursor.sh:443",
		"cursor.sh:443",
	}
	for _, host := range pass {
		if got := c.Classify(host, ""); got == Capture {
			t.Errorf("SECURITY: %s must not be decrypted, got %v", host, got)
		}
	}
}

// A client that offers only h2 is not distrusting our certificate — it is asking
// for a protocol we do not serve. Reported as pinning, it sent everyone hunting
// through trust stores and CA environment variables for a day.
func TestHandshakeFailureTellsALPNApartFromDistrust(t *testing.T) {
	reason, hint := handshakeFailure([]string{"h2"})
	if !strings.Contains(reason, "http/1.1") {
		t.Errorf("an h2-only client reported as %q", reason)
	}
	if strings.Contains(hint, "CA environment variable") {
		t.Errorf("hint sends the reader after certificates: %q", hint)
	}

	// Offering http/1.1 and still failing means the certificate really was the
	// problem; so does offering nothing at all (an old client, no ALPN).
	for _, offered := range [][]string{{"h2", "http/1.1"}, {"http/1.1"}, nil} {
		reason, _ := handshakeFailure(offered)
		if !strings.Contains(reason, "certificate") {
			t.Errorf("offered %v reported as %q, want a certificate problem", offered, reason)
		}
	}
}
