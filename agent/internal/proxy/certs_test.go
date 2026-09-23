package proxy

import (
	"crypto/x509"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/pkisan/aiul/internal/ca"
)

func testCache(t *testing.T) *CertCache {
	t.Helper()
	// A throwaway root in a temporary HOME, so no test ever touches the real CA.
	t.Setenv("HOME", t.TempDir())
	root, err := ca.Init(false)
	if err != nil {
		t.Fatalf("ca.Init: %v", err)
	}
	return NewCertCache(root)
}

func TestCertCacheReusesCertificates(t *testing.T) {
	c := testCache(t)

	first, err := c.Get("api.openai.com")
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.Get("api.openai.com:443") // same host, port stripped
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Error("the second Get should return the cached certificate, not mint a new one")
	}
	if c.Len() != 1 {
		t.Errorf("cache holds %d entries, want 1", c.Len())
	}

	// Only the host itself: never the provider's other names (rule 3).
	if got := first.Leaf.DNSNames; len(got) != 1 || got[0] != "api.openai.com" {
		t.Errorf("DNS names = %v, want exactly [api.openai.com]", got)
	}
}

// The bug this guards: leaves copied the provider's SANs, and any name outside
// the device intermediate's constraints made strict clients (macOS, Go, Cursor,
// Antigravity) reject the whole chain. A leaf minted under a constrained
// intermediate must verify, the way a real client checks it.
func TestLeafVerifiesUnderNameConstrainedIntermediate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root, err := ca.Init(false)
	if err != nil {
		t.Fatalf("ca.Init: %v", err)
	}
	inter, _, err := ca.ProvisionDevice(root, "test-device", []string{"api2.cursor.sh"})
	if err != nil {
		t.Fatalf("ProvisionDevice: %v", err)
	}
	cert, err := NewCertCache(inter).Get("api2.cursor.sh")
	if err != nil {
		t.Fatal(err)
	}

	roots, inters := x509.NewCertPool(), x509.NewCertPool()
	roots.AddCert(root.Cert)
	inters.AddCert(inter.Cert)
	if _, err := cert.Leaf.Verify(x509.VerifyOptions{
		DNSName: "api2.cursor.sh", Roots: roots, Intermediates: inters,
	}); err != nil {
		t.Fatalf("leaf does not verify under the constrained intermediate: %v", err)
	}
}

func TestCertCacheSeparatesHosts(t *testing.T) {
	c := testCache(t)
	a, _ := c.Get("api.openai.com")
	b, _ := c.Get("api.anthropic.com")
	if a == b {
		t.Fatal("different hosts must get different certificates")
	}
	if a.Leaf.DNSNames[0] == b.Leaf.DNSNames[0] {
		t.Error("certificates claim the same name")
	}
	if c.Len() != 2 {
		t.Errorf("cache holds %d entries, want 2", c.Len())
	}
}

func TestCertCacheRejectsBadHost(t *testing.T) {
	c := testCache(t)
	if _, err := c.Get(""); err == nil {
		t.Error("an empty host must be rejected")
	}
}

func TestCertCacheReplacesExpiringCertificates(t *testing.T) {
	c := testCache(t)

	// Put a certificate that expires in a minute into the cache by hand, which is
	// inside the renewal window.
	expiring, err := c.issuer.MintLeaf(ca.LeafRequest{
		Hosts:     []string{"api.openai.com"},
		NotBefore: time.Now().Add(-2 * time.Hour),
		NotAfter:  time.Now().Add(1 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	c.entries["api.openai.com"] = &cacheEntry{cert: expiring, lastUsed: time.Now()}

	fresh, err := c.Get("api.openai.com")
	if err != nil {
		t.Fatal(err)
	}
	if fresh == expiring {
		t.Error("a certificate inside the renewal window must be replaced, not served")
	}
	if time.Until(fresh.Leaf.NotAfter) < renewBefore {
		t.Error("the replacement is itself already inside the renewal window")
	}
}

func TestCertCacheIsBounded(t *testing.T) {
	c := testCache(t)
	c.max = 5

	for i := 0; i < 20; i++ {
		if _, err := c.Get(fmt.Sprintf("host%d.example.com", i)); err != nil {
			t.Fatal(err)
		}
	}
	if c.Len() > c.max {
		t.Errorf("cache grew to %d entries, bound is %d", c.Len(), c.max)
	}
}

// Run with -race: GetCertificate is called from many handshakes at once.
func TestCertCacheIsConcurrencySafe(t *testing.T) {
	c := testCache(t)
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := c.Get(fmt.Sprintf("host%d.example.com", i%4)); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
}
