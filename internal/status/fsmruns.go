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
// StoppedNote is the event name that closes a run on purpose. `metareview fsm record stopped
// --data '{"reason":"..."}'` writes it, and a run carrying one is finished rather than abandoned.
//
// Without it every deliberate stop looked exactly like a loop nobody came back to, so the
// abandoned-run report accumulated entries no one could ever clear — and a signal that only grows
// stops being read. The distinction it draws is the honest one: abandoned means stopped with no
// reason recorded.
const StoppedNote = "stopped"

type AbandonedRun struct {
	RunID    string `json:"runId"`
	Workflow string `json:"workflow"`
	State    string `json:"state"`
	// Node is the node the machine was waiting on, when it stopped at a handoff.
	Node    string `json:"node,omitempty"`
	Updated string `json:"updated,omitempty"`
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
	var stopped bool
	for _, line := range strings.Split(string(audit), "\n") {
		if line == "" {
			continue
		}
		var ev struct {
			Name  string `json:"name"`
			Type  string `json:"type"`
			At    string `json:"at"`
			State string `json:"state"`
			Data  struct {
				Name     string `json:"name"`
				Workflow string `json:"workflow"`
				Mock     string `json:"mock"`
				To       string `json:"to"`
				ToKind   string `json:"to_kind"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		if ev.Name == StoppedNote || ev.Data.Name == StoppedNote {
			stopped = true
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
		} else if ev.State != "" {
			state = ev.State
		}
	}
	// A mock run proves nothing and is not work left undone, and a run stopped on purpose is
	// finished — the operator said so and the reason is in the audit.
	if state == "" || mock != "" || stopped {
		return AbandonedRun{}, false
	}
	got.State = state
	if w.IsTerminal(run.State(state)) {
		return AbandonedRun{}, false
	}
	return got, true
}
