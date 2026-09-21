package proxy

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"io"
	"strings"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

// This file works on OUR COPY of a body only. The client and the provider always
// receive the original bytes, untouched — rule 8. Nothing here can affect them.

// decompress returns readable bytes for a body that was sent compressed.
//
// gzip and deflate are handled by the standard library. Brotli and zstd need a
// third-party package; until one is added (D6 below), we return the raw bytes and
// report that they are unreadable, so the event records "we saw this exchange but
// could not read it" rather than storing rubbish.
func decompress(body []byte, contentEncoding string) (out []byte, readable bool) {
	switch strings.ToLower(strings.TrimSpace(contentEncoding)) {
	case "", "identity":
		return body, true

	case "gzip", "x-gzip":
		r, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return body, false
		}
		defer r.Close()
		// A truncated copy is normal: we cap what we keep. Take what decodes.
		got, err := io.ReadAll(r)
		if len(got) == 0 && err != nil {
			return body, false
		}
		return got, true

	case "deflate":
		r, err := zlib.NewReader(bytes.NewReader(body))
		if err != nil {
			return body, false
		}
		defer r.Close()
		got, err := io.ReadAll(r)
		if len(got) == 0 && err != nil {
			return body, false
		}
		return got, true

	case "br":
		// Brotli and zstd were left undecoded until 2026-09-21 (D12), on the
		// evidence that no real capture had ever needed them. claude.ai then
		// started answering with zstd, which would have reached a parser as
		// rubbish and produced an event with an empty answer — the failure that
		// looks like success. See D15.
		got, err := io.ReadAll(brotli.NewReader(bytes.NewReader(body)))
		if len(got) == 0 && err != nil {
			return body, false
		}

		return got, true

	case "zstd":
		r, err := zstd.NewReader(bytes.NewReader(body))
		if err != nil {
			return body, false
		}
		defer r.Close()

		got, err := io.ReadAll(r)
		if len(got) == 0 && err != nil {
			return body, false
		}

		return got, true

	default:
		return body, false
	}
}

// SSEEvent is one message from a Server-Sent Events stream.
//
// SSE is the format every streaming AI API uses. The provider sends a series of
// small text blocks separated by blank lines:
//
//	event: content_block_delta
//	data: {"delta":{"text":"Hello"}}
//
//	data: {"delta":{"text":" world"}}
//
//	data: [DONE]
//
// Each block is one event. Reassembling them is how we turn a stream of fragments
// back into the single answer the user actually saw.
type SSEEvent struct {
	Name string // from an "event:" line, often empty
	Data string // the joined "data:" lines
}

// parseSSE splits a Server-Sent Events body into its events, in order.
//
// The format is defined by lines: a field name, a colon, an optional space, then
// the value. A blank line ends the current event. A line starting with ":" is a
// comment (providers use these as keep-alives) and is ignored.
func parseSSE(body []byte) []SSEEvent {
	var events []SSEEvent
	var name string
	var data []string

	flushEvent := func() {
		if len(data) == 0 && name == "" {
			return
		}
		events = append(events, SSEEvent{Name: name, Data: strings.Join(data, "\n")})
		name, data = "", nil
	}

	// Normalise line endings; some servers use CRLF.
	for _, raw := range strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n") {
		line := strings.TrimSuffix(raw, "\r")

		switch {
		case line == "":
			flushEvent()
		case strings.HasPrefix(line, ":"):
			// comment / keep-alive, ignore
		default:
			field, value, found := strings.Cut(line, ":")
			if !found {
				// A bare field name with no value. Nothing useful to us.
				continue
			}
			value = strings.TrimPrefix(value, " ")
			switch field {
			case "data":
				data = append(data, value)
			case "event":
				name = value
			}
			// "id" and "retry" exist in the spec but carry nothing we log.
		}
	}
	flushEvent()
	return events
}

// isSSE reports whether a content type marks a Server-Sent Events stream.
func isSSE(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "text/event-stream")
}
