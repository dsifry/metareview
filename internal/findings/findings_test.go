package findings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
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

// Issue #147: the push gate needs a read-only view of the ledger to reconcile
// log-level blockers. Load is that view: the records on disk, missing file = empty.
func TestLoadReturnsRecordsAndTreatsMissingAsEmpty(t *testing.T) {
	root := t.TempDir()
	records, err := Load(root)
	if err != nil {
		t.Fatalf("Load on a repo with no ledger: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("Load = %d records; want empty", len(records))
	}
	one := Record{ID: "mrvf-1", Scope: "pr-ready", Status: StatusOverridden, Classification: "blocking", Severity: "high"}
	if err := writeJSONL(filepath.Join(root, ".metareview", "findings.jsonl"), []Record{one}); err != nil {
		t.Fatal(err)
	}
	records, err = Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(records) != 1 || records[0].ID != "mrvf-1" {
		t.Fatalf("Load = %+v; want the written row", records)
	}
}

// IsResolvedTerminal is the allowlist the reconciliation consumers gate on: only the
// three recognized terminal values resolve; everything else — typos, empty, unknown
// future values — is unvouched and must keep a log blocking (issue #147 review).
func TestIsResolvedTerminalIsAnAllowlist(t *testing.T) {
	yes := []string{"fixed", StatusOverridden, StatusSuperseded}
	no := []string{"", "open", StatusOverridePending, "overridn", "resolved-ish", "FIXED"}
	for _, s := range yes {
		if !IsResolvedTerminal(s) {
			t.Errorf("IsResolvedTerminal(%q) = false; want true", s)
		}
	}
	for _, s := range no {
		if IsResolvedTerminal(s) {
			t.Errorf("IsResolvedTerminal(%q) = true; want false", s)
		}
	}
}

// The committed docs/metareview/FINDINGS.md is the durable, shared audit trail; the local
// .metareview/findings.jsonl is per-worktree transient state. Before the carry-over fix
// (issue #151), a gate run in a fresh worktree rendered the index purely from its empty
// local ledger and rewrote the committed file to "No unresolved findings recorded yet.",
// destroying granted-override provenance and open blockers recorded elsewhere. These tests
// pin the three properties the fix exists for.
func TestRenderPreservesCommittedLinesTheLedgerDoesNotKnow(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "docs", "metareview", "FINDINGS.md")
	committed := `# metareview Findings

- mrvf-20260908-x-001 [high] No adjudicated lens review recorded (adversarial-review-reviewer)

## Process Overrides

Deliberate exceptions to the review workflow. Pending entries still block CI.

- mrvf-20260903-y-001 [granted] Adversarial review was in-session-emulated — granted by agent-session-140 (shared-ledger residue cleanup per issue #138) at 2026-09-07T17:58:40Z: Historical advisory from a prior session's branch.
`
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(committed), 0o644); err != nil {
		t.Fatal(err)
	}

	// A fresh worktree's ledger is EMPTY: the render must carry both committed lines
	// forward verbatim, not collapse to the no-findings sentinel.
	if err := RenderIndexWithRecords(root, nil); err != nil {
		t.Fatalf("render: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	g := string(got)
	if !strings.Contains(g, "- mrvf-20260908-x-001 [high] No adjudicated lens review recorded") {
		t.Errorf("committed open blocker destroyed by an empty local ledger:\n%s", g)
	}
	if !strings.Contains(g, "- mrvf-20260903-y-001 [granted] Adversarial review was in-session-emulated") {
		t.Errorf("committed override provenance destroyed by an empty local ledger:\n%s", g)
	}

	// Once the ledger KNOWS a finding (any status — here a fixed one), it renders from the
	// ledger and suppresses the committed line: fresh local knowledge wins.
	fixed := Record{ID: "mrvf-20260908-x-001", Status: "fixed", Severity: "high", Title: "No adjudicated lens review recorded", Reviewer: "adversarial-review-reviewer"}
	if err := RenderIndexWithRecords(root, []Record{fixed}); err != nil {
		t.Fatalf("render with known record: %v", err)
	}
	got, _ = os.ReadFile(path)
	g = string(got)
	if strings.Contains(g, "mrvf-20260908-x-001") {
		t.Errorf("a finding the ledger knows must render from the ledger, not carry over:\n%s", g)
	}
	if !strings.Contains(g, "mrvf-20260903-y-001") {
		t.Errorf("the override the ledger still does not know must survive:\n%s", g)
	}
}

func TestRenderFreshRepositoryUnchanged(t *testing.T) {
	root := t.TempDir()
	// no committed index at all — the first render must behave exactly as before
	if err := RenderIndexWithRecords(root, nil); err != nil {
		t.Fatalf("render: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(root, "docs", "metareview", "FINDINGS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(got)) != "# metareview Findings\n\nNo unresolved findings recorded yet." {
		t.Errorf("fresh render changed: %q", string(got))
	}
}

func TestCarryOverLinesSplitsSectionsAndSkipsKnown(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "docs", "metareview", "FINDINGS.md")
	doc := `# metareview Findings

- mrvf-a-001 [high] known blocker (reviewer)
- mrvf-a-002 [high] unknown blocker (reviewer)

## Process Overrides

Deliberate exceptions to the review workflow. Pending entries still block CI.

- mrvf-a-001 [granted] known override — granted by someone at some time: reason
- mrvf-a-003 [pending] unknown override — requested by someone at some time: reason

## Stale

- mrvf-a-004 [low] bullet under a later section — must NOT be carried at all
`
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	blockers, overrides := carryOverLines(raw, map[string]bool{"mrvf-a-001": true})
	if len(blockers) != 1 || !strings.Contains(blockers[0], "mrvf-a-002") {
		t.Errorf("blockers carry-over = %v, want only the unknown mrvf-a-002", blockers)
	}
	if len(overrides) != 1 || !strings.Contains(overrides[0], "mrvf-a-003") {
		t.Errorf("overrides carry-over = %v, want only the unknown mrvf-a-003", overrides)
	}
	// a later ## section's bullets are NOT carried at all — verbatim preservation of a
	// line is not preservation of its meaning (a Stale/history section must not be
	// promoted to current blockers)
	if len(blockers) != 1 || strings.Contains(strings.Join(blockers, "\n"), "mrvf-a-004") {
		t.Errorf("blockers carry-over = %v, want only mrvf-a-002; post-section bullets must not carry", blockers)
	}
	// an ABSENT committed index is no information, not an error — the READ layer's job now;
	// the parser just sees no bytes
	b, o := carryOverLines(nil, nil)
	if b != nil || o != nil {
		t.Errorf("no committed bytes must carry nothing, got %v %v", b, o)
	}
}

// The read-error half of the fail-closed rule: a committed index that EXISTS but cannot be
// read (permissions, transient I/O) must abort the render, because proceeding would
// overwrite the committed audit trail with the local-only view — the issue-#151 destruction,
// re-opened on the error path (caught in adversarial review of this fix).
func TestCarryOverFailsClosedOnUnreadableCommittedIndex(t *testing.T) {
	// The 0o000 trick only makes the file unreadable as an unprivileged POSIX process: as
	// root or on Windows it stays readable, the render legitimately succeeds, and the test
	// would fail against correct code (the repo's established guard — epicsource/status
	// use the same one).
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permission-based unreadability does not apply on windows or as root")
	}
	root := t.TempDir()
	path := filepath.Join(root, "docs", "metareview", "FINDINGS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# metareview Findings\n\n- mrvf-x-001 [high] a committed blocker (r)\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
	if err := RenderIndexWithRecords(root, nil); err == nil {
		t.Fatal("an unreadable committed index must fail the render, not overwrite the file")
	}
	if err := os.Chmod(path, 0o644); err != nil { // restore so the untouched-content check can read it
		t.Fatal(err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "# metareview Findings\n\n- mrvf-x-001 [high] a committed blocker (r)\n" {
		t.Errorf("the unreadable committed index must be left untouched, got %q", string(got))
	}
}

// The end-to-end regression for issue #151: Reconcile — the caller every gate actually
// drives — run in a fresh worktree (empty local ledger) against a pre-seeded committed
// FINDINGS.md must not destroy the committed provenance. Every other Reconcile test runs in
// a bare TempDir with no committed index, so this is the only test that exercises the
// caller-level path the clobber actually rode.
func TestReconcileInFreshWorktreePreservesCommittedIndex(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "docs", "metareview", "FINDINGS.md")
	committed := "# metareview Findings\n\n" +
		"- mrvf-20260908-060804291514000-task-done-kind-b7bb121c-001 [high] No adjudicated lens review recorded (adversarial-review-reviewer)\n\n" +
		"## Process Overrides\n\n" +
		"Deliberate exceptions to the review workflow. Pending entries still block CI.\n\n" +
		"- mrvf-20260903-233630855862000-pr-ready-branch-10d735e5-001 [granted] Adversarial review was in-session-emulated — granted by agent-session-140 at 2026-09-07T17:58:40Z: Historical advisory from a prior session's branch.\n"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(committed), 0o644); err != nil {
		t.Fatal(err)
	}

	run := Run{ID: "mrv-fresh", Scope: "pr-ready", Target: map[string]string{"type": "branch", "id": "feature-x"}, RepoRoot: root, GitHead: "fff"}
	local := unsafeEval("eval introduced by this run.")
	local.Fingerprint = "security:eval:lib/fresh.js"
	if _, err := Reconcile(root, run, []Input{local}, Options{}); err != nil {
		t.Fatalf("reconcile in fresh worktree: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	g := string(got)
	if !strings.Contains(g, "mrvf-20260908-060804291514000-task-done-kind-b7bb121c-001") {
		t.Errorf("committed open blocker destroyed by a fresh-worktree gate run:\n%s", g)
	}
	if !strings.Contains(g, "mrvf-20260903-233630855862000-pr-ready-branch-10d735e5-001 [granted]") {
		t.Errorf("committed override provenance destroyed by a fresh-worktree gate run:\n%s", g)
	}
	if !strings.Contains(g, "mrvf-fresh") {
		t.Errorf("the fresh run's own finding must also render:\n%s", g)
	}
}

// Suppression must hold for the overridden status too, not just fixed: a record the ledger
// holds as StatusOverridden renders its override line from the ledger, and the committed
// line with the same ID must not ALSO carry — a failure here duplicates the Process
// Overrides entry (caught in adversarial review of this fix).
func TestRenderOverriddenLedgerRecordSuppressesItsCommittedOverrideLine(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "docs", "metareview", "FINDINGS.md")
	committed := "# metareview Findings\n\n" +
		"## Process Overrides\n\n" +
		"Deliberate exceptions to the review workflow. Pending entries still block CI.\n\n" +
		"- mrvf-o-001 [granted] committed render of the override — granted by old-session at 2026-09-01: reason\n"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(committed), 0o644); err != nil {
		t.Fatal(err)
	}
	ledger := Record{ID: "mrvf-o-001", Status: StatusOverridden, Title: "ledger render of the override",
		OverrideGrantedBy: "new-session", OverrideGrantedAt: "2026-09-08", OverrideGrantReason: "fresh local knowledge"}
	if err := RenderIndexWithRecords(root, []Record{ledger}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	g := string(got)
	if strings.Contains(g, "committed render of the override") {
		t.Errorf("a ledger-known overridden record must suppress its committed line:\n%s", g)
	}
	if !strings.Contains(g, "ledger render of the override") || !strings.Contains(g, "new-session") {
		t.Errorf("the ledger's own override line must render:\n%s", g)
	}
}

// The atomic-replace error paths: the render must fail (and clean up its temp file) when
// the temp write or the rename cannot complete, never leave a half-written committed index.
func TestWriteIndexAtomicFailsClosedAndCleansUp(t *testing.T) {
	// rename failure, portable: the target is a directory, so the temp write succeeds and
	// the rename onto it cannot.
	root := t.TempDir()
	path := filepath.Join(root, "FINDINGS.md")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeIndexAtomic(path, "# index"); err == nil {
		t.Fatal("a rename that cannot complete must fail the write")
	}
	leftovers, err := filepath.Glob(filepath.Join(root, ".FINDINGS.md.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(leftovers) != 0 {
		t.Errorf("the temp file must be cleaned up after a failed rename, got %v", leftovers)
	}

	// temp-write failure: the containing directory is read-only, so creating the temp file
	// fails (permission-based, unprivileged POSIX only).
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permission-based unreadability does not apply on windows or as root")
	}
	root2 := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root2, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(root2, "sub"), 0o555); err != nil {
		t.Fatal(err)
	}
	if err := writeIndexAtomic(filepath.Join(root2, "sub", "FINDINGS.md"), "# index"); err == nil {
		t.Fatal("a temp write that cannot complete must fail the write")
	}
	leftovers, err = filepath.Glob(filepath.Join(root2, "sub", ".FINDINGS.md.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(leftovers) != 0 {
		t.Errorf("no temp file may survive a failed write, got %v", leftovers)
	}
}

// The replacement preserves the committed index's mode (rename does not carry it), and a
// write-PROTECTED index — an operator's lock on the audit trail — fails closed instead of
// being silently replaced by a fresh writable file (the old in-place write failed EACCES).
func TestWriteIndexAtomicPreservesModeAndRespectsWriteProtection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits")
	}
	root := t.TempDir()
	path := filepath.Join(root, "FINDINGS.md")
	// 0o664, not 0o600: os.CreateTemp already creates the temp at 0600, so seeding 0600
	// passes even with the mode-preservation chmod deleted — the assertion must see a mode
	// the temp file never has on its own.
	if err := os.WriteFile(path, []byte("# old"), 0o644); err != nil {
		t.Fatal(err)
	}
	// chmod after creation: WriteFile's perm is masked by the process umask (0664 & ~022 =
	// 0644), which would quietly turn this back into a mode CreateTemp also produces.
	if err := os.Chmod(path, 0o664); err != nil {
		t.Fatal(err)
	}
	if err := writeIndexAtomic(path, "# new"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o664 {
		t.Errorf("replacement mode = %v, want the destination's 0664 preserved", info.Mode().Perm())
	}

	if os.Geteuid() == 0 {
		t.Skip("root ignores write-protection bits")
	}
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	if err := writeIndexAtomic(path, "# clobber"); err == nil {
		t.Fatal("a write-protected committed index must fail the render, not be replaced")
	}
	got, _ := os.ReadFile(path)
	if string(got) != "# new" {
		t.Errorf("write-protected index must be untouched, got %q", string(got))
	}
}

// Unique temp names make concurrent renders independent: two renders racing in one checkout
// (a gate run overlapping a Stop hook) must both succeed, and no fixed-name temp exists for
// one render's cleanup to delete the other's.
func TestWriteIndexAtomicConcurrentRendersAreIndependent(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "FINDINGS.md")
	if err := os.WriteFile(path, []byte("# seed"), 0o644); err != nil {
		t.Fatal(err)
	}
	// DISTINCT payloads per writer: with identical documents, a fixed shared-temp
	// implementation (the exact race this test exists to exclude) can still pass — every
	// interleaving of equal writes over one name is benign. Distinct payloads make the
	// interleavings observable.
	const n = 8
	docs := make([]string, n)
	for i := range docs {
		docs[i] = "# metareview Findings\n\nwriter " + strconv.Itoa(i) + "\n"
	}
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func(doc string) { errs <- writeIndexAtomic(path, doc) }(docs[i])
	}
	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent render failed: %v", err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	final := string(got)
	// every writer succeeded, so the file must be exactly one writer's document —
	// whole and uncorrupted, never a splice of two
	whole := false
	for i := range docs {
		if final == docs[i] {
			whole = true
			break
		}
	}
	if !whole {
		t.Errorf("final index is not any single writer's document (spliced?): %q", final)
	}
	leftovers, err := filepath.Glob(filepath.Join(root, ".FINDINGS.md.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(leftovers) != 0 {
		t.Errorf("temp files survived concurrent renders: %v", leftovers)
	}
}

// The injected-fault half of writeIndexAtomic's error surface: every seam failure must
// fail the write closed AND remove the temp file — a leftover would sit untracked in the
// committed docs/metareview tree until the next render, exactly the kind of file a careless
// `git add docs/metareview` sweeps into a commit.
func TestWriteIndexAtomicFaultInjectionCleansUp(t *testing.T) {
	for _, tc := range []struct {
		name   string
		inject func()
	}{
		{"write", func() { seamWriteString = func(*os.File, string) error { return fmt.Errorf("disk full") } }},
		{"sync", func() { seamSync = func(*os.File) error { return fmt.Errorf("sync failed") } }},
		{"chmod", func() { seamChmod = func(string, os.FileMode) error { return fmt.Errorf("chmod failed") } }},
		{"close", func() { seamClose = func(*os.File) error { return fmt.Errorf("close failed") } }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			savedWrite, savedSync, savedChmod, savedClose := seamWriteString, seamSync, seamChmod, seamClose
			defer func() {
				seamWriteString, seamSync, seamChmod, seamClose = savedWrite, savedSync, savedChmod, savedClose
			}()
			tc.inject()
			root := t.TempDir()
			path := filepath.Join(root, "FINDINGS.md")
			if err := os.WriteFile(path, []byte("# seed"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := writeIndexAtomic(path, "# new"); err == nil {
				t.Fatalf("%s fault must fail the write", tc.name)
			}
			leftovers, err := filepath.Glob(filepath.Join(root, ".FINDINGS.md.tmp-*"))
			if err != nil {
				t.Fatal(err)
			}
			if len(leftovers) != 0 {
				t.Errorf("%s fault left temp files behind: %v", tc.name, leftovers)
			}
			got, _ := os.ReadFile(path)
			if string(got) != "# seed" {
				t.Errorf("%s fault must leave the committed index untouched, got %q", tc.name, string(got))
			}
		})
	}
}

// A Stat failure that is NOT not-exist (here: ENOTDIR — the index's parent is a file) is an
// error, not "no committed index yet".
func TestWriteIndexAtomicFailsOnStrangeStatError(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "docs")
	if err := os.WriteFile(parent, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeIndexAtomic(filepath.Join(parent, "metareview", "FINDINGS.md"), "# x"); err == nil {
		t.Fatal("a non-notexist Stat error must fail the write")
	}
}

// fsync-before-rename is the durability guarantee the atomic replacement claims; only its
// failure branch is fault-injected, so deleting the sync call would leave the suite green.
// A spy around the real seamSync pins that a happy-path render actually syncs.
func TestWriteIndexAtomicSyncsBeforeRename(t *testing.T) {
	saved := seamSync
	defer func() { seamSync = saved }()
	synced := 0
	seamSync = func(f *os.File) error { synced++; return saved(f) }
	root := t.TempDir()
	path := filepath.Join(root, "FINDINGS.md")
	if err := os.WriteFile(path, []byte("# old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeIndexAtomic(path, "# new"); err != nil {
		t.Fatal(err)
	}
	if synced != 1 {
		t.Errorf("happy-path render synced %d times, want exactly 1", synced)
	}
}

// The carry-over parser and the render's own emitters are two hand-maintained descriptions
// of one committed-index format; if they drift apart (severity rendering, ID format, the
// "- ID [" prefix, the single-line canonical form), every carried line silently stops
// matching and the next render drops it — the issue-#151 destruction reopened on a
// format-drift path. This round-trip pins their agreement the way reviewlog's schema does:
// one owner, one round-trip test.
func TestCarryOverMatchesWhatTheRenderEmits(t *testing.T) {
	root := t.TempDir()
	records := []Record{
		{ID: "mrvf-rt-001", Status: "open", Severity: "high", Classification: "blocking",
			Title: "an open blocker", Reviewer: "security-reviewer"},
		{ID: "mrvf-rt-002", Status: StatusOverridden, Title: "an overridden finding",
			OverrideGrantedBy: "human", OverrideGrantedAt: "2026-09-08T00:00:00Z", OverrideGrantReason: "accepted risk"},
		{ID: "mrvf-rt-003", Status: StatusOverridePending, Title: "a pending override",
			OverrideRequestedBy: "agent", OverrideRequestedAt: "2026-09-08T00:00:00Z", OverrideRequestReason: "needs a human",
			OverrideEscalation: "an escalation that spans\ntwo physical lines"},
		// a free-text field with an embedded newline: the emitter must flatten it to one
		// physical line, or carry-over re-reads only the first line of a two-line entry
		{ID: "mrvf-rt-004", Status: "open", Severity: "critical", Classification: "blocking",
			Title: "a title that spans\ntwo physical lines", Reviewer: "reviewer"},
	}
	if err := RenderIndexWithRecords(root, records); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "docs", "metareview", "FINDINGS.md")

	// fresh worktree: nothing known — EVERY emitted line must carry back through the parser
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	blockers, overrides := carryOverLines(raw, map[string]bool{})
	if len(blockers) != 2 {
		t.Errorf("emitted blockers did not carry back: %v", blockers)
	}
	for _, id := range []string{"mrvf-rt-001", "mrvf-rt-004"} {
		if !strings.Contains(strings.Join(blockers, "\n"), id) {
			t.Errorf("blocker %s missing from carry-back: %v", id, blockers)
		}
	}
	if !strings.Contains(strings.Join(blockers, "\n"), "a title that spans two physical lines") {
		t.Errorf("the multi-line title was not flattened to the canonical single line: %v", blockers)
	}
	if !strings.Contains(strings.Join(overrides, "\n"), "an escalation that spans two physical lines") {
		t.Errorf("the multi-line escalation was not flattened to the canonical single line: %v", overrides)
	}
	// every emitted line must be exactly one physical line
	for _, l := range append(append([]string{}, blockers...), overrides...) {
		if strings.ContainsAny(l, "\n\r") {
			t.Errorf("emitted entry spans physical lines: %q", l)
		}
	}
	if len(overrides) != 2 {
		t.Errorf("emitted overrides did not carry back: %v", overrides)
	}
	for _, id := range []string{"mrvf-rt-002", "mrvf-rt-003"} {
		if !strings.Contains(strings.Join(overrides, "\n"), id) {
			t.Errorf("override %s missing from carry-back: %v", id, overrides)
		}
	}
}

// Hand-maintained committed sections the renderer does not emit (a history note, the
// shelved #93 "Stale" partition) must survive the rewrite — the render regenerates its own
// two sections and must not delete the rest of the committed file.
func TestRenderPreservesSectionsItDoesNotEmit(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "docs", "metareview", "FINDINGS.md")
	committed := "# metareview Findings\n\n" +
		"- mrvf-a-001 [high] an open blocker (reviewer)\n\n" +
		"## Process Overrides\n\n" +
		"Deliberate exceptions to the review workflow. Pending entries still block CI.\n\n" +
		"- mrvf-a-002 [granted] an override — granted by someone at some time: reason\n\n" +
		"## Stale\n\n" +
		"Findings from old heads, kept for the audit trail.\n\n" +
		"- mrvf-a-003 [medium] a stale-head finding (reviewer)\n"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(committed), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RenderIndexWithRecords(root, nil); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	g := string(got)
	if !strings.Contains(g, "## Stale") || !strings.Contains(g, "mrvf-a-003") {
		t.Errorf("a committed section the renderer does not emit was deleted:\n%s", g)
	}
	if !strings.Contains(g, "mrvf-a-001") || !strings.Contains(g, "mrvf-a-002") {
		t.Errorf("carried lines lost:\n%s", g)
	}
	// and the stale bullet is NOT promoted: it stays inside its own section, not the top
	topEnd := strings.Index(g, "## Process Overrides")
	if topEnd < 0 {
		t.Fatalf("the overrides header vanished from the render:\n%s", g)
	}
	if strings.Contains(g[:topEnd], "mrvf-a-003") {
		t.Errorf("stale-section bullet promoted to a current blocker:\n%s", g)
	}
}

func TestWriteIndexSeedIsAtomic(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "docs", "metareview", "FINDINGS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteIndexSeed(path); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# metareview Findings\n\nNo unresolved findings recorded yet.\n" {
		t.Errorf("seed content = %q", string(got))
	}
	leftovers, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".FINDINGS.md.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(leftovers) != 0 {
		t.Errorf("seed left temp files behind: %v", leftovers)
	}
}

// Rewrite stability (the property issue #151 is about): rendering twice over the file the
// first render wrote — carried lines, preserved sections and local records re-read from the
// new layout — must not duplicate a carried line or a preserved section.
func TestRenderIsIdempotentOverItsOwnOutput(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "docs", "metareview", "FINDINGS.md")
	committed := "# metareview Findings\n\n" +
		"- mrvf-idem-001 [high] an open blocker (reviewer)\n\n" +
		"## Process Overrides\n\n" +
		"Deliberate exceptions to the review workflow. Pending entries still block CI.\n\n" +
		"- mrvf-idem-002 [granted] an override — granted by someone at some time: reason\n\n" +
		"## Stale\n\n" +
		"- mrvf-idem-003 [medium] a stale-head finding (reviewer)\n"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(committed), 0o644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := RenderIndexWithRecords(root, nil); err != nil {
			t.Fatalf("render %d: %v", i, err)
		}
	}
	got, _ := os.ReadFile(path)
	g := string(got)
	for _, id := range []string{"mrvf-idem-001", "mrvf-idem-002", "mrvf-idem-003"} {
		if n := strings.Count(g, id); n != 1 {
			t.Errorf("%s appears %d times after two renders:\n%s", id, n, g)
		}
	}
	if n := strings.Count(g, "## Stale"); n != 1 {
		t.Errorf("the Stale section appears %d times after two renders:\n%s", n, g)
	}
}

// A hand-maintained section whose header merely STARTS with the overrides header must not
// be mistaken for it: its prose survives, its bullets are not promoted into the real
// Process Overrides section (the exact-header-match regression pin).
func TestNearMissOverrideHeaderIsItsOwnSection(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "docs", "metareview", "FINDINGS.md")
	committed := "# metareview Findings\n\n" +
		"- mrvf-nm-001 [high] an open blocker (reviewer)\n\n" +
		"## Process Overrides\n\n" +
		"Deliberate exceptions to the review workflow. Pending entries still block CI.\n\n" +
		"- mrvf-nm-002 [granted] a real override — granted by someone at some time: reason\n\n" +
		"## Process Overrides History\n\n" +
		"Hand-maintained history of overrides past.\n\n" +
		"- mrvf-nm-003 [granted] a historical override — granted by someone at some time: reason\n"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(committed), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RenderIndexWithRecords(root, nil); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	g := string(raw)
	if !strings.Contains(g, "## Process Overrides History") || !strings.Contains(g, "Hand-maintained history") {
		t.Errorf("the near-miss header section was deleted:\n%s", g)
	}
	// the history bullet must survive INSIDE its own section, not be promoted toward the
	// real overrides section
	histAt := strings.Index(g, "## Process Overrides History")
	realAt := strings.Index(g, "## Process Overrides\n")
	if realAt < 0 || histAt < 0 || histAt < realAt {
		t.Fatalf("section order unexpected:\n%s", g)
	}
	if strings.Contains(g[:histAt], "mrvf-nm-003") {
		t.Errorf("the history bullet was promoted toward the real overrides section:\n%s", g)
	}
	if !strings.Contains(g, "mrvf-nm-003") {
		t.Errorf("the history bullet must survive:\n%s", g)
	}
}
