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
