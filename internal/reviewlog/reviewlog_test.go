package reviewlog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverReviewLogsDeterministically(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "b.md"), reviewMarkdown("mrv-b", "task-2", "NEEDS_REVISION", "mrvf-b-001"))
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "a.md"), reviewMarkdown("mrv-a", "task-1", "PASS", ""))
	mustWrite(t, filepath.Join(root, ".metareview", "findings.jsonl"), `{"id":"mrvf-b-001","runId":"mrv-b","status":"open","classification":"blocking","severity":"high","title":"Blocked","target":{"id":"task-2","type":"beads-task"}}`+"\n")

	logs, err := Discover(root)
	if err != nil {
		t.Fatalf("discover logs: %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("expected 2 logs, got %d", len(logs))
	}
	if logs[0].Path != "docs/metareview/reviews/a.md" || logs[1].Path != "docs/metareview/reviews/b.md" {
		t.Fatalf("logs not sorted by path: %+v", logs)
	}
	if logs[0].RunID != "mrv-a" || logs[0].Target != "task-1" || logs[0].Verdict != "PASS" {
		t.Fatalf("unexpected first log summary: %+v", logs[0])
	}
	if !logs[1].HasUnresolvedBlockers || len(logs[1].FindingIDs) != 1 || logs[1].FindingIDs[0] != "mrvf-b-001" {
		t.Fatalf("expected unresolved blocker summary: %+v", logs[1])
	}
}

func TestForTargetIncludesFindingsStateEvenWhenMarkdownOmitsIDs(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "task.md"), reviewMarkdown("mrv-task", "task-1", "NEEDS_REVISION", ""))
	mustWrite(t, filepath.Join(root, ".metareview", "findings.jsonl"), `{"id":"mrvf-task-001","runId":"mrv-task","status":"open","classification":"blocking","severity":"critical","title":"Unsafe","target":{"id":"task-1","type":"beads-task"}}`+"\n")

	logs, err := ForTarget(root, "task-1")
	if err != nil {
		t.Fatalf("target logs: %v", err)
	}
	if len(logs) != 1 || !logs[0].HasUnresolvedBlockers || logs[0].FindingIDs[0] != "mrvf-task-001" {
		t.Fatalf("expected blocker from findings state: %+v", logs)
	}
}

func TestArtifactNotReviewedIsUnresolved(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "artifact.md"), artifactReviewMarkdown("mrv-artifact", "docs/spec.md", "NOT_REVIEWED", nil))

	logs, err := ForTarget(root, "docs/spec.md")
	if err != nil {
		t.Fatalf("target logs: %v", err)
	}
	if len(logs) != 1 || !logs[0].HasUnresolvedBlockers {
		t.Fatalf("expected NOT_REVIEWED artifact to be unresolved: %+v", logs)
	}
}

func TestArtifactMissingRequiredReviewerRowsIsUnresolved(t *testing.T) {
	root := t.TempDir()
	rows := []string{
		"| Feasibility | PASS | 0 | 0 | ok |",
		"| Completeness | PASS | 0 | 0 | ok |",
	}
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "artifact.md"), artifactReviewMarkdown("mrv-artifact", "docs/spec.md", "PASS", rows))

	logs, err := ForTarget(root, "docs/spec.md")
	if err != nil {
		t.Fatalf("target logs: %v", err)
	}
	if len(logs) != 1 || !logs[0].HasUnresolvedBlockers {
		t.Fatalf("expected missing reviewer rows to be unresolved: %+v", logs)
	}
}

func TestCompletedArtifactReviewIsNotUnresolved(t *testing.T) {
	root := t.TempDir()
	rows := []string{
		"| Feasibility | PASS | 0 | 0 | ok |",
		"| Completeness | PASS | 0 | 0 | ok |",
		"| Scope and alignment | PASS | 0 | 0 | ok |",
		"| Architecture | PASS | 0 | 0 | ok |",
		"| Intent preservation | PASS | 0 | 0 | ok |",
	}
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "artifact.md"), artifactReviewMarkdown("mrv-artifact", "docs/spec.md", "PASS", rows))

	logs, err := ForTarget(root, "docs/spec.md")
	if err != nil {
		t.Fatalf("target logs: %v", err)
	}
	if len(logs) != 1 || logs[0].HasUnresolvedBlockers {
		t.Fatalf("expected completed artifact review not to be unresolved: %+v", logs)
	}
}

func reviewMarkdown(runID, target, verdict, findingID string) string {
	finding := "No blocking findings.\n"
	if findingID != "" {
		finding = "### " + findingID + ": Blocked\n"
	}
	return "# metareview: task-done review\n\n" +
		"Run ID: `" + runID + "`\n\n" +
		"Target: `" + target + "`\n\n" +
		"## Verdict\n\n" + verdict + "\n\n" +
		"## Findings\n\n" + finding
}

func artifactReviewMarkdown(runID, target, verdict string, rows []string) string {
	table := "| Reviewer | Verdict | Blocking | Warnings | Notes |\n| --- | --- | ---: | ---: | --- |\n"
	for _, row := range rows {
		table += row + "\n"
	}
	return "# metareview: artifact review\n\n" +
		"Run ID: `" + runID + "`\n\n" +
		"Target: `" + target + "`\n\n" +
		"## Verdict\n\n" + verdict + "\n\n" +
		"## Reviewer Results\n\n" + table + "\n" +
		"## Findings\n\nNo blocking findings.\n"
}

func mustWrite(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
