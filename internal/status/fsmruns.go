package status

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dsifry/metareview/internal/fsm/kind"
	"github.com/dsifry/metareview/internal/fsm/run"
	"github.com/dsifry/metareview/internal/fsm/workflow"
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
// gain by forging one. If suppression is ever genuinely wanted it should arrive as an FSM
// operation that makes the run terminal — the machine already owns run.Outcomes and StopReason —
// carrying the override system's separation of actor, not as a string a consumer package greps
// for.
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
}

// DiscoverAbandonedRuns reports FSM runs left in a non-terminal state.
//
// Terminality is read from each run's OWN stored workflow, not from a list of state names here:
// a workflow names its own ending, and a second definition would drift from the first — the
// failure this repository keeps finding.
func DiscoverAbandonedRuns(root string) []AbandonedRun {
	return discoverAbandonedRuns(root, kind.Deps{})
}

// discoverAbandonedRuns takes the registry deps so the misconfigured case is reachable from a
// test. Without the seam that branch could not be exercised — kind.New only errors when Mock
// disagrees with the judge type, and the caller above supplies neither — and an untestable
// branch in a gate is the shape this repository keeps finding defects in.
func discoverAbandonedRuns(root string, deps kind.Deps) []AbandonedRun {
	dir := filepath.Join(root, ".metareview", "runs")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	reg, err := kind.New(deps)
	if err != nil {
		// A registry that will not build cannot say what a workflow's ending is, so nothing is
		// claimed about any run. Reporting none is safe here because these are additional
		// blockers: the report is narrower, never falsely clean about the reviews themselves.
		return nil
	}
	out := []AbandonedRun{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if r, ok := abandonedRun(filepath.Join(dir, e.Name()), reg.Info()); ok {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RunID < out[j].RunID })
	return out
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
