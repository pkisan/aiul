package proxy

import (
	"bufio"
	"bytes"
	"compress/flate"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/pkisan/aiul/internal/parsers"
	"github.com/pkisan/aiul/internal/tasks"
)

// A WebSocket starts as an HTTP request that the server answers with
// "101 Switching Protocols". From then on the connection carries FRAMES, not
// HTTP: small binary headers, each followed by a piece of a message, in both
// directions at once.
//
// Codex talks to chatgpt.com/backend-api/codex/responses this way. Before this
// file existed the proxy read the first frame as if it were an HTTP request,
// failed ("malformed HTTP request") and dropped the connection; Codex then fell
// back to plain HTTP, which is the only reason any of it was recorded.
//
// What happens now, in line with rules 6 and 8:
//   - every frame is copied to the other side byte for byte, as soon as it
//     arrives, in both directions. Nothing is buffered, reordered or changed.
//   - on the side, we keep a copy of each message: unmasked (the client XORs
//     its frames with a random key) and, if the two ends agreed on compression,
//     inflated.
//   - for Codex, messages are grouped into turns: the client's
//     {"type":"response.create",...} up to the server's response.completed. Each
//     turn is handed to the existing pipeline as if it had been an HTTP request
//     answered with an event stream, so the Responses parser reads it unchanged.

// WebSocket opcodes (RFC 6455 section 5.2).
const (
	wsContinuation = 0x0
	wsText         = 0x1
	wsBinary       = 0x2
)

// relayWebSocket runs after the 101 has been written to the client. It returns
// when either side closes, and always closes both.
func (p *Proxy) relayWebSocket(req *http.Request, resp *http.Response,
	clientIn *bufio.Reader, clientOut net.Conn, upstreamIn *bufio.Reader, upstreamOut net.Conn,
	host string, taskCtx tasks.Info) {

	var turns *wsTurns
	if parsers.For(host, req.URL.Path) != nil {
		turns = &wsTurns{p: p, host: host, path: req.URL.Path, method: req.Method,
			reqHead: req.Header, status: resp.StatusCode, task: taskCtx}
	}

	// What the server agreed to, not what the client offered.
	ext := strings.ToLower(resp.Header.Get("Sec-WebSocket-Extensions"))
	deflate := strings.Contains(ext, "permessage-deflate")

	var wg sync.WaitGroup
	pump := func(src *bufio.Reader, dst net.Conn, fromClient bool, noTakeover bool, max int) {
		defer wg.Done()
		r := &wsReader{deflate: deflate, noTakeover: noTakeover, max: max}
		_ = r.pump(src, dst, func(msg []byte) {
			if turns != nil {
				turns.message(fromClient, msg)
			}
		})
		// One side is gone; closing both unblocks the other pump.
		_ = clientOut.Close()
		_ = upstreamOut.Close()
	}

	wg.Add(2)
	go pump(clientIn, upstreamOut, true, strings.Contains(ext, "client_no_context_takeover"), maxRequestCopyBytes)
	go pump(upstreamIn, clientOut, false, strings.Contains(ext, "server_no_context_takeover"), maxResponseCopyBytes)
	wg.Wait()

	if turns != nil {
		turns.finish() // a turn cut off by the connection closing is still a turn
	}
}

// wsReader forwards frames from one side and assembles that side's messages.
type wsReader struct {
	deflate    bool
	noTakeover bool
	max        int

	// The message being assembled across continuation frames.
	msg        bytes.Buffer
	compressed bool
	tooBig     bool

	// With context takeover, each compressed message may refer back to the last
	// 32 KiB of what this side sent before, so we keep that window.
	window []byte
}

func (r *wsReader) pump(src *bufio.Reader, dst io.Writer, deliver func([]byte)) error {
	var head [14]byte
	for {
		// Two fixed bytes, then an extended length and a mask key if present.
		if _, err := io.ReadFull(src, head[:2]); err != nil {
			return err
		}
		fin := head[0]&0x80 != 0
		rsv1 := head[0]&0x40 != 0
		opcode := head[0] & 0x0f
		masked := head[1]&0x80 != 0

		n := 2
		length := uint64(head[1] & 0x7f)
		switch length {
		case 126:
			if _, err := io.ReadFull(src, head[n:n+2]); err != nil {
				return err
			}
			length = uint64(binary.BigEndian.Uint16(head[n : n+2]))
			n += 2
		case 127:
			if _, err := io.ReadFull(src, head[n:n+8]); err != nil {
				return err
			}
			length = binary.BigEndian.Uint64(head[n : n+8])
			n += 8
		}
		var mask []byte
		if masked {
			if _, err := io.ReadFull(src, head[n:n+4]); err != nil {
				return err
			}
			mask = head[n : n+4]
			n += 4
		}

		isData := opcode == wsText || opcode == wsBinary || opcode == wsContinuation
		if isData && opcode != wsContinuation {
			r.msg.Reset()
			r.compressed = r.deflate && rsv1
			r.tooBig = false
		}
		// A message is noted the moment its last byte is READ, before that byte
		// is forwarded. Otherwise the two directions race: the client's next
		// "response.create" can be noted before the server's "response.completed"
		// that it answers, and a turn loses its answer. Noting is cheap (decode
		// and read one field); the recording itself happens elsewhere.
		finishes := isData && fin
		deliverNow := func() {
			if msg, ok := r.complete(); ok {
				deliver(msg)
			}
		}
		if finishes && length == 0 {
			deliverNow() // the header is the message's last byte
		}

		// The header goes on at once; the payload follows as it arrives.
		if _, err := dst.Write(head[:n]); err != nil {
			return err
		}

		// Stream the payload through, keeping a copy of data frames only.
		offset := uint64(0)
		buf := make([]byte, 32*1024)
		for offset < length {
			chunk := buf
			if left := length - offset; left < uint64(len(chunk)) {
				chunk = chunk[:left]
			}
			got, err := io.ReadFull(src, chunk)
			if got > 0 {
				if isData && !r.tooBig {
					if r.msg.Len()+got > r.max {
						r.tooBig = true // honest gap: a partial message is not parsed
					} else {
						start := r.msg.Len()
						r.msg.Write(chunk[:got])
						if mask != nil {
							b := r.msg.Bytes()[start:]
							for i := range b {
								b[i] ^= mask[(offset+uint64(i))%4]
							}
						}
					}
				}
				offset += uint64(got)
				if finishes && offset == length {
					deliverNow()
				}
				if _, werr := dst.Write(chunk[:got]); werr != nil {
					return werr
				}
			}
			if err != nil {
				return err
			}
		}
		flush(dst) // rule 6: this frame is with the other side now
	}
}

// complete returns the finished message, inflated when it was compressed.
func (r *wsReader) complete() ([]byte, bool) {
	if r.tooBig {
		return nil, false
	}
	data := append([]byte(nil), r.msg.Bytes()...)
	if !r.compressed {
		return data, true
	}

	// permessage-deflate (RFC 7692) strips the 00 00 ff ff that ends a sync
	// flush; put it back so the inflater sees a complete block, and start from
	// the window this side built up earlier.
	fr := flate.NewReaderDict(io.MultiReader(bytes.NewReader(data), bytes.NewReader([]byte{0, 0, 0xff, 0xff})), r.window)
	out, err := io.ReadAll(fr)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, false
	}
	if !r.noTakeover {
		r.window = append(r.window, out...)
		if len(r.window) > 32*1024 {
			r.window = append([]byte(nil), r.window[len(r.window)-32*1024:]...)
		}
	}

	return out, true
}

// wsTurns groups a Responses-over-WebSocket conversation into turns.
type wsTurns struct {
	p       *Proxy
	host    string
	path    string
	method  string
	reqHead http.Header
	status  int
	task    tasks.Info

	mu        sync.Mutex
	open      bool
	started   time.Time
	request   []byte
	events    bytes.Buffer
	respBytes int64
}

func (t *wsTurns) message(fromClient bool, msg []byte) {
	var head struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(msg, &head) != nil {
		return // not a JSON message; nothing we parse
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if fromClient {
		if head.Type == "response.create" {
			t.finishLocked() // the previous turn, if it never completed
			t.open = true
			t.started = time.Now()
			t.request = msg
		}
		return
	}

	if !t.open {
		return
	}
	// Written as event-stream lines, the framing the parsers already read.
	t.events.WriteString("data: ")
	t.events.Write(msg)
	t.events.WriteString("\n\n")
	t.respBytes += int64(len(msg))

	switch head.Type {
	case "response.completed", "response.failed", "response.incomplete", "error":
		t.finishLocked()
	}
}

func (t *wsTurns) finish() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.finishLocked()
}

func (t *wsTurns) finishLocked() {
	if !t.open {
		return
	}
	t.open = false

	head := http.Header{}
	head.Set("Content-Type", "text/event-stream")
	// Parsing, redaction and the sink run off the relay's goroutines, so a turn
	// being recorded never holds back the next frame.
	go t.p.record(interaction{
		Host:          t.host,
		Method:        t.method,
		Path:          t.path,
		Status:        t.status,
		RequestBytes:  int64(len(t.request)),
		ResponseBytes: t.respBytes,
		RequestCopy:   t.request,
		ResponseCopy:  append([]byte(nil), t.events.Bytes()...),
		RequestHeader: t.reqHead,
		ResponseHead:  head,
		Started:       t.started,
		Duration:      time.Since(t.started),
		Task:          t.task,
	})
	t.request = nil
	t.events.Reset()
	t.respBytes = 0
}
