package reviewstate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/findings"
	"github.com/dsifry/metareview/internal/reviewlog"
)

func TestTargetKey(t *testing.T) {
	if got := TargetKey("pr-ready", map[string]string{"type": "branch", "id": "feature"}); got != "pr-ready:branch:feature" {
		t.Fatalf("unexpected target key: %s", got)
	}
	if got := TargetKey("artifact", map[string]string{"type": "path", "path": "docs/spec.md"}); got != "artifact:path:docs/spec.md" {
		t.Fatalf("unexpected path target key: %s", got)
	}
}

func TestProjectReadsRepositoryReviewState(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "docs", "metareview", "reviews", "artifact.md"),
		"# metareview: artifact review\n\n"+
			"Run ID: `mrv-artifact`\n\n"+
			"Target: `docs/spec.md`\n\n"+
			"## Verdict\n\nNOT_REVIEWED\n")
	writeFile(t, filepath.Join(root, ".metareview", "findings.jsonl"),
		`{"schemaVersion":1,"id":"mrvf-path-001","runId":"mrv-artifact","status":"open","classification":"blocking","severity":"high","target":{"type":"path","path":"docs/spec.md"}}`+"\n")

	projection, err := Project(root, Options{
		Scope:        "pr-ready",
		Target:       map[string]string{"type": "branch", "id": "feature"},
		ChangedPaths: []string{"lib/parser.js"},
	})
	if err != nil {
		t.Fatalf("project repository state: %v", err)
	}

	if projection.TargetKey() != "pr-ready:branch:feature" {
		t.Fatalf("unexpected target key: %s", projection.TargetKey())
	}
	if len(projection.CurrentBlockers()) != 0 {
		t.Fatalf("expected unrelated path blocker to be historical: %+v", projection.CurrentBlockers())
	}
	if len(projection.HistoricalUnrelated()) != 1 || projection.HistoricalUnrelated()[0].RunID != "mrv-artifact" {
		t.Fatalf("expected artifact log to be historical: %+v", projection.HistoricalUnrelated())
	}
	if len(projection.HistoricalBlockers()) != 1 || projection.HistoricalBlockers()[0].ID != "mrvf-path-001" {
		t.Fatalf("expected path blocker to be historical: %+v", projection.HistoricalBlockers())
	}
}

func TestProjectResolvesPreviousRunChainFromRepositoryState(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".metareview", "runs.jsonl"),
		`{"id":"mrv-root","scope":"pr-ready","target":{"type":"branch","id":"feature"},"verdict":"NEEDS_REVISION","attemptNumber":1,"maxAttempts":3}`+"\n"+
			`{"id":"mrv-leaf","scope":"pr-ready","target":{"type":"branch","id":"feature"},"verdict":"NEEDS_REVISION","previousRunId":"mrv-root","attemptNumber":2,"maxAttempts":3}`+"\n")
	writeFile(t, filepath.Join(root, "docs", "metareview", "reviews", "root.md"),
		"# metareview: pr-ready review\n\n"+
			"Run ID: `mrv-root`\n\n"+
			"Target: `feature`\n\n"+
			"## Verdict\n\nNEEDS_REVISION\n")
	writeFile(t, filepath.Join(root, "docs", "metareview", "reviews", "leaf.md"),
		"# metareview: pr-ready review\n\n"+
			"Run ID: `mrv-leaf`\n\n"+
			"Target: `feature`\n\n"+
			"## Verdict\n\nNEEDS_REVISION\n")
	writeFile(t, filepath.Join(root, ".metareview", "findings.jsonl"),
		`{"schemaVersion":1,"id":"mrvf-root-001","runId":"mrv-root","status":"open","classification":"blocking","severity":"high","target":{"type":"branch","id":"feature"}}`+"\n"+
			`{"schemaVersion":1,"id":"mrvf-leaf-001","runId":"mrv-leaf","status":"open","classification":"blocking","severity":"high","target":{"type":"branch","id":"feature"}}`+"\n")

	projection, err := Project(root, Options{
		Scope:         "pr-ready",
		Target:        map[string]string{"type": "branch", "id": "feature"},
		PreviousRunID: "mrv-leaf",
	})
	if err != nil {
		t.Fatalf("project previous chain: %v", err)
	}

	if len(projection.CurrentBlockers()) != 0 {
		t.Fatalf("expected previous-chain blockers to be superseded: %+v", projection.CurrentBlockers())
	}
	if !projection.SupersededRunIDs()["mrv-root"] || !projection.SupersededRunIDs()["mrv-leaf"] {
		t.Fatalf("expected previous-chain run IDs to be superseded: %+v", projection.SupersededRunIDs())
	}
	if !projection.SupersededFindingIDs()["mrvf-root-001"] || !projection.SupersededFindingIDs()["mrvf-leaf-001"] {
		t.Fatalf("expected previous-chain finding IDs to be superseded: %+v", projection.SupersededFindingIDs())
	}
}

func TestProjectRejectsMismatchedPreviousRunTarget(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".metareview", "runs.jsonl"),
		`{"id":"mrv-root","scope":"pr-ready","target":{"type":"branch","id":"other"},"verdict":"NEEDS_REVISION","attemptNumber":1,"maxAttempts":3}`+"\n")

	_, err := Project(root, Options{
		Scope:         "pr-ready",
		Target:        map[string]string{"type": "branch", "id": "feature"},
		PreviousRunID: "mrv-root",
	})
	if err == nil || !strings.Contains(err.Error(), "does not match pr-ready feature") {
		t.Fatalf("expected target mismatch error, got %v", err)
	}
}

func TestProjectFiltersPreviousRunChainState(t *testing.T) {
	logs := []reviewlog.Summary{
		{RunID: "mrv-old", Target: "codex/issue-2", Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
		{RunID: "mrv-other", Target: "task-2", Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
	}
	blockers := []findings.Record{
		{ID: "mrvf-old-001", RunID: "mrv-old", Status: "open", Classification: "blocking", Severity: "high"},
		{ID: "mrvf-other-001", RunID: "mrv-other", Status: "open", Classification: "blocking", Severity: "high"},
	}

	projection := ProjectRecords(logs, blockers, Options{PreviousRunIDs: []string{"mrv-old"}})

	if len(projection.CurrentReviewLogs()) != 1 || projection.CurrentReviewLogs()[0].RunID != "mrv-other" {
		t.Fatalf("expected only non-previous review log to remain current: %+v", projection.CurrentReviewLogs())
	}
	if len(projection.CurrentBlockers()) != 1 || projection.CurrentBlockers()[0].RunID != "mrv-other" {
		t.Fatalf("expected only non-previous blocker to remain current: %+v", projection.CurrentBlockers())
	}
	if !projection.SupersededRunIDs()["mrv-old"] || !projection.SupersededFindingIDs()["mrvf-old-001"] {
		t.Fatalf("expected previous run and finding to be marked superseded: %+v", projection)
	}
}

// LogBlocks is the ONE shared blocking predicate (dedup + gate). An ESCALATED verdict blocks even with no
// recorded open-blocker flag (a hard stop must never be skipped), open blockers block, and a clean PASS does not.
func TestLogBlocks(t *testing.T) {
	cases := []struct {
		name string
		log  reviewlog.Summary
		want bool
	}{
		{"escalated without findings still blocks", reviewlog.Summary{Verdict: "ESCALATED"}, true},
		{"escalated is case-insensitive", reviewlog.Summary{Verdict: "escalated"}, true},
		{"open blockers block", reviewlog.Summary{Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true}, true},
		{"clean pass does not block", reviewlog.Summary{Verdict: "PASS"}, false},
		{"clean with no verdict does not block", reviewlog.Summary{}, false},
	}
	for _, c := range cases {
		if got := LogBlocks(c.log); got != c.want {
			t.Fatalf("%s: LogBlocks=%v, want %v", c.name, got, c.want)
		}
	}
}

// Re-running the SAME review over the SAME (kind, target, head) must not stack duplicate blockers (issue
// #97): only the LATEST run is current; earlier same-head re-runs are superseded, so `review pr-ready` run
// three times renders the branch as one blocker, not three.
func TestProjectDedupsSameHeadReruns(t *testing.T) {
	head := "abc123"
	logs := []reviewlog.Summary{
		{RunID: "mrv-1", Kind: "pr-ready", Target: "current branch", HeadSHA: head, Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
		{RunID: "mrv-3", Kind: "pr-ready", Target: "current branch", HeadSHA: head, Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
		{RunID: "mrv-2", Kind: "pr-ready", Target: "current branch", HeadSHA: head, Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
	}
	blockers := []findings.Record{
		{ID: "mrvf-1", RunID: "mrv-1", Status: "open", Classification: "blocking", Severity: "high", GitHead: head, Target: map[string]any{"type": "branch", "id": "feature"}},
		{ID: "mrvf-2", RunID: "mrv-2", Status: "open", Classification: "blocking", Severity: "high", GitHead: head, Target: map[string]any{"type": "branch", "id": "feature"}},
		{ID: "mrvf-3", RunID: "mrv-3", Status: "open", Classification: "blocking", Severity: "high", GitHead: head, Target: map[string]any{"type": "branch", "id": "feature"}},
	}

	projection := ProjectRecords(logs, blockers, Options{CurrentTarget: map[string]string{"type": "branch", "id": "feature"}})

	if n := len(projection.CurrentReviewLogs()); n != 1 || projection.CurrentReviewLogs()[0].RunID != "mrv-3" {
		t.Fatalf("only the latest same-head run must remain current; got %d: %+v", n, projection.CurrentReviewLogs())
	}
	if !projection.SupersededRunIDs()["mrv-1"] || !projection.SupersededRunIDs()["mrv-2"] {
		t.Fatalf("earlier same-head re-runs must be superseded: %+v", projection.SupersededRunIDs())
	}
	if n := len(projection.CurrentBlockers()); n != 1 || projection.CurrentBlockers()[0].RunID != "mrv-3" {
		t.Fatalf("only the latest same-head run's blocker must remain current; got %d: %+v", n, projection.CurrentBlockers())
	}
	if !projection.SupersededFindingIDs()["mrvf-1"] || !projection.SupersededFindingIDs()["mrvf-2"] {
		t.Fatalf("earlier same-head findings must be superseded: %+v", projection.SupersededFindingIDs())
	}
}

// FALSE-CLEAR GUARD (#97 review): a later CLEAN re-run over the SAME commit must NOT supersede an earlier
// BLOCKING run. The code is byte-identical, so a clean second look is a reviewer miss (adversarial reviews
// are non-deterministic), never a fix — the blocker must survive. Only a later BLOCKING run may retire it.
func TestProjectCleanRerunDoesNotSupersedeSameHeadBlocker(t *testing.T) {
	head := "abc123"
	logs := []reviewlog.Summary{
		{RunID: "mrv-1", Kind: "pr-ready", Target: "current branch", HeadSHA: head, Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
		{RunID: "mrv-2", Kind: "pr-ready", Target: "current branch", HeadSHA: head, Verdict: "PASS"}, // later, CLEAN, no blockers
	}
	blockers := []findings.Record{
		{ID: "mrvf-1", RunID: "mrv-1", Status: "open", Classification: "blocking", Severity: "high", GitHead: head, Target: map[string]any{"type": "branch", "id": "feature"}},
	}

	projection := ProjectRecords(logs, blockers, Options{CurrentTarget: map[string]string{"type": "branch", "id": "feature"}})

	if projection.SupersededRunIDs()["mrv-1"] {
		t.Fatal("a later CLEAN same-head re-run must NOT supersede an earlier BLOCKING run (false-CLEAR)")
	}
	if n := len(projection.CurrentBlockers()); n != 1 || projection.CurrentBlockers()[0].RunID != "mrv-1" {
		t.Fatalf("the earlier blocker must stay current; got %d: %+v", n, projection.CurrentBlockers())
	}
}

// A later CLEAN re-run must not supersede an earlier ESCALATED run over the same commit — an escalation is a
// hard stop, and dedup is verdict-aware so it treats ESCALATED as blocking even if HasUnresolvedBlockers is unset.
func TestProjectCleanRerunDoesNotSupersedeSameHeadEscalation(t *testing.T) {
	head := "abc123"
	logs := []reviewlog.Summary{
		{RunID: "mrv-1", Kind: "pr-ready", Target: "current branch", HeadSHA: head, Verdict: "ESCALATED"},
		{RunID: "mrv-2", Kind: "pr-ready", Target: "current branch", HeadSHA: head, Verdict: "PASS"},
	}

	projection := ProjectRecords(logs, nil, Options{CurrentTarget: map[string]string{"type": "branch", "id": "feature"}})

	if projection.SupersededRunIDs()["mrv-1"] {
		t.Fatal("a later CLEAN same-head re-run must NOT supersede an earlier ESCALATED run")
	}
}

// Two CLEAN re-runs over the same head collapse to the latest (harmless — neither blocks), so the projection
// carries one current log, not two. Exercises the clean-run supersede path.
func TestProjectDedupsSameHeadCleanReruns(t *testing.T) {
	head := "abc123"
	logs := []reviewlog.Summary{
		{RunID: "mrv-1", Kind: "pr-ready", Target: "current branch", HeadSHA: head, Verdict: "PASS"},
		{RunID: "mrv-2", Kind: "pr-ready", Target: "current branch", HeadSHA: head, Verdict: "PASS"},
	}

	projection := ProjectRecords(logs, nil, Options{CurrentTarget: map[string]string{"type": "branch", "id": "feature"}})

	if !projection.SupersededRunIDs()["mrv-1"] {
		t.Fatal("the earlier clean same-head re-run should be superseded by the latest")
	}
	if n := len(projection.CurrentReviewLogs()); n != 1 || projection.CurrentReviewLogs()[0].RunID != "mrv-2" {
		t.Fatalf("only the latest clean run remains current; got %d: %+v", n, projection.CurrentReviewLogs())
	}
}

// Different HEADS are NOT deduped — a fix loop reviews a new commit each attempt, and staleness of an old
// head is handled elsewhere (HistoricalRunIDs). Same-head dedup must not collapse across commits.
func TestProjectDoesNotDedupAcrossHeads(t *testing.T) {
	logs := []reviewlog.Summary{
		{RunID: "mrv-1", Kind: "pr-ready", Target: "current branch", HeadSHA: "head-a", Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
		{RunID: "mrv-2", Kind: "pr-ready", Target: "current branch", HeadSHA: "head-b", Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
	}

	projection := ProjectRecords(logs, nil, Options{CurrentTarget: map[string]string{"type": "branch", "id": "feature"}})

	if len(projection.CurrentReviewLogs()) != 2 {
		t.Fatalf("logs at different heads must both remain current (no cross-head dedup): %+v", projection.CurrentReviewLogs())
	}
}

// A log with no HeadSHA (a legacy log that predates the field) must NOT be deduped — we cannot tell whether
// two headless logs are the same review, so collapsing them could hide a live blocker.
func TestProjectDoesNotDedupHeadlessLogs(t *testing.T) {
	logs := []reviewlog.Summary{
		{RunID: "mrv-1", Kind: "pr-ready", Target: "current branch", Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
		{RunID: "mrv-2", Kind: "pr-ready", Target: "current branch", Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
	}

	projection := ProjectRecords(logs, nil, Options{CurrentTarget: map[string]string{"type": "branch", "id": "feature"}})

	if len(projection.CurrentReviewLogs()) != 2 {
		t.Fatalf("headless legacy logs must not be deduped: %+v", projection.CurrentReviewLogs())
	}
}

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyPreviousRunIDsRecoversChainFromLogs(t *testing.T) {
	logs := []reviewlog.Summary{
		{RunID: "mrv-root", Kind: "pr-ready", Target: "current branch", Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
		{RunID: "mrv-leaf", Kind: "pr-ready", Target: "current branch", Verdict: "NEEDS_REVISION", PreviousRunID: "mrv-root", HasUnresolvedBlockers: true},
	}

	ids := LegacyPreviousRunIDs(logs, "mrv-leaf")

	if len(ids) != 2 || ids[0] != "mrv-root" || ids[1] != "mrv-leaf" {
		t.Fatalf("expected root-to-leaf legacy chain IDs, got %+v", ids)
	}
}

func TestProjectDoesNotApplyUnvalidatedLegacyPreviousRunID(t *testing.T) {
	logs := []reviewlog.Summary{
		{RunID: "mrv-task", Kind: "task-done", Target: "task-1", Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
	}
	blockers := []findings.Record{
		{ID: "mrvf-task-001", RunID: "mrv-task", Status: "open", Classification: "blocking", Severity: "high"},
	}

	projection := ProjectRecords(logs, blockers, Options{})

	if len(projection.CurrentReviewLogs()) != 1 || projection.CurrentReviewLogs()[0].RunID != "mrv-task" {
		t.Fatalf("projector should not filter legacy logs without validated previous IDs: %+v", projection.CurrentReviewLogs())
	}
	if len(projection.CurrentBlockers()) != 1 || projection.CurrentBlockers()[0].RunID != "mrv-task" {
		t.Fatalf("projector should not filter legacy blockers without validated previous IDs: %+v", projection.CurrentBlockers())
	}
}

func TestProjectTreatsUnrelatedArtifactLogAsHistorical(t *testing.T) {
	logs := []reviewlog.Summary{
		{RunID: "mrv-artifact", Kind: "artifact", Target: "docs/spec.md", Verdict: "NOT_REVIEWED", HasUnresolvedBlockers: true},
	}

	projection := ProjectRecords(logs, nil, Options{ChangedPaths: []string{"lib/parser.js"}})

	if len(projection.CurrentReviewLogs()) != 0 {
		t.Fatalf("unrelated artifact log should not remain current: %+v", projection.CurrentReviewLogs())
	}
	if len(projection.HistoricalUnrelated()) != 1 || projection.HistoricalUnrelated()[0].RunID != "mrv-artifact" {
		t.Fatalf("expected unrelated artifact to be historical: %+v", projection.HistoricalUnrelated())
	}
}

func TestProjectTreatsArtifactLogAsHistoricalWhenNoPathsReviewed(t *testing.T) {
	logs := []reviewlog.Summary{
		{RunID: "mrv-artifact", Kind: "artifact", Target: "docs/spec.md", Verdict: "NOT_REVIEWED", HasUnresolvedBlockers: true},
	}

	projection := ProjectRecords(logs, nil, Options{})

	if len(projection.CurrentReviewLogs()) != 0 {
		t.Fatalf("artifact log should not block when no reviewed path overlaps it: %+v", projection.CurrentReviewLogs())
	}
	if len(projection.HistoricalUnrelated()) != 1 || projection.HistoricalUnrelated()[0].RunID != "mrv-artifact" {
		t.Fatalf("expected artifact log to be historical: %+v", projection.HistoricalUnrelated())
	}
}

func TestProjectTreatsBlockersFromUnrelatedArtifactRunAsHistorical(t *testing.T) {
	logs := []reviewlog.Summary{
		{RunID: "mrv-artifact", Kind: "artifact", Target: "docs/spec.md", Verdict: "NOT_REVIEWED", HasUnresolvedBlockers: true},
	}
	blockers := []findings.Record{
		{ID: "mrvf-artifact-001", RunID: "mrv-artifact", Status: "open", Classification: "blocking", Severity: "high"},
		{ID: "mrvf-ambiguous-001", RunID: "mrv-ambiguous", Status: "open", Classification: "blocking", Severity: "high"},
	}

	projection := ProjectRecords(logs, blockers, Options{ChangedPaths: []string{"lib/parser.js"}})

	if len(projection.CurrentBlockers()) != 1 || projection.CurrentBlockers()[0].RunID != "mrv-ambiguous" {
		t.Fatalf("expected only ambiguous blocker to remain current: %+v", projection.CurrentBlockers())
	}
	if projection.SupersededFindingIDs()["mrvf-artifact-001"] {
		t.Fatalf("unrelated historical blocker should not be marked fixed/superseded: %+v", projection.SupersededFindingIDs())
	}
}

func TestProjectTreatsUnrelatedPathBlockerWithoutLogAsHistorical(t *testing.T) {
	blockers := []findings.Record{
		{ID: "mrvf-path-001", RunID: "mrv-path", Status: "open", Classification: "blocking", Severity: "high", Target: map[string]any{"type": "path", "path": "docs/spec.md"}},
		{ID: "mrvf-ambiguous-001", RunID: "mrv-ambiguous", Status: "open", Classification: "blocking", Severity: "high"},
	}

	projection := ProjectRecords(nil, blockers, Options{ChangedPaths: []string{"lib/parser.js"}})

	if len(projection.CurrentBlockers()) != 1 || projection.CurrentBlockers()[0].RunID != "mrv-ambiguous" {
		t.Fatalf("expected only ambiguous blocker to remain current: %+v", projection.CurrentBlockers())
	}
	if len(projection.HistoricalBlockers()) != 1 || projection.HistoricalBlockers()[0].RunID != "mrv-path" {
		t.Fatalf("expected path blocker to be historical: %+v", projection.HistoricalBlockers())
	}
}

func TestProjectKeepsRelevantPathBlockerCurrent(t *testing.T) {
	blockers := []findings.Record{
		{ID: "mrvf-path-001", RunID: "mrv-path", Status: "open", Classification: "blocking", Severity: "high", Target: map[string]any{"type": "path", "path": "lib/parser.js"}},
	}

	projection := ProjectRecords(nil, blockers, Options{ChangedPaths: []string{"lib/parser.js"}})

	if len(projection.CurrentBlockers()) != 1 || projection.CurrentBlockers()[0].RunID != "mrv-path" {
		t.Fatalf("expected relevant path blocker to remain current: %+v", projection.CurrentBlockers())
	}
}

func TestProjectTreatsMismatchedBranchRunAsHistorical(t *testing.T) {
	logs := []reviewlog.Summary{
		{RunID: "mrv-branch-a", Kind: "pr-ready", Target: "current branch", Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
	}
	blockers := []findings.Record{
		{ID: "mrvf-branch-a-001", RunID: "mrv-branch-a", Status: "open", Classification: "blocking", Severity: "high", Target: map[string]any{"type": "branch", "id": "branch-a"}},
		{ID: "mrvf-task-001", RunID: "mrv-task", Status: "open", Classification: "blocking", Severity: "high", Target: map[string]any{"type": "beads-task", "id": "task-1"}},
	}

	projection := ProjectRecords(logs, blockers, Options{
		HistoricalRunIDs: []string{"mrv-branch-a"},
		CurrentTarget:    map[string]string{"type": "branch", "id": "branch-b"},
	})

	if len(projection.CurrentReviewLogs()) != 0 {
		t.Fatalf("mismatched branch review log should not remain current: %+v", projection.CurrentReviewLogs())
	}
	if len(projection.CurrentBlockers()) != 1 || projection.CurrentBlockers()[0].RunID != "mrv-task" {
		t.Fatalf("expected task blocker to remain current and branch blocker historical: %+v", projection.CurrentBlockers())
	}
	if len(projection.HistoricalBlockers()) != 1 || projection.HistoricalBlockers()[0].RunID != "mrv-branch-a" {
		t.Fatalf("expected branch blocker to be historical: %+v", projection.HistoricalBlockers())
	}
}

func TestProjectTreatsMismatchedBranchBlockerAsHistoricalWithoutLog(t *testing.T) {
	blockers := []findings.Record{
		{ID: "mrvf-branch-a-001", RunID: "mrv-branch-a", Status: "open", Classification: "blocking", Severity: "high", Target: map[string]any{"type": "branch", "id": "branch-a"}},
	}

	projection := ProjectRecords(nil, blockers, Options{CurrentTarget: map[string]string{"type": "branch", "id": "branch-b"}})

	if len(projection.CurrentBlockers()) != 0 {
		t.Fatalf("mismatched branch blocker should not remain current: %+v", projection.CurrentBlockers())
	}
	if len(projection.HistoricalBlockers()) != 1 {
		t.Fatalf("expected branch blocker to be historical: %+v", projection.HistoricalBlockers())
	}
}

func TestProjectKeepsRelevantArtifactLogCurrent(t *testing.T) {
	logs := []reviewlog.Summary{
		{RunID: "mrv-artifact", Kind: "artifact", Target: "lib/parser.js", Verdict: "NOT_REVIEWED", HasUnresolvedBlockers: true},
	}

	projection := ProjectRecords(logs, nil, Options{ChangedPaths: []string{"lib/parser.js"}})

	if len(projection.CurrentReviewLogs()) != 1 || projection.CurrentReviewLogs()[0].RunID != "mrv-artifact" {
		t.Fatalf("expected changed-path artifact to remain current: %+v", projection.CurrentReviewLogs())
	}
}
