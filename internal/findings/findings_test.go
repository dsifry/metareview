package findings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReconcileFindingsLifecycle(t *testing.T) {
	root := t.TempDir()
	target := map[string]string{"type": "beads-task", "id": "task-1"}
	runA := Run{ID: "mrv-a", Scope: "task-done", Target: target, RepoRoot: root, GitHead: "aaa"}

	result, err := Reconcile(root, runA, []Input{unsafeEval("eval is introduced.")}, Options{})
	if err != nil {
		t.Fatalf("reconcile first run: %v", err)
	}
	if result.OpenBlockingCount != 1 {
		t.Fatalf("first run should block, got %d", result.OpenBlockingCount)
	}
	if len(result.Findings) != 1 || result.Findings[0].ID != "mrvf-a-001" {
		t.Fatalf("unexpected first finding result: %+v", result.Findings)
	}

	runB := Run{ID: "mrv-b", Scope: "task-done", Target: target, RepoRoot: root, GitHead: "bbb"}
	result, err = Reconcile(root, runB, nil, Options{PreviousRunID: "mrv-a"})
	if err != nil {
		t.Fatalf("reconcile fixed run: %v", err)
	}
	if result.OpenBlockingCount != 0 {
		t.Fatalf("fixed rerun should not block, got %d", result.OpenBlockingCount)
	}
	records := readRecords(t, root)
	if !hasRecord(records, "mrvf-a-001", "fixed") {
		t.Fatalf("previous finding should be fixed: %+v", records)
	}
	index := mustRead(t, filepath.Join(root, "docs", "metareview", "FINDINGS.md"))
	if !strings.Contains(index, "No unresolved findings recorded yet.") {
		t.Fatalf("index should clear fixed finding: %s", index)
	}

	runC := Run{ID: "mrv-c", Scope: "task-done", Target: target, RepoRoot: root, GitHead: "ccc"}
	result, err = Reconcile(root, runC, []Input{unsafeEval("eval is still introduced.")}, Options{})
	if err != nil {
		t.Fatalf("reconcile recurrence: %v", err)
	}
	if result.OpenBlockingCount != 1 || len(result.Findings) != 1 || result.Findings[0].ID != "mrvf-c-001" {
		t.Fatalf("recurrence should create a new open blocker: %+v", result)
	}

	runD := Run{ID: "mrv-d", Scope: "task-done", Target: target, RepoRoot: root, GitHead: "ddd"}
	result, err = Reconcile(root, runD, []Input{unsafeEval("eval remains.")}, Options{PreviousRunID: "mrv-c"})
	if err != nil {
		t.Fatalf("reconcile repeated open finding: %v", err)
	}
	if result.OpenBlockingCount != 1 {
		t.Fatalf("repeated unresolved finding should still block, got %d", result.OpenBlockingCount)
	}
	if len(result.Findings) != 1 || result.Findings[0].ID != "mrvf-c-001" {
		t.Fatalf("repeated unresolved finding should be returned for review log rendering: %+v", result.Findings)
	}

	if err := RenderIndex(root); err != nil {
		t.Fatalf("render index: %v", err)
	}
	index = mustRead(t, filepath.Join(root, "docs", "metareview", "FINDINGS.md"))
	if !strings.Contains(index, "mrvf-c-001") {
		t.Fatalf("unresolved repeated finding should remain in index: %s", index)
	}
	blockers, err := UnresolvedBlocking(root)
	if err != nil {
		t.Fatalf("unresolved blocking: %v", err)
	}
	if len(blockers) != 1 {
		t.Fatalf("expected one unresolved blocker, got %d", len(blockers))
	}
}

func TestRecordsUseDesignSpecSchemaFields(t *testing.T) {
	root := t.TempDir()
	run := Run{ID: "mrv-schema", Scope: "task-done", Target: map[string]string{"type": "path", "path": "docs/task.md"}, RepoRoot: root, GitHead: "abc"}
	if _, err := Reconcile(root, run, []Input{unsafeEval("eval is introduced.")}, Options{}); err != nil {
		t.Fatalf("reconcile schema run: %v", err)
	}
	records := readRecords(t, root)
	if len(records) != 1 {
		t.Fatalf("expected one record, got %d", len(records))
	}
	record := records[0]
	if record.SchemaVersion != 1 || record.RunID != "mrv-schema" || record.Status != "open" || record.Owner != "implementer" {
		t.Fatalf("missing required schema fields: %+v", record)
	}
	if record.BeadsFollowupID != nil {
		t.Fatalf("expected nil beads followup id, got %+v", record.BeadsFollowupID)
	}
	if record.CreatedAt == "" || record.UpdatedAt == "" || record.RepoRoot != root || record.GitHead != "abc" {
		t.Fatalf("missing provenance fields: %+v", record)
	}
	if len(record.Evidence) != 1 || record.Evidence[0].Type != "file-line" || record.Fingerprint == "" {
		t.Fatalf("missing evidence/fingerprint fields: %+v", record)
	}
}

func TestSpecContractFindingsBlockRegardlessOfSeverity(t *testing.T) {
	root := t.TempDir()
	run := Run{ID: "mrv-contract", Scope: "task-done", Target: map[string]string{"type": "path", "path": "docs/task.md"}, RepoRoot: root, GitHead: "abc"}
	input := unsafeEval("Required acceptance evidence is missing.")
	input.Severity = "medium"
	input.Classification = "spec-contract"
	input.Fingerprint = "contract:missing-acceptance"

	result, err := Reconcile(root, run, []Input{input}, Options{})
	if err != nil {
		t.Fatalf("reconcile spec-contract run: %v", err)
	}
	if result.OpenBlockingCount != 1 {
		t.Fatalf("spec-contract finding should block regardless of severity, got %d", result.OpenBlockingCount)
	}
}

func unsafeEval(finding string) Input {
	return Input{
		Reviewer:       "security-reviewer",
		Severity:       "high",
		Classification: "blocking",
		Title:          "Unsafe eval",
		Finding:        finding,
		Expected:       "Input is parsed without code execution.",
		Found:          "eval(userInput)",
		Evidence:       []Evidence{{Type: "file-line", Path: "lib/example.js", Line: 4}},
		Recommendation: "Remove eval.",
		Fingerprint:    "security:eval:lib/example.js",
	}
}

func readRecords(t *testing.T, root string) []Record {
	t.Helper()
	path := filepath.Join(root, ".metareview", "findings.jsonl")
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var records []Record
	for _, line := range strings.Split(strings.TrimSpace(string(bytes)), "\n") {
		var record Record
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	return records
}

func hasRecord(records []Record, id, status string) bool {
	for _, record := range records {
		if record.ID == id && record.Status == status {
			return true
		}
	}
	return false
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(bytes)
}
