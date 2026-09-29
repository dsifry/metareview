package status

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/findings"
)

const closeReason = "inherited through a reused branch name; nobody will finish it"

// closeRequest and closeGrant file the override halves on an abandoned run the way `override request|grant <run-id>`
// does: with the closure row RunClosureSubject builds.
func closeRequest(t *testing.T, root, id, by string) error {
	t.Helper()
	subject, _ := RunClosureSubject(root, id, "2026-09-29T08:00:00Z")
	return findings.RequestOverride(root, id, findings.OverrideRequest{By: by, Reason: closeReason, Now: "2026-09-29T08:00:00Z", Subject: subject})
}

func closeGrant(t *testing.T, root, id, by string) error {
	t.Helper()
	subject, _ := RunClosureSubject(root, id, "2026-09-29T09:00:00Z")
	return findings.GrantOverride(root, id, findings.OverrideGrant{By: by, Reason: "accepted: the old fix branch was abandoned", Now: "2026-09-29T09:00:00Z", Subject: subject})
}

// AC-4.10 (#179): an abandoned run inherited through a reused branch name (AC-4.7) is closed through the override
// flow. A request alone does not clear it; a grant by the requester is refused; a grant by another actor makes it
// terminal — it no longer blocks this branch or any other, `status --all` lists it as closed with actor and reason, and
// the reason is rendered under Process Overrides.
func TestClosingAnAbandonedRunTakesARequestAndAGrantFromAnotherActor(t *testing.T) {
	root, common := newRepo(t)
	gitRun(t, root, "checkout", "-q", "-b", "fix")
	old := commit(t, root, "old fix")
	const id = "mrv-reuse-00001"
	writeStoreRun(t, common, id, "fix", old)
	gitRun(t, root, "checkout", "-q", "main")
	gitRun(t, root, "branch", "-q", "-D", "fix")
	gitRun(t, root, "checkout", "-q", "-b", "fix")
	commit(t, root, "new fix")
	blocked := func() []AbandonedRun {
		t.Helper()
		r, err := Build(root)
		if err != nil {
			t.Fatal(err)
		}
		return r.Abandoned
	}
	if got := blocked(); len(got) != 1 || got[0].RunID != id {
		t.Fatalf("the inherited run blocks before anything is filed: %+v", got)
	}

	if err := closeRequest(t, root, id, "claude-agent"); err != nil {
		t.Fatal(err)
	}
	got := blocked()
	if len(got) != 1 || got[0].CloseRequestedBy != "claude-agent" || got[0].ClosedBy != "" {
		t.Fatalf("a request alone must not clear the run, and names who asked: %+v", got)
	}
	if err := closeGrant(t, root, id, "claude-agent"); err == nil || !strings.Contains(err.Error(), "cannot also grant") {
		t.Fatalf("the requester's own grant must be refused, got %v", err)
	}
	if got := blocked(); len(got) != 1 {
		t.Fatalf("a refused grant leaves the run blocking: %+v", got)
	}

	if err := closeGrant(t, root, id, "maintainer@example.com"); err != nil {
		t.Fatal(err)
	}
	r, err := Build(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Abandoned) != 0 || r.Blocked || r.ClosedRuns != 1 || r.OrphanedRuns != 0 || r.OtherBranchRuns != 0 {
		t.Fatalf("a granted closure makes the run terminal here: %+v", r)
	}
	if len(r.Elsewhere) != 1 || r.Elsewhere[0].Scope != ClosedScope || r.Elsewhere[0].ClosedBy != "maintainer@example.com" ||
		r.Elsewhere[0].CloseReason != "accepted: the old fix branch was abandoned" || r.Elsewhere[0].CloseRequestedBy != "claude-agent" ||
		r.Elsewhere[0].ClosedAt != "2026-09-29T09:00:00Z" {
		t.Fatalf("--all lists it as closed, with actor and reason: %+v", r.Elsewhere)
	}
	if strings.Contains(strings.Join(r.Warnings, "\n"), "belong elsewhere") {
		t.Fatalf("a closed run is not reported as belonging elsewhere: %v", r.Warnings)
	}
	// Any branch: on main the run was never this branch's, and it is still listed as closed, never counted elsewhere.
	gitRun(t, root, "checkout", "-q", "main")
	if inScope, elsewhere := ScanAbandonedRuns(root); len(inScope) != 0 || len(elsewhere) != 1 || elsewhere[0].Scope != ClosedScope {
		t.Fatalf("from main: in scope %+v, elsewhere %+v", inScope, elsewhere)
	}
	index, err := os.ReadFile(filepath.Join(root, "docs", "metareview", "FINDINGS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), id+" [granted] Abandoned FSM run (t @ fix (branch fix))") ||
		!strings.Contains(string(index), "accepted: the old fix branch was abandoned") || !strings.Contains(string(index), closeReason) {
		t.Fatalf("the closure must be rendered under Process Overrides:\n%s", index)
	}
	// `stopped` is unchanged: an annotation still never removes a run (TestAStopNoteExplainsARunWithoutHidingIt).
}

// A closure row is bookkeeping, not a finding: a pending request never becomes a review-gate blocker.
func TestARunClosureRowIsNeverAGateBlocker(t *testing.T) {
	root, common := newRepo(t)
	head := gitOut(t, root, "rev-parse", "HEAD")
	writeStoreRun(t, common, "mrv-main-0000001", "main", head)
	if err := closeRequest(t, root, "mrv-main-0000001", "claude-agent"); err != nil {
		t.Fatal(err)
	}
	inScope, elsewhere, err := findings.ScopedBlocking(root)
	if err != nil || len(inScope) != 0 || len(elsewhere) != 0 {
		t.Fatalf("blockers %+v / %+v (%v)", inScope, elsewhere, err)
	}
	if pending, _ := findings.PendingOverrides(root); len(pending) != 1 {
		t.Fatalf("the request is still a pending override (CI stays red): %+v", pending)
	}
}

// Only a run left in a non-terminal state takes a closure row: a finished run, a mock run and an unknown ID do not.
func TestRunClosureSubjectNamesOnlyAbandonedRuns(t *testing.T) {
	root := t.TempDir()
	const init = `{"seq":1,"type":"init","at":"2026-08-28T01:00:00Z","data":{"workflow":"t","branch":"feat","head":"abc"}}`
	writeRun(t, root, "run-stuck", init, `{"seq":2,"type":"transition","at":"2026-08-28T01:05:00Z","state":"discover","data":{"from":"discover","to":"fix","to_kind":"agent-edit"}}`)
	writeRun(t, root, "run-done", init,
		`{"seq":2,"type":"transition","state":"discover","data":{"from":"discover","to":"fix"}}`,
		`{"seq":3,"type":"transition","state":"fix","data":{"from":"fix","to":"done"}}`)
	subject, ok := RunClosureSubject(root, "run-stuck", "now")
	if !ok || subject.ID != "run-stuck" || subject.Branch != "feat" || subject.GitHead != "abc" || !findings.IsRunClosure(*subject) ||
		subject.Title != "Abandoned FSM run (t @ fix (branch feat))" || subject.CreatedAt != "now" {
		t.Fatalf("got %+v ok=%v", subject, ok)
	}
	for _, id := range []string{"run-done", "run-nope"} {
		if _, ok := RunClosureSubject(root, id, "now"); ok {
			t.Errorf("%s must not take a closure row", id)
		}
	}
	if err := closeRequest(t, root, "run-done", "claude-agent"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("closing a finished run: %v", err)
	}
}

// An unreadable ledger closes nothing: the run keeps blocking.
func TestAnUnreadableLedgerClosesNoRun(t *testing.T) {
	root, common := newRepo(t)
	head := gitOut(t, root, "rev-parse", "HEAD")
	writeStoreRun(t, common, "mrv-main-0000001", "main", head)
	if err := closeRequest(t, root, "mrv-main-0000001", "claude-agent"); err != nil {
		t.Fatal(err)
	}
	if err := closeGrant(t, root, "mrv-main-0000001", "maintainer"); err != nil {
		t.Fatal(err)
	}
	orig := loadFindings
	loadFindings = func(string) ([]findings.Record, error) { return nil, errors.New("unreadable") }
	t.Cleanup(func() { loadFindings = orig })
	if got := DiscoverAbandonedRuns(root); len(got) != 1 {
		t.Fatalf("an unreadable ledger must leave the run blocking: %+v", got)
	}
}

// A closure on another branch's run is listed with it, and the JSON carries the closure fields.
func TestAClosedRunIsReportedInJSON(t *testing.T) {
	root, common := newRepo(t)
	head := gitOut(t, root, "rev-parse", "HEAD")
	writeStoreRun(t, common, "mrv-main-0000001", "main", head)
	writeStoreRun(t, common, "mrv-main-0000002", "", head)
	if err := closeGrant(t, root, "mrv-main-0000001", "maintainer"); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if _, err := EmitForAll(root, "", &out); err != nil {
		t.Fatal(err)
	}
	var rep struct {
		ClosedRuns int              `json:"closedRuns"`
		Elsewhere  []map[string]any `json:"elsewhere"`
		Abandoned  []map[string]any `json:"abandoned"`
	}
	if err := json.Unmarshal([]byte(out.String()), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.ClosedRuns != 1 || len(rep.Elsewhere) != 1 || rep.Elsewhere[0]["closedBy"] != "maintainer" || rep.Elsewhere[0]["scope"] != "closed" || len(rep.Abandoned) != 1 {
		t.Fatalf("got %s", out.String())
	}
}

// #179 review: a grant closes the run as it stood. A run resumed since (a later event) is open again and blocks, and a
// fresh request and grant close it anew; a closure row with no snapshot (an older binary dropped it) closes nothing.
func TestAResumedRunIsOpenAgainUntilClosedAnew(t *testing.T) {
	root, common := newRepo(t)
	head := gitOut(t, root, "rev-parse", "HEAD")
	const id = "mrv-main-0000001"
	writeStoreRun(t, common, id, "main", head)
	if err := closeRequest(t, root, id, "claude-agent"); err != nil {
		t.Fatal(err)
	}
	if err := closeGrant(t, root, id, "maintainer"); err != nil {
		t.Fatal(err)
	}
	if got := DiscoverAbandonedRuns(root); len(got) != 0 {
		t.Fatalf("closed: %+v", got)
	}
	audit := filepath.Join(common, "metareview", "runs", id, "audit.jsonl")
	f, err := os.OpenFile(audit, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString(`{"type":"transition","at":"2026-09-29T10:00:00Z","state":"fix","data":{"to":"discover","to_kind":"review-lenses"}}` + "\n")
	_ = f.Close()
	if got := DiscoverAbandonedRuns(root); len(got) != 1 || got[0].State != "discover" {
		t.Fatalf("a resumed run is open again: %+v", got)
	}
	if err := closeRequest(t, root, id, "claude-agent"); err != nil {
		t.Fatalf("a stale closure can be requested afresh: %v", err)
	}
	if got := DiscoverAbandonedRuns(root); len(got) != 1 || got[0].CloseRequestedBy != "claude-agent" {
		t.Fatalf("pending again: %+v", got)
	}
	if err := closeGrant(t, root, id, "maintainer"); err != nil {
		t.Fatal(err)
	}
	if got := DiscoverAbandonedRuns(root); len(got) != 0 {
		t.Fatalf("closed anew: %+v", got)
	}
	ledger, _ := findings.Load(root)
	ledger[0].RunUpdated = ""
	orig := loadFindings
	loadFindings = func(string) ([]findings.Record, error) { return ledger, nil }
	t.Cleanup(func() { loadFindings = orig })
	if got := DiscoverAbandonedRuns(root); len(got) != 1 {
		t.Fatalf("a closure with no snapshot closes nothing: %+v", got)
	}
}

// A mock run is not work left undone, so it never takes a closure row.
func TestAMockRunTakesNoClosureRow(t *testing.T) {
	root := t.TempDir()
	writeRun(t, root, "run-mock", `{"seq":1,"type":"init","at":"2026-08-28T01:00:00Z","data":{"workflow":"t","mock":"scenarios/happy#abc"}}`,
		`{"seq":2,"type":"transition","at":"2026-08-28T01:05:00Z","state":"discover","data":{"from":"discover","to":"fix"}}`)
	if _, ok := RunClosureSubject(root, "run-mock", "now"); ok {
		t.Fatal("a mock run must not take a closure row")
	}
}
