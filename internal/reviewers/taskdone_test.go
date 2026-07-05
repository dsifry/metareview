package reviewers

import (
	"strings"
	"testing"
)

func TestTaskDoneReviewersBlockUnsafeDiffsAndMissingValidation(t *testing.T) {
	findings := RunTaskDone(Context{
		Task: TaskContext{Type: "beads-task", ID: "task-1", Text: "Acceptance: do not execute user input"},
		Git: GitContext{
			ChangedFiles:             []string{"lib/unsafe.js"},
			Diff:                     "+module.exports = input => eval(input);\n+// TODO: add tests later\n",
			DiffTruncated:            false,
			WorkingTreeDiffTruncated: false,
		},
		Knowledge: KnowledgeContext{},
	})

	assertFinding(t, findings, "security-reviewer", "critical", "Unsafe eval introduced")
	assertFinding(t, findings, "test-reviewer", "high", "Missing test changes")
	assertFinding(t, findings, "code-quality-reviewer", "high", "TODO")
}

func TestTaskDoneReviewersInspectUnsafeUntrackedSource(t *testing.T) {
	findings := RunTaskDone(Context{
		Task: TaskContext{Type: "beads-task", ID: "task-1", Text: "Acceptance: do not execute user input"},
		Git: GitContext{
			UntrackedFiles:    []string{"lib/new-unsafe.js"},
			UntrackedExcerpts: "--- lib/new-unsafe.js\n+module.exports = input => eval(input);\n",
		},
	})

	assertFinding(t, findings, "security-reviewer", "critical", "Unsafe eval introduced")
}

func TestTaskDoneReviewersFlagArchitectureRisks(t *testing.T) {
	truncated := RunTaskDone(Context{
		Task: TaskContext{Type: "beads-task", ID: "task-1", Text: "Acceptance: parse JSON safely"},
		Git:  GitContext{ChangedFiles: []string{"lib/safe.js"}, DiffTruncated: true},
	})
	assertFinding(t, truncated, "architecture-reviewer", "high", "Diff context was truncated")

	duplicate := RunTaskDone(Context{
		Task: TaskContext{Type: "beads-task", ID: "task-1", Text: "Acceptance: parse JSON safely"},
		Git: GitContext{
			ChangedFiles: []string{"lib/safe.js", "tests/lib/test-safe.sh", "lib/billing-service-v2.js"},
			Diff:         "+module.exports = input => JSON.parse(input);\n",
		},
		Knowledge: KnowledgeContext{
			ServiceInventoryPath: "docs/SERVICE_INVENTORY.md",
			ServiceInventory:     "- Billing service: `lib/billing-service.js`",
			Facts:                []KnowledgeFact{{Source: ".beads/knowledge/gotchas.jsonl", Text: "Use the existing Billing service instead of creating parallel billing paths."}},
		},
		EvidenceText: "bash tests/run-all.sh exited 0",
	})
	assertFinding(t, duplicate, "architecture-reviewer", "high", "Possible duplicate code path")
}

func TestTaskDoneReviewersAllowCleanValidatedChanges(t *testing.T) {
	findings := RunTaskDone(Context{
		Task: TaskContext{Type: "beads-task", ID: "task-1", Text: "Acceptance: parse JSON safely"},
		Git: GitContext{
			ChangedFiles: []string{"lib/safe.js", "tests/lib/test-safe.sh"},
			Diff:         "+module.exports = input => JSON.parse(input);\n",
		},
		Knowledge:    KnowledgeContext{},
		EvidenceText: "bash tests/run-all.sh exited 0",
	})
	if len(findings) != 0 {
		t.Fatalf("clean review should not produce findings: %+v", findings)
	}
}

func TestTaskDoneReviewersAcceptStructuredValidationReceipt(t *testing.T) {
	findings := RunTaskDone(Context{
		Task: TaskContext{Type: "beads-task", ID: "task-1", Text: "Acceptance: parse JSON safely"},
		Git: GitContext{
			ChangedFiles: []string{"lib/safe.js"},
			Diff:         "+module.exports = input => JSON.parse(input);\n",
		},
		EvidenceText: `{"schemaVersion":1,"kind":"validation","command":["go","test","./..."],"exitCode":0,"summary":"go test ./... exited 0"}`,
	})
	if len(findings) != 0 {
		t.Fatalf("structured validation receipt should satisfy validation: %+v", findings)
	}
}

func TestTaskDoneReviewersRejectFailedStructuredValidationReceipt(t *testing.T) {
	findings := RunTaskDone(Context{
		Task: TaskContext{Type: "beads-task", ID: "task-1", Text: "Acceptance: parse JSON safely"},
		Git: GitContext{
			ChangedFiles: []string{"lib/safe.js"},
			Diff:         "+module.exports = input => JSON.parse(input);\n",
		},
		EvidenceText: `{"schemaVersion":1,"kind":"validation","command":["go","test","./..."],"exitCode":1,"summary":"tests passed before final failure"}`,
	})
	assertFinding(t, findings, "test-reviewer", "high", "Missing test changes")
}

func TestTaskDoneReviewersRejectMalformedStructuredValidationReceipt(t *testing.T) {
	findings := RunTaskDone(Context{
		Task: TaskContext{Type: "beads-task", ID: "task-1", Text: "Acceptance: parse JSON safely"},
		Git: GitContext{
			ChangedFiles: []string{"lib/safe.js"},
			Diff:         "+module.exports = input => JSON.parse(input);\n",
		},
		EvidenceText: `{"schemaVersion":1,"kind":"validation","summary":"missing exitCode defaults to zero"}`,
	})
	assertFinding(t, findings, "test-reviewer", "high", "Missing test changes")
}

func assertFinding(t *testing.T, findings []Finding, reviewer, severity, titlePart string) {
	t.Helper()
	for _, finding := range findings {
		if finding.Reviewer == reviewer && finding.Severity == severity && strings.Contains(finding.Title, titlePart) {
			return
		}
	}
	t.Fatalf("missing finding reviewer=%s severity=%s title~=%s in %+v", reviewer, severity, titlePart, findings)
}
