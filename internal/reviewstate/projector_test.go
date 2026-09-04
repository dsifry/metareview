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
	head, base := "abc123", "base-a"
	logs := []reviewlog.Summary{
		{RunID: "mrv-1", Kind: "pr-ready", Target: "current branch", HeadSHA: head, BaseSHA: base, Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
		{RunID: "mrv-3", Kind: "pr-ready", Target: "current branch", HeadSHA: head, BaseSHA: base, Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
		{RunID: "mrv-2", Kind: "pr-ready", Target: "current branch", HeadSHA: head, BaseSHA: base, Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
	}
	blockers := []findings.Record{
		{ID: "mrvf-1", RunID: "mrv-1", Status: "open", Classification: "blocking", Severity: "high", Fingerprint: "security:duplicate", GitHead: head, Target: map[string]any{"type": "branch", "id": "feature"}},
		{ID: "mrvf-2", RunID: "mrv-2", Status: "open", Classification: "blocking", Severity: "high", Fingerprint: "security:duplicate", GitHead: head, Target: map[string]any{"type": "branch", "id": "feature"}},
		{ID: "mrvf-3", RunID: "mrv-3", Status: "open", Classification: "blocking", Severity: "high", Fingerprint: "security:duplicate", GitHead: head, Target: map[string]any{"type": "branch", "id": "feature"}},
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
	head, base := "abc123", "base-a"
	logs := []reviewlog.Summary{
		{RunID: "mrv-1", Kind: "pr-ready", Target: "current branch", HeadSHA: head, BaseSHA: base, Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
		{RunID: "mrv-2", Kind: "pr-ready", Target: "current branch", HeadSHA: head, BaseSHA: base, Verdict: "PASS"}, // later, CLEAN, no blockers
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
	head, base := "abc123", "base-a"
	logs := []reviewlog.Summary{
		{RunID: "mrv-1", Kind: "pr-ready", Target: "current branch", HeadSHA: head, BaseSHA: base, Verdict: "ESCALATED"},
		{RunID: "mrv-2", Kind: "pr-ready", Target: "current branch", HeadSHA: head, BaseSHA: base, Verdict: "PASS"},
	}

	projection := ProjectRecords(logs, nil, Options{CurrentTarget: map[string]string{"type": "branch", "id": "feature"}})

	if projection.SupersededRunIDs()["mrv-1"] {
		t.Fatal("a later CLEAN same-head re-run must NOT supersede an earlier ESCALATED run")
	}
}

// Two CLEAN re-runs over the same head collapse to the latest (harmless — neither blocks), so the projection
// carries one current log, not two. Exercises the clean-run supersede path.
func TestProjectDedupsSameHeadCleanReruns(t *testing.T) {
	head, base := "abc123", "base-a"
	logs := []reviewlog.Summary{
		{RunID: "mrv-1", Kind: "pr-ready", Target: "current branch", HeadSHA: head, BaseSHA: base, Verdict: "PASS"},
		{RunID: "mrv-2", Kind: "pr-ready", Target: "current branch", HeadSHA: head, BaseSHA: base, Verdict: "PASS"},
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
		{RunID: "mrv-1", Kind: "pr-ready", Target: "current branch", HeadSHA: "head-a", BaseSHA: "base-a", Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
		{RunID: "mrv-2", Kind: "pr-ready", Target: "current branch", HeadSHA: "head-b", BaseSHA: "base-a", Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
	}

	projection := ProjectRecords(logs, nil, Options{CurrentTarget: map[string]string{"type": "branch", "id": "feature"}})

	if len(projection.CurrentReviewLogs()) != 2 {
		t.Fatalf("logs at different heads must both remain current (no cross-head dedup): %+v", projection.CurrentReviewLogs())
	}
}

// Issue #99: two reviews at the SAME head but a DIFFERENT base reviewed DIFFERENT diffs (main advanced, so
// merge-base(HEAD, main) moved). They must NOT be deduped — the later must not supersede the earlier's
// base-specific blocking findings. Same head, same base still dedups (that is #97).
func TestProjectDoesNotDedupSameHeadDifferentBase(t *testing.T) {
	head := "abc123"
	logs := []reviewlog.Summary{
		{RunID: "mrv-1", Kind: "pr-ready", Target: "current branch", HeadSHA: head, BaseSHA: "base-a", Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
		{RunID: "mrv-2", Kind: "pr-ready", Target: "current branch", HeadSHA: head, BaseSHA: "base-b", Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
	}
	blockers := []findings.Record{
		{ID: "mrvf-1", RunID: "mrv-1", Status: "open", Classification: "blocking", Severity: "high", GitHead: head, Target: map[string]any{"type": "branch", "id": "feature"}},
		{ID: "mrvf-2", RunID: "mrv-2", Status: "open", Classification: "blocking", Severity: "high", GitHead: head, Target: map[string]any{"type": "branch", "id": "feature"}},
	}

	projection := ProjectRecords(logs, blockers, Options{CurrentTarget: map[string]string{"type": "branch", "id": "feature"}})

	if n := len(projection.CurrentReviewLogs()); n != 2 {
		t.Fatalf("same head, different base reviewed different diffs and must both stay current; got %d: %+v", n, projection.CurrentReviewLogs())
	}
	if projection.SupersededRunIDs()["mrv-1"] || projection.SupersededRunIDs()["mrv-2"] {
		t.Fatalf("neither same-head/different-base run may be superseded: %+v", projection.SupersededRunIDs())
	}
	if n := len(projection.CurrentBlockers()); n != 2 {
		t.Fatalf("both base-specific blockers must stay current; got %d: %+v", n, projection.CurrentBlockers())
	}
}

// Same head AND same base still dedups (issue #97 unchanged): two re-runs over the identical diff collapse.
func TestProjectDedupsSameHeadSameBase(t *testing.T) {
	head := "abc123"
	logs := []reviewlog.Summary{
		{RunID: "mrv-1", Kind: "pr-ready", Target: "current branch", HeadSHA: head, BaseSHA: "base-a", Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
		{RunID: "mrv-2", Kind: "pr-ready", Target: "current branch", HeadSHA: head, BaseSHA: "base-a", Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
	}

	projection := ProjectRecords(logs, nil, Options{CurrentTarget: map[string]string{"type": "branch", "id": "feature"}})

	if !projection.SupersededRunIDs()["mrv-1"] {
		t.Fatalf("same head AND same base must still dedup (#97): %+v", projection.SupersededRunIDs())
	}
	if n := len(projection.CurrentReviewLogs()); n != 1 || projection.CurrentReviewLogs()[0].RunID != "mrv-2" {
		t.Fatalf("only the latest same-head/same-base run remains current; got %d: %+v", n, projection.CurrentReviewLogs())
	}
}

// A log with a head but NO base (a legacy run record written before baseSha was threaded) must NOT be
// grouped: keying on an empty base would either collapse unrelated legacy logs or, worse, group a legacy
// blocking log with a current one. Mirror the empty-head skip.
func TestProjectDoesNotDedupSameHeadEmptyBase(t *testing.T) {
	head := "abc123"
	logs := []reviewlog.Summary{
		{RunID: "mrv-1", Kind: "pr-ready", Target: "current branch", HeadSHA: head, Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
		{RunID: "mrv-2", Kind: "pr-ready", Target: "current branch", HeadSHA: head, Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
	}

	projection := ProjectRecords(logs, nil, Options{CurrentTarget: map[string]string{"type": "branch", "id": "feature"}})

	if len(projection.CurrentReviewLogs()) != 2 {
		t.Fatalf("head-only logs with no base must not be deduped: %+v", projection.CurrentReviewLogs())
	}
}

func TestProjectDoesNotDedupSameHeadFindingWithEmptyBase(t *testing.T) {
	head := "abc123"
	logs := []reviewlog.Summary{
		{RunID: "mrv-1", Kind: "pr-ready", Target: "current branch", HeadSHA: head, Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
		{RunID: "mrv-2", Kind: "pr-ready", Target: "current branch", HeadSHA: head, Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
	}
	blockers := []findings.Record{
		{ID: "mrvf-1", RunID: "mrv-1", Status: "open", Classification: "blocking", Severity: "high", Fingerprint: "security:duplicate", GitHead: head, Target: map[string]any{"type": "branch", "id": "feature"}},
		{ID: "mrvf-2", RunID: "mrv-2", Status: "open", Classification: "blocking", Severity: "high", Fingerprint: "security:duplicate", GitHead: head, Target: map[string]any{"type": "branch", "id": "feature"}},
	}

	projection := ProjectRecords(logs, blockers, Options{CurrentTarget: map[string]string{"type": "branch", "id": "feature"}})

	if len(projection.CurrentBlockers()) != 2 || len(projection.SupersededFindingIDs()) != 0 {
		t.Fatalf("findings without a reviewed base must not be deduped: current=%+v superseded=%+v", projection.CurrentBlockers(), projection.SupersededFindingIDs())
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

func TestProjectKeepsSoleInheritedFindingWhenSameHeadOutcomeReruns(t *testing.T) {
	head, base := "abc123", "base-a"
	logs := []reviewlog.Summary{
		{RunID: "mrv-1", Kind: "pr-ready", Target: "current branch", HeadSHA: head, BaseSHA: base, Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
		{RunID: "mrv-2", Kind: "pr-ready", Target: "current branch", HeadSHA: head, BaseSHA: base, Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
	}
	blockers := []findings.Record{
		{ID: "mrvf-1", RunID: "mrv-1", Status: "open", Classification: "blocking", Severity: "high", Fingerprint: "security:inherited", GitHead: head, Target: map[string]any{"type": "branch", "id": "feature"}},
	}

	projection := ProjectRecords(logs, blockers, Options{CurrentTarget: map[string]string{"type": "branch", "id": "feature"}})

	if !projection.SupersededRunIDs()["mrv-1"] {
		t.Fatal("the earlier same-head outcome should be superseded")
	}
	if projection.SupersededFindingIDs()["mrvf-1"] {
		t.Fatal("a later outcome that inherited the finding must not supersede its sole record")
	}
	if n := len(projection.CurrentBlockers()); n != 1 || projection.CurrentBlockers()[0].ID != "mrvf-1" {
		t.Fatalf("the sole inherited finding must remain current; got %d: %+v", n, projection.CurrentBlockers())
	}
}

func TestStaleSameHeadDoesNotCrossAuthenticatedBranchTargets(t *testing.T) {
	head, base := "abc123", "base-a"
	logs := []reviewlog.Summary{
		{RunID: "mrv-1", Kind: "pr-ready", Target: "current branch", TargetRecord: map[string]string{"type": "branch", "id": "feature-a"}, RunRecordAuthenticated: true, HeadSHA: head, BaseSHA: base, Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
		{RunID: "mrv-2", Kind: "pr-ready", Target: "current branch", TargetRecord: map[string]string{"type": "branch", "id": "feature-b"}, RunRecordAuthenticated: true, HeadSHA: head, BaseSHA: base, Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
	}

	if stale := StaleSameHeadRunIDs(logs); len(stale) != 0 {
		t.Fatalf("authenticated branch targets sharing a head must remain distinct: %+v", stale)
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

func TestProjectTreatsUnrelatedMarkdownBlockerWithoutLogAsHistorical(t *testing.T) {
	blocker := findings.Record{
		ID:             "mrvf-markdown-001",
		RunID:          "mrv-markdown",
		Status:         "open",
		Classification: "blocking",
		Severity:       "high",
		Target:         map[string]any{"type": "markdown", "path": "docs/spec.md"},
	}

	projection := ProjectRecords(nil, []findings.Record{blocker}, Options{ChangedPaths: []string{"lib/parser.js"}})

	if len(projection.CurrentBlockers()) != 0 || len(projection.HistoricalBlockers()) != 1 {
		t.Fatalf("markdown must share path-target scoping: current=%+v historical=%+v", projection.CurrentBlockers(), projection.HistoricalBlockers())
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
	if len(projection.CurrentBlockers()) != 0 {
		t.Fatalf("unlinked task and mismatched branch blockers should both be historical: %+v", projection.CurrentBlockers())
	}
	if len(projection.HistoricalBlockers()) != 2 {
		t.Fatalf("expected branch and unlinked task blockers to be historical: %+v", projection.HistoricalBlockers())
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

func TestPRReadyProjectionKeepsOnlyCurrentTargetLinkedFindingsBlocking(t *testing.T) {
	logs := []reviewlog.Summary{
		{RunID: "mrv-old-task", Kind: "task-done", Target: "GUIDE-old", Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true, CoveredPaths: []string{"docs/old.md"}, CoveredPathsKnown: true},
		{RunID: "mrv-current-task", Kind: "task-done", Target: "GUIDE-current", Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true, CoveredPaths: []string{"internal/prready/review.go"}, CoveredPathsKnown: true},
		{RunID: "mrv-prior-pass", Kind: "pr-ready", Target: "current branch", Verdict: "PASS", CoveredPaths: []string{"internal/prready/review.go"}, CoveredPathsKnown: true},
	}
	blockers := []findings.Record{
		{ID: "mrvf-old-task", RunID: "mrv-old-task", Status: "open", Classification: "blocking", Severity: "high", Target: map[string]any{"type": "beads-task", "id": "GUIDE-old"}},
		{ID: "mrvf-current-task", RunID: "mrv-current-task", Status: "open", Classification: "blocking", Severity: "high", Target: map[string]any{"type": "beads-task", "id": "GUIDE-current"}},
		{ID: "mrvf-other-branch", RunID: "mrv-other-branch", Status: "open", Classification: "blocking", Severity: "high", Target: map[string]any{"type": "branch", "id": "other"}},
		{ID: "mrvf-current-pr", RunID: "mrv-current-pr", Status: "open", Classification: "blocking", Severity: "high", Target: map[string]any{"type": "pull-request", "id": "42"}},
	}

	projection := ProjectRecords(logs, blockers, Options{
		Scope:         "pr-ready",
		ChangedPaths:  []string{"internal/prready/review.go"},
		CurrentTarget: map[string]string{"type": "branch", "id": "feature"},
		LinkedTargets: []map[string]string{{"type": "pull-request", "id": "42"}},
	})

	if got := projection.CurrentReviewLogs(); len(got) != 1 || got[0].RunID != "mrv-current-task" {
		t.Fatalf("only the task review linked by the current diff should remain current: %+v", got)
	}
	got := projection.CurrentBlockers()
	if len(got) != 2 || got[0].ID != "mrvf-current-task" || got[1].ID != "mrvf-current-pr" {
		t.Fatalf("only current task/PR-linked findings should block: %+v", got)
	}
	if len(projection.HistoricalBlockers()) != 2 {
		t.Fatalf("unrelated blockers must remain visible as historical repository health: %+v", projection.HistoricalBlockers())
	}
}
