package prready

import (
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/findings"
	"github.com/dsifry/metareview/internal/githubcontext"
	"github.com/dsifry/metareview/internal/reviewlog"
)

func TestRenderEvidenceIncludesRequiredSections(t *testing.T) {
	markdown := RenderEvidence(EvidenceInput{
		Summary:    "Parser hardening for safe expression handling.",
		Validation: []string{"bash tests/run-all.sh exited 0"},
		TaskReviews: []ReviewEvidence{FromReviewLog(reviewlog.Summary{
			Target:  "task-1",
			Verdict: "PASS",
			Path:    "docs/metareview/reviews/task-1.md",
		})},
		EpicReviews: []ReviewEvidence{FromReviewLog(reviewlog.Summary{
			Target:  "epic-1",
			Verdict: "PASS",
			Path:    "docs/metareview/reviews/epic-1.md",
		})},
		Blockers: []Blocker{{ID: "mrvf-1", Title: "Missing test", Status: "fixed"}},
		GitHub: githubcontext.Context{
			Available:         false,
			UnavailableReason: "gh-unavailable",
		},
	})

	for _, required := range []string{
		"## metareview PR Evidence",
		"Parser hardening",
		"bash tests/run-all.sh exited 0",
		"task-1",
		"docs/metareview/reviews/task-1.md",
		"epic-1",
		"mrvf-1",
		"GitHub context unavailable: gh-unavailable",
		// The blocker-status section names its scope (recorded evidence) and points
		// to the live gate's own findings, so a clear status can't be misread as the
		// gate verdict (#135).
		"### Recorded-Evidence Blocker Status",
		"findings this gate run raised are under `## Blocking Findings` above.",
	} {
		if !strings.Contains(markdown, required) {
			t.Fatalf("rendered evidence missing %q:\n%s", required, markdown)
		}
	}
}

func TestRenderEvidenceRedactsGitHubText(t *testing.T) {
	credentialValue := "redaction-test-value"
	markdown := RenderEvidence(EvidenceInput{
		Summary:    "Ready.",
		Validation: []string{"go test ./... exited 0"},
		GitHub: githubcontext.Context{
			Available: true,
			URL:       "https://github.com/acme/repo/pull/9",
			Title:     "Contains token=" + credentialValue,
			Comments: []githubcontext.Entry{{
				Author: "alice",
				URL:    "https://github.com/acme/repo/pull/9#issuecomment-1",
				Body:   "password=" + credentialValue,
			}},
		},
	})

	for _, forbidden := range []string{credentialValue} {
		if strings.Contains(markdown, forbidden) {
			t.Fatalf("rendered evidence leaked %q:\n%s", forbidden, markdown)
		}
	}
	if !strings.Contains(markdown, "REDACTED") {
		t.Fatalf("expected redaction marker:\n%s", markdown)
	}
}

func TestRenderEvidenceIncludesAttemptCountsAndEscalation(t *testing.T) {
	body := RenderEvidence(EvidenceInput{
		Summary:    "branch summary",
		Validation: []string{"go test ./... exited 0"},
		TaskReviews: []ReviewEvidence{{
			Target:                "task-1",
			Verdict:               "ESCALATED",
			Path:                  "docs/metareview/reviews/task.md",
			HasUnresolvedBlockers: true,
			AttemptNumber:         3,
			MaxAttempts:           3,
			BlockingFindingCount:  1,
			AdvisoryFindingCount:  2,
			FollowUpFindingCount:  1,
		}},
		CurrentReview: &ReviewEvidence{
			Target:                "current branch",
			Verdict:               "ESCALATED",
			Path:                  "docs/metareview/reviews/pr.md",
			HasUnresolvedBlockers: true,
			AttemptNumber:         2,
			MaxAttempts:           2,
			BlockingFindingCount:  1,
			AdvisoryFindingCount:  1,
		},
	})
	if !strings.Contains(body, "task-1: ESCALATED with unresolved blockers attempt 3/3 findings: blocking 1, advisory 2, follow-up 1") ||
		!strings.Contains(body, "current branch: ESCALATED with unresolved blockers attempt 2/2 findings: blocking 1, advisory 1, follow-up 0") {
		t.Fatalf("expected attempt count and escalation in evidence:\n%s", body)
	}
}

// TestRenderEvidenceReconcilesOverriddenTaskReview reproduces issue #40: a
// historical NEEDS_REVISION task review whose blocking finding was cleared by a
// granted override must not still read "with unresolved blockers" while the
// blocker-status section correctly reports the branch clear, and an over-limit
// attempt counter (5/3) must never render bare.
func TestRenderEvidenceReconcilesOverriddenTaskReview(t *testing.T) {
	body := RenderEvidence(EvidenceInput{
		Summary:    "branch summary",
		Validation: []string{"go test ./... exited 0"},
		TaskReviews: []ReviewEvidence{{
			Target:                "task-7",
			Verdict:               "NEEDS_REVISION",
			Path:                  "docs/metareview/reviews/task-7.md",
			FindingIDs:            []string{"mrvf-over-1"},
			HasUnresolvedBlockers: true,
			AttemptNumber:         5,
			MaxAttempts:           3,
			BlockingFindingCount:  1,
		}},
		// Blocker status is clear: the override cleared the only blocker.
		Blockers: nil,
		Findings: []findings.Record{{
			ID:                  "mrvf-over-1",
			Status:              findings.StatusOverridden,
			Classification:      "blocking",
			Severity:            "high",
			Title:               "unsafe eval",
			OverrideGrantedBy:   "maintainer",
			OverrideGrantReason: "accepted risk for the 0.10 release cutover",
		}},
	})

	if strings.Contains(body, "task-7: NEEDS_REVISION with unresolved blockers") {
		t.Fatalf("resolved task review still shows unresolved blockers:\n%s", body)
	}
	if strings.Contains(body, "attempt 5/3\n") || strings.Contains(body, "attempt 5/3 ") || strings.HasSuffix(strings.TrimSpace(firstReviewLine(body, "task-7")), "attempt 5/3") {
		t.Fatalf("over-limit attempt counter rendered bare:\n%s", body)
	}
	if !strings.Contains(body, "override") || !strings.Contains(body, "maintainer") {
		t.Fatalf("resolved task review omits its resolver reference:\n%s", body)
	}
}

// firstReviewLine returns the first line mentioning target, for precise assertions.
func firstReviewLine(body, target string) string {
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, target+":") {
			return line
		}
	}
	return ""
}

func TestReconcileReviewResolvesByStatus(t *testing.T) {
	cases := []struct {
		name       string
		record     findings.Record
		wantResolv string
	}{
		{
			name:       "override",
			record:     findings.Record{ID: "f", Status: findings.StatusOverridden, Classification: "blocking", Severity: "high", OverrideGrantedBy: "boss", OverrideGrantReason: "accepted for release"},
			wantResolv: "override granted by boss (accepted for release)",
		},
		{
			name:       "fixed",
			record:     findings.Record{ID: "f", Status: "fixed", Classification: "blocking", Severity: "high", FixedInRunID: "mrv-run-9"},
			wantResolv: "fixed in run mrv-run-9",
		},
		{
			name:       "superseded",
			record:     findings.Record{ID: "f", Status: findings.StatusSuperseded, Classification: "spec-contract"},
			wantResolv: "superseded",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			review := ReviewEvidence{Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true, FindingIDs: []string{"f"}}
			got := reconcileReview(review, indexFindings([]findings.Record{tc.record}))
			if !got.Resolved {
				t.Fatalf("expected resolved, got %+v", got)
			}
			if got.HasUnresolvedBlockers {
				t.Fatalf("resolved review must clear HasUnresolvedBlockers: %+v", got)
			}
			if got.Resolution != tc.wantResolv {
				t.Fatalf("resolution = %q, want %q", got.Resolution, tc.wantResolv)
			}
		})
	}
}

func TestReconcileReviewKeepsBlockingWhenAnyFindingStillBlocks(t *testing.T) {
	review := ReviewEvidence{Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true, FindingIDs: []string{"a", "b"}}
	byID := indexFindings([]findings.Record{
		{ID: "a", Status: findings.StatusOverridden, Classification: "blocking", Severity: "high", OverrideGrantedBy: "boss", OverrideGrantReason: "accepted for release"},
		{ID: "b", Status: "open", Classification: "blocking", Severity: "high"},
	})
	got := reconcileReview(review, byID)
	if got.Resolved {
		t.Fatalf("a still-open blocker must keep the review unresolved: %+v", got)
	}
	if !got.HasUnresolvedBlockers {
		t.Fatalf("expected HasUnresolvedBlockers to remain true: %+v", got)
	}
}

// TestReconcileReviewIgnoresStillOpenAdvisory guards the defect both adversarial
// lenses found: an open ADVISORY (not a blocker class) on a review whose only
// blocker was overridden must NOT keep it "with unresolved blockers", or the
// task summary contradicts a clear blocker-status section again. The blocker
// predicate must match findings.UnresolvedBlocking (blocks AND blocking-class).
func TestReconcileReviewIgnoresStillOpenAdvisory(t *testing.T) {
	review := ReviewEvidence{Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true, FindingIDs: []string{"blk", "adv"}}
	byID := indexFindings([]findings.Record{
		{ID: "blk", Status: findings.StatusOverridden, Classification: "blocking", Severity: "high", OverrideGrantedBy: "boss", OverrideGrantReason: "accepted for release"},
		{ID: "adv", Status: "open", Classification: "advisory", Severity: "low"},
	})
	got := reconcileReview(review, byID)
	if !got.Resolved || got.HasUnresolvedBlockers {
		t.Fatalf("an open advisory must not keep the review blocking once its blocker is cleared: %+v", got)
	}
}

// TestReconcileReviewIgnoresDemotedBlocking: a blocking-classification finding at
// low severity is demoted to warning by classForCount, so it is not a blocker
// class and must be ignored — matching the blocker-status section.
func TestReconcileReviewIgnoresDemotedBlocking(t *testing.T) {
	review := ReviewEvidence{Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true, FindingIDs: []string{"blk", "low"}}
	byID := indexFindings([]findings.Record{
		{ID: "blk", Status: "fixed", Classification: "blocking", Severity: "high", FixedInRunID: "mrv-run-1"},
		{ID: "low", Status: "open", Classification: "blocking", Severity: "low"},
	})
	got := reconcileReview(review, byID)
	if !got.Resolved || got.HasUnresolvedBlockers {
		t.Fatalf("a demoted (low-severity) blocking finding must not keep the review blocking: %+v", got)
	}
}

// TestReconcileReviewNotResolvedWithoutBlockerClassFinding: a review flagged
// unresolved purely by its verdict, referencing only a non-blocker finding, has
// no blocker-class evidence of clearance and must stay unresolved.
func TestReconcileReviewNotResolvedWithoutBlockerClassFinding(t *testing.T) {
	review := ReviewEvidence{Verdict: "ESCALATED", HasUnresolvedBlockers: true, FindingIDs: []string{"adv"}}
	byID := indexFindings([]findings.Record{
		{ID: "adv", Status: findings.StatusOverridden, Classification: "advisory", Severity: "low", OverrideGrantedBy: "boss", OverrideGrantReason: "accepted for release"},
	})
	got := reconcileReview(review, byID)
	if got.Resolved || !got.HasUnresolvedBlockers {
		t.Fatalf("no blocker-class finding means no proof of clearance; must stay unresolved: %+v", got)
	}
}

// TestResolverPhraseSanitizesFreeText guards the second reviewer finding: a
// grant reason carrying newlines must not break out of its bullet.
func TestResolverPhraseSanitizesFreeText(t *testing.T) {
	phrase := resolverPhrase(findings.Record{
		Status:              findings.StatusOverridden,
		Classification:      "blocking",
		Severity:            "high",
		OverrideGrantedBy:   "lead\n### Injected",
		OverrideGrantReason: "accepted\n- fake bullet",
	})
	if strings.ContainsAny(phrase, "\n\r") {
		t.Fatalf("resolver phrase leaked a newline (markdown injection): %q", phrase)
	}
}

func TestReconcileReviewKeepsBlockingWhenNoFindingKnown(t *testing.T) {
	review := ReviewEvidence{Verdict: "ESCALATED", HasUnresolvedBlockers: true, FindingIDs: []string{"missing"}}
	got := reconcileReview(review, indexFindings(nil))
	if got.Resolved || !got.HasUnresolvedBlockers {
		t.Fatalf("unknown finding IDs must not resolve the review: %+v", got)
	}
}

func TestAttemptSummary(t *testing.T) {
	cases := []struct {
		name   string
		review ReviewEvidence
		want   string
	}{
		{name: "within limit", review: ReviewEvidence{AttemptNumber: 2, MaxAttempts: 3}, want: " attempt 2/3"},
		{name: "at limit", review: ReviewEvidence{AttemptNumber: 3, MaxAttempts: 3}, want: " attempt 3/3"},
		{name: "no counters", review: ReviewEvidence{}, want: ""},
		{name: "zero max", review: ReviewEvidence{AttemptNumber: 5, MaxAttempts: 0}, want: ""},
		{name: "over limit no note", review: ReviewEvidence{AttemptNumber: 5, MaxAttempts: 3}, want: " attempt 5 (exceeds recorded limit; no override recorded)"},
		{name: "over limit with note", review: ReviewEvidence{AttemptNumber: 5, MaxAttempts: 3, AttemptNote: "override granted by boss"}, want: " attempt 5 (override granted by boss)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := attemptSummary(tc.review); got != tc.want {
				t.Fatalf("attemptSummary = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestReconcileReviewAnnotatesOverLimitAttemptWithResolver(t *testing.T) {
	review := ReviewEvidence{Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true, FindingIDs: []string{"f"}, AttemptNumber: 5, MaxAttempts: 3}
	got := reconcileReview(review, indexFindings([]findings.Record{
		{ID: "f", Status: findings.StatusOverridden, Classification: "blocking", Severity: "high", OverrideGrantedBy: "boss", OverrideGrantReason: "accepted for release"},
	}))
	if !strings.Contains(got.AttemptNote, "over the recorded limit of 3") || !strings.Contains(got.AttemptNote, "override granted by boss") {
		t.Fatalf("over-limit attempt note missing resolver: %q", got.AttemptNote)
	}
}

func TestReconcileReviewAnnotatesOverLimitWithResolverEvenWhenNotResolved(t *testing.T) {
	// A PASS review with an over-limit attempt and an override on a referenced
	// finding: not "resolved" (it was not flagged unresolved), but the counter
	// still gets the override reference rather than the generic note.
	review := ReviewEvidence{Verdict: "PASS", FindingIDs: []string{"f"}, AttemptNumber: 5, MaxAttempts: 3}
	got := reconcileReview(review, indexFindings([]findings.Record{
		{ID: "f", Status: findings.StatusOverridden, Classification: "blocking", Severity: "high", OverrideGrantedBy: "boss", OverrideGrantReason: "accepted for release"},
	}))
	if got.Resolved {
		t.Fatalf("a PASS review was not flagged unresolved; it should not become resolved: %+v", got)
	}
	if !strings.Contains(got.AttemptNote, "over the recorded limit of 3") || !strings.Contains(got.AttemptNote, "override granted by boss") {
		t.Fatalf("expected override reference in attempt note: %q", got.AttemptNote)
	}
}

func TestReconcileReviewsHandlesEmptyAndPreservesOrder(t *testing.T) {
	if got := reconcileReviews(nil, nil); got != nil {
		t.Fatalf("nil input should return nil, got %+v", got)
	}
	reviews := []ReviewEvidence{{Target: "a"}, {Target: "b"}}
	got := reconcileReviews(reviews, indexFindings(nil))
	if len(got) != 2 || got[0].Target != "a" || got[1].Target != "b" {
		t.Fatalf("order not preserved: %+v", got)
	}
}

func TestDedupeStrings(t *testing.T) {
	got := dedupeStrings([]string{"a", "", "a", "b", "b", "c"})
	want := []string{"a", "b", "c"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("dedupeStrings = %v, want %v", got, want)
	}
}

func TestRenderEvidenceReconcilesEpicReviews(t *testing.T) {
	body := RenderEvidence(EvidenceInput{
		Summary: "branch summary",
		EpicReviews: []ReviewEvidence{{
			Target:                "epic-3",
			Verdict:               "NEEDS_REVISION",
			Path:                  "docs/metareview/reviews/epic-3.md",
			FindingIDs:            []string{"mrvf-epic-1"},
			HasUnresolvedBlockers: true,
		}},
		Findings: []findings.Record{{
			ID:                  "mrvf-epic-1",
			Status:              findings.StatusOverridden,
			Classification:      "blocking",
			Severity:            "high",
			OverrideGrantedBy:   "maintainer",
			OverrideGrantReason: "carried by a later epic pass",
		}},
	})
	if strings.Contains(body, "epic-3: NEEDS_REVISION with unresolved blockers") {
		t.Fatalf("epic review not reconciled:\n%s", body)
	}
	if !strings.Contains(body, "historical; resolved") {
		t.Fatalf("epic review missing historical annotation:\n%s", body)
	}
}

func TestRenderEvidenceHistoricalTaskAgreesWithBlockerStatus(t *testing.T) {
	// The acceptance shape: historical unresolved-looking review + override record;
	// task summary and blocker status agree, attempt counter annotated.
	body := RenderEvidence(EvidenceInput{
		Summary: "branch summary",
		TaskReviews: []ReviewEvidence{{
			Target:                "task-9",
			Verdict:               "ESCALATED",
			FindingIDs:            []string{"mrvf-9"},
			HasUnresolvedBlockers: true,
			AttemptNumber:         4,
			MaxAttempts:           3,
		}},
		Blockers: nil,
		Findings: []findings.Record{{
			ID:                  "mrvf-9",
			Status:              findings.StatusOverridden,
			Classification:      "blocking",
			Severity:            "high",
			OverrideGrantedBy:   "lead",
			OverrideGrantReason: "documented exception",
		}},
	})
	if !strings.Contains(body, "No unresolved blockers in the recorded review evidence.") {
		t.Fatalf("expected clear blocker status:\n%s", body)
	}
	if strings.Contains(body, "with unresolved blockers") {
		t.Fatalf("task summary contradicts clear blocker status:\n%s", body)
	}
	if !strings.Contains(body, "over the recorded limit of 3") {
		t.Fatalf("over-limit attempt not annotated:\n%s", body)
	}
}

func TestRenderEvidenceDistinguishesStructuredValidation(t *testing.T) {
	body := RenderEvidence(EvidenceInput{
		Summary: "branch summary",
		Validation: []string{
			`structured validation: go test ./... exited 0 (exit 0)`,
			`freeform fallback validation: npm run build exited 0 (exit 0)`,
		},
	})
	for _, required := range []string{
		"structured validation: go test ./... exited 0 (exit 0)",
		"freeform fallback validation: npm run build exited 0 (exit 0)",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("expected rendered validation summary %q:\n%s", required, body)
		}
	}
}
