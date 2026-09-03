package reviewstate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dsifry/metareview/internal/state"
)

// A recorded marker round-trips: RecordReviewEvidence fills the invariant fields and DiscoverReviewEvidence
// reads it back, while an ordinary run record sharing runs.jsonl is skipped (it carries no review-evidence
// Kind). This is the coexistence the design depends on.
func TestReviewEvidenceRecordDiscoverRoundTrip(t *testing.T) {
	root := t.TempDir()
	runs := filepath.Join(root, ".metareview", "runs.jsonl")
	if err := os.MkdirAll(filepath.Dir(runs), 0o755); err != nil {
		t.Fatal(err)
	}
	// An ordinary pr-ready RUN record already in the file — the marker must not be confused with it.
	if err := state.AppendJSONL(runs, map[string]any{
		"schemaVersion": 1, "id": "mrv-x", "scope": "pr-ready", "verdict": "PASS",
		"headSha": "deadbeef", "target": map[string]string{"type": "branch", "id": "b"},
		"reviewers": []string{"pr-readiness-reviewer"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := RecordReviewEvidence(root, ReviewEvidence{
		ReviewedScope: "pr-ready", HeadSHA: "abc123", BaseSHA: "base0",
		LensSet: []string{"security", "code-quality"}, AdjudicatedVerdict: "PASS",
		ExecutionMode: ReviewModeSubagentAdjudicated, FromFSMRunID: "fsm-1",
	}); err != nil {
		t.Fatal(err)
	}

	markers, err := DiscoverReviewEvidence(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(markers) != 1 {
		t.Fatalf("expected exactly one marker (the run record must be skipped); got %d", len(markers))
	}
	m := markers[0]
	if m.Kind != ReviewEvidenceKind || m.Scope != ReviewEvidenceScope || m.SchemaVersion != 1 {
		t.Fatalf("invariant fields not filled: %+v", m)
	}
	if m.ReviewedScope != "pr-ready" || m.HeadSHA != "abc123" || m.AdjudicatedVerdict != "PASS" {
		t.Fatalf("marker content wrong: %+v", m)
	}
	if m.CreatedAt == "" {
		t.Fatal("CreatedAt must be filled")
	}
	if m.IsEmulated() {
		t.Fatal("a subagent-adjudicated marker is not emulated")
	}
}

// The gate query is head-scoped: a marker satisfies only its own (reviewedScope, headSHA). A different head,
// or a different scope, does not — a new commit requires a fresh review.
func TestLatestReviewEvidenceIsHeadAndScopeScoped(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".metareview"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustRecord := func(scope, head, mode, created string) {
		if err := RecordReviewEvidence(root, ReviewEvidence{
			ReviewedScope: scope, HeadSHA: head, ExecutionMode: mode, AdjudicatedVerdict: "PASS", CreatedAt: created,
		}); err != nil {
			t.Fatal(err)
		}
	}
	mustRecord("pr-ready", "head-1", ReviewModeSubagentAdjudicated, "2026-09-03T00:00:01Z")
	mustRecord("pr-ready", "head-1", ReviewModeInSessionEmulated, "2026-09-03T00:00:02Z") // later, same head
	mustRecord("task-done", "head-1", ReviewModeSubagentAdjudicated, "2026-09-03T00:00:03Z")

	// Matches head-1 pr-ready → the LATEST (the emulated one, by CreatedAt).
	m, ok, err := LatestReviewEvidence(root, "pr-ready", "head-1")
	if err != nil || !ok {
		t.Fatalf("expected a pr-ready marker for head-1; ok=%v err=%v", ok, err)
	}
	if !m.IsEmulated() {
		t.Fatalf("expected the later (emulated) marker to win by CreatedAt; got %+v", m)
	}
	// A different head is not satisfied.
	if _, ok, _ := LatestReviewEvidence(root, "pr-ready", "head-2"); ok {
		t.Fatal("a marker for head-1 must not satisfy head-2")
	}
	// A different scope is not satisfied by the pr-ready marker.
	if _, ok, _ := LatestReviewEvidence(root, "epic-ready", "head-1"); ok {
		t.Fatal("no epic-ready marker exists; must not be satisfied")
	}
}

// Two markers with the SAME CreatedAt (same scope+head) must resolve deterministically: the strict `>`
// keeps the first-recorded one (a later duplicate does not silently displace it). This pins the tie-break
// boundary — `>=` would let the last-appended marker win instead.
func TestLatestReviewEvidenceTieBreakKeepsFirstRecorded(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".metareview"), 0o755); err != nil {
		t.Fatal(err)
	}
	const same = "2026-09-03T12:00:00Z"
	// First-recorded is the PASS; a later duplicate at the identical timestamp is NEEDS_REVISION.
	if err := RecordReviewEvidence(root, ReviewEvidence{
		ReviewedScope: "pr-ready", HeadSHA: "head-1", AdjudicatedVerdict: "PASS", CreatedAt: same,
	}); err != nil {
		t.Fatal(err)
	}
	if err := RecordReviewEvidence(root, ReviewEvidence{
		ReviewedScope: "pr-ready", HeadSHA: "head-1", AdjudicatedVerdict: "NEEDS_REVISION", CreatedAt: same,
	}); err != nil {
		t.Fatal(err)
	}
	m, ok, err := LatestReviewEvidence(root, "pr-ready", "head-1")
	if err != nil || !ok {
		t.Fatalf("expected a marker; ok=%v err=%v", ok, err)
	}
	if m.AdjudicatedVerdict != "PASS" {
		t.Fatalf("on an identical-timestamp tie the first-recorded marker must win; got verdict %q", m.AdjudicatedVerdict)
	}
}

// No runs.jsonl at all → no markers, no error (a fresh repo).
func TestDiscoverReviewEvidenceEmptyRepo(t *testing.T) {
	markers, err := DiscoverReviewEvidence(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(markers) != 0 {
		t.Fatalf("a fresh repo has no markers; got %d", len(markers))
	}
}

// A corrupt runs.jsonl surfaces the read error rather than silently reporting "no review" — and the error
// propagates through LatestReviewEvidence, which the gate must not mistake for "not present".
func TestReviewEvidenceReadErrorPropagates(t *testing.T) {
	root := t.TempDir()
	runs := filepath.Join(root, ".metareview", "runs.jsonl")
	if err := os.MkdirAll(filepath.Dir(runs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runs, []byte("{not valid json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := DiscoverReviewEvidence(root); err == nil {
		t.Fatal("a malformed runs.jsonl must surface a read error")
	}
	if _, ok, err := LatestReviewEvidence(root, "pr-ready", "abc"); err == nil || ok {
		t.Fatalf("the read error must propagate (not present-false); ok=%v err=%v", ok, err)
	}
}

// The feature flag: required by default, opted out only by METAREVIEW_ALLOW_MECHANICAL_PASS=1.
func TestRequireAdjudicatedReview(t *testing.T) {
	t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "")
	if !RequireAdjudicatedReview() {
		t.Fatal("the adjudicated review is required by default")
	}
	t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "1")
	if RequireAdjudicatedReview() {
		t.Fatal("=1 must opt out of the requirement")
	}
	t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "0")
	if !RequireAdjudicatedReview() {
		t.Fatal("only the exact value 1 opts out")
	}
}
