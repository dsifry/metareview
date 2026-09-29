package epicready

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/gitcontext"
	"github.com/dsifry/metareview/internal/reviewstate"
)

// #161 on epic-ready: a marker recorded before the gate's own artifacts were committed still counts at the new head;
// a code commit after it does not.
func TestAdversarialStatusCarriesOverGateArtifactCommits(t *testing.T) {
	root := epicRepo(t)
	commit := func(rel string) string {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		for _, args := range [][]string{{"add", "-f", rel}, {"commit", "-q", "-m", rel}, {"rev-parse", "HEAD"}} {
			cmd := exec.Command("git", args...)
			cmd.Dir = root
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
			if args[0] == "rev-parse" {
				return strings.TrimSpace(string(out))
			}
		}
		return ""
	}
	gc, err := gitcontext.Collect(root, "main")
	if err != nil {
		t.Fatal(err)
	}
	if err := reviewstate.RecordReviewEvidence(root, reviewstate.ReviewEvidence{
		ReviewedScope: "epic-ready", BaseSHA: gc.BaseSHA, HeadSHA: gc.HeadSHA, AdjudicatedVerdict: "PASS",
		ExecutionMode: reviewstate.ReviewModeSubagentAdjudicated,
	}); err != nil {
		t.Fatal(err)
	}
	head := commit("docs/metareview/reviews/mrv-epic.md")
	if s := adversarialStatus(root, gitcontext.Context{BaseSHA: gc.BaseSHA, HeadSHA: head}, gitcontext.Context{}); !s.Present {
		t.Fatalf("a marker must carry over a review-log commit: %+v", s)
	}
	head = commit("src/more.go")
	if s := adversarialStatus(root, gitcontext.Context{BaseSHA: gc.BaseSHA, HeadSHA: head}, gitcontext.Context{}); s.Present {
		t.Fatalf("a code commit after the marker must invalidate it: %+v", s)
	}
}
