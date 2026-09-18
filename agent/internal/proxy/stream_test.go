package proxy

import (
	"bytes"
	"compress/gzip"
	"testing"
)

func TestDecompressGzip(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write([]byte(`{"prompt":"hello"}`))
	zw.Close()

	got, readable := decompress(buf.Bytes(), "gzip")
	if !readable {
		t.Fatal("gzip should be readable")
	}
	if string(got) != `{"prompt":"hello"}` {
		t.Errorf("got %q", got)
	}
}

func TestDecompressPassesThroughPlainBodies(t *testing.T) {
	for _, enc := range []string{"", "identity", "IDENTITY"} {
		got, readable := decompress([]byte("plain"), enc)
		if !readable || string(got) != "plain" {
			t.Errorf("encoding %q: got %q readable=%v", enc, got, readable)
		}
	}
}

func TestDecompressReportsUnsupportedEncodings(t *testing.T) {
	// br and zstd are not decoded yet: we must say so rather than store rubbish.
	for _, enc := range []string{"br", "zstd"} {
		got, readable := decompress([]byte("\x00\x01binary"), enc)
		if readable {
			t.Errorf("encoding %q should be reported as unreadable", enc)
		}
		if !bytes.Equal(got, []byte("\x00\x01binary")) {
			t.Errorf("the raw bytes should come back unchanged for %q", enc)
		}
	}
}

func TestDecompressHandlesCorruptData(t *testing.T) {
	if _, readable := decompress([]byte("not actually gzip"), "gzip"); readable {
		t.Error("corrupt gzip must be reported as unreadable, not crash")
	}
}

func TestParseSSE(t *testing.T) {
	body := ": keep-alive\n" +
		"event: message_start\n" +
		`data: {"type":"message_start"}` + "\n" +
		"\n" +
		`data: {"text":"Hello"}` + "\n" +
		"\n" +
		"data: line one\n" +
		"data: line two\n" +
		"\n" +
		"data: [DONE]\n\n"

	events := parseSSE([]byte(body))
	if len(events) != 4 {
		t.Fatalf("got %d events, want 4: %+v", len(events), events)
	}
	if events[0].Name != "message_start" || events[0].Data != `{"type":"message_start"}` {
		t.Errorf("event 0 = %+v", events[0])
	}
	if events[1].Data != `{"text":"Hello"}` {
		t.Errorf("event 1 = %+v", events[1])
	}
	// Multiple data lines in one event join with a newline, per the SSE spec.
	if events[2].Data != "line one\nline two" {
		t.Errorf("event 2 = %q", events[2].Data)
	}
	if events[3].Data != "[DONE]" {
		t.Errorf("event 3 = %+v", events[3])
	}
}

func TestParseSSEHandlesCRLFAndTruncation(t *testing.T) {
	events := parseSSE([]byte("data: a\r\n\r\ndata: b"))
	if len(events) != 2 || events[0].Data != "a" || events[1].Data != "b" {
		t.Errorf("events = %+v", events)
	}
}

func TestParseSSEOnEmptyBody(t *testing.T) {
	if got := parseSSE(nil); len(got) != 0 {
		t.Errorf("got %+v", got)
	}
}

func TestIsSSE(t *testing.T) {
	if !isSSE("text/event-stream; charset=utf-8") {
		t.Error("should detect SSE")
	}
	if isSSE("application/json") {
		t.Error("JSON is not SSE")
	}
}
