package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// branchFindingsRepo is a single checkout for #178: main with a seed commit, and a git helper that runs with the
// config isolated. Each branch the tests create carries its own task file.
func branchFindingsRepo(t *testing.T) (root string, git func(args ...string) string, write func(rel, body string)) {
	t.Helper()
	root = t.TempDir()
	env := append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_OPTIONAL_LOCKS=0",
		"GIT_EDITOR=true", "GIT_AUTHOR_DATE=2026-09-28T00:00:00Z", "GIT_COMMITTER_DATE=2026-09-28T00:00:00Z")
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
	write = func(rel, body string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q", "-b", "main")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "Test User")
	git("config", "commit.gpgsign", "false")
	write("seed.txt", "seed\n")
	git("add", ".")
	git("commit", "-q", "-m", "initial")
	return root, git, write
}

// prReadyLog runs pr-ready on the checked-out branch and returns its review log. The output it writes is removed
// again, so it never becomes part of a later diff or blocks a later `git switch`.
func prReadyLog(t *testing.T, root, evidence string) string {
	t.Helper()
	before := untracked(t, root)
	_, out, errOut := runCLI(t, root, nil, "review", "pr-ready", "--base", "main", "--evidence", evidence)
	log := reviewLogFrom(t, root, out+errOut)
	for p := range untracked(t, root) {
		if !before[p] {
			_ = os.Remove(filepath.Join(root, p))
		}
	}
	return log
}

// prReadyBlocksTA reports whether pr-ready on the checked-out branch lists the target t-a (as a whole entry, not a
// substring) among its blocked targets.
func prReadyBlocksTA(t *testing.T, root, evidence string) bool {
	t.Helper()
	return slices.Contains(blockedTargets(prReadyLog(t, root, evidence)), "t-a")
}

// blockedTargets is the target list of a log's "Blocked targets: " line — that line only, split as the reviewer joins it.
func blockedTargets(log string) []string {
	_, rest, ok := strings.Cut(log, "Blocked targets: ")
	if !ok {
		return nil
	}
	line, _, _ := strings.Cut(rest, "\n")
	return strings.Split(strings.TrimSpace(line), ", ")
}

func untracked(t *testing.T, root string) map[string]bool {
	t.Helper()
	cmd := exec.Command("git", "ls-files", "--others", "--exclude-standard")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	set := map[string]bool{}
	for _, p := range strings.Fields(string(out)) {
		set[p] = true
	}
	return set
}

// reviewLogFrom reads the review log a gate run names on its output.
func reviewLogFrom(t *testing.T, root, out string) string {
	t.Helper()
	for _, f := range strings.Fields(out) {
		f = strings.Trim(f, "`\"',")
		if strings.HasPrefix(f, "docs/metareview/reviews/") && strings.HasSuffix(f, ".md") {
			b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
			if err != nil {
				t.Fatal(err)
			}
			return string(b)
		}
	}
	t.Fatalf("no review log named in the gate's output:\n%s", out)
	return ""
}

// taskDoneOnBranchA builds #178's reproduction up to its step 2: branch-a carries task t-a, whose task-done review
// is NEEDS_REVISION, and that review's log is committed on branch-a.
func taskDoneOnBranchA(t *testing.T, root string, git func(...string) string, write func(string, string)) {
	t.Helper()
	git("checkout", "-q", "-b", "branch-a")
	write("docs/tasks/t-a.md", "# Task A\n\nDo A.\n")
	write("src/a.go", "package src\n\nvar A = 1\n")
	git("add", "-A")
	git("commit", "-q", "-m", "work on A")
	code, out, errOut := runCLI(t, root, nil, "review", "task-done", "t-a", "--base", "main")
	if code == 0 || !strings.Contains(reviewLogFrom(t, root, out+errOut), "NEEDS_REVISION") {
		t.Fatalf("task-done on branch-a must be NEEDS_REVISION: code=%d\n%s%s", code, out, errOut)
	}
	git("add", "docs/metareview")
	git("commit", "-q", "-m", "task-done review of t-a")
}

func branchFindingsEvidence(t *testing.T) string {
	t.Helper()
	t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "1")
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	evidence := filepath.Join(t.TempDir(), "evidence.md")
	if err := os.WriteFile(evidence, []byte("go test ./... exited 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return evidence
}

// TestOpenFindingOnOneBranchDoesNotBlockAnother is #178 AC-4.1, the issue's exact sequence in one checkout: branch-b's
// pr-ready no longer lists branch-a's t-a as a blocker, and switching back to branch-a blocks on it again.
func TestOpenFindingOnOneBranchDoesNotBlockAnother(t *testing.T) {
	evidence := branchFindingsEvidence(t)
	root, git, write := branchFindingsRepo(t)
	taskDoneOnBranchA(t, root, git, write)

	git("switch", "-q", "-c", "branch-b", "main")
	write("src/b.go", "package src\n\nvar B = 1\n")
	git("add", "src/b.go")
	git("commit", "-q", "-m", "work on B")
	if log := prReadyLog(t, root, evidence); strings.Contains(log, "Blocked targets: ") || !strings.Contains(log, "Open on other branches: 1 ") {
		t.Fatalf("branch-b's pr-ready must not block on branch-a's open finding (t-a), and must list it as an advisory:\n%s", log)
	}

	git("switch", "-q", "branch-a")
	if !prReadyBlocksTA(t, root, evidence) {
		t.Fatal("back on branch-a, its own open finding (t-a) must block pr-ready again")
	}
}

// TestOpenFindingSurvivesRebaseAndAmend is #178 AC-4.3: an open finding on a branch still blocks that branch after
// `git rebase main` and after `git commit --amend`, which both leave the recorded head behind.
func TestOpenFindingSurvivesRebaseAndAmend(t *testing.T) {
	evidence := branchFindingsEvidence(t)
	root, git, write := branchFindingsRepo(t)
	taskDoneOnBranchA(t, root, git, write)

	git("switch", "-q", "main")
	write("seed.txt", "seed, advanced\n")
	git("commit", "-q", "-am", "main advances")
	git("switch", "-q", "branch-a")
	git("rebase", "-q", "main")
	if !prReadyBlocksTA(t, root, evidence) {
		t.Fatal("after `git rebase main` branch-a's open finding must still block it")
	}

	write("src/a.go", "package src\n\nvar A = 2\n")
	git("add", "src/a.go")
	git("commit", "-q", "--amend", "--no-edit")
	if !prReadyBlocksTA(t, root, evidence) {
		t.Fatal("after `git commit --amend` branch-a's open finding must still block it")
	}
}

// TestMergedAndDeletedBranchFindingsBlockNothing is #178 AC-4.6: once branch-a is merged and deleted, its findings
// block nothing on a branch cut from main afterwards. (A NEEDS_REVISION review *log* merged into main is
// committed evidence, not the ledger, and still blocks a later branch whose diff overlaps it: that is #188's stale
// committed-log blocker. The ledger half of AC-4.6 is pinned in internal/findings.)
func TestMergedAndDeletedBranchFindingsBlockNothing(t *testing.T) {
	evidence := branchFindingsEvidence(t)
	root, git, write := branchFindingsRepo(t)
	taskDoneOnBranchA(t, root, git, write)

	git("switch", "-q", "main")
	git("merge", "-q", "--no-ff", "-m", "merge branch-a", "branch-a")
	git("branch", "-q", "-D", "branch-a")
	git("switch", "-q", "-c", "branch-c")
	write("src/c.go", "package src\n\nvar C = 1\n")
	git("add", "src/c.go")
	git("commit", "-q", "-m", "work on C")
	if prReadyBlocksTA(t, root, evidence) {
		t.Fatal("a merged-and-deleted branch's open finding must not block a branch cut after the merge")
	}
}
