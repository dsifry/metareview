package prready

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
	t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "") // the gate under test must not be opted out by an inherited env
	base, head := diffEndpoints(t, root)
	if err := reviewstate.RecordReviewEvidence(root, reviewstate.ReviewEvidence{
		ReviewedScope: "pr-ready", BaseSHA: base, HeadSHA: head,
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
	t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "") // the gate under test must not be opted out by an inherited env
	result, err := Create(root, Options{Base: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prReviewBody(t, root, result), "adversarial-review-reviewer") {
		t.Fatal("with no marker recorded, pr-ready must block on the adversarial gate")
	}
}
