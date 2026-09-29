package status

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dsifry/metareview/internal/findings"
	"github.com/dsifry/metareview/internal/fsm/kind"
	"github.com/dsifry/metareview/internal/fsm/run"
	"github.com/dsifry/metareview/internal/fsm/workflow"
	"github.com/dsifry/metareview/internal/repo"
	"github.com/dsifry/metareview/internal/scope"
)

// AbandonedRun is an FSM run that stopped somewhere that is not an ending.
//
// Every loop in this system ends by handing control to an agent — `fix` is an agent-edit node, so
// the machine transitions into it, exits, and waits for the host to do the work and come back.
// Nothing ever brought control back. Six runs on this repository sit at `fix`, the oldest from
// 2026-08-28, and not one has ever reached `verify` or `done`: the loop has never closed.
//
// They were invisible, which is the part that matters. `status` read review logs only, so an
// abandoned run looked exactly like no run at all, and the gate that exists to say "work is
// unfinished" could not see the most direct evidence of unfinished work in the repository.
// StopNote is the event name an operator uses to say WHY a run was left where it is:
// `metareview fsm record stopped --data '{"reason":"..."}'`.
//
// It ANNOTATES the run. It does not remove it from this report, and that is the whole design.
//
// The first version suppressed the run, which made this a gate the principal it constrains could
// turn off: `fsm record <event>` is advertised to agents in the driver contract, so one append
// flipped `status` from blocked to clean — and hooks/pre-finish.sh branches on exactly that exit
// code. An empty payload sufficed. That is `override request` and `override grant` fused into a
// single unprivileged append, when this repository deliberately separates them: a request does
// not clear a gate, a grant must come from a different actor, it must carry a reason, and it
// lapses when the code it excused changes.
//
// An annotation needs none of that machinery, because it takes nothing away. A reader learns why
// a loop was abandoned; the loop still counts as abandoned and still blocks. There is nothing to
// gain by forging one. Suppression arrived as #179 instead: `override request|grant <run-id>` closes a
// run through the override system's separation of actor (closeRuns), never through an append here.
const StopNote = "stopped"

type AbandonedRun struct {
	RunID    string `json:"runId"`
	Workflow string `json:"workflow"`
	State    string `json:"state"`
	// Node is the node the machine was waiting on, when it stopped at a handoff.
	Node    string `json:"node,omitempty"`
	Updated string `json:"updated,omitempty"`
	// StopReason is what an operator recorded about why the run was left here, empty when they
	// recorded nothing. It explains the entry; it never removes it.
	StopReason string `json:"stopReason,omitempty"`
	// Branch is the branch the run was started for (#177): the checked-out branch at init, or --for-branch. Empty for
	// a run from before branches were recorded, which is scoped by reachability alone.
	Branch string `json:"branch,omitempty"`
	// Scope is where the run belongs relative to the branch in hand (#177): in-scope, other-branch or orphaned.
	Scope string `json:"scope,omitempty"`
	// CloseRequestedBy is who asked, through `override request <run-id>` (#179), for this run to be closed. A request
	// does not close it: the run keeps blocking until someone else grants it.
	CloseRequestedBy string `json:"closeRequestedBy,omitempty"`
	// ClosedBy, ClosedAt and CloseReason record the granted override that closed the run (#179): it no longer blocks
	// any branch, and `status --all` lists it with Scope "closed".
	ClosedBy    string `json:"closedBy,omitempty"`
	ClosedAt    string `json:"closedAt,omitempty"`
	CloseReason string `json:"closeReason,omitempty"`
	// Dir is the run's directory (the shared store, or the 0.13.x legacy one): what an operator deletes to clear a
	// run nobody will finish.
	Dir string `json:"dir,omitempty"`
	// head is the commit the run's init recorded. Not reported.
	head string
}

// DiscoverAbandonedRuns reports this branch's FSM runs left in a non-terminal state: the ones that block.
//
// Terminality is read from each run's OWN stored workflow, not from a list of state names here:
// a workflow names its own ending, and a second definition would drift from the first — the
// failure this repository keeps finding.
func DiscoverAbandonedRuns(root string) []AbandonedRun {
	return discoverAbandonedRuns(root, kind.Deps{})
}

// ScanAbandonedRuns is DiscoverAbandonedRuns plus every other abandoned run, scoped by internal/scope (#177): a run
// belongs to the branch it was started for, or to the branch in hand when its head lies in merge-base..HEAD. Runs
// that belong to another live branch, or to no live branch (orphaned), come back in elsewhere — listed by
// `status --all`, never a blocker here.
func ScanAbandonedRuns(root string) (inScope, elsewhere []AbandonedRun) {
	return closeRuns(root)(scanAbandonedRuns(root, kind.Deps{}, scope.Load(root, nil)))
}

// ClosedScope is the Scope of an abandoned run closed by a granted override (#179).
const ClosedScope = "closed"

// closeRuns applies the ledger's run closures (#179): a run whose closure row is granted — for the run as it stands,
// its last event unchanged since the grant — leaves the blockers and is listed elsewhere as closed; one with a pending
// request stays a blocker, naming who asked. A run resumed since its grant is open again, and an unreadable ledger
// closes nothing — the runs keep blocking.
func closeRuns(root string) func(inScope, elsewhere []AbandonedRun) ([]AbandonedRun, []AbandonedRun) {
	closures := map[string]findings.Record{}
	if ledger, err := loadFindings(root); err == nil {
		for _, record := range ledger {
			if findings.IsRunClosure(record) {
				closures[record.ID] = record
			}
		}
	}
	return func(inScope, elsewhere []AbandonedRun) ([]AbandonedRun, []AbandonedRun) {
		if inScope == nil && elsewhere == nil {
			return nil, nil
		}
		blocking := []AbandonedRun{}
		var closed []AbandonedRun
		mark := func(r AbandonedRun) (AbandonedRun, bool) {
			c, ok := closures[r.RunID]
			switch {
			case ok && c.Status == findings.StatusOverridden && c.RunUpdated != "" && c.RunUpdated == r.Updated:
				r.Scope, r.ClosedBy, r.ClosedAt, r.CloseReason = ClosedScope, c.OverrideGrantedBy, c.OverrideGrantedAt, c.OverrideGrantReason
				r.CloseRequestedBy = c.OverrideRequestedBy
				return r, true
			case ok && c.Status == findings.StatusOverridePending:
				r.CloseRequestedBy = c.OverrideRequestedBy
			}
			return r, false
		}
		for _, r := range inScope {
			if r, isClosed := mark(r); isClosed {
				closed = append(closed, r)
			} else {
				blocking = append(blocking, r)
			}
		}
		rest := make([]AbandonedRun, 0, len(elsewhere)+len(closed))
		for _, r := range elsewhere {
			r, _ = mark(r)
			rest = append(rest, r)
		}
		rest = append(rest, closed...)
		sort.SliceStable(rest, func(i, j int) bool {
			if rest[i].Branch != rest[j].Branch {
				return rest[i].Branch < rest[j].Branch
			}
			return rest[i].RunID < rest[j].RunID
		})
		if len(rest) == 0 {
			rest = nil
		}
		return blocking, rest
	}
}

// RunClosureSubject is the ledger row through which `override request|grant <run-id>` closes an abandoned FSM run
// (#179), for any run of the store left in a non-terminal state, whichever branch it belongs to. false for anything
// else: a finished or mock run, or an ID that names no run.
func RunClosureSubject(root, runID, now string) (*findings.Record, bool) {
	inScope, elsewhere := scanAbandonedRuns(root, kind.Deps{}, scope.Load(root, nil))
	for _, r := range append(inScope, elsewhere...) {
		if r.RunID == runID {
			record := findings.AbandonedRunRecord(r.RunID, "("+abandonedTarget(r)+")", r.Branch, r.head, r.Updated, now)
			return &record, true
		}
	}
	return nil, false
}

// discoverAbandonedRuns takes the registry deps so the misconfigured case is reachable from a
// test. Without the seam that branch could not be exercised — kind.New only errors when Mock
// disagrees with the judge type, and the caller above supplies neither — and an untestable
// branch in a gate is the shape this repository keeps finding defects in.
func discoverAbandonedRuns(root string, deps kind.Deps) []AbandonedRun {
	out, _ := closeRuns(root)(scanAbandonedRuns(root, deps, scope.Load(root, nil)))
	return out
}

func scanAbandonedRuns(root string, deps kind.Deps, sc scope.Scope) (out, elsewhere []AbandonedRun) {
	reg, err := kind.New(deps)
	if err != nil {
		// A registry that will not build cannot say what a workflow's ending is, so nothing is
		// claimed about any run. Reporting none is safe here because these are additional
		// blockers: the report is narrower, never falsely clean about the reviews themselves.
		return nil, nil
	}
	readable := false          // no runs directory anywhere reports nil, as it always has; an empty one reports []
	seen := map[string]bool{}  // ids reported as this branch's blockers
	other := map[string]bool{} // ids reported elsewhere
	// The 0.13.x location first, then the store: a migration renames a run from the first to the second, so a run
	// moved mid-scan is seen in one or the other (and deduped by id), never missed by both.
	// run-store: shared — the single 0.13.x location (the main checkout's .metareview/runs), read for one release
	// until an fsm command migrates it; never any other worktree's directory.
	sources := []string{filepath.Join(repo.RunStoreRoot(root), ".metareview", "runs")}
	if store, err := repo.StoreDir(root); err == nil {
		sources = append(sources, filepath.Join(store, "runs"))
	}
	for _, dir := range sources {
		runs, ok := abandonedIn(dir, reg.Info())
		readable = readable || ok
		for _, r := range runs {
			class := sc.Classify(r.Branch, r.head)
			r.Scope = class.String()
			// Blockers and the rest are deduped apart: on an id collision (the migration keeps both copies) a copy
			// that belongs elsewhere must never hide one that blocks here.
			switch {
			case class == scope.InScope && !seen[r.RunID]:
				out, seen[r.RunID] = append(out, r), true
			case class != scope.InScope && !other[r.RunID]:
				elsewhere, other[r.RunID] = append(elsewhere, r), true
			}
		}
	}
	// An id that blocks is not also listed elsewhere.
	kept := elsewhere[:0]
	for _, e := range elsewhere {
		if !seen[e.RunID] {
			kept = append(kept, e)
		}
	}
	elsewhere = kept
	if !readable {
		return nil, nil
	}
	if out == nil {
		out = []AbandonedRun{}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RunID < out[j].RunID })
	sort.Slice(elsewhere, func(i, j int) bool {
		if elsewhere[i].Branch != elsewhere[j].Branch {
			return elsewhere[i].Branch < elsewhere[j].Branch
		}
		return elsewhere[i].RunID < elsewhere[j].RunID
	})
	return out, elsewhere
}

// LegacyRunsPending reports whether the main checkout still holds 0.13.x FSM runs that the next fsm command would
// migrate into git's common directory (#173) — never bookkeeping, a non-run directory or a collision, which no
// migration moves, so the warning it drives always clears once a command runs.
func LegacyRunsPending(root string) bool {
	store, err := repo.StoreDir(root)
	if err != nil {
		return false
	}
	// run-store: shared — the single 0.13.x location; store is <common>/metareview, so its parent is the common dir.
	return len(run.PendingLegacyRuns(repo.RunStoreRoot(root), filepath.Dir(store))) > 0
}

// abandonedIn lists the abandoned runs directly under dir, and whether dir could be read at all.
func abandonedIn(dir string, kinds map[string]workflow.KindInfo) ([]AbandonedRun, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, false
	}
	var out []AbandonedRun
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if r, ok := abandonedRun(filepath.Join(dir, e.Name()), kinds); ok {
			r.Dir = filepath.Join(dir, e.Name())
			out = append(out, r)
		}
	}
	return out, true
}

func abandonedRun(dir string, kinds map[string]workflow.KindInfo) (AbandonedRun, bool) {
	src, err := os.ReadFile(filepath.Join(dir, "workflow.yaml")) // #nosec G304 -- a run directory this package walks
	if err != nil {
		return AbandonedRun{}, false
	}
	w, err := workflow.Parse(src, workflow.Options{Kinds: kinds})
	if err != nil {
		return AbandonedRun{}, false
	}
	audit, err := os.ReadFile(filepath.Join(dir, "audit.jsonl")) // #nosec G304 -- as above
	if err != nil {
		return AbandonedRun{}, false
	}
	got := AbandonedRun{RunID: filepath.Base(dir)}
	var state, mock string
	for _, line := range strings.Split(string(audit), "\n") {
		if line == "" {
			continue
		}
		var ev struct {
			Type  string `json:"type"`
			At    string `json:"at"`
			State string `json:"state"`
			Data  struct {
				// RecordData's fields, read for TypeRecord ONLY. `name` is not unique to records:
				// CmdCallData, GateData and OverflowHandlerData all serialise one, and a workflow
				// may legally declare a command called "stopped" — so reading `name` without
				// checking the type let a guarded command annotate every run of its workflow.
				Name     string          `json:"name"`
				Branch   string          `json:"branch"`
				Head     string          `json:"head"`
				Note     json.RawMessage `json:"data"`
				Workflow string          `json:"workflow"`
				Mock     string          `json:"mock"`
				To       string          `json:"to"`
				ToKind   string          `json:"to_kind"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		if ev.Type == run.TypeInit { // branch and head are the init event's; later events reuse "head" for trees
			got.Branch, got.head = ev.Data.Branch, ev.Data.Head
		}
		if ev.Type == run.TypeRecord && ev.Data.Name == StopNote {
			// Last note wins: an operator may record a stop, resume the run, and record another.
			got.StopReason = noteReason(ev.Data.Note)
		}
		if ev.Data.Workflow != "" {
			got.Workflow = ev.Data.Workflow
		}
		if ev.Data.Mock != "" {
			mock = ev.Data.Mock
		}
		if ev.At != "" {
			got.Updated = ev.At
		}
		if ev.Data.To != "" {
			state, got.Node = ev.Data.To, ev.Data.ToKind
		} else if ev.State != "" && ev.State != state {
			// The state moved without a transition carrying a node kind. Node belongs to the
			// state it arrived with, so carrying it forward makes the report name a node the
			// machine is not waiting on — a confident wrong answer about where a run is stuck.
			state, got.Node = ev.State, ""
		}
	}
	// A mock run proves nothing and is not work left undone. A stop note does NOT exclude a run:
	// it is reported with the operator's reason attached.
	if state == "" || mock != "" {
		return AbandonedRun{}, false
	}
	got.State = state
	if w.IsTerminal(run.State(state)) {
		return AbandonedRun{}, false
	}
	return got, true
}

// noteReason reads the `reason` a stop note carries. An absent or unreadable payload yields
// nothing, reported as a run stopped without a stated reason rather than treated as an error: the
// note is an annotation, and a malformed one is simply an annotation that says little.
func noteReason(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var note struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(raw, &note); err != nil {
		return ""
	}
	return strings.TrimSpace(note.Reason)
}
