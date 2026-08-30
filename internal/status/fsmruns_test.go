package status

import (
	"github.com/dsifry/metareview/internal/fsm/kind"
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
