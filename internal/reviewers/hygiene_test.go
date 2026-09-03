package reviewers

import (
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/findings"
)

func TestGeneratedArtifactHygieneAdvisesOnStaleHeadBlockers(t *testing.T) {
	got := generatedArtifactHygieneFindings(HygieneContext{StaleHeadBlockers: []findings.Record{
		{ID: "mrvf-old-001", Severity: "high", Title: "Stale bug", Reviewer: "security-reviewer"},
	}})
	if len(got) != 1 {
		t.Fatalf("a stale-head finding must produce one note, got %d", len(got))
	}
	f := got[0]
	if f.Reviewer != "generated-artifact-hygiene-reviewer" {
		t.Fatalf("wrong reviewer: %q", f.Reviewer)
	}
	// R5: ADVISORY, never blocking — same-target stale already blocks via the verdict, and blocking on
	// cross-target stale would false-positive on a legit other-branch blocker.
	if f.Classification != "advisory" {
		t.Fatalf("the hygiene finding must be advisory, got %q", f.Classification)
	}
	if !strings.Contains(f.Found, "mrvf-old-001") {
		t.Fatalf("the finding must name the stale finding: %q", f.Found)
	}
	if !strings.Contains(f.Recommendation, ".metareview/findings.jsonl") {
		t.Fatalf("the finding must name the remediation: %q", f.Recommendation)
	}
}

func TestGeneratedArtifactHygienePassesWhenNoStaleBlockers(t *testing.T) {
	if got := generatedArtifactHygieneFindings(HygieneContext{}); len(got) != 0 {
		t.Fatalf("no stale-head blockers must produce no finding, got %+v", got)
	}
}

func TestGeneratedArtifactHygieneSingularVsPluralWording(t *testing.T) {
	one := generatedArtifactHygieneFindings(HygieneContext{StaleHeadBlockers: []findings.Record{{ID: "a"}}})
	if !strings.Contains(one[0].Finding, "1 open blocking finding ") {
		t.Fatalf("singular wording expected: %q", one[0].Finding)
	}
	two := generatedArtifactHygieneFindings(HygieneContext{StaleHeadBlockers: []findings.Record{{ID: "a"}, {ID: "b"}}})
	if !strings.Contains(two[0].Finding, "2 open blocking findings ") {
		t.Fatalf("plural wording expected: %q", two[0].Finding)
	}
}
