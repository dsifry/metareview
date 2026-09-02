package status

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
		// In scope by the TARGET route: it is a review OF a document this branch changed. This
		// case previously carried no Target, so it returned false for the trivial reason that
		// InScope never reads CoveredPaths — the test asserted a guarantee the code did not
		// provide and could not have failed. What the target route actually admits is asserted
		// here, and what it is allowed to CLEAR is asserted in Unreviewed below.
		"a review of a file this branch changed": {reviewlog.Summary{HeadSHA: "othersha", Target: "internal/b.go"}, true},
	} {
		if got := s.InScope(tc.r); got != tc.want {
			t.Errorf("%s: InScope = %v, want %v", name, got, tc.want)
		}
	}
	// Being in scope and having examined this branch's code are different facts, and only the
	// second can vouch for a file's current contents.
	if !s.InScopeByCommit(reviewlog.Summary{HeadSHA: "c1"}) {
		t.Error("a review of a branch commit is in scope BY COMMIT")
	}
	if s.InScopeByCommit(reviewlog.Summary{HeadSHA: "othersha", Target: "internal/b.go"}) {
		t.Error("the target route is not commit identity, and must not be reported as it")
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
		{HeadSHA: "c1", CoveredPaths: []string{"a.go"}, CoveredPathsKnown: true},
		// In scope but itself blocked: has cleared nothing, however much it read.
		{HeadSHA: "c1", CoveredPaths: []string{"b.go"}, CoveredPathsKnown: true, HasUnresolvedBlockers: true},
		// Passing, and it read c.go — but on another branch, so at a version this branch has
		// since changed. It clears nothing here.
		//
		// This case carried no Target before, so it was excluded for the trivial reason that
		// InScope refuses a review with neither a matching commit nor a target — the assertion
		// could not fail whatever Unreviewed did with CoveredPaths. It now takes the TARGET
		// route into scope, which is the case that was actually broken: the review is a
		// legitimate artifact review of c.go, it belongs in scope, and it still must not clear
		// c.go, because it examined some other commit's version of it.
		{HeadSHA: "elsewhere", Target: "c.go", CoveredPaths: []string{"c.go", "b.go"}, CoveredPathsKnown: true},
	}
	// c.go is cleared: it is what that review is OF, and the target route exists so an artifact
	// review can answer for its own document. b.go is NOT cleared, though the same review lists
	// it among the paths it read — reading a file on another commit is not reviewing this one.
	// That distinction is the fix: crediting the whole CoveredPaths set here let a PASS on
	// another branch clear this branch's rewritten files.
	got := s.Unreviewed(logs)
	if len(got) != 1 || got[0] != "b.go" {
		t.Errorf("Unreviewed = %v, want [b.go]: a review of c.go clears c.go and nothing else", got)
	}
	// A file an in-scope passing review read is not reported.
	s.Files = []string{"a.go"}
	if got := s.Unreviewed(logs); len(got) != 0 {
		t.Errorf("a reviewed file must not be reported unreviewed: %v", got)
	}
	// Order follows the branch's file list, so the answer is stable between runs. Both files here
	// are genuinely unreviewed: d.go no review mentions at all, b.go only a blocked review and a
	// non-current one list.
	s.Files = []string{"d.go", "b.go"}
	if got := s.Unreviewed(logs); len(got) != 2 || got[0] != "d.go" || got[1] != "b.go" {
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

// The repair loop must reach clean IN BRANCH SCOPE — the scope the Stop/commit hook actually uses.
// BuildForBranch rebuilds must_clear from scoped reviews, and an earlier version of the supersede fix was
// wired only into the unscoped Build path, so a superseded parent kept blocking the branch forever. This
// pins that the branch answer honours the supersede.
func TestBuildForBranchSupersedesRepairedParent(t *testing.T) {
	root, _, headSHA := gitRepo(t)
	// Parent attempt found blockers; the child re-review of the same branch head passed and read both
	// changed files, so nothing is unreviewed and only the supersede decides the answer.
	mustWriteFile(t, filepath.Join(root, "docs", "metareview", "reviews", "parent.md"),
		"# metareview: pr-ready review\n\nRun ID: `mrv-parent`\nTarget: `current branch`\n\n## Verdict\n\nNEEDS_REVISION\n")
	mustWriteFile(t, filepath.Join(root, "docs", "metareview", "reviews", "child.md"),
		"# metareview: pr-ready review\n\nRun ID: `mrv-child`\nTarget: `current branch`\n\nPrevious run: `mrv-parent`\n\n## Verdict\n\nPASS\n")
	mustWriteFile(t, filepath.Join(root, ".metareview", "runs.jsonl"),
		`{"id":"mrv-parent","scope":"pr-ready","verdict":"NEEDS_REVISION","headSha":"`+headSHA+`"}`+"\n"+
			`{"id":"mrv-child","scope":"pr-ready","verdict":"PASS","headSha":"`+headSHA+`","previousRunId":"mrv-parent","coveredPaths":["a.go","b.go"]}`+"\n")

	got, err := BuildForBranch(root, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Blocked || len(got.MustClear) != 0 {
		t.Fatalf("a branch whose repair passed must not block; blocked=%v must_clear=%+v", got.Blocked, got.MustClear)
	}
	var buf bytes.Buffer
	if code, _ := EmitForBranch(root, "", nil, &buf); code != 0 {
		t.Fatalf("exit code = %d, want 0 once the branch is clean:\n%s", code, buf.String())
	}
}

// BuildForBranch computes supersede over the FULL log set, not the branch-scoped subset: a repair whose
// CLEAN child sits at a commit OFF this branch (out of scope) must still retire the in-scope open parent it
// links back to. A mutation narrowing supersededRuns(all) -> supersededRuns(scoped) drops that child and
// wrongly keeps the parent blocking — this pins the `all` choice as load-bearing. An in-scope PASS covers
// both changed files so unreviewed-files never confounds the supersede answer.
func TestBuildForBranchSupersedesFromFullLogSetNotScoped(t *testing.T) {
	root, _, headSHA := gitRepo(t)
	mustWriteFile(t, filepath.Join(root, "docs", "metareview", "reviews", "cover.md"),
		"# metareview: pr-ready review\n\nRun ID: `mrv-cover`\nTarget: `current branch`\n\n## Verdict\n\nPASS\n")
	mustWriteFile(t, filepath.Join(root, "docs", "metareview", "reviews", "parent.md"),
		"# metareview: pr-ready review\n\nRun ID: `mrv-parent`\nTarget: `current branch`\n\n## Verdict\n\nNEEDS_REVISION\n")
	mustWriteFile(t, filepath.Join(root, "docs", "metareview", "reviews", "child.md"),
		"# metareview: pr-ready review\n\nRun ID: `mrv-child`\nTarget: `current branch`\n\nPrevious run: `mrv-parent`\n\n## Verdict\n\nPASS\n")
	offBranch := "0000000000000000000000000000000000cafe00" // a commit not in this branch's set
	mustWriteFile(t, filepath.Join(root, ".metareview", "runs.jsonl"),
		`{"id":"mrv-cover","scope":"pr-ready","verdict":"PASS","headSha":"`+headSHA+`","coveredPaths":["a.go","b.go"]}`+"\n"+
			`{"id":"mrv-parent","scope":"pr-ready","verdict":"NEEDS_REVISION","headSha":"`+headSHA+`"}`+"\n"+
			`{"id":"mrv-child","scope":"pr-ready","verdict":"PASS","headSha":"`+offBranch+`","previousRunId":"mrv-parent"}`+"\n")

	got, err := BuildForBranch(root, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Blocked || len(got.MustClear) != 0 {
		t.Fatalf("an out-of-scope clean child must still supersede the in-scope open parent (full log set, not scoped); blocked=%v must_clear=%+v", got.Blocked, got.MustClear)
	}
}

// CommitGate is scoped to the STAGED files (this commit), not the branch: a staged unreviewed file blocks,
// but a committed-but-not-staged change (the branch's other commits) is PushGate's concern, not this one.
func TestCommitGateScopedToStagedFiles(t *testing.T) {
	root := t.TempDir()
	run := func(args ...string) string {
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
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-q", "-b", "main")
	write("base.go", "package p\n")
	run("add", ".")
	run("commit", "-qm", "base")
	run("checkout", "-q", "-b", "work")

	// Nothing staged yet: the commit gate has nothing to gate.
	if blocked, _, err := CommitGate(root, "", nil); err != nil || blocked {
		t.Fatalf("nothing staged must not block: blocked=%v err=%v", blocked, err)
	}
	// A COMMITTED (not staged) change is the branch's business, not this commit's — CommitGate ignores it.
	write("committed.go", "package p\nvar C = 1\n")
	run("add", "committed.go")
	run("commit", "-qm", "prior")
	if blocked, _, err := CommitGate(root, "", nil); err != nil || blocked {
		t.Fatalf("a committed-but-not-staged change is PushGate's concern, not CommitGate's: blocked=%v", blocked)
	}
	// Now STAGE an unreviewed change: the commit gate blocks it and names it.
	write("staged.go", "package p\nvar S = 1\n")
	run("add", "staged.go")
	blocked, msg, err := CommitGate(root, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !blocked || !strings.Contains(msg, "staged.go") {
		t.Fatalf("a staged unreviewed file must block and be named; blocked=%v msg=%q", blocked, msg)
	}
	if strings.Contains(msg, "committed.go") {
		t.Fatalf("CommitGate must not report the committed-but-not-staged file:\n%s", msg)
	}
}

// The `git commit -a` / pathspec hole: a tracked file MODIFIED in the working tree but not staged is
// invisible to the staged scope (a plain commit would not write it), but `git commit -a` writes it. The
// ScopeAll gate must catch exactly that content — and must NOT pull in untracked files, which `-a` does not
// commit either.
func TestCommitGateAllScopeCatchesUnstagedTrackedChange(t *testing.T) {
	root := t.TempDir()
	run := func(args ...string) string {
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
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-q", "-b", "main")
	write("tracked.go", "package p\nvar T = 1\n")
	run("add", ".")
	run("commit", "-qm", "base")
	run("checkout", "-q", "-b", "work")

	// Modify a tracked file WITHOUT staging it, and drop an untracked file alongside.
	write("tracked.go", "package p\nvar T = 2\n") // modified, unstaged
	write("untracked.go", "package p\nvar U = 1\n")

	// Staged scope: a plain `git commit` writes the (empty) index, so it has nothing to gate — this is the
	// exact state that let `git commit -am` slip through when the gate was staged-only.
	if blocked, _, err := CommitGate(root, "", nil); err != nil || blocked {
		t.Fatalf("staged scope must not block an unstaged change (a plain commit would not write it): blocked=%v err=%v", blocked, err)
	}

	// -a scope: `git commit -a` WOULD write the modified tracked file, so the gate must block and name it.
	blocked, msg, err := CommitGateScoped(root, "", ScopeAll, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !blocked || !strings.Contains(msg, "tracked.go") {
		t.Fatalf("-a scope must block the unstaged tracked change and name it; blocked=%v msg=%q", blocked, msg)
	}
	// `git commit -a` does not commit untracked files, so the gate must not invent a blocker for one.
	if strings.Contains(msg, "untracked.go") {
		t.Fatalf("-a does not commit untracked files; the gate must not block on one:\n%s", msg)
	}
}

// PushGate is the whole-branch decision the pre-push hook stands on; its block/allow AND message are tested
// here rather than left to untested cmd glue.
func TestPushGateBlocksUnreviewedAndClearsWhenReviewed(t *testing.T) {
	root, _, headSHA := gitRepo(t) // branch changes a.go and b.go

	// No review yet: the gate blocks and names the unreviewed files and the review-prompt command.
	blocked, msg, err := PushGate(root, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !blocked {
		t.Fatal("an unreviewed branch must block a push")
	}
	if !strings.Contains(msg, "a.go") || !strings.Contains(msg, "b.go") {
		t.Fatalf("the message must name the unreviewed files:\n%s", msg)
	}
	if !strings.Contains(msg, "review pr-ready") || !strings.Contains(msg, "override") {
		t.Fatalf("the push message must point at the whole-branch review and the override:\n%s", msg)
	}

	// A passing pr-ready review of this branch head that read both files clears the gate.
	mustWriteFile(t, filepath.Join(root, "docs", "metareview", "reviews", "now.md"),
		"# metareview: pr-ready review\n\nRun ID: `mrv-now`\nTarget: `current branch`\n\n## Verdict\n\nPASS\n")
	mustWriteFile(t, filepath.Join(root, ".metareview", "runs.jsonl"),
		`{"id":"mrv-now","scope":"pr-ready","verdict":"PASS","headSha":"`+headSHA+`","coveredPaths":["a.go","b.go"]}`+"\n")
	blocked, msg, err = PushGate(root, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if blocked || msg != "" {
		t.Fatalf("a fully-reviewed branch must not block; blocked=%v msg=%q", blocked, msg)
	}
}

// There is NO claim-free exemption: every changed file must be reviewed, whitespace-only included. The
// exemption was removed because a false exemption is a review gate's worst failure (unreviewed code ships
// as "exempt"), and it had already produced a critical rename+edit false-exemption. A whitespace-only diff
// is trivial for a reviewer to clear, so blocking it costs a glance and buys correctness.
func TestPushGateDoesNotExemptWhitespaceOnlyChange(t *testing.T) {
	root := t.TempDir()
	run := func(args ...string) string {
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
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-q", "-b", "main")
	write("ws.go", "package p\nvar X = 1\n")
	run("add", ".")
	run("commit", "-qm", "base")
	run("checkout", "-q", "-b", "work")
	write("ws.go", "package p\nvar  X = 1\n") // whitespace-only, unreviewed
	run("add", "-A")
	run("commit", "-qm", "ws")

	blocked, msg, err := PushGate(root, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !blocked || !strings.Contains(msg, "ws.go") {
		t.Fatalf("a whitespace-only change is no longer exempt: it must block and be named; blocked=%v msg=%q", blocked, msg)
	}
}

// A scope that cannot be resolved reports the UNSCOPED answer with a warning, never an empty one,
// and it BLOCKS. A gate that cannot work out what the work is must fail toward blocking.
//
// This test used to seed a NEEDS_REVISION review before asking, so `Blocked` was true because of
// that pre-existing blocker and would have stayed true however the unresolvable scope was
// handled. It certified a guarantee the code did not provide: with a clean log, an unresolvable
// scope returned blocked:false and exit 0, warning into JSON that nothing branches on. The
// repository is clean here for exactly that reason — the scope failure has to be the only thing
// that can block.
func TestBuildForBranchFailsTowardBlockingWhenTheScopeIsUnknown(t *testing.T) {
	root := t.TempDir() // not a git repository, and no reviews at all

	got, err := BuildForBranch(root, "", nil)
	if err != nil {
		t.Fatalf("an unresolvable scope is reported, not an error: %v", err)
	}
	if !got.Blocked {
		t.Fatal("a gate that cannot tell what the work is must block, not pass")
	}
	var scopeBlocker bool
	for _, b := range got.MustClear {
		if b.Verdict == VerdictUnscoped && b.Kind == "scope" {
			scopeBlocker = true
		}
	}
	if !scopeBlocker {
		t.Errorf("must_clear must name the unresolved scope as the reason: %+v", got.MustClear)
	}
	if len(got.Warnings) == 0 {
		t.Error("an answer wider than asked for must say so")
	}

	// And the exit code, which is the only part a Stop hook reads.
	var buf bytes.Buffer
	code, err := EmitForBranch(root, "", nil, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 {
		t.Errorf("exit code = %d, want 1: an unresolvable scope must not read as a clean tree", code)
	}

	// A pre-existing blocker must still be reported alongside it, not replaced by it.
	mustWriteFile(t, filepath.Join(root, "docs", "metareview", "reviews", "old.md"),
		"# metareview: task-done review\n\nRun ID: `mrv-old`\nTarget: `ancient`\n\n## Verdict\n\nNEEDS_REVISION\n")
	got, err = BuildForBranch(root, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.MustClear) != 2 {
		t.Errorf("both the old blocker and the scope failure must be reported: %+v", got.MustClear)
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

	// Uncommitted work too: a Stop hook fires exactly when an agent is about to finish, which is
	// when work is most likely written and not yet committed. Committed-only scope made the hook
	// emit nothing and exit 0 on `git add` with no commit — failing OPEN at the one moment it
	// exists for, where the unscoped query it replaced at least failed closed.
	mustWriteFile(t, filepath.Join(root, "staged.go"), "package p // staged\n")
	git("add", "staged.go")
	mustWriteFile(t, filepath.Join(root, "untracked.go"), "package p // untracked\n")

	scope, err := ResolveBranchScope(root, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	inScope := map[string]bool{}
	for _, f := range scope.Files {
		inScope[f] = true
		if strings.HasPrefix(f, "docs/metareview/") || strings.HasPrefix(f, ".metareview/") {
			t.Errorf("metareview's own artifact is in the branch scope and can never be cleared: %q", f)
		}
	}
	for _, want := range []string{"staged.go", "untracked.go"} {
		if !inScope[want] {
			t.Errorf("uncommitted %s is invisible to the gate: %v", want, scope.Files)
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

// A review that never recorded WHAT it read vouches for nothing, even in scope and passing. That
// is the whole reason CoveredPathsKnown exists: "examined nothing" is an answer, "cannot say" is
// not, and every review written before the field carries the second. Treating them alike would
// let a log predating the field clear a file it never opened.
func TestAReviewThatCannotSayWhatItReadClearsNothing(t *testing.T) {
	s := BranchScope{Commits: map[string]bool{"c1": true}, Files: []string{"a.go"}}
	silent := []reviewlog.Summary{{HeadSHA: "c1", CoveredPaths: []string{"a.go"}}} // Known is false
	if got := s.Unreviewed(silent); len(got) != 1 || got[0] != "a.go" {
		t.Errorf("a review with unknown coverage must clear nothing, got %v", got)
	}
	spoke := []reviewlog.Summary{{HeadSHA: "c1", CoveredPaths: []string{"a.go"}, CoveredPathsKnown: true}}
	if got := s.Unreviewed(spoke); len(got) != 0 {
		t.Errorf("a review that said what it read must clear it, got %v", got)
	}
}

// An artifact review records its head at SCAFFOLD time, which on a fresh branch is the base — a
// commit rev-list base..HEAD excludes by construction. Commit identity alone therefore dropped
// every artifact review from the scoped answer while the unscoped one still reported it: the same
// repository at the same commit giving opposite answers. A review OF a document the branch
// changed speaks to the branch, whatever commit it names.
func TestAReviewOfAChangedDocumentIsInScope(t *testing.T) {
	s := BranchScope{Commits: map[string]bool{"c1": true}, Files: []string{"docs/spec.md", "a.go"}}
	artifact := reviewlog.Summary{Target: "docs/spec.md", HeadSHA: "base-commit"}
	if !s.InScope(artifact) {
		t.Error("an artifact review of a document this branch changed must be in scope")
	}
	elsewhere := reviewlog.Summary{Target: "docs/other.md", HeadSHA: "base-commit"}
	if s.InScope(elsewhere) {
		t.Error("a review of a document this branch did not touch is not in scope")
	}
	if s.InScope(reviewlog.Summary{HeadSHA: "base-commit"}) {
		t.Error("no target and no branch commit is not in scope")
	}
}

// Uncommitted work is in scope, and metareview's own artifacts are not — the same rule the
// committed set gets from git's pathspec, applied by hand because uncommitted paths never pass
// through it. Without the first half the hook fails open at exactly the moment it fires: an agent
// about to finish is the agent most likely to have written code and not yet committed it.
func TestBranchScopeIncludesUncommittedWorkButNotGeneratedArtifacts(t *testing.T) {
	got := appendUnseen([]string{"committed.go"},
		[]string{"staged.go", "committed.go"},            // a duplicate must not repeat
		[]string{"worktree.go", ""},                      // an empty path is not a path
		[]string{"untracked.go", "docs/metareview/x.md"}, // generated artifacts stay out
		[]string{".metareview/runs.jsonl", ".metareview", "docs/metareview"},
	)
	want := []string{"committed.go", "staged.go", "worktree.go", "untracked.go"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("position %d = %q, want %q (order must be stable)", i, got[i], want[i])
		}
	}
	for path, want := range map[string]bool{
		".metareview":            true,
		".metareview/runs.jsonl": true,
		"docs/metareview":        true,
		"docs/metareview/x.md":   true,
		"docs/metareviewing.md":  false,
		"internal/metareview.go": false,
		"a/docs/metareview/x.md": false,
		"":                       false,
	} {
		if got := isGeneratedMetareviewPath(path); got != want {
			t.Errorf("isGeneratedMetareviewPath(%q) = %v, want %v", path, got, want)
		}
	}
}

// An abandoned FSM run must block in EVERY scope, and branch scope is the one that matters most:
// hooks/pre-finish.sh defaults to --scope branch, so this is the scope enforcement actually runs
// in. BuildForBranch emptied MustClear to rebuild it from scoped reviews and never re-added the
// abandoned runs Build had put there, so the gate returned exit 0 with a run abandoned mid-loop —
// the precise failure the enforcement layer exists to prevent — while the unscoped and --target
// reports still blocked on the same run. Both bots on #23 caught it independently.
func TestBuildForBranchStillBlocksOnAnAbandonedRun(t *testing.T) {
	root, _, headSHA := gitRepo(t)

	// Everything this branch changed has been reviewed and passed: no review blocker, no
	// unreviewed file. The abandoned run is the ONLY thing left to block on, which is what makes
	// this the case the old code got wrong.
	mustWriteFile(t, filepath.Join(root, "docs", "metareview", "reviews", "now.md"),
		"# metareview: pr-ready review\n\nRun ID: `mrv-now`\nTarget: `current branch`\n\n## Verdict\n\nPASS\n")
	mustWriteFile(t, filepath.Join(root, ".metareview", "runs.jsonl"),
		`{"id":"mrv-now","scope":"pr-ready","verdict":"PASS","headSha":"`+headSHA+`","coveredPaths":["a.go","b.go"]}`+"\n")

	clean, err := BuildForBranch(root, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if clean.Blocked {
		t.Fatalf("nothing should block yet: %+v", clean.MustClear)
	}

	// Now abandon a run mid-loop, changing nothing else.
	writeRun(t, root, "run-stuck-at-fix",
		`{"seq":1,"type":"init","at":"2026-08-28T01:00:00Z","data":{"workflow":"sdlc-loop"}}`,
		`{"seq":2,"type":"transition","at":"2026-08-28T01:05:00Z","data":{"to":"fix","node":"agent-edit"}}`)

	got, err := BuildForBranch(root, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Abandoned) != 1 {
		t.Fatalf("the run is abandoned and must be reported: %+v", got.Abandoned)
	}
	if !got.Blocked {
		t.Fatal("an abandoned run must block in branch scope; the Stop hook runs in this scope")
	}
	var found bool
	for _, b := range got.MustClear {
		if b.Verdict == VerdictAbandoned && b.Kind == "fsm-run" && b.RunID == "run-stuck-at-fix" {
			found = true
		}
	}
	if !found {
		t.Errorf("must_clear must name the abandoned run: %+v", got.MustClear)
	}

	// The scopes must agree. Disagreeing about the same unfinished work is how an agent picks
	// the scope that says yes.
	unscoped, err := Build(root)
	if err != nil {
		t.Fatal(err)
	}
	if unscoped.Blocked != got.Blocked {
		t.Errorf("scopes disagree about the same abandoned run: unscoped=%v branch=%v",
			unscoped.Blocked, got.Blocked)
	}
}

// The Stop gate runs synchronously, so an unbounded git call holds session end for the host's
// whole command budget and a cancelled hook renders no decision at all. A stalled subprocess must
// become a fast error instead — which ResolveBranchScope reports as an unresolvable scope, and an
// unresolvable scope blocks.
func TestGitIsBounded(t *testing.T) {
	if gitDeadline <= 0 || gitDeadline > time.Minute {
		t.Fatalf("the deadline must be set and short: %s", gitDeadline)
	}
	// A real invocation still works, and returns well inside the deadline.
	root, _, _ := gitRepo(t)
	start := time.Now()
	out, err := realGit(root, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("an ordinary git call must still succeed: %v", err)
	}
	if len(bytes.TrimSpace(out)) == 0 {
		t.Error("rev-parse returned nothing")
	}
	if elapsed := time.Since(start); elapsed > gitDeadline {
		t.Errorf("an ordinary call took %s, longer than the deadline itself", elapsed)
	}
	// The timeout path itself. git is asked to do real work with no time to do it in, and the
	// error must SAY it timed out — ResolveBranchScope turns that into an unresolvable scope,
	// which blocks, so the difference between "timed out" and a generic failure is what an
	// operator has to read to know the gate was starved rather than the branch broken.
	restore := gitDeadline
	gitDeadline = time.Nanosecond
	_, err = realGit(root, "rev-list", "--all")
	gitDeadline = restore
	if err == nil {
		t.Error("a call with no time to run must fail, not return an empty answer")
	} else if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("the error must name the timeout: %v", err)
	}

	// A failing call is still an ordinary error, not a timeout report.
	if _, err := realGit(root, "rev-parse", "definitely-not-a-ref"); err == nil {
		t.Error("an invalid ref must error")
	} else if strings.Contains(err.Error(), "timed out") {
		t.Errorf("an invalid ref is not a timeout: %v", err)
	}
}

// A rev-list TIMEOUT must make the scope unresolvable, where an ordinary rev-list failure does
// not. The distinction matters: ordinary failure is tolerated because the changed files, not
// rev-list, are the authority on what the work is — but a timeout means git is stalled, so the
// commit set is not merely narrow, it is unknown. Swallowing it made the deadline decorative,
// because the caller saw a partial scope with a nil error exactly as if nothing had gone wrong.
func TestARevListTimeoutMakesTheScopeUnresolvable(t *testing.T) {
	root, _, _ := gitRepo(t)

	timedOut := func(_ string, _ ...string) ([]byte, error) {
		return nil, errors.New("git rev-list timed out after 20s")
	}
	if _, err := ResolveBranchScope(root, "", timedOut); err == nil {
		t.Error("a stalled git must leave the scope unresolvable, not partially answered")
	}
	// And an unresolvable scope blocks, which is the whole point of propagating it.
	rep, err := BuildForBranch(root, "", timedOut)
	if err != nil {
		t.Fatalf("it is reported, not returned as an error: %v", err)
	}
	if !rep.Blocked {
		t.Error("a scope that could not be resolved must block")
	}

	// An ORDINARY rev-list failure is still tolerated: the changed files remain the authority.
	failed := func(_ string, _ ...string) ([]byte, error) {
		return nil, errors.New("fatal: bad revision")
	}
	scope, err := ResolveBranchScope(root, "", failed)
	if err != nil {
		t.Errorf("an ordinary rev-list failure is not fatal to the scope: %v", err)
	}
	if scope.Head == "" || !scope.Commits[scope.Head] {
		t.Error("HEAD is in scope even when rev-list failed")
	}
}

// ChangeKinds maps each changed path to its git change type, keyed on the NEW path for a rename.
func TestChangeKinds(t *testing.T) {
	root := t.TempDir()
	run := func(args ...string) string {
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
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-q", "-b", "main")
	write("edit.go", "package p\nvar E = 1\n")
	// gone.go and new.go are deliberately DISSIMILAR so git's rename detection does not pair the deletion
	// with the addition (it pairs a deleted file with a similar added one — real behavior, see ChangeKinds).
	write("gone.go", "package removed\n\nfunc ToBeDeletedEntirely() string { return \"delete me\" }\n")
	write("move.go", "package p\nvar M = 1\n")
	run("add", ".")
	run("commit", "-qm", "base")
	base := run("rev-parse", "HEAD")
	run("checkout", "-q", "-b", "work")
	write("edit.go", "package p\nvar E = 2\n")                                         // modified
	write("new.go", "package fresh\n\ntype BrandNewUnrelated struct{ X, Y, Z int }\n") // added
	run("rm", "-q", "gone.go")                                                         // deleted
	run("mv", "move.go", "moved.go")                                                   // renamed (new path = moved.go)
	run("add", "-A")
	run("commit", "-qm", "changes")

	k := ChangeKinds(root, base, nil)
	for path, want := range map[string]string{"edit.go": "modified", "new.go": "added", "gone.go": "deleted", "moved.go": "renamed"} {
		if k[path] != want {
			t.Errorf("ChangeKinds[%q] = %q, want %q (full: %v)", path, k[path], want, k)
		}
	}
	if _, ok := k["move.go"]; ok {
		t.Errorf("a rename must key on the NEW path only, not the old one: %v", k)
	}
}

// ChangeKinds edge branches: a git error, a copy (C) keyed on the new path, an unhandled status letter
// defaulting to modified, and base resolution (success in a repo; failure outside one).
func TestChangeKindsEdges(t *testing.T) {
	if k := ChangeKinds("/nope", "base", func(string, ...string) ([]byte, error) { return nil, errors.New("boom") }); len(k) != 0 {
		t.Fatalf("a git error must yield no kinds, got %v", k)
	}
	run := func(string, ...string) ([]byte, error) {
		return []byte("C100\told.go\tcopied.go\nT\tchanged.go\n"), nil
	}
	k := ChangeKinds("/x", "base", run)
	if k["copied.go"] != "added" {
		t.Errorf("a copy must key the NEW path as added: %v", k)
	}
	if _, ok := k["old.go"]; ok {
		t.Errorf("a copy must not key the OLD path: %v", k)
	}
	if k["changed.go"] != "modified" {
		t.Errorf("an unhandled status letter must default to modified: %v", k)
	}
	// base=="" resolves the merge-base and diffs (success continuation) in a real repo.
	root, _, _ := gitRepo(t)
	_ = ChangeKinds(root, "", nil)
	// base=="" outside a repo: ResolveBranchScope errors, so ChangeKinds returns empty.
	if k := ChangeKinds(t.TempDir(), "", nil); len(k) != 0 {
		t.Fatalf("base-resolution failure must yield no kinds, got %v", k)
	}
}
