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

// history F (fork point on main) <- C1 <- C2 <- C3 <- C4 <- C5 (HEAD); X forked from C1 on another branch;
// O is an older main commit (an ancestor of F).
var ancestors = map[string][]string{
	"C5": {"O", "F", "C1", "C2", "C3", "C4"}, "C4": {"O", "F", "C1", "C2", "C3"}, "C3": {"O", "F", "C1", "C2"},
	"C2": {"O", "F", "C1"}, "C1": {"O", "F"}, "F": {"O"}, "X": {"O", "F", "C1"},
}

func isAncestor(a, d string) (bool, error) {
	if a == d {
		return true, nil
	}
	for _, x := range ancestors[d] {
		if x == a {
			return true, nil
		}
	}
	return false, nil
}

func checkpoint(t *testing.T, root, scope string) (string, bool) {
	t.Helper()
	got, ok, err := Checkpoint(root, scope, "C5", "F", isAncestor)
	if err != nil {
		t.Fatal(err)
	}
	return got, ok
}

// AC-3.5 (#176): the checkpoint is the head of the most recently recorded PASSING marker for the scope whose head
// is a strict ancestor of HEAD — not a NEEDS_REVISION one, not one on an unrelated branch, not another scope's.
func TestCheckpointIsTheLatestPassingAncestorMarker(t *testing.T) {
	root := t.TempDir()
	recordAll(t, root,
		ReviewEvidence{ReviewedScope: "task-done", BaseSHA: "F", HeadSHA: "C1", AdjudicatedVerdict: "PASS"},
		ReviewEvidence{ReviewedScope: "task-done", BaseSHA: "C1", HeadSHA: "C3", AdjudicatedVerdict: "pass_advisory"},
		ReviewEvidence{ReviewedScope: "task-done", BaseSHA: "C3", HeadSHA: "C4", AdjudicatedVerdict: "NEEDS_REVISION"},
		ReviewEvidence{ReviewedScope: "task-done", BaseSHA: "F", HeadSHA: "X", AdjudicatedVerdict: "PASS"},
		ReviewEvidence{ReviewedScope: "pr-ready", BaseSHA: "F", HeadSHA: "C4", AdjudicatedVerdict: "PASS"},
	)
	if got, ok := checkpoint(t, root, "task-done"); !ok || got != "C3" {
		t.Fatalf("Checkpoint = %q %v, want C3", got, ok)
	}
}

// A marker AT HEAD is the review this checkpoint anchors, not its base: after recording C3..C5, last-reviewed
// must still be C3 so the gate finds that marker (strict ancestry).
func TestCheckpointSkipsAMarkerAtHead(t *testing.T) {
	root := t.TempDir()
	recordAll(t, root,
		ReviewEvidence{ReviewedScope: "pr-ready", BaseSHA: "F", HeadSHA: "C3", AdjudicatedVerdict: "PASS"},
		ReviewEvidence{ReviewedScope: "pr-ready", BaseSHA: "C3", HeadSHA: "C5", AdjudicatedVerdict: "PASS"},
	)
	if got, ok := checkpoint(t, root, "pr-ready"); !ok || got != "C3" {
		t.Fatalf("Checkpoint = %q %v, want C3", got, ok)
	}
}

// The latest review of a head decides it, as the gate's last-recorded-wins rule does: a PASS at C3 that a later
// NEEDS_REVISION at C3 superseded is not a checkpoint, so the next increment starts earlier, not after rejected code.
func TestCheckpointHonoursALaterRejectionOfTheSameHead(t *testing.T) {
	root := t.TempDir()
	recordAll(t, root,
		ReviewEvidence{ReviewedScope: "task-done", BaseSHA: "F", HeadSHA: "C1", AdjudicatedVerdict: "PASS"},
		ReviewEvidence{ReviewedScope: "task-done", BaseSHA: "C1", HeadSHA: "C3", AdjudicatedVerdict: "PASS"},
		ReviewEvidence{ReviewedScope: "task-done", BaseSHA: "F", HeadSHA: "C3", AdjudicatedVerdict: "NEEDS_REVISION"},
	)
	if got, ok := checkpoint(t, root, "task-done"); !ok || got != "C1" {
		t.Fatalf("Checkpoint = %q %v, want C1", got, ok)
	}
}

// A checkpoint must vouch for everything since the fork point: its marker's base is at or before the fork point, or
// is itself a qualifying checkpoint. A passing review over a narrow base (C2..C3) with nothing reviewed before it
// would otherwise let F..C2 go unreviewed; a marker with no recorded base (pre-#175) cannot prove coverage.
func TestCheckpointRequiresContinuousCoverageFromTheForkPoint(t *testing.T) {
	root := t.TempDir()
	recordAll(t, root,
		ReviewEvidence{ReviewedScope: "pr-ready", BaseSHA: "C2", HeadSHA: "C3", AdjudicatedVerdict: "PASS"},
		ReviewEvidence{ReviewedScope: "pr-ready", HeadSHA: "C4", AdjudicatedVerdict: "PASS"},
	)
	if got, ok := checkpoint(t, root, "pr-ready"); ok {
		t.Fatalf("a checkpoint without coverage back to the fork point was accepted: %q", got)
	}
	// Covering C2 from an older main commit (a base before the fork point) completes the chain F..C2..C3.
	recordAll(t, root, ReviewEvidence{ReviewedScope: "pr-ready", BaseSHA: "O", HeadSHA: "C2", AdjudicatedVerdict: "PASS"})
	if got, ok := checkpoint(t, root, "pr-ready"); !ok || got != "C3" {
		t.Fatalf("Checkpoint = %q %v, want C3 (chained through C2)", got, ok)
	}
}

// A cycle in the recorded bases (hand-edited or replayed markers) terminates and does not qualify.
func TestCheckpointChainCycleTerminates(t *testing.T) {
	root := t.TempDir()
	recordAll(t, root,
		ReviewEvidence{ReviewedScope: "pr-ready", BaseSHA: "C3", HeadSHA: "C2", AdjudicatedVerdict: "PASS"},
		ReviewEvidence{ReviewedScope: "pr-ready", BaseSHA: "C2", HeadSHA: "C3", AdjudicatedVerdict: "PASS"},
	)
	if got, ok := checkpoint(t, root, "pr-ready"); ok {
		t.Fatalf("a base cycle must not qualify: %q", got)
	}
}

func TestCheckpointNoneAndErrors(t *testing.T) {
	root := t.TempDir()
	if _, ok := checkpoint(t, root, "epic-ready"); ok {
		t.Fatal("no markers must give no checkpoint")
	}
	recordAll(t, root, ReviewEvidence{ReviewedScope: "epic-ready", BaseSHA: "F", HeadSHA: "C3", AdjudicatedVerdict: "PASS"})
	boom := errors.New("boom")
	for _, failOn := range []string{"head", "base"} {
		fail := func(a, d string) (bool, error) {
			if (failOn == "head" && d == "C5") || (failOn == "base" && d == "F") {
				return false, boom
			}
			return isAncestor(a, d)
		}
		if _, _, err := Checkpoint(root, "epic-ready", "C5", "F", fail); !errors.Is(err, boom) {
			t.Fatalf("an ancestry failure (%s) must surface: %v", failOn, err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, ".metareview", "runs.jsonl"), []byte("{not json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Checkpoint(root, "epic-ready", "C5", "F", isAncestor); err == nil {
		t.Fatal("an unreadable store must surface")
	}
}

// The nearest qualifying head wins even when an older one was recorded later; an ancestry failure while ordering
// them surfaces.
func TestCheckpointPrefersTheNearestQualifyingHead(t *testing.T) {
	root := t.TempDir()
	recordAll(t, root,
		ReviewEvidence{ReviewedScope: "task-done", BaseSHA: "F", HeadSHA: "C3", AdjudicatedVerdict: "PASS"},
		ReviewEvidence{ReviewedScope: "task-done", BaseSHA: "F", HeadSHA: "C1", AdjudicatedVerdict: "PASS"},
	)
	if got, ok := checkpoint(t, root, "task-done"); !ok || got != "C3" {
		t.Fatalf("Checkpoint = %q %v, want the nearer C3", got, ok)
	}
	boom := errors.New("boom")
	fail := func(a, d string) (bool, error) {
		if a == "C1" && d == "C3" {
			return false, boom
		}
		return isAncestor(a, d)
	}
	if _, _, err := Checkpoint(root, "task-done", "C5", "F", fail); !errors.Is(err, boom) {
		t.Fatalf("an ancestry failure while ordering must surface: %v", err)
	}
}

// A chain only vouches through commits in this history: after a rebase, a checkpoint whose marker's base is a
// pre-rebase commit B (reviewed back to the fork point, but not an ancestor of it any more) proves nothing about the
// rewritten commits, so it does not qualify.
func TestCheckpointChainStaysInThisHistory(t *testing.T) {
	root := t.TempDir()
	recordAll(t, root,
		ReviewEvidence{ReviewedScope: "pr-ready", BaseSHA: "F", HeadSHA: "B", AdjudicatedVerdict: "PASS"},
		ReviewEvidence{ReviewedScope: "pr-ready", BaseSHA: "B", HeadSHA: "C3", AdjudicatedVerdict: "PASS"},
	)
	if got, ok := checkpoint(t, root, "pr-ready"); ok {
		t.Fatalf("a chain through a commit outside this history was accepted: %q", got)
	}
}
