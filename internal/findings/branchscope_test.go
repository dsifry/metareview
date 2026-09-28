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

// TestReconcileRecordsTheBranch: a new finding records the checked-out branch, and a detached run none; a detached run
// deduplicates against a row that gates it — named or branchless — and never moves it; raised on another branch, the
// finding gets that branch's own row.
func TestReconcileRecordsTheBranch(t *testing.T) {
	root, git := scopeRepo(t)
	git("switch", "-q", "-c", "feat")
	git("commit", "-q", "--allow-empty", "-m", "F")
	featHead := git("rev-parse", "HEAD")
	input := unsafeEval("eval")
	reconcileOn(t, root, "mrv-1", featHead, input)
	if got := loadOne(t, root); got.Branch != "feat" {
		t.Fatalf("a new finding records the checked-out branch, got %q", got.Branch)
	}

	git("switch", "-q", "--detach")
	git("commit", "-q", "--allow-empty", "-m", "detached")
	reconcileOn(t, root, "mrv-2", git("rev-parse", "HEAD"), input)
	if got := loadOne(t, root); got.Branch != "feat" || got.GitHead != featHead {
		t.Fatalf("a detached run deduplicates against the named row that gates it and never re-stamps it: %+v", got)
	}

	git("switch", "-q", "--detach", "main")
	git("commit", "-q", "--allow-empty", "-m", "detached elsewhere")
	reconcileOn(t, root, "mrv-3", git("rev-parse", "HEAD"), input)
	records := readRecords(t, root)
	if len(records) != 2 || records[1].Branch != "" {
		t.Fatalf("a detached run whose HEAD no row gates records its own branchless row: %+v", records)
	}
	first := records[1].GitHead
	git("commit", "-q", "--allow-empty", "-m", "detached elsewhere, again")
	reconcileOn(t, root, "mrv-4", git("rev-parse", "HEAD"), input)
	if records = readRecords(t, root); len(records) != 2 || records[1].GitHead != first {
		t.Fatalf("a detached run deduplicates against the branchless row that gates it and never moves it: %+v", records)
	}

	git("switch", "-q", "-c", "other", "main")
	reconcileOn(t, root, "mrv-5", git("rev-parse", "HEAD"), input)
	if records = readRecords(t, root); len(records) != 3 || records[2].Branch != "other" {
		t.Fatalf("raised on another branch, the finding gets that branch's own row: %+v", records)
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

// TestReraisedFindingFollowsTheBranch is a lens finding on #178's first cut: task t's finding, raised on branch-a and
// then raised again on branch-b (branch-a still live), must stay branch-b's after branch-b rewrites the head it was
// recorded at — the first cut kept one row with branch-a's name and branch-b's head, and an amend on branch-b handed it
// back to branch-a, silently clearing branch-b's gate. Each branch now has a row of its own.
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
	if in, elsewhere, err := ScopedBlocking(root); err != nil || ids(in) != "mrvf-b-001" || ids(elsewhere) != "mrvf-a-001" {
		t.Fatalf("after an amend on branch-b its re-raised finding must still block it: in=%s elsewhere=%s err=%v", ids(in), ids(elsewhere), err)
	}
}

// TestReraisedFindingKeepsTheFirstBranchsObligation is the recheck finding on the second cut, which moved the one row
// to whichever branch raised the finding last: raising it again elsewhere — a stacked branch, or a throwaway branch
// deleted afterwards — must never take it from the branch that raised it first, which still has the defect.
func TestReraisedFindingKeepsTheFirstBranchsObligation(t *testing.T) {
	root, git := scopeRepo(t)
	input := unsafeEval("eval")
	git("switch", "-q", "-c", "branch-a")
	git("commit", "-q", "--allow-empty", "-m", "A")
	reconcileOn(t, root, "mrv-a", git("rev-parse", "HEAD"), input)

	git("switch", "-q", "-c", "branch-b") // stacked on branch-a
	git("commit", "-q", "--allow-empty", "-m", "B")
	reconcileOn(t, root, "mrv-b", git("rev-parse", "HEAD"), input)
	git("switch", "-q", "branch-a")
	if in, _, err := ScopedBlocking(root); err != nil || !strings.Contains(ids(in), "mrvf-a-001") {
		t.Fatalf("a stacked branch raising it again must not take branch-a's finding: in=%s err=%v", ids(in), err)
	}

	git("switch", "-q", "-c", "tmp")
	reconcileOn(t, root, "mrv-tmp", git("rev-parse", "HEAD"), input)
	git("switch", "-q", "branch-a")
	git("branch", "-q", "-D", "tmp")
	git("commit", "-q", "--amend", "--allow-empty", "-m", "A, amended")
	if in, _, err := ScopedBlocking(root); err != nil || !strings.Contains(ids(in), "mrvf-a-001") {
		t.Fatalf("a throwaway branch raising it again must not orphan branch-a's finding: in=%s err=%v", ids(in), err)
	}

	// A --previous-run chain closes every row it names, whichever branch recorded it.
	chain := func(runID string, previous ...string) {
		t.Helper()
		run := Run{ID: runID, Scope: "task-done", Target: map[string]string{"type": "advisory", "id": "t"}, RepoRoot: root, GitHead: git("rev-parse", "HEAD")}
		if _, err := Reconcile(root, run, nil, Options{PreviousRunIDs: previous}); err != nil {
			t.Fatal(err)
		}
	}
	status := func() map[string]string {
		got := map[string]string{}
		for _, r := range readRecords(t, root) {
			got[r.RunID] = r.Status
		}
		return got
	}
	// The deleted tmp's row is orphaned from an unrelated branch (not in its range, no live owner): the chain that
	// names it still closes it, so it is never stranded.
	git("switch", "-q", "-c", "unrelated", "main")
	chain("mrv-u", "mrv-tmp")
	if got := status(); got["mrv-tmp"] != "fixed" || got["mrv-a"] != "open" {
		t.Fatalf("a chain closes the orphaned row it names, and only that row: %v", got)
	}
	git("switch", "-q", "branch-b")
	chain("mrv-b2", "mrv-a", "mrv-b")
	if got := status(); got["mrv-a"] != "fixed" || got["mrv-b"] != "fixed" {
		t.Fatalf("stacked branch-b's chain closes its own row and the lower branch's row it inherits: %v", got)
	}
}

// TestFixBranchChainClosesTheFindingItInherits is a recheck finding on the third cut: a branch cut from the one that
// raised a finding (a fix branch) is blocked by it through the range leg, so its --previous-run fix chain must be able
// to close it — the third cut let only the owning branch's name close a row.
func TestFixBranchChainClosesTheFindingItInherits(t *testing.T) {
	root, git := scopeRepo(t)
	git("switch", "-q", "-c", "feat")
	git("commit", "-q", "--allow-empty", "-m", "F")
	reconcileOn(t, root, "mrv-a", git("rev-parse", "HEAD"), unsafeEval("eval"))
	git("switch", "-q", "-c", "feat-fix")
	git("commit", "-q", "--allow-empty", "-m", "the fix")
	run := Run{ID: "mrv-fix", Scope: "task-done", Target: map[string]string{"type": "advisory", "id": "t"}, RepoRoot: root, GitHead: git("rev-parse", "HEAD")}
	if _, err := Reconcile(root, run, nil, Options{PreviousRunIDs: []string{"mrv-a"}}); err != nil {
		t.Fatal(err)
	}
	if in, _, _ := ScopedBlocking(root); len(in) != 0 {
		t.Fatalf("the fix branch's chain must close the finding it inherited: in=%s", ids(in))
	}
}

// TestReconcileVerdictCountsOnlyThisBranchsRows is a recheck finding on the third cut: the open findings Reconcile
// returns (task-done's and epic-ready's verdict) are the ones that gate this branch, not every row for the target —
// another live branch's row for the same task must never fail this branch's review, which cannot close it.
func TestReconcileVerdictCountsOnlyThisBranchsRows(t *testing.T) {
	root, git := scopeRepo(t)
	git("switch", "-q", "-c", "branch-a")
	git("commit", "-q", "--allow-empty", "-m", "A")
	reconcileOn(t, root, "mrv-a", git("rev-parse", "HEAD"), unsafeEval("eval"))
	git("switch", "-q", "-c", "branch-b", "main")
	git("commit", "-q", "--allow-empty", "-m", "B")
	target := map[string]string{"type": "advisory", "id": "t"}
	head := git("rev-parse", "HEAD")
	result, err := Reconcile(root, Run{ID: "mrv-b", Scope: "task-done", Target: target, RepoRoot: root, GitHead: head}, nil, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if result.OpenBlockingCount != 0 || len(result.OpenFindings) != 0 {
		t.Fatalf("branch-a's row must not count in branch-b's verdict: %s", ids(result.OpenFindings))
	}
	result, err = Reconcile(root, Run{ID: "mrv-b2", Scope: "task-done", Target: target, RepoRoot: root, GitHead: head}, []Input{unsafeEval("eval")}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if ids(result.OpenFindings) != "mrvf-b2-001" || ids(result.Findings) != "mrvf-b2-001" {
		t.Fatalf("raised on branch-b, only branch-b's own row counts: open=%s findings=%s", ids(result.OpenFindings), ids(result.Findings))
	}
}

// TestBranchlessRowIsNeverTakenByAnotherBranch is a recheck finding on the third cut: a row with no branch (from before
// #178, or recorded on a detached HEAD) is never re-stamped by a named branch that raises the finding again, so a
// throwaway branch cannot carry a legacy blocker away from the branch that still has the defect.
func TestBranchlessRowIsNeverTakenByAnotherBranch(t *testing.T) {
	root, git := scopeRepo(t)
	git("switch", "-q", "-c", "branch-a")
	git("commit", "-q", "--allow-empty", "-m", "A")
	input := unsafeEval("eval")
	reconcileOn(t, root, "mrv-legacy", git("rev-parse", "HEAD"), input)
	legacy := loadOne(t, root)
	legacy.Branch = ""
	seedRecords(t, root, legacy)

	git("switch", "-q", "-c", "tmp")
	git("commit", "-q", "--allow-empty", "-m", "tmp")
	reconcileOn(t, root, "mrv-tmp", git("rev-parse", "HEAD"), input)
	if got := readRecords(t, root)[0]; got.Branch != "" || got.GitHead != legacy.GitHead {
		t.Fatalf("a named branch must not re-stamp a branchless row: %+v", got)
	}
	git("switch", "-q", "branch-a")
	git("branch", "-q", "-D", "tmp")
	git("commit", "-q", "--amend", "--allow-empty", "-m", "A, amended")
	if in, _, _ := ScopedBlocking(root); !strings.Contains(ids(in), legacy.ID) {
		t.Fatalf("branch-a's legacy blocker must still block it: in=%s", ids(in))
	}
}

// TestReraisedOverridePendingFindingBlocksTheNewBranch: a finding whose override is only requested still blocks. Raised
// again on an unrelated branch, it must block there too — the first cut wrote no row for the new branch.
func TestReraisedOverridePendingFindingBlocksTheNewBranch(t *testing.T) {
	root, git := scopeRepo(t)
	input := unsafeEval("eval")
	git("switch", "-q", "-c", "branch-a")
	git("commit", "-q", "--allow-empty", "-m", "A")
	reconcileOn(t, root, "mrv-a", git("rev-parse", "HEAD"), input)
	pending := loadOne(t, root)
	pending.Status = StatusOverridePending
	seedRecords(t, root, pending)

	git("switch", "-q", "-c", "branch-b", "main")
	git("commit", "-q", "--allow-empty", "-m", "B")
	reconcileOn(t, root, "mrv-b", git("rev-parse", "HEAD"), input)
	if in, _, err := ScopedBlocking(root); err != nil || ids(in) != "mrvf-b-001" {
		t.Fatalf("raised again on branch-b, the finding must block branch-b: in=%s err=%v", ids(in), err)
	}
}

// TestNameLegAloneKeepsARecordedBranchsFinding: a finding whose recorded head git no longer has (pruned after a rewrite)
// blocks its branch by name alone, and nowhere else; a legacy row at the same head blocks nothing. This is the case the
// legacy rule cannot cover, so it fails if findings stop recording their branch.
func TestNameLegAloneKeepsARecordedBranchsFinding(t *testing.T) {
	root, git := scopeRepo(t)
	git("switch", "-q", "-c", "feat")
	gone := strings.Repeat("f", 40)
	seedRecords(t, root, blockerOn("named", "feat", gone), blockerOn("legacy", "", gone))
	if in, elsewhere, _ := ScopedBlocking(root); ids(in) != "named" || ids(elsewhere) != "legacy" {
		t.Fatalf("on feat: in=%s elsewhere=%s", ids(in), ids(elsewhere))
	}
	git("switch", "-q", "main")
	if in, _, _ := ScopedBlocking(root); len(in) != 0 {
		t.Fatalf("on main: in=%s", ids(in))
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
	// Raised again after the rename, it is still the same branch's one row, now under the new name.
	reconcileOn(t, root, "mrv-f2", git("rev-parse", "HEAD"), unsafeEval("eval"))
	if records := readRecords(t, root); len(records) != 1 || records[0].Branch != "feat-renamed" {
		t.Fatalf("a renamed branch refreshes its own row: %+v", records)
	}
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

// TestDetachedRunRefreshesOnlyRowsThatGateIt is a recheck finding on the fourth cut: a detached run refreshed every
// branchless row for the target, so a task-done on a detached HEAD from another lineage moved branch-a's legacy blocker
// to its own head and branch-a's gate cleared without a fix.
func TestDetachedRunRefreshesOnlyRowsThatGateIt(t *testing.T) {
	root, git := scopeRepo(t)
	git("switch", "-q", "-c", "branch-a")
	git("commit", "-q", "--allow-empty", "-m", "A")
	input := unsafeEval("eval")
	reconcileOn(t, root, "mrv-legacy", git("rev-parse", "HEAD"), input)
	legacy := loadOne(t, root)
	legacy.Branch = ""
	seedRecords(t, root, legacy)

	git("switch", "-q", "--detach", "main")
	git("commit", "-q", "--allow-empty", "-m", "elsewhere")
	reconcileOn(t, root, "mrv-detached", git("rev-parse", "HEAD"), input)
	if got := readRecords(t, root)[0]; got.GitHead != legacy.GitHead {
		t.Fatalf("a detached run must not move a branchless row that does not gate it: %+v", got)
	}
	git("switch", "-q", "branch-a")
	if in, _, _ := ScopedBlocking(root); !strings.Contains(ids(in), legacy.ID) {
		t.Fatalf("branch-a's legacy blocker must still block it: in=%s", ids(in))
	}
}

// TestNamedRunDedupesAgainstABranchlessRowThatGatesIt: raised again on the branch whose legacy row gates it, the finding
// is that row — no duplicate named row beside it.
func TestNamedRunDedupesAgainstABranchlessRowThatGatesIt(t *testing.T) {
	root, git := scopeRepo(t)
	git("switch", "-q", "-c", "feat")
	git("commit", "-q", "--allow-empty", "-m", "F")
	input := unsafeEval("eval")
	reconcileOn(t, root, "mrv-legacy", git("rev-parse", "HEAD"), input)
	legacy := loadOne(t, root)
	legacy.Branch = ""
	seedRecords(t, root, legacy)
	reconcileOn(t, root, "mrv-again", git("rev-parse", "HEAD"), input)
	if records := readRecords(t, root); len(records) != 1 || records[0].Branch != "" {
		t.Fatalf("a named run must not write a duplicate beside the branchless row that gates it: %+v", records)
	}
}

// TestDetachedRunDedupesAgainstTheRowThatGatesIt is a recheck finding on the fifth cut: a detached review at a branch's
// own head wrote a second, branchless row beside the branch's named row, so the branch's own fix chain closed its row
// and the duplicate kept blocking. Before #178 the re-raise deduplicated.
func TestDetachedRunDedupesAgainstTheRowThatGatesIt(t *testing.T) {
	root, git := scopeRepo(t)
	git("switch", "-q", "-c", "feat")
	git("commit", "-q", "--allow-empty", "-m", "F")
	input := unsafeEval("eval")
	reconcileOn(t, root, "mrv-feat", git("rev-parse", "HEAD"), input)
	git("switch", "-q", "--detach")
	reconcileOn(t, root, "mrv-det", git("rev-parse", "HEAD"), input)
	if records := readRecords(t, root); len(records) != 1 || records[0].Branch != "feat" {
		t.Fatalf("a detached re-raise at the branch's head must not duplicate its row: %+v", records)
	}
}

// TestFreshnessSupersedeOnlyForRowsThatGateTheRun: fresh mutation evidence on one branch never supersedes another live
// branch's stale-mutation blocker for the same target — that branch never re-ran mutation testing.
func TestFreshnessSupersedeOnlyForRowsThatGateTheRun(t *testing.T) {
	root, git := scopeRepo(t)
	engines := Options{MutationEngines: []string{"stryker"}}
	target := map[string]string{"type": "advisory", "id": "t"}
	git("switch", "-q", "-c", "branch-a")
	git("commit", "-q", "--allow-empty", "-m", "A")
	if _, err := Reconcile(root, Run{ID: "r-a", Scope: "task-done", Target: target, GitHead: git("rev-parse", "HEAD")}, []Input{staleInput()}, engines); err != nil {
		t.Fatal(err)
	}
	git("switch", "-q", "-c", "branch-b", "main")
	git("commit", "-q", "--allow-empty", "-m", "B")
	if _, err := Reconcile(root, Run{ID: "r-b", Scope: "task-done", Target: target, GitHead: git("rev-parse", "HEAD")}, nil, engines); err != nil {
		t.Fatal(err)
	}
	if got := statusOf(t, root, staleFP); len(got) != 1 || got[0] != "open/" {
		t.Fatalf("branch-b's fresh evidence must not supersede branch-a's stale row: %v", got)
	}
}

// TestBranchCutAfterADetachedReviewKeepsItsOwnRow is a recheck finding on the sixth cut: a finding recorded on a
// detached HEAD (a branchless row), raised again on a branch cut later from a descendant commit, deduplicated onto
// that row — which is not in the branch's reflog, so a routine rebase then orphaned it and the branch had no row of
// its own. The branch now records its own row unless the branchless row's head is one of its past heads.
func TestBranchCutAfterADetachedReviewKeepsItsOwnRow(t *testing.T) {
	root, git := scopeRepo(t)
	input := unsafeEval("eval")
	git("switch", "-q", "--detach")
	git("commit", "-q", "--allow-empty", "-m", "D1")
	reconcileOn(t, root, "mrv-det", git("rev-parse", "HEAD"), input)
	git("commit", "-q", "--allow-empty", "-m", "D2")
	git("switch", "-q", "-c", "feat")
	git("commit", "-q", "--allow-empty", "-m", "F")
	reconcileOn(t, root, "mrv-feat", git("rev-parse", "HEAD"), input)
	git("switch", "-q", "main")
	git("commit", "-q", "--allow-empty", "-m", "main advances")
	git("switch", "-q", "feat")
	git("rebase", "-q", "main")
	if in, _, _ := ScopedBlocking(root); !strings.Contains(ids(in), "mrvf-feat-001") {
		t.Fatalf("after a rebase feat must still block on the finding it raised itself: in=%s", ids(in))
	}
}

// TestGrantedOverrideOfALowerBranchAbsorbsTheReraise is a recheck finding on the sixth cut: a stacked branch raising a
// finding again whose override a human already granted on the lower branch it inherits the code from must not get a
// fresh open row demanding a second grant (before #178 the overridden row deduplicated the re-raise).
func TestGrantedOverrideOfALowerBranchAbsorbsTheReraise(t *testing.T) {
	root, git := scopeRepo(t)
	input := unsafeEval("eval")
	git("switch", "-q", "-c", "lower")
	git("commit", "-q", "--allow-empty", "-m", "L")
	reconcileOn(t, root, "mrv-lower", git("rev-parse", "HEAD"), input)
	granted := loadOne(t, root)
	granted.Status = StatusOverridden
	seedRecords(t, root, granted)
	git("switch", "-q", "-c", "upper")
	git("commit", "-q", "--allow-empty", "-m", "U")
	reconcileOn(t, root, "mrv-upper", git("rev-parse", "HEAD"), input)
	if records := readRecords(t, root); len(records) != 1 {
		t.Fatalf("the lower branch's granted override must absorb the stacked branch's re-raise: %+v", records)
	}
}

// TestThrowawayDetachedReviewNeverMovesABranchsRow is a recheck finding on the seventh cut: a detached run refreshed a
// branchless row that gated it, moving its head — so a review on a throwaway detached commit carried feat's only row
// off feat's history, and feat's gate cleared with the defect still present. No run moves a branchless row now.
func TestThrowawayDetachedReviewNeverMovesABranchsRow(t *testing.T) {
	root, git := scopeRepo(t)
	input := unsafeEval("eval")
	git("switch", "-q", "-c", "feat")
	git("commit", "-q", "--allow-empty", "-m", "F")
	git("switch", "-q", "--detach")
	reconcileOn(t, root, "mrv-det1", git("rev-parse", "HEAD"), input)
	git("switch", "-q", "feat")
	reconcileOn(t, root, "mrv-feat", git("rev-parse", "HEAD"), input)
	git("switch", "-q", "--detach")
	git("commit", "-q", "--allow-empty", "-m", "scratch")
	reconcileOn(t, root, "mrv-det2", git("rev-parse", "HEAD"), input)
	git("switch", "-q", "feat")
	if in, _, _ := ScopedBlocking(root); len(in) == 0 {
		t.Fatal("a review on a throwaway detached commit must not clear feat's blocker")
	}
}

// TestPartlyReadScopeDedupesAsBeforeBranches: when git fails after the branch name is read (the scope is unknown), a
// re-raise deduplicates against every row that gates it — which is every row — as before #178, rather than writing a
// duplicate beside a renamed branch's row.
func TestPartlyReadScopeDedupesAsBeforeBranches(t *testing.T) {
	root, git := scopeRepo(t)
	git("switch", "-q", "-c", "feat")
	git("commit", "-q", "--allow-empty", "-m", "F")
	input := unsafeEval("eval")
	reconcileOn(t, root, "mrv-1", git("rev-parse", "HEAD"), input)
	orig := loadScope
	loadScope = func(string) scope.Scope { return scope.Scope{Current: "feat-renamed"} }
	t.Cleanup(func() { loadScope = orig })
	reconcileOn(t, root, "mrv-2", git("rev-parse", "HEAD"), input)
	if records := readRecords(t, root); len(records) != 1 {
		t.Fatalf("an unknown scope must deduplicate the re-raise: %+v", records)
	}
}

// TestPartlyReadScopeNeverCarriesAnotherBranchsRow is a recheck finding on the eighth cut: when git read the branch name
// and then failed (the scope is unknown), the refresh re-stamped another live branch's row with this branch's name and
// head, so once git recovered that branch's own finding read as another branch's and its gate cleared.
func TestPartlyReadScopeNeverCarriesAnotherBranchsRow(t *testing.T) {
	root, git := scopeRepo(t)
	input := unsafeEval("eval")
	git("switch", "-q", "-c", "branch-a")
	git("commit", "-q", "--allow-empty", "-m", "A")
	reconcileOn(t, root, "mrv-a", git("rev-parse", "HEAD"), input)
	git("switch", "-q", "-c", "branch-b", "main")
	git("commit", "-q", "--allow-empty", "-m", "B")
	orig := loadScope
	loadScope = func(string) scope.Scope { return scope.Scope{Current: "branch-b"} }
	reconcileOn(t, root, "mrv-b", git("rev-parse", "HEAD"), input)
	loadScope = orig
	git("switch", "-q", "branch-a")
	if in, _, _ := ScopedBlocking(root); !strings.Contains(ids(in), "mrvf-a-001") {
		t.Fatalf("a partly read scope on branch-b must not take branch-a's row: in=%s rows=%+v", ids(in), readRecords(t, root))
	}
}

// TestSquashMergedAndDeletedBranchBlocksNothing is #178 AC-4.6 for a squash merge (this repository's own merge
// style): the recorded head never reaches main, the branch is gone, and its finding blocks nothing afterwards.
func TestSquashMergedAndDeletedBranchBlocksNothing(t *testing.T) {
	root, git := scopeRepo(t)
	git("switch", "-q", "-c", "feat")
	git("commit", "-q", "--allow-empty", "-m", "F")
	reconcileOn(t, root, "mrv-f", git("rev-parse", "HEAD"), unsafeEval("eval"))
	git("switch", "-q", "main")
	git("merge", "-q", "--squash", "feat")
	git("commit", "-q", "--allow-empty", "-m", "squash feat")
	git("branch", "-q", "-D", "feat")
	if in, _, _ := ScopedBlocking(root); len(in) != 0 {
		t.Fatalf("on main after the squash merge: in=%s", ids(in))
	}
	git("switch", "-q", "-c", "next")
	git("commit", "-q", "--allow-empty", "-m", "N")
	if in, _, _ := ScopedBlocking(root); len(in) != 0 {
		t.Fatalf("on a branch cut after the squash merge: in=%s", ids(in))
	}
}
