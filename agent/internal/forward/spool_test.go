package forward

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
)

type testEvent struct {
	ID     string `json:"id"`
	Prompt string `json:"prompt"`
}

func TestSpoolWritesOneFilePerEvent(t *testing.T) {
	s, err := NewSpool(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	s.Record(testEvent{ID: "a", Prompt: "first"})
	s.Record(testEvent{ID: "b", Prompt: "second"})

	files, err := s.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2", len(files))
	}

	// Files are named by time, so reading them in order gives the events in order.
	var got []testEvent
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var e testEvent
		if err := json.Unmarshal(data, &e); err != nil {
			t.Fatalf("spooled file is not valid JSON: %v", err)
		}
		got = append(got, e)
	}
	if got[0].ID != "a" || got[1].ID != "b" {
		t.Errorf("events out of order: %+v", got)
	}
}

func TestSpoolFilesAreOwnerOnly(t *testing.T) {
	s, _ := NewSpool(t.TempDir())
	s.Record(testEvent{ID: "x", Prompt: "a prompt may contain anything"})

	files, _ := s.Pending()
	info, err := os.Stat(files[0])
	if err != nil {
		t.Fatal(err)
	}
	// Prompts are sensitive: nobody else on the machine may read them.
	if mode := info.Mode().Perm(); mode&0o077 != 0 {
		t.Errorf("spooled event has mode %#o, want 0600", mode)
	}
}

func TestSpoolLeavesNoTemporaryFiles(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewSpool(dir)
	s.Record(testEvent{ID: "x"})

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("a half-written file was left behind: %s", e.Name())
		}
	}
}

func TestSpoolIsConcurrencySafe(t *testing.T) {
	s, _ := NewSpool(t.TempDir())
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); s.Record(testEvent{ID: "x"}) }()
	}
	wg.Wait()

	files, _ := s.Pending()
	if len(files) != 50 {
		t.Errorf("got %d files, want 50 — file names must not collide", len(files))
	}
}
