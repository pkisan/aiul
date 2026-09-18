// Package tasks works out which piece of work an AI interaction belongs to.
//
// The chain, with no action required from the person using the tool:
//
//	the connection's source port
//	  -> the process that owns it        (internal/platform)
//	  -> that process's working directory
//	  -> the git branch checked out there
//	  -> the task ID in the branch name
//
// A branch called "feature/ABC-123-add-login" yields the task ID "ABC-123", which
// is already a row in the PM tool this module lives in.
package tasks

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// DefaultPattern matches the usual ticket shape: a short project key, a hyphen,
// digits — "ABC-123", "PROJ-9", "AIUL-4567".
//
// The two-to-six letter limit is what keeps ordinary branch names out. Without it,
// "release-2026" reads as the ticket RELEASE-2026 and "v1-2-3" as V1-2, and an
// event gets attached to a task that does not exist. Six letters covers every
// project key we have seen; a team needing longer configures its own pattern.
var DefaultPattern = regexp.MustCompile(`\b[A-Z]{2,6}-\d+\b`)

// Info is what we worked out about one interaction's context.
type Info struct {
	// Dir is the working directory the tool was run from.
	Dir string

	// Repo is the root of the checkout, if the directory is inside one.
	Repo string

	// Branch is the checked-out branch, empty on a detached HEAD.
	Branch string

	// TaskID is the ticket extracted from the branch name, empty if there is none.
	// An event with no task ID goes into the "untagged" bucket rather than being
	// guessed at.
	TaskID string

	// Process is the program that opened the connection.
	Process string
}

// Resolver turns a directory into an Info, remembering what it finds.
type Resolver struct {
	pattern *regexp.Regexp
	ttl     time.Duration

	mu    sync.Mutex
	cache map[string]cached
}

type cached struct {
	info Info
	at   time.Time
}

// NewResolver returns a resolver using the default pattern.
func NewResolver() *Resolver { return NewResolverWith(DefaultPattern) }

// NewResolverWith returns a resolver using a custom pattern.
func NewResolverWith(pattern *regexp.Regexp) *Resolver {
	if pattern == nil {
		pattern = DefaultPattern
	}
	return &Resolver{
		pattern: pattern,
		// Short: someone switches branch while a tool is open, and the next
		// interaction should land on the new task.
		ttl:   30 * time.Second,
		cache: map[string]cached{},
	}
}

// Resolve reads the git state of dir. An empty or unreadable directory gives an
// empty Info rather than an error: not knowing the task is normal, not a failure.
func (r *Resolver) Resolve(dir string) Info {
	if dir == "" {
		return Info{}
	}

	r.mu.Lock()
	if hit, ok := r.cache[dir]; ok && time.Since(hit.at) < r.ttl {
		r.mu.Unlock()
		return hit.info
	}
	r.mu.Unlock()

	info := Info{Dir: dir}
	if repo, ok := findRepo(dir); ok {
		info.Repo = repo
		info.Branch = branchOf(repo)
		info.TaskID = r.TaskIDFrom(info.Branch)
	}

	r.mu.Lock()
	r.cache[dir] = cached{info: info, at: time.Now()}
	r.mu.Unlock()
	return info
}

// TaskIDFrom pulls the ticket out of a branch name.
func (r *Resolver) TaskIDFrom(branch string) string {
	if branch == "" {
		return ""
	}
	// Match against the upper-cased name so "feature/abc-123" is found too, but
	// return the ID in its canonical upper-case form.
	return r.pattern.FindString(strings.ToUpper(branch))
}

// findRepo walks up from dir looking for a .git entry, the way git itself does.
func findRepo(dir string) (string, bool) {
	current := filepath.Clean(dir)
	for i := 0; i < 40; i++ { // a depth limit, so a symlink loop cannot hang us
		if _, err := os.Stat(filepath.Join(current, ".git")); err == nil {
			return current, true
		}
		parent := filepath.Dir(current)
		if parent == current { // reached the filesystem root
			return "", false
		}
		current = parent
	}
	return "", false
}

// branchOf reads the checked-out branch by reading .git/HEAD directly, so no git
// binary is needed and nothing is executed.
//
// HEAD holds either a reference:
//
//	ref: refs/heads/feature/ABC-123-add-login
//
// or, on a detached HEAD, a bare commit hash — in which case there is no branch.
func branchOf(repo string) string {
	gitPath := filepath.Join(repo, ".git")

	info, err := os.Stat(gitPath)
	if err != nil {
		return ""
	}
	// In a worktree or a submodule, .git is a FILE pointing at the real directory.
	if !info.IsDir() {
		data, err := os.ReadFile(gitPath)
		if err != nil {
			return ""
		}
		target := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(data)), "gitdir:"))
		if target == "" {
			return ""
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(repo, target)
		}
		gitPath = target
	}

	head, err := os.ReadFile(filepath.Join(gitPath, "HEAD"))
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(head))
	ref, found := strings.CutPrefix(line, "ref: refs/heads/")
	if !found {
		return "" // detached HEAD: a commit hash, no branch
	}
	return ref
}
