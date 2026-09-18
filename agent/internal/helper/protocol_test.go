package helper

import (
	"errors"
	"strings"
	"testing"
)

// The parser is the entire attack surface of a root process, so most of these
// tests are about what it REFUSES.
func TestParseRequestAcceptsTheFourVerbs(t *testing.T) {
	cases := map[string]Request{
		"PING":          {Verb: VerbPing},
		"PROXY-ON":      {Verb: VerbProxyOn},
		"PROXY-OFF":     {Verb: VerbProxyOff},
		"PROCESS 54321": {Verb: VerbProcess, Port: 54321},
		"ping":          {Verb: VerbPing}, // case-insensitive verb
		"  PING  ":      {Verb: VerbPing},
	}

	for line, want := range cases {
		got, err := ParseRequest(line)
		if err != nil {
			t.Errorf("ParseRequest(%q) failed: %v", line, err)

			continue
		}
		if got != want {
			t.Errorf("ParseRequest(%q) = %+v, want %+v", line, got, want)
		}
	}
}

func TestParseRequestRefusesEverythingElse(t *testing.T) {
	// Each of these would be a way to make a root process do something it was
	// never meant to do.
	bad := []string{
		"",
		"   ",
		"EXEC /bin/sh",
		"PROXY-ON http://evil.example:8080", // no argument may redirect traffic
		"PROXY-ON extra",
		"PROXY-OFF anything",
		"PING PING",
		"PROCESS",                 // missing the port
		"PROCESS abc",             // not a number
		"PROCESS 0",               // out of range
		"PROCESS 65536",           // out of range
		"PROCESS -1",              // out of range
		"PROCESS 443 /etc/passwd", // extra argument
		"PROCESS 443; rm -rf /",   // shell metacharacters are just a bad argument
		"UNLINK /var/run/aiul.sock",
		"HELP",
	}

	for _, line := range bad {
		if req, err := ParseRequest(line); err == nil {
			t.Errorf("ParseRequest(%q) should have been refused, got %+v", line, req)
		}
	}
}

// An error message from a root process must not reflect back whatever was sent.
func TestUnknownVerbIsNotEchoedBack(t *testing.T) {
	_, err := ParseRequest("SOMETHING-ODD-AND-LONG")
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "ODD") {
		t.Errorf("the error echoes the input back: %q", err)
	}
}

func TestReplyRoundTrip(t *testing.T) {
	// A payload with spaces must survive: process names and paths both have them.
	line := FormatOK("1234", "Cursor Helper", "/Users/dev/my project")

	fields, err := ParseReply(line)
	if err != nil {
		t.Fatalf("ParseReply: %v", err)
	}
	if len(fields) != 3 || fields[1] != "Cursor Helper" || fields[2] != "/Users/dev/my project" {
		t.Errorf("fields = %q", fields)
	}
}

func TestEmptyOKReply(t *testing.T) {
	fields, err := ParseReply(FormatOK())
	if err != nil || len(fields) != 0 {
		t.Errorf("fields = %q, err = %v", fields, err)
	}
}

func TestErrorReply(t *testing.T) {
	_, err := ParseReply(FormatErr(errors.New("no process found")))
	if err == nil {
		t.Fatal("an ERR reply must come back as an error")
	}
	if !strings.Contains(err.Error(), "no process found") {
		t.Errorf("err = %v", err)
	}
}

// A newline inside an error would turn one reply into two and desynchronise the
// protocol, so it must be flattened.
func TestErrorRepliesStayOnOneLine(t *testing.T) {
	line := FormatErr(errors.New("first line\nsecond line"))
	if strings.Contains(line, "\n") {
		t.Errorf("the reply spans two lines: %q", line)
	}
}

func TestUnreadableReply(t *testing.T) {
	if _, err := ParseReply("something else entirely"); err == nil {
		t.Error("an unreadable reply must be an error")
	}
}
