// Package forward holds the local event spool and, later, the forwarder that sends
// events to the backend.
//
// The spool is a directory of small JSON files, one per event. A file on disk is
// the simplest thing that survives a crash, a logout and a reboot: the proxy
// writes an event and forgets about it, and the forwarder deletes a file only
// after the backend has confirmed it. Nothing is lost if either side stops.
package forward

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/pkisan/aiul/internal/paths"
	"time"
)

// Spool writes one JSON file per event into a directory.
type Spool struct {
	dir string

	mu  sync.Mutex
	seq int
}

// DefaultDir is where events are spooled. Where that is depends on how the agent
// is running — see internal/paths.
func DefaultDir() (string, error) {
	return paths.SpoolDir()
}

// NewSpool creates the directory if needed and returns a spool writing into it.
func NewSpool(dir string) (*Spool, error) {
	if dir == "" {
		d, err := DefaultDir()
		if err != nil {
			return nil, err
		}
		dir = d
	}
	// 0700: events hold prompts, so only the owner may read them.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create spool directory %s: %w", dir, err)
	}
	return &Spool{dir: dir}, nil
}

// Dir returns the spool directory.
func (s *Spool) Dir() string { return s.dir }

// Record writes one event. It never returns an error to its caller, because the
// caller is the proxy: a failure to log must never disturb a user's request.
// Failures are counted and surfaced by `aiul doctor` instead.
func (s *Spool) Record(e any) {
	if err := s.write(e); err != nil {
		// Writing to stderr is the last resort; the agent's logger picks this up.
		fmt.Fprintf(os.Stderr, "aiul: could not spool event: %v\n", err)
	}
}

func (s *Spool) write(e any) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}

	// Sortable, unique file name: time first so the forwarder sends events roughly
	// in order, a counter to break ties within the same millisecond.
	s.mu.Lock()
	s.seq++
	name := fmt.Sprintf("%s-%04d.json", time.Now().UTC().Format("20060102T150405.000"), s.seq)
	s.mu.Unlock()

	final := filepath.Join(s.dir, name)
	tmp := final + ".tmp"

	// Write to a temporary name and rename into place, so the forwarder never
	// reads a half-written file. Rename within one directory is atomic.
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}

// Pending lists the spooled event files, oldest first.
func (s *Spool) Pending() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			out = append(out, filepath.Join(s.dir, e.Name()))
		}
	}
	return out, nil // ReadDir returns entries sorted by name, which is time order
}
