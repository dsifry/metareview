package reviewers

import (
	"sort"

	"github.com/dsifry/metareview/internal/mutation"
)

// MutationContext is what a mutation-testing engine reported for this review.
//
// It is a review input like any other, not a separate mode: a survivor is a defect the gate
// should hold, and it belongs in the same findings ledger as every deterministic lint, so that
// one run chain, one set of fingerprints and one override mechanism cover both. The engine is
// declared per repository — metareview owns the translation, the repository owns which engine
// runs and against what — which is why nothing here knows or cares what produced the report.
//
// Absent is the ordinary case. A repository that runs no mutation engine passes exactly as
// before: this raises nothing on its own, and never a "you should be running mutation testing"
// finding. A gate that scolds you for not opting in is a gate people opt out of.
type MutationContext struct {
	Reports []mutation.Report
}

// Findings translates every report. Reports are ordered by engine and target so the review log
// reads the same way twice for the same inputs, and duplicates across reports collapse: running
// two engines over the same package — which the roadmap wants, since gremlins and ooze disagreed
// by 137 mutants on one package — must not double-report the site they agree on.
func (m MutationContext) Findings() []Finding {
	reports := append([]mutation.Report(nil), m.Reports...)
	sort.SliceStable(reports, func(i, j int) bool {
		if reports[i].Engine != reports[j].Engine {
			return reports[i].Engine < reports[j].Engine
		}
		return reports[i].Target < reports[j].Target
	})
	var out []Finding
	seen := map[string]bool{}
	for _, r := range reports {
		for _, f := range r.Findings() {
			// Fingerprints from different engines differ by construction (the reviewer name is
			// not part of them, but the operator is), so this collapses only what is genuinely
			// the same claim about the same line.
			if seen[f.Fingerprint] {
				continue
			}
			seen[f.Fingerprint] = true
			out = append(out, f)
		}
	}
	return out
}
