package parsers

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"io"
	"strings"
)

// Cursor parses the agent chat of the Cursor editor:
//
//	POST api2.cursor.sh/agent.v1.AgentService/RunSSE
//
// Cursor speaks Connect RPC, so the body is protobuf, not JSON — and there is no
// published schema. The field numbers below were read off a recorded answer
// (testdata/cursor, 2026-09-23). If Cursor renumbers them, this returns an empty
// prompt and answer rather than rubbish, and the event keeps its metadata.
//
// The response is labelled text/event-stream but is a Connect stream: frames of
// one flag byte, a 4-byte big-endian length, then a protobuf message. Flag bit 1
// means the frame is gzipped; bit 2 marks the end-of-stream trailer (JSON).
//
// The request body holds only a conversation id; the prompt comes back in the
// response. The model name travels in a separate BidiAppend request and is not
// joined in yet, so Model stays empty.
//
// Cursor only sends this through the proxy with these in its settings.json:
// "cursor.general.disableHttp2": true and "http.proxy" set — see docs/PROGRESS.md.
type Cursor struct{}

func (Cursor) Name() string { return "cursor" }

func (Cursor) Handles(host, path string) bool {
	return strings.HasSuffix(host, ".cursor.sh") && path == "/agent.v1.AgentService/RunSSE"
}

// Field paths inside each streamed message, as field numbers.
var (
	cursorPromptPath = []int{1, 6, 1, 1} // the user's message, echoed back
	cursorDeltaPath  = []int{1, 1, 1}    // a piece of the answer text
	cursorUsagePath  = []int{1, 14}      // token counts: 1 = input, 2 = output

	// Conversation checkpoints repeat the workspace (field 21.1) in two shapes.
	cursorWorkDirPaths = [][]int{{3, 21, 1}, {4, 3, 2, 21, 1}}
)

func (Cursor) Parse(ex Exchange) (Result, error) {
	res := Result{Tool: "cursor", Streamed: true, Kind: KindHuman}

	frames, err := connectFrames(ex.RespBody)
	var answer strings.Builder
	for _, msg := range frames {
		if s, ok := pbString(msg, cursorPromptPath); ok && res.Prompt == "" {
			res.Prompt = s
		}
		if s, ok := pbString(msg, cursorDeltaPath); ok {
			answer.WriteString(s)
		}
		for _, path := range cursorWorkDirPaths {
			if s, ok := pbString(msg, path); ok && res.WorkDir == "" {
				res.WorkDir = s
			}
		}
		if usage, ok := pbBytes(msg, cursorUsagePath); ok {
			res.PromptTokens = int(pbVarint(usage, 1))
			res.ResponseTokens = int(pbVarint(usage, 2))
		}
	}
	res.Answer = answer.String()

	// ponytail: every RunSSE with an echoed prompt is counted as a human turn.
	// Tool-result follow-ups have not been recorded yet; split them out when seen.
	if res.Prompt == "" {
		res.Kind = KindAgent
	}
	res.Automated = res.Kind != KindHuman
	return res, err
}

// connectFrames splits a Connect stream into its messages, unzipping gzipped
// frames and skipping the end-of-stream trailer. A truncated stream returns the
// frames read so far and an error.
func connectFrames(b []byte) ([][]byte, error) {
	var out [][]byte
	for len(b) > 0 {
		if len(b) < 5 {
			return out, errors.New("connect stream: truncated frame header")
		}
		flag, n := b[0], binary.BigEndian.Uint32(b[1:5])
		if uint64(len(b)-5) < uint64(n) {
			return out, errors.New("connect stream: truncated frame")
		}
		msg := b[5 : 5+n]
		b = b[5+n:]

		if flag&2 != 0 {
			continue // trailer, JSON metadata
		}
		if flag&1 != 0 {
			zr, err := gzip.NewReader(bytes.NewReader(msg))
			if err != nil {
				return out, err
			}
			if msg, err = io.ReadAll(zr); err != nil {
				return out, err
			}
		}
		out = append(out, msg)
	}
	return out, nil
}

// pbBytes follows a path of field numbers through nested protobuf messages and
// returns the first length-delimited value at the end of it. It is a tiny
// schema-less reader: enough to pull strings out, nothing more.
func pbBytes(msg []byte, path []int) ([]byte, bool) {
	for _, want := range path {
		found := false
		for f := range pbFieldsOf(msg) {
			if f.num == want && f.wire == 2 {
				msg, found = f.data, true
				break
			}
		}
		if !found {
			return nil, false
		}
	}
	return msg, true
}

func pbString(msg []byte, path []int) (string, bool) {
	b, ok := pbBytes(msg, path)
	return string(b), ok
}

// pbVarint returns the integer in field num of msg, or 0.
func pbVarint(msg []byte, num int) uint64 {
	for f := range pbFieldsOf(msg) {
		if f.num == num && f.wire == 0 {
			return f.value
		}
	}
	return 0
}

type pbField struct {
	num   int
	wire  int
	value uint64 // wire type 0
	data  []byte // wire type 2
}

// pbFieldsOf yields the top-level fields of one protobuf message and stops at
// the first malformed byte.
func pbFieldsOf(b []byte) func(yield func(pbField) bool) {
	return func(yield func(pbField) bool) {
		for len(b) > 0 {
			key, n := binary.Uvarint(b)
			if n <= 0 {
				return
			}
			b = b[n:]
			f := pbField{num: int(key >> 3), wire: int(key & 7)}
			switch f.wire {
			case 0:
				v, n := binary.Uvarint(b)
				if n <= 0 {
					return
				}
				f.value, b = v, b[n:]
			case 1:
				if len(b) < 8 {
					return
				}
				b = b[8:]
			case 2:
				l, n := binary.Uvarint(b)
				if n <= 0 || uint64(len(b)-n) < l {
					return
				}
				f.data, b = b[n:n+int(l)], b[n+int(l):]
			case 5:
				if len(b) < 4 {
					return
				}
				b = b[4:]
			default:
				return
			}
			if !yield(f) {
				return
			}
		}
	}
}
