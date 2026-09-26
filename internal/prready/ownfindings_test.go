package prready

import (
	"testing"

	"github.com/dsifry/metareview/internal/findings"
)

// Only pr-ready's own "Unresolved review blockers" finding on this branch is dropped: it is derived
// from other blockers, so reading it back as one is a loop. Every other finding passes through.
func TestWithoutOwnPRReadyFindings(t *testing.T) {
	branch := map[string]string{"type": "branch", "id": "work"}
	on := func(id string) map[string]any { return map[string]any{"type": "branch", "id": id} }
	in := []findings.Record{
		{ID: "loop", Scope: "pr-ready", Fingerprint: "pr:unresolved-review-blockers:work", Target: on("work")},
		{ID: "stale-mutation", Scope: "pr-ready", Fingerprint: "mutation:stale:enforce:stryker:edge:src/e.ts:0123", Target: on("work")},
		{ID: "other-branch", Scope: "pr-ready", Fingerprint: "pr:unresolved-review-blockers:elsewhere", Target: on("elsewhere")},
		{ID: "epic", Scope: "epic-ready", Fingerprint: "pr:unresolved-review-blockers:work", Target: on("work")},
	}
	got := withoutOwnPRReadyFindings(in, branch)
	if len(got) != 3 || got[0].ID != "stale-mutation" || got[1].ID != "other-branch" || got[2].ID != "epic" {
		t.Fatalf("got %+v", got)
	}
}
