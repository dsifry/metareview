package taskdone

import (
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/contextprofile"
	"github.com/dsifry/metareview/internal/findings"
	"github.com/dsifry/metareview/internal/gitcontext"
	"github.com/dsifry/metareview/internal/knowledge"
	"github.com/dsifry/metareview/internal/reviewmanifest"
	"github.com/dsifry/metareview/internal/runchain"
	"github.com/dsifry/metareview/internal/tasksource"
)

func TestReviewMarkdownSeparatesNonBlockingFindings(t *testing.T) {
	records := []findings.Record{
		{Reviewer: "code-quality-reviewer", Classification: "advisory", Severity: "medium", Title: "Prefer helper", Finding: "Helper would reduce duplication."},
		{Reviewer: "architecture-reviewer", Classification: "follow-up", Severity: "low", Title: "Track cleanup", Finding: "Cleanup belongs in a later target."},
		{Reviewer: "security-reviewer", Classification: "warning", Severity: "high", Title: "Unknown class", Finding: "Unknown classification was downgraded to warning."},
	}
	md := reviewMarkdown("mrv-task", "task-1", "ctx.md", "", "gate", "PASS_ADVISORY", records, "", reviewMetadata{AdvisoryFindingCount: 1, FollowUpFindingCount: 1, WarningFindingCount: 1})
	if strings.Contains(md, "| code-quality-reviewer | NEEDS_REVISION | 1 |") || strings.Contains(md, "| architecture-reviewer | NEEDS_REVISION | 1 |") {
		t.Fatalf("non-blocking findings must not render as blocking reviewer failures:\n%s", md)
	}
	for _, required := range []string{"| code-quality-reviewer | PASS_ADVISORY | 0 | Prefer helper |", "## Advisory Findings", "## Follow-up Findings", "## Warnings", "Unknown classification was downgraded to warning."} {
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
	md := runChainMarkdown("mrv-task", "ESCALATED", reviewMetadata{
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

func TestContextMarkdownIncludesReviewManifest(t *testing.T) {
	profile := contextprofile.Profile{Files: []contextprofile.FileProfile{{Path: "internal/a.go", DiffBytes: 10}}}
	manifest, aggregate := manifestArgs(profile)
	body := contextMarkdown(
		"mrv-task",
		tasksource.Source{ID: "task-1", Body: "Review manifest task"},
		gitcontext.Context{BaseSHA: "base", HeadSHA: "head", Branch: "feature", ChangedFiles: []string{"internal/a.go"}},
		profile,
		contextprofile.ShardPlan{},
		"",
		manifest,
		aggregate,
		knowledge.Context{},
		"go test ./... exited 0",
		"gate",
	)

	for _, required := range []string{
		"## Review Manifest",
		"Manifest verdict:",
		"Runtime assessment: static-only; runtime not assessed",
		"internal/a.go",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("task-done context missing %q:\n%s", required, body)
		}
	}
}

func TestContextMarkdownDispositionsGeneratedReviewArtifacts(t *testing.T) {
	profile := contextprofile.Profile{
		Files:                  []contextprofile.FileProfile{{Path: "internal/a.go", DiffBytes: 10}},
		GeneratedExcludedFiles: []string{"docs/metareview/context/generated-context.md"},
	}
	manifest, aggregate := manifestArgs(profile)
	body := contextMarkdown(
		"mrv-task",
		tasksource.Source{ID: "task-1", Body: "Review manifest task"},
		gitcontext.Context{BaseSHA: "base", HeadSHA: "head", Branch: "feature", ChangedFiles: []string{"internal/a.go"}},
		profile,
		contextprofile.ShardPlan{},
		"",
		manifest,
		aggregate,
		knowledge.Context{},
		"go test ./... exited 0",
		"gate",
	)

	for _, required := range []string{
		"docs/metareview/context/generated-context.md: generated",
		"metareview generated review artifact excluded from source manifest",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("task-done context missing generated disposition %q:\n%s", required, body)
		}
	}
	if strings.Contains(body, "missing disposition for docs/metareview/context/generated-context.md") {
		t.Fatalf("task-done context should not flag generated review artifact as missing disposition:\n%s", body)
	}
}

// manifestArgs builds the manifest the review now passes into contextMarkdown.
func manifestArgs(profile contextprofile.Profile) (reviewmanifest.Manifest, reviewmanifest.AggregateResult) {
	manifest := reviewmanifest.Build(reviewmanifest.Input{
		Scope:            "task-done",
		Profile:          profile,
		PathDispositions: reviewmanifest.GeneratedPathDispositions(profile.GeneratedExcludedFiles),
	})
	return manifest, reviewmanifest.Aggregate(manifest)
}

// The review log has to carry the head and the files it read, because that is the only copy that
// survives leaving this machine: .metareview/runs.jsonl is untracked, so a clone, a fresh
// worktree or a CI checkout had logs it could not attribute to any commit — every file read as
// UNREVIEWED and no historical blocker was ever in scope. This asserts the WRITER emits them;
// the parser was already tested, which is exactly why the gap survived.
func TestReviewMarkdownRecordsHeadAndCoveredPaths(t *testing.T) {
	meta := reviewMetadata{HeadSHA: "abc1234def", CoveredPaths: []string{"internal/a.go", "internal/b.go"}}
	md := reviewMarkdown("mrv-1", "task-1", "ctx.md", "", "gate", "PASS", nil, "", meta)
	for _, required := range []string{"Head: `abc1234def`", "Covered paths: `[\"internal/a.go\",\"internal/b.go\"]`"} {
		if !strings.Contains(md, required) {
			t.Fatalf("review markdown missing %q:\n%s", required, md)
		}
	}
	// A review with neither says so explicitly. "unknown" and "none" are answers; a blank field
	// would be indistinguishable from a log written before these existed.
	md = reviewMarkdown("mrv-1", "task-1", "ctx.md", "", "gate", "PASS", nil, "", reviewMetadata{})
	for _, required := range []string{"Head: `unknown`", "Covered paths: `none`"} {
		if !strings.Contains(md, required) {
			t.Fatalf("review markdown missing %q:\n%s", required, md)
		}
	}
}
