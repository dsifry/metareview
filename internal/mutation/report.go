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
	"sort"
	"strconv"
	"strings"

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

// Summary states the score the way an engine's own summary will not.
//
// It exists on the finding because the honest formula is the package's whole argument and it has
// to reach the operator to be worth anything: an engine reporting "Test efficacy: 100.00%" while
// 97 mutants timed out is exactly the number this contradicts, and a reader comparing the two
// needs to see both. Before this, Score and Complete had no non-test caller at all — the claim
// that this package computes its own totals was delivered entirely by Findings(), and the
// coverage floor was certifying an unreachable type kept alive by its own unit tests.
func (s Score) Summary() string {
	return fmt.Sprintf("Honest score: %d killed, %d survived, %d undecided, %d uncovered; efficacy %.1f%% (undecided counted against, uncovered excluded)%s.",
		s.Killed, s.Survived, s.Unresolved, s.Uncovered, s.Efficacy*100, incompleteNote(s))
}

func incompleteNote(s Score) string {
	if s.Complete() {
		return ""
	}
	return "; this run is INCOMPLETE, so no threshold should be applied to it"
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

// Findings turns a report into the findings a gate can act on.
//
// The shape matters as much as the content, because this feeds a gate a human has to survive. A
// survivor is one finding: it names a line and a test that must exist, and there is nothing to
// aggregate. The other two classes are not like that. Uncovered sites arrive by the file — the
// first real run of this against an under-tested package produces hundreds — so they are grouped
// per file, and they are advisory: a gate that blocks on every uncovered line is a gate that gets
// switched off, and a gate that is switched off is worse than one that was never trusted.
//
// Unresolved mutants collapse into a SINGLE finding, and this is the important one. They are not
// N defects; they are one fact — this run did not measure what it claims to have measured. The
// measured gremlins run on 2026-08-29 left 97 mutants undecided from one bad timeout, and a
// finding apiece would have buried the ten real survivors under a wall of noise caused by a
// single misconfiguration. It blocks, because a run that decided nothing is not proof of
// anything, but it is owned by whoever configured the engine rather than by the implementer:
// the code is not what is wrong.
func (r Report) Findings() []findings.Input {
	out := make([]findings.Input, 0, len(r.Mutants))
	uncovered := map[string][]Mutant{}
	uncoveredOrder := []string{}
	var unresolved []Mutant

	for _, m := range r.Mutants {
		switch m.Status {
		case Killed:
			continue
		case Survived:
			out = append(out, findings.Input{
				Reviewer:       r.reviewer(),
				Severity:       "medium",
				Classification: "blocking",
				Owner:          "implementer",
				Title:          "Surviving mutant: " + m.Operator,
				Finding:        fmt.Sprintf("%s:%d was mutated (%s) and every test still passed.", m.File, m.Line, m.Operator),
				Expected:       "A test fails when this code is deliberately broken.",
				Found:          m.Detail,
				Recommendation: "Add or strengthen a test that fails under this mutation, or delete the code if nothing depends on it.",
				Evidence:       []findings.Evidence{{Type: "mutant", Path: m.File, Line: m.Line}},
				Fingerprint:    fmt.Sprintf("mutation:survived:%s:%s:%d", m.Operator, m.File, m.Line),
			})
		case Uncovered:
			if _, seen := uncovered[m.File]; !seen {
				uncoveredOrder = append(uncoveredOrder, m.File)
			}
			uncovered[m.File] = append(uncovered[m.File], m)
		default:
			unresolved = append(unresolved, m)
		}
	}

	for _, file := range uncoveredOrder {
		ms := uncovered[file]
		out = append(out, findings.Input{
			Reviewer:       r.reviewer(),
			Severity:       "low",
			Classification: "advisory",
			Owner:          "implementer",
			Title:          "Uncovered mutation sites in " + file,
			Finding: fmt.Sprintf("%d site(s) in %s are executed by no test, so no mutation could be run there: %s.",
				len(ms), file, lineList(ms)),
			Expected:       "Every line the gate is asked to trust is executed by at least one test.",
			Recommendation: "Cover those lines, or remove them.",
			Evidence:       []findings.Evidence{{Type: "mutant", Path: file, Line: ms[0].Line}},
			// The ENGINE is part of the identity. Without it, two engines reporting uncovered
			// sites in the same file produced one fingerprint, and the cross-engine dedupe in
			// reviewers.MutationContext dropped the second — losing its sites and leaving the
			// surviving finding's count wrong. Engines disagree about what is uncovered (gremlins
			// and ooze differed by 137 mutants on one package here), so their reports are
			// different claims and must not silently overwrite each other.
			Fingerprint: "mutation:uncovered:" + r.engine() + ":" + file,
		})
	}

	if len(unresolved) > 0 {
		out = append(out, findings.Input{
			Reviewer:       r.reviewer(),
			Severity:       "high",
			Classification: "blocking",
			// Not the implementer: an undecided mutant says the engine could not answer, which
			// is a fact about how the run was configured, not about the code under test.
			Owner: "reviewer",
			Title: "Mutation run did not decide every mutant",
			Finding: fmt.Sprintf("%s left %d mutant(s) undecided (timeout or engine error), so this run measured less than it appears to: %s. %s",
				r.engine(), len(unresolved), siteList(unresolved), r.Score().Summary()),
			Expected:       "Every attempted mutant is decided; an undecided one is not evidence of anything.",
			Recommendation: "Raise the engine's timeout or narrow the target until nothing is undecided, then re-run. Do not treat an undecided mutant as a kill.",
			Evidence:       []findings.Evidence{{Type: "mutant", Path: unresolved[0].File, Line: unresolved[0].Line}},
			// Deliberately NOT keyed on r.Target. Load fills Target with the report's FILE PATH,
			// so a CI job writing /tmp/build-1234/mut.json and a developer's local copy produced
			// different fingerprints for the same run. Fingerprint identity is what makes an
			// `overridden` record suppress rediscovery, so a granted override on this — the
			// highest-severity finding this package raises — silently failed to rematch on the
			// next run, and the failure was invisible until someone relied on it. The engine and
			// the files it could not decide are the stable identity of that claim.
			Fingerprint: "mutation:unresolved:" + r.engine() + ":" + unresolvedKey(unresolved),
		})
	}
	return out
}

func (r Report) engine() string {
	if r.Engine == "" {
		return "the mutation engine"
	}
	return r.Engine
}

func (r Report) reviewer() string {
	if r.Engine == "" {
		return "mutation-reviewer"
	}
	return "mutation-" + r.Engine
}

// lineList names the lines a grouped finding covers, capped so one bad file cannot produce a
// finding too long to read. The count in the sentence is always the true one.
func lineList(ms []Mutant) string {
	const max = 12
	parts := make([]string, 0, max+1)
	for i, m := range ms {
		if i == max {
			parts = append(parts, fmt.Sprintf("and %d more", len(ms)-max))
			break
		}
		parts = append(parts, "line "+strconv.Itoa(m.Line))
	}
	return strings.Join(parts, ", ")
}

// siteList is lineList for mutants that may span files.
func siteList(ms []Mutant) string {
	const max = 8
	parts := make([]string, 0, max+1)
	for i, m := range ms {
		if i == max {
			parts = append(parts, fmt.Sprintf("and %d more", len(ms)-max))
			break
		}
		parts = append(parts, fmt.Sprintf("%s:%d", m.File, m.Line))
	}
	return strings.Join(parts, ", ")
}

// unresolvedKey identifies WHICH mutants an engine could not decide, independent of where the
// report file happened to be written. Sorted, so the same run keys the same way whatever order
// the engine emitted, and capped so one enormous failure cannot produce an unbounded key.
func unresolvedKey(ms []Mutant) string {
	const max = 8
	sites := make([]string, 0, len(ms))
	for _, m := range ms {
		sites = append(sites, fmt.Sprintf("%s:%d", m.File, m.Line))
	}
	sort.Strings(sites)
	if len(sites) > max {
		sites = append(sites[:max], fmt.Sprintf("+%d", len(ms)-max))
	}
	return strings.Join(sites, ",")
}
