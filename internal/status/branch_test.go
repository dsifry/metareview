package status

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/reviewlog"
)

// A review speaks to this branch if it reviewed one of the branch's own commits, or if it read
// one of the files the branch changed. Both routes matter: the first lets an earlier attempt in
// the same chain still count, and the second is the only thing that can answer for a file, since
// review targets are task ids and `current branch`, never paths.
func TestInScopeAcceptsBothRoutes(t *testing.T) {
	s := BranchScope{
		Base:    "base0000",
		Head:    "head0000",
		Commits: map[string]bool{"c1": true, "head0000": true},
		Files:   []string{"internal/a.go", "internal/b.go"},
	}
	for name, tc := range map[string]struct {
		r    reviewlog.Summary
		want bool
	}{
		"reviewed a branch commit": {reviewlog.Summary{HeadSHA: "c1"}, true},
		"reviewed the branch head": {reviewlog.Summary{HeadSHA: "head0000"}, true},
		"a commit from elsewhere":  {reviewlog.Summary{HeadSHA: "othersha"}, false},
		// The one that matters. A review on another branch that read a file this branch also
		// changed saw a DIFFERENT version of it, and must not be able to vouch for this one —
		// that is a stale review clearing current work, which is what scoping exists to stop.
		"read the same file on another branch": {reviewlog.Summary{HeadSHA: "othersha", CoveredPaths: []string{"internal/b.go"}}, false},
		"paths but no commit at all":           {reviewlog.Summary{CoveredPaths: []string{"internal/b.go"}}, false},
		"legacy review, no data":               {reviewlog.Summary{Target: "task-1"}, false},
	} {
		if got := s.InScope(tc.r); got != tc.want {
			t.Errorf("%s: InScope = %v, want %v", name, got, tc.want)
		}
	}
}

// The Completion Rule, made mechanical: a file this branch changed that no PASSING review has
// read is unreviewed, and unreviewed is a blocker. Without this a branch could change anything
// so long as some unrelated review had once passed.
func TestUnreviewedNamesFilesNoPassingReviewRead(t *testing.T) {
	s := BranchScope{
		Commits: map[string]bool{"c1": true},
		Files:   []string{"a.go", "b.go", "c.go"},
	}
	logs := []reviewlog.Summary{
		// In scope and passing: clears what it read.
		{HeadSHA: "c1", CoveredPaths: []string{"a.go"}},
		// In scope but itself blocked: has cleared nothing, however much it read.
		{HeadSHA: "c1", CoveredPaths: []string{"b.go"}, HasUnresolvedBlockers: true},
		// Passing, and it read c.go — but on another branch, so at a version this branch has
		// since changed. It clears nothing here.
		{HeadSHA: "elsewhere", CoveredPaths: []string{"c.go"}},
	}
	got := s.Unreviewed(logs)
	if len(got) != 2 || got[0] != "b.go" || got[1] != "c.go" {
		t.Errorf("Unreviewed = %v, want [b.go c.go]", got)
	}
	// A file an in-scope passing review read is not reported.
	s.Files = []string{"a.go"}
	if got := s.Unreviewed(logs); len(got) != 0 {
		t.Errorf("a reviewed file must not be reported unreviewed: %v", got)
	}
	// Order follows the branch's file list, so the answer is stable between runs.
	s.Files = []string{"c.go", "b.go"}
	if got := s.Unreviewed(logs); len(got) != 2 || got[0] != "c.go" || got[1] != "b.go" {
		t.Errorf("the order must follow the branch's files: %v", got)
	}
}

// rev-list failing is not a reason to report an empty scope: the changed files are the authority
// on what the work is, and a scope that silently became empty would clear the whole branch.
func TestBranchScopeSurvivesRevListFailing(t *testing.T) {
	root := t.TempDir()
	if _, err := ResolveBranchScope(root, "", func(string, ...string) ([]byte, error) {
		return nil, errors.New("not a git repository")
	}); err == nil {
		t.Error("a directory that is not a repository must be an error, not an empty scope")
	}
}

// gitRepo builds a small real repository: a base commit, then a branch commit changing two files.
// gitcontext resolves the base by merge-base with main, so the branch has to actually exist.
func gitRepo(t *testing.T) (root, baseSHA, headSHA string) {
	t.Helper()
	root = t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-q", "-b", "main")
	write("base.go", "package p\n")
	run("add", ".")
	run("commit", "-qm", "base")
	baseSHA = run("rev-parse", "HEAD")
	// A real branch off main, or merge-base HEAD main is HEAD and the branch changes nothing.
	run("checkout", "-q", "-b", "work")
	write("a.go", "package p // a\n")
	write("b.go", "package p // b\n")
	run("add", ".")
	run("commit", "-qm", "work")
	headSHA = run("rev-parse", "HEAD")
	return root, baseSHA, headSHA
}

// The whole point, end to end: on a branch, `status` answers about THIS work — blockers against
// its own commits, and files it changed that no passing review has read. Unscoped the same
// repository answers over the entire history, which is what made a Stop hook refuse every
// session for work it never touched.
func TestBuildForBranchAnswersAboutTheWorkInHand(t *testing.T) {
	root, _, headSHA := gitRepo(t)

	// A blocked review of some unrelated, older work. It must not reach the branch answer.
	mustWriteFile(t, filepath.Join(root, "docs", "metareview", "reviews", "old.md"),
		"# metareview: task-done review\n\nRun ID: `mrv-old`\nTarget: `ancient`\n\n## Verdict\n\nNEEDS_REVISION\n")
	// A passing review of this branch's head that read one of the two changed files.
	mustWriteFile(t, filepath.Join(root, "docs", "metareview", "reviews", "now.md"),
		"# metareview: pr-ready review\n\nRun ID: `mrv-now`\nTarget: `current branch`\n\n## Verdict\n\nPASS\n")
	mustWriteFile(t, filepath.Join(root, ".metareview", "runs.jsonl"),
		`{"id":"mrv-old","scope":"task-done","verdict":"NEEDS_REVISION","headSha":"deadbeef"}`+"\n"+
			`{"id":"mrv-now","scope":"pr-ready","verdict":"PASS","headSha":"`+headSHA+`","coveredPaths":["a.go"]}`+"\n")

	unscoped, err := Build(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(unscoped.MustClear) != 1 || unscoped.MustClear[0].Target != "ancient" {
		t.Fatalf("unscoped must see the old blocker: %+v", unscoped.MustClear)
	}

	got, err := BuildForBranch(root, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Warnings) != 0 {
		t.Fatalf("the scope resolved, so there is nothing to warn about: %v", got.Warnings)
	}
	if !strings.HasPrefix(got.Target, "branch ") {
		t.Errorf("the report must say it was scoped to a branch: %q", got.Target)
	}
	// The unrelated blocker is out of scope; b.go is in it and nothing reviewed it.
	var unreviewed []string
	for _, b := range got.MustClear {
		if b.Target == "ancient" {
			t.Errorf("an unrelated blocker leaked into the branch answer: %+v", b)
		}
		if b.Verdict == VerdictUnreviewed {
			unreviewed = append(unreviewed, b.Target)
		}
	}
	if len(unreviewed) != 1 || unreviewed[0] != "b.go" {
		t.Errorf("unreviewed = %v, want [b.go]: a.go was read by a passing review of this head", unreviewed)
	}
	if !got.Blocked {
		t.Error("a changed file nobody reviewed must block")
	}

	// And the exit code a hook branches on follows the same answer.
	var buf bytes.Buffer
	code, err := EmitForBranch(root, "", nil, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 {
		t.Errorf("exit code = %d, want 1 while something must be cleared", code)
	}
	if !strings.Contains(buf.String(), "UNREVIEWED") {
		t.Errorf("the emitted report must name the unreviewed file:\n%s", buf.String())
	}
}

// A scope that cannot be resolved reports the UNSCOPED answer with a warning, never an empty
// one. A gate that cannot work out what the work is must fail toward blocking.
func TestBuildForBranchFailsTowardBlockingWhenTheScopeIsUnknown(t *testing.T) {
	root := t.TempDir() // not a git repository
	mustWriteFile(t, filepath.Join(root, "docs", "metareview", "reviews", "old.md"),
		"# metareview: task-done review\n\nRun ID: `mrv-old`\nTarget: `ancient`\n\n## Verdict\n\nNEEDS_REVISION\n")
	got, err := BuildForBranch(root, "", nil)
	if err != nil {
		t.Fatalf("an unresolvable scope is reported, not an error: %v", err)
	}
	if !got.Blocked || len(got.MustClear) != 1 {
		t.Errorf("the unscoped answer must stand: %+v", got)
	}
	if len(got.Warnings) == 0 {
		t.Error("an answer wider than asked for must say so")
	}
}

func TestShortSHA(t *testing.T) {
	if got := shortSHA("0123456789abcdef"); got != "01234567" {
		t.Errorf("got %q", got)
	}
	if got := shortSHA("abc"); got != "abc" {
		t.Errorf("a short value is left alone, got %q", got)
	}
}

func mustWriteFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The remaining branches, each a way the answer could quietly become wrong.
func TestBranchScopeEdges(t *testing.T) {
	root, _, headSHA := gitRepo(t)

	// rev-list failing must not empty the scope: the changed files are the authority on what the
	// work is, and a silently empty scope would clear the whole branch.
	s, err := ResolveBranchScope(root, "", func(string, ...string) ([]byte, error) {
		return nil, errors.New("rev-list unavailable")
	})
	if err != nil {
		t.Fatalf("rev-list failing is not fatal: %v", err)
	}
	if len(s.Files) == 0 {
		t.Error("the changed files must survive rev-list failing")
	}
	// HEAD is in scope even then, so a review of the tip still counts.
	if !s.Commits[headSHA] {
		t.Errorf("the branch head must always be in scope: %v", s.Commits)
	}

	// A blocked review OF THIS BRANCH is reported, with its counts carried through.
	mustWriteFile(t, filepath.Join(root, "docs", "metareview", "reviews", "mine.md"),
		"# metareview: pr-ready review\n\nRun ID: `mrv-mine`\nTarget: `current branch`\n\n## Verdict\n\nNEEDS_REVISION\n")
	mustWriteFile(t, filepath.Join(root, ".metareview", "runs.jsonl"),
		`{"id":"mrv-mine","scope":"pr-ready","verdict":"NEEDS_REVISION","headSha":"`+headSHA+`","coveredPaths":["a.go","b.go"],"blockingFindingCount":4,"attemptNumber":2,"maxAttempts":3}`+"\n")
	got, err := BuildForBranch(root, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	var mine *Blocker
	for i := range got.MustClear {
		if got.MustClear[i].RunID == "mrv-mine" {
			mine = &got.MustClear[i]
		}
	}
	if mine == nil {
		t.Fatalf("a blocked review of this branch must be reported: %+v", got.MustClear)
	}
	if mine.BlockingCount != 4 || mine.AttemptNumber != 2 || mine.MaxAttempts != 3 {
		t.Errorf("the blocker's counts must survive scoping: %+v", mine)
	}
	// It read both changed files, but it is blocked, so it has cleared neither.
	for _, b := range got.MustClear {
		if b.Verdict == VerdictUnreviewed {
			return
		}
	}
	t.Error("a blocked review clears nothing it read, so its files are still unreviewed")
}

// An explicit --base is honoured, which is what lets a hook ask about work measured from
// somewhere other than the default merge-base.
func TestBuildForBranchHonoursAnExplicitBase(t *testing.T) {
	root, baseSHA, headSHA := gitRepo(t)
	mustWriteFile(t, filepath.Join(root, "docs", "metareview", "reviews", "now.md"),
		"# metareview: pr-ready review\n\nRun ID: `mrv-now`\nTarget: `current branch`\n\n## Verdict\n\nPASS\n")
	mustWriteFile(t, filepath.Join(root, ".metareview", "runs.jsonl"),
		`{"id":"mrv-now","scope":"pr-ready","verdict":"PASS","headSha":"`+headSHA+`","coveredPaths":["a.go","b.go"]}`+"\n")

	got, err := BuildForBranch(root, baseSHA, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Blocked {
		t.Errorf("every changed file was read by a passing review of this head: %+v", got.MustClear)
	}

	var buf bytes.Buffer
	code, err := EmitForBranch(root, baseSHA, nil, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0 when the branch is clear", code)
	}

	// An unresolvable base is an error the caller sees, not a silently different answer.
	if _, err := EmitForBranch(root, "no-such-ref", nil, &buf); err == nil {
		// gitcontext refuses the ref, so BuildForBranch warns and reports unscoped rather than
		// failing — either is safe, but it must never come back clean and silent.
		r, _ := BuildForBranch(root, "no-such-ref", nil)
		if len(r.Warnings) == 0 {
			t.Error("a base that cannot be resolved must be reported, not ignored")
		}
	}
}

// A repository whose review logs cannot be read is an error the caller sees. It must not become
// a clean answer: "I could not read the reviews" and "there are no blockers" are the same bytes
// to a hook branching on the exit code, and only one of them means work may proceed.
func TestBranchAnswerSurfacesAnUnreadableReviewLog(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: file permissions are not enforced")
	}
	root, _, _ := gitRepo(t)
	dir := filepath.Join(root, "docs", "metareview", "reviews")
	mustWriteFile(t, filepath.Join(dir, "one.md"),
		"# metareview: task-done review\n\nRun ID: `mrv-1`\nTarget: `t`\n\n## Verdict\n\nPASS\n")
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	if _, err := BuildForBranch(root, "", nil); err == nil {
		t.Error("an unreadable review log must be an error, not an empty report")
	}
	var buf bytes.Buffer
	code, err := EmitForBranch(root, "", nil, &buf)
	if err == nil {
		t.Error("EmitForBranch must surface the same error")
	}
	if code != 0 || buf.Len() != 0 {
		t.Errorf("nothing may be emitted when the answer could not be built: code=%d out=%q", code, buf.String())
	}
}

// metareview's own generated artifacts must not be counted as work needing review.
//
// A review's covered paths come from an exclude-filtered context, so .metareview/** and
// docs/metareview/** can never appear in one. Enumerating the unfiltered set here made every
// committed review log and context pack permanently unreviewed, and each new review committed
// three more files no future review could clear — so on a repository that commits its review
// artifacts, which CLAUDE.md requires, the branch could never reach a clean state. That is the
// livelock this scoping exists to prevent, one level down.
func TestBranchScopeIgnoresMetareviewsOwnArtifacts(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	mustWriteFile(t, filepath.Join(root, "base.go"), "package p\n")
	git("init", "-q", "-b", "main")
	git("add", ".")
	git("commit", "-qm", "base")
	git("checkout", "-q", "-b", "work")
	mustWriteFile(t, filepath.Join(root, "real.go"), "package p // real\n")
	// Exactly what a review run commits alongside the source change.
	mustWriteFile(t, filepath.Join(root, "docs", "metareview", "reviews", "mrv-1.md"), "# metareview: task-done review\n")
	mustWriteFile(t, filepath.Join(root, "docs", "metareview", "context", "mrv-1-context.md"), "context\n")
	mustWriteFile(t, filepath.Join(root, "docs", "metareview", "FINDINGS.md"), "findings\n")
	git("add", ".")
	git("commit", "-qm", "work")

	scope, err := ResolveBranchScope(root, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range scope.Files {
		if strings.HasPrefix(f, "docs/metareview/") || strings.HasPrefix(f, ".metareview/") {
			t.Errorf("metareview's own artifact is in the branch scope and can never be cleared: %q", f)
		}
	}
	var sawReal bool
	for _, f := range scope.Files {
		if f == "real.go" {
			sawReal = true
		}
	}
	if !sawReal {
		t.Errorf("the actual source change must still be in scope: %v", scope.Files)
	}
}
