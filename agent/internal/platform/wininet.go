package platform

import (
	"encoding/binary"
	"strconv"
	"strings"
)

// Windows keeps each person's proxy in two places under
// HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings:
//
//   - the plain values ProxyEnable, ProxyServer and ProxyOverride, and
//   - Connections\DefaultConnectionSettings, a binary record that WinINET — and
//     so Chrome and Edge — treat as the real setting.
//
// Writing only the plain values is known to be ignored once the binary record
// exists, so the agent writes both. This file reads and writes the record. It
// has no build tag so its test runs on any machine.
//
// Layout, all numbers little-endian 32-bit:
//
//	version | counter | flags | len+proxy | len+bypass | len+PAC URL | rest
//
// The counter goes up on every change. "rest" is kept byte for byte.
const (
	connFlagDirect     = 0x01
	connFlagProxy      = 0x02 // "Use a proxy server"
	connFlagAutoConfig = 0x04 // a setup script (PAC) URL
	connFlagAutoDetect = 0x08 // "Automatically detect settings"

	connVersion = 0x46 // what Windows 7 and later write
)

type connSettings struct {
	version, counter, flags uint32
	proxy, bypass, pac      string
	rest                    []byte
}

// decodeConnSettings reads the record. ok is false for anything malformed, in
// which case the caller starts from a fresh one.
func decodeConnSettings(b []byte) (c connSettings, ok bool) {
	u32 := func() (uint32, bool) {
		if len(b) < 4 {
			return 0, false
		}
		v := binary.LittleEndian.Uint32(b)
		b = b[4:]
		return v, true
	}
	str := func() (string, bool) {
		n, ok := u32()
		if !ok || uint32(len(b)) < n {
			return "", false
		}
		s := string(b[:n])
		b = b[n:]
		return s, true
	}

	if c.version, ok = u32(); !ok {
		return c, false
	}
	if c.counter, ok = u32(); !ok {
		return c, false
	}
	if c.flags, ok = u32(); !ok {
		return c, false
	}
	if c.proxy, ok = str(); !ok {
		return c, false
	}
	if c.bypass, ok = str(); !ok {
		return c, false
	}
	if c.pac, ok = str(); !ok {
		return c, false
	}
	c.rest = append([]byte(nil), b...)
	return c, true
}

func (c connSettings) encode() []byte {
	var out []byte
	u32 := func(v uint32) { out = binary.LittleEndian.AppendUint32(out, v) }
	str := func(s string) { u32(uint32(len(s))); out = append(out, s...) }

	u32(c.version)
	u32(c.counter)
	u32(c.flags)
	str(c.proxy)
	str(c.bypass)
	str(c.pac)
	return append(out, c.rest...)
}

// withProxy returns the record with our proxy switched on. Anything we do not
// own — automatic detection, a setup script — is left as the person had it.
func withProxy(existing []byte, server, bypass string) []byte {
	c, ok := decodeConnSettings(existing)
	if !ok {
		c = connSettings{version: connVersion, flags: connFlagDirect, rest: make([]byte, 32)}
	}
	c.counter++
	c.flags |= connFlagProxy
	c.proxy = server
	c.bypass = bypass
	return c.encode()
}

// withoutProxy switches the proxy off, but only when it is ours. The second
// result says whether anything changed.
func withoutProxy(existing []byte) ([]byte, bool) {
	c, ok := decodeConnSettings(existing)
	if !ok || !isOurProxyServer(c.proxy) {
		return existing, false
	}
	c.counter++
	c.flags &^= connFlagProxy
	c.proxy = ""
	c.bypass = ""
	return c.encode(), true
}

// winProxyServer is the ProxyServer value for our listener: HTTPS only, as on
// macOS. AI tools speak HTTPS; plain HTTP keeps going direct.
func winProxyServer(hostport string) string { return "https=" + hostport }

// winProxyHost reads the HTTPS proxy out of a ProxyServer value, which is either
// "host:port" for every protocol or "http=a:1;https=b:2" per protocol.
func winProxyHost(value string) string {
	if !strings.Contains(value, "=") {
		return value
	}
	for _, part := range strings.Split(value, ";") {
		if k, v, ok := strings.Cut(part, "="); ok && strings.EqualFold(strings.TrimSpace(k), "https") {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// isOurProxyServer marks a setting as ours; killswitch.ps1 uses the same test.
func isOurProxyServer(value string) bool { return strings.Contains(value, "127.0.0.1:8899") }

// winBypass is NoProxyList in WinINET's syntax: semicolons, wildcards rather
// than CIDR ranges, and <local> for any name without a dot.
//
// ponytail: 172.16.0.0/12 is written as its sixteen /16 prefixes, since WinINET
// has no CIDR; keep in step with NoProxyList by hand.
func winBypass() string {
	parts := []string{"<local>", "localhost", "127.0.0.1", "[::1]", "*.local", "*.test", "169.254.*", "10.*", "192.168.*"}
	for i := 16; i <= 31; i++ {
		parts = append(parts, "172."+strconv.Itoa(i)+".*")
	}
	return strings.Join(parts, ";")
}
