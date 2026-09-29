package taskdone

import (
	"os"
	"os/exec"
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

// #161 on task-done: committing the gate's own review log after recording the marker keeps it current.
func TestTaskDoneMarkerCarriesOverGateArtifactCommits(t *testing.T) {
	root := shardedTaskRepo(t)
	t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "")
	base, head := diffEndpoints(t, root)
	if err := reviewstate.RecordReviewEvidence(root, reviewstate.ReviewEvidence{
		ReviewedScope: "task-done", BaseSHA: base, HeadSHA: head,
		AdjudicatedVerdict: "PASS", ExecutionMode: reviewstate.ReviewModeSubagentAdjudicated,
	}); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(root, "docs", "metareview", "reviews", "mrv-task.md")
	if err := os.MkdirAll(filepath.Dir(log), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(log, []byte("# review\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "-f", "docs/metareview/reviews/mrv-task.md"}, {"commit", "-q", "-m", "review log"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
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
		t.Fatalf("a marker must carry over a review-log commit on task-done:\n%s", body)
	}
	// A code commit after it invalidates it.
	code := filepath.Join(root, "src", "later.go")
	if err := os.MkdirAll(filepath.Dir(code), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(code, []byte("package src\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "src/later.go"}, {"commit", "-q", "-m", "code"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	result, err = Create(root, "docs/tasks/big-task.md", Options{
		Base: "main", ShardWriter: &fakeWriter{satisfy: true}, EvidencePath: writeEvidence(t, root),
	})
	if err != nil {
		t.Fatal(err)
	}
	if body, _ = os.ReadFile(filepath.Join(root, filepath.FromSlash(result.ReviewRel))); !strings.Contains(string(body), "adversarial-review-reviewer") {
		t.Fatal("a code commit after the marker must invalidate it on task-done")
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
