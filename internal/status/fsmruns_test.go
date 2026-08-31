package status

import (
	"encoding/json"
	"github.com/dsifry/metareview/internal/fsm/kind"
	"github.com/dsifry/metareview/internal/fsm/run"
	"os"
	"path/filepath"
	"testing"
)

const testWorkflow = `workflow: t
version: 1
vars: {}
states: [discover, fix, done, failed]
transitions:
  - {from: discover, to: fix,  gate: findings_nonempty}
  - {from: fix,      to: done, gate: commit_exists, outcome: fixed}
nodes:
  discover: {kind: review-lenses, exec: subagent, lenses: 2}
  fix:      {kind: agent-edit}
convergence:
  any: [{max_iterations: 2}]
`

func writeRun(t *testing.T, root, id string, events ...string) {
	t.Helper()
	dir := filepath.Join(root, ".metareview", "runs", id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "workflow.yaml"), []byte(testWorkflow), 0o644); err != nil {
		t.Fatal(err)
	}
	body := ""
	for _, e := range events {
		body += e + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "audit.jsonl"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Every loop in this system ends by handing control to an agent: `fix` is an agent-edit node, so
// the machine transitions into it and waits for the host to do the work and come back. On this
// repository nothing ever came back — runs sit at `fix`, the oldest from 2026-08-28, and none has
// reached `verify`. They were invisible to `status`, so the gate whose job is saying "work is
// unfinished" could not see the plainest unfinished work in the tree.
func TestAbandonedRunsAreVisibleAndTerminalOnesAreNot(t *testing.T) {
	root := t.TempDir()
	const init = `{"seq":1,"type":"init","at":"2026-08-28T01:00:00Z","data":{"workflow":"t","run_id":"r"}}`

	writeRun(t, root, "run-stuck-at-fix", init,
		`{"seq":2,"type":"transition","at":"2026-08-28T01:05:00Z","state":"discover","data":{"from":"discover","to":"fix","to_kind":"agent-edit"}}`)
	writeRun(t, root, "run-finished", init,
		`{"seq":2,"type":"transition","at":"2026-08-28T01:05:00Z","state":"discover","data":{"from":"discover","to":"fix"}}`,
		`{"seq":3,"type":"transition","at":"2026-08-28T01:06:00Z","state":"fix","data":{"from":"fix","to":"done"}}`)
	// A mock run proves nothing and is not work left undone.
	writeRun(t, root, "run-mock", `{"seq":1,"type":"init","at":"2026-08-28T01:00:00Z","data":{"workflow":"t","mock":"scenarios/happy#abc"}}`,
		`{"seq":2,"type":"transition","at":"2026-08-28T01:05:00Z","state":"discover","data":{"from":"discover","to":"fix"}}`)
	// A run that never transitioned at all has no state to report.
	writeRun(t, root, "run-empty", init)

	got := DiscoverAbandonedRuns(root)
	if len(got) != 1 {
		t.Fatalf("got %d abandoned runs, want 1: %+v", len(got), got)
	}
	if got[0].RunID != "run-stuck-at-fix" || got[0].State != "fix" {
		t.Errorf("wrong run reported: %+v", got[0])
	}
	if got[0].Workflow != "t" || got[0].Node != "agent-edit" {
		t.Errorf("the report must name the workflow and the node it was waiting on: %+v", got[0])
	}
	if got[0].Updated != "2026-08-28T01:05:00Z" {
		t.Errorf("the report must carry when it stalled: %+v", got[0])
	}

	// And it reaches the gate: an abandoned run is a blocker, distinct from a review finding.
	rep, err := Build(root)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Blocked || len(rep.Abandoned) != 1 {
		t.Fatalf("an abandoned run must block: %+v", rep)
	}
	var found bool
	for _, b := range rep.MustClear {
		if b.Verdict == VerdictAbandoned && b.Kind == "fsm-run" && b.RunID == "run-stuck-at-fix" {
			found = true
		}
	}
	if !found {
		t.Errorf("must_clear must name the abandoned run: %+v", rep.MustClear)
	}
}

// The run directory is written by another process, so it is read defensively: anything
// unreadable is skipped, never fatal and never invented as an abandoned run.
func TestDiscoverAbandonedRunsToleratesAnImperfectDirectory(t *testing.T) {
	if got := DiscoverAbandonedRuns(t.TempDir()); got != nil {
		t.Errorf("no runs directory means no runs, got %+v", got)
	}
	root := t.TempDir()
	dir := filepath.Join(root, ".metareview", "runs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A file where a run directory should be.
	if err := os.WriteFile(filepath.Join(dir, "stray.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A run directory with no workflow, one with no audit, and one whose workflow is invalid.
	for name, files := range map[string]map[string]string{
		"no-workflow":      {"audit.jsonl": `{"seq":1,"type":"init","data":{"workflow":"t"}}`},
		"no-audit":         {"workflow.yaml": testWorkflow},
		"broken-workflow":  {"workflow.yaml": "not: a workflow\n", "audit.jsonl": `{"seq":1,"type":"init"}`},
		"unparseable-line": {"workflow.yaml": testWorkflow, "audit.jsonl": "not json\n"},
	} {
		d := filepath.Join(dir, name)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		for f, body := range files {
			if err := os.WriteFile(filepath.Join(d, f), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	if got := DiscoverAbandonedRuns(root); len(got) != 0 {
		t.Errorf("nothing readable means nothing to report, got %+v", got)
	}
}

// A registry that will not build cannot say what a workflow's ending is, so nothing is claimed
// about any run. Mock true with no MockJudge is exactly that misconfiguration.
func TestDiscoverAbandonedRunsWithAnUnusableRegistry(t *testing.T) {
	root := t.TempDir()
	writeRun(t, root, "run-stuck", `{"seq":1,"type":"init","at":"2026-08-28T01:00:00Z","data":{"workflow":"t"}}`,
		`{"seq":2,"type":"transition","at":"2026-08-28T01:05:00Z","state":"discover","data":{"from":"discover","to":"fix"}}`)
	if got := discoverAbandonedRuns(root, kind.Deps{}); len(got) != 1 {
		t.Fatalf("the ordinary registry must find the stuck run: %+v", got)
	}
	if got := discoverAbandonedRuns(root, kind.Deps{Mock: true}); got != nil {
		t.Errorf("a registry that will not build must claim nothing: %+v", got)
	}
}

// The state can also come from an event's own state field rather than a transition's target —
// a run stopped mid-node has recorded where it was without recording an arrival.
// The node belongs to the state it arrived with. A later event that moves the state without
// carrying a node kind used to leave Node naming the PREVIOUS node, so the report told an
// operator the run was waiting on something it had already left — a confident wrong answer about
// where a run is stuck, which is worse than no answer.
func TestNodeDoesNotOutliveTheStateItArrivedWith(t *testing.T) {
	root := t.TempDir()
	writeRun(t, root, "run-moved",
		`{"seq":1,"type":"init","at":"2026-08-28T01:00:00Z","data":{"workflow":"t","run_id":"r"}}`,
		`{"seq":2,"type":"transition","at":"2026-08-28T01:01:00Z","state":"discover","data":{"from":"discover","to":"fix","to_kind":"agent-edit"}}`,
		`{"seq":3,"type":"record","at":"2026-08-28T01:02:00Z","state":"discover","data":{"name":"x"}}`)
	got := DiscoverAbandonedRuns(root)
	if len(got) != 1 {
		t.Fatalf("want one abandoned run, got %d", len(got))
	}
	if got[0].State != "discover" {
		t.Errorf("state = %q, want discover", got[0].State)
	}
	if got[0].Node != "" {
		t.Errorf("Node = %q, but the run left the node that kind belonged to", got[0].Node)
	}
}

func TestAbandonedRunReadsStateFromANonTransitionEvent(t *testing.T) {
	root := t.TempDir()
	writeRun(t, root, "run-mid-node", `{"seq":1,"type":"init","at":"2026-08-28T01:00:00Z","data":{"workflow":"t"}}`,
		`{"seq":2,"type":"needs_input","at":"2026-08-28T01:02:00Z","state":"discover","data":{}}`)
	got := DiscoverAbandonedRuns(root)
	if len(got) != 1 || got[0].State != "discover" {
		t.Fatalf("a run stopped inside a node must still report where it is: %+v", got)
	}
	if got[0].Node != "" {
		t.Errorf("no arrival was recorded, so no node kind is claimed: %+v", got[0])
	}
}

// Directory order is not sorted order, so the report has to sort. A gate whose output reshuffles
// between runs over an unchanged repository cannot be read as a diff, which is how a reviewer
// notices that one more loop was abandoned since last time.
func TestAbandonedRunsAreReportedInAStableOrder(t *testing.T) {
	root := t.TempDir()
	stall := func(id string) {
		writeRun(t, root, id, `{"seq":1,"type":"init","at":"2026-08-28T01:00:00Z","data":{"workflow":"t"}}`,
			`{"seq":2,"type":"transition","at":"2026-08-28T01:05:00Z","state":"discover","data":{"from":"discover","to":"fix"}}`)
	}
	for _, id := range []string{"run-c", "run-a", "run-b"} {
		stall(id)
	}
	got := DiscoverAbandonedRuns(root)
	if len(got) != 3 {
		t.Fatalf("got %d, want 3: %+v", len(got), got)
	}
	for i, want := range []string{"run-a", "run-b", "run-c"} {
		if got[i].RunID != want {
			t.Errorf("position %d = %q, want %q (the report must be sorted)", i, got[i].RunID, want)
		}
	}
}

// A stop note ANNOTATES a run; it never removes it from the report.
//
// The first version suppressed the run, which made this a gate the principal it constrains could
// turn off: `fsm record <event>` is advertised to agents, so one append flipped `status` from
// blocked to clean and hooks/pre-finish.sh branches on exactly that exit code. An annotation
// cannot be used that way, which is why it needs none of the override system's actor separation —
// it takes nothing away, so there is nothing to gain by forging one.
func TestAStopNoteExplainsARunWithoutHidingIt(t *testing.T) {
	root := t.TempDir()
	const init = `{"seq":1,"type":"init","at":"2026-08-30T01:00:00Z","data":{"workflow":"t","run_id":"r"}}`
	const moved = `{"seq":2,"type":"transition","at":"2026-08-30T01:05:00Z","state":"discover","data":{"from":"discover","to":"fix","to_kind":"agent-edit"}}`

	// Built by marshalling the REAL types: machine.Record appends run.RecordData as the payload,
	// so the name lives at data.name and the note at data.data. A hand-written fixture would pin
	// a shape the writer does not produce, and a change to RecordData's serialisation would then
	// break `fsm record stopped` in production with the suite still green.
	stop := func(seq int, reason string) string {
		t.Helper()
		note, err := json.Marshal(map[string]string{"reason": reason})
		if err != nil {
			t.Fatal(err)
		}
		payload, err := json.Marshal(run.RecordData{Name: StopNote, Data: note})
		if err != nil {
			t.Fatal(err)
		}
		line, err := json.Marshal(map[string]any{
			"seq": seq, "type": run.TypeRecord, "at": "2026-08-30T01:06:00Z", "state": "fix",
			"data": json.RawMessage(payload),
		})
		if err != nil {
			t.Fatal(err)
		}
		return string(line)
	}

	writeRun(t, root, "run-annotated", init, moved, stop(3, "superseded by a newer run"))
	writeRun(t, root, "run-silent", init, moved)

	got := DiscoverAbandonedRuns(root)
	if len(got) != 2 {
		t.Fatalf("a stop note must not remove a run from the report: got %d, want 2 (%+v)", len(got), got)
	}
	by := map[string]AbandonedRun{}
	for _, r := range got {
		by[r.RunID] = r
	}
	if by["run-annotated"].StopReason != "superseded by a newer run" {
		t.Errorf("the operator's reason must be carried: %q", by["run-annotated"].StopReason)
	}
	if by["run-silent"].StopReason != "" {
		t.Errorf("a run with no note has no reason: %q", by["run-silent"].StopReason)
	}
	// Still blocking. An annotation explains unfinished work; it does not finish it.
	rep, err := Build(root)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Blocked || len(rep.Abandoned) != 2 {
		t.Errorf("annotated or not, both runs must still block: %+v", rep)
	}
}

// `name` is not unique to record events: CmdCallData, GateData and OverflowHandlerData all
// serialise one and cmdPattern admits "stopped", so reading it without checking the type let a
// workflow that declared a guarded command called `stopped` annotate — and, in the suppressing
// version, HIDE — every run of that workflow.
func TestOnlyARecordEventCarriesAStopNote(t *testing.T) {
	root := t.TempDir()
	const init = `{"seq":1,"type":"init","at":"2026-08-30T01:00:00Z","data":{"workflow":"t","run_id":"r"}}`
	const moved = `{"seq":2,"type":"transition","at":"2026-08-30T01:05:00Z","state":"discover","data":{"from":"discover","to":"fix"}}`
	// Each collision fixture carries a `data` payload holding a reason, so that if the type check
	// were dropped the reason WOULD be read and the assertion below would fail. An earlier version
	// omitted it: the mutation that removes `ev.Type == run.TypeRecord` then produced an empty
	// StopReason and the test passed, so the control could not detect the thing it controls for.
	for name, ev := range map[string]string{
		"cmd-call":         `{"seq":3,"type":"cmd_call","data":{"name":"stopped","data":{"reason":"forged by a command name"}}}`,
		"gate":             `{"seq":3,"type":"gate","data":{"name":"stopped","data":{"reason":"forged by a gate name"}}}`,
		"overflow-handler": `{"seq":3,"type":"overflow_handler","data":{"name":"stopped","data":{"reason":"forged by a handler name"}}}`,
		"a-different-note": `{"seq":3,"type":"record","data":{"name":"observation","data":{"reason":"just a note"}}}`,
	} {
		writeRun(t, root, "run-"+name, init, moved, ev)
	}
	got := DiscoverAbandonedRuns(root)
	if len(got) != 4 {
		t.Fatalf("every run must still be reported: got %d, want 4", len(got))
	}
	for _, r := range got {
		if r.StopReason != "" {
			t.Errorf("%s: only a record event named %q carries a stop note, got %q", r.RunID, StopNote, r.StopReason)
		}
	}
}

// A note whose payload says nothing is an annotation that says nothing — not an error, and not a
// reason. The run is reported either way, so an empty note buys nothing.
func TestAStopNoteWithNoReasonIsReportedWithoutOne(t *testing.T) {
	root := t.TempDir()
	const init = `{"seq":1,"type":"init","at":"2026-08-30T01:00:00Z","data":{"workflow":"t","run_id":"r"}}`
	const moved = `{"seq":2,"type":"transition","at":"2026-08-30T01:05:00Z","state":"discover","data":{"from":"discover","to":"fix"}}`
	for name, note := range map[string]string{
		"empty-object": `{"name":"stopped","data":{}}`,
		"blank-reason": `{"name":"stopped","data":{"reason":"   "}}`,
		"no-payload":   `{"name":"stopped"}`,
		"unparseable":  `{"name":"stopped","data":"not an object"}`,
	} {
		writeRun(t, root, "run-"+name, init, moved, `{"seq":3,"type":"record","data":`+note+`}`)
	}
	got := DiscoverAbandonedRuns(root)
	if len(got) != 4 {
		t.Fatalf("got %d runs, want 4", len(got))
	}
	for _, r := range got {
		if r.StopReason != "" {
			t.Errorf("%s: an empty note yields no reason, got %q", r.RunID, r.StopReason)
		}
	}
}
