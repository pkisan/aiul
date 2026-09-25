package platform

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// A record as a fresh Windows 11 writes it: automatic detection on, no proxy.
var freshConnSettings = append([]byte{
	0x46, 0, 0, 0, 0x05, 0, 0, 0, 0x09, 0, 0, 0, // version, counter 5, direct+autodetect
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // empty proxy, bypass, PAC
}, make([]byte, 32)...)

func TestConnSettingsProxyOnAndOff(t *testing.T) {
	on := withProxy(freshConnSettings, winProxyServer("127.0.0.1:8899"), winBypass())
	c, ok := decodeConnSettings(on)
	if !ok {
		t.Fatal("could not read back the record we wrote")
	}
	if c.flags != connFlagDirect|connFlagAutoDetect|connFlagProxy {
		t.Errorf("flags %#x: want the person's automatic detection kept and the proxy added", c.flags)
	}
	if c.counter != 6 || c.proxy != "https=127.0.0.1:8899" || c.version != 0x46 {
		t.Errorf("got %+v", c)
	}
	if !bytes.Equal(c.rest, make([]byte, 32)) {
		t.Error("the unknown tail was not kept")
	}

	off, changed := withoutProxy(on)
	if !changed {
		t.Fatal("our proxy was not removed")
	}
	c, _ = decodeConnSettings(off)
	if c.flags != connFlagDirect|connFlagAutoDetect || c.proxy != "" || c.bypass != "" || c.counter != 7 {
		t.Errorf("after removal got %+v", c)
	}
}

// Someone else's proxy (a company one) is never removed.
func TestConnSettingsLeavesOtherProxiesAlone(t *testing.T) {
	theirs := connSettings{version: 0x46, flags: 3, proxy: "proxy.corp:3128", rest: make([]byte, 32)}.encode()
	if _, changed := withoutProxy(theirs); changed {
		t.Error("removed a proxy that is not ours")
	}
}

// Garbage or a missing record gives a fresh, valid one.
func TestConnSettingsFromNothing(t *testing.T) {
	c, ok := decodeConnSettings(withProxy([]byte{1, 2}, "https=127.0.0.1:8899", "<local>"))
	if !ok || c.flags != connFlagDirect|connFlagProxy || c.version != connVersion {
		t.Errorf("got %+v, ok=%v", c, ok)
	}
}

func TestWinProxyHost(t *testing.T) {
	for in, want := range map[string]string{
		"https=127.0.0.1:8899":          "127.0.0.1:8899",
		"http=a:1;https=127.0.0.1:8899": "127.0.0.1:8899",
		"proxy.corp:3128":               "proxy.corp:3128",
		"http=a:1":                      "",
	} {
		if got := winProxyHost(in); got != want {
			t.Errorf("winProxyHost(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFindPIDInTCPTable(t *testing.T) {
	row := func(state uint32, lport, rport int, pid uint32) []byte {
		be := func(p int) uint32 { return uint32(p>>8&0xff) | uint32(p&0xff)<<8 }
		var b []byte
		for _, v := range []uint32{state, 0x0100007f, be(lport), 0x0100007f, be(rport), pid} {
			b = binary.LittleEndian.AppendUint32(b, v)
		}
		return b
	}
	table := binary.LittleEndian.AppendUint32(nil, 3)
	table = append(table, row(5, 8899, 50123, 111)...) // the proxy's own end
	table = append(table, row(2, 50123, 8899, 222)...) // not established
	table = append(table, row(5, 50123, 8899, 333)...) // the client

	if pid := findPIDInTCPTable(table, 50123, 8899); pid != 333 {
		t.Errorf("pid %d, want 333", pid)
	}
	if pid := findPIDInTCPTable(table, 40000, 8899); pid != 0 {
		t.Errorf("pid %d for an unknown port, want 0", pid)
	}
	if pid := findPIDInTCPTable(table[:10], 50123, 8899); pid != 0 {
		t.Errorf("a truncated table gave pid %d", pid)
	}
}
