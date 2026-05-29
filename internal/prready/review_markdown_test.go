package prready

import (
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/findings"
	"github.com/dsifry/metareview/internal/runchain"
)

func TestReviewMarkdownSeparatesNonBlockingFindings(t *testing.T) {
	records := []findings.Record{
		{Reviewer: "pr-readiness-reviewer", Classification: "advisory", Severity: "medium", Title: "Prefer helper", Finding: "Helper would reduce duplication."},
		{Reviewer: "validation-reviewer", Classification: "follow-up", Severity: "low", Title: "Track cleanup", Finding: "Cleanup belongs in a later target."},
		{Reviewer: "security-reviewer", Classification: "warning", Severity: "high", Title: "Unknown class", Finding: "Unknown classification was downgraded to warning."},
	}
	md := reviewMarkdown("mrv-pr", "ctx.md", "", "gate", "PASS_ADVISORY", records, "evidence", reviewMetadata{AdvisoryFindingCount: 1, FollowUpFindingCount: 1, WarningFindingCount: 1})
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
