package mutation

import (
	"strings"
	"testing"
)

// The spike that motivated this package: gremlins on internal/findings reported
// "Killed 10, Lived 0, Test efficacy: 100.00%" and exited 0, while ten mutants had in fact
// survived and 97 were unresolved. Its JSON summary has fields for killed, lived, not-viable and
// not-covered and NONE for timed-out, and efficacy is killed/(killed+lived) — so the unresolved
// mutants vanish from both the numerator and the denominator, and a worse-configured run scores
// higher. We therefore never read an engine's summary or exit code; totals are computed here.
func TestScoreNeverCountsAnUnresolvedMutantAsAKill(t *testing.T) {
	// The exact shape of that misleading run: 10 killed, 0 survived, 97 unresolved.
	r := Report{Engine: "gremlins", Mutants: mutants(10, 0, 97, 0)}
	s := r.Score()
	if s.Unresolved != 97 {
		t.Fatalf("Unresolved = %d, want 97", s.Unresolved)
	}
	if s.Complete() {
		t.Error("a run with unresolved mutants is not complete, whatever its efficacy looks like")
	}
	// The engine would call this 100%. It is not: 97 of 107 were never decided.
	if s.Efficacy > 0.10 {
		t.Errorf("efficacy = %.4f; unresolved mutants must count against the score, not vanish from it", s.Efficacy)
	}
}

// The same package once the timeout is calibrated: nothing unresolved, and the number is the
// honest one the engine reports only when configured correctly.
func TestScoreOnACompleteRun(t *testing.T) {
	r := Report{Engine: "gremlins", Mutants: mutants(99, 8, 0, 0)}
	s := r.Score()
	if !s.Complete() {
		t.Fatal("no unresolved mutants means the run is complete")
	}
	if diff := s.Efficacy - 0.9252; diff > 0.0001 || diff < -0.0001 {
		t.Errorf("efficacy = %.4f, want 0.9252", s.Efficacy)
	}
}

// An uncovered site is not a survivor and not a kill: no test executes that code at all. It is
// the cheapest and most damning class — found without running a mutation — and it must be
// reported rather than folded into either side of the efficacy fraction.
func TestUncoveredSitesAreTheirOwnClass(t *testing.T) {
	r := Report{Engine: "gremlins", Mutants: mutants(5, 0, 0, 3)}
	s := r.Score()
	if s.Uncovered != 3 {
		t.Fatalf("Uncovered = %d, want 3", s.Uncovered)
	}
	if !s.Complete() {
		t.Error("uncovered sites do not make a run incomplete; they are a decided outcome")
	}
	if s.Efficacy != 1.0 {
		t.Errorf("efficacy = %.4f; uncovered sites belong in their own count, not the efficacy fraction", s.Efficacy)
	}
}

// Findings are what the gate consumes, so every actionable outcome must become one, and each must
// name a place a human can go.
func TestFindingsNameSurvivorsAndUncoveredSites(t *testing.T) {
	r := Report{Engine: "gremlins", Mutants: []Mutant{
		{Status: Killed, File: "a.go", Line: 1, Operator: "CONDITIONALS_NEGATION"},
		{Status: Survived, File: "bind.go", Line: 53, Operator: "INVERT_NEGATIVES"},
		{Status: Uncovered, File: "b.go", Line: 9, Operator: "ARITHMETIC_BASE"},
		{Status: Unresolved, File: "c.go", Line: 4, Operator: "CONDITIONALS_BOUNDARY"},
	}}
	got := r.Findings()
	if len(got) != 3 {
		t.Fatalf("got %d findings, want 3 (survivor, uncovered, unresolved); killed is not a finding", len(got))
	}
	var joined string
	for _, f := range got {
		joined += f.Title + "|" + f.Finding + "|"
		if f.Evidence == nil || f.Evidence[0].Path == "" {
			t.Errorf("finding %q names no file", f.Title)
		}
	}
	for _, want := range []string{"bind.go", "b.go", "c.go", "INVERT_NEGATIVES"} {
		if !strings.Contains(joined, want) {
			t.Errorf("findings do not mention %q:\n%s", want, joined)
		}
	}
}

// mutants builds a report body with the given counts, so a test can state the shape it cares
// about without listing every site.
func mutants(killed, survived, unresolved, uncovered int) []Mutant {
	var out []Mutant
	add := func(n int, s Status, file string) {
		for i := 0; i < n; i++ {
			out = append(out, Mutant{Status: s, File: file, Line: i + 1, Operator: "CONDITIONALS_NEGATION"})
		}
	}
	add(killed, Killed, "k.go")
	add(survived, Survived, "s.go")
	add(unresolved, Unresolved, "u.go")
	add(uncovered, Uncovered, "n.go")
	return out
}

// Parsed from the real report gremlins wrote during the 2026-08-29 spike, including the run whose
// summary claimed 100% efficacy. The mapping that matters is TIMED OUT -> Unresolved: gremlins
// leaves that class out of its summary, so a caller trusting the summary sees a run in which
// those mutants never existed.
func TestParseGremlinsMapsEveryStatusAndNeverInventsAKill(t *testing.T) {
	raw := []byte(`{"go_module":"github.com/dsifry/metareview","files":[
		{"file_name":"bind.go","mutations":[
			{"type":"INVERT_NEGATIVES","status":"TIMED OUT","line":53,"column":48},
			{"type":"ARITHMETIC_BASE","status":"KILLED","line":87,"column":42}]},
		{"file_name":"override.go","mutations":[
			{"type":"CONDITIONALS_BOUNDARY","status":"LIVED","line":62,"column":44},
			{"type":"CONDITIONALS_NEGATION","status":"NOT COVERED","line":9,"column":2},
			{"type":"ARITHMETIC_BASE","status":"NOT VIABLE","line":11,"column":2},
			{"type":"CONDITIONALS_NEGATION","status":"SOME_FUTURE_STATUS","line":13,"column":2}]}]}`)
	r, err := ParseGremlins(raw, "./internal/findings")
	if err != nil {
		t.Fatalf("ParseGremlins: %v", err)
	}
	if len(r.Mutants) != 6 {
		t.Fatalf("got %d mutants, want 6", len(r.Mutants))
	}
	s := r.Score()
	if s.Killed != 1 || s.Survived != 1 || s.Uncovered != 1 || s.Unresolved != 3 {
		t.Errorf("score = %+v; want killed 1, survived 1, uncovered 1, unresolved 3 (timed-out, not-viable and the unknown status)", s)
	}
	if s.Complete() {
		t.Error("a report containing a timed-out mutant is not a complete run")
	}
	// File names must survive: a finding that cannot be located is not actionable.
	var files []string
	for _, m := range r.Mutants {
		files = append(files, m.File)
	}
	if !strings.Contains(strings.Join(files, ","), "bind.go") {
		t.Errorf("file names lost: %v", files)
	}
}

// Malformed engine output is an error, never an empty report. An empty report scores as a
// complete run with nothing wrong in it, which is the most dangerous possible way to fail.
func TestUnparseableEngineOutputIsAnError(t *testing.T) {
	if _, err := ParseGremlins([]byte("not json at all"), "./x"); err == nil {
		t.Fatal("unparseable output must surface as an error, not as an empty passing report")
	}
	// And the empty-but-valid case is complete with nothing to report, which is honest: the
	// engine ran and found no mutation sites.
	r, err := ParseGremlins([]byte(`{"go_module":"m","files":[]}`), "./x")
	if err != nil {
		t.Fatalf("valid empty report: %v", err)
	}
	if s := r.Score(); !s.Complete() || len(r.Findings()) != 0 {
		t.Errorf("empty report: score=%+v findings=%d", s, len(r.Findings()))
	}
}
