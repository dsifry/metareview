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
	// The engine would call this 100%. It is not: 97 of 107 were never decided, so the honest
	// number is 10/107. Asserted exactly, not as an upper bound: mutation testing this file found
	// that "efficacy < 0.10" also passes when the denominator is computed as
	// killed+survived-unresolved, which yields a NEGATIVE efficacy. A one-sided assertion on a
	// number cannot constrain the arithmetic that produced it.
	if diff := s.Efficacy - 10.0/107.0; diff > 0.0001 || diff < -0.0001 {
		t.Errorf("efficacy = %.4f, want %.4f (10 killed of 107 attempted)", s.Efficacy, 10.0/107.0)
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
	s := r.Score()
	if !s.Complete() || len(r.Findings()) != 0 {
		t.Errorf("empty report: score=%+v findings=%d", s, len(r.Findings()))
	}
	// Zero mutants must give zero efficacy, not NaN. Found by mutating `denom > 0` to `denom >= 0`,
	// which divides zero by zero: NaN compares false against every threshold, so a gate reading it
	// would silently pass anything.
	if s.Efficacy != 0 {
		t.Errorf("empty report efficacy = %v, want 0 (a NaN here passes every threshold silently)", s.Efficacy)
	}
}

// The shape of the output decides whether a human can survive this gate, so it is pinned here.
// The measured gremlins run on 2026-08-29 left 97 mutants undecided from ONE bad timeout; a
// finding apiece would have buried ten real survivors under 97 lines of noise, and a gate that
// does that gets switched off. Undecided mutants are one fact about the run, uncovered sites
// group per file, and only survivors are per-site — because only a survivor names a specific
// missing test.
func TestFindingVolumeSurvivesARealMisconfiguredRun(t *testing.T) {
	r := Report{Engine: "gremlins", Target: "./internal/findings", Mutants: mutants(10, 0, 97, 0)}
	got := r.Findings()
	if len(got) != 1 {
		t.Fatalf("97 undecided mutants are one fact about the run, not %d findings", len(got))
	}
	f := got[0]
	if f.Severity != "high" || f.Classification != "blocking" {
		t.Errorf("a run that decided nothing must block: %+v", f)
	}
	// The code is not what is wrong here — the engine's configuration is — and sending an
	// implementer to fix 97 "defects" caused by a timeout wastes the whole iteration.
	if f.Owner != "reviewer" {
		t.Errorf("an undecided mutant is a configuration fact, not an implementer's defect: %q", f.Owner)
	}
	if !strings.Contains(f.Finding, "97 mutant(s) undecided") {
		t.Errorf("the true count must survive the summarising: %q", f.Finding)
	}
	if strings.Count(f.Finding, "u.go:") > 9 {
		t.Errorf("the example list must be capped so the finding stays readable: %q", f.Finding)
	}
}

func TestUncoveredSitesGroupPerFileAndDoNotBlock(t *testing.T) {
	r := Report{Engine: "gremlins", Mutants: []Mutant{
		{Status: Uncovered, File: "b.go", Line: 9, Operator: "ARITHMETIC_BASE"},
		{Status: Uncovered, File: "a.go", Line: 2, Operator: "INVERT_NEGATIVES"},
		{Status: Uncovered, File: "b.go", Line: 40, Operator: "CONDITIONALS_BOUNDARY"},
	}}
	got := r.Findings()
	if len(got) != 2 {
		t.Fatalf("three sites in two files are two findings, got %d", len(got))
	}
	for _, f := range got {
		// Advisory on purpose. An uncovered line is worth knowing and worth fixing, but blocking
		// every one of them makes the gate unsurvivable on any under-tested package, and an
		// unsurvivable gate is switched off — which costs more than it ever caught.
		if f.Classification != "advisory" || f.Severity != "low" {
			t.Errorf("uncovered sites are advisory, not blockers: %+v", f)
		}
	}
	// Grouping must not lose the lines, or the finding is unactionable.
	var b string
	for _, f := range got {
		if strings.Contains(f.Title, "b.go") {
			b = f.Finding
		}
	}
	if !strings.Contains(b, "line 9") || !strings.Contains(b, "line 40") || !strings.Contains(b, "2 site(s)") {
		t.Errorf("the group must name every line it covers: %q", b)
	}
}

// A survivor is the finding this whole node exists to raise, so it stays one per site, blocking,
// and owned by the implementer: it names a line and the test that must exist.
func TestSurvivorsStayPerSiteAndBlock(t *testing.T) {
	r := Report{Engine: "gremlins", Mutants: mutants(0, 3, 0, 0)}
	got := r.Findings()
	if len(got) != 3 {
		t.Fatalf("survivors are per-site, got %d", len(got))
	}
	seen := map[string]bool{}
	for _, f := range got {
		if f.Classification != "blocking" || f.Owner != "implementer" {
			t.Errorf("a survivor is a defect in the tests and must block: %+v", f)
		}
		if seen[f.Fingerprint] {
			t.Errorf("two survivors share a fingerprint, so one would silently vanish: %q", f.Fingerprint)
		}
		seen[f.Fingerprint] = true
	}
}

// A clean run raises nothing at all. Repositories that run no engine, and packages that are
// fully killed, must pass exactly as before.
func TestACleanRunRaisesNothing(t *testing.T) {
	if got := (Report{Engine: "gremlins", Mutants: mutants(12, 0, 0, 0)}).Findings(); len(got) != 0 {
		t.Errorf("a fully-killed run is not a finding: %+v", got)
	}
	if got := (Report{}).Findings(); len(got) != 0 {
		t.Errorf("an empty report is not a finding: %+v", got)
	}
}

// An engine-less report can still be translated; it must not produce a reviewer named
// "mutation-" or a sentence with a hole in it.
func TestReportWithoutAnEngineNameStaysReadable(t *testing.T) {
	got := (Report{Mutants: mutants(0, 0, 1, 0)}).Findings()
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1", len(got))
	}
	if got[0].Reviewer != "mutation-reviewer" || strings.Contains(got[0].Finding, " left") == false {
		t.Errorf("unnamed engine produced a malformed finding: %+v", got[0])
	}
	if strings.HasPrefix(got[0].Finding, " ") {
		t.Errorf("the sentence begins with a hole where the engine name should be: %q", got[0].Finding)
	}
}

// The honest score has to REACH the operator, or computing it is a private virtue. An engine
// reporting "Test efficacy: 100.00%" while 97 mutants timed out is exactly the number this
// contradicts, and a reader comparing the two needs both in front of them. Before this, Score and
// Complete had no non-test caller anywhere in the module — the package's headline claim was
// delivered entirely by Findings(), and the coverage floor certified an unreachable type kept
// alive by its own unit tests.
func TestTheUndecidedFindingCarriesTheHonestScore(t *testing.T) {
	// The measured 2026-08-29 gremlins run: 10 killed, 97 timed out, and it reported 100%.
	got := (Report{Engine: "gremlins", Mutants: mutants(10, 0, 97, 0)}).Findings()
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1", len(got))
	}
	text := got[0].Finding
	for _, want := range []string{"10 killed", "97 undecided", "efficacy 9.3%", "INCOMPLETE"} {
		if !strings.Contains(text, want) {
			t.Errorf("the finding must carry %q:\n%s", want, text)
		}
	}
	// The number an engine would have printed must not be reproducible from ours.
	if strings.Contains(text, "100.0%") {
		t.Errorf("efficacy must count undecided against the score:\n%s", text)
	}
	// A complete run says so by omission rather than claiming incompleteness.
	done := (Report{Engine: "gremlins", Mutants: mutants(8, 2, 0, 0)}).Findings()
	if len(done) != 2 {
		t.Fatalf("two survivors are two findings, got %d", len(done))
	}
	if s := (Report{Mutants: mutants(8, 2, 0, 0)}).Score(); !s.Complete() || s.Efficacy != 0.8 {
		t.Errorf("a decided run: complete=%v efficacy=%v", s.Complete(), s.Efficacy)
	}
	if strings.Contains((Score{Killed: 8, Survived: 2}).Summary(), "INCOMPLETE") {
		t.Error("a complete run must not be labelled incomplete")
	}
}
