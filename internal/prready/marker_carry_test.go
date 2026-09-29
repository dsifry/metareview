package prready

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/reviewstate"
)

// commitFile writes rel and commits it on the checked-out branch.
func commitFile(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "-f", rel}, {"commit", "-q", "-m", "add " + rel}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

// TestPRReadyMarkerSurvivesGateArtifactCommits is #161 / its handoff: committing the gate's own artifacts (review
// logs, context packs, shard results, FSM bundles, FINDINGS.md, all under docs/metareview/) after record-lenses must
// not strand the marker — the reviewed code is unchanged. Any other change still invalidates it: code, and a file
// that is not a gate artifact even when it sits under docs/metareview/ (a .go file there would be compiled).
func TestPRReadyMarkerSurvivesGateArtifactCommits(t *testing.T) {
	t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "")
	evidence := filepath.Join(t.TempDir(), "evidence.md")
	if err := os.WriteFile(evidence, []byte("go test ./... exited 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	blocked := func(t *testing.T, root string) bool {
		t.Helper()
		result, err := Create(root, Options{Base: "main", EvidencePath: evidence})
		if err != nil {
			t.Fatal(err)
		}
		body := prReviewBody(t, root, result)
		for _, p := range []string{result.ReviewRel, result.ContextRel} { // keep the run's own output out of later diffs
			_ = os.Remove(filepath.Join(root, filepath.FromSlash(p)))
		}
		return strings.Contains(body, "- Reviewer: adversarial-review-reviewer")
	}
	mark := func(t *testing.T, root string) {
		t.Helper()
		base, head := diffEndpoints(t, root)
		if err := reviewstate.RecordReviewEvidence(root, reviewstate.ReviewEvidence{
			ReviewedScope: "pr-ready", BaseSHA: base, HeadSHA: head,
			AdjudicatedVerdict: "PASS", ExecutionMode: reviewstate.ReviewModeSubagentAdjudicated,
		}); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("gate artifacts only", func(t *testing.T) {
		root := smallPRReadyRepo(t)
		mark(t, root)
		commitFile(t, root, "docs/metareview/reviews/mrv-1-pr-ready.md", "# review\n")
		commitFile(t, root, "docs/metareview/context/mrv-1-pr-ready-context.md", "# context\n")
		for _, f := range []string{"audit.redacted.jsonl", "manifest.json", "snapshot.json", "workflow.yaml"} { // an `fsm export` bundle
			commitFile(t, root, "docs/metareview/fsm/mrv-run/"+f, "{}\n")
		}
		commitFile(t, root, "docs/metareview/shards/pr-ready/x/shard-0.abc.result.json", "{}\n")
		commitFile(t, root, "docs/metareview/FINDINGS.md", "# findings\n")
		if blocked(t, root) {
			t.Fatal("a marker must survive commits that add only gate artifacts")
		}
	})
	for name, rel := range map[string]string{
		"a code change":                     "src/extra.go",
		"code hidden under docs/metareview": "docs/metareview/fsm/mrv-run/sneak.go",
		"a doc outside the gate's folders":  "docs/metareview/notes.md",
	} {
		t.Run(name, func(t *testing.T) {
			root := smallPRReadyRepo(t)
			mark(t, root)
			commitFile(t, root, "docs/metareview/reviews/mrv-1-pr-ready.md", "# review\n")
			commitFile(t, root, rel, "package p\n")
			if !blocked(t, root) {
				t.Fatalf("%s after the marker must invalidate it", name)
			}
		})
	}
}
