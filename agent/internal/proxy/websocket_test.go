package proxy

import (
	"bufio"
	"bytes"
	"compress/flate"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// A recorded Codex conversation over a WebSocket, two turns, 36 messages.
type wsFixtureLine struct {
	From    string `json:"from"`
	Payload string `json:"payload"`
}

func loadWSFixture(t *testing.T) []wsFixtureLine {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/openai/codex-responses.ws.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	var lines []wsFixtureLine
	for _, l := range bytes.Split(bytes.TrimSpace(raw), []byte("\n")) {
		var f wsFixtureLine
		if err := json.Unmarshal(l, &f); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, f)
	}
	return lines
}

// writeFrame writes one frame. mask is nil for the server, 4 bytes for a client.
func writeFrame(w io.Writer, fin, rsv1 bool, opcode byte, payload, mask []byte) error {
	var b bytes.Buffer
	first := opcode
	if fin {
		first |= 0x80
	}
	if rsv1 {
		first |= 0x40
	}
	b.WriteByte(first)
	maskBit := byte(0)
	if mask != nil {
		maskBit = 0x80
	}
	switch n := len(payload); {
	case n < 126:
		b.WriteByte(maskBit | byte(n))
	case n < 1<<16:
		b.WriteByte(maskBit | 126)
		binary.Write(&b, binary.BigEndian, uint16(n))
	default:
		b.WriteByte(maskBit | 127)
		binary.Write(&b, binary.BigEndian, uint64(n))
	}
	if mask != nil {
		b.Write(mask)
		masked := make([]byte, len(payload))
		for i := range payload {
			masked[i] = payload[i] ^ mask[i%4]
		}
		payload = masked
	}
	b.Write(payload)
	_, err := w.Write(b.Bytes())
	return err
}

// readMessage reads frames until a FIN data frame; returns the raw (unmasked,
// still compressed) payload and whether the first frame had RSV1 set.
func readMessage(r *bufio.Reader) ([]byte, bool, error) {
	var msg []byte
	rsv1 := false
	first := true
	for {
		h := make([]byte, 2)
		if _, err := io.ReadFull(r, h); err != nil {
			return nil, false, err
		}
		if first {
			rsv1 = h[0]&0x40 != 0
			first = false
		}
		n := uint64(h[1] & 0x7f)
		if n == 126 {
			var x uint16
			binary.Read(r, binary.BigEndian, &x)
			n = uint64(x)
		} else if n == 127 {
			binary.Read(r, binary.BigEndian, &n)
		}
		var mask []byte
		if h[1]&0x80 != 0 {
			mask = make([]byte, 4)
			io.ReadFull(r, mask)
		}
		p := make([]byte, n)
		if _, err := io.ReadFull(r, p); err != nil {
			return nil, false, err
		}
		for i := range p {
			if mask != nil {
				p[i] ^= mask[i%4]
			}
		}
		msg = append(msg, p...)
		if h[0]&0x80 != 0 {
			return msg, rsv1, nil
		}
	}
}

func TestCodexOverWebSocketIsRelayedAndRecorded(t *testing.T) {
	for _, compress := range []bool{false, true} {
		t.Run(fmt.Sprintf("deflate=%t", compress), func(t *testing.T) {
			testCodexWebSocket(t, compress)
		})
	}
}

func testCodexWebSocket(t *testing.T, compress bool) {
	lines := loadWSFixture(t)
	root := newTestRoot(t)

	// What the server put on the wire, message by message, for comparison with
	// what the client receives.
	sent := make(chan []byte, len(lines))

	origin := newOriginServer(t, "chatgpt.com", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") != "websocket" {
			t.Errorf("no upgrade request")
			return
		}
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()

		ext := ""
		if compress {
			ext = "Sec-WebSocket-Extensions: permessage-deflate\r\n"
		}
		fmt.Fprintf(rw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: test\r\n%s\r\n", ext)
		rw.Flush()

		// One compressor for the whole connection: context takeover, the harder case.
		var zbuf bytes.Buffer
		zw, _ := flate.NewWriter(&zbuf, flate.DefaultCompression)

		for i, l := range lines {
			if l.From == "client" {
				got, _, err := readMessage(rw.Reader)
				if err != nil {
					t.Errorf("origin reading client message %d: %v", i, err)
					return
				}
				if string(got) != l.Payload {
					t.Errorf("the provider must receive the client's message unchanged (message %d)", i)
				}
				continue
			}

			payload := []byte(l.Payload)
			if compress {
				zbuf.Reset()
				zw.Write(payload)
				zw.Flush()
				payload = bytes.TrimSuffix(append([]byte(nil), zbuf.Bytes()...), []byte{0, 0, 0xff, 0xff})
			}
			sent <- payload

			// Split the big messages in two frames, to exercise continuation.
			if len(payload) > 1000 {
				half := len(payload) / 2
				writeFrame(rw, false, compress, wsText, payload[:half], nil)
				writeFrame(rw, true, false, wsContinuation, payload[half:], nil)
			} else {
				writeFrame(rw, true, compress, wsText, payload, nil)
			}
			rw.Flush()
		}
	}))
	defer origin.close()

	sink := &collector{}
	proxyAddr, _ := startProxy(t, Config{Issuer: root, Sink: sink, UpstreamRootCAs: origin.rootPool}, origin.addr)

	// The client: CONNECT through the proxy, TLS trusting our root, then the upgrade.
	raw, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	raw.SetDeadline(time.Now().Add(10 * time.Second))
	fmt.Fprintf(raw, "CONNECT chatgpt.com:443 HTTP/1.1\r\nHost: chatgpt.com:443\r\n\r\n")
	br := bufio.NewReader(raw)
	if resp, err := http.ReadResponse(br, nil); err != nil || resp.StatusCode != 200 {
		t.Fatalf("CONNECT: %v %v", resp, err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(root.Cert)
	// Nothing follows the CONNECT reply until our ClientHello, so br holds nothing.
	conn := tls.Client(raw, &tls.Config{ServerName: "chatgpt.com", RootCAs: pool, NextProtos: []string{"http/1.1"}})

	fmt.Fprintf(conn, "GET /backend-api/codex/responses HTTP/1.1\r\nHost: chatgpt.com\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n\r\n")
	cr := bufio.NewReader(conn)
	resp, err := http.ReadResponse(cr, nil)
	if err != nil || resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("upgrade: %v %v", resp, err)
	}

	received := 0
	for _, l := range lines {
		if l.From == "client" {
			if err := writeFrame(conn, true, false, wsText, []byte(l.Payload), []byte{1, 2, 3, 4}); err != nil {
				t.Fatal(err)
			}
			continue
		}
		got, _, err := readMessage(cr)
		if err != nil {
			t.Fatalf("client reading server message: %v", err)
		}
		if !bytes.Equal(got, <-sent) {
			t.Fatalf("server message %d reached the client changed", received)
		}
		received++
	}
	conn.Close()

	if !eventually(3*time.Second, func() bool { return len(sink.all()) == 1 }) {
		// The first turn is Codex's generate:false warm-up, which is not recorded.
		t.Fatalf("got %d events, want the one answered turn", len(sink.all()))
	}
	// Events are recorded asynchronously, so find the answered turn by content.
	var e Event
	for _, ev := range sink.all() {
		if strings.Contains(ev.Answer, "Footprints fade in mist") {
			e = ev
		}
	}
	if e.Answer == "" {
		t.Fatal("no event carries the second turn's answer")
	}
	if e.Parser != "openai" || e.Model == "" {
		t.Errorf("parser=%q model=%q", e.Parser, e.Model)
	}
	// Only what the person typed, not the instructions and history before it.
	if e.Prompt != "write a haiku about a proxy" {
		t.Errorf("prompt=%.120q, want only the last user message", e.Prompt)
	}
	// A follow-up turn carries previous_response_id and no tools of its own; it
	// is still the person typing, not the tool's housekeeping.
	if e.Kind != "human" {
		t.Errorf("kind=%q, want human", e.Kind)
	}
}
