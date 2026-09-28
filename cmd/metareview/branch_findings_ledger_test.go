package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// taskDoneOnBranchALedgerOnly is taskDoneOnBranchA without the committed review log: the task-done review log and context
// pack are removed, so the only thing left to block on t-a is the checkout's findings ledger. branch_findings_test.go commits
// the log, which blocks branch-a by itself and would hide a ledger that never blocks.
func taskDoneOnBranchALedgerOnly(t *testing.T, root string, git func(...string) string, write func(string, string)) {
	t.Helper()
	git("checkout", "-q", "-b", "branch-a")
	write("docs/tasks/t-a.md", "# Task A\n\nDo A.\n")
	write("src/a.go", "package src\n\nvar A = 1\n")
	git("add", "-A")
	git("commit", "-q", "-m", "work on A")
	before := untracked(t, root)
	code, out, errOut := runCLI(t, root, nil, "review", "task-done", "t-a", "--base", "main")
	if code == 0 || !strings.Contains(reviewLogFrom(t, root, out+errOut), "NEEDS_REVISION") {
		t.Fatalf("task-done on branch-a must be NEEDS_REVISION: code=%d\n%s%s", code, out, errOut)
	}
	for p := range untracked(t, root) {
		// Only the review output: the ledger (.metareview/) is what this test is about.
		if !before[p] && strings.HasPrefix(p, "docs/metareview/") {
			if err := os.Remove(filepath.Join(root, p)); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// TestLedgerFindingFollowsItsBranch is #178 AC-4.1 and AC-4.3 through the ledger alone: branch-a's open finding does
// not block branch-b, blocks branch-a again after switching back, and keeps blocking it after `git rebase main` and
// `git commit --amend`.
func TestLedgerFindingFollowsItsBranch(t *testing.T) {
	evidence := branchFindingsEvidence(t)
	root, git, write := branchFindingsRepo(t)
	taskDoneOnBranchALedgerOnly(t, root, git, write)

	git("switch", "-q", "-c", "branch-b", "main")
	write("src/b.go", "package src\n\nvar B = 1\n")
	git("add", "src/b.go")
	git("commit", "-q", "-m", "work on B")
	if prReadyBlocksTA(t, root, evidence) {
		t.Fatal("branch-b's pr-ready must not block on branch-a's ledger finding (t-a)")
	}

	git("switch", "-q", "branch-a")
	if !prReadyBlocksTA(t, root, evidence) {
		t.Fatal("back on branch-a, its ledger finding (t-a) must block pr-ready again")
	}

	git("switch", "-q", "main")
	write("seed.txt", "seed, advanced\n")
	git("commit", "-q", "-am", "main advances")
	git("switch", "-q", "branch-a")
	git("rebase", "-q", "main")
	if !prReadyBlocksTA(t, root, evidence) {
		t.Fatal("after `git rebase main` branch-a's ledger finding must still block it")
	}
	write("src/a.go", "package src\n\nvar A = 2\n")
	git("add", "src/a.go")
	git("commit", "-q", "--amend", "--no-edit")
	if !prReadyBlocksTA(t, root, evidence) {
		t.Fatal("after `git commit --amend` branch-a's ledger finding must still block it")
	}
}
