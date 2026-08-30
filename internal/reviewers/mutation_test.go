package reviewers

import (
	"strings"
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

// Two engines disagreeing about one file must not silently become one engine's answer.
//
// The uncovered fingerprint was once "mutation:uncovered:<file>" with no engine in it, so the
// dedupe above treated gremlins' and stryker's reports of the same file as the same claim and
// discarded the second — losing its sites and leaving the kept finding's count wrong. Engines
// genuinely disagree here (gremlins and ooze differed by 137 mutants on one package during this
// work), so dropping one is data loss, not deduplication.
func TestTwoEnginesUncoveredInOneFileAreTwoClaims(t *testing.T) {
	site := func(line int) mutation.Mutant {
		// The typed constant, never a literal: this test first used "NoCoverage", which is not a
		// status this package knows, so every mutant fell through to `unresolved` and the test
		// passed without ever reaching the uncovered path it exists to guard.
		return mutation.Mutant{File: "lib/parser.js", Line: line, Status: mutation.Uncovered, Operator: "Cond"}
	}
	got := MutationContext{Reports: []mutation.Report{
		{Engine: "gremlins", Mutants: []mutation.Mutant{site(10)}},
		{Engine: "stryker", Mutants: []mutation.Mutant{site(99)}},
	}}.Findings()
	if len(got) != 2 {
		t.Fatalf("two engines, two claims; got %d: %+v", len(got), got)
	}
	all := got[0].Finding + "\n" + got[1].Finding
	for _, want := range []string{"10", "99"} {
		if !strings.Contains(all, want) {
			t.Errorf("line %s was dropped:\n%s", want, all)
		}
	}
	if got[0].Fingerprint == got[1].Fingerprint {
		t.Errorf("both engines share fingerprint %q, so one will be dropped", got[0].Fingerprint)
	}
	// The same engine reporting the same file twice IS one claim, and still collapses.
	same := MutationContext{Reports: []mutation.Report{
		{Engine: "gremlins", Mutants: []mutation.Mutant{site(10)}},
		{Engine: "gremlins", Mutants: []mutation.Mutant{site(10)}},
	}}.Findings()
	if len(same) != 1 {
		t.Errorf("one engine repeating itself is one finding, got %d", len(same))
	}
}

// The unresolved fingerprint must not depend on where the report file was written.
//
// Load fills Target with the report's file path, so keying on it gave a CI job writing
// /tmp/build-1234/mut.json and a developer's local copy different fingerprints for the same run.
// Fingerprint identity is what makes an `overridden` record suppress rediscovery, so a granted
// override on this — the highest-severity finding the package raises — silently failed to
// rematch, and nothing surfaced the failure.
func TestTheUnresolvedFingerprintSurvivesADifferentPath(t *testing.T) {
	// Anything the package does not recognise is undecided; "timeout" is what gremlins emits.
	undecided := []mutation.Mutant{{File: "a.go", Line: 4, Status: "timeout", Operator: "Cond"}}
	fp := func(target string) string {
		f := (mutation.Report{Engine: "gremlins", Target: target, Mutants: undecided}).Findings()
		if len(f) != 1 {
			t.Fatalf("want one undecided finding, got %d", len(f))
		}
		return f[0].Fingerprint
	}
	ci, local := fp("/tmp/build-1234/mut.json"), fp("/Users/dev/project/mut.json")
	if ci != local {
		t.Errorf("the same run keys differently by path:\n  ci    %s\n  local %s", ci, local)
	}
	// It still distinguishes genuinely different claims: another engine, and other mutants.
	other := (mutation.Report{Engine: "stryker", Mutants: undecided}).Findings()[0].Fingerprint
	if other == ci {
		t.Error("two engines' undecided sets must not share a fingerprint")
	}
	elsewhere := (mutation.Report{Engine: "gremlins", Mutants: []mutation.Mutant{
		{File: "b.go", Line: 9, Status: "timeout"}}}).Findings()[0].Fingerprint
	if elsewhere == ci {
		t.Error("different undecided mutants are different claims")
	}

	// A large failure must not produce an unbounded key. The measured gremlins run left 97
	// mutants undecided from one bad timeout, so this is the ordinary case, not the edge: the
	// key names the first few sites and then the count, and stays a fingerprint rather than
	// becoming a transcript.
	many := func(n int, file string) string {
		ms := make([]mutation.Mutant, n)
		for i := range ms {
			ms[i] = mutation.Mutant{File: file, Line: i + 1, Status: "timeout"}
		}
		return (mutation.Report{Engine: "gremlins", Mutants: ms}).Findings()[0].Fingerprint
	}
	big := many(97, "a.go")
	if len(big) > 200 {
		t.Errorf("the key grows with the failure (%d chars): %s", len(big), big)
	}
	if !strings.Contains(big, "+89") {
		t.Errorf("a truncated key must still carry how many it dropped: %s", big)
	}
	// Truncation must not erase the difference between two different large failures.
	if big == many(96, "a.go") {
		t.Error("97 undecided and 96 undecided must not share a fingerprint")
	}
	if big == many(97, "b.go") {
		t.Error("the same count in a different file must not share a fingerprint")
	}
	// Order of arrival is not part of the claim.
	shuffled := (mutation.Report{Engine: "gremlins", Mutants: []mutation.Mutant{
		{File: "z.go", Line: 2, Status: "timeout"}, {File: "a.go", Line: 1, Status: "timeout"}}}).Findings()[0].Fingerprint
	reversed := (mutation.Report{Engine: "gremlins", Mutants: []mutation.Mutant{
		{File: "a.go", Line: 1, Status: "timeout"}, {File: "z.go", Line: 2, Status: "timeout"}}}).Findings()[0].Fingerprint
	if shuffled != reversed {
		t.Errorf("emission order changed the fingerprint:\n  %s\n  %s", shuffled, reversed)
	}
}
