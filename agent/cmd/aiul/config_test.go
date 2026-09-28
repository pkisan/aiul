package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// The installed agent gets its endpoint and token from this file and nowhere else,
// so the parser has to survive what a person actually types into it.
func TestReadAgentConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.conf")
	contents := `
# The backend this device reports to.
AIUL_ENDPOINT=https://aiul.example.com/api/aiul/events

  AIUL_DEVICE_TOKEN = "aiul_quoted_and_indented"

# A comment mentioning AIUL_ENDPOINT=https://wrong.example.com
NOT_A_PAIR
EMPTY=
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	got := readAgentConfig(path)

	if got["AIUL_ENDPOINT"] != "https://aiul.example.com/api/aiul/events" {
		t.Errorf("endpoint = %q", got["AIUL_ENDPOINT"])
	}
	if got["AIUL_DEVICE_TOKEN"] != "aiul_quoted_and_indented" {
		t.Errorf("token = %q — quotes and spaces must be tolerated", got["AIUL_DEVICE_TOKEN"])
	}
	if _, ok := got["NOT_A_PAIR"]; ok {
		t.Error("a line with no = must be ignored")
	}
	if _, ok := got["EMPTY"]; ok {
		t.Error("a key with no value must be ignored, not stored as empty")
	}
	if len(got) != 2 {
		t.Errorf("got %d settings, want 2: %v", len(got), got)
	}
}

// Running by hand there is no config file, and that is not an error.
func TestReadAgentConfigWithNoFile(t *testing.T) {
	if got := readAgentConfig(filepath.Join(t.TempDir(), "absent.conf")); len(got) != 0 {
		t.Errorf("a missing config file must read as empty, got %v", got)
	}
}

func TestFirstSet(t *testing.T) {
	if got := firstSet("", "", "third"); got != "third" {
		t.Errorf("got %q", got)
	}
	if got := firstSet("first", "second"); got != "first" {
		t.Errorf("got %q", got)
	}
	if got := firstSet("", ""); got != "" {
		t.Errorf("got %q", got)
	}
}

// A config file the worker cannot open reads as empty, not as a crash; the
// reason goes to stderr (the worker's log) instead of being swallowed.
func TestUnreadableAgentConfigIsEmpty(t *testing.T) {
	if os.Geteuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("root and Windows ignore the mode bits")
	}
	path := filepath.Join(t.TempDir(), "agent.conf")
	if err := os.WriteFile(path, []byte("AIUL_ENDPOINT=https://x/api/aiul/events\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	if got := readAgentConfig(path); len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}
}
