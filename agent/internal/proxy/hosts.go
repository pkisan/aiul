// Package proxy contains the explicit HTTPS proxy: it decides which connections to
// decrypt, mints certificates for those, and passes everything else through
// untouched.
package proxy

import (
	"strings"
	"sync"
)

// AllowListVersion is bumped whenever the list below changes, so a captured event
// records which version of the list decided to capture it.
const AllowListVersion = 1

// Decision is what the proxy does with one connection.
type Decision int

const (
	// Pass: not an AI host. Raw bytes are copied in both directions, no certificate
	// is minted, nothing is read or logged. This is the default and covers banking,
	// email, updates — everything.
	Pass Decision = iota

	// Capture: an allow-listed AI host. We terminate TLS, read the conversation,
	// and re-encrypt to the real provider.
	Capture

	// Tunnel: an AI host that has refused our certificate before. Bytes are copied
	// through untouched like Pass, but we log metadata (host, timing, bytes)
	// because we know it is an AI tool. Rule 4: never break a tool.
	Tunnel
)

func (d Decision) String() string {
	switch d {
	case Capture:
		return "capture"
	case Tunnel:
		return "tunnel"
	default:
		return "pass"
	}
}

// allowList is the versioned list of AI hostnames we decrypt. Rule 3 governs it:
//
//   - Entries are exact hostnames, or a "*." prefix meaning "this domain and its
//     subdomains" — both matched against the WHOLE hostname, never as a substring.
//     So "openai.com" never matches "notopenai.com" or "openai.com.evil.net".
//   - Broad shared domains (google.com, googleapis.com, github.com, amazonaws.com)
//     are never listed: decrypting those would mean decrypting half the internet's
//     traffic to read a few AI calls.
//
// Anything not on this list is passed through sealed.
var allowList = []string{
	// OpenAI
	"api.openai.com",
	"chatgpt.com",
	"*.chatgpt.com",
	"chat.openai.com",

	// Anthropic
	"api.anthropic.com",
	"claude.ai",
	"*.claude.ai",
	"statsig.anthropic.com",

	// Google Gemini. Note the narrow host: generativelanguage.googleapis.com only,
	// never all of googleapis.com.
	"generativelanguage.googleapis.com",
	"gemini.google.com",
	"aistudio.google.com",

	// GitHub Copilot. Again narrow: the Copilot API hosts only, never github.com.
	"api.githubcopilot.com",
	"copilot-proxy.githubusercontent.com",

	// Other providers commonly used from CLIs and IDEs.
	"api.mistral.ai",
	"api.groq.com",
	"api.deepseek.com",
	"api.x.ai",
	"api.cohere.com",
	"api.perplexity.ai",
	"openrouter.ai",
	"api.together.xyz",
}

// AllowListEntries returns a copy of the allow-list.
//
// It exists so the device intermediate can be name-constrained to exactly the
// hosts we are allowed to decrypt and no others (D3). A copy, because a caller
// must not be able to widen what this proxy will intercept.
func AllowListEntries() []string {
	out := make([]string, len(allowList))
	copy(out, allowList)

	return out
}

// Classifier decides what to do with a hostname. It is safe for concurrent use:
// the proxy handles many connections at once, and any of them may discover a host
// that has to be tunneled.
type Classifier struct {
	mu      sync.RWMutex
	allowed []string        // from allowList, lower-cased
	tunnel  map[string]bool // hosts that rejected our certificate (rule 4)
}

// NewClassifier builds a classifier from the built-in allow-list.
func NewClassifier() *Classifier {
	return NewClassifierWith(allowList)
}

// NewClassifierWith builds a classifier from an explicit list. Tests use it; it
// also leaves room for a configurable list later.
func NewClassifierWith(hosts []string) *Classifier {
	c := &Classifier{tunnel: make(map[string]bool)}
	for _, h := range hosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h != "" {
			c.allowed = append(c.allowed, h)
		}
	}
	return c
}

// Classify returns what to do with this host. The argument may include a port
// ("api.openai.com:443"), as it does in a CONNECT request.
func (c *Classifier) Classify(hostport string) Decision {
	host := normalizeHost(hostport)
	if host == "" {
		return Pass
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	// Tunnel wins over capture: once a host has rejected our certificate, we never
	// try again, or we would break the tool on every connection.
	if c.tunnel[host] {
		return Tunnel
	}
	for _, pattern := range c.allowed {
		if matchHost(pattern, host) {
			return Capture
		}
	}
	return Pass
}

// AddTunnel records that this host refused our certificate, so every future
// connection to it passes through sealed. Returns false if it was already known.
func (c *Classifier) AddTunnel(hostport string) bool {
	host := normalizeHost(hostport)
	if host == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tunnel[host] {
		return false
	}
	c.tunnel[host] = true
	return true
}

// TunnelHosts lists the hosts currently being tunneled, for `aiul status`.
func (c *Classifier) TunnelHosts() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, 0, len(c.tunnel))
	for h := range c.tunnel {
		out = append(out, h)
	}
	return out
}

// normalizeHost strips the port, lower-cases, removes a trailing dot (the root
// label, "openai.com." and "openai.com" are the same name), and rejects anything
// that is not plausibly a hostname.
func normalizeHost(hostport string) string {
	h := strings.TrimSpace(hostport)
	if h == "" {
		return ""
	}

	// An IPv6 literal is written [::1]:443. Take what is inside the brackets.
	if strings.HasPrefix(h, "[") {
		if end := strings.Index(h, "]"); end > 0 {
			return strings.ToLower(h[1:end])
		}
		return ""
	}

	// Strip a trailing :port. Only the LAST colon, and only if what follows is
	// digits, so an IPv6 address without brackets is not mangled.
	if i := strings.LastIndex(h, ":"); i >= 0 {
		port := h[i+1:]
		if port != "" && isAllDigits(port) {
			h = h[:i]
		}
	}

	h = strings.ToLower(strings.TrimSuffix(h, "."))

	// A hostname with whitespace, a slash or an empty label is malformed. Treat it
	// as unknown rather than trying to be clever: unknown means Pass, which is the
	// safe direction.
	if strings.ContainsAny(h, " \t/\\") || strings.Contains(h, "..") {
		return ""
	}
	return h
}

// matchHost compares one allow-list pattern against a whole hostname.
//
// This is the function rule 3 is about. It matches whole labels only:
//
//	"openai.com"    matches openai.com and nothing else
//	"*.openai.com"  matches api.openai.com, a.b.openai.com, and openai.com itself
//	                but NOT notopenai.com, and NOT openai.com.evil.net
func matchHost(pattern, host string) bool {
	if pattern == "" || host == "" {
		return false
	}

	if suffix, ok := strings.CutPrefix(pattern, "*."); ok {
		// The bare domain counts as a match: "*.claude.ai" covers claude.ai.
		if host == suffix {
			return true
		}
		// Otherwise the host must END with ".claude.ai". Requiring the dot is what
		// stops "notclaude.ai" matching, and anchoring at the end is what stops
		// "claude.ai.evil.net" matching.
		return strings.HasSuffix(host, "."+suffix)
	}

	// No wildcard: the entire hostname must be identical.
	return host == pattern
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
