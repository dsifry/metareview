package reviewers

import (
	"strings"
	"testing"
)

func TestEpicReadyReviewersBlockContradictionsAndIntentDrift(t *testing.T) {
	findings := RunEpicReady(EpicReadyContext{
		Epic: EpicContext{ID: "epic-1", Body: "Build a parser without executing user input."},
		Children: []EpicChild{
			{ID: "task-1", Body: "Use eval for expression parsing."},
			{ID: "task-2", Body: "Avoid eval and parse JSON safely."},
		},
		Git: EpicGitContext{Diff: "+module.exports = input => eval(input);\n"},
		ReviewLogs: []EpicReviewLog{
			{Target: "task-1", Verdict: "PASS"},
			{Target: "task-2", Verdict: "PASS"},
		},
		EvidenceText: "task-1 passed\ntask-2 passed\n",
	})

	assertEpicFinding(t, findings, "epic-integration-reviewer", "Cross-task contradiction")
	assertEpicFinding(t, findings, "intent-preservation-reviewer", "Epic intent drift")
}

func TestEpicReadyReviewersBlockMissingEvidenceAndUnresolvedBlockers(t *testing.T) {
	findings := RunEpicReady(EpicReadyContext{
		Epic: EpicContext{ID: "epic-1", Body: "Ship parser."},
		Children: []EpicChild{
			{ID: "task-1", Body: "Parser implementation."},
			{ID: "task-2", Body: "Parser tests."},
		},
		ReviewLogs: []EpicReviewLog{
			{Target: "task-1", Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true, FindingIDs: []string{"mrvf-1"}},
		},
		EvidenceText: "task-1 passed\n",
	})

	assertEpicFinding(t, findings, "acceptance-reviewer", "Missing child acceptance evidence")
	assertEpicFinding(t, findings, "epic-integration-reviewer", "Unresolved child blockers")
}

func TestEpicReadyReviewersDoNotTreatBareTaskMentionAsEvidence(t *testing.T) {
	findings := RunEpicReady(EpicReadyContext{
		Epic:         EpicContext{ID: "epic-1", Body: "Ship parser."},
		Children:     []EpicChild{{ID: "task-1", Body: "Parser implementation."}},
		EvidenceText: "task-1 still needs review\n",
	})

	assertEpicFinding(t, findings, "acceptance-reviewer", "Missing child acceptance evidence")
}

func TestEpicReadyReviewersBlockMissingServiceInventoryForServiceChanges(t *testing.T) {
	findings := RunEpicReady(EpicReadyContext{
		Epic:     EpicContext{ID: "epic-1", Body: "Add billing integration."},
		Children: []EpicChild{{ID: "task-1", Body: "Add billing service."}},
		Git:      EpicGitContext{ChangedFiles: []string{"internal/billing/payment_service.go"}},
		ReviewLogs: []EpicReviewLog{
			{Target: "task-1", Verdict: "PASS"},
		},
		EvidenceText: "task-1 passed\n",
	})

	assertEpicFinding(t, findings, "architecture-reviewer", "Missing service inventory update")
}

func TestEpicReadyReceivesContextRiskFlags(t *testing.T) {
	findings := RunEpicReady(EpicReadyContext{
		Epic:     EpicContext{ID: "epic-1", Body: "Build a parser without executing user input."},
		Children: []EpicChild{{ID: "task-1", Body: "Add context profiling."}},
		Git: EpicGitContext{
			ChangedFiles: []string{"internal/contextprofile/profile.go"},
			Diff:         "+module.exports = input => ev" + "al(input);\n",
			RiskLevel:    "context-risk",
			RiskReasons:  []string{"LARGE_DIFF", "UNTRACKED_OMITTED"},
		},
		ReviewLogs:   []EpicReviewLog{{Target: "task-1", Verdict: "PASS"}},
		Knowledge:    EpicKnowledgeContext{ServiceInventory: "Context profile: `internal/contextprofile/profile.go`"},
		EvidenceText: "task-1 passed\n",
	})

	assertEpicFinding(t, findings, "architecture-reviewer", "Review context risk")
	if len(findings) != 1 {
		t.Fatalf("context risk should preflight domain reviewers, got %+v", findings)
	}
}

func TestEpicReadyReviewersAllowCleanEpic(t *testing.T) {
	findings := RunEpicReady(EpicReadyContext{
		Epic:     EpicContext{ID: "epic-1", Body: "Parse JSON safely."},
		Children: []EpicChild{{ID: "task-1", Body: "Use JSON parser."}},
		Git:      EpicGitContext{ChangedFiles: []string{"internal/parser/parser.go"}, Diff: "+return json.Valid(input)\n"},
		ReviewLogs: []EpicReviewLog{
			{Target: "task-1", Verdict: "PASS"},
		},
		Knowledge:    EpicKnowledgeContext{ServiceInventory: "Parser: `internal/parser/parser.go`"},
		EvidenceText: "task-1 passed\nbash tests/run-all.sh exited 0\n",
	})
	if len(findings) != 0 {
		t.Fatalf("clean epic should not produce findings: %+v", findings)
	}
}

func assertEpicFinding(t *testing.T, findings []Finding, reviewer, titlePart string) {
	t.Helper()
	for _, finding := range findings {
		if finding.Reviewer == reviewer && strings.Contains(finding.Title, titlePart) {
			return
		}
	}
	t.Fatalf("missing finding reviewer=%s title~=%s in %+v", reviewer, titlePart, findings)
}
