package prready

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dsifry/metareview/internal/reviewlog"
)

// TestChainAdoptsSameDiffRunsOutsideIt is bead mr-mrf: run A (standalone, NEEDS_REVISION: no evidence), then run B
// (standalone again, same head), then run C repairing B's chain (--previous-run B, evidence supplied). A's open
// finding was outside C's chain, so nothing could ever close it and C stayed blocked — only an override cleared it.
// C's lineage now holds A too (same target, same base..head as B). A standalone re-run still adopts nothing.
func TestChainAdoptsSameDiffRunsOutsideIt(t *testing.T) {
	t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "1")
	root := smallPRReadyRepo(t)
	evidence := filepath.Join(t.TempDir(), "evidence.md")
	if err := os.WriteFile(evidence, []byte("go test ./... exited 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 29, 3, 0, 0, 0, time.UTC)
	a, err := Create(root, Options{Base: "main", Now: now})
	if err != nil || !a.Blocking {
		t.Fatalf("setup: run A must block: %+v %v", a, err)
	}
	b, err := Create(root, Options{Base: "main", Now: now.Add(time.Minute)})
	if err != nil || !b.Blocking {
		t.Fatalf("setup: standalone run B must block too: %+v %v", b, err)
	}
	c, err := Create(root, Options{Base: "main", EvidencePath: evidence, PreviousRunID: b.RunID, Now: now.Add(2 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if c.Blocking {
		body, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(c.ReviewRel)))
		t.Fatalf("the repair chain must close the same-diff run A's finding it no longer reproduces:\n%s", body)
	}
}

func TestSameDiffPRReadyRunIDs(t *testing.T) {
	target := map[string]string{"type": "branch", "id": "feature"}
	log := func(id, kind, base, head, targetID string) reviewlog.Summary {
		return reviewlog.Summary{RunID: id, Kind: kind, BaseSHA: base, HeadSHA: head, TargetRecord: map[string]string{"type": "branch", "id": targetID}}
	}
	logs := []reviewlog.Summary{
		log("chain-1", "pr-ready", "b", "h1", "feature"),
		log("same-diff", "pr-ready", "b", "h1", "feature"),    // adopted
		log("other-head", "pr-ready", "b", "h0", "feature"),   // a different diff
		log("other-base", "pr-ready", "b0", "h1", "feature"),  // a different diff
		log("no-record", "pr-ready", "", "", "feature"),       // no authenticated run record
		log("other-target", "pr-ready", "b", "h1", "other"),   // another branch
		log("task-review", "task-done", "b", "h1", "feature"), // not a pr-ready run
		log("same-diff", "pr-ready", "b", "h1", "feature"),    // listed twice: adopted once
	}
	if got := sameDiffPRReadyRunIDs(logs, target, []string{"chain-1"}); strings.Join(got, ",") != "same-diff" {
		t.Fatalf("adopted %v", got)
	}
	if got := sameDiffPRReadyRunIDs(logs, target, nil); len(got) != 0 {
		t.Fatalf("a run with no chain adopts nothing, got %v", got)
	}
}
