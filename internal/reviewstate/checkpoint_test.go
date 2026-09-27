package reviewstate

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func recordAll(t *testing.T, root string, markers ...ReviewEvidence) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".metareview"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, m := range markers {
		if err := RecordReviewEvidence(root, m); err != nil {
			t.Fatal(err)
		}
	}
}

// history C1 <- C2 <- C3 <- C4 <- C5 (HEAD); X forked from C1 on another branch.
var ancestors = map[string][]string{"C5": {"C1", "C2", "C3", "C4"}, "C3": {"C1", "C2"}}

func isAncestor(a, d string) (bool, error) {
	for _, x := range ancestors[d] {
		if x == a {
			return true, nil
		}
	}
	return false, nil
}

// AC-3.5 (#176): the checkpoint is the head of the most recently recorded PASSING marker for the scope whose head
// is a strict ancestor of HEAD — not a NEEDS_REVISION one, not one on an unrelated branch, not another scope's.
func TestCheckpointIsTheLatestPassingAncestorMarker(t *testing.T) {
	root := t.TempDir()
	recordAll(t, root,
		ReviewEvidence{ReviewedScope: "task-done", HeadSHA: "C1", AdjudicatedVerdict: "PASS"},
		ReviewEvidence{ReviewedScope: "task-done", HeadSHA: "C3", AdjudicatedVerdict: "pass_advisory"},
		ReviewEvidence{ReviewedScope: "task-done", HeadSHA: "C4", AdjudicatedVerdict: "NEEDS_REVISION"},
		ReviewEvidence{ReviewedScope: "task-done", HeadSHA: "X", AdjudicatedVerdict: "PASS"},
		ReviewEvidence{ReviewedScope: "pr-ready", HeadSHA: "C4", AdjudicatedVerdict: "PASS"},
	)
	got, ok, err := Checkpoint(root, "task-done", "C5", isAncestor)
	if err != nil || !ok || got != "C3" {
		t.Fatalf("Checkpoint = %q %v %v, want C3", got, ok, err)
	}
}

// A marker AT HEAD is the review this checkpoint anchors, not its base: after recording C3..C5, last-reviewed
// must still be C3 so the gate finds that marker (strict ancestry).
func TestCheckpointSkipsAMarkerAtHead(t *testing.T) {
	root := t.TempDir()
	recordAll(t, root,
		ReviewEvidence{ReviewedScope: "pr-ready", HeadSHA: "C3", AdjudicatedVerdict: "PASS"},
		ReviewEvidence{ReviewedScope: "pr-ready", HeadSHA: "C5", BaseSHA: "C3", AdjudicatedVerdict: "PASS"},
	)
	if got, ok, err := Checkpoint(root, "pr-ready", "C5", isAncestor); err != nil || !ok || got != "C3" {
		t.Fatalf("Checkpoint = %q %v %v, want C3", got, ok, err)
	}
}

func TestCheckpointNoneAndErrors(t *testing.T) {
	root := t.TempDir()
	if _, ok, err := Checkpoint(root, "epic-ready", "C5", isAncestor); ok || err != nil {
		t.Fatalf("no markers: ok=%v err=%v", ok, err)
	}
	recordAll(t, root, ReviewEvidence{ReviewedScope: "epic-ready", HeadSHA: "C3", AdjudicatedVerdict: "PASS"})
	boom := errors.New("boom")
	if _, _, err := Checkpoint(root, "epic-ready", "C5", func(string, string) (bool, error) { return false, boom }); !errors.Is(err, boom) {
		t.Fatalf("an ancestry failure must surface: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".metareview", "runs.jsonl"), []byte("{not json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Checkpoint(root, "epic-ready", "C5", isAncestor); err == nil {
		t.Fatal("an unreadable store must surface")
	}
}
