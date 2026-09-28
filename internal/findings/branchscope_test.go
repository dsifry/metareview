package findings

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/scope"
)

// scopeRepo is a real single checkout for #178: main with one commit. git runs with the config isolated.
func scopeRepo(t *testing.T) (root string, git func(args ...string) string) {
	t.Helper()
	root = t.TempDir()
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + root, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com", "GIT_EDITOR=true"}
	git = func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q", "-b", "main")
	git("commit", "-q", "--allow-empty", "-m", "initial")
	return root, git
}

func blockerOn(id, branch, head string) Record {
	r := openBlocker(id)
	r.Branch, r.GitHead = branch, head
	return r
}

func seedRecords(t *testing.T, root string, records ...Record) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".metareview"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONL(findingsPath(root), records); err != nil {
		t.Fatal(err)
	}
}

func ids(records []Record) string {
	var out []string
	for _, r := range records {
		out = append(out, r.ID)
	}
	return strings.Join(out, ",")
}

// TestScopedBlockingFollowsTheBranch is #178 in the ledger: a blocker belongs to the branch it was raised on, whichever
// branch is checked out in the one checkout; a legacy row (no branch) stays in scope while its head is reachable; and a
// merged-and-deleted branch's blocker blocks nothing (AC-4.6).
func TestScopedBlockingFollowsTheBranch(t *testing.T) {
	root, git := scopeRepo(t)
	git("switch", "-q", "-c", "branch-a")
	git("commit", "-q", "--allow-empty", "-m", "A")
	headA := git("rev-parse", "HEAD")
	git("switch", "-q", "-c", "branch-b", "main")
	git("commit", "-q", "--allow-empty", "-m", "B")
	headB := git("rev-parse", "HEAD")
	advisory := openBlocker("adv")
	advisory.Classification, advisory.Severity = "advisory", "low"
	seedRecords(t, root, blockerOn("on-a", "branch-a", headA), blockerOn("on-b", "branch-b", headB),
		blockerOn("legacy-a", "", headA), blockerOn("legacy-b", "", headB), advisory)

	in, elsewhere, err := ScopedBlocking(root)
	if err != nil {
		t.Fatal(err)
	}
	if ids(in) != "on-b,legacy-b" || ids(elsewhere) != "on-a,legacy-a" {
		t.Fatalf("on branch-b: in=%s elsewhere=%s", ids(in), ids(elsewhere))
	}
	if got, _ := UnresolvedBlocking(root); ids(got) != "on-b,legacy-b" {
		t.Fatalf("UnresolvedBlocking is the in-scope half, got %s", ids(got))
	}

	git("switch", "-q", "branch-a")
	if in, elsewhere, _ = ScopedBlocking(root); ids(in) != "on-a,legacy-a" || ids(elsewhere) != "on-b,legacy-b" {
		t.Fatalf("back on branch-a: in=%s elsewhere=%s", ids(in), ids(elsewhere))
	}

	// AC-4.6: branch-a merges and is deleted; a branch cut afterwards owes nothing for it.
	git("switch", "-q", "main")
	git("merge", "-q", "--no-ff", "-m", "merge a", "branch-a")
	git("branch", "-q", "-D", "branch-a")
	git("switch", "-q", "-c", "branch-c")
	git("commit", "-q", "--allow-empty", "-m", "C")
	if in, _, _ = ScopedBlocking(root); strings.Contains(ids(in), "on-a") {
		t.Fatalf("a merged-and-deleted branch's blocker must not block branch-c, in=%s", ids(in))
	}
}

// TestScopedBlockingFailsClosedAndAsksGitOnlyWhenNeeded: an unreadable scope (not a repository) keeps every blocker in
// scope, a ledger without blockers never asks git, and a ledger read failure is returned.
func TestScopedBlockingFailsClosedAndAsksGitOnlyWhenNeeded(t *testing.T) {
	root := t.TempDir()
	seedRecords(t, root, blockerOn("x", "elsewhere", "abc"))
	if in, elsewhere, err := ScopedBlocking(root); err != nil || ids(in) != "x" || len(elsewhere) != 0 {
		t.Fatalf("outside a repository everything stays in scope: in=%s elsewhere=%s err=%v", ids(in), ids(elsewhere), err)
	}

	loads := 0
	orig := loadScope
	loadScope = func(string) scope.Scope { loads++; return scope.Scope{} }
	t.Cleanup(func() { loadScope = orig })
	resolved := openBlocker("fixed")
	resolved.Status = "fixed"
	seedRecords(t, root, resolved)
	if in, _, err := ScopedBlocking(root); err != nil || len(in) != 0 || loads != 0 {
		t.Fatalf("no blockers: in=%s err=%v loads=%d", ids(in), err, loads)
	}

	if err := os.WriteFile(findingsPath(root), []byte("{not json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ScopedBlocking(root); err == nil {
		t.Fatal("an unreadable ledger must fail")
	}
	if _, err := UnresolvedBlocking(root); err == nil {
		t.Fatal("UnresolvedBlocking must surface the ledger error")
	}
}

// TestReconcileRecordsTheBranch: a new finding records the checked-out branch, or the run's own when the caller names
// one; a re-seen row takes the branch of the run that raised it again, as its head does.
func TestReconcileRecordsTheBranch(t *testing.T) {
	root, git := scopeRepo(t)
	git("switch", "-q", "-c", "feat")
	head := git("rev-parse", "HEAD")
	input := unsafeEval("eval")
	run := Run{ID: "mrv-1", Scope: "task-done", Target: map[string]string{"type": "path", "id": "t.md"}, RepoRoot: root, GitHead: head}
	if _, err := Reconcile(root, run, []Input{input}, Options{}); err != nil {
		t.Fatal(err)
	}
	if got := loadOne(t, root); got.Branch != "feat" {
		t.Fatalf("a new finding records the checked-out branch, got %q", got.Branch)
	}

	legacy := loadOne(t, root)
	legacy.Branch = ""
	seedRecords(t, root, legacy)
	run.ID, run.Branch = "mrv-2", "named"
	if _, err := Reconcile(root, run, []Input{input}, Options{}); err != nil {
		t.Fatal(err)
	}
	if got := loadOne(t, root); got.Branch != "named" {
		t.Fatalf("a re-seen legacy row adopts the run's branch, got %q", got.Branch)
	}
	run.ID, run.Branch = "mrv-3", "other"
	if _, err := Reconcile(root, run, []Input{input}, Options{}); err != nil {
		t.Fatal(err)
	}
	if got := loadOne(t, root); got.Branch != "other" {
		t.Fatalf("a re-seen row moves to the branch that raised it again, got %q", got.Branch)
	}
	run.ID, run.Branch = "mrv-4", ""
	loadScope = func(string) scope.Scope { return scope.Scope{} }
	t.Cleanup(func() { loadScope = func(root string) scope.Scope { return scope.Load(root, nil) } })
	if _, err := Reconcile(root, run, []Input{input}, Options{}); err != nil {
		t.Fatal(err)
	}
	if got := loadOne(t, root); got.Branch != "other" {
		t.Fatalf("a run on no branch (detached) leaves the row's branch alone, got %q", got.Branch)
	}
}

// reconcileOn records input as task t's finding at the checked-out head, the way a task-done run does.
func reconcileOn(t *testing.T, root, runID, head string, input Input) {
	t.Helper()
	run := Run{ID: runID, Scope: "task-done", Target: map[string]string{"type": "advisory", "id": "t"}, RepoRoot: root, GitHead: head}
	if _, err := Reconcile(root, run, []Input{input}, Options{}); err != nil {
		t.Fatal(err)
	}
}

// TestReraisedFindingFollowsTheBranch is the lens finding on #178's first cut: task t's finding, raised on branch-a and
// then raised again on branch-b (branch-a still live), is one row. It must stay branch-b's after branch-b rewrites the
// head it was recorded at — before, the row kept branch-a's name with branch-b's head, and an amend on branch-b handed
// it back to branch-a, silently clearing branch-b's gate.
func TestReraisedFindingFollowsTheBranch(t *testing.T) {
	root, git := scopeRepo(t)
	input := unsafeEval("eval")
	git("switch", "-q", "-c", "branch-a")
	git("commit", "-q", "--allow-empty", "-m", "A")
	reconcileOn(t, root, "mrv-a", git("rev-parse", "HEAD"), input)
	git("switch", "-q", "-c", "branch-b", "main")
	git("commit", "-q", "--allow-empty", "-m", "B")
	reconcileOn(t, root, "mrv-b", git("rev-parse", "HEAD"), input)
	git("commit", "-q", "--amend", "--allow-empty", "-m", "B, amended")
	if in, elsewhere, err := ScopedBlocking(root); err != nil || len(in) != 1 || len(elsewhere) != 0 {
		t.Fatalf("after an amend on branch-b its re-raised finding must still block it: in=%s elsewhere=%s err=%v", ids(in), ids(elsewhere), err)
	}
}

// TestScopedBlockingSurvivesRewrites is #178 AC-4.3 in the ledger alone (no review log to block by itself): a branch's
// own finding keeps blocking it after `git rebase main`, `git commit --amend` and `git branch -m`.
func TestScopedBlockingSurvivesRewrites(t *testing.T) {
	root, git := scopeRepo(t)
	git("switch", "-q", "-c", "feat")
	git("commit", "-q", "--allow-empty", "-m", "F")
	reconcileOn(t, root, "mrv-f", git("rev-parse", "HEAD"), unsafeEval("eval"))
	blocks := func(step string) {
		t.Helper()
		if in, elsewhere, err := ScopedBlocking(root); err != nil || len(in) != 1 {
			t.Fatalf("after %s feat's own finding must still block it: in=%s elsewhere=%s err=%v", step, ids(in), ids(elsewhere), err)
		}
	}
	git("switch", "-q", "main")
	git("commit", "-q", "--allow-empty", "-m", "main advances")
	git("switch", "-q", "feat")
	git("rebase", "-q", "main")
	blocks("git rebase main")
	git("commit", "-q", "--amend", "--allow-empty", "-m", "F, amended")
	blocks("git commit --amend")
	git("branch", "-m", "feat-renamed")
	blocks("git branch -m")
	git("switch", "-q", "main")
	if in, _, _ := ScopedBlocking(root); len(in) != 0 {
		t.Fatalf("on main, feat's finding must not block: in=%s", ids(in))
	}
}

// TestUnresolvedBlockingAllBranches: a caller that names its targets (epic-ready's child tasks) sees every unresolved
// blocker, whichever branch raised it; resolved rows and read errors behave as UnresolvedBlocking's do.
func TestUnresolvedBlockingAllBranches(t *testing.T) {
	root, git := scopeRepo(t)
	git("switch", "-q", "-c", "child")
	git("commit", "-q", "--allow-empty", "-m", "child")
	head := git("rev-parse", "HEAD")
	git("switch", "-q", "-c", "epic", "main")
	fixed := blockerOn("fixed", "child", head)
	fixed.Status = "fixed"
	seedRecords(t, root, blockerOn("on-child", "child", head), fixed)
	if in, _ := UnresolvedBlocking(root); len(in) != 0 {
		t.Fatalf("the branch-scoped read leaves the child branch's blocker out on epic, got %s", ids(in))
	}
	if all, err := UnresolvedBlockingAllBranches(root); err != nil || ids(all) != "on-child" {
		t.Fatalf("all branches: %s %v", ids(all), err)
	}
	if err := os.WriteFile(findingsPath(root), []byte("{not json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := UnresolvedBlockingAllBranches(root); err == nil {
		t.Fatal("an unreadable ledger must fail")
	}
}
