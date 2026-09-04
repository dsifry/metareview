package findings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCountByClassSeparatesBlockersAdvisoriesFollowUpsAndWarnings(t *testing.T) {
	records := []Record{
		{Classification: "spec-contract", Severity: "medium"},
		{Classification: "advisory", Severity: "medium"},
		{Classification: "follow-up", Severity: "low"},
		{Classification: "novel", Severity: "high"},
	}
	counts := CountByClass(records)
	if counts.Blocking != 1 || counts.Advisory != 1 || counts.FollowUp != 1 || counts.Warnings != 1 {
		t.Fatalf("unexpected counts: %+v", counts)
	}
}

func TestReconcileTracksOpenFindingsAcrossAncestorChain(t *testing.T) {
	root := t.TempDir()
	target := map[string]string{"type": "beads-task", "id": "task-1"}
	runA := Run{ID: "mrv-a", Scope: "task-done", Target: target, RepoRoot: root, GitHead: "aaa"}
	blocker := unsafeEval("eval is introduced.")
	blocker.Fingerprint = "security:eval:lib/example.js"
	if _, err := Reconcile(root, runA, []Input{blocker}, Options{}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	runB := Run{ID: "mrv-b", Scope: "task-done", Target: target, RepoRoot: root, GitHead: "bbb"}
	result, err := Reconcile(root, runB, []Input{blocker}, Options{PreviousRunID: "mrv-a", PreviousRunIDs: []string{"mrv-a"}})
	if err != nil {
		t.Fatalf("reconcile repeat run: %v", err)
	}
	if len(result.OpenFindings) != 1 || result.OpenBlockingCount != 1 {
		t.Fatalf("repeated open finding should remain unresolved: %+v", result)
	}

	runC := Run{ID: "mrv-c", Scope: "task-done", Target: target, RepoRoot: root, GitHead: "ccc"}
	result, err = Reconcile(root, runC, nil, Options{PreviousRunID: "mrv-b", PreviousRunIDs: []string{"mrv-a", "mrv-b"}})
	if err != nil {
		t.Fatalf("reconcile closure run: %v", err)
	}
	if result.OpenBlockingCount != 0 || len(result.OpenFindings) != 0 {
		t.Fatalf("ancestor finding should close when absent from current run: %+v", result)
	}

	records := readRecords(t, root)
	if !hasRecord(records, "mrvf-a-001", "fixed") {
		t.Fatalf("ancestor finding should be marked fixed: %+v", records)
	}
}

func TestAllReturnsEveryStatus(t *testing.T) {
	root := t.TempDir()
	if got, err := All(root); err != nil || len(got) != 0 {
		t.Fatalf("empty ledger: got %d records, err %v", len(got), err)
	}
	target := map[string]string{"type": "beads-task", "id": "task-1"}
	run := Run{ID: "mrv-a", Scope: "task-done", Target: target, RepoRoot: root, GitHead: "aaa"}
	if _, err := Reconcile(root, run, []Input{unsafeEval("eval is introduced.")}, Options{}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := GrantOverride(root, "mrvf-a-001", OverrideGrant{By: "boss", Reason: "accepted for release", Now: "2026-09-04T00:00:00Z"}); err != nil {
		t.Fatalf("grant override: %v", err)
	}
	all, err := All(root)
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(all) != 1 || all[0].Status != StatusOverridden {
		t.Fatalf("All should return the overridden record regardless of status: %+v", all)
	}
	// An overridden finding is not an unresolved blocker, so All and
	// UnresolvedBlocking must disagree — proving All is not just the blocker set.
	blockers, err := UnresolvedBlocking(root)
	if err != nil {
		t.Fatalf("UnresolvedBlocking: %v", err)
	}
	if len(blockers) != 0 {
		t.Fatalf("overridden finding should not be an unresolved blocker: %+v", blockers)
	}
}

func TestReconcileReturnsOpenFindingsForCurrentTarget(t *testing.T) {
	root := t.TempDir()
	target := map[string]string{"type": "beads-task", "id": "task-1"}
	run := Run{ID: "mrv-a", Scope: "task-done", Target: target, RepoRoot: root, GitHead: "aaa"}
	blocker := unsafeEval("eval is introduced.")
	blocker.Fingerprint = "security:eval:lib/example.js"
	result, err := Reconcile(root, run, []Input{blocker}, Options{})
	if err != nil {
		t.Fatalf("reconcile first run: %v", err)
	}
	if len(result.OpenFindings) != 1 || result.OpenFindings[0].Status != "open" {
		t.Fatalf("current target open findings should be returned: %+v", result)
	}
}

func TestReconcileKeepsSameHeadOpenFindingsWithoutPreviousRun(t *testing.T) {
	root := t.TempDir()
	target := map[string]string{"type": "beads-task", "id": "task-1"}
	runA := Run{ID: "mrv-a", Scope: "task-done", Target: target, RepoRoot: root, GitHead: "aaa"}
	if _, err := Reconcile(root, runA, []Input{unsafeEval("eval is introduced.")}, Options{}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	runB := Run{ID: "mrv-b", Scope: "task-done", Target: target, RepoRoot: root, GitHead: "aaa"}
	result, err := Reconcile(root, runB, nil, Options{})
	if err != nil {
		t.Fatalf("reconcile same-head fresh run: %v", err)
	}
	if result.OpenBlockingCount != 1 {
		t.Fatalf("same-head fresh run should not clear open blockers, got %+v", result)
	}
}

func TestReconcileKeepsDifferentHeadOpenFindingsWithoutResetRun(t *testing.T) {
	root := t.TempDir()
	target := map[string]string{"type": "beads-task", "id": "task-1"}
	runA := Run{ID: "mrv-a", Scope: "task-done", Target: target, RepoRoot: root, GitHead: "aaa"}
	if _, err := Reconcile(root, runA, []Input{unsafeEval("eval is introduced.")}, Options{}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	runB := Run{ID: "mrv-b", Scope: "task-done", Target: target, RepoRoot: root, GitHead: "bbb"}
	result, err := Reconcile(root, runB, nil, Options{})
	if err != nil {
		t.Fatalf("reconcile changed-head fresh run: %v", err)
	}
	if result.OpenBlockingCount != 1 {
		t.Fatalf("changed-head fresh run without reset should keep old blockers open: %+v", result)
	}
}

func TestReconcileClosesExplicitResetRunFindingsAtDifferentHead(t *testing.T) {
	root := t.TempDir()
	target := map[string]string{"type": "beads-task", "id": "task-1"}
	runA := Run{ID: "mrv-a", Scope: "task-done", Target: target, RepoRoot: root, GitHead: "aaa"}
	if _, err := Reconcile(root, runA, []Input{unsafeEval("eval is introduced.")}, Options{}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	runB := Run{ID: "mrv-b", Scope: "task-done", Target: target, RepoRoot: root, GitHead: "bbb"}
	result, err := Reconcile(root, runB, nil, Options{ResetRunIDs: []string{"mrv-a"}})
	if err != nil {
		t.Fatalf("reconcile reset run: %v", err)
	}
	if result.OpenBlockingCount != 0 || len(result.OpenFindings) != 0 {
		t.Fatalf("explicit changed-head reset should clear absent old blockers: %+v", result)
	}
	if !hasRecord(readRecords(t, root), "mrvf-a-001", "fixed") {
		t.Fatalf("old finding should be fixed after explicit changed-head reset")
	}
}

func TestReconcileDoesNotResetDifferentScopeSameTarget(t *testing.T) {
	root := t.TempDir()
	target := map[string]string{"type": "path", "id": "docs/spec.md"}
	runA := Run{ID: "mrv-a", Scope: "task-done", Target: target, RepoRoot: root, GitHead: "aaa"}
	if _, err := Reconcile(root, runA, []Input{unsafeEval("eval is introduced.")}, Options{}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	runB := Run{ID: "mrv-b", Scope: "epic-ready", Target: target, RepoRoot: root, GitHead: "bbb"}
	result, err := Reconcile(root, runB, nil, Options{ResetRunIDs: []string{"mrv-a"}})
	if err != nil {
		t.Fatalf("reconcile cross-scope reset: %v", err)
	}
	if result.OpenBlockingCount != 0 {
		t.Fatalf("different scope run should not inherit blocker count: %+v", result)
	}
	if !hasRecord(readRecords(t, root), "mrvf-a-001", "open") {
		t.Fatalf("different scope reset should not close original finding")
	}
}

func TestReconcileUpdatesRepeatedOpenFindingHead(t *testing.T) {
	root := t.TempDir()
	target := map[string]string{"type": "beads-task", "id": "task-1"}
	runA := Run{ID: "mrv-a", Scope: "task-done", Target: target, RepoRoot: root, GitHead: "aaa"}
	blocker := unsafeEval("eval is introduced.")
	if _, err := Reconcile(root, runA, []Input{blocker}, Options{}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	runB := Run{ID: "mrv-b", Scope: "task-done", Target: target, RepoRoot: root, GitHead: "bbb"}
	if _, err := Reconcile(root, runB, []Input{blocker}, Options{ResetRunIDs: []string{"mrv-a"}}); err != nil {
		t.Fatalf("reconcile repeated finding: %v", err)
	}
	records := readRecords(t, root)
	if len(records) != 1 || records[0].GitHead != "bbb" || records[0].RunID != "mrv-a" {
		t.Fatalf("repeated open finding should update last-seen head without duplicating: %+v", records)
	}

	runC := Run{ID: "mrv-c", Scope: "task-done", Target: target, RepoRoot: root, GitHead: "bbb"}
	result, err := Reconcile(root, runC, nil, Options{ResetRunIDs: []string{"mrv-a"}})
	if err != nil {
		t.Fatalf("reconcile same-head reset: %v", err)
	}
	if result.OpenBlockingCount != 1 {
		t.Fatalf("same-head reset should keep finding open after repeated observation: %+v", result)
	}
}

func TestReconcileClosesOriginalFindingFromEscalatedResetChain(t *testing.T) {
	root := t.TempDir()
	target := map[string]string{"type": "beads-task", "id": "task-1"}
	blocker := unsafeEval("eval is introduced.")
	runA := Run{ID: "mrv-a", Scope: "task-done", Target: target, RepoRoot: root, GitHead: "aaa"}
	if _, err := Reconcile(root, runA, []Input{blocker}, Options{}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	runB := Run{ID: "mrv-b", Scope: "task-done", Target: target, RepoRoot: root, GitHead: "aaa"}
	if _, err := Reconcile(root, runB, []Input{blocker}, Options{PreviousRunID: "mrv-a", PreviousRunIDs: []string{"mrv-a"}}); err != nil {
		t.Fatalf("reconcile second attempt: %v", err)
	}
	runC := Run{ID: "mrv-c", Scope: "task-done", Target: target, RepoRoot: root, GitHead: "aaa"}
	if _, err := Reconcile(root, runC, []Input{blocker}, Options{PreviousRunID: "mrv-b", PreviousRunIDs: []string{"mrv-a", "mrv-b"}}); err != nil {
		t.Fatalf("reconcile escalated attempt: %v", err)
	}

	runD := Run{ID: "mrv-d", Scope: "task-done", Target: target, RepoRoot: root, GitHead: "bbb"}
	result, err := Reconcile(root, runD, nil, Options{ResetRunIDs: []string{"mrv-a", "mrv-b", "mrv-c"}})
	if err != nil {
		t.Fatalf("reconcile reset attempt: %v", err)
	}
	if result.OpenBlockingCount != 0 {
		t.Fatalf("reset chain should close original finding when absent at new head: %+v", result)
	}
	if !hasRecord(readRecords(t, root), "mrvf-a-001", "fixed") {
		t.Fatalf("original finding should be fixed after reset chain")
	}
}

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

func TestLegacyContextRiskRowSuperseded(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		scope       string
		fingerprint string
		options     Options
	}{
		{"task-done unchained", "task-done", "architecture:context-risk:DIFF_TRUNCATED|LARGE_DIFF", Options{}},
		{"pr-ready unchained", "pr-ready", "pr:architecture:context-risk:DIFF_TRUNCATED", Options{}},
		{"epic-ready escalated", "epic-ready", "epic:context-risk:LARGE_DIFF", Options{ResetRunIDs: []string{"mrv-escalated"}}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, ".metareview", "findings.jsonl")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			legacy := Record{
				SchemaVersion:  1,
				ID:             "mrvf-legacy-1",
				RunID:          "mrv-escalated",
				Scope:          testCase.scope,
				Status:         "open",
				Classification: "blocking",
				Severity:       "high",
				Fingerprint:    testCase.fingerprint,
				Target:         map[string]string{"type": "task", "id": "t-1"},
			}
			data, err := json.Marshal(legacy)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
				t.Fatal(err)
			}

			run := Run{ID: "mrv-new", Scope: testCase.scope, Target: map[string]string{"type": "task", "id": "t-1"}, RepoRoot: root}
			result, err := Reconcile(root, run, nil, testCase.options)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.OpenFindings) != 0 {
				t.Fatalf("a superseded row must not stay open: %+v", result.OpenFindings)
			}
			records, err := readJSONL(path)
			if err != nil {
				t.Fatal(err)
			}
			if len(records) != 1 || records[0].Status != StatusSuperseded {
				t.Fatalf("records = %+v, want one superseded row", records)
			}
			if records[0].FixedInRunID != "" {
				t.Fatalf("fixedInRunId must stay empty, got %q", records[0].FixedInRunID)
			}
			if _, err := os.Stat(path + ".pre-0.8.3.bak"); err != nil {
				t.Fatalf("the ledger must be backed up before the alias pass: %v", err)
			}
		})
	}
}

func TestSupersedeLeavesUnrelatedRowsAlone(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".metareview", "findings.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	keep := Record{SchemaVersion: 1, ID: "mrvf-keep", RunID: "mrv-1", Scope: "task-done", Status: "open",
		Classification: "blocking", Severity: "high", Fingerprint: "security:eval",
		Target: map[string]string{"type": "task", "id": "t-1"}}
	other := Record{SchemaVersion: 1, ID: "mrvf-other", RunID: "mrv-1", Scope: "task-done", Status: "open",
		Classification: "blocking", Severity: "high", Fingerprint: "architecture:context-risk:LARGE_DIFF",
		Target: map[string]string{"type": "task", "id": "t-2"}}
	var lines []byte
	for _, record := range []Record{keep, other} {
		data, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		lines = append(append(lines, data...), '\n')
	}
	if err := os.WriteFile(path, lines, 0o644); err != nil {
		t.Fatal(err)
	}

	run := Run{ID: "mrv-2", Scope: "task-done", Target: map[string]string{"type": "task", "id": "t-1"}, RepoRoot: root}
	if _, err := Reconcile(root, run, nil, Options{}); err != nil {
		t.Fatal(err)
	}
	records, err := readJSONL(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if record.Status != "open" {
			t.Fatalf("unrelated rows must stay open: %+v", record)
		}
	}
	if _, err := os.Stat(path + ".pre-0.8.3.bak"); !os.IsNotExist(err) {
		t.Fatal("no backup should be taken when nothing is superseded")
	}
}

func TestReadersAcceptOneMiBLines(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".metareview", "findings.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	long := Record{SchemaVersion: 1, ID: "mrvf-long", RunID: "mrv-1", Scope: "task-done", Status: "open",
		Classification: "blocking", Severity: "high", Fingerprint: "security:eval",
		Found:  strings.Repeat("x", 300_000),
		Target: map[string]string{"type": "task", "id": "t-1"}}
	data, err := json.Marshal(long)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) <= 64*1024 {
		t.Fatalf("fixture line is only %d bytes; it must exceed bufio's default", len(data))
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	records, err := readJSONL(path)
	if err != nil {
		t.Fatalf("a 1 MiB line must be readable: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
}

// TestReadJSONLAcceptsExactlyMaxLine pins the boundary the constant documents:
// bufio rejects a token equal to the buffer maximum, so a record of exactly
// maxJSONLLineBytes is only readable when the buffer is one byte larger.
func TestReadJSONLAcceptsExactlyMaxLine(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".metareview", "findings.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	record := Record{SchemaVersion: 1, ID: "mrvf-max", RunID: "mrv-1", Scope: "task-done", Status: "open",
		Classification: "blocking", Severity: "high", Fingerprint: "security:eval",
		Target: map[string]string{"type": "task", "id": "t-1"}}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	// Grow Found until the encoded line is exactly maxJSONLLineBytes. The first
	// pass sizes it roughly; the second corrects for the added field's own bytes.
	pad := maxJSONLLineBytes - len(data)
	for i := 0; i < 2; i++ {
		record.Found = strings.Repeat("x", pad)
		data, err = json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		pad += maxJSONLLineBytes - len(data)
	}
	if len(data) != maxJSONLLineBytes {
		t.Fatalf("fixture line is %d bytes, want exactly %d", len(data), maxJSONLLineBytes)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	records, err := readJSONL(path)
	if err != nil {
		t.Fatalf("a line of exactly %d bytes must be readable: %v", maxJSONLLineBytes, err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
}

// TestReadJSONLAcceptsExactlyMaxLineCRLF pins the same boundary for a
// CRLF-terminated file. bufio.ScanLines drops the trailing \r from the token,
// but the carriage return still has to fit in the buffer alongside the record,
// so an exact-limit CRLF line needs two bytes of headroom rather than one.
// readJSONL already trims the \r, so CRLF input is expected to work.
func TestReadJSONLAcceptsExactlyMaxLineCRLF(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".metareview", "findings.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	record := Record{SchemaVersion: 1, ID: "mrvf-max-crlf", RunID: "mrv-1", Scope: "task-done",
		Status: "open", Classification: "blocking", Severity: "high", Fingerprint: "security:eval",
		Target: map[string]string{"type": "task", "id": "t-1"}}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	pad := maxJSONLLineBytes - len(data)
	for i := 0; i < 2; i++ {
		record.Found = strings.Repeat("x", pad)
		data, err = json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		pad += maxJSONLLineBytes - len(data)
	}
	if len(data) != maxJSONLLineBytes {
		t.Fatalf("fixture line is %d bytes, want exactly %d", len(data), maxJSONLLineBytes)
	}
	if err := os.WriteFile(path, append(data, '\r', '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	records, err := readJSONL(path)
	if err != nil {
		t.Fatalf("an exact-limit CRLF line must be readable: %v", err)
	}
	if len(records) != 1 || records[0].ID != "mrvf-max-crlf" {
		t.Fatalf("records = %+v, want the exact-limit row", records)
	}
}
