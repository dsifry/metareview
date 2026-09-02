package status

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gateRepo builds a throwaway repo on a `work` branch off `main` with one committed change, so the
// branch scope resolves (base = main) and the branch's own head commit is in scope. It returns the
// root, a git runner, a file writer, and the work-branch head SHA.
func gateRepo(t *testing.T) (root string, run func(...string) string, write func(name, body string), head string) {
	t.Helper()
	root = t.TempDir()
	run = func(args ...string) string {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = root
		c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e")
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write = func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-q", "-b", "main")
	write("base.go", "package p\n")
	run("add", ".")
	run("commit", "-qm", "base")
	run("checkout", "-q", "-b", "work")
	write("work.go", "package p\nvar W = 1\n")
	run("add", ".")
	run("commit", "-qm", "work")
	head = run("rev-parse", "HEAD")
	return root, run, write, head
}

// gitCommitFiles must SURFACE a git failure, not swallow it: a caller that cannot list what a commit
// writes has to fail CLOSED, and an empty set would read as "nothing to gate" and wave the commit through.
func TestGitCommitFilesErrorsOnGitFailure(t *testing.T) {
	fail := func(string, ...string) ([]byte, error) { return nil, errors.New("boom") }
	if f, err := gitCommitFiles(fail, "/nope", ScopeStaged); f != nil || err == nil {
		t.Fatalf("a git error (staged) must return nil files and a non-nil error, got files=%v err=%v", f, err)
	}
	if f, err := gitCommitFiles(fail, "/nope", ScopeAll); f != nil || err == nil {
		t.Fatalf("a git error (all) must return nil files and a non-nil error, got files=%v err=%v", f, err)
	}
}

// A pathname with a leading space is a valid Unix path; the gate must see it VERBATIM. NUL-delimited git
// output plus no trimming preserves it — the old newline-split + TrimSpace turned " x.go" into "x.go", a
// different path the gate would then fail to match and thus fail to gate (CodeRabbit: branch.go:283).
func TestGitCommitFilesPreservesSpacePaths(t *testing.T) {
	root, run, _, _ := gateRepo(t)
	name := " space-prefixed.go" // leading space — unusual but legal, and exactly what quoting/trim would mangle
	if err := os.WriteFile(filepath.Join(root, name), []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "--", name)
	files, err := gitCommitFiles(realGit, root, ScopeStaged)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range files {
		if f == name {
			found = true
		}
	}
	if !found {
		t.Fatalf("a space-prefixed path must be preserved verbatim; got %q", files)
	}
}

// When the branch scope resolves but the commit's own file list cannot be produced, CommitGateScoped
// fails CLOSED — the same posture as an unresolvable scope. gitcontext (which ResolveBranchScope uses)
// runs real git, so a real repo plus a run seam that fails only `diff` reaches exactly this branch.
func TestCommitGateScopedFailsClosedWhenCommitFilesUnlistable(t *testing.T) {
	root, _, _, _ := gateRepo(t)
	stub := func(_ string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "diff" {
			return nil, errors.New("diff exploded")
		}
		return realGit(root, args...) // let rev-list and friends run for real so the scope resolves
	}
	blocked, msg, err := CommitGateScoped(root, "", ScopeStaged, stub)
	if err != nil {
		t.Fatal(err)
	}
	if !blocked || !strings.Contains(msg, "failing closed") {
		t.Fatalf("an unlistable commit file set must fail closed; blocked=%v msg=%q", blocked, msg)
	}
}

// A commit gate that cannot resolve the branch scope must fail CLOSED: not knowing what the branch
// is means it cannot vouch that the committed files were reviewed, so it blocks and says why.
func TestCommitGateScopedFailsClosedWhenScopeUnresolvable(t *testing.T) {
	blocked, msg, err := CommitGateScoped(t.TempDir(), "", ScopeStaged, nil) // not a git repo
	if err != nil {
		t.Fatal(err)
	}
	if !blocked || !strings.Contains(msg, "failing closed") {
		t.Fatalf("an unresolvable scope must fail closed; blocked=%v msg=%q", blocked, msg)
	}
}

// If the review store cannot be read, CommitGateScoped surfaces the error rather than silently
// treating the commit as reviewed. A `docs/metareview/reviews` that is a FILE (not a directory)
// makes os.ReadDir fail with a non-IsNotExist error — the exact "store is broken" case.
func TestCommitGateScopedSurfacesDiscoverError(t *testing.T) {
	root, run, write, _ := gateRepo(t)
	write("staged.go", "package p\nvar S = 1\n")
	run("add", "staged.go")
	if err := os.MkdirAll(filepath.Join(root, "docs", "metareview"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "metareview", "reviews"), []byte("not a dir\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := CommitGateScoped(root, "", ScopeStaged, nil); err == nil {
		t.Fatal("a broken review store must surface an error, not clear the commit")
	}
}

// A staged file that a passing review (of the branch head) already read clears the commit gate:
// coverage is by file, so the gate is a small check that never needs the whole-branch review.
func TestCommitGateScopedClearsWhenStagedFileReviewed(t *testing.T) {
	root, run, write, head := gateRepo(t)
	write("staged.go", "package p\nvar S = 1\n")
	run("add", "staged.go")
	mustWriteFile(t, filepath.Join(root, "docs", "metareview", "reviews", "r.md"),
		"# metareview: pr-ready review\n\nRun ID: `mrv-r`\nTarget: `current branch`\n\n## Verdict\n\nPASS\n")
	mustWriteFile(t, filepath.Join(root, ".metareview", "runs.jsonl"),
		`{"id":"mrv-r","scope":"pr-ready","verdict":"PASS","headSha":"`+head+`","coveredPaths":["staged.go"]}`+"\n")

	blocked, msg, err := CommitGateScoped(root, "", ScopeStaged, nil)
	if err != nil {
		t.Fatal(err)
	}
	if blocked || msg != "" {
		t.Fatalf("a staged file the branch-head review read must clear; blocked=%v msg=%q", blocked, msg)
	}
}

// When a base is given, the commit gate's fix-it message threads it through the review-prompt
// command so the operator reviews against the same base the gate measured.
func TestCommitGateScopedNamesBaseInMessage(t *testing.T) {
	root, run, write, _ := gateRepo(t)
	write("staged.go", "package p\nvar S = 1\n") // unreviewed
	run("add", "staged.go")
	blocked, msg, err := CommitGateScoped(root, "main", ScopeStaged, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !blocked || !strings.Contains(msg, "--base main") {
		t.Fatalf("the message must thread the base through the review command; blocked=%v msg=%q", blocked, msg)
	}
}

// PushGate surfaces a build error rather than allowing the push: a review store it cannot read is
// not a reviewed branch. Same broken-store trigger as the commit-gate case.
func TestPushGateSurfacesBuildError(t *testing.T) {
	root, _, _, _ := gateRepo(t)
	if err := os.MkdirAll(filepath.Join(root, "docs", "metareview"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "metareview", "reviews"), []byte("not a dir\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := PushGate(root, "", nil); err == nil {
		t.Fatal("a broken review store must surface an error, not clear the push")
	}
}

// A by-target report for a FILE resolves the branch's own commit set, so a review's coverage of that
// file is judged current against this branch rather than credited blindly — or refused when it cannot be.
// This pins buildFor's `current == nil && target != ""` scope-resolution: the review below can only answer
// for a.go via covers()'s file+currency route, which needs the branch commit set. Drop that line and
// `current` stays nil, covers() refuses the CoveredPaths credit, and a.go blocks — so the assertion fails.
func TestBuildForByTargetResolvesBranchScopeForFileCurrency(t *testing.T) {
	root, _, head := gitRepo(t) // branch changes a.go and b.go; head is in the branch commit set
	// Target is the branch, NOT the path, so this review answers for a.go only through file+currency.
	mustWriteFile(t, filepath.Join(root, "docs", "metareview", "reviews", "r.md"),
		"# metareview: pr-ready review\n\nRun ID: `mrv-r`\nTarget: `current branch`\n\n## Verdict\n\nPASS\n")
	mustWriteFile(t, filepath.Join(root, ".metareview", "runs.jsonl"),
		`{"id":"mrv-r","scope":"pr-ready","verdict":"PASS","headSha":"`+head+`","coveredPaths":["a.go"]}`+"\n")

	got, err := BuildFor(root, "a.go")
	if err != nil {
		t.Fatal(err)
	}
	if got.Blocked {
		t.Fatalf("a review that read a.go at the branch head must clear the a.go file target; must_clear=%+v", got.MustClear)
	}
}

// A push sends COMMITTED work, so PushGate must ignore uncommitted local WIP: an unreviewed untracked
// file (or unstaged edit) that is not part of the push must not block a branch whose commits are reviewed.
// The old scope folded WIP in and blocked a clean push, which only pressured people toward --no-verify
// (Cursor Bugbot: status.go — "Push gate includes uncommitted files").
func TestPushGateIgnoresUncommittedWIP(t *testing.T) {
	root, _, headSHA := gitRepo(t) // branch commits a.go and b.go
	// A passing pr-ready review that read both committed files → the push is clean.
	mustWriteFile(t, filepath.Join(root, "docs", "metareview", "reviews", "now.md"),
		"# metareview: pr-ready review\n\nRun ID: `mrv-now`\nTarget: `current branch`\n\n## Verdict\n\nPASS\n")
	mustWriteFile(t, filepath.Join(root, ".metareview", "runs.jsonl"),
		`{"id":"mrv-now","scope":"pr-ready","verdict":"PASS","headSha":"`+headSHA+`","coveredPaths":["a.go","b.go"]}`+"\n")
	if blocked, msg, err := PushGate(root, "", nil); err != nil || blocked {
		t.Fatalf("a branch whose committed files are reviewed must push clean; blocked=%v msg=%q err=%v", blocked, msg, err)
	}
	// Now drop UNCOMMITTED, unreviewed WIP that the push will not send: an untracked file and an unstaged edit.
	if err := os.WriteFile(filepath.Join(root, "wip.go"), []byte("package p\nvar WIP = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package p // a, edited but unstaged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	blocked, msg, err := PushGate(root, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if blocked {
		t.Fatalf("uncommitted WIP is not part of the push and must not block it; msg=%q", msg)
	}
}

// With a base, the push message threads it through the whole-branch review command.
func TestPushGateNamesBaseInMessage(t *testing.T) {
	root, _, _ := gitRepo(t) // branch changes a.go and b.go, unreviewed
	blocked, msg, err := PushGate(root, "main", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !blocked || !strings.Contains(msg, "--base main") {
		t.Fatalf("the push message must thread the base through the review command; blocked=%v msg=%q", blocked, msg)
	}
}
