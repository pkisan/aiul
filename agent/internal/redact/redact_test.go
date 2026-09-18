package redact

import (
	"strings"
	"testing"
)

// mustMask checks that the secret is gone, the rule fired under the right name,
// and any surrounding text survived.
func mustMask(t *testing.T, rule, input, secret string) Result {
	t.Helper()
	res := New().String(input)
	if strings.Contains(res.Text, secret) {
		t.Errorf("%s: the secret survived redaction\n  in:  %s\n  out: %s", rule, input, res.Text)
	}
	if res.Found[rule] == 0 {
		t.Errorf("%s: rule did not fire (fired: %v)\n  in: %s", rule, res.Names(), input)
	}
	return res
}

// mustNotMask checks we are not over-masking ordinary text.
func mustNotMask(t *testing.T, input string) {
	t.Helper()
	res := New().String(input)
	if res.Any() {
		t.Errorf("over-masked ordinary text %q -> %q (rules: %v)", input, res.Text, res.Names())
	}
}

func TestProviderAPIKeys(t *testing.T) {
	cases := []struct{ rule, input, secret string }{
		{"openai-key",
			"my key is sk-proj-abcdefGHIJKL0123456789mnopqrstuvwxyz and it works",
			"sk-proj-abcdefGHIJKL0123456789mnopqrstuvwxyz"},
		{"openai-key",
			"OPENAI_API_KEY=sk-abcdefGHIJKL0123456789mnopqrst",
			"sk-abcdefGHIJKL0123456789mnopqrst"},
		{"anthropic-key",
			"export ANTHROPIC_API_KEY=sk-ant-api03-AbCdEf0123456789_GhIjKlMnOpQrSt",
			"sk-ant-api03-AbCdEf0123456789_GhIjKlMnOpQrSt"},
		{"google-api-key",
			"key: AIzaSyA1234567890abcdefghijklmnopqrstu",
			"AIzaSyA1234567890abcdefghijklmnopqrstu"},
		{"aws-access-key-id",
			"AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE",
			"AKIAIOSFODNN7EXAMPLE"},
		{"aws-secret-key",
			"aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"},
		{"github-token",
			"clone with ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789",
			"ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"},
		{"stripe-key",
			"sk_live_ABCDEFGHIJKLMNOPQRSTUVWX is the live key",
			"sk_live_ABCDEFGHIJKLMNOPQRSTUVWX"},
		{"slack-token",
			"xoxb-1234567890-ABCDEFGHIJKLMNOP",
			"xoxb-1234567890-ABCDEFGHIJKLMNOP"},
	}
	for _, c := range cases {
		mustMask(t, c.rule, c.input, c.secret)
	}
}

func TestBearerTokensAndJWTs(t *testing.T) {
	res := mustMask(t, "bearer-token",
		"Authorization: Bearer abcdefghijklmnopqrstuvwxyz0123456789",
		"abcdefghijklmnopqrstuvwxyz0123456789")
	// The header name must survive, or the record loses its meaning.
	if !strings.Contains(res.Text, "Authorization") || !strings.Contains(res.Text, "Bearer") {
		t.Errorf("the surrounding text should survive: %q", res.Text)
	}

	mustMask(t, "jwt",
		"token eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dBjftJeZ4CVPmB92K27uhbUJU1p1r_wW1gFWFOEjXk",
		"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9")
}

func TestPasswordsInKeyValueForm(t *testing.T) {
	cases := []struct{ input, secret string }{
		{"password=hunter2secret", "hunter2secret"},
		{`"api_key": "abcdef123456"`, "abcdef123456"},
		{"DB_PASSWORD = 'p@ssw0rd!'", "p@ssw0rd!"},
		{"client_secret: GOCSPX-abcdef123456", "GOCSPX-abcdef123456"},
		{"access-token=abcdef1234567890", "abcdef1234567890"},
	}
	for _, c := range cases {
		res := mustMask(t, "password-assignment", c.input, c.secret)
		// The field name is kept on purpose: it shows a password was there.
		if !strings.ContainsAny(res.Text, "=:") {
			t.Errorf("the assignment should survive: %q", res.Text)
		}
	}
}

func TestPrivateKeyBlock(t *testing.T) {
	input := `Here is the key:
-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEAsecretkeymaterialthatmustneverbestored
morelinesofsecretkeymaterialgohereandhere
-----END RSA PRIVATE KEY-----
please use it`
	res := mustMask(t, "private-key-block", input, "MIIEowIBAAKCAQEA")
	if !strings.Contains(res.Text, "please use it") {
		t.Errorf("text after the block was lost: %q", res.Text)
	}
	if strings.Contains(res.Text, "BEGIN RSA PRIVATE KEY") {
		t.Error("the whole block must go, markers included")
	}
}

func TestConnectionStringPassword(t *testing.T) {
	res := mustMask(t, "connection-string-password",
		"DATABASE_URL=postgres://appuser:s3cr3tp4ss@db.internal:5432/app",
		"s3cr3tp4ss")
	// The host and user are useful context and are not secrets.
	if !strings.Contains(res.Text, "db.internal") || !strings.Contains(res.Text, "appuser") {
		t.Errorf("host and user should survive: %q", res.Text)
	}
}

func TestPersonalData(t *testing.T) {
	mustMask(t, "email", "write to punit@example.com about it", "punit@example.com")
	mustMask(t, "phone", "call me on 9876543210 tomorrow", "9876543210")
	mustMask(t, "phone", "my number is +91 9876543210", "9876543210")
	mustMask(t, "aadhaar", "aadhaar: 234567890123", "234567890123")
	mustMask(t, "aadhaar", "the number 2345 6789 0123 is mine", "2345 6789 0123")
	mustMask(t, "pan", "PAN ABCDE1234F for the invoice", "ABCDE1234F")
	mustMask(t, "credit-card", "card 4111 1111 1111 1111 expires soon", "4111 1111 1111 1111")
}

// The other half of the job: ordinary prompts must come through untouched, or
// people stop trusting the captured record.
func TestDoesNotOverMask(t *testing.T) {
	for _, s := range []string{
		"Explain what a TLS proxy does in two sentences.",
		"Refactor this Laravel controller to use a form request.",
		"The build failed with exit code 127 on line 4321.",
		"Use port 8899 and timeout 30000 milliseconds.",
		"git commit -m 'fix: handle the 404 case'",
		"The order id is 12345678 and the invoice is INV-2026-0042.",
		"sk- is a prefix, not a key.",
		"My name is Punit and I work on WordPress plugins.",
		"See https://example.com/docs/v1/messages for details.",
		"version 1.2.3, released 2026-09-18",
	} {
		mustNotMask(t, s)
	}
}

func TestSeveralSecretsInOnePrompt(t *testing.T) {
	input := "use sk-abcdefGHIJKL0123456789mnopqrst and mail me at punit@example.com, password=hunter2secret"
	res := New().String(input)

	for _, rule := range []string{"openai-key", "email", "password-assignment"} {
		if res.Found[rule] == 0 {
			t.Errorf("%s did not fire; fired: %v", rule, res.Names())
		}
	}
	for _, secret := range []string{"sk-abcdefGHIJKL0123456789mnopqrst", "punit@example.com", "hunter2secret"} {
		if strings.Contains(res.Text, secret) {
			t.Errorf("%q survived: %q", secret, res.Text)
		}
	}
}

func TestStringsMergesFindings(t *testing.T) {
	texts, res := New().Strings(
		"my key is sk-abcdefGHIJKL0123456789mnopqrst",
		"and my email is punit@example.com",
		"this one is clean",
	)
	if len(texts) != 3 {
		t.Fatalf("got %d texts", len(texts))
	}
	if texts[2] != "this one is clean" {
		t.Errorf("clean text changed: %q", texts[2])
	}
	if res.Found["openai-key"] != 1 || res.Found["email"] != 1 {
		t.Errorf("findings not merged: %v", res.Found)
	}
}

func TestEmptyAndCleanInput(t *testing.T) {
	if res := New().String(""); res.Any() || res.Text != "" {
		t.Errorf("empty input: %+v", res)
	}
	if got := Describe(New().String("nothing to see here")); got != "nothing masked" {
		t.Errorf("Describe = %q", got)
	}
}

// Describe must never leak the secret it is describing.
func TestDescribeNeverContainsTheSecret(t *testing.T) {
	res := New().String("key sk-abcdefGHIJKL0123456789mnopqrst and punit@example.com")
	desc := Describe(res)
	if strings.Contains(desc, "sk-abcdef") || strings.Contains(desc, "punit@") {
		t.Errorf("Describe leaked a secret: %q", desc)
	}
	if !strings.Contains(desc, "openai-key") || !strings.Contains(desc, "email") {
		t.Errorf("Describe should name the rules: %q", desc)
	}
}

func TestRedactionIsStable(t *testing.T) {
	in := "key sk-abcdefGHIJKL0123456789mnopqrst"
	once := New().String(in).Text
	twice := New().String(once).Text
	if once != twice {
		t.Errorf("redacting twice changed the text:\n  %q\n  %q", once, twice)
	}
}
