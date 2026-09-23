package helper

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadClaudeAccount(t *testing.T) {
	dir := t.TempDir()
	write := func(body string) string {
		path := filepath.Join(dir, ".claude.json")
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}

	cases := map[string]struct{ body, want string }{
		"full name first": {`{"oauthAccount":{"fullName":"Alex John","displayName":"Alex","emailAddress":"a@x.com"}}`, "Alex John"},
		"display name":    {`{"oauthAccount":{"displayName":"Alex","emailAddress":"a@x.com"}}`, "Alex"},
		"email only":      {`{"oauthAccount":{"emailAddress":"a@x.com"}}`, "a@x.com"},
		"api key user":    {`{"numStartups":3}`, ""},
		"not json":        {`{`, ""},
	}
	for name, c := range cases {
		if got := readClaudeAccount(write(c.body)); got != c.want {
			t.Errorf("%s: got %q, want %q", name, got, c.want)
		}
	}

	if got := readClaudeAccount(filepath.Join(dir, "absent")); got != "" {
		t.Errorf("missing file: got %q", got)
	}
}

func TestClaudeCodeAccountIgnoresOtherPrograms(t *testing.T) {
	if got := ClaudeCodeAccount(os.Getpid(), "Cursor Helper", "/Applications/Cursor.app/x"); got != "" {
		t.Errorf("got %q for a program that is not Claude Code", got)
	}
}
