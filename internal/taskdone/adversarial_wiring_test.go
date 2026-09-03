package taskdone

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/gitcontext"
	"github.com/dsifry/metareview/internal/reviewstate"
)

// diffEndpoints returns the exact (base, head) SHAs the gate computes for base ref "main", the pair a marker
// must carry to satisfy the currency check.
func diffEndpoints(t *testing.T, root string) (base, head string) {
	t.Helper()
	gc, err := gitcontext.Collect(root, "main")
	if err != nil {
		t.Fatalf("gitcontext.Collect: %v", err)
	}
	return gc.BaseSHA, gc.HeadSHA
}

// A recorded adjudicated marker over THIS base..head satisfies build B's require-lenses gate: the run
// reaches the present-and-passing branch, so the adversarial-review blocker is not raised. This is the
// marker-present path in Create() that the flag-opt-out tests never exercise.
func TestTaskDoneRequireLensesSatisfiedByMarker(t *testing.T) {
	root := shardedTaskRepo(t)
	t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "") // the gate under test must not be opted out by an inherited env
	base, head := diffEndpoints(t, root)
	// A PASS, subagent-adjudicated marker over base..head — the review-lenses evidence the gate now demands.
	if err := reviewstate.RecordReviewEvidence(root, reviewstate.ReviewEvidence{
		ReviewedScope: "task-done", BaseSHA: base, HeadSHA: head,
		AdjudicatedVerdict: "PASS", ExecutionMode: reviewstate.ReviewModeSubagentAdjudicated,
	}); err != nil {
		t.Fatal(err)
	}

	result, err := Create(root, "docs/tasks/big-task.md", Options{
		Base: "main", ShardWriter: &fakeWriter{satisfy: true}, EvidencePath: writeEvidence(t, root),
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(result.ReviewRel)))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "adversarial-review-reviewer") {
		t.Fatalf("a present, passing marker must clear the adversarial gate; review still blocks:\n%s", body)
	}
	if result.Blocking {
		t.Fatal("with shards satisfied, evidence present, and a passing marker, the run must not block")
	}
}

// A marker whose verdict is not a pass must NOT satisfy the gate: the review still carries the
// adversarial-review blocker (the "unresolved findings" branch of the reviewer).
func TestTaskDoneRequireLensesRejectsNonPassMarker(t *testing.T) {
	root := shardedTaskRepo(t)
	t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "") // the gate under test must not be opted out by an inherited env
	base, head := diffEndpoints(t, root)
	if err := reviewstate.RecordReviewEvidence(root, reviewstate.ReviewEvidence{
		ReviewedScope: "task-done", BaseSHA: base, HeadSHA: head,
		AdjudicatedVerdict: "NEEDS_REVISION", ExecutionMode: reviewstate.ReviewModeSubagentAdjudicated,
	}); err != nil {
		t.Fatal(err)
	}
	result, err := Create(root, "docs/tasks/big-task.md", Options{
		Base: "main", ShardWriter: &fakeWriter{satisfy: true}, EvidencePath: writeEvidence(t, root),
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(result.ReviewRel)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "adversarial-review-reviewer") {
		t.Fatalf("a non-pass marker must still block on the adversarial gate:\n%s", body)
	}
}
