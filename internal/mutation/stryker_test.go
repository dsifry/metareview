package mutation

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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

// A report that parses but describes NO mutants measured nothing, and must not be readable as a
// clean run. Zero mutants scores as efficacy 0/0 with Complete() true and no findings — byte for
// byte what a package whose every mutant was killed produces. Parse already refuses a file it
// cannot READ for the same reason; this is the file it can read that says nothing.
func TestAReportWithNoMutantsIsRefused(t *testing.T) {
	for name, body := range map[string]string{
		"stryker with no files":    `{"schemaVersion":"1.0","files":{}}`,
		"stryker with empty file":  `{"schemaVersion":"1.0","files":{"a.ts":{"mutants":[]}}}`,
		"gremlins with no files":   `{"go_module":"m","files":[]}`,
		"gremlins with empty file": `{"go_module":"m","files":[{"file_name":"a.go","mutations":[]}]}`,
	} {
		if _, err := Parse([]byte(body), "report.json"); err == nil {
			t.Errorf("%s: a report that measured nothing must be refused, not read as clean", name)
		} else if !errors.Is(err, errNoMutants) {
			t.Errorf("%s: wrong error: %v", name, err)
		}
	}
	// And a report that DID measure something still parses.
	ok := `{"schemaVersion":"1.0","files":{"a.ts":{"mutants":[{"mutatorName":"X","status":"Killed","location":{"start":{"line":1}}}]}}}`
	if _, err := Parse([]byte(ok), "report.json"); err != nil {
		t.Errorf("a report with mutants must parse: %v", err)
	}
}

// Load and LoadAll are the operator-facing entry points, and their failure behaviour is the
// safety property: a report that cannot be read must stop the review, never be skipped. A skipped
// report is indistinguishable from a package with no survivors, so a typo in a path would read
// exactly like a clean run.
func TestLoadAndLoadAllRefuseRatherThanSkip(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.json")
	body := `{"schemaVersion":"1.0","files":{"a.ts":{"mutants":[{"mutatorName":"X","status":"Survived","location":{"start":{"line":3}}}]}}}`
	if err := os.WriteFile(good, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("this is a test log, not a report"), 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := Load(good)
	if err != nil || len(r.Mutants) != 1 {
		t.Fatalf("a readable report must load: %v %+v", err, r)
	}
	if _, err := Load(filepath.Join(dir, "absent.json")); err == nil {
		t.Error("a missing file must be an error")
	}
	if _, err := Load(bad); err == nil {
		t.Error("an unreadable report must be an error, not an empty report")
	}

	all, err := LoadAll([]string{good, good})
	if err != nil || len(all) != 2 {
		t.Fatalf("LoadAll: %v %d", err, len(all))
	}
	// It stops at the first unreadable one rather than reviewing with a partial set.
	if got, err := LoadAll([]string{good, bad, good}); err == nil {
		t.Errorf("LoadAll must refuse a partial set, got %d reports", len(got))
	}
	if got, err := LoadAll(nil); err != nil || len(got) != 0 {
		t.Errorf("no paths is no reports, not an error: %v %v", got, err)
	}
}

// The uncovered edges of the format probe and the finding renderer.
func TestParseEdgesAndFindingCaps(t *testing.T) {
	// Leading whitespace before the files value must not confuse the structural probe.
	ws := "{\"schemaVersion\":\"1.0\",\"files\":   {\"a.ts\":{\"mutants\":[{\"mutatorName\":\"X\",\"status\":\"Killed\",\"location\":{\"start\":{\"line\":1}}}]}}}"
	if _, err := Parse([]byte(ws), ""); err != nil {
		t.Errorf("whitespace before the files object must parse: %v", err)
	}
	// An empty target still produces a readable message rather than a sentence with a hole.
	_, err := Parse([]byte(`{"files":"none"}`), "")
	if err == nil || !strings.Contains(err.Error(), "the report") {
		t.Errorf("an unnamed report must still name itself in the error: %v", err)
	}
	// lineList caps its examples so one bad file cannot produce an unreadable finding, while the
	// count in the sentence stays true.
	var many []Mutant
	for i := 0; i < 30; i++ {
		many = append(many, Mutant{Status: Uncovered, File: "wide.go", Line: i + 1, Operator: "OP"})
	}
	got := (Report{Engine: "gremlins", Mutants: many}).Findings()
	if len(got) != 1 {
		t.Fatalf("30 uncovered sites in one file are one finding, got %d", len(got))
	}
	if !strings.Contains(got[0].Finding, "30 site(s)") {
		t.Errorf("the true count must survive the capping: %q", got[0].Finding)
	}
	if !strings.Contains(got[0].Finding, "and 18 more") {
		t.Errorf("the example list must be capped and say how many it dropped: %q", got[0].Finding)
	}
}
