package prready

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dsifry/metareview/internal/gitcontext"
	"github.com/dsifry/metareview/internal/reviewlog"
)

// writeTaskDoneLog writes a committed-style task-done NEEDS_REVISION review log and its context pack, shaped like
// the real ones (header, Covered paths, a blocking finding) and recording headSHA only in the pack's Git section —
// as a log from another clone, with no local run record, does.
func writeTaskDoneLog(t *testing.T, root, runID, target, headSHA string, covered string) {
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
	pack := "# metareview Context Pack\n\nRun ID: `" + runID + "`\n\n## Git\n\n- Base: `" + headSHA + "`\n- Head: `" + headSHA + "`\n- Branch: `main`\n"
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

// TestPRReadyIgnoresTaskReviewsOfLandedCommits is #187: a task-done review of a commit already on the PR's base
// reviewed work that has landed, so it is history — even when its covered paths overlap this branch's diff. It
// must not block pr-ready. A task review of the branch's own commit (inside base..HEAD) still blocks.
func TestPRReadyIgnoresTaskReviewsOfLandedCommits(t *testing.T) {
	t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "1")
	evidence := filepath.Join(t.TempDir(), "evidence.md")
	if err := os.WriteFile(evidence, []byte("go test ./... exited 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 27, 7, 0, 0, 0, time.UTC)

	// Landed: the review's head is main's commit, an ancestor of the PR base.
	landed := smallPRReadyRepo(t)
	writeTaskDoneLog(t, landed, "mrv-20260905-223221349809000-task-done-help-9a8265a5", "--help", revParse(t, landed, "main"), "seed.txt")
	result, err := Create(landed, Options{Base: "main", EvidencePath: evidence, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(filepath.Join(landed, filepath.FromSlash(result.ReviewRel)))
	if result.Blocking || strings.Contains(string(body), "Blocked targets: --help") {
		t.Fatalf("a task review of a landed commit must not block pr-ready: verdict=%s\n%s", result.Verdict, body)
	}

	// Control: the same review of the branch's own commit is part of this PR and still blocks.
	current := smallPRReadyRepo(t)
	writeTaskDoneLog(t, current, "mrv-20260926-010101000000000-task-done-feat-1a2b3c4d", "feat", revParse(t, current, "HEAD"), "seed.txt")
	result, err = Create(current, Options{Base: "main", EvidencePath: evidence, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	body, _ = os.ReadFile(filepath.Join(current, filepath.FromSlash(result.ReviewRel)))
	if !result.Blocking || !strings.Contains(string(body), "Blocked targets: feat") {
		t.Fatalf("a task review of the branch's own commit must still block: verdict=%s\n%s", result.Verdict, body)
	}
}

// TestLandedTaskReviewRunIDsFailsClosed covers each way landedTaskReviewRunIDs declines to call a review landed.
func TestLandedTaskReviewRunIDsFailsClosed(t *testing.T) {
	root := smallPRReadyRepo(t)
	base, head := revParse(t, root, "main"), revParse(t, root, "HEAD")
	git := gitcontext.Context{BaseSHA: base}
	writeTaskDoneLog(t, root, "mrv-landed-pack", "t", base, "seed.txt")
	logs := []reviewlog.Summary{
		{RunID: "mrv-landed-record", Kind: "task-done", HeadSHA: base},                                                  // run-record head on base: landed
		{RunID: "mrv-landed-pack", Kind: "task-done", ContextRel: "docs/metareview/context/mrv-landed-pack-context.md"}, // pack head on base: landed
		{RunID: "mrv-branch", Kind: "task-done", HeadSHA: head},                                                         // branch commit: current
		{RunID: "", Kind: "task-done", HeadSHA: base},                                                                   // no run id
		{RunID: "mrv-pr", Kind: "pr-ready", HeadSHA: base},                                                              // not a task review
		{RunID: "mrv-no-pack", Kind: "task-done", ContextRel: "docs/metareview/context/missing.md"},                     // unreadable pack
		{RunID: "mrv-bad-head", Kind: "task-done", HeadSHA: "not-a-sha"},                                                // invalid head
		{RunID: "mrv-unknown", Kind: "task-done", HeadSHA: strings.Repeat("a", 40)},                                     // head git cannot place
	}
	got := landedTaskReviewRunIDs(root, logs, git)
	if strings.Join(got, ",") != "mrv-landed-record,mrv-landed-pack" {
		t.Fatalf("landed ids = %v, want only the two reviews of the base commit", got)
	}
	if got := landedTaskReviewRunIDs(root, logs, gitcontext.Context{BaseSHA: "unknown"}); got != nil {
		t.Fatalf("an invalid base must yield nothing, got %v", got)
	}
}
