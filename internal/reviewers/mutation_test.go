package reviewers

import (
	"testing"

	"github.com/dsifry/metareview/internal/mutation"
)

// The ordinary repository runs no mutation engine, and must review exactly as it did before.
// This gate is opt-in, and it never raises "you should be running mutation testing" — a gate that
// scolds you for not opting in is a gate people opt out of.
func TestNoMutationReportsChangeNothing(t *testing.T) {
	if got := (MutationContext{}).Findings(); len(got) != 0 {
		t.Errorf("an absent engine raises nothing: %+v", got)
	}
	plain := RunTaskDone(Context{Git: GitContext{ChangedFiles: []string{"a.go"}}})
	withEmpty := RunTaskDone(Context{Git: GitContext{ChangedFiles: []string{"a.go"}}, Mutation: MutationContext{}})
	if len(plain) != len(withEmpty) {
		t.Errorf("an empty mutation context changed the review: %d vs %d", len(plain), len(withEmpty))
	}
}

// Two engines over the same package is a thing the roadmap wants — gremlins and ooze disagreed by
// 137 mutants on one package, so their union finds more than either alone — and it must not turn
// the site they AGREE on into two findings.
func TestTwoEnginesDoNotDoubleReportTheSameSite(t *testing.T) {
	agreed := mutation.Mutant{Status: mutation.Survived, File: "bind.go", Line: 53, Operator: "INVERT_NEGATIVES"}
	onlyOoze := mutation.Mutant{Status: mutation.Survived, File: "bind.go", Line: 77, Operator: "SWAP_BRANCH"}
	ctx := MutationContext{Reports: []mutation.Report{
		{Engine: "ooze", Target: "./internal/findings", Mutants: []mutation.Mutant{agreed, onlyOoze}},
		{Engine: "gremlins", Target: "./internal/findings", Mutants: []mutation.Mutant{agreed}},
	}}
	got := ctx.Findings()
	if len(got) != 2 {
		t.Fatalf("the agreed site is one finding and the extra one is another, got %d: %+v", len(got), got)
	}
	// Reports are sorted before translation, so the same inputs always produce the same list.
	// A review log that reshuffles between runs cannot be read as a diff.
	if got[0].Reviewer != "mutation-gremlins" {
		t.Errorf("engines must be ordered deterministically, got %q first", got[0].Reviewer)
	}
	if got[0].Fingerprint == got[1].Fingerprint {
		t.Error("two distinct sites collapsed into one fingerprint")
	}
}

// Mutation findings are ordinary findings: they join the same ledger the deterministic lints use,
// so one run chain, one set of fingerprints and one override mechanism cover both.
func TestMutationFindingsJoinEverySurface(t *testing.T) {
	ctx := MutationContext{Reports: []mutation.Report{{
		Engine:  "gremlins",
		Mutants: []mutation.Mutant{{Status: mutation.Survived, File: "a.go", Line: 3, Operator: "INVERT_NEGATIVES"}},
	}}}
	surfaces := map[string][]Finding{
		"task-done":  RunTaskDone(Context{Mutation: ctx}),
		"pr-ready":   RunPRReady(PRReadyContext{Mutation: ctx}),
		"epic-ready": RunEpicReady(EpicReadyContext{Mutation: ctx}),
	}
	for name, got := range surfaces {
		var found bool
		for _, f := range got {
			if f.Reviewer == "mutation-gremlins" {
				found = true
			}
		}
		if !found {
			t.Errorf("%s did not surface the surviving mutant: %+v", name, got)
		}
	}
}
