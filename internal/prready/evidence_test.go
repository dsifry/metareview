package prready

import (
	"strings"
	"testing"

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
