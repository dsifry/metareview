package prready

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/reviewstate"
)

// headSHA returns the repo's current HEAD, the SHA a marker must match to satisfy the gate.
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

func prReviewBody(t *testing.T, root string, result Result) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(result.ReviewRel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// The pr-ready gate is the one the pre-push hook runs, so its require-lenses wiring is pinned directly: a
// PASS marker recorded at HEAD clears the adversarial-review blocker; without one the blocker is present.
func TestPRReadyRequireLensesSatisfiedByMarker(t *testing.T) {
	root := shardedRepo(t)
	if err := reviewstate.RecordReviewEvidence(root, reviewstate.ReviewEvidence{
		ReviewedScope: "pr-ready", HeadSHA: headSHA(t, root),
		AdjudicatedVerdict: "PASS", ExecutionMode: reviewstate.ReviewModeSubagentAdjudicated,
	}); err != nil {
		t.Fatal(err)
	}
	result, err := Create(root, Options{Base: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prReviewBody(t, root, result), "adversarial-review-reviewer") {
		t.Fatal("a present, passing marker must clear the adversarial gate on pr-ready")
	}
}

// The mirror: with no marker at HEAD the gate blocks on the adversarial-review reviewer.
func TestPRReadyRequireLensesBlocksWithoutMarker(t *testing.T) {
	root := shardedRepo(t)
	result, err := Create(root, Options{Base: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prReviewBody(t, root, result), "adversarial-review-reviewer") {
		t.Fatal("with no marker recorded, pr-ready must block on the adversarial gate")
	}
}
