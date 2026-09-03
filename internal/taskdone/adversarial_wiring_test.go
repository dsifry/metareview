package taskdone

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/reviewstate"
)

// headSHA returns the repo's current HEAD, the SHA the marker must match.
func headSHA(t *testing.T, root string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// A recorded adjudicated marker at THIS head satisfies build B's require-lenses gate: the run reaches the
// present-and-passing branch, so the adversarial-review blocker is not raised. This is the marker-present
// path in Create() that the flag-opt-out tests never exercise.
func TestTaskDoneRequireLensesSatisfiedByMarker(t *testing.T) {
	root := shardedTaskRepo(t)
	// A PASS, subagent-adjudicated marker at HEAD — the review-lenses evidence the gate now demands.
	if err := reviewstate.RecordReviewEvidence(root, reviewstate.ReviewEvidence{
		ReviewedScope: "task-done", HeadSHA: headSHA(t, root),
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
	if err := reviewstate.RecordReviewEvidence(root, reviewstate.ReviewEvidence{
		ReviewedScope: "task-done", HeadSHA: headSHA(t, root),
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
