package prready

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dsifry/metareview/internal/contextprofile"
	"github.com/dsifry/metareview/internal/findings"
	"github.com/dsifry/metareview/internal/gitcontext"
	"github.com/dsifry/metareview/internal/githubcontext"
	"github.com/dsifry/metareview/internal/knowledge"
	"github.com/dsifry/metareview/internal/reviewers"
	"github.com/dsifry/metareview/internal/reviewlog"
	"github.com/dsifry/metareview/internal/reviewmanifest"
	"github.com/dsifry/metareview/internal/runchain"
)

// The pr-ready log must carry a Covered paths: header that the reviewlog parser reads back, so `status`
// can credit a clean pr-ready review for the branch files it examined.
func TestPRReadyReviewMarkdownEmitsRoundTrippableCoveredPaths(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "docs", "metareview", "reviews")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	md := reviewMarkdown("mrv-pr-cov", "ctx.md", "", "gate", "PASS", []string{"internal/foo.go", "a,b.go"}, nil, "ev", "", reviewMetadata{})
	if err := os.WriteFile(filepath.Join(dir, "mrv-pr-cov.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	logs, err := reviewlog.Discover(root)
	if err != nil || len(logs) != 1 {
		t.Fatalf("Discover: %v (%d logs)", err, len(logs))
	}
	s := logs[0]
	if !s.CoveredPathsKnown || len(s.CoveredPaths) != 2 || s.CoveredPaths[1] != "a,b.go" {
		t.Fatalf("pr-ready covered paths did not round-trip: known=%v %+v", s.CoveredPathsKnown, s.CoveredPaths)
	}
}

func TestReviewMarkdownSeparatesNonBlockingFindings(t *testing.T) {
	records := []findings.Record{
		{Reviewer: "pr-readiness-reviewer", Classification: "advisory", Severity: "medium", Title: "Prefer helper", Finding: "Helper would reduce duplication."},
		{Reviewer: "validation-reviewer", Classification: "follow-up", Severity: "low", Title: "Track cleanup", Finding: "Cleanup belongs in a later target."},
		{Reviewer: "security-reviewer", Classification: "warning", Severity: "high", Title: "Unknown class", Finding: "Unknown classification was downgraded to warning."},
	}
	md := reviewMarkdown("mrv-pr", "ctx.md", "", "gate", "PASS_ADVISORY", []string{"internal/foo.go"}, records, "evidence", "", reviewMetadata{AdvisoryFindingCount: 1, FollowUpFindingCount: 1, WarningFindingCount: 1})
	if strings.Contains(md, "| pr-readiness-reviewer | NEEDS_REVISION | 1 |") || strings.Contains(md, "| validation-reviewer | NEEDS_REVISION | 1 |") {
		t.Fatalf("non-blocking findings must not render as blocking reviewer failures:\n%s", md)
	}
	for _, required := range []string{"| pr-readiness-reviewer | PASS_ADVISORY | 0 | Prefer helper |", "## Advisory Findings", "## Follow-up Findings", "## Warnings", "Unknown classification was downgraded to warning."} {
		if !strings.Contains(md, required) {
			t.Fatalf("review markdown missing %q:\n%s", required, md)
		}
	}
}

func TestVerdictForNonBlockingFindingsIsPassAdvisory(t *testing.T) {
	counts := findings.ClassCounts{Advisory: 1, FollowUp: 1}
	verdict, status, blocking, reason := verdictForCounts(counts, "gate", 1, 3)
	if verdict != "PASS_ADVISORY" || status != "passed" || blocking || reason != "" {
		t.Fatalf("non-blocking findings must produce PASS_ADVISORY, got verdict=%s status=%s blocking=%v reason=%q", verdict, status, blocking, reason)
	}
}

func TestRunChainMarkdownIncludesEscalationDetails(t *testing.T) {
	md := runChainMarkdown("mrv-pr", "ESCALATED", reviewMetadata{
		AttemptNumber:        2,
		MaxAttempts:          2,
		RunChain:             []runchain.Record{{ID: "mrv-root", Verdict: "NEEDS_REVISION", AttemptNumber: 1, MaxAttempts: 2}},
		BlockingFindingCount: 1,
		AdvisoryFindingCount: 1,
		FollowUpFindingCount: 0,
		WarningFindingCount:  1,
	})
	for _, required := range []string{"## Run Chain", "mrv-root", "2/2", "Blocking: 1", "Warnings: 1"} {
		if !strings.Contains(md, required) {
			t.Fatalf("run chain markdown missing %q:\n%s", required, md)
		}
	}
}

func TestReviewMarkdownIdentifiesReuseAndHistoricalRepositoryHealth(t *testing.T) {
	md := reviewMarkdown("mrv-pr", "ctx.md", "", "gate", "PASS", []string{"internal/foo.go"}, nil, "evidence", "", reviewMetadata{
		ReviewInputDigest:  "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ReusedFromRunID:    "mrv-prior",
		HistoricalBlockers: []findings.Record{{ID: "mrvf-old", Title: "Old unrelated blocker", Target: map[string]any{"type": "beads-task", "id": "GUIDE-old"}}},
	})
	for _, want := range []string{"Execution mode: `deterministic-local-reused`", "Reused verdict from: `mrv-prior`", "Reviewer input digest: `sha256:aaaa", "## Repository Health Advisory", "Old unrelated blocker", "does not block this target"} {
		if !strings.Contains(md, want) {
			t.Fatalf("review markdown missing %q:\n%s", want, md)
		}
	}
}

func TestCreateReusesAuthenticatedUnchangedVerdictWithoutReviewerInvocation(t *testing.T) {
	root := smallPRReadyRepo(t)
	t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "1")
	evidence := filepath.Join(t.TempDir(), "evidence.md")
	if err := os.WriteFile(evidence, []byte("go test ./... exited 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	original := runPRReadyReviewers
	calls := 0
	runPRReadyReviewers = func(ctx reviewers.PRReadyContext) []reviewers.Finding {
		calls++
		return original(ctx)
	}
	t.Cleanup(func() { runPRReadyReviewers = original })

	first, err := Create(root, Options{Base: "main", EvidencePath: evidence, Now: time.Date(2026, 9, 3, 1, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Create(root, Options{Base: "main", EvidencePath: evidence, Now: time.Date(2026, 9, 3, 1, 0, 1, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("reviewers invoked %d times, want once for byte-identical inputs", calls)
	}
	if !second.Reused || second.ReusedFromRunID != first.RunID || second.Verdict != first.Verdict {
		t.Fatalf("second result did not identify authenticated reuse: first=%+v second=%+v", first, second)
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(second.ReviewRel)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "Reused verdict from: `"+first.RunID+"`") {
		t.Fatalf("reused review does not name its source:\n%s", body)
	}
}

func TestChangedReviewerInputsStartFreshReview(t *testing.T) {
	root := smallPRReadyRepo(t)
	t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "1")
	evidence := filepath.Join(t.TempDir(), "evidence.md")
	if err := os.WriteFile(evidence, []byte("go test ./... exited 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	original := runPRReadyReviewers
	calls := 0
	runPRReadyReviewers = func(ctx reviewers.PRReadyContext) []reviewers.Finding {
		calls++
		return original(ctx)
	}
	t.Cleanup(func() { runPRReadyReviewers = original })
	first, err := Create(root, Options{Base: "main", EvidencePath: evidence, Now: time.Date(2026, 9, 3, 2, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(evidence, []byte("go test ./... and go vet ./... exited 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := Create(root, Options{Base: "main", EvidencePath: evidence, PreviousRunID: first.RunID, Now: time.Date(2026, 9, 3, 2, 0, 1, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || second.Reused {
		t.Fatalf("changed evidence must execute a fresh review: calls=%d result=%+v", calls, second)
	}
}

func TestExplicitPreviousRunWithOpenFindingCannotReuseOlderPass(t *testing.T) {
	root := smallPRReadyRepo(t)
	t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "1")
	evidence := filepath.Join(t.TempDir(), "evidence.md")
	if err := os.WriteFile(evidence, []byte("go test ./... exited 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	original := runPRReadyReviewers
	calls := 0
	runPRReadyReviewers = func(reviewers.PRReadyContext) []reviewers.Finding {
		calls++
		if calls != 2 {
			return nil
		}
		return []reviewers.Finding{{
			Reviewer:       "security-reviewer",
			Severity:       "high",
			Classification: "blocking",
			Title:          "Predecessor blocker",
			Finding:        "The predecessor still records a release risk.",
			Expected:       "An explicit fix run must execute reviewers before reconciling it.",
			Found:          "An older successful digest is otherwise reusable.",
			Recommendation: "Run the review and reconcile the predecessor finding.",
			Fingerprint:    "security:predecessor-blocker",
		}}
	}
	t.Cleanup(func() { runPRReadyReviewers = original })

	first, err := Create(root, Options{Base: "main", EvidencePath: evidence, Now: time.Date(2026, 9, 3, 3, 30, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if first.Blocking {
		t.Fatalf("first run must establish the older reusable success: %+v", first)
	}
	if err := os.WriteFile(evidence, []byte("go test ./... and go vet ./... exited 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := Create(root, Options{Base: "main", EvidencePath: evidence, Now: time.Date(2026, 9, 3, 3, 30, 1, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if !second.Blocking {
		t.Fatalf("second run must record the predecessor blocker: %+v", second)
	}
	if err := os.WriteFile(evidence, []byte("go test ./... exited 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	third, err := Create(root, Options{Base: "main", EvidencePath: evidence, PreviousRunID: second.RunID, Now: time.Date(2026, 9, 3, 3, 30, 2, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 || third.Reused || third.Blocking {
		t.Fatalf("explicit fix run must execute and reconcile instead of reusing an older PASS: calls=%d result=%+v", calls, third)
	}
	open, err := findings.UnresolvedBlocking(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 0 {
		t.Fatalf("successful explicit fix run must reconcile the predecessor finding: %+v", open)
	}
}

func TestCommittedPreviousRunContinuesInFreshCloneWithAuthenticatedContext(t *testing.T) {
	root := smallPRReadyRepo(t)
	t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "1")
	evidence := filepath.Join(t.TempDir(), "evidence.md")
	if err := os.WriteFile(evidence, []byte("go test ./... exited 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	original := runPRReadyReviewers
	calls := 0
	runPRReadyReviewers = func(reviewers.PRReadyContext) []reviewers.Finding {
		calls++
		if calls != 1 {
			return nil
		}
		return []reviewers.Finding{{
			Reviewer:       "security-reviewer",
			Severity:       "high",
			Classification: "blocking",
			Title:          "Committed predecessor blocker",
			Finding:        "The first committed revision still has a release risk.",
			Expected:       "A later commit can continue this authenticated committed chain.",
			Found:          "The local run record will be absent in a fresh clone.",
			Recommendation: "Commit the review evidence and fix the source.",
			Fingerprint:    "security:committed-predecessor",
		}}
	}
	t.Cleanup(func() { runPRReadyReviewers = original })

	first, err := Create(root, Options{Base: "main", EvidencePath: evidence, Now: time.Date(2026, 9, 3, 3, 40, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if !first.Blocking {
		t.Fatalf("first run must record the committed predecessor blocker: %+v", first)
	}
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	runGit("add", "docs/metareview")
	runGit("commit", "-m", "record predecessor review")
	if err := os.WriteFile(filepath.Join(root, "seed.txt"), []byte("fixed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "seed.txt")
	runGit("commit", "-m", "fix predecessor finding")
	if err := os.RemoveAll(filepath.Join(root, ".metareview")); err != nil {
		t.Fatal(err)
	}

	second, err := Create(root, Options{Base: "main", EvidencePath: evidence, PreviousRunID: first.RunID, Now: time.Date(2026, 9, 3, 3, 40, 1, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || second.Reused || second.Blocking {
		t.Fatalf("fresh clone must continue the authenticated committed fix chain with a new review: calls=%d result=%+v", calls, second)
	}
	runs, err := os.ReadFile(filepath.Join(root, ".metareview", "runs.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(runs), `"previousRunId":"`+first.RunID+`"`) || !strings.Contains(string(runs), `"attemptNumber":2`) {
		t.Fatalf("fresh-clone continuation must retain predecessor and attempt identity:\n%s", runs)
	}
}

func TestEscalatedReviewAtAncestorHeadDoesNotLockNewBranchCommit(t *testing.T) {
	root := smallPRReadyRepo(t)
	t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "1")
	evidence := filepath.Join(t.TempDir(), "evidence.md")
	if err := os.WriteFile(evidence, []byte("go test ./... exited 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	original := runPRReadyReviewers
	runPRReadyReviewers = func(reviewers.PRReadyContext) []reviewers.Finding {
		return []reviewers.Finding{{
			Reviewer:       "security-reviewer",
			Severity:       "high",
			Classification: "blocking",
			Title:          "Persistent blocker",
			Fingerprint:    "security:persistent-blocker",
		}}
	}
	t.Cleanup(func() { runPRReadyReviewers = original })

	first, err := Create(root, Options{Base: "main", EvidencePath: evidence, MaxAttempts: 2, Now: time.Date(2026, 9, 3, 3, 45, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Create(root, Options{Base: "main", EvidencePath: evidence, PreviousRunID: first.RunID, MaxAttempts: 2, Now: time.Date(2026, 9, 3, 3, 45, 1, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if second.Verdict != "ESCALATED" {
		t.Fatalf("second attempt must establish the old-head escalation: %+v", second)
	}
	if err := os.WriteFile(filepath.Join(root, "seed.txt"), []byte("redesigned\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "add", "seed.txt")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "commit", "-m", "redesign after escalation")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}

	third, err := Create(root, Options{Base: "main", EvidencePath: evidence, MaxAttempts: 2, Now: time.Date(2026, 9, 3, 3, 45, 2, 0, time.UTC)})
	if err != nil {
		t.Fatalf("a new branch commit must start a fresh attempt budget: %v", err)
	}
	if third.Verdict != "NEEDS_REVISION" {
		t.Fatalf("new head must be attempt 1 rather than inherit escalation: %+v", third)
	}
}

func TestExplicitPRBlockerRemainsCurrentWhenGitHubIsUnavailable(t *testing.T) {
	root := smallPRReadyRepo(t)
	t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "1")
	remote := exec.Command("git", "remote", "add", "origin", "https://github.com/acme/repo.git")
	remote.Dir = root
	if out, err := remote.CombinedOutput(); err != nil {
		t.Fatalf("add origin: %v\n%s", err, out)
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := os.MkdirAll(filepath.Join(root, ".metareview"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".metareview", "findings.jsonl"), []byte(`{"schemaVersion":1,"id":"mrvf-pr-99","runId":"mrv-pr-99","scope":"pr-ready","reviewer":"security-reviewer","severity":"high","classification":"blocking","status":"open","title":"PR-linked blocker","fingerprint":"security:pr-99","target":{"type":"pull-request","id":"99"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	evidence := filepath.Join(t.TempDir(), "evidence.md")
	if err := os.WriteFile(evidence, []byte("go test ./... exited 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	original := runPRReadyReviewers
	var reviewedEvidence string
	runPRReadyReviewers = func(ctx reviewers.PRReadyContext) []reviewers.Finding {
		reviewedEvidence = ctx.PREvidenceMarkdown
		if !strings.Contains(reviewedEvidence, "PR-linked blocker") {
			return nil
		}
		return []reviewers.Finding{{Reviewer: "security-reviewer", Severity: "high", Classification: "blocking", Title: "PR-linked blocker", Fingerprint: "security:pr-99-current"}}
	}
	t.Cleanup(func() { runPRReadyReviewers = original })

	result, err := Create(root, Options{Base: "main", EvidencePath: evidence, GitHubPR: "99", Now: time.Date(2026, 9, 3, 3, 50, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reviewedEvidence, "PR-linked blocker") || !result.Blocking {
		t.Fatalf("explicit PR identity must retain its blocker when live GitHub context is unavailable: evidence=%q result=%+v", reviewedEvidence, result)
	}
}

func TestSmallPRReadyRepoIgnoresGlobalSigningAndHooks(t *testing.T) {
	settings := t.TempDir()
	hooks := filepath.Join(settings, "hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	globalConfig := filepath.Join(settings, "gitconfig")
	configure := func(key, value string) {
		t.Helper()
		cmd := exec.Command("git", "config", "--file", globalConfig, key, value)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("configure global Git %s: %v\n%s", key, err, out)
		}
	}
	configure("commit.gpgsign", "true")
	configure("gpg.program", "/usr/bin/false")
	configure("core.hooksPath", hooks)
	t.Setenv("GIT_CONFIG_GLOBAL", globalConfig)

	root := smallPRReadyRepo(t)
	for key, want := range map[string]string{
		"commit.gpgsign": "false",
		"core.hooksPath": filepath.Join(root, ".git", "hooks"),
	} {
		cmd := exec.Command("git", "config", "--local", "--get", key)
		cmd.Dir = root
		out, err := cmd.Output()
		if err != nil || strings.TrimSpace(string(out)) != want {
			t.Fatalf("fixture-local %s = %q, want %q (err=%v)", key, strings.TrimSpace(string(out)), want, err)
		}
	}
}

func TestPriorCurrentTargetBlockerSurvivesMissingGitHubContextAndPreventsReuse(t *testing.T) {
	root := smallPRReadyRepo(t)
	t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "1")
	remote := exec.Command("git", "remote", "add", "origin", "https://github.com/acme/repo.git")
	remote.Dir = root
	if out, err := remote.CombinedOutput(); err != nil {
		t.Fatalf("add origin: %v\n%s", err, out)
	}
	bin := t.TempDir()
	gh := filepath.Join(bin, "gh")
	if err := os.WriteFile(gh, []byte(`#!/bin/sh
if [ "$1" = "auth" ] && [ "$2" = "status" ]; then
  exit 0
fi
if [ "$1" = "pr" ] && [ "$2" = "view" ]; then
  printf '%s\n' '{"number":99,"url":"https://github.com/acme/repo/pull/99","title":"Current target","body":"","reviewDecision":"APPROVED","comments":[],"reviews":[]}'
  exit 0
fi
exit 1
`), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	evidence := filepath.Join(t.TempDir(), "evidence.md")
	if err := os.WriteFile(evidence, []byte("go test ./... exited 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	original := runPRReadyReviewers
	calls := 0
	runPRReadyReviewers = func(reviewers.PRReadyContext) []reviewers.Finding {
		calls++
		if calls != 2 {
			return nil
		}
		return []reviewers.Finding{{
			Reviewer:       "security-reviewer",
			Severity:       "high",
			Classification: "blocking",
			Title:          "Current target blocker",
			Finding:        "The current branch still has an unresolved release risk.",
			Expected:       "The risk remains blocking until an explicit fix run supersedes it.",
			Found:          "The risk is still open.",
			Recommendation: "Resolve the current-target risk and continue its run chain.",
			Fingerprint:    "security:current-target-blocker",
		}}
	}
	t.Cleanup(func() { runPRReadyReviewers = original })

	first, err := Create(root, Options{Base: "main", EvidencePath: evidence, Now: time.Date(2026, 9, 3, 4, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if first.Blocking || (first.Verdict != "PASS" && first.Verdict != "PASS_ADVISORY") {
		t.Fatalf("first run must establish the otherwise reusable PASS: %+v", first)
	}
	if err := os.WriteFile(evidence, []byte("go test ./... and go vet ./... exited 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := Create(root, Options{Base: "main", EvidencePath: evidence, GitHubPR: "99", Now: time.Date(2026, 9, 3, 4, 0, 1, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if !second.Blocking || second.Verdict != "NEEDS_REVISION" {
		t.Fatalf("second run must record the current-target blocker: %+v", second)
	}
	if err := os.WriteFile(evidence, []byte("go test ./... exited 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	third, err := Create(root, Options{Base: "main", EvidencePath: evidence, Now: time.Date(2026, 9, 3, 4, 0, 2, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 || third.Reused || !third.Blocking || third.Verdict != "NEEDS_REVISION" {
		t.Fatalf("missing GitHub context must not erase the current blocker or reuse the old PASS: calls=%d result=%+v", calls, third)
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(third.ReviewRel)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "Current target blocker") {
		t.Fatalf("current blocker must remain visible in the third review:\n%s", body)
	}
	fourth, err := Create(root, Options{Base: "main", EvidencePath: evidence, Now: time.Date(2026, 9, 3, 4, 0, 3, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 4 || fourth.Reused || !fourth.Blocking || fourth.Verdict != "NEEDS_REVISION" {
		t.Fatalf("same-head dedup must not erase the sole inherited blocker: calls=%d result=%+v", calls, fourth)
	}
	body, err = os.ReadFile(filepath.Join(root, filepath.FromSlash(fourth.ReviewRel)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "Current target blocker") {
		t.Fatalf("inherited current blocker must remain visible after same-head dedup:\n%s", body)
	}
}

func TestExplicitPreviousRunRejectsStalePersistedDigest(t *testing.T) {
	root := smallPRReadyRepo(t)
	t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "1")
	evidence := filepath.Join(t.TempDir(), "evidence.md")
	if err := os.WriteFile(evidence, []byte("go test ./... exited 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := Create(root, Options{Base: "main", EvidencePath: evidence, Now: time.Date(2026, 9, 3, 3, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, filepath.FromSlash(first.ReviewRel))
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(string(body), "sha256:", "sha256:b", 1)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = Create(root, Options{Base: "main", EvidencePath: evidence, PreviousRunID: first.RunID, Now: time.Date(2026, 9, 3, 3, 0, 1, 0, time.UTC)})
	if err == nil || !strings.Contains(err.Error(), "stale or unauthenticated reviewer input digest") {
		t.Fatalf("expected stale explicit previous run to fail, got %v", err)
	}
}

func TestLatestLogsByTargetReturnsDeterministicTargetOrder(t *testing.T) {
	logs := []reviewlog.Summary{
		{Target: "task-z", RunID: "mrv-20260901-older", Path: "docs/metareview/reviews/2026-09-01-older.md"},
		{Target: "task-a", RunID: "mrv-20260902-only", Path: "docs/metareview/reviews/2026-09-02-only.md"},
		{Target: "task-z", RunID: "mrv-20260903-newer", Path: "docs/metareview/reviews/2026-09-03-newer.md"},
	}

	got := latestLogsByTarget(logs)
	if len(got) != 2 || got[0].Target != "task-a" || got[1].Target != "task-z" || got[1].RunID != "mrv-20260903-newer" {
		t.Fatalf("latest logs must use stable target order and retain the latest run: %+v", got)
	}
}

func smallPRReadyRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("init", "-b", "main")
	run("config", "--local", "commit.gpgsign", "false")
	run("config", "--local", "core.hooksPath", filepath.Join(root, ".git", "hooks"))
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test User")
	run("add", ".")
	run("commit", "-m", "initial")
	run("checkout", "-b", "feature")
	if err := os.WriteFile(filepath.Join(root, "seed.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "change")
	return root
}

func TestContextMarkdownIncludesReviewManifest(t *testing.T) {
	profile := contextprofile.Profile{Files: []contextprofile.FileProfile{{Path: "internal/a.go", DiffBytes: 10}}}
	manifest, aggregate := manifestArgs(profile)
	body := contextMarkdown(
		"mrv-pr",
		gitcontext.Context{BaseSHA: "base", HeadSHA: "head", Branch: "feature", ChangedFiles: []string{"internal/a.go"}},
		profile,
		contextprofile.ShardPlan{},
		"",
		manifest,
		aggregate,
		knowledge.Context{},
		nil,
		"go test ./... exited 0",
		githubcontext.Context{},
		"## metareview PR Evidence\n\nvalidation",
		"gate",
		"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	)

	for _, required := range []string{
		"## Review Manifest",
		"Manifest verdict:",
		"Runtime assessment: static-only; runtime not assessed",
		"internal/a.go",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("pr-ready context missing %q:\n%s", required, body)
		}
	}
}

func TestContextMarkdownDispositionsGeneratedReviewArtifacts(t *testing.T) {
	profile := contextprofile.Profile{
		Files:                  []contextprofile.FileProfile{{Path: "internal/a.go", DiffBytes: 10}},
		GeneratedExcludedFiles: []string{"docs/metareview/reviews/generated-review.md"},
	}
	manifest, aggregate := manifestArgs(profile)
	body := contextMarkdown(
		"mrv-pr",
		gitcontext.Context{BaseSHA: "base", HeadSHA: "head", Branch: "feature", ChangedFiles: []string{"internal/a.go"}},
		profile,
		contextprofile.ShardPlan{},
		"",
		manifest,
		aggregate,
		knowledge.Context{},
		nil,
		"go test ./... exited 0",
		githubcontext.Context{},
		"## metareview PR Evidence\n\nvalidation",
		"gate",
		"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	)

	for _, required := range []string{
		"docs/metareview/reviews/generated-review.md: generated",
		"metareview generated review artifact excluded from source manifest",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("pr-ready context missing generated disposition %q:\n%s", required, body)
		}
	}
	if strings.Contains(body, "missing disposition for docs/metareview/reviews/generated-review.md") {
		t.Fatalf("pr-ready context should not flag generated review artifact as missing disposition:\n%s", body)
	}
}

// manifestArgs builds the manifest the review now passes into contextMarkdown.
func manifestArgs(profile contextprofile.Profile) (reviewmanifest.Manifest, reviewmanifest.AggregateResult) {
	manifest := reviewmanifest.Build(reviewmanifest.Input{
		Scope:            "pr-ready",
		Profile:          profile,
		PathDispositions: reviewmanifest.GeneratedPathDispositions(profile.GeneratedExcludedFiles),
	})
	return manifest, reviewmanifest.Aggregate(manifest)
}
