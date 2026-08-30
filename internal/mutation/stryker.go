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
		Mutants []struct {
			MutatorName string `json:"mutatorName"`
			Status      string `json:"status"`
			Location    struct {
				Start struct {
					Line   int `json:"line"`
					Column int `json:"column"`
				} `json:"start"`
			} `json:"location"`
			Description  string `json:"description,omitempty"`
			StatusReason string `json:"statusReason,omitempty"`
		} `json:"mutants"`
	} `json:"files"`
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
