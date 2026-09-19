package main

import (
	"fmt"
	"sync"
	"time"
)

// The helper answers "which process, and where is it working" in ONE reply,
// because asking twice would mean two round trips to a root process for every
// connection. The proxy's interface asks in two steps, so the directory is parked
// here between them.
//
// Entries are short-lived: a pid is reused eventually, and a stale directory would
// tag an event with the wrong task.
var (
	dirMu    sync.Mutex
	dirCache = map[int]dirEntry{}
)

type dirEntry struct {
	dir string
	// The checkout at that directory, read by the helper in the same reply. The
	// worker cannot read it itself, so this is the only copy it will get.
	repo   string
	branch string
	at     time.Time
}

const dirTTL = 60 * time.Second

func (f sourceFinder) remember(pid int, dir, repo, branch string) {
	dirMu.Lock()
	defer dirMu.Unlock()

	// Take the opportunity to drop anything stale, so this map cannot grow
	// without bound on a busy machine.
	for key, entry := range dirCache {
		if time.Since(entry.at) > dirTTL {
			delete(dirCache, key)
		}
	}

	dirCache[pid] = dirEntry{dir: dir, repo: repo, branch: branch, at: time.Now()}
}

func lookupRememberedDir(pid int) (string, error) {
	dirMu.Lock()
	defer dirMu.Unlock()

	entry, ok := dirCache[pid]
	if !ok || time.Since(entry.at) > dirTTL || entry.dir == "" {
		return "", fmt.Errorf("no working directory known for pid %d", pid)
	}

	return entry.dir, nil
}

// rememberedCheckout answers the proxy's Checkout hook from what the helper
// already told us about this directory. An empty answer means "not a checkout, or
// we were never told", and the event goes into the untagged bucket.
func rememberedCheckout(dir string) (repo, branch string) {
	dirMu.Lock()
	defer dirMu.Unlock()

	for _, entry := range dirCache {
		if entry.dir == dir && time.Since(entry.at) <= dirTTL {
			return entry.repo, entry.branch
		}
	}

	return "", ""
}
