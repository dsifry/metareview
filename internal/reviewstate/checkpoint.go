package reviewstate

import "strings"

// LastReviewedBase is the reserved `--base` token for an incremental review (#176): review only what is new since
// the last passing adjudicated review of the same scope.
const LastReviewedBase = "last-reviewed"

// Checkpoint returns the head of the most recently recorded passing review-evidence marker for scope whose head is
// a STRICT ancestor of head, and false when there is none. It is derived from the existing markers — no new state.
// A marker at head itself is skipped: it is the review a checkpoint base anchors (its base is the checkpoint), so
// counting it would turn the next `--base last-reviewed` into an empty range and orphan that marker. A marker on
// another branch fails the ancestry test; a NEEDS_REVISION one is not a checkpoint. "Passing" is the gate's own
// rule for a marker verdict: PASS or PASS_ADVISORY, case-insensitively.
func Checkpoint(root, scope, head string, isAncestor func(ancestor, descendant string) (bool, error)) (string, bool, error) {
	markers, err := DiscoverReviewEvidence(root)
	if err != nil {
		return "", false, err
	}
	for i := len(markers) - 1; i >= 0; i-- { // record order: the latest first
		m := markers[i]
		if m.ReviewedScope != scope || m.HeadSHA == "" || m.HeadSHA == head || !passingVerdict(m.AdjudicatedVerdict) {
			continue
		}
		ok, err := isAncestor(m.HeadSHA, head)
		if err != nil {
			return "", false, err
		}
		if ok {
			return m.HeadSHA, true, nil
		}
	}
	return "", false, nil
}

func passingVerdict(v string) bool {
	return strings.EqualFold(v, "PASS") || strings.EqualFold(v, "PASS_ADVISORY")
}
