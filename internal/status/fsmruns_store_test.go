package status

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeStoreRun writes an abandoned run into git's common-dir store (#173) with the branch and head its init recorded
// (#177). An empty branch is a run from before branches were recorded.
func writeStoreRun(t *testing.T, common, id, branch, head string) {
	t.Helper()
	writeRunAt(t, filepath.Join(common, "metareview", "runs", id), branch, head)
}

func writeRunAt(t *testing.T, dir, branch, head string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "workflow.yaml"), []byte(testWorkflow), 0o600); err != nil {
		t.Fatal(err)
	}
	audit := `{"type":"init","at":"2026-09-28T00:00:00Z","state":"discover","data":{"workflow":"t","branch":"` + branch + `","head":"` + head + `"}}` + "\n" +
		`{"type":"transition","at":"2026-09-28T00:00:01Z","state":"discover","data":{"to":"fix","to_kind":"agent-edit"}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "audit.jsonl"), []byte(audit), 0o600); err != nil {
		t.Fatal(err)
	}
}

// gitEnv is the environment every fixture git runs in: no inherited GIT_* (a hook's GIT_DIR would aim it at another
// repository), no global or system config.
func gitEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			env = append(env, kv)
		}
	}
	return append(env, "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_EDITOR=true")
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = gitEnv()
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", append([]string{"-C", dir}, args...)...)
	c.Env = gitEnv()
	out, err := c.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

func ids(runs []AbandonedRun) []string {
	var out []string
	for _, r := range runs {
		out = append(out, r.RunID)
	}
	return out
}

// newRepo is a repository on main with one commit; it returns the checkout and its common dir.
func newRepo(t *testing.T) (root, common string) {
	t.Helper()
	root, _ = filepath.EvalSymlinks(t.TempDir())
	gitRun(t, root, "init", "-q", "-b", "main")
	gitRun(t, root, "commit", "-q", "--allow-empty", "-m", "base")
	return root, filepath.Join(root, ".git")
}

// commit makes an empty commit in dir and returns it.
func commit(t *testing.T, dir, msg string) string {
	t.Helper()
	gitRun(t, dir, "commit", "-q", "--allow-empty", "-m", msg)
	return gitOut(t, dir, "rev-parse", "HEAD")
}

// AC-4.2: an abandoned run started in a linked worktree blocks status there; it does not block the main checkout,
// and `--all` lists it there as another branch's.
func TestAbandonedRunsAreScopedByBranch(t *testing.T) {
	root, common := newRepo(t)
	wt := filepath.Join(filepath.Dir(root), "wt-"+filepath.Base(root))
	gitRun(t, root, "worktree", "add", "-q", "-b", "feat", wt)
	featHead := commit(t, wt, "feat work")
	mainHead := gitOut(t, root, "rev-parse", "HEAD")
	writeStoreRun(t, common, "mrv-feat-0000001", "feat", featHead)
	writeStoreRun(t, common, "mrv-main-0000001", "main", mainHead)
	if got := strings.Join(ids(DiscoverAbandonedRuns(wt)), ","); got != "mrv-feat-0000001" {
		t.Fatalf("the linked worktree on feat: got %s", got)
	}
	if got := strings.Join(ids(DiscoverAbandonedRuns(root)), ","); got != "mrv-main-0000001" {
		t.Fatalf("the main checkout: got %s", got)
	}
	_, elsewhere := ScanAbandonedRuns(root)
	if len(elsewhere) != 1 || elsewhere[0].RunID != "mrv-feat-0000001" || elsewhere[0].Scope != "other-branch" || elsewhere[0].Branch != "feat" {
		t.Fatalf("--all from main lists feat's run as another branch's: %+v", elsewhere)
	}
	r, err := Build(root)
	if err != nil {
		t.Fatal(err)
	}
	if r.OtherBranchRuns != 1 || !strings.Contains(strings.Join(r.Warnings, "\n"), "`metareview status --all` lists them with the directory to delete") {
		t.Fatalf("status counts the other branch's run and points at --all: %+v %v", r.OtherBranchRuns, r.Warnings)
	}
}

// AC-4.3: rebase does not clear it — the name leg holds after the recorded head becomes unreachable.
func TestRebaseDoesNotClearABranchRun(t *testing.T) {
	root, common := newRepo(t)
	gitRun(t, root, "checkout", "-q", "-b", "feat")
	h := commit(t, root, "feat work")
	writeStoreRun(t, common, "mrv-rebase-00001", "feat", h)
	gitRun(t, root, "checkout", "-q", "main")
	commit(t, root, "main moves on")
	gitRun(t, root, "checkout", "-q", "feat")
	gitRun(t, root, "rebase", "-q", "main")
	if gitOut(t, root, "rev-parse", "HEAD") == h {
		t.Fatal("setup: the rebase must rewrite the recorded head")
	}
	if got := strings.Join(ids(DiscoverAbandonedRuns(root)), ","); got != "mrv-rebase-00001" {
		t.Fatalf("after `git rebase main` the run must still block feat, got %q", got)
	}
}

// The rename bypass: amend (the head leaves the range), then `git branch -m` — the rename entry the branch's reflog
// carries makes the old name one of its former names, so the name leg still holds. Mid-rebase HEAD is detached, and the
// branch being rebased keeps its runs while an agent sits on the conflict.
func TestARewriteThenRenameDoesNotClearABranchRun(t *testing.T) {
	root, common := newRepo(t)
	gitRun(t, root, "checkout", "-q", "-b", "feat")
	h := commit(t, root, "feat work")
	writeStoreRun(t, common, "mrv-rename-00001", "feat", h)
	gitRun(t, root, "commit", "-q", "--amend", "--allow-empty", "-m", "feat work v2")
	gitRun(t, root, "branch", "-m", "feat", "feat-v2")
	if got := strings.Join(ids(DiscoverAbandonedRuns(root)), ","); got != "mrv-rename-00001" {
		t.Fatalf("an amended and renamed branch must still be blocked by its run, got %q", got)
	}
	gitRun(t, root, "checkout", "-q", "main")
	if got := DiscoverAbandonedRuns(root); len(got) != 0 {
		t.Fatalf("main does not own feat-v2's run: %v", ids(got))
	}
}

// A legacy run (no branch) on a feature branch survives that branch's rebase through the branch's reflog: an upgrade
// never turns a routine rebase into a cleared gate.
func TestALegacyRunSurvivesItsBranchsRebase(t *testing.T) {
	root, common := newRepo(t)
	gitRun(t, root, "checkout", "-q", "-b", "feat")
	h := commit(t, root, "feat work")
	writeStoreRun(t, common, "mrv-legacy-rb001", "", h)
	gitRun(t, root, "checkout", "-q", "main")
	commit(t, root, "main moves on")
	gitRun(t, root, "checkout", "-q", "feat")
	gitRun(t, root, "rebase", "-q", "main")
	if got := strings.Join(ids(DiscoverAbandonedRuns(root)), ","); got != "mrv-legacy-rb001" {
		t.Fatalf("a legacy run must keep blocking its rebased branch, got %q", got)
	}
}

// A branch created on another feature branch and later moved onto main no longer inherits that branch's runs: once its
// history no longer holds the parent's commits, the parent's runs are the parent's.
func TestAReRootedBranchDropsItsFormerParentsRuns(t *testing.T) {
	root, common := newRepo(t)
	gitRun(t, root, "checkout", "-q", "-b", "feat-a")
	a := commit(t, root, "a")
	writeStoreRun(t, common, "mrv-parent-a0001", "feat-a", a)
	gitRun(t, root, "checkout", "-q", "-b", "feat-b") // stacked on feat-a
	commit(t, root, "b")
	if got := strings.Join(ids(DiscoverAbandonedRuns(root)), ","); got != "mrv-parent-a0001" {
		t.Fatalf("while stacked, feat-a's run blocks feat-b (AC-4.5), got %q", got)
	}
	gitRun(t, root, "rebase", "-q", "--onto", "main", "feat-a", "feat-b")
	if got := DiscoverAbandonedRuns(root); len(got) != 0 {
		t.Fatalf("re-rooted onto main, feat-b no longer carries feat-a's work: %v", ids(got))
	}
}

// A tag that shares the branch's name makes git's short names ambiguous ("heads/feat"); the name leg compares full
// refnames, so it still matches — on the default-branch layout too, where there is no range to fall back on.
func TestATagNamedLikeTheBranchDoesNotHideItsRun(t *testing.T) {
	root, common := newRepo(t)
	gitRun(t, root, "checkout", "-q", "-b", "feat")
	h := commit(t, root, "feat work")
	writeStoreRun(t, common, "mrv-tagged-00001", "feat", h)
	gitRun(t, root, "tag", "feat")
	gitRun(t, root, "branch", "-q", "-f", "main", "refs/heads/feat") // no fork point: only the name leg can hold it
	if got := strings.Join(ids(DiscoverAbandonedRuns(root)), ","); got != "mrv-tagged-00001" {
		t.Fatalf("a same-named tag must not unmatch the branch, got %q", got)
	}
}

// A rebase begun on a detached HEAD writes "detached HEAD" as its head-name: that is no branch, and it must not break
// the scope into blocking every run in the repository.
func TestADetachedRebaseIsNoBranch(t *testing.T) {
	root, common := newRepo(t)
	gitRun(t, root, "checkout", "-q", "-b", "feat")
	// The conflict comes at the second pick, so HEAD has moved past main and there is a fork point to scope by.
	for _, f := range []string{"g.txt", "f.txt"} {
		if err := os.WriteFile(filepath.Join(root, f), []byte("feat\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		gitRun(t, root, "add", f)
		gitRun(t, root, "commit", "-q", "-m", f+" on feat")
	}
	gitRun(t, root, "checkout", "-q", "main")
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "add", "f.txt")
	gitRun(t, root, "commit", "-q", "-m", "f on main")
	writeStoreRun(t, common, "mrv-othermain001", "main", gitOut(t, root, "rev-parse", "HEAD"))
	gitRun(t, root, "checkout", "-q", "--detach", "feat")
	c := exec.Command("git", "rebase", "main")
	c.Dir = root
	c.Env = gitEnv()
	if err := c.Run(); err == nil {
		t.Fatal("setup: the rebase must stop on a conflict")
	}
	if got := DiscoverAbandonedRuns(root); len(got) != 0 {
		t.Fatalf("a detached rebase belongs to no branch: main's run must not block it, got %v", ids(got))
	}
}

// A legacy run whose head git has pruned cannot be reachable from anything: it is orphaned, not in scope everywhere.
func TestALegacyRunWhoseHeadWasPrunedIsOrphaned(t *testing.T) {
	root, common := newRepo(t)
	gitRun(t, root, "checkout", "-q", "-b", "gone")
	h := commit(t, root, "squashed away")
	writeStoreRun(t, common, "mrv-pruned-00001", "", h)
	gitRun(t, root, "checkout", "-q", "main")
	gitRun(t, root, "branch", "-q", "-D", "gone")
	gitRun(t, root, "reflog", "expire", "--expire=now", "--all")
	gitRun(t, root, "gc", "-q", "--prune=now")
	mine, elsewhere := ScanAbandonedRuns(root)
	if len(mine) != 0 || len(elsewhere) != 1 || elsewhere[0].Scope != "orphaned" {
		t.Fatalf("a pruned head is unreachable: mine %v, elsewhere %+v", ids(mine), elsewhere)
	}
}

// A stacked branch that fast-forwards to its parent's newer tip does not take the parent's runs as its own: once the
// parent is merged and deleted and the child moved onto main, the parent's run is orphaned (AC-4.6).
func TestAFastForwardedParentHeadStaysTheParents(t *testing.T) {
	root, common := newRepo(t)
	gitRun(t, root, "checkout", "-q", "-b", "feat-a")
	commit(t, root, "a1")
	gitRun(t, root, "checkout", "-q", "-b", "feat-b")
	gitRun(t, root, "checkout", "-q", "feat-a")
	a2 := commit(t, root, "a2")
	writeStoreRun(t, common, "mrv-ffparent-001", "feat-a", a2)
	gitRun(t, root, "checkout", "-q", "feat-b")
	gitRun(t, root, "merge", "-q", "--ff-only", "feat-a") // a2 is now one of feat-b's own reflog heads
	commit(t, root, "b1")
	gitRun(t, root, "branch", "-q", "-D", "feat-a")
	gitRun(t, root, "rebase", "-q", "--onto", "main", a2, "feat-b")
	mine, elsewhere := ScanAbandonedRuns(root)
	if len(mine) != 0 || len(elsewhere) != 1 || elsewhere[0].Scope != "orphaned" {
		t.Fatalf("feat-a's run is orphaned, not feat-b's: mine %v, elsewhere %+v", ids(mine), elsewhere)
	}
}

// The rename protection holds however the branch began: created on its own unpushed work (committed on main by
// mistake, then `switch -c`), and with the run recorded at the creation head before any new commit.
func TestARenameKeepsARunRecordedAtTheCreationHead(t *testing.T) {
	root, common := newRepo(t)
	w1 := commit(t, root, "w1 on main by mistake")
	gitRun(t, root, "switch", "-q", "-c", "feat")
	gitRun(t, root, "branch", "-q", "-f", "main", "HEAD~1")
	writeStoreRun(t, common, "mrv-creation-001", "feat", w1)
	gitRun(t, root, "commit", "-q", "--amend", "--allow-empty", "-m", "w1 amended")
	gitRun(t, root, "branch", "-m", "feat", "feat2")
	if got := strings.Join(ids(DiscoverAbandonedRuns(root)), ","); got != "mrv-creation-001" {
		t.Fatalf("the renamed branch must still own the run recorded at its creation head, got %q", got)
	}
	gitRun(t, root, "branch", "-m", "feat2", "feat3") // a chain of renames
	if got := strings.Join(ids(DiscoverAbandonedRuns(root)), ","); got != "mrv-creation-001" {
		t.Fatalf("every former name is still the branch's own, got %q", got)
	}
}

// A former name belongs to the renamed branch only while no live branch holds it: a new branch that reuses the name
// owns what is recorded under it, and the renamed branch is not blocked by it.
func TestAReusedFormerNameBelongsToTheNewBranch(t *testing.T) {
	root, common := newRepo(t)
	gitRun(t, root, "checkout", "-q", "-b", "feat")
	commit(t, root, "old feat")
	gitRun(t, root, "branch", "-m", "feat", "feat-backup")
	gitRun(t, root, "checkout", "-q", "-b", "feat", "main")
	h := commit(t, root, "new feat")
	writeStoreRun(t, common, "mrv-newfeat-0001", "feat", h)
	gitRun(t, root, "checkout", "-q", "feat-backup")
	mine, elsewhere := ScanAbandonedRuns(root)
	if len(mine) != 0 || len(elsewhere) != 1 || elsewhere[0].Scope != "other-branch" {
		t.Fatalf("the new feat's run is feat's, not feat-backup's: mine %v, elsewhere %+v", ids(mine), elsewhere)
	}
}

// `git branch -c` then deleting the original is a rename in two steps; the copy's reflog records where it came from.
func TestACopyThenDeleteKeepsTheRun(t *testing.T) {
	root, common := newRepo(t)
	gitRun(t, root, "checkout", "-q", "-b", "feat")
	h := commit(t, root, "feat work")
	writeStoreRun(t, common, "mrv-copied-00001", "feat", h)
	gitRun(t, root, "commit", "-q", "--amend", "--allow-empty", "-m", "feat work v2")
	gitRun(t, root, "branch", "-c", "feat", "feat2")
	gitRun(t, root, "checkout", "-q", "feat2")
	gitRun(t, root, "branch", "-q", "-D", "feat")
	if got := strings.Join(ids(DiscoverAbandonedRuns(root)), ","); got != "mrv-copied-00001" {
		t.Fatalf("the copy must still own the deleted original's run, got %q", got)
	}
}

// An unborn branch (`checkout --orphan`, before its first commit) has no reflog to read: that is not a git failure
// that blocks every run in the repository.
func TestAnUnbornBranchIsNotAnUnknownScope(t *testing.T) {
	root, common := newRepo(t)
	gitRun(t, root, "checkout", "-q", "-b", "feat")
	h := commit(t, root, "feat work")
	writeStoreRun(t, common, "mrv-unborn-00001", "feat", h)
	gitRun(t, root, "checkout", "-q", "--orphan", "gh-pages")
	mine, elsewhere := ScanAbandonedRuns(root)
	if len(mine) != 0 || len(elsewhere) != 1 || elsewhere[0].Scope != "other-branch" {
		t.Fatalf("feat's run is feat's, not the orphan branch's: mine %v, elsewhere %+v", ids(mine), elsewhere)
	}
}

// A branch cut from a fresh origin/main while local main lags must not take in the commits merged upstream since —
// nor the abandoned runs of the branches they came from (AC-4.6 with merge commits).
func TestAStaleLocalMainDoesNotPullInMergedBranchesRuns(t *testing.T) {
	root, common := newRepo(t)
	gitRun(t, root, "checkout", "-q", "-b", "feat")
	f := commit(t, root, "feat work")
	writeStoreRun(t, common, "mrv-merged-up-01", "feat", f)
	gitRun(t, root, "checkout", "-q", "--detach", "main")
	gitRun(t, root, "merge", "-q", "--no-ff", "-m", "merge feat upstream", "feat")
	gitRun(t, root, "update-ref", "refs/remotes/origin/main", "HEAD") // origin/main has feat; local main does not
	gitRun(t, root, "branch", "-q", "-D", "feat")
	gitRun(t, root, "switch", "-q", "-c", "next", "origin/main")
	commit(t, root, "next work")
	mine, elsewhere := ScanAbandonedRuns(root)
	if len(mine) != 0 || len(elsewhere) != 1 || elsewhere[0].Scope != "orphaned" {
		t.Fatalf("feat merged upstream and deleted: its run is orphaned, not next's: mine %v, elsewhere %+v", ids(mine), elsewhere)
	}
}

// AC-4.4: a run created detached with --for-branch feat blocks feat, including after feat is rebased.
func TestADetachedRunForABranchBlocksThatBranch(t *testing.T) {
	root, common := newRepo(t)
	gitRun(t, root, "checkout", "-q", "-b", "feat")
	h := commit(t, root, "feat work")
	gitRun(t, root, "checkout", "-q", "--detach", h) // the review snapshot
	writeStoreRun(t, common, "mrv-detach-00001", "feat", h)
	gitRun(t, root, "checkout", "-q", "main")
	commit(t, root, "main moves on")
	gitRun(t, root, "checkout", "-q", "feat")
	gitRun(t, root, "rebase", "-q", "main")
	if got := strings.Join(ids(DiscoverAbandonedRuns(root)), ","); got != "mrv-detach-00001" {
		t.Fatalf("a --for-branch feat run must block feat after its rebase, got %q", got)
	}
}

// AC-4.5: stacked — an abandoned run on feat-a blocks feat-b, built on feat-a, and the blocker names feat-a.
func TestAStackedBranchInheritsTheBaseBranchsRun(t *testing.T) {
	root, common := newRepo(t)
	gitRun(t, root, "checkout", "-q", "-b", "feat-a")
	a := commit(t, root, "a")
	writeStoreRun(t, common, "mrv-stack-a0001", "feat-a", a)
	gitRun(t, root, "checkout", "-q", "-b", "feat-b")
	commit(t, root, "b")
	if got := strings.Join(ids(DiscoverAbandonedRuns(root)), ","); got != "mrv-stack-a0001" {
		t.Fatalf("feat-a's run must block feat-b, got %q", got)
	}
	r, err := Build(root)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, b := range r.MustClear {
		if b.RunID == "mrv-stack-a0001" && strings.Contains(b.Target, "feat-a") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the blocker must name feat-a: %+v", r.MustClear)
	}
}

// AC-4.6: after feat merges and is deleted, its runs block nothing and --all shows them orphaned.
func TestAMergedAndDeletedBranchsRunsAreOrphaned(t *testing.T) {
	root, common := newRepo(t)
	gitRun(t, root, "checkout", "-q", "-b", "feat")
	h := commit(t, root, "feat work")
	writeStoreRun(t, common, "mrv-merged-00001", "feat", h)
	gitRun(t, root, "checkout", "-q", "main")
	gitRun(t, root, "merge", "-q", "--no-ff", "-m", "merge feat", "feat")
	gitRun(t, root, "branch", "-q", "-D", "feat")
	mine, elsewhere := ScanAbandonedRuns(root)
	if len(mine) != 0 {
		t.Fatalf("a merged, deleted branch's run blocks nothing, got %v", ids(mine))
	}
	if len(elsewhere) != 1 || elsewhere[0].Scope != "orphaned" || elsewhere[0].Dir != filepath.Join(common, "metareview", "runs", "mrv-merged-00001") {
		t.Fatalf("--all shows it orphaned, with the directory to delete: %+v", elsewhere)
	}
}

// AC-4.7: branch-name reuse — delete fix, recreate an unrelated fix — blocks through the name leg only. That is the
// documented trade-off: the name leg is what survives rebase, and clearing a stale run is the closing operation's job
// (its own sub-issue), not something a new branch of the same name should do silently.
func TestBranchNameReuseBlocksThroughTheNameLeg(t *testing.T) {
	root, common := newRepo(t)
	gitRun(t, root, "checkout", "-q", "-b", "fix")
	old := commit(t, root, "old fix")
	writeStoreRun(t, common, "mrv-reuse-00001", "fix", old)
	gitRun(t, root, "checkout", "-q", "main")
	gitRun(t, root, "branch", "-q", "-D", "fix")
	gitRun(t, root, "checkout", "-q", "-b", "fix") // unrelated: old is not in its range
	commit(t, root, "new fix")
	if got := strings.Join(ids(DiscoverAbandonedRuns(root)), ","); got != "mrv-reuse-00001" {
		t.Fatalf("a reused branch name inherits the old run through the name leg, got %q", got)
	}
}

// A run from before branches were recorded is scoped by reachability alone: a head reachable from HEAD blocks — on the
// default branch too, where there is no range, so an upgrade never silently clears one — and a head on unmerged
// sibling work is orphaned (the documented residual gap).
func TestALegacyRunWithoutABranchIsScopedByReachability(t *testing.T) {
	root, common := newRepo(t)
	onMain := gitOut(t, root, "rev-parse", "HEAD")
	gitRun(t, root, "checkout", "-q", "-b", "sibling")
	sibling := commit(t, root, "sibling work")
	gitRun(t, root, "checkout", "-q", "main")
	gitRun(t, root, "branch", "-q", "-D", "sibling")
	writeStoreRun(t, common, "mrv-legacy-main1", "", onMain)
	writeStoreRun(t, common, "mrv-legacy-sib01", "", sibling)
	mine, elsewhere := ScanAbandonedRuns(root)
	if strings.Join(ids(mine), ",") != "mrv-legacy-main1" || len(elsewhere) != 1 || elsewhere[0].Scope != "orphaned" {
		t.Fatalf("on main: the reachable legacy run blocks, the sibling's is orphaned: %v %+v", ids(mine), elsewhere)
	}
	gitRun(t, root, "checkout", "-q", "-b", "feat")
	h := commit(t, root, "feat work")
	writeStoreRun(t, common, "mrv-legacy-feat1", "", h)
	if got := strings.Join(ids(DiscoverAbandonedRuns(root)), ","); got != "mrv-legacy-feat1,mrv-legacy-main1" {
		t.Fatalf("on feat: in range and reachable both block, got %q", got)
	}
}

// On an id collision (the migration keeps both copies) a copy that belongs elsewhere never hides one that blocks,
// and a blocking id is not also listed elsewhere. The 0.13.x legacy location is scanned first.
func TestACopyElsewhereNeverHidesABlockingCopy(t *testing.T) {
	root, common := newRepo(t)
	gitRun(t, root, "checkout", "-q", "-b", "feat")
	h := commit(t, root, "feat work")
	const id = "mrv-collide-00001"
	writeRunAt(t, filepath.Join(root, ".metareview", "runs", id), "gone-branch", "0000000000000000000000000000000000000000")
	writeStoreRun(t, common, id, "feat", h)
	mine, elsewhere := ScanAbandonedRuns(root)
	if strings.Join(ids(mine), ",") != id || len(elsewhere) != 0 {
		t.Fatalf("the blocking copy wins and is not listed elsewhere: %v %v", ids(mine), ids(elsewhere))
	}
}

// An unreadable repository (not a git repository) proves nothing belongs elsewhere: every run is in scope.
func TestOutsideARepositoryEveryRunIsInScope(t *testing.T) {
	root := t.TempDir()
	writeRunAt(t, filepath.Join(root, ".metareview", "runs", "mrv-norepo-00001"), "whatever", "")
	if got := strings.Join(ids(DiscoverAbandonedRuns(root)), ","); got != "mrv-norepo-00001" {
		t.Fatalf("outside a repository every run blocks, got %q", got)
	}
}

// AC-4.9: status with 2,000 runs over 50 branches answers well inside a second. Scope makes a fixed number of git
// calls (pinned in internal/scope); this is the end-to-end bound.
func TestStatusScalesToThousandsOfRuns(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test")
	}
	root, common := newRepo(t)
	for i := 0; i < 2000; i++ {
		writeStoreRun(t, common, fmt.Sprintf("mrv-scale-%06d", i), fmt.Sprintf("b%02d", i%50), "")
	}
	// Best of three: one slow pass on a loaded CI runner is noise, a scan that is slow every time is the regression.
	// The fixed git-call count is pinned separately, in internal/scope.
	var r Report
	best := time.Hour
	for i := 0; i < 3; i++ {
		start := time.Now()
		var err error
		if r, err = Build(root); err != nil {
			t.Fatal(err)
		}
		if d := time.Since(start); d < best {
			best = d
		}
	}
	if best > time.Second {
		t.Fatalf("status over 2,000 runs took %v at best (want < 1s)", best)
	}
	if r.OrphanedRuns != 2000 {
		t.Fatalf("every run is on a missing branch: %d", r.OrphanedRuns)
	}
}

func TestLegacyRunsPendingBookkeeping(t *testing.T) {
	root, _ := newRepo(t)
	if err := os.MkdirAll(filepath.Join(root, ".metareview", "runs", ".torn"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".metareview", "runs", "mrv-pending-000001"), 0o700); err != nil {
		t.Fatal(err)
	}
	if !LegacyRunsPending(root) {
		t.Fatal("a legacy run must be pending")
	}
	r, err := Build(root)
	if err != nil {
		t.Fatal(err)
	}
	want := "0.13.x FSM runs are still in " + filepath.Join(root, ".metareview", "runs") + "; any `metareview fsm` command migrates them"
	if !strings.Contains(strings.Join(r.Warnings, "\n"), want) {
		t.Fatalf("status warnings = %q", r.Warnings)
	}
	_ = os.Remove(filepath.Join(root, ".metareview", "runs", "mrv-pending-000001"))
	if LegacyRunsPending(root) {
		t.Error("a legacy store holding only its own bookkeeping has nothing to migrate")
	}
}

// AC-4.8 at the package seam: the --all emitters list the runs elsewhere and exit exactly as the plain ones do.
func TestTheAllEmittersChangeTheListNeverTheExit(t *testing.T) {
	root, common := newRepo(t)
	gitRun(t, root, "checkout", "-q", "-b", "feat")
	writeStoreRun(t, common, "mrv-emit-mine001", "feat", "")
	writeStoreRun(t, common, "mrv-emit-main001", "main", "")
	type emitter func(w *strings.Builder) (int, error)
	for name, pair := range map[string][2]emitter{
		"target": {
			func(w *strings.Builder) (int, error) { return EmitFor(root, "", w) },
			func(w *strings.Builder) (int, error) { return EmitForAll(root, "", w) },
		},
		"branch": {
			func(w *strings.Builder) (int, error) { return EmitForBranch(root, "", nil, w) },
			func(w *strings.Builder) (int, error) { return EmitForBranchAll(root, "", nil, w) },
		},
	} {
		var plain, all strings.Builder
		code, err := pair[0](&plain)
		codeAll, errAll := pair[1](&all)
		if err != nil || errAll != nil || code != 1 || codeAll != 1 {
			t.Fatalf("%s: exits %d/%d errs %v/%v (want 1/1: feat's run blocks)", name, code, codeAll, err, errAll)
		}
		if strings.Contains(plain.String(), "mrv-emit-main001") || !strings.Contains(all.String(), "mrv-emit-main001") {
			t.Fatalf("%s: only --all lists main's run:\n%s\n---\n%s", name, plain.String(), all.String())
		}
	}
}
