// Package mutation turns a mutation-testing engine's output into findings the gate can act on.
//
// It deliberately does not trust an engine's own score or exit code. Measured on 2026-08-29,
// gremlins reported "Killed 10, Lived 0, Test efficacy: 100.00%" and exited 0 for a package in
// which ten mutants had survived and ninety-seven were unresolved: its JSON summary has no field
// for the unresolved class, and its efficacy is killed/(killed+lived), so unresolved mutants
// leave both the numerator and the denominator. The worse the configuration, the better the
// score. Totals are therefore computed here, from the per-mutant list, for every engine.
package mutation

import (
	"fmt"

	"github.com/dsifry/metareview/internal/findings"
)

// Status is the outcome of one mutant, normalised across engines.
type Status string

const (
	// Killed: a test failed, so the mutation was detected. The only good outcome.
	Killed Status = "killed"
	// Survived: every test passed with the code deliberately broken. A real gap.
	Survived Status = "survived"
	// Uncovered: no test executes this code, so no mutation was run. Cheapest to find and the
	// shape of most "guards that cannot fail"; a decided outcome, not an unknown.
	Uncovered Status = "uncovered"
	// Unresolved: the engine could not decide — timed out, errored, was killed mid-flight. NOT a
	// kill. A run containing any of these has not measured what it claims to have measured.
	Unresolved Status = "unresolved"
)

// Mutant is one mutation site and what became of it.
type Mutant struct {
	Status   Status `json:"status"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Column   int    `json:"column,omitempty"`
	Operator string `json:"operator"`
	// Detail carries whatever the engine said about this mutant, e.g. a diff of the change.
	Detail string `json:"detail,omitempty"`
}

// Report is one engine's run over one target.
type Report struct {
	Engine  string   `json:"engine"`
	Target  string   `json:"target,omitempty"`
	Mutants []Mutant `json:"mutants"`
}

// Score is the honest summary: every class counted, nothing folded away.
type Score struct {
	Killed     int
	Survived   int
	Uncovered  int
	Unresolved int
	// Efficacy is killed / (killed + survived + unresolved). Unresolved mutants count AGAINST the
	// score rather than vanishing from it, so an under-configured run cannot outscore a careful
	// one. Uncovered sites are excluded: no mutation ran, so there is nothing for a test to catch,
	// and they are reported in their own right instead.
	Efficacy float64
}

// Complete reports whether the run decided every mutant it attempted. A run that did not is not a
// measurement, and no threshold should be applied to it.
func (s Score) Complete() bool { return s.Unresolved == 0 }

// Score counts the report. It reads only the per-mutant list.
func (r Report) Score() Score {
	var s Score
	for _, m := range r.Mutants {
		switch m.Status {
		case Killed:
			s.Killed++
		case Survived:
			s.Survived++
		case Uncovered:
			s.Uncovered++
		default:
			s.Unresolved++
		}
	}
	if denom := s.Killed + s.Survived + s.Unresolved; denom > 0 {
		s.Efficacy = float64(s.Killed) / float64(denom)
	}
	return s
}

// Findings turns every actionable outcome into a finding. A killed mutant is not actionable and
// produces nothing; the other three each name a file and line a human can go to.
func (r Report) Findings() []findings.Input {
	out := make([]findings.Input, 0, len(r.Mutants))
	for _, m := range r.Mutants {
		var title, what, expected, recommend string
		severity := "medium"
		switch m.Status {
		case Killed:
			continue
		case Survived:
			title = "Surviving mutant: " + m.Operator
			what = fmt.Sprintf("%s:%d was mutated (%s) and every test still passed.", m.File, m.Line, m.Operator)
			expected = "A test fails when this code is deliberately broken."
			recommend = "Add or strengthen a test that fails under this mutation, or delete the code if nothing depends on it."
		case Uncovered:
			title = "Uncovered mutation site: " + m.Operator
			what = fmt.Sprintf("%s:%d is executed by no test, so no mutation could be run there.", m.File, m.Line)
			expected = "Every line the gate is asked to trust is executed by at least one test."
			recommend = "Cover the line, or remove it."
			severity = "low"
		default:
			title = "Undecided mutant: " + m.Operator
			what = fmt.Sprintf("%s:%d could not be decided by %s (timeout or engine error).", m.File, m.Line, r.Engine)
			expected = "Every attempted mutant is decided; an undecided one is not evidence of anything."
			recommend = "Raise the engine's timeout until nothing is undecided, then re-run. Do not treat this as a kill."
			severity = "high"
		}
		out = append(out, findings.Input{
			Reviewer:       "mutation-" + r.Engine,
			Severity:       severity,
			Classification: "blocking",
			Title:          title,
			Finding:        what,
			Expected:       expected,
			Found:          m.Detail,
			Recommendation: recommend,
			Evidence:       []findings.Evidence{{Type: "mutant", Path: m.File, Line: m.Line}},
			Fingerprint:    fmt.Sprintf("mutation:%s:%s:%s:%d", m.Status, m.Operator, m.File, m.Line),
		})
	}
	return out
}
