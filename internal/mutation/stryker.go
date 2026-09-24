package mutation

import (
	"encoding/json"
	"fmt"
	"strings"
)

// strykerReport is the subset of mutation-testing-report-schema this package reads.
//
// The schema is Stryker's cross-language JSON standard, already emitted by StrykerJS,
// Stryker.NET and Stryker4s, and it is metareview's input contract on purpose: a repository in
// any language can participate by producing one file, and nothing here is ours to version.
// Engines that do not speak it — gremlins for Go, PIT for Java, mutmut for Python — get a shim
// like ParseGremlins instead.
//
// As with gremlins, no summary field is read even where one exists. Totals are computed from the
// per-mutant list, for every engine.
type strykerReport struct {
	SchemaVersion string `json:"schemaVersion"`
	Files         map[string]struct {
		Source  string `json:"source"`
		Mutants []struct {
			ID          string   `json:"id"`
			MutatorName string   `json:"mutatorName"`
			Status      string   `json:"status"`
			KilledBy    []string `json:"killedBy"`
			CoveredBy   []string `json:"coveredBy"`
			Location    struct {
				Start struct {
					Line   int `json:"line"`
					Column int `json:"column"`
				} `json:"start"`
				End struct {
					Line int `json:"line"`
				} `json:"end"`
			} `json:"location"`
			Description  string `json:"description,omitempty"`
			StatusReason string `json:"statusReason,omitempty"`
		} `json:"mutants"`
	} `json:"files"`
	TestFiles map[string]struct {
		Tests []struct {
			ID string `json:"id"`
		} `json:"tests"`
	} `json:"testFiles"`
}

// StrykerDetail is what the freshness gate reads from a mutation-testing-report-schema report
// (spec §6.1). Ids are meaningful only within one report.
type StrykerDetail struct {
	Files   map[string]StrykerFile
	TestIDs map[string][]string // test file → its test ids
}

// StrykerFile is one mutated file: the source Stryker read and its mutants.
type StrykerFile struct {
	Source  string
	Mutants []StrykerMutant
}

// StrykerMutant carries the ids and span freshness needs.
type StrykerMutant struct {
	ID        string
	Status    Status
	KilledBy  []string
	CoveredBy []string
	StartLine int
	EndLine   int
}

// ParseStryker normalises a mutation-testing-report-schema report.
func ParseStryker(data []byte, target string) (Report, error) {
	var raw strykerReport
	if err := json.Unmarshal(data, &raw); err != nil {
		return Report{}, fmt.Errorf("mutation: parsing mutation-testing-report-schema report: %w", err)
	}
	r := Report{Engine: "stryker", Target: target}
	// A map has no order, and a review log that reshuffles its findings on every run is unusable
	// as a diff. Emit files in sorted order so the same report always produces the same list.
	for _, path := range sortedKeys(raw.Files) {
		for _, m := range raw.Files[path].Mutants {
			r.Mutants = append(r.Mutants, Mutant{
				Status:   strykerStatus(m.Status),
				File:     path,
				Line:     m.Location.Start.Line,
				Column:   m.Location.Start.Column,
				Operator: m.MutatorName,
				Detail:   firstNonEmpty(m.StatusReason, m.Description),
			})
		}
	}
	r.Detail = &StrykerDetail{Files: map[string]StrykerFile{}, TestIDs: map[string][]string{}}
	for path, f := range raw.Files {
		file := StrykerFile{Source: f.Source}
		for _, m := range f.Mutants {
			file.Mutants = append(file.Mutants, StrykerMutant{
				ID: m.ID, Status: strykerStatus(m.Status), KilledBy: m.KilledBy, CoveredBy: m.CoveredBy,
				StartLine: m.Location.Start.Line, EndLine: m.Location.End.Line,
			})
		}
		r.Detail.Files[path] = file
	}
	for path, tf := range raw.TestFiles {
		for _, test := range tf.Tests {
			r.Detail.TestIDs[path] = append(r.Detail.TestIDs[path], test.ID)
		}
	}
	return r, nil
}

// strykerStatus maps the schema's vocabulary onto ours.
//
// The schema's own classes already separate "decided" from "could not decide", which is the
// distinction gremlins loses: Timeout, CompileError and RuntimeError are each explicitly NOT a
// kill. NoCoverage is its own class, exactly as Uncovered is here. Ignored is deliberate
// exclusion by configuration — nothing was measured and nothing is claimed, so it is unresolved
// rather than silently dropped: a run that skipped half its sites must not look complete.
func strykerStatus(s string) Status {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "killed":
		return Killed
	case "survived":
		return Survived
	case "nocoverage", "no_coverage":
		return Uncovered
	default:
		// Timeout, CompileError, RuntimeError, Ignored, Pending, and anything a later version of
		// the schema adds. Defaulting to Unresolved is the same rule as everywhere else here: an
		// outcome this code does not recognise must never be scored as a success.
		return Unresolved
	}
}
