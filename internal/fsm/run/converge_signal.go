package run

import "sort"

// LensConvergence counts, per file, how many DISTINCT reviewers reported a finding there.
//
// Independent convergence is the cheapest evidence that a defect is a class rather than a
// one-off: when six reviewers looking through different lenses land on the same file, they are
// unlikely to have found six unrelated things. One reviewer repeating itself is not that, which
// is why the count is over distinct sources rather than over findings.
//
// The signal was being destroyed before anything could use it. reviewLenses.Reduce flattened each
// lens report into a findings list and dropped the lens name, so downstream could see duplicates
// but not whether they were independent. It exists here as a plain function over recorded data —
// no judge, no inference — so "these lenses agreed" is a fact in the machine rather than
// something a human notices reading transcripts.
func LensConvergence(findings []Finding) map[string]int {
	seen := map[string]map[string]bool{}
	for _, f := range findings {
		if f.File == "" || f.Source == "" {
			// A finding with no file cannot be attributed to one, and one with no source predates
			// the stamping or came from a node that does not have lenses. Neither is evidence of
			// convergence, and counting them would inflate it.
			continue
		}
		if seen[f.File] == nil {
			seen[f.File] = map[string]bool{}
		}
		seen[f.File][f.Source] = true
	}
	out := map[string]int{}
	for file, sources := range seen {
		out[file] = len(sources)
	}
	return out
}

// ConvergedFiles are the files at least min distinct reviewers reported, most-agreed first, with
// ties broken by path so the order is stable between runs over the same data.
func ConvergedFiles(findings []Finding, min int) []string {
	counts := LensConvergence(findings)
	out := []string{}
	for file, n := range counts {
		if n >= min {
			out = append(out, file)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if counts[out[i]] != counts[out[j]] {
			return counts[out[i]] > counts[out[j]]
		}
		return out[i] < out[j]
	})
	return out
}
