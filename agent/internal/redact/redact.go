// Package redact masks secrets and obvious personal data in text.
//
// It runs on OUR COPY only, before anything is written to disk or sent to the
// backend. The request forwarded to the provider is never modified — rule 8. A
// prompt routinely contains an API key someone pasted in, so this is the
// difference between a useful product and a liability from the first captured
// prompt.
//
// The approach is a rule list: each rule is a name, a pattern, and how to mask
// what it finds. Rules are deliberately boring and independent, so a bad rule can
// be removed without disturbing the others.
package redact

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// RulesVersion is bumped whenever the rule list changes, so a stored record says
// which version of the rules produced it.
const RulesVersion = 1

// Rule is one thing we look for.
type Rule struct {
	// Name appears in the list of what was masked, never the value itself.
	Name string

	// Pattern finds the secret. If the pattern has a capture group, only that
	// group is masked and the surrounding text is kept — so "password=hunter2"
	// becomes "password=[REDACTED:password]" rather than losing the field name.
	Pattern *regexp.Regexp

	// Mask replaces the match. Empty means "[REDACTED:<Name>]".
	Mask string
}

func (r Rule) mask() string {
	if r.Mask != "" {
		return r.Mask
	}
	return "[REDACTED:" + r.Name + "]"
}

// Result is what a redaction did.
type Result struct {
	Text string

	// Found counts each rule that matched, by name. Values are never included:
	// the point is to know a secret was there, not to keep it.
	Found map[string]int

	RulesVersion int
}

// Names lists the rules that matched, sorted, for logging and for the stored
// record.
func (r Result) Names() []string {
	out := make([]string, 0, len(r.Found))
	for name := range r.Found {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Any reports whether anything was masked.
func (r Result) Any() bool { return len(r.Found) > 0 }

// Redactor applies a rule list.
type Redactor struct {
	rules []Rule
}

// New returns a redactor with the default rules.
func New() *Redactor { return &Redactor{rules: DefaultRules()} }

// NewWith returns a redactor with an explicit rule list, for tests.
func NewWith(rules []Rule) *Redactor { return &Redactor{rules: rules} }

// String masks every rule's matches in s.
func (r *Redactor) String(s string) Result {
	res := Result{Text: s, Found: map[string]int{}, RulesVersion: RulesVersion}
	if s == "" {
		return res
	}

	for _, rule := range r.rules {
		count := 0
		res.Text = rule.Pattern.ReplaceAllStringFunc(res.Text, func(match string) string {
			count++
			// With a capture group, keep everything outside the group.
			groups := rule.Pattern.FindStringSubmatch(match)
			if len(groups) > 1 && groups[1] != "" {
				return strings.Replace(match, groups[1], rule.mask(), 1)
			}
			return rule.mask()
		})
		if count > 0 {
			res.Found[rule.Name] += count
		}
	}
	return res
}

// Strings masks several pieces of text and merges what was found, so an event's
// prompt and answer can be redacted together and reported once.
func (r *Redactor) Strings(in ...string) ([]string, Result) {
	out := make([]string, len(in))
	merged := Result{Found: map[string]int{}, RulesVersion: RulesVersion}
	for i, s := range in {
		res := r.String(s)
		out[i] = res.Text
		for name, n := range res.Found {
			merged.Found[name] += n
		}
	}
	return out, merged
}

// DefaultRules is the rule list. Order matters a little: the most specific
// patterns run first, so a key is masked as what it is rather than caught by a
// broader rule.
func DefaultRules() []Rule {
	return []Rule{
		// ---- provider API keys ------------------------------------------------
		// These have distinctive prefixes, which makes them safe to match without
		// any surrounding context.
		{
			// Before the OpenAI rule: an Anthropic key also starts with "sk-", so
			// whichever runs first decides the name in the record.
			Name:    "anthropic-key",
			Pattern: regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_-]{20,}\b`),
		},
		{
			Name:    "openai-key",
			Pattern: regexp.MustCompile(`\bsk-(?:proj-|svcacct-|admin-)?[A-Za-z0-9_-]{20,}\b`),
		},
		{
			Name:    "google-api-key",
			Pattern: regexp.MustCompile(`\bAIza[A-Za-z0-9_-]{33,37}\b`),
		},
		{
			Name:    "aws-access-key-id",
			Pattern: regexp.MustCompile(`\b(?:AKIA|ASIA|ABIA|ACCA)[A-Z0-9]{16}\b`),
		},
		{
			// An AWS secret key is 40 characters of base64 with no distinctive
			// shape, so it is only matched when it is labelled as one. Matching it
			// blind would mask any long random-looking string.
			Name:    "aws-secret-key",
			Pattern: regexp.MustCompile(`(?i)aws_?secret_?access_?key["'\s:=]+([A-Za-z0-9/+=]{40})`),
		},
		{
			Name:    "github-token",
			Pattern: regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,}\b`),
		},
		{
			Name:    "stripe-key",
			Pattern: regexp.MustCompile(`\b[sr]k_(?:live|test)_[A-Za-z0-9]{20,}\b`),
		},
		{
			Name:    "slack-token",
			Pattern: regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}\b`),
		},

		// ---- credentials in context -------------------------------------------
		{
			Name:    "bearer-token",
			Pattern: regexp.MustCompile(`(?i)\bbearer\s+([A-Za-z0-9._~+/=-]{16,})`),
		},
		{
			// A JSON Web Token: three dot-separated base64 parts, always starting
			// with the header "{"alg"..." encoded as eyJ.
			Name:    "jwt",
			Pattern: regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`),
		},
		{
			// password=..., "api_key": "...", SECRET_TOKEN = '...'
			// The value is the capture group, so the field name survives and the
			// record still shows that a password was there.
			Name: "password-assignment",
			Pattern: regexp.MustCompile(
				`(?i)(?:password|passwd|pwd|secret|api[_-]?key|access[_-]?token|auth[_-]?token|private[_-]?key|client[_-]?secret)\b["'\s]*[:=]\s*["']?([^\s"',;)]{6,})`),
		},
		{
			// A whole PEM block: private keys, and nothing else.
			Name:    "private-key-block",
			Pattern: regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`),
		},
		{
			Name:    "connection-string-password",
			Pattern: regexp.MustCompile(`(?i)\b(?:postgres|postgresql|mysql|mongodb(?:\+srv)?|redis|amqp)://[^:/@\s]+:([^@\s]+)@`),
		},

		// ---- personal data ------------------------------------------------------
		{
			Name:    "email",
			Pattern: regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`),
		},
		{
			// Indian mobile numbers, with or without the +91 country code, and
			// international numbers in E.164 form. Deliberately conservative: it
			// requires a country code or exactly ten digits starting 6-9, so an
			// order number or a port is not masked.
			Name:    "phone",
			Pattern: regexp.MustCompile(`(?:\+\d{1,3}[\s-]?)?\b[6-9]\d{9}\b|\+\d{1,3}[\s-]?\d{6,12}\b`),
		},
		{
			// Before the Aadhaar rule: a sixteen-digit card written in groups of four
			// also fits the Aadhaar shape, and it should be named for what it is.
			Name:    "credit-card",
			Pattern: regexp.MustCompile(`\b(?:\d{4}[\s-]?){3}\d{4}\b`),
		},
		{
			// Aadhaar: twelve digits, usually written in groups of four. Masked
			// whenever it is labelled, or when the grouped form appears.
			Name:    "aadhaar",
			Pattern: regexp.MustCompile(`\b[2-9]\d{3}[\s-]\d{4}[\s-]\d{4}\b|(?i)aadhaar["'\s:=]+(\d{12})\b`),
		},
		{
			// PAN: five letters, four digits, one letter.
			Name:    "pan",
			Pattern: regexp.MustCompile(`\b[A-Z]{5}\d{4}[A-Z]\b`),
		},
	}
}

// Describe returns a one-line summary of what was masked, for logs and for
// `aiul doctor`. It never contains a secret.
func Describe(res Result) string {
	if !res.Any() {
		return "nothing masked"
	}
	parts := make([]string, 0, len(res.Found))
	for _, name := range res.Names() {
		parts = append(parts, fmt.Sprintf("%s x%d", name, res.Found[name]))
	}
	return strings.Join(parts, ", ")
}
