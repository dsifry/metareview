package mutation

import (
	"testing"
)

// A report in the cross-language schema must be readable without being told which engine wrote
// it, and its "could not decide" classes must not score as kills. The schema separates those
// classes explicitly where gremlins does not, so this is the format the node prefers.
func TestParseStrykerReadsTheCrossLanguageSchema(t *testing.T) {
	raw := []byte(`{
	  "schemaVersion": "1.0",
	  "files": {
	    "src/b.ts": {"mutants": [
	      {"mutatorName": "EqualityOperator", "status": "Killed",  "location": {"start": {"line": 3, "column": 5}}},
	      {"mutatorName": "BooleanLiteral",   "status": "Survived","location": {"start": {"line": 7}}, "description": "true -> false"}
	    ]},
	    "src/a.ts": {"mutants": [
	      {"mutatorName": "ArithmeticOperator", "status": "Timeout",      "location": {"start": {"line": 1}}},
	      {"mutatorName": "StringLiteral",      "status": "NoCoverage",   "location": {"start": {"line": 2}}},
	      {"mutatorName": "LogicalOperator",    "status": "CompileError", "location": {"start": {"line": 4}}, "statusReason": "type error"},
	      {"mutatorName": "ConditionalExpression", "status": "Ignored",   "location": {"start": {"line": 6}}},
	      {"mutatorName": "FromTheFuture",      "status": "Reticulated",  "location": {"start": {"line": 8}}}
	    ]}
	  }
	}`)
	r, err := Parse(raw, "web")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if r.Engine != "stryker" {
		t.Errorf("the schema must be detected without a flag, got engine %q", r.Engine)
	}
	// Files come out of a map, which has no order. A review log that reshuffles its findings on
	// every run cannot be read as a diff, so the order must be the sorted one.
	if r.Mutants[0].File != "src/a.ts" || r.Mutants[len(r.Mutants)-1].File != "src/b.ts" {
		t.Errorf("files must be emitted in sorted order: %+v", r.Mutants)
	}
	s := r.Score()
	// Timeout, CompileError, Ignored and the unrecognised status are all undecided: four.
	if s.Killed != 1 || s.Survived != 1 || s.Uncovered != 1 || s.Unresolved != 4 {
		t.Errorf("wrong classes: %+v", s)
	}
	if s.Complete() {
		t.Error("a run with undecided mutants is not a complete measurement")
	}
	if s.Efficacy != 1.0/6.0 {
		t.Errorf("undecided mutants must count against efficacy, got %v", s.Efficacy)
	}
	if got := r.Mutants[2].Detail; got != "type error" {
		t.Errorf("statusReason must be carried through, got %q", got)
	}
}

// Ignored is the one that could reasonably have gone the other way. A site excluded by
// configuration was not measured, so calling it decided would let a run that skipped half its
// targets report as complete — the same failure as gremlins' missing timeout field.
func TestStrykerIgnoredIsNotDecided(t *testing.T) {
	if got := strykerStatus("Ignored"); got != Unresolved {
		t.Errorf("an excluded site measured nothing, got %q", got)
	}
	for status, want := range map[string]Status{
		"Killed": Killed, "killed": Killed, " Survived ": Survived,
		"NoCoverage": Uncovered, "no_coverage": Uncovered,
		"Timeout": Unresolved, "RuntimeError": Unresolved, "": Unresolved,
	} {
		if got := strykerStatus(status); got != want {
			t.Errorf("strykerStatus(%q) = %q, want %q", status, got, want)
		}
	}
}

// Detection is on structure, so neither format can be read as the other, and something that is
// neither must be refused rather than becoming an empty report — an empty report scores as a
// clean run, which is the worst possible reading of a file we failed to understand.
func TestParseDetectsFormatAndRefusesWhatItCannotRead(t *testing.T) {
	gremlins, err := Parse([]byte(`{"go_module":"m","files":[{"file_name":"a.go","mutations":[{"type":"T","status":"LIVED","line":1}]}]}`), "./x")
	if err != nil {
		t.Fatalf("gremlins JSON must still be readable: %v", err)
	}
	if gremlins.Engine != "gremlins" || len(gremlins.Mutants) != 1 {
		t.Errorf("wrong parser chosen: %+v", gremlins)
	}
	for name, data := range map[string]string{
		"no files key":  `{"schemaVersion":"1.0"}`,
		"files is null": `{"files":null}`,
		"files is text": `{"files":"none"}`,
		"not json":      `this is a test log, not a report`,
	} {
		if _, err := Parse([]byte(data), "report.json"); err == nil {
			t.Errorf("%s: must be refused, not read as an empty (clean) run", name)
		}
	}
}
