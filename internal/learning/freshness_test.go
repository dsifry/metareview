package learning

import (
	"testing"

	"github.com/dsifry/metareview/internal/findings"
)

// Spec §6.9: mutation-freshness findings are evidence bookkeeping, not lessons — a stale row that
// fresh evidence replaced is neither a fixed defect nor a repeated blocker theme.
func TestLearningSkipsFreshnessFindings(t *testing.T) {
	stale := findings.Record{ID: "f1", Status: "fixed", FixedInRunID: "r2", KnowledgeCandidate: true, Classification: "blocking", Severity: "high",
		Title: "Mutation evidence stale: src/a.ts changed", Fingerprint: "mutation:stale:enforce:stryker:src/a.ts:01234567"}
	again := stale
	again.ID = "f2"
	if got := knowledgeFromFindings([]findings.Record{stale}); len(got) != 0 {
		t.Errorf("no knowledge candidate from a freshness row: %+v", got)
	}
	if got := repeatedBlockerThemes([]findings.Record{stale, again}); len(got) != 0 {
		t.Errorf("no blocker theme from freshness rows: %+v", got)
	}
	// An ordinary finding is still learned from.
	ordinary := stale
	ordinary.Fingerprint = "quality:lint-marker"
	if got := knowledgeFromFindings([]findings.Record{ordinary}); len(got) == 0 {
		t.Error("ordinary findings still produce candidates")
	}
}
