package learning

import (
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/findings"
	"github.com/dsifry/metareview/internal/githubcontext"
	"github.com/dsifry/metareview/internal/sessionhistory"
)

func TestExtractCandidatesFromReviewFindingsAndRepeatedThemes(t *testing.T) {
	result := ExtractCandidates(Input{
		Findings: []findings.Record{
			{
				ID:                 "mrvf-1",
				Reviewer:           "architecture-reviewer",
				Severity:           "high",
				Classification:     "blocking",
				Status:             "fixed",
				Title:              "Duplicate service path in internal/acme/payment_clone.go",
				Finding:            "Added another payment service path instead of reusing the inventoried one.",
				Expected:           "Reuse existing service paths from SERVICE_INVENTORY.md.",
				Recommendation:     "Check service inventory before adding parallel service code.",
				KnowledgeCandidate: true,
				FixedInRunID:       "mrv-2",
			},
			{
				ID:             "mrvf-2",
				Reviewer:       "architecture-reviewer",
				Severity:       "high",
				Classification: "blocking",
				Status:         "open",
				Title:          "Duplicate service path",
				Finding:        "Second duplicate service path blocker.",
			},
			{
				ID:             "mrvf-3",
				Reviewer:       "architecture-reviewer",
				Severity:       "high",
				Classification: "blocking",
				Status:         "open",
				Title:          "Duplicate service path",
				Finding:        "Third duplicate service path blocker.",
			},
		},
	})

	if len(result.Knowledge) < 3 {
		t.Fatalf("expected knowledge candidates from marked, fixed, and repeated blockers: %+v", result.Knowledge)
	}
	assertKnowledge(t, result.Knowledge, "knowledge-candidate", "service inventory")
	assertKnowledge(t, result.Knowledge, "fixed-review-defect", "review-driven fix")
	assertKnowledge(t, result.Knowledge, "repeated-blocker-theme", "Repeated blocker theme")
	for _, candidate := range result.Knowledge {
		if candidate.Scope == "" || candidate.ProposedTarget == "" || candidate.Confidence == "" || len(candidate.SourceRefs) == 0 {
			t.Fatalf("candidate missing required metadata: %+v", candidate)
		}
		if strings.Contains(candidate.Text, "payment_clone.go") {
			t.Fatalf("candidate was not generalized: %+v", candidate)
		}
	}
}

func TestExtractCandidatesFromGitHubAndSessionCorrectionsWithRedaction(t *testing.T) {
	credentialValue := "redaction-test-value"
	result := ExtractCandidates(Input{
		GitHub: githubcontext.Context{
			Available: true,
			PRNumber:  "42",
			Reviews: []githubcontext.Entry{{
				State: "CHANGES_REQUESTED",
				URL:   "https://github.com/acme/repo/pull/42#pullrequestreview-1",
				Body:  "Blocker: do not store token=" + credentialValue + " in review artifacts.",
			}},
		},
		Session: sessionhistory.Context{
			Available: true,
			Signals: []sessionhistory.Signal{{
				Path:       "~/.codex/sessions/2026/05/session.jsonl",
				SourceType: "codex-jsonl",
				RecordKind: "raw-transcript",
				Confidence: "high",
				Excerpt:    "Reviewer correction: use the existing post-merge learning log instead of creating another registry.",
			}},
		},
	})

	assertKnowledge(t, result.Knowledge, "github-review-blocker", "GitHub review blocker")
	assertKnowledge(t, result.Knowledge, "session-derived-correction", "Reviewer correction")
	for _, candidate := range result.Knowledge {
		if strings.Contains(candidate.Text, credentialValue) {
			t.Fatalf("candidate leaked secret-like text: %+v", candidate)
		}
	}
}

func TestExtractCalibrationCandidatesFromFalsePositiveAndAcceptedRiskPatterns(t *testing.T) {
	result := ExtractCandidates(Input{
		Findings: []findings.Record{
			{ID: "mrvf-fp", Reviewer: "security-reviewer", Status: "false-positive", Title: "False positive eval finding", Finding: "The flagged eval path was test fixture data."},
			{ID: "mrvf-risk", Reviewer: "architecture-reviewer", Status: "accepted-risk", Title: "Accepted risk for staged rollout", Finding: "The team accepted a temporary compatibility path."},
			{ID: "mrvf-rebut", Reviewer: "test-reviewer", Status: "rebutted", Title: "Rebutted missing test finding", Finding: "Existing golden tests covered this path."},
		},
	})

	if len(result.Calibration) != 3 {
		t.Fatalf("expected calibration candidates, got %+v", result.Calibration)
	}
	assertCalibration(t, result.Calibration, "false-positive")
	assertCalibration(t, result.Calibration, "accepted-risk")
	assertCalibration(t, result.Calibration, "rebutted")
	for _, candidate := range result.Calibration {
		if candidate.Scope != "reviewer-calibration" || candidate.ProposedTarget != "reviewer-calibration" {
			t.Fatalf("unexpected calibration metadata: %+v", candidate)
		}
	}
}

func assertKnowledge(t *testing.T, candidates []Candidate, kind, text string) {
	t.Helper()
	for _, candidate := range candidates {
		if candidate.Kind == kind && strings.Contains(candidate.Text, text) {
			return
		}
	}
	t.Fatalf("missing knowledge candidate kind=%s text=%q in %+v", kind, text, candidates)
}

func assertCalibration(t *testing.T, candidates []Candidate, disposition string) {
	t.Helper()
	for _, candidate := range candidates {
		if candidate.Disposition == disposition {
			return
		}
	}
	t.Fatalf("missing calibration disposition=%s in %+v", disposition, candidates)
}
