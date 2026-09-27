package reviewstate

import "strings"

// LastReviewedBase is the reserved `--base` token for an incremental review (#176): review only what is new since
// the last passing adjudicated review of the same scope.
const LastReviewedBase = "last-reviewed"

// Checkpoint returns the head of the most recently reviewed commit of scope that an incremental review may start
// from, and false when there is none. It is derived from the existing review-evidence markers — no new state. A
// head qualifies when:
//
//   - it is a STRICT ancestor of head. A marker at head itself is the review a checkpoint base anchors (its base is
//     the checkpoint), so counting it would turn the next `--base last-reviewed` into an empty range and orphan it;
//     a marker on another branch fails the ancestry test.
//   - the LATEST marker recorded for it passed (PASS or PASS_ADVISORY, case-insensitively — the gate's rule for a
//     marker verdict). As in the gate, the last-recorded review of a head decides it: a later NEEDS_REVISION at the
//     same head withdraws an earlier PASS, so the next increment does not start after rejected code.
//   - its marker vouches for everything back to the fork point: the marker's base is at or before forkPoint, or is
//     itself a qualifying head that is an ancestor of it. Without this a passing review over a narrow base would let the commits before it go
//     unreviewed; a marker with no recorded base cannot prove coverage and never qualifies.
//
// Of the qualifying heads the NEAREST to head wins — the one no other qualifying head descends from — so the
// increment is as narrow as the reviews allow; heads on merged lines that are unordered go to record order.
func Checkpoint(root, scope, head, forkPoint string, isAncestor func(ancestor, descendant string) (bool, error)) (string, bool, error) {
	all, err := DiscoverReviewEvidence(root)
	if err != nil {
		return "", false, err
	}
	latest := map[string]ReviewEvidence{} // head → its last-recorded marker of scope
	var order []string                    // heads by the record order of their latest marker, newest last
	for _, m := range all {
		if m.ReviewedScope != scope || m.HeadSHA == "" {
			continue
		}
		if _, seen := latest[m.HeadSHA]; seen {
			order = removeString(order, m.HeadSHA)
		}
		latest[m.HeadSHA] = m
		order = append(order, m.HeadSHA)
	}
	covered := map[string]bool{}
	visiting := map[string]bool{}
	var vouches func(h string) (bool, error)
	vouches = func(h string) (bool, error) {
		if done, ok := covered[h]; ok {
			return done, nil
		}
		m, ok := latest[h]
		if !ok || !passingVerdict(m.AdjudicatedVerdict) || m.BaseSHA == "" || visiting[h] {
			return false, nil
		}
		visiting[h] = true
		defer delete(visiting, h)
		result, err := isAncestor(m.BaseSHA, forkPoint)
		if err == nil && !result {
			// Chain only through a base in this history: a pre-rebase base reviewed back to the fork point proves
			// nothing about the rewritten commits between it and h.
			if result, err = isAncestor(m.BaseSHA, h); err == nil && result {
				result, err = vouches(m.BaseSHA)
			}
		}
		if err != nil {
			return false, err
		}
		covered[h] = result
		return result, nil
	}
	var qualifying []string // newest-recorded first
	for i := len(order) - 1; i >= 0; i-- {
		h := order[i]
		if h == head {
			continue
		}
		ok, err := isAncestor(h, head)
		if err != nil {
			return "", false, err
		}
		if !ok {
			continue
		}
		if ok, err = vouches(h); err != nil {
			return "", false, err
		}
		if ok {
			qualifying = append(qualifying, h)
		}
	}
	// The nearest one: no other qualifying head descends from it (ties, from merged lines, go to record order).
	for _, h := range qualifying {
		nearest := true
		for _, other := range qualifying {
			if other == h {
				continue
			}
			below, err := isAncestor(h, other)
			if err != nil {
				return "", false, err
			}
			if below {
				nearest = false
				break
			}
		}
		if nearest {
			return h, true, nil
		}
	}
	return "", false, nil
}

func passingVerdict(v string) bool {
	return strings.EqualFold(v, "PASS") || strings.EqualFold(v, "PASS_ADVISORY")
}

func removeString(list []string, s string) []string {
	out := list[:0]
	for _, v := range list {
		if v != s {
			out = append(out, v)
		}
	}
	return out
}
