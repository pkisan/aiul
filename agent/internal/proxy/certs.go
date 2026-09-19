package proxy

import (
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	"github.com/pkisan/aiul/internal/ca"
)

// defaultCacheSize is how many minted certificates we keep. Each one is a few
// hundred bytes; a real device talks to a handful of AI hosts, so this is
// generous. The bound exists so a strange client cannot make us mint without end.
const defaultCacheSize = 128

// renewBefore is how long before expiry we mint a replacement, so a connection
// never starts with a certificate that expires mid-handshake.
const renewBefore = 1 * time.Hour

// CertCache mints leaf certificates on demand and remembers them.
//
// Minting costs a key generation and a signature, which is milliseconds — but it
// happens inside the TLS handshake, on the path of every request, so caching keeps
// repeat connections to the same host fast.
type CertCache struct {
	issuer Issuer
	max    int

	mu      sync.Mutex
	entries map[string]*cacheEntry
}

type cacheEntry struct {
	cert     *tls.Certificate
	lastUsed time.Time
}

// Issuer is anything that can mint a leaf: the root CA when running by hand, or
// this device's name-constrained intermediate once the agent is installed (D3).
// Two implementations, both in internal/ca.
type Issuer interface {
	MintLeaf(ca.LeafRequest) (*tls.Certificate, error)
}

// NewCertCache returns a cache that signs with the given issuer.
func NewCertCache(issuer Issuer) *CertCache {
	return &CertCache{issuer: issuer, max: defaultCacheSize, entries: make(map[string]*cacheEntry)}
}

// Get returns a certificate valid for host, minting one if needed.
//
// sanNames are the names copied from the real provider's certificate, so our copy
// claims exactly what the genuine server claims. If it is empty we fall back to
// the hostname alone.
func (c *CertCache) Get(host string, sanNames []string) (*tls.Certificate, error) {
	key := normalizeHost(host)
	if key == "" {
		return nil, fmt.Errorf("cert cache: invalid host %q", host)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if e, ok := c.entries[key]; ok {
		if time.Until(e.cert.Leaf.NotAfter) > renewBefore {
			e.lastUsed = time.Now()
			return e.cert, nil
		}
		// Expired or nearly so: drop it and mint a fresh one below.
		delete(c.entries, key)
	}

	hosts := sanNames
	if len(hosts) == 0 {
		hosts = []string{key}
	}

	cert, err := c.issuer.MintLeaf(ca.LeafRequest{Hosts: hosts})
	if err != nil {
		return nil, fmt.Errorf("cert cache: %w", err)
	}

	c.evictIfFullLocked()
	c.entries[key] = &cacheEntry{cert: cert, lastUsed: time.Now()}
	return cert, nil
}

// evictIfFullLocked drops the least recently used entry when the cache is full.
// The caller must hold the lock.
//
// ponytail: linear scan over at most defaultCacheSize entries. A proper LRU list
// is only worth it if this cache ever grows to thousands of hosts, which would
// itself mean something is wrong with the allow-list.
func (c *CertCache) evictIfFullLocked() {
	if len(c.entries) < c.max {
		return
	}
	var oldestKey string
	var oldest time.Time
	for k, e := range c.entries {
		if oldestKey == "" || e.lastUsed.Before(oldest) {
			oldestKey, oldest = k, e.lastUsed
		}
	}
	delete(c.entries, oldestKey)
}

// Len reports how many certificates are cached, for tests and `aiul status`.
func (c *CertCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}
