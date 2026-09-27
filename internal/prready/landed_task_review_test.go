package prready

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dsifry/metareview/internal/reviewlog"
)

// writeTaskDoneLog writes a committed-style task-done NEEDS_REVISION review log and its context pack, shaped like
// the real ones (header, Covered paths, a blocking finding) and recording headSHA only in the pack's Git section —
// as a log from another clone, with no local run record, does.
func writeTaskDoneLog(t *testing.T, root, runID, target, headSHA, branch string, covered string) {
	t.Helper()
	contextRel := "docs/metareview/context/" + runID + "-context.md"
	reviewRel := "docs/metareview/reviews/" + runID + ".md"
	log := "# metareview: task-done review\n\nRun ID: `" + runID + "`\n\nTarget: `" + target + "`\n\n" +
		"Context pack: `" + contextRel + "`\n\nExecution mode: `deterministic-local`\n\nGate effect: `gate`\n\n" +
		"Previous run: `none`\n\nCovered paths: `[\"" + covered + "\"]`\n\n## Verdict\n\nNEEDS_REVISION\n\n" +
		"## Reviewer Results\n\n| Reviewer | Verdict | Blocking | Notes |\n| --- | --- | ---: | --- |\n" +
		"| adversarial-review-reviewer | NEEDS_REVISION | 1 | No adjudicated lens review recorded |\n\n" +
		"## Blocking Findings\n\n### " + runID + "-001: No adjudicated lens review recorded\n\n" +
		"- Reviewer: adversarial-review-reviewer\n- Severity: high\n- Classification: blocking\n" +
		"- Finding: none is recorded for HEAD " + headSHA + ".\n\n\n## Advisory Findings\n\nNo findings in this class.\n"
	pack := "# metareview Context Pack\n\nRun ID: `" + runID + "`\n\n## Git\n\n- Base: `" + headSHA + "`\n- Head: `" + headSHA + "`\n- Branch: `" + branch + "`\n"
	for rel, body := range map[string]string{reviewRel: log, contextRel: pack} {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func revParse(t *testing.T, root, ref string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", ref)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

// historyRepo builds main: A → B (the fork point), feature: B → C (checked out), and an unmerged branch old: A → D
// standing in for a squash-merged feature branch whose commits never reach main.
func historyRepo(t *testing.T) (root string, a, b, c, d string) {
	t.Helper()
	root = t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	commit := func(file, body, msg string) string {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, file), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		run("add", ".")
		run("commit", "-q", "-m", msg)
		return revParse(t, root, "HEAD")
	}
	run("init", "-q", "-b", "main")
	run("config", "--local", "commit.gpgsign", "false")
	run("config", "--local", "core.hooksPath", filepath.Join(root, ".git", "hooks"))
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test User")
	a = commit("seed.txt", "a\n", "A")
	run("checkout", "-q", "-b", "old")
	d = commit("other.txt", "d\n", "D")
	run("checkout", "-q", "main")
	b = commit("seed.txt", "b\n", "B")
	run("checkout", "-q", "-b", "feature")
	c = commit("seed.txt", "c\n", "C")
	return root, a, b, c, d
}

// TestPRReadyRetiresOnlyFlagTargetTaskReviews is #187: a task-done review whose target is a command-line flag (the
// "--help" artifact of the bug #164 fixed) was never a review of work, so it does not block pr-ready even though its
// covered paths overlap the diff. Nothing else is inferred: the same stale review under an ordinary target, and a
// review of this branch's own work, still block — other stale blockers are cleared by a human-granted override.
func TestPRReadyRetiresOnlyFlagTargetTaskReviews(t *testing.T) {
	t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "1")
	evidence := filepath.Join(t.TempDir(), "evidence.md")
	if err := os.WriteFile(evidence, []byte("go test ./... exited 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 27, 7, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, target, headOf, branch string
		blocks                       bool
	}{
		{"flag target (the --help artifact)", "--help", "A", "main", false},
		{"short flag target", "-h", "A", "main", false},
		{"the same stale review under an ordinary target", "help", "A", "main", true},
		{"this branch's own work", "feat", "C", "feature", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, a, _, c, _ := historyRepo(t)
			head := map[string]string{"A": a, "C": c}[tc.headOf]
			writeTaskDoneLog(t, root, "mrv-20260905-223221349809000-task-done-x-9a8265a5", tc.target, head, tc.branch, "seed.txt")
			result, err := Create(root, Options{Base: "main", EvidencePath: evidence, Now: now})
			if err != nil {
				t.Fatal(err)
			}
			body, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(result.ReviewRel)))
			blocked := result.Blocking && strings.Contains(string(body), "Blocked targets: "+tc.target)
			if blocked != tc.blocks {
				t.Fatalf("blocks=%v, want %v\n%s", blocked, tc.blocks, body)
			}
		})
	}
}

// TestFlagTargetTaskReviewRunIDs pins the selection: task-done only, a run id, and a target that is a flag.
func TestFlagTargetTaskReviewRunIDs(t *testing.T) {
	logs := []reviewlog.Summary{
		{RunID: "mrv-help", Kind: "task-done", Target: "--help"},
		{RunID: "mrv-h", Kind: "task-done", Target: " -h "},
		{RunID: "mrv-task", Kind: "task-done", Target: "task-1"},
		{RunID: "mrv-dash-inside", Kind: "task-done", Target: "fix--help"},
		{RunID: "", Kind: "task-done", Target: "--help"},
		{RunID: "mrv-pr", Kind: "pr-ready", Target: "--help"},
		{RunID: "mrv-empty", Kind: "task-done", Target: ""},
	}
	if got := strings.Join(flagTargetTaskReviewRunIDs(logs), ","); got != "mrv-help,mrv-h" {
		t.Fatalf("flag-target ids = %q", got)
	}
}
