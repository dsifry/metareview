package reviewlog

import (
	"os"
	"path/filepath"
	"strings"
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

func TestPassReviewMentioningHistoricalNeedsRevisionIsNotUnresolved(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "task.md"),
		"# metareview: task-done review\n\n"+
			"Run ID: `mrv-task`\n\n"+
			"Target: `task-1`\n\n"+
			"## Verdict\n\nPASS\n\n"+
			"## Notes\n\nPrevious run mrv-old was NEEDS_REVISION; this run fixed it.\n\n"+
			"## Findings\n\nNo blocking findings.\n")

	logs, err := ForTarget(root, "task-1")
	if err != nil {
		t.Fatalf("target logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected one log, got %+v", logs)
	}
	if logs[0].HasUnresolvedBlockers {
		t.Fatalf("historical NEEDS_REVISION prose must not poison PASS: %+v", logs[0])
	}
}

func TestDiscoverParsesLegacyPreviousRunFromMarkdown(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "task.md"),
		"# metareview: pr-ready review\n\n"+
			"Run ID: `mrv-task`\n\n"+
			"Target: `current branch`\n\n"+
			"Context pack: `docs/metareview/context/mrv-task-context.md`\n\n"+
			"Previous run: `mrv-root`\n\n"+
			"## Verdict\n\nNEEDS_REVISION\n")

	logs, err := Discover(root)
	if err != nil {
		t.Fatalf("discover logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected one log, got %+v", logs)
	}
	if logs[0].PreviousRunID != "mrv-root" {
		t.Fatalf("expected previous run from Markdown, got %+v", logs[0])
	}
	if logs[0].ContextRel != "docs/metareview/context/mrv-task-context.md" {
		t.Fatalf("expected context pack from Markdown, got %+v", logs[0])
	}
}

func TestEscalatedVerdictIsUnresolved(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "task.md"), reviewMarkdown("mrv-task", "task-1", "ESCALATED", ""))

	logs, err := ForTarget(root, "task-1")
	if err != nil {
		t.Fatalf("target logs: %v", err)
	}
	if len(logs) != 1 || !logs[0].HasUnresolvedBlockers {
		t.Fatalf("expected ESCALATED to be unresolved: %+v", logs)
	}
}

func TestDiscoverMergesRunAttemptMetadata(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "task.md"), reviewMarkdown("mrv-task", "task-1", "ESCALATED", ""))
	mustWrite(t, filepath.Join(root, ".metareview", "runs.jsonl"), `{"id":"mrv-root","scope":"task-done","target":{"type":"path","id":"task-1"},"verdict":"NEEDS_REVISION","attemptNumber":1,"maxAttempts":3}`+"\n"+
		`{"id":"mrv-task","scope":"task-done","target":{"type":"path","id":"task-1"},"verdict":"ESCALATED","previousRunId":"mrv-root","attemptNumber":3,"maxAttempts":3,"blockingFindingCount":1,"advisoryFindingCount":2,"followUpFindingCount":1}`+"\n")

	logs, err := ForTarget(root, "task-1")
	if err != nil {
		t.Fatalf("target logs: %v", err)
	}
	log := logs[0]
	if log.AttemptNumber != 3 || log.MaxAttempts != 3 || log.BlockingFindingCount != 1 || log.AdvisoryFindingCount != 2 || log.FollowUpFindingCount != 1 {
		t.Fatalf("expected run metadata merged into summary: %+v", log)
	}
	if len(log.RunChain) != 2 || log.RunChain[0].ID != "mrv-root" || log.RunChain[1].ID != "mrv-task" {
		t.Fatalf("expected full run chain in summary: %+v", log.RunChain)
	}
}

func TestDiscoverSurfacesUnknownClassificationWarning(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "task.md"), reviewMarkdown("mrv-task", "task-1", "PASS_ADVISORY", ""))
	mustWrite(t, filepath.Join(root, ".metareview", "runs.jsonl"), `{"id":"mrv-task","attemptNumber":1,"maxAttempts":3,"warningFindingCount":1}`+"\n")

	logs, err := ForTarget(root, "task-1")
	if err != nil {
		t.Fatalf("target logs: %v", err)
	}
	if len(logs[0].Warnings) != 1 || !strings.Contains(logs[0].Warnings[0], "unknown finding classification") {
		t.Fatalf("expected unknown-classification warning in review log summary: %+v", logs[0].Warnings)
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
