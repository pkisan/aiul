package proxy

import (
	"bufio"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"

	"github.com/pkisan/aiul/internal/parsers"
	"github.com/pkisan/aiul/internal/tasks"
)

// A Claude desktop cloud (Cowork) session sends its answers on one event stream
// that stays open for as long as the session is on screen — hours. Recording it
// when it closes would be far too late, and a crash would lose it, so each turn
// is recorded the moment it finishes.
//
// Rule 6 still holds: the chunks reach the client first, unchanged. We get a
// copy through a buffered channel and decode it on our own goroutine. If that
// goroutine falls behind and the channel fills, we stop copying rather than make
// the client wait: the session loses its answers, the user loses nothing.
type streamTurns struct {
	ch     chan []byte
	once   sync.Once
	broken bool
}

// ponytail: 1024 chunks of up to 32 KiB; a decoder that far behind is stuck.
const streamTurnsBacklog = 1024

func (p *Proxy) startStreamTurns(req *http.Request, resp *http.Response, host string, taskCtx tasks.Info) *streamTurns {
	s := &streamTurns{ch: make(chan []byte, streamTurnsBacklog)}
	go p.readStreamTurns(s.ch, req, resp, host, taskCtx)
	return s
}

// feed hands over one chunk the client has already received. Never blocks.
func (s *streamTurns) feed(chunk []byte) {
	if s.broken {
		return
	}
	select {
	case s.ch <- append([]byte(nil), chunk...):
	default:
		// A gap would corrupt the compressed stream, so stop here for good.
		s.broken = true
		s.close()
	}
}

func (s *streamTurns) close() { s.once.Do(func() { close(s.ch) }) }

func (p *Proxy) readStreamTurns(ch <-chan []byte, req *http.Request, resp *http.Response, host string, taskCtx tasks.Info) {
	body, err := streamDecoder(&chanReader{ch: ch}, resp.Header.Get("Content-Encoding"))
	if err != nil {
		p.log.Debug("cannot read the session stream", "host", host, "err", err)
		for range ch { // keep the channel drained so feed never sees it full
		}
		return
	}

	var splitter parsers.CoworkTurns
	var data []string
	lines := bufio.NewReader(body)
	for {
		line, err := lines.ReadString('\n')
		line = strings.TrimRight(line, "\r\n")
		switch {
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		case line == "" && len(data) > 0:
			if turn := splitter.Add(strings.Join(data, "\n")); turn != nil {
				p.recordStreamTurn(turn, req, resp, host, taskCtx)
			}
			data = nil
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				p.log.Debug("session stream ended", "host", host, "err", err)
			}
			for range ch {
			}
			return
		}
	}
}

func (p *Proxy) recordStreamTurn(turn []string, req *http.Request, resp *http.Response, host string, taskCtx tasks.Info) {
	var body strings.Builder
	for _, d := range turn {
		body.WriteString("data: " + d + "\n\n")
	}
	head := http.Header{}
	head.Set("Content-Type", "text/event-stream")
	p.record(interaction{
		Host:          host,
		Method:        parsers.TurnMethod,
		Path:          req.URL.Path,
		Status:        resp.StatusCode,
		ResponseBytes: int64(body.Len()),
		ResponseCopy:  []byte(body.String()),
		RequestHeader: req.Header,
		ResponseHead:  head,
		Started:       time.Now(),
		Task:          taskCtx,
	})
}

// streamDecoder undoes the response's compression as the bytes arrive.
func streamDecoder(r io.Reader, encoding string) (io.Reader, error) {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "", "identity":
		return r, nil
	case "gzip", "x-gzip":
		return gzip.NewReader(r)
	case "br":
		return brotli.NewReader(r), nil
	case "zstd":
		return zstd.NewReader(r)
	default:
		return nil, errors.New("unsupported content-encoding " + encoding)
	}
}

// chanReader reads the chunks a channel delivers, in order, as one stream.
type chanReader struct {
	ch   <-chan []byte
	left []byte
}

func (c *chanReader) Read(p []byte) (int, error) {
	for len(c.left) == 0 {
		chunk, ok := <-c.ch
		if !ok {
			return 0, io.EOF
		}
		c.left = chunk
	}
	n := copy(p, c.left)
	c.left = c.left[n:]
	return n, nil
}
