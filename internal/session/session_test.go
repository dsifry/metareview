package session

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// git runs a git command in dir and fails the test on error.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// fixture is the shape the bug was captured in: a session opened on the main checkout while its
// work happens in a sibling worktree of the same repository.
type fixture struct {
	main, worktree string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(base, "thread")
	if err := os.MkdirAll(main, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, main, "init", "-q", "-b", "main")
	git(t, main, "config", "user.email", "t@e")
	git(t, main, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(main, "base.go"), []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, main, "add", "base.go")
	git(t, main, "-c", "commit.gpgsign=false", "commit", "-qm", "base")
	wt := filepath.Join(base, "thread-keeper-mvp")
	git(t, main, "worktree", "add", "-q", "-b", "codex/keeper-mvp", wt)
	return fixture{main: main, worktree: wt}
}

const sid = "01a0de69-1277-7e72-a974-aa89e60b2eea" // the captured Codex session id

func TestUnboundSessionResolvesToTheHostCheckout(t *testing.T) {
	f := newFixture(t)
	got := Resolve(f.main, sid)
	if got.Dir != f.main || got.Bound || got.Warning != "" {
		t.Fatalf("unbound session: got %+v, want the host checkout %s", got, f.main)
	}
}

func TestResolveFromASubdirectoryReturnsTheCheckoutRoot(t *testing.T) {
	f := newFixture(t)
	deep := filepath.Join(f.main, "internal", "deep")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := Resolve(deep, ""); got.Dir != f.main {
		t.Fatalf("got %+v, want root %s", got, f.main)
	}
}

func TestBoundSessionResolvesToItsWorktree(t *testing.T) {
	f := newFixture(t)
	b, err := Bind(f.main, sid, f.worktree, "t@e", time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if b.Worktree != f.worktree || b.Branch != "codex/keeper-mvp" || b.BoundBy != "t@e" {
		t.Fatalf("binding recorded wrong: %+v", b)
	}
	// From the main checkout — where the host runs the hook — AND from any other worktree, since
	// the binding lives in the shared git directory.
	for _, from := range []string{f.main, f.worktree} {
		got := Resolve(from, sid)
		if got.Dir != f.worktree || !got.Bound || got.Warning != "" {
			t.Fatalf("resolve from %s: got %+v, want bound to %s", from, got, f.worktree)
		}
	}
	// Another session is unaffected: a binding is per session, not per repository.
	if got := Resolve(f.main, "other-session"); got.Dir != f.main || got.Bound {
		t.Fatalf("other session leaked the binding: %+v", got)
	}
}

func TestBindAcceptsASubdirectoryOfTheWorktree(t *testing.T) {
	f := newFixture(t)
	sub := filepath.Join(f.worktree, "docs")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := Bind(f.main, sid, sub, "", time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if b.Worktree != f.worktree {
		t.Fatalf("a path inside the worktree must bind the worktree root, got %s", b.Worktree)
	}
}

func TestBindRefusesADifferentRepository(t *testing.T) {
	f := newFixture(t)
	other := filepath.Join(filepath.Dir(f.main), "unrelated")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, other, "init", "-q", "-b", "main")
	_, err := Bind(f.main, sid, other, "", time.Unix(0, 0))
	if !errors.Is(err, ErrNotSameRepository) {
		t.Fatalf("binding to another repository must be refused, got %v", err)
	}
	if got := Resolve(f.main, sid); got.Bound {
		t.Fatalf("a refused bind must record nothing: %+v", got)
	}
}

func TestBindRefusesPathsThatAreNotCheckouts(t *testing.T) {
	f := newFixture(t)
	notGit := t.TempDir()
	for _, target := range []string{filepath.Join(f.main, "missing"), notGit, filepath.Join(f.main, "base.go")} {
		if _, err := Bind(f.main, sid, target, "", time.Unix(0, 0)); err == nil {
			t.Fatalf("bind to %s must fail", target)
		}
	}
}

func TestBindRefusesUnsafeSessionIDs(t *testing.T) {
	f := newFixture(t)
	for _, bad := range []string{"", "../escape", "a/b", ".hidden", strings.Repeat("x", 129), "sp ace", "new\nline"} {
		if _, err := Bind(f.main, bad, f.worktree, "", time.Unix(0, 0)); !errors.Is(err, ErrInvalidSessionID) {
			t.Fatalf("session id %q must be refused, got %v", bad, err)
		}
	}
}

func TestBindOutsideARepositoryFails(t *testing.T) {
	f := newFixture(t)
	if _, err := Bind(t.TempDir(), sid, f.worktree, "", time.Unix(0, 0)); err == nil {
		t.Fatal("bind from outside any repository must fail")
	}
}

func TestRebindReplacesTheBinding(t *testing.T) {
	f := newFixture(t)
	if _, err := Bind(f.main, sid, f.worktree, "", time.Unix(0, 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := Bind(f.main, sid, f.main, "", time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	if got := Resolve(f.main, sid); got.Dir != f.main || !got.Bound {
		t.Fatalf("rebind must win: %+v", got)
	}
}

func TestUnbindRemovesTheBinding(t *testing.T) {
	f := newFixture(t)
	if _, err := Bind(f.main, sid, f.worktree, "", time.Unix(0, 0)); err != nil {
		t.Fatal(err)
	}
	removed, err := Unbind(f.worktree, sid)
	if err != nil || !removed {
		t.Fatalf("unbind: removed=%v err=%v", removed, err)
	}
	if got := Resolve(f.main, sid); got.Bound || got.Dir != f.main {
		t.Fatalf("after unbind: %+v", got)
	}
	removed, err = Unbind(f.main, sid)
	if err != nil || removed {
		t.Fatalf("second unbind must report nothing removed: removed=%v err=%v", removed, err)
	}
	if _, err := Unbind(f.main, "../x"); !errors.Is(err, ErrInvalidSessionID) {
		t.Fatalf("unbind must validate the id, got %v", err)
	}
}

// A binding whose worktree was removed must not strand the session or silently pass it: it falls
// back to the host checkout — the pre-binding behaviour — and says why.
func TestStaleBindingFallsBackLoudly(t *testing.T) {
	f := newFixture(t)
	if _, err := Bind(f.main, sid, f.worktree, "", time.Unix(0, 0)); err != nil {
		t.Fatal(err)
	}
	git(t, f.main, "worktree", "remove", "--force", f.worktree)
	got := Resolve(f.main, sid)
	if got.Dir != f.main || got.Bound {
		t.Fatalf("stale binding must fall back to the host checkout: %+v", got)
	}
	if !strings.Contains(got.Warning, f.worktree) || !strings.Contains(got.Warning, "unbind") {
		t.Fatalf("the fallback must name the stale path and the way out: %q", got.Warning)
	}
}

// A binding file edited to point outside the repository is re-validated on every resolve, not
// trusted because it was valid once.
func TestTamperedBindingIsRevalidated(t *testing.T) {
	f := newFixture(t)
	if _, err := Bind(f.main, sid, f.worktree, "", time.Unix(0, 0)); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(filepath.Dir(f.main), "elsewhere")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, other, "init", "-q", "-b", "main")
	path := bindingPath(filepath.Join(f.main, ".git"), sid)
	if err := os.WriteFile(path, []byte(`{"schemaVersion":1,"sessionId":"`+sid+`","worktree":"`+other+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Resolve(f.main, sid)
	if got.Bound || got.Dir != f.main || got.Warning == "" {
		t.Fatalf("a binding to another repository must be rejected on resolve: %+v", got)
	}
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Resolve(f.main, sid); got.Bound || got.Dir != f.main || got.Warning == "" {
		t.Fatalf("an unreadable binding must fall back loudly: %+v", got)
	}
}

func TestResolveOutsideARepositoryReturnsTheStart(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got := Resolve(dir, sid); got.Dir != dir || got.Bound {
		t.Fatalf("outside a repository resolve changes nothing: %+v", got)
	}
}

func TestResolveIgnoresAnInvalidSessionID(t *testing.T) {
	f := newFixture(t)
	if got := Resolve(f.main, "../../etc"); got.Dir != f.main || got.Bound {
		t.Fatalf("an invalid id must never read a file: %+v", got)
	}
}

func TestBindResolvesARelativeTargetFromStart(t *testing.T) {
	f := newFixture(t)
	b, err := Bind(f.main, sid, filepath.Join("..", filepath.Base(f.worktree)), "", time.Unix(0, 0))
	if err != nil || b.Worktree != f.worktree {
		t.Fatalf("relative target: %+v %v", b, err)
	}
}

// Write failures surface as errors and leave no binding behind.
func TestBindReportsWriteFailures(t *testing.T) {
	f := newFixture(t)
	common := filepath.Join(f.main, ".git")
	// A file where the metareview directory belongs: MkdirAll fails.
	if err := os.WriteFile(filepath.Join(common, "metareview"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Bind(f.main, sid, f.worktree, "", time.Unix(0, 0)); err == nil {
		t.Fatal("bind must fail when the sessions directory cannot be created")
	}
	if err := os.Remove(filepath.Join(common, "metareview")); err != nil {
		t.Fatal(err)
	}
	// A read-only sessions directory: the temp write fails.
	dir := filepath.Dir(bindingPath(common, sid))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	if _, err := Bind(f.main, sid, f.worktree, "", time.Unix(0, 0)); err == nil {
		t.Fatal("bind must fail when the binding cannot be written")
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A non-empty directory at the binding path: the rename fails, and the temp file is removed.
	if err := os.MkdirAll(filepath.Join(bindingPath(common, sid), "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Bind(f.main, sid, f.worktree, "", time.Unix(0, 0)); err == nil {
		t.Fatal("bind must fail when the binding cannot be renamed into place")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Fatalf("a failed rename left its temp file: %s", e.Name())
		}
	}
	// ...and a directory where the binding should be is reported by resolve, not trusted.
	if got := Resolve(f.main, sid); got.Bound || got.Warning == "" {
		t.Fatalf("an unreadable binding path must fall back loudly: %+v", got)
	}
	// ...and unbind reports that it could not remove it.
	if _, err := Unbind(f.main, sid); err == nil {
		t.Fatal("unbind must report a binding it could not remove")
	}
}

func TestUnbindOutsideARepositoryFails(t *testing.T) {
	if _, err := Unbind(t.TempDir(), sid); err == nil {
		t.Fatal("unbind from outside any repository must fail")
	}
}

func TestBindingForAnotherSessionIsNotUsed(t *testing.T) {
	f := newFixture(t)
	path := bindingPath(filepath.Join(f.main, ".git"), sid)
	if err := writeAtomic(path, []byte(`{"schemaVersion":1,"sessionId":"someone-else","worktree":"`+f.worktree+`"}`)); err != nil {
		t.Fatal(err)
	}
	if got := Resolve(f.main, sid); got.Bound || got.Warning == "" {
		t.Fatalf("a file naming another session must not bind this one: %+v", got)
	}
}

func TestGitOutTimesOut(t *testing.T) {
	old := gitDeadline
	gitDeadline = time.Nanosecond
	t.Cleanup(func() { gitDeadline = old })
	if _, err := gitOut(t.TempDir(), "version"); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("want a timeout, got %v", err)
	}
}

// An inherited GIT_DIR would answer for the wrong repository; it is removed, the rest kept.
func TestScrubGitEnv(t *testing.T) {
	got := scrubGitEnv([]string{"GIT_DIR=/x", "PATH=/bin", "GIT_WORK_TREE=/y", "GIT_INDEX_FILE=i", "GIT_COMMON_DIR=c", "GIT_PREFIX=p", "GIT_AUTHOR_NAME=a"})
	if strings.Join(got, ",") != "PATH=/bin,GIT_AUTHOR_NAME=a" {
		t.Fatalf("got %v", got)
	}
}

// A directory whose .git FILE points at the repository shares its common directory without being
// one of its worktrees. Only a worktree git itself lists is accepted — on bind and on resolve.
func TestBindRefusesAGitFileThatIsNotAListedWorktree(t *testing.T) {
	f := newFixture(t)
	fake := filepath.Join(filepath.Dir(f.main), "fake")
	if err := os.MkdirAll(fake, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fake, ".git"), []byte("gitdir: "+filepath.Join(f.main, ".git")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Bind(f.main, sid, fake, "", time.Unix(0, 0)); !errors.Is(err, ErrNotSameRepository) {
		t.Fatalf("an unlisted checkout must be refused, got %v", err)
	}
	if err := writeAtomic(bindingPath(filepath.Join(f.main, ".git"), sid), []byte(`{"schemaVersion":1,"sessionId":"`+sid+`","worktree":"`+fake+`"}`)); err != nil {
		t.Fatal(err)
	}
	if got := Resolve(f.main, sid); got.Bound || got.Warning == "" {
		t.Fatalf("an unlisted checkout must be rejected on resolve: %+v", got)
	}
}
