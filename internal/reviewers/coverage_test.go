package reviewers

import (
	"strings"
	"testing"
)

// With the working tree left out of a PR-ready review, any dirty (non-generated) files raise a
// blocking finding so the reviewer never silently ignores uncommitted local changes.
func TestRunPRReadyFlagsExcludedDirtyWorkingTree(t *testing.T) {
	got := RunPRReady(PRReadyContext{
		IncludeWorkingTree:    false,
		WorkingTreeDirtyFiles: []string{"internal/foo/bar.go", "README.md"},
	})
	found := false
	for _, f := range got {
		if f.Title == "Working tree changes excluded from PR-ready review" {
			found = true
			if !strings.Contains(f.Found, "internal/foo/bar.go") {
				t.Fatalf("finding should name the dirty files: %q", f.Found)
			}
		}
	}
	if !found {
		t.Fatalf("expected a working-tree-excluded finding, got %+v", got)
	}
}

// When the caller opts into reviewing the working tree, the dirty-files finding is suppressed.
func TestRunPRReadyIncludeWorkingTreeSuppressesDirtyFinding(t *testing.T) {
	got := RunPRReady(PRReadyContext{
		IncludeWorkingTree:    true,
		WorkingTreeDirtyFiles: []string{"internal/foo/bar.go"},
	})
	for _, f := range got {
		if f.Title == "Working tree changes excluded from PR-ready review" {
			t.Fatalf("include-working-tree must suppress the dirty-files finding: %+v", f)
		}
	}
}

// missingChildEvidence ignores children with a blank ID and reports only real children that lack a
// passing review log or inline evidence.
func TestMissingChildEvidenceSkipsBlankIDs(t *testing.T) {
	ctx := EpicReadyContext{
		Children: []EpicChild{
			{ID: ""},   // blank — must be skipped, never reported
			{ID: "c1"}, // no passing log, no evidence — reported
			{ID: "c2"}, // has a passing log — not reported
		},
		ReviewLogs: []EpicReviewLog{{Target: "c2", Verdict: "PASS"}},
	}
	missing := missingChildEvidence(ctx)
	if len(missing) != 1 || missing[0] != "c1" {
		t.Fatalf("only c1 should be missing evidence, got %+v", missing)
	}
}

// firstNonEmpty returns the first non-blank value trimmed, and "" when all are blank.
func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "  ", "\tv\t"); got != "v" {
		t.Fatalf("expected first non-blank trimmed, got %q", got)
	}
	if got := firstNonEmpty("", "   "); got != "" {
		t.Fatalf("all-blank should yield empty, got %q", got)
	}
}

func TestBoolString(t *testing.T) {
	if boolString(true) != "yes" || boolString(false) != "no" {
		t.Fatalf("boolString: got %q/%q, want yes/no", boolString(true), boolString(false))
	}
}

// contextRiskFound summarizes whichever risk signals are present, and falls back to a neutral
// sentence when the profile flagged risk but carried no quantified reason.
func TestContextRiskFound(t *testing.T) {
	if got := contextRiskFound(GitContext{}); got != "Context profile reported risk." {
		t.Fatalf("no signals should yield the neutral sentence, got %q", got)
	}
	got := contextRiskFound(GitContext{
		RiskReasons:           []string{"oversized"},
		RawDiffBytes:          1000,
		UntrackedOmittedCount: 3,
	})
	for _, want := range []string{"Reasons: oversized", "Raw diff bytes: 1000", "Untracked files omitted: 3"} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary %q missing %q", got, want)
		}
	}
}

// duplicatePathFindings flags a changed path that resembles an inventoried service path, but never
// flags a path against its own exact self.
func TestDuplicatePathFindings(t *testing.T) {
	// Exact self-match is not a duplicate: no finding.
	if got := duplicatePathFindings(
		KnowledgeContext{ServiceInventory: "internal/foo/bar.go"},
		[]string{"internal/foo/bar.go"},
	); len(got) != 0 {
		t.Fatalf("an exact-path self match must not be flagged, got %+v", got)
	}
	// A near-duplicate (same normalized token, different literal) is flagged.
	got := duplicatePathFindings(
		KnowledgeContext{ServiceInventory: "internal/foo/service.go"},
		[]string{"internal/foo/serviceV2.go"},
	)
	if len(got) != 1 || got[0].Title != "Possible duplicate code path" {
		t.Fatalf("a near-duplicate path should be flagged, got %+v", got)
	}
}

// An empty inventory short-circuits with no findings.
func TestDuplicatePathFindingsEmptyInventory(t *testing.T) {
	if got := duplicatePathFindings(KnowledgeContext{}, []string{"internal/foo/bar.go"}); got != nil {
		t.Fatalf("no inventory should yield no findings, got %+v", got)
	}
}

func TestUniqueStrings(t *testing.T) {
	got := uniqueStrings([]string{"a", "a", "b", "a", "c", "b"})
	want := []string{"a", "b", "c"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("uniqueStrings dedup/order wrong: got %v want %v", got, want)
	}
}
