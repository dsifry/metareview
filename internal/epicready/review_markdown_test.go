package epicready

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/findings"
	"github.com/dsifry/metareview/internal/reviewlog"
	"github.com/dsifry/metareview/internal/runchain"
)

// The epic-ready log must carry a Covered paths: header the reviewlog parser reads back, so `status` can
// credit a clean epic-ready review for the files it examined.
func TestEpicReadyReviewMarkdownEmitsRoundTrippableCoveredPaths(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "docs", "metareview", "reviews")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	md := reviewMarkdown("mrv-epic-cov", "epic-1", "ctx.md", "", "gate", "PASS", []string{"internal/foo.go", "a,b.go"}, nil, reviewMetadata{})
	if err := os.WriteFile(filepath.Join(dir, "mrv-epic-cov.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	logs, err := reviewlog.Discover(root)
	if err != nil || len(logs) != 1 {
		t.Fatalf("Discover: %v (%d logs)", err, len(logs))
	}
	s := logs[0]
	if !s.CoveredPathsKnown || len(s.CoveredPaths) != 2 || s.CoveredPaths[1] != "a,b.go" {
		t.Fatalf("epic-ready covered paths did not round-trip: known=%v %+v", s.CoveredPathsKnown, s.CoveredPaths)
	}
}

func TestReviewMarkdownSeparatesNonBlockingFindings(t *testing.T) {
	records := []findings.Record{
		{Reviewer: "epic-integration-reviewer", Classification: "advisory", Severity: "medium", Title: "Prefer helper", Finding: "Helper would reduce duplication."},
		{Reviewer: "acceptance-reviewer", Classification: "follow-up", Severity: "low", Title: "Track cleanup", Finding: "Cleanup belongs in a later target."},
		{Reviewer: "architecture-reviewer", Classification: "warning", Severity: "high", Title: "Unknown class", Finding: "Unknown classification was downgraded to warning."},
	}
	md := reviewMarkdown("mrv-epic", "epic-1", "ctx.md", "", "gate", "PASS_ADVISORY", []string{"internal/foo.go"}, records, reviewMetadata{AdvisoryFindingCount: 1, FollowUpFindingCount: 1, WarningFindingCount: 1})
	if strings.Contains(md, "| epic-integration-reviewer | NEEDS_REVISION | 1 |") || strings.Contains(md, "| acceptance-reviewer | NEEDS_REVISION | 1 |") {
		t.Fatalf("non-blocking findings must not render as blocking reviewer failures:\n%s", md)
	}
	for _, required := range []string{"| epic-integration-reviewer | PASS_ADVISORY | 0 | Prefer helper |", "## Advisory Findings", "## Follow-up Findings", "## Warnings", "Unknown classification was downgraded to warning."} {
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
	md := runChainMarkdown("mrv-epic", "ESCALATED", reviewMetadata{
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
