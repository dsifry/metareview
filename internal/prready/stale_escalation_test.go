package prready

import (
	"testing"

	"github.com/dsifry/metareview/internal/findings"
)

// Spec §6.8: a chain blocked only by stale mutation evidence waits for fresh evidence up to
// 2 × maxAttempts before escalating; any other blocker escalates at maxAttempts as before.
func TestStaleOnlyChainsWaitForFreshEvidence(t *testing.T) {
	blocking := findings.ClassCounts{Blocking: 1}
	cases := []struct {
		attempt, max int
		staleOnly    bool
		verdict      string
		reason       string
	}{
		{3, 3, false, "ESCALATED", "blocking findings remain after attempt 3 of 3"},
		{3, 3, true, "NEEDS_REVISION", ""},
		{5, 3, true, "NEEDS_REVISION", ""},
		{6, 3, true, "ESCALATED", "stale mutation evidence not refreshed after 6 attempts"},
		{2, 3, true, "NEEDS_REVISION", ""},
	}
	for _, c := range cases {
		verdict, _, _, reason := verdictForCounts(blocking, "gate", c.attempt, c.max, c.staleOnly)
		if verdict != c.verdict || reason != c.reason {
			t.Errorf("%+v: got %s %q", c, verdict, reason)
		}
	}
}
