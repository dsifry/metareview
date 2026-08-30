package run

import (
	"reflect"
	"testing"
)

// Independent convergence is the cheapest evidence that a defect is a class. The signal was being
// destroyed before anything could use it: reviewLenses.Reduce flattened each lens report into a
// findings list and dropped the lens name, so four lenses agreeing and one lens repeating itself
// produced identical data. Counting DISTINCT sources is what separates them.
func TestLensConvergenceCountsDistinctReviewers(t *testing.T) {
	findings := []Finding{
		{File: "a.go", Source: "Security"},
		{File: "a.go", Source: "Architecture"},
		{File: "a.go", Source: "Testing-quality"},
		// The same lens reporting the same file three times is ONE reviewer, not three. This is
		// the case the count exists to distinguish, and a naive tally of findings gets it wrong.
		{File: "b.go", Source: "Security"},
		{File: "b.go", Source: "Security"},
		{File: "b.go", Source: "Security"},
		{File: "c.go", Source: "Feasibility"},
	}
	got := LensConvergence(findings)
	want := map[string]int{"a.go": 3, "b.go": 1, "c.go": 1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LensConvergence = %v, want %v", got, want)
	}
	if files := ConvergedFiles(findings, 2); len(files) != 1 || files[0] != "a.go" {
		t.Errorf("ConvergedFiles(min=2) = %v, want [a.go]", files)
	}
	if files := ConvergedFiles(findings, 1); len(files) != 3 || files[0] != "a.go" {
		t.Errorf("ConvergedFiles(min=1) = %v, want a.go first (most agreed)", files)
	}
}

// Findings that cannot be attributed are not evidence of convergence, and counting them would
// inflate it. A finding with no file cannot be tied to one; a finding with no source predates the
// stamping or came from a node with no lenses — mutation-verify stamps its own source, and the
// count must not read that as a reviewer agreeing with a lens.
func TestLensConvergenceIgnoresUnattributableFindings(t *testing.T) {
	findings := []Finding{
		{File: "a.go", Source: "Security"},
		{File: "a.go"},                     // no source: not a second reviewer
		{Source: "Architecture"},           // no file: cannot be attributed
		{File: "a.go", Source: ""},         // explicitly empty
		{File: "a.go", Source: "Security"}, // duplicate of the first
	}
	if got := LensConvergence(findings); !reflect.DeepEqual(got, map[string]int{"a.go": 1}) {
		t.Errorf("LensConvergence = %v, want {a.go: 1}", got)
	}
	if got := ConvergedFiles(findings, 2); len(got) != 0 {
		t.Errorf("nothing converged, got %v", got)
	}
	if got := LensConvergence(nil); len(got) != 0 {
		t.Errorf("no findings is no convergence, got %v", got)
	}
}

// The order must be stable between runs over the same data, or a report that lists it cannot be
// read as a diff: most-agreed first, ties broken by path.
func TestConvergedFilesOrderIsStable(t *testing.T) {
	findings := []Finding{
		{File: "z.go", Source: "A"}, {File: "z.go", Source: "B"},
		{File: "a.go", Source: "A"}, {File: "a.go", Source: "B"},
		{File: "m.go", Source: "A"}, {File: "m.go", Source: "B"}, {File: "m.go", Source: "C"},
	}
	want := []string{"m.go", "a.go", "z.go"}
	for i := 0; i < 5; i++ {
		if got := ConvergedFiles(findings, 2); !reflect.DeepEqual(got, want) {
			t.Fatalf("ConvergedFiles = %v, want %v (stable across calls)", got, want)
		}
	}
}
