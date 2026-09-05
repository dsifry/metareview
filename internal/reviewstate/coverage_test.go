package reviewstate

import (
	"errors"
	"testing"

	"github.com/dsifry/metareview/internal/findings"
	"github.com/dsifry/metareview/internal/reviewlog"
)

func TestProjectSurfacesReadErrors(t *testing.T) {
	root := t.TempDir()
	// discoverLogs error.
	origDiscover := discoverLogs
	t.Cleanup(func() { discoverLogs = origDiscover })
	discoverLogs = func(string) ([]reviewlog.Summary, error) { return nil, errors.New("discover boom") }
	if _, err := Project(root, Options{}); err == nil || err.Error() != "discover boom" {
		t.Fatalf("expected the discover error, got %v", err)
	}
	discoverLogs = origDiscover
	// unresolvedBlocking error (discover succeeds).
	origBlockers := unresolvedBlocking
	t.Cleanup(func() { unresolvedBlocking = origBlockers })
	unresolvedBlocking = func(string) ([]findings.Record, error) { return nil, errors.New("blockers boom") }
	if _, err := Project(root, Options{}); err == nil || err.Error() != "blockers boom" {
		t.Fatalf("expected the blockers error, got %v", err)
	}
}

// A prior pr-ready log for an unrelated target is filed historical (with its run id recorded), so its
// blockers do not join the current pr-ready decision.
func TestProjectRecordsPrReadyUnrelatedHistorical(t *testing.T) {
	logs := []reviewlog.Summary{
		// An unrelated pr-ready target (its TargetRecord is not linked to the current review).
		{RunID: "mrv-old", Kind: "pr-ready", Verdict: "NEEDS_REVISION", TargetRecord: map[string]string{"type": "path", "id": "src/other.go"}, HasUnresolvedBlockers: true},
	}
	proj := ProjectRecords(logs, nil, Options{
		Scope:        "pr-ready",
		ChangedPaths: []string{"src/current.go"},
	})
	if len(proj.currentReviewLogs) != 0 {
		t.Fatalf("an unrelated pr-ready log must not be a current reviewer input: %+v", proj.currentReviewLogs)
	}
	if len(proj.historicalLogs) != 1 {
		t.Fatalf("the unrelated pr-ready log must be historical: %+v", proj.historicalLogs)
	}
}

func TestLegacyPreviousRunIDs(t *testing.T) {
	// Empty previous -> nil.
	if got := LegacyPreviousRunIDs(nil, ""); got != nil {
		t.Errorf("empty previous -> nil, got %v", got)
	}
	// Not found -> nil.
	if got := LegacyPreviousRunIDs([]reviewlog.Summary{{RunID: "a"}}, "missing"); got != nil {
		t.Errorf("unknown previous -> nil, got %v", got)
	}
	// A cycle -> nil.
	cyclic := []reviewlog.Summary{
		{RunID: "a", PreviousRunID: "b"},
		{RunID: "b", PreviousRunID: "a"},
	}
	if got := LegacyPreviousRunIDs(cyclic, "a"); got != nil {
		t.Errorf("a cycle -> nil, got %v", got)
	}
	// A valid chain resolves root-first.
	chain := []reviewlog.Summary{
		{RunID: "a"},
		{RunID: "b", PreviousRunID: "a"},
	}
	got := LegacyPreviousRunIDs(chain, "b")
	if len(got) != 2 {
		t.Fatalf("expected a two-link chain, got %v", got)
	}
}

func TestUnrelatedHelpers(t *testing.T) {
	// unrelatedArtifact: a non-artifact log is never unrelated by this check.
	if unrelatedArtifact(reviewlog.Summary{Kind: "task-done", Target: "x"}, map[string]bool{}) {
		t.Error("a non-artifact log must not be flagged by unrelatedArtifact")
	}
	// An artifact log with a blank target normalizes to "" -> not unrelated (fail closed).
	if unrelatedArtifact(reviewlog.Summary{Kind: "artifact", Target: "   "}, map[string]bool{"src/a.go": true}) {
		t.Error("a blank artifact target must not be treated as unrelated")
	}
	// An artifact log whose target is not among the changed paths IS unrelated; one that overlaps is not.
	if !unrelatedArtifact(reviewlog.Summary{Kind: "artifact", Target: "docs/old.md"}, map[string]bool{"docs/new.md": true}) {
		t.Error("an artifact for an unchanged path should be unrelated")
	}
	if unrelatedArtifact(reviewlog.Summary{Kind: "artifact", Target: "docs/new.md"}, map[string]bool{"docs/new.md": true}) {
		t.Error("an artifact for a changed path should not be unrelated")
	}
	// unrelatedTargetLog: a task-done log whose target is linked is NOT unrelated.
	linked := map[string]bool{canonicalTargetKey("task", "task-1"): true}
	if unrelatedTargetLog(reviewlog.Summary{Kind: "task-done", Target: "task-1"}, map[string]bool{}, linked) {
		t.Error("a linked task-done log must not be unrelated")
	}
}

func TestFindingTargetShapes(t *testing.T) {
	// map[string]string case.
	tt, id := findingTarget(map[string]string{"type": "path", "id": "x"})
	if tt != "path" || id != "x" {
		t.Errorf("map[string]string: %q %q", tt, id)
	}
	// map[string]any case (path fallback when id empty).
	tt, id = findingTarget(map[string]any{"type": "path", "path": "p"})
	if tt != "path" || id != "p" {
		t.Errorf("map[string]any: %q %q", tt, id)
	}
	// default (unrecognized) -> empty.
	if tt, id := findingTarget("neither"); tt != "" || id != "" {
		t.Errorf("default: %q %q", tt, id)
	}
}

func TestNormalizePath(t *testing.T) {
	if got := normalizePath("  "); got != "" {
		t.Errorf("blank path -> empty, got %q", got)
	}
	if got := normalizePath("a/../b/c.go"); got != "b/c.go" {
		t.Errorf("normalizePath cleaned: %q", got)
	}
}
