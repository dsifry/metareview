package reviewers

import (
	"os"
	"slices"
	"sort"

	"github.com/dsifry/metareview/internal/findings"
	"github.com/dsifry/metareview/internal/mutation"
	"github.com/dsifry/metareview/internal/mutationfresh"
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
	// Freshness (spec §6.7). Serialized into pr-ready's reviewer-input digest only when reports were
	// supplied (omitempty), so a run without reports keeps its digest.
	Mode      string                          `json:"mode,omitempty"`
	Freshness []mutationfresh.ReportFreshness `json:"freshness,omitempty"`
	// Views are the run's --mutation-view names (spec §11.3), in the digest only when used.
	Views []string `json:"views,omitempty"`
	// FreshnessFindings and FreshnessSection are derived from the fields above.
	FreshnessFindings []findings.Input `json:"-"`
	FreshnessSection  string           `json:"-"`
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
			// Every fingerprint names its engine, so this collapses only what is genuinely the
			// same claim from the same engine. That was not always true: the uncovered
			// fingerprint was once just the file path, so gremlins and stryker reporting
			// uncovered sites in one file collided here and the second report's sites were
			// dropped silently, leaving the surviving finding's site count wrong.
			if seen[f.Fingerprint] {
				continue
			}
			seen[f.Fingerprint] = true
			out = append(out, f)
		}
	}
	for _, f := range m.FreshnessFindings {
		if !seen[f.Fingerprint] {
			seen[f.Fingerprint] = true
			out = append(out, f)
		}
	}
	return out
}

// Engines lists the engines of this run's reports, in first-seen order (findings.Options.MutationEngines).
func (m MutationContext) Engines() []string {
	var out []string
	for _, r := range m.Reports {
		if !slices.Contains(out, r.Engine) {
			out = append(out, r.Engine)
		}
	}
	return out
}

// ViewMaps is, per engine, the sorted view names in the maps of this run's attested reports that
// carry one (findings.Options.MutationViewMaps, the rename sweep of spec §11.3).
func (m MutationContext) ViewMaps() map[string][]string {
	var out map[string][]string
	for _, f := range m.Freshness {
		if !f.Attested || f.ViewNames == nil {
			continue
		}
		if out == nil {
			out = map[string][]string{}
		}
		for _, name := range f.ViewNames {
			if !slices.Contains(out[f.Engine], name) {
				out[f.Engine] = append(out[f.Engine], name)
			}
		}
		sort.Strings(out[f.Engine])
	}
	return out
}

// LoadMutationContext loads the declared reports and judges their freshness against the content
// under review: HEAD for pr-ready (unless --include-working-tree), the working tree otherwise. An
// unreadable report, an invalid mode or a content read error stops the review: a mutation gate
// that quietly drops a report passes because it looked at less.
func LoadMutationContext(root string, paths, views []string, scope string, head bool) (MutationContext, error) {
	if len(paths) == 0 {
		return MutationContext{}, nil
	}
	reports, err := mutation.LoadAll(paths)
	if err != nil {
		return MutationContext{}, err
	}
	mode, err := mutationfresh.ModeFromEnv(os.Getenv)
	if err != nil {
		return MutationContext{}, err
	}
	content := mutationfresh.Worktree(root)
	if head {
		content = mutationfresh.Head(root)
	}
	res, err := mutationfresh.Build(reports, content, mutationfresh.EffectiveMode(scope, mode), views)
	if err != nil {
		return MutationContext{}, err
	}
	return MutationContext{Reports: reports, Mode: res.Mode, Freshness: res.Freshness, Views: views, FreshnessFindings: res.Findings, FreshnessSection: res.Section}, nil
}
