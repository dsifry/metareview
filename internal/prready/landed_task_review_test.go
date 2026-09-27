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

// TestPRReadyTaskReviewHistoryIsAboutWhoseWorkItWas is #187 end to end: a task-done review that covered other,
// landed work — on main before the fork point, or on another (squash-merged) branch — does not block pr-ready
// even though its covered paths overlap the diff; a review of this branch's own work still blocks, including the
// first chunk reviewed before it was committed (recorded at the fork point, on this branch).
func TestPRReadyTaskReviewHistoryIsAboutWhoseWorkItWas(t *testing.T) {
	t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "1")
	evidence := filepath.Join(t.TempDir(), "evidence.md")
	if err := os.WriteFile(evidence, []byte("go test ./... exited 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 27, 7, 0, 0, 0, time.UTC)
	gate := func(t *testing.T, target, headOf, branch string) (bool, string) {
		t.Helper()
		root, a, b, c, d := historyRepo(t)
		head := map[string]string{"A": a, "B": b, "C": c, "D": d}[headOf]
		writeTaskDoneLog(t, root, "mrv-20260905-223221349809000-task-done-"+target+"-9a8265a5", target, head, branch, "seed.txt")
		result, err := Create(root, Options{Base: "main", EvidencePath: evidence, Now: now})
		if err != nil {
			t.Fatal(err)
		}
		body, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(result.ReviewRel)))
		return result.Blocking && strings.Contains(string(body), "Blocked targets: "+target), string(body)
	}
	for _, tc := range []struct {
		name, target, head, branch string
		blocks                     bool
	}{
		{"main before the fork point (the --help case)", "help", "A", "main", false},
		{"another, squash-merged branch", "oldtask", "D", "old", false},
		{"this branch's first chunk, uncommitted, at the fork point", "firstchunk", "B", "feature", true},
		{"this branch's committed work", "feat", "C", "feature", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if blocked, body := gate(t, tc.target, tc.head, tc.branch); blocked != tc.blocks {
				t.Fatalf("blocks=%v, want %v\n%s", blocked, tc.blocks, body)
			}
		})
	}
}

// TestLandedTaskReviewRunIDsFailsClosed covers each rule of taskReviewIsHistory, including every fail-closed one.
func TestLandedTaskReviewRunIDsFailsClosed(t *testing.T) {
	root, a, b, c, d := historyRepo(t)
	git := gitcontext.Context{BaseSHA: b, HeadSHA: c, Branch: "feature"}
	pack := func(id, head, branch string) reviewlog.Summary {
		writeTaskDoneLog(t, root, id, "t", head, branch, "seed.txt")
		return reviewlog.Summary{RunID: id, Kind: "task-done", ContextRel: "docs/metareview/context/" + id + "-context.md"}
	}
	unknown := strings.Repeat("a", 40)
	logs := []reviewlog.Summary{
		pack("mrv-main-old", a, "main"),                           // history: on main, before the fork point
		pack("mrv-other-branch", d, "old"),                        // history: another branch's commit
		pack("mrv-gone", unknown, "old"),                          // history: another branch's head git no longer has
		pack("mrv-fork-point", b, "main"),                         // current: at the fork point (maybe this branch's first chunk)
		pack("mrv-own-branch", a, "feature"),                      // current: recorded on this branch (e.g. before a rebase)
		pack("mrv-in-range", c, "renamed"),                        // current: head inside base..HEAD
		pack("mrv-detached", a, ""),                               // current: detached review of a commit in this history
		pack("mrv-detached-gone", unknown, ""),                    // current: detached review of a head git cannot place
		pack("mrv-ref-name", "main", "old"),                       // current: a ref name is not a commit id
		{RunID: "mrv-record-head", Kind: "task-done", HeadSHA: a}, // current: run-record head with no pack has no known branch
		{RunID: "", Kind: "task-done", HeadSHA: a},                // ignored: no run id
		{RunID: "mrv-pr", Kind: "pr-ready", HeadSHA: a},           // ignored: not a task review
	}
	got := landedTaskReviewRunIDs(root, logs, git)
	if strings.Join(got, ",") != "mrv-main-old,mrv-other-branch,mrv-gone" {
		t.Fatalf("history ids = %v", got)
	}
	for _, bad := range []gitcontext.Context{{BaseSHA: "main", HeadSHA: c, Branch: "feature"}, {BaseSHA: b, HeadSHA: "HEAD", Branch: "feature"}} {
		if got := landedTaskReviewRunIDs(root, logs, bad); got != nil {
			t.Fatalf("an unresolved base or head must yield nothing, got %v", got)
		}
	}
}
