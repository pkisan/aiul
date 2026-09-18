package proxy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"time"
)

// maxCopyBytes caps how much of a body we keep for logging. Streams can be large,
// and our copy lives in memory. Past this we keep the head and note the truncation;
// the client still receives every byte.
const maxCopyBytes = 4 << 20 // 4 MiB

// forward sends one request upstream and streams the response back.
//
// Rule 6 is the point of this function: bytes are written to the client and
// flushed as they arrive, never buffered until the response is complete. The
// logging copy is taken on the side and never delays the user.
//
// Rule 8's other half: the request we send upstream is byte-for-byte what the
// client sent. Redaction happens later, on our copy only.
func (p *Proxy) forward(req *http.Request, client io.Writer, upstream *tls.Conn, upstreamBuf *bufio.Reader, host string, connStarted time.Time) error {
	started := time.Now()

	// Tee the request body: the original goes upstream, a bounded copy comes to us.
	var reqCopy bytes.Buffer
	if req.Body != nil {
		req.Body = readCloser{
			Reader: io.TeeReader(req.Body, limitedWriter{w: &reqCopy, max: maxCopyBytes}),
			Closer: req.Body,
		}
	}

	// Write returns the request exactly as received, preserving the original
	// request URI, so the provider sees what the client sent.
	if err := req.Write(upstream); err != nil {
		return fmt.Errorf("write request upstream: %w", err)
	}

	resp, err := http.ReadResponse(upstreamBuf, req)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	defer resp.Body.Close()

	// Status line and headers, unchanged, straight to the client.
	if err := writeResponseHead(client, resp); err != nil {
		return err
	}
	flush(client)

	var respCopy bytes.Buffer
	written, err := streamBody(client, resp, &respCopy)

	ev := interaction{
		Host:          host,
		Method:        req.Method,
		Path:          req.URL.Path,
		Status:        resp.StatusCode,
		RequestBytes:  req.ContentLength,
		ResponseBytes: written,
		RequestCopy:   reqCopy.Bytes(),
		ResponseCopy:  respCopy.Bytes(),
		RequestHeader: req.Header,
		ResponseHead:  resp.Header,
		Started:       started,
		Duration:      time.Since(started),
	}
	p.record(ev)

	if err != nil {
		return fmt.Errorf("stream response: %w", err)
	}
	if resp.Close || req.Close {
		return io.EOF
	}
	return nil
}

// streamBody copies the response to the client chunk by chunk, flushing after
// each one, while taking a bounded copy for logging.
func streamBody(client io.Writer, resp *http.Response, copyTo io.Writer) (int64, error) {
	// A chunked response must stay chunked on the wire, or the client cannot tell
	// where the body ends.
	var out io.Writer = client
	var chunked io.WriteCloser
	if isChunked(resp) {
		// httputil.NewChunkedWriter re-frames each Write as one HTTP chunk, which
		// is exactly the framing a streaming response needs.
		chunked = httputil.NewChunkedWriter(client)
		out = chunked
	}

	buf := make([]byte, 32*1024)
	var total int64
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			if _, err := out.Write(chunk); err != nil {
				return total, err
			}
			flush(client) // rule 6: the user sees this chunk now, not at the end
			_, _ = limitedWriter{w: copyTo, max: maxCopyBytes}.Write(chunk)
			total += int64(n)
		}
		if readErr != nil {
			if chunked != nil {
				// Close writes the final zero-length chunk, but NOT the blank line
				// that ends the message — httputil leaves that to the caller so
				// trailers can be written. Without it the client waits for ever.
				if err := chunked.Close(); err != nil {
					return total, err
				}
				if _, err := io.WriteString(client, "\r\n"); err != nil {
					return total, err
				}
				flush(client)
			}
			if readErr == io.EOF {
				return total, nil
			}
			return total, readErr
		}
	}
}

func writeResponseHead(w io.Writer, resp *http.Response) error {
	var b bytes.Buffer
	fmt.Fprintf(&b, "HTTP/%d.%d %d %s\r\n", resp.ProtoMajor, resp.ProtoMinor, resp.StatusCode, http.StatusText(resp.StatusCode))
	if err := resp.Header.Write(&b); err != nil {
		return err
	}
	// Transfer-Encoding is not part of resp.Header after parsing, so restore it.
	if isChunked(resp) {
		b.WriteString("Transfer-Encoding: chunked\r\n")
	}
	b.WriteString("\r\n")
	_, err := w.Write(b.Bytes())
	return err
}

func isChunked(resp *http.Response) bool {
	for _, te := range resp.TransferEncoding {
		if te == "chunked" {
			return true
		}
	}
	return false
}

// flush pushes bytes out of any buffering in the way. A *tls.Conn writes straight
// to the socket, but tests and future wrappers may buffer.
func flush(w io.Writer) {
	type flusher interface{ Flush() error }
	switch f := w.(type) {
	case http.Flusher:
		f.Flush()
	case flusher:
		_ = f.Flush()
	}
}

// limitedWriter writes at most max bytes and silently drops the rest. Used for the
// logging copy only, never for anything the client or provider receives.
type limitedWriter struct {
	w   io.Writer
	max int
}

func (l limitedWriter) Write(p []byte) (int, error) {
	type lener interface{ Len() int }
	if b, ok := l.w.(lener); ok {
		room := l.max - b.Len()
		if room <= 0 {
			return len(p), nil
		}
		if len(p) > room {
			p = p[:room]
		}
	}
	_, _ = l.w.Write(p)
	return len(p), nil
}

type readCloser struct {
	io.Reader
	io.Closer
}

// handshakeContext bounds a TLS handshake so a dead peer cannot hold a goroutine
// for ever.
func handshakeContext() context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	// The handshake finishes long before this fires; the cancel exists only to
	// release the timer.
	go func() { <-ctx.Done(); cancel() }()
	return ctx
}
