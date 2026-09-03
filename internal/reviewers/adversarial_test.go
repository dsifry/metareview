package reviewers

import "testing"

func blockingCount(fs []Finding) (blocking, advisory int) {
	for _, f := range fs {
		if f.Classification == "advisory" {
			advisory++
		} else {
			blocking++ // finding() defaults Classification to "blocking"
		}
	}
	return
}

// The core of build B: the gate must REQUIRE an adjudicated lens review over this head.
func TestAdversarialReviewFindings(t *testing.T) {
	cases := []struct {
		name         string
		require      bool
		status       AdversarialReviewStatus
		wantBlocking int
		wantAdvisory int
		wantFinding  string // substring in the (first) finding's Title, "" = no findings
		wantHeadText string // substring the finding body must name for the head, "" = don't check
	}{
		{"flag off allows the mechanical pass", false, AdversarialReviewStatus{}, 0, 0, "", ""},
		{"required + no marker blocks", true, AdversarialReviewStatus{HeadSHA: "abc"}, 1, 0, "No adjudicated lens review", "HEAD abc"},
		{"required + no marker + empty head still blocks", true, AdversarialReviewStatus{}, 1, 0, "No adjudicated lens review", "the current head"},
		{"required + marker not passing blocks", true, AdversarialReviewStatus{Present: true, Verdict: "NEEDS_REVISION", HeadSHA: "abc"}, 1, 0, "unresolved findings", "HEAD abc"},
		{"required + passing independent marker satisfies", true, AdversarialReviewStatus{Present: true, Verdict: "PASS", HeadSHA: "abc"}, 0, 0, "", ""},
		{"required + PASS_ADVISORY satisfies", true, AdversarialReviewStatus{Present: true, Verdict: "PASS_ADVISORY", HeadSHA: "abc"}, 0, 0, "", ""},
		{"required + passing emulated is advisory, not blocking", true, AdversarialReviewStatus{Present: true, Verdict: "PASS", Emulated: true, HeadSHA: "abc"}, 0, 1, "in-session-emulated", ""},
		{"required + passing marker but unattested working tree blocks", true, AdversarialReviewStatus{Present: true, Verdict: "PASS", WorkingTreeUnattested: true, HeadSHA: "abc"}, 1, 0, "does not cover the working tree", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := adversarialReviewFindings(c.require, c.status)
			b, a := blockingCount(got)
			if b != c.wantBlocking || a != c.wantAdvisory {
				t.Fatalf("blocking=%d advisory=%d, want blocking=%d advisory=%d (%+v)", b, a, c.wantBlocking, c.wantAdvisory, got)
			}
			if c.wantFinding == "" {
				if len(got) != 0 {
					t.Fatalf("expected no findings, got %+v", got)
				}
				return
			}
			if len(got) == 0 || !contains(got[0].Title, c.wantFinding) {
				t.Fatalf("expected a finding titled ~%q, got %+v", c.wantFinding, got)
			}
			if got[0].Reviewer != "adversarial-review-reviewer" {
				t.Fatalf("wrong reviewer: %q", got[0].Reviewer)
			}
			// The body must name the head it reviewed (the real SHA, or the empty-head fallback phrase) —
			// in whichever field carries it (Finding for the no-marker branch, Found for the not-a-pass one).
			body := got[0].Finding + " " + got[0].Found
			if c.wantHeadText != "" && !contains(body, c.wantHeadText) {
				t.Fatalf("finding must name the head as %q; got %q", c.wantHeadText, body)
			}
		})
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
