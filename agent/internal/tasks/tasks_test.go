package tasks

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
)

// newRepo makes a real git repository on a branch, so the tests exercise the same
// files git actually writes rather than a guess at their shape.
func newRepo(t *testing.T, branch string) string {
	t.Helper()
	dir := t.TempDir()

	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}

	// A commit is needed before a branch can be created.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"add", "."},
		{"commit", "-qm", "first"},
		{"checkout", "-qb", branch},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	return dir
}

func TestResolveFindsBranchAndTaskID(t *testing.T) {
	repo := newRepo(t, "feature/ABC-123-add-login")
	got := NewResolver().Resolve(repo)

	if got.Repo != repo {
		t.Errorf("repo = %q, want %q", got.Repo, repo)
	}
	if got.Branch != "feature/ABC-123-add-login" {
		t.Errorf("branch = %q", got.Branch)
	}
	if got.TaskID != "ABC-123" {
		t.Errorf("task = %q, want ABC-123", got.TaskID)
	}
}

// A tool is usually run from a subdirectory of the checkout, not its root.
func TestResolveWalksUpToTheRepository(t *testing.T) {
	repo := newRepo(t, "PROJ-42-fix-the-thing")
	sub := filepath.Join(repo, "app", "Http", "Controllers")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	got := NewResolver().Resolve(sub)
	if got.TaskID != "PROJ-42" {
		t.Errorf("task = %q, want PROJ-42", got.TaskID)
	}
	if got.Repo != repo {
		t.Errorf("repo = %q, want %q", got.Repo, repo)
	}
	if got.Dir != sub {
		t.Errorf("dir = %q, want the actual working directory %q", got.Dir, sub)
	}
}

func TestBranchWithNoTaskIDIsLeftUntagged(t *testing.T) {
	repo := newRepo(t, "refactor-the-parser")
	got := NewResolver().Resolve(repo)

	if got.Branch != "refactor-the-parser" {
		t.Errorf("branch = %q", got.Branch)
	}
	// Guessing would be worse than admitting we do not know.
	if got.TaskID != "" {
		t.Errorf("task = %q, want empty so the event lands in the untagged bucket", got.TaskID)
	}
}

func TestDirectoryThatIsNotARepository(t *testing.T) {
	got := NewResolver().Resolve(t.TempDir())
	if got.Repo != "" || got.Branch != "" || got.TaskID != "" {
		t.Errorf("got %+v, want everything empty", got)
	}
}

func TestMissingAndEmptyDirectories(t *testing.T) {
	r := NewResolver()
	if got := r.Resolve(""); got != (Info{}) {
		t.Errorf("empty dir gave %+v", got)
	}
	if got := r.Resolve("/no/such/directory/anywhere"); got.TaskID != "" {
		t.Errorf("missing dir gave %+v", got)
	}
}

func TestDetachedHeadHasNoBranch(t *testing.T) {
	repo := newRepo(t, "XYZ-9-thing")

	cmd := exec.Command("git", "checkout", "-q", "--detach", "HEAD")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("detach: %v: %s", err, out)
	}

	got := NewResolver().Resolve(repo)
	if got.Repo != repo {
		t.Errorf("a detached HEAD is still a repository: %+v", got)
	}
	if got.Branch != "" || got.TaskID != "" {
		t.Errorf("a detached HEAD has no branch: %+v", got)
	}
}

// git worktrees write a .git FILE rather than a directory, which is the case most
// likely to be got wrong.
func TestWorktreeIsSupported(t *testing.T) {
	repo := newRepo(t, "MAIN-1-base")
	worktree := filepath.Join(t.TempDir(), "wt")

	cmd := exec.Command("git", "worktree", "add", "-q", "-b", "WORK-77-side-branch", worktree)
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("git worktree unavailable: %v: %s", err, out)
	}

	got := NewResolver().Resolve(worktree)
	if got.Branch != "WORK-77-side-branch" {
		t.Errorf("branch = %q, want the worktree's own branch", got.Branch)
	}
	if got.TaskID != "WORK-77" {
		t.Errorf("task = %q", got.TaskID)
	}
}

func TestTaskIDFrom(t *testing.T) {
	r := NewResolver()
	cases := map[string]string{
		"ABC-123":                   "ABC-123",
		"feature/ABC-123-add-login": "ABC-123",
		"bugfix/PROJ-9":             "PROJ-9",
		"abc-123-lowercase":         "ABC-123", // matched case-insensitively
		"AIUL-4567/nested/branch":   "AIUL-4567",
		"main":                      "",
		"refactor-parser":           "",
		"release-2026":              "",
		"v1-2-3":                    "",
		"":                          "",
	}
	for branch, want := range cases {
		if got := r.TaskIDFrom(branch); got != want {
			t.Errorf("TaskIDFrom(%q) = %q, want %q", branch, got, want)
		}
	}
}

func TestCustomPattern(t *testing.T) {
	// A team using "#1234" instead of Jira keys.
	r := NewResolverWith(regexp.MustCompile(`#\d+`))
	if got := r.TaskIDFrom("fix/#4321-crash"); got != "#4321" {
		t.Errorf("got %q", got)
	}
	if got := r.TaskIDFrom("feature/ABC-123"); got != "" {
		t.Errorf("the default pattern should not apply: %q", got)
	}
}

// Switching branch mid-session must be picked up, so the cache has to expire.
func TestCacheExpiresSoTaskFollowsTheBranch(t *testing.T) {
	repo := newRepo(t, "FIRST-1-start")
	r := NewResolver()

	if got := r.Resolve(repo).TaskID; got != "FIRST-1" {
		t.Fatalf("task = %q", got)
	}

	cmd := exec.Command("git", "checkout", "-qb", "SECOND-2-continue")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("checkout: %v: %s", err, out)
	}

	// Still cached, so the old answer is expected here.
	if got := r.Resolve(repo).TaskID; got != "FIRST-1" {
		t.Errorf("within the cache window the old task is fine, got %q", got)
	}

	r.ttl = 0 // expire everything
	if got := r.Resolve(repo).TaskID; got != "SECOND-2" {
		t.Errorf("after the cache expires the task must follow the branch, got %q", got)
	}
}

// A repository whose HEAD cannot be read must say so. On macOS, ~/Desktop,
// ~/Documents and ~/Downloads are protected by TCC: a daemon without Full Disk
// Access can stat .git and is denied when it opens .git/HEAD, so every
// interaction in that checkout was recorded with an empty branch and no reason.
func TestAnUnreadableHeadIsReportedRatherThanSwallowed(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	head := filepath.Join(dir, ".git", "HEAD")
	if err := os.WriteFile(head, []byte("ref: refs/heads/main\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions, so this cannot be tested as root")
	}

	repo, branch, _, err := CheckoutAtVerbose(dir)

	if repo != dir {
		t.Errorf("repo = %q, want %q — the repository is still found", repo, dir)
	}
	if branch != "" {
		t.Errorf("branch = %q, want empty", branch)
	}
	if err == nil {
		t.Error("an unreadable HEAD must come back as an error, not as silence")
	}

	// A detached HEAD is NOT an error: there is genuinely no branch.
	detached := t.TempDir()
	if err := os.MkdirAll(filepath.Join(detached, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(detached, ".git", "HEAD"), []byte("9f2c1a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, branch, _, err := CheckoutAtVerbose(detached); branch != "" || err != nil {
		t.Errorf("detached HEAD = %q, err %v; want empty and no error", branch, err)
	}
}

// Every way of writing a clone URL must land on one key, or the PM tool sees one
// project as several. Credentials must never survive: HTTPS clones often carry a
// personal access token.
func TestNormaliseRemote(t *testing.T) {
	cases := map[string]string{
		"git@github.com:Acme/Shop.git":                      "github.com/Acme/Shop",
		"https://github.com/Acme/Shop.git":                  "github.com/Acme/Shop",
		"https://github.com/Acme/Shop/":                     "github.com/Acme/Shop",
		"https://dev:ghp_TOKEN@github.com/Acme/Shop.git":    "github.com/Acme/Shop",
		"https://x-access-token:p@ss@GitHub.com/Acme/Shop":  "github.com/Acme/Shop",
		"ssh://git@gitlab.example.com:2222/grp/sub/app.git": "gitlab.example.com/grp/sub/app",
		"git+ssh://git@bitbucket.org/team/app":              "bitbucket.org/team/app",
		// Not a shared project: another directory on disk.
		"/srv/repos/shop.git":    "",
		"../shop":                "",
		"file:///srv/repos/shop": "",
		`C:\repos\shop`:          "",
		"C:/repos/shop":          "",
		"":                       "",
		"https://github.com":     "",
	}
	for in, want := range cases {
		if got := NormaliseRemote(in); got != want {
			t.Errorf("NormaliseRemote(%q) = %q, want %q", in, got, want)
		}
	}
}

// origin wins over other remotes, whatever order the config lists them in.
func TestRemotePrefersOrigin(t *testing.T) {
	repo := t.TempDir()
	writeGit(t, repo, "HEAD", "ref: refs/heads/main\n")
	writeGit(t, repo, "config", `[core]
	url = https://not-a-remote.example/x
[remote "upstream"]
	url = git@github.com:upstream/shop.git
[remote "origin"]
	url = git@github.com:acme/shop.git
	fetch = +refs/heads/*:refs/remotes/origin/*
`)
	if _, _, remote := CheckoutAt(repo); remote != "github.com/acme/shop" {
		t.Errorf("remote = %q, want origin's", remote)
	}

	// No origin: the first remote.
	writeGit(t, repo, "config", "[remote \"fork\"]\n\turl = https://github.com/me/shop\n")
	if _, _, remote := CheckoutAt(repo); remote != "github.com/me/shop" {
		t.Errorf("remote = %q, want the first remote", remote)
	}
}

// A worktree's .git is a file; its own git directory has no config, and the
// remote lives in the main checkout's, reached through commondir.
func TestRemoteOfAWorktree(t *testing.T) {
	main := t.TempDir()
	writeGit(t, main, "config", "[remote \"origin\"]\n\turl = git@github.com:acme/shop.git\n")
	wtGit := filepath.Join(main, ".git", "worktrees", "hotfix")
	if err := os.MkdirAll(wtGit, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtGit, "HEAD"), []byte("ref: refs/heads/hotfix\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtGit, "commondir"), []byte("../..\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	wt := t.TempDir()
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: "+wtGit+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, branch, remote := CheckoutAt(wt)
	if branch != "hotfix" || remote != "github.com/acme/shop" {
		t.Errorf("branch = %q, remote = %q", branch, remote)
	}
}

func writeGit(t *testing.T, repo, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".git", name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
