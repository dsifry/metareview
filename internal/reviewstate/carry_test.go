package reviewstate

import (
	"errors"
	"testing"
)

func TestIsGateArtifact(t *testing.T) {
	for path, want := range map[string]bool{
		"docs/metareview/FINDINGS.md":                               true,
		"docs/metareview/reviews/mrv-1-pr-ready.md":                 true,
		"docs/metareview/context/mrv-1-context.md":                  true,
		"docs/metareview/shards/pr-ready/x/shard-0.abc.result.json": true,
		"docs/metareview/fsm/mrv-run/audit.redacted.jsonl":          true,
		"docs/metareview/learning/mrv-2-accepted.md":                true,
		"docs/metareview/fsm/mrv-run/sneak.go":                      false, // compiled, whatever folder it sits in
		"docs/metareview/notes.md":                                  false, // beside the gate's folders, not in one
		"docs/metareview/reviews/../../../src/x.md":                 false,
		"docs/ARCHITECTURE.md":                                      false,
		"internal/findings/findings.go":                             false,
		"docs/metareview/FINDINGS.md.bak":                           false,
	} {
		if got := IsGateArtifact(path); got != want {
			t.Errorf("IsGateArtifact(%q) = %v, want %v", path, got, want)
		}
	}
}

// CurrentReviewEvidence counts a marker at an earlier head only when everything since is a gate artifact, asks git
// once per distinct marker head, fails closed on a git error or a non-ancestor, and keeps last-recorded-wins.
func TestCurrentReviewEvidenceCarriesOnlyOverGateArtifacts(t *testing.T) {
	root := t.TempDir()
	record := func(head, verdict string) {
		t.Helper()
		if err := RecordReviewEvidence(root, ReviewEvidence{ReviewedScope: "pr-ready", BaseSHA: "b", HeadSHA: head, AdjudicatedVerdict: verdict}); err != nil {
			t.Fatal(err)
		}
	}
	record("h-artifacts", "PASS")
	record("h-code", "PASS")
	record("h-gone", "PASS")
	record("h-other", "PASS")
	record("h-artifacts", "PASS_ADVISORY") // same head again: asked once
	asked := map[string]int{}
	changed := func(from, to string) ([]string, bool, error) {
		asked[from]++
		switch from {
		case "h-artifacts":
			return []string{"docs/metareview/reviews/r.md", "docs/metareview/FINDINGS.md"}, true, nil
		case "h-code":
			return []string{"docs/metareview/reviews/r.md", "src/a.go"}, true, nil
		case "h-gone":
			return nil, false, errors.New("git failed")
		}
		return nil, false, nil // h-other: not an ancestor
	}
	got, ok, err := CurrentReviewEvidence(root, "pr-ready", "b", "HEAD", changed)
	if err != nil || !ok || got.HeadSHA != "h-artifacts" || got.AdjudicatedVerdict != "PASS_ADVISORY" {
		t.Fatalf("got %+v ok=%v err=%v", got, ok, err)
	}
	if asked["h-artifacts"] != 1 {
		t.Errorf("a marker head is asked about once, asked %d", asked["h-artifacts"])
	}
	// An exact marker recorded later still wins; a different scope or base never counts.
	record("HEAD", "NEEDS_REVISION")
	if got, _, _ := CurrentReviewEvidence(root, "pr-ready", "b", "HEAD", changed); got.AdjudicatedVerdict != "NEEDS_REVISION" {
		t.Fatalf("the last-recorded marker must win, got %+v", got)
	}
	if _, ok, _ := CurrentReviewEvidence(root, "task-done", "b", "HEAD", changed); ok {
		t.Fatal("another scope's marker never counts")
	}
	if _, ok, _ := CurrentReviewEvidence(root, "pr-ready", "other-base", "HEAD", changed); ok {
		t.Fatal("another base's marker never counts")
	}
}
