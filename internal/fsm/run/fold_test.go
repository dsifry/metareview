package run

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

// ---- fixtures ---------------------------------------------------------------------------------

const runA = "mrv-20260826-000000000000000-fsm-sdlc-loop-sdlc-loop-aaaaaaaa"
const runB = "mrv-20260826-000000000000001-fsm-sdlc-loop-sdlc-loop-bbbbbbbb"

func baseInit() InitData {
	return InitData{
		Workflow: "sdlc-loop", WorkflowHash: "wh", RepoMode: "advisory", RepoRoot: "/r", WorkDir: "/r",
		BaseSHA: "base0000", Head: "head0000", InitialState: "discover",
		AllowedCmds: []AllowedCmd{{Name: "notify", Argv: []string{"/bin/notify"}, FileHashes: map[string]string{"/bin/notify": "h"}}},
	}
}

func bug(id string) Bug {
	return Bug{ID: id, Desc: "d " + id, Verdict: "real_but_ungold", Confidence: 0.9}
}

func out(raw string) NodeOutputData { return NodeOutputData{Output: json.RawMessage(raw)} }

func deltaFor(raw string, d Delta) DeltaAppliedData {
	return DeltaAppliedData{Delta: d, OutputHash: OutputHash([]byte(raw))}
}

// happyLog builds discover→adjudicate→fix→verify→done with one confirmed bug fixed.
func happyLog() *Builder {
	b := NewBuilder(runA)
	b.Init(baseInit())
	b.Event(TypeTree, TreeData{Head: "head0000", TreeHash: "t0", Status: ""})
	b.Event(TypeNeedsInput, EmptyData{}, WithNode("discover"))
	b.Event(TypeNodeOutput, out(`{"findings":[{"issue_text":"x"}]}`), WithNode("discover"))
	b.Event(TypeDeltaApplied, deltaFor(`{"findings":[{"issue_text":"x"}]}`, Delta{Findings: []Finding{{IssueText: "x"}}}), WithNode("discover"))
	b.Event(TypeGate, GateData{Name: "findings_nonempty", Passed: true})
	b.Event(TypeTransition, TransitionData{From: "discover", To: "adjudicate", Gate: "findings_nonempty", Head: "head0000"})
	b.Event(TypeLLMCall, LLMCallData{Kind: "adjudicate", Model: "m", Effort: "e", Index: 0, InputHash: "ih", Verdict: json.RawMessage(`{"is_real":true}`), Confidence: 0.9, Tokens: TokenTotals{Input: 10, Output: 5}}, WithNode("adjudicate"))
	b.Event(TypeNodeOutput, out(`{"confirmed":["b1"]}`), WithNode("adjudicate"))
	b.Event(TypeDeltaApplied, deltaFor(`{"confirmed":["b1"]}`, Delta{Confirmed: []Bug{bug("b1")}}), WithNode("adjudicate"))
	b.Event(TypeGate, GateData{Name: "confirmed_nonempty", Passed: true})
	b.Event(TypeTransition, TransitionData{From: "adjudicate", To: "fix", Gate: "confirmed_nonempty", ToKind: KindAgentEdit, Head: "head0000"})
	b.Event(TypeNeedsInput, EmptyData{}, WithNode("fix"))
	b.Event(TypeNodeOutput, out(`{"commit":"c1"}`), WithNode("fix"))
	b.Event(TypeDeltaApplied, deltaFor(`{"commit":"c1"}`, Delta{Commit: "c1"}), WithNode("fix"))
	b.Event(TypeTree, TreeData{Head: "c1", TreeHash: "t1", Status: ""})
	b.Event(TypeGate, GateData{Name: "commit_exists", Passed: true})
	b.Event(TypeTransition, TransitionData{From: "fix", To: "verify", Gate: "commit_exists", Head: "c1"})
	b.Event(TypeLLMCall, LLMCallData{Kind: "still-present", Model: "m", Effort: "e", Index: 0, InputHash: "ih2", Verdict: json.RawMessage(`{"still_present":false}`), Tokens: TokenTotals{Input: 3}}, WithNode("verify"))
	b.Event(TypeNodeOutput, out(`{"status":[{"id":"b1","still_present":false}]}`), WithNode("verify"))
	b.Event(TypeDeltaApplied, deltaFor(`{"status":[{"id":"b1","still_present":false}]}`, Delta{Status: []BugStatus{{ID: "b1", StillPresent: false, Confidence: 0.8}}}), WithNode("verify"))
	b.Event(TypeConverge, ConvergeData{Atom: "all_fixed", Class: OutcomeFixed, Stop: true, Reason: "0 unfixed"})
	b.Event(TypeGate, GateData{Name: "all_fixed", Passed: true})
	b.Event(TypeTransition, TransitionData{From: "verify", To: "done", Gate: "all_fixed", Outcome: OutcomeFixed, Head: "c1"})
	return b
}

func mustFold(t *testing.T, evs []Event) Snapshot {
	t.Helper()
	s, err := Fold(evs)
	if err != nil {
		t.Fatalf("fold: %v", err)
	}
	return s
}

func expectReason(t *testing.T, evs []Event, reason string, seq int64, typ string) {
	t.Helper()
	_, err := Fold(evs)
	fe, ok := err.(*FoldError)
	if !ok {
		t.Fatalf("expected *FoldError %s, got %v", reason, err)
	}
	if fe.Reason != reason || fe.Code != CodeFor(reason) || fe.Seq != seq || fe.Type != typ {
		t.Fatalf("expected {%s %s %d %s} got %+v", CodeFor(reason), reason, seq, typ, fe)
	}
}

// ---- R1: happy path derivations -----------------------------------------------------------------

func TestFoldHappyPath(t *testing.T) {
	b := happyLog()
	s := mustFold(t, b.Events())
	if s.RunID != runA || s.SchemaVersion != SchemaVersion || s.Seq != int64(len(b.Events())) || s.Workflow != "sdlc-loop" {
		t.Fatalf("identity: %+v", s)
	}
	if s.State != "done" || s.Outcome != OutcomeFixed || s.Iteration != 0 || s.PrevUnfixed != nil {
		t.Fatalf("terminal: state=%s outcome=%s iter=%d prev=%v", s.State, s.Outcome, s.Iteration, s.PrevUnfixed)
	}
	if s.Head != "c1" || s.FixEntryHead != "head0000" || s.TreeHash != "t1" || s.StateKind != "" {
		t.Fatalf("heads: %+v", s)
	}
	if len(s.Findings) != 1 || len(s.Confirmed) != 1 || len(s.AllFound) != 1 || s.AllFound[0].ID != "b1" || s.Unfixed != 0 || len(s.Status) != 1 {
		t.Fatalf("bugs: %+v", s)
	}
	if s.Tokens != (TokenTotals{Input: 13, Output: 5}) {
		t.Fatalf("tokens: %+v", s.Tokens)
	}
	if !reflect.DeepEqual(s.NodesRun, []string{"discover", "adjudicate", "fix", "verify"}) {
		t.Fatalf("nodes run: %v", s.NodesRun)
	}
	for _, k := range []string{"discover@0", "adjudicate@0", "fix@0", "verify@0"} {
		if !s.Applied[k] || s.NodeOutputs[k] == nil {
			t.Fatalf("missing applied/output for %s", k)
		}
	}
	if s.StopReason != "all_fixed" || s.LastError != nil || len(s.Warnings) != 0 || s.MockTainted || s.OverflowHandled {
		t.Fatalf("misc: %+v", s)
	}
	// nil-free JSON
	js := marshalCanonical(s)
	if strings.Contains(string(js), "null") && !strings.Contains(string(js), `"prev_unfixed":null`) {
		t.Fatalf("unexpected null in snapshot json: %s", js)
	}
	for _, key := range []string{`"lineage":[]`, `"goldens":[]`, `"warnings":[]`, `"vars":{}`} {
		if !strings.Contains(string(js), key) {
			t.Fatalf("snapshot json missing %s: %s", key, js)
		}
	}
}

func TestFoldInitDerivations(t *testing.T) {
	b := NewBuilder(runA)
	d := baseInit()
	d.InitialKind = KindAgentEdit
	d.Mock = "scen"
	d.Vars = map[string]string{"JUDGE": "j"}
	d.Goldens = []Golden{{Comment: "g"}}
	d.AllowedCmds = nil // Builder defaults it to an empty list
	b.Init(d)
	s := mustFold(t, b.Events())
	if s.AllowedCmds == nil || len(s.AllowedCmds) != 0 {
		t.Fatalf("allowed cmds must fold to an empty list")
	}
	if s.FixEntryHead != "head0000" || s.StateKind != KindAgentEdit || s.Mock != "scen" || s.Vars["JUDGE"] != "j" || len(s.Goldens) != 1 || s.CreatedAt.IsZero() {
		t.Fatalf("init: %+v", s)
	}
	// a forked child with an agent-edit initial state does NOT get FixEntryHead from init
	c := NewBuilder(runB)
	d.RunID = runB
	d.ParentRunID = runA
	d.Lineage = []string{runA}
	d.ForkedAtSeq = 1
	c.Init(d)
	cs := mustFold(t, c.Events())
	if cs.FixEntryHead != "" || cs.ParentRunID != runA || len(cs.Lineage) != 1 {
		t.Fatalf("forked init: %+v", cs)
	}
}

func TestFoldStateNameNeverDerivesOutcome(t *testing.T) {
	b := NewBuilder(runA)
	b.Init(baseInit())
	b.Event(TypeTransition, TransitionData{From: "discover", To: "failed", Gate: "g", Head: "h"})
	b.Event(TypeRecord, RecordData{Name: "still-going", Data: json.RawMessage(`{}`)})
	s := mustFold(t, b.Events())
	if s.Outcome != "" || s.State != "failed" {
		t.Fatalf("state named failed must not be terminal: %+v", s)
	}
	b2 := NewBuilder(runA)
	b2.Init(baseInit())
	b2.Event(TypeTransition, TransitionData{From: "discover", To: "failed", Gate: "g", Outcome: OutcomeFailed, Head: "h"})
	if s := mustFold(t, b2.Events()); s.Outcome != OutcomeFailed {
		t.Fatalf("payload outcome must be applied")
	}
	b3 := NewBuilder(runA)
	b3.Init(baseInit())
	b3.Event(TypeTransition, TransitionData{From: "discover", To: "done", Outcome: Outcome("bogus"), Head: "h"})
	expectReason(t, b3.Events(), ReasonBadOutcome, 2, TypeTransition)
}

func TestFoldUnfixedFailClosed(t *testing.T) {
	b := NewBuilder(runA)
	b.Init(baseInit())
	raw := `{"c":3}`
	b.Event(TypeNodeOutput, out(raw), WithNode("adjudicate"))
	b.Event(TypeDeltaApplied, deltaFor(raw, Delta{Confirmed: []Bug{bug("b1"), bug("b2"), bug("b3")}}), WithNode("adjudicate"))
	s := mustFold(t, b.Events())
	if s.Unfixed != 3 {
		t.Fatalf("unstatused bugs must count as unfixed: %d", s.Unfixed)
	}
	raw2 := `{"s":1}`
	b.Event(TypeNodeOutput, out(raw2), WithNode("verify"))
	b.Event(TypeDeltaApplied, deltaFor(raw2, Delta{Status: []BugStatus{{ID: "b1"}, {ID: "b2", StillPresent: true}, {ID: "b3", StillPresent: false}}}), WithNode("verify"))
	s = mustFold(t, b.Events())
	if s.Unfixed != 1 {
		t.Fatalf("StillPresent true=1 false=2 → unfixed 1, got %d", s.Unfixed)
	}
	// AllFound grows after the last Status; Unfixed recomputed on the Confirmed-only delta
	raw3 := `{"c":4}`
	b.Event(TypeTransition, TransitionData{From: "discover", To: "discover", Loop: true, Head: "h"})
	b.Event(TypeNodeOutput, out(raw3), WithNode("adjudicate"))
	b.Event(TypeDeltaApplied, deltaFor(raw3, Delta{Confirmed: []Bug{bug("b4")}}), WithNode("adjudicate"))
	s = mustFold(t, b.Events())
	if s.Unfixed != 2 || len(s.AllFound) != 4 || *s.PrevUnfixed != 1 || s.Iteration != 1 {
		t.Fatalf("after growth: unfixed=%d all=%d prev=%v iter=%d", s.Unfixed, len(s.AllFound), s.PrevUnfixed, s.Iteration)
	}
	// a Status that omits b4 is incomplete
	raw4 := `{"s":2}`
	b.Event(TypeNodeOutput, out(raw4), WithNode("verify"))
	b.Event(TypeDeltaApplied, deltaFor(raw4, Delta{Status: []BugStatus{{ID: "b1"}, {ID: "b2"}, {ID: "b3"}}}), WithNode("verify"))
	expectReason(t, b.Events(), ReasonStatusIncomplete, int64(len(b.Events())), TypeDeltaApplied)
}

func TestFoldStatusRules(t *testing.T) {
	mk := func(status []BugStatus) []Event {
		b := NewBuilder(runA)
		b.Init(baseInit())
		b.Event(TypeNodeOutput, out(`{"c":1}`), WithNode("a"))
		b.Event(TypeDeltaApplied, deltaFor(`{"c":1}`, Delta{Confirmed: []Bug{bug("b1"), bug("b2")}}), WithNode("a"))
		b.Event(TypeNodeOutput, out(`{"s":1}`), WithNode("v"))
		b.Event(TypeDeltaApplied, deltaFor(`{"s":1}`, Delta{Status: status}), WithNode("v"))
		return b.Events()
	}
	expectReason(t, mk([]BugStatus{{ID: "b1"}, {ID: "zz"}}), ReasonStatusNotSubset, 5, TypeDeltaApplied)
	expectReason(t, mk([]BugStatus{{ID: "b1"}, {ID: "b1"}}), ReasonStatusDuplicate, 5, TypeDeltaApplied)
	expectReason(t, mk([]BugStatus{{ID: "b1"}}), ReasonStatusIncomplete, 5, TypeDeltaApplied)
	s := mustFold(t, mk([]BugStatus{{ID: "b2"}, {ID: "b1", StillPresent: true}}))
	if s.Unfixed != 1 || s.Status[0].ID != "b2" {
		t.Fatalf("status replace: %+v", s.Status)
	}
	// AllFound union: duplicates by ID are not re-added, first-seen order kept
	b := NewBuilder(runA)
	b.Init(baseInit())
	b.Event(TypeNodeOutput, out(`{"c":1}`), WithNode("a"))
	b.Event(TypeDeltaApplied, deltaFor(`{"c":1}`, Delta{Confirmed: []Bug{bug("z"), bug("m"), bug("a")}}), WithNode("a"))
	b.Event(TypeTransition, TransitionData{From: "discover", To: "discover", Loop: true, Head: "h"})
	b.Event(TypeNodeOutput, out(`{"c":2}`), WithNode("a"))
	b.Event(TypeDeltaApplied, deltaFor(`{"c":2}`, Delta{Confirmed: []Bug{bug("m"), bug("q")}}), WithNode("a"))
	s = mustFold(t, b.Events())
	ids := []string{}
	for _, x := range s.AllFound {
		ids = append(ids, x.ID)
	}
	if !reflect.DeepEqual(ids, []string{"z", "m", "a", "q"}) || len(s.Confirmed) != 2 {
		t.Fatalf("union order: %v confirmed=%d", ids, len(s.Confirmed))
	}
}

// R1: node@iter keying — a node re-run in iteration 2 with interleaved keys.
func TestFoldNodeIterKeying(t *testing.T) {
	b := NewBuilder(runA)
	b.Init(baseInit())
	b.Event(TypeNodeOutput, out(`{"i":0}`), WithNode("n"))
	b.Event(TypeLLMCall, LLMCallData{Kind: "k", Index: 0, Verdict: json.RawMessage(`{}`)}, WithNode("n"))
	b.Event(TypeLLMCall, LLMCallData{Kind: "k", Index: 0, Verdict: json.RawMessage(`{}`)}, WithNode("m"))
	b.Event(TypeLLMCall, LLMCallData{Kind: "k", Index: 1, Verdict: json.RawMessage(`{}`)}, WithNode("n"))
	b.Event(TypeDeltaApplied, deltaFor(`{"i":0}`, Delta{}), WithNode("n"))
	b.Event(TypeTransition, TransitionData{From: "discover", To: "discover", Loop: true, Head: "h"})
	b.Event(TypeNodeOutput, out(`{"i":1}`), WithNode("n"))
	b.Event(TypeLLMCall, LLMCallData{Kind: "k", Index: 0, Verdict: json.RawMessage(`{}`)}, WithNode("n"))
	b.Event(TypeDeltaApplied, deltaFor(`{"i":1}`, Delta{}), WithNode("n"))
	s := mustFold(t, b.Events())
	if !s.Applied["n@0"] || !s.Applied["n@1"] || string(s.NodeOutputs["n@0"]) != `{"i":0}` || string(s.NodeOutputs["n@1"]) != `{"i":1}` {
		t.Fatalf("keys: %+v %+v", s.Applied, s.NodeOutputs)
	}
	if !reflect.DeepEqual(s.NodesRun, []string{"n"}) { // NodesRun records outputs, not calls
		t.Fatalf("nodes run %v", s.NodesRun)
	}
	// a global (non-keyed) index counter would reject Index 0 for m@0 above; a node-keyed one would
	// reject Index 0 for n@1. Both accepted → keyed by node@iter. Now a wrong index is rejected:
	b.Event(TypeLLMCall, LLMCallData{Kind: "k", Index: 5, Verdict: json.RawMessage(`{}`)}, WithNode("n"))
	expectReason(t, b.Events(), ReasonStamp, int64(len(b.Events())), TypeLLMCall)
}

func TestFoldOutputRules(t *testing.T) {
	b := NewBuilder(runA)
	b.Init(baseInit())
	b.Event(TypeNodeOutput, out(`{"v":1}`), WithNode("n"))
	b.Event(TypeNodeOutput, out(`{"v":2}`), WithNode("n")) // replace before delta: last wins
	s := mustFold(t, b.Events())
	if string(s.NodeOutputs["n@0"]) != `{"v":2}` || s.Applied["n@0"] {
		t.Fatalf("last output before delta wins: %+v", s)
	}
	b.Event(TypeDeltaApplied, deltaFor(`{"v":2}`, Delta{}), WithNode("n"))
	mustFold(t, b.Events())
	b.Event(TypeNodeOutput, out(`{"v":3}`), WithNode("n"))
	expectReason(t, b.Events(), ReasonOutputAfterDelta, 5, TypeNodeOutput)

	b2 := NewBuilder(runA)
	b2.Init(baseInit())
	b2.Event(TypeDeltaApplied, deltaFor(`{}`, Delta{}), WithNode("n"))
	expectReason(t, b2.Events(), ReasonDeltaWithoutOutput, 2, TypeDeltaApplied)

	b3 := NewBuilder(runA)
	b3.Init(baseInit())
	b3.Event(TypeNodeOutput, out(`{}`), WithNode("n"))
	b3.Event(TypeDeltaApplied, deltaFor(`{}`, Delta{}), WithNode("n"))
	b3.Event(TypeDeltaApplied, deltaFor(`{}`, Delta{}), WithNode("n"))
	expectReason(t, b3.Events(), ReasonSecondDelta, 4, TypeDeltaApplied)

	b4 := NewBuilder(runA)
	b4.Init(baseInit())
	b4.Event(TypeNodeOutput, out(`{"a":1}`), WithNode("n"))
	b4.Event(TypeDeltaApplied, deltaFor(`{"a":2}`, Delta{}), WithNode("n"))
	expectReason(t, b4.Events(), ReasonOutputHash, 3, TypeDeltaApplied)

	// canonicalization: a non-canonical output hashes and stores canonically
	b5 := NewBuilder(runA)
	b5.Init(baseInit())
	b5.Event(TypeNodeOutput, out(`{ "a" : "<" }`), WithNode("n"))
	b5.Event(TypeDeltaApplied, DeltaAppliedData{OutputHash: OutputHash([]byte(`{"a":"<"}`))}, WithNode("n"))
	s = mustFold(t, b5.Events())
	if string(s.NodeOutputs["n@0"]) != `{"a":"<"}` {
		t.Fatalf("stored output must be canonical: %s", s.NodeOutputs["n@0"])
	}
	// node scope
	b6 := NewBuilder(runA)
	b6.Init(baseInit())
	b6.Event(TypeNodeOutput, out(`{}`))
	expectReason(t, b6.Events(), ReasonNodeScope, 2, TypeNodeOutput)
	b7 := NewBuilder(runA)
	b7.Init(baseInit())
	b7.Event(TypeNeedsInput, EmptyData{})
	expectReason(t, b7.Events(), ReasonNodeScope, 2, TypeNeedsInput)
}

func TestFoldTransitionLoopAndBaseline(t *testing.T) {
	b := NewBuilder(runA)
	b.Init(baseInit())
	b.Event(TypeNodeOutput, out(`{"f":1}`), WithNode("d"))
	b.Event(TypeDeltaApplied, deltaFor(`{"f":1}`, Delta{Findings: []Finding{{IssueText: "f"}}, Confirmed: []Bug{bug("b1")}}), WithNode("d"))
	b.Event(TypeGate, GateData{Name: "x", Passed: false, Error: &GateError{Code: "E", Gate: "x"}})
	s := mustFold(t, b.Events())
	if s.LastError == nil || s.LastError.Code != "E" {
		t.Fatalf("gate failure must set LastError")
	}
	b.Event(TypeTransition, TransitionData{From: "discover", To: "discover", Loop: true, Head: "h1"})
	s = mustFold(t, b.Events())
	if s.Iteration != 1 || s.PrevUnfixed == nil || *s.PrevUnfixed != 1 || len(s.Findings) != 0 || len(s.Confirmed) != 0 || len(s.AllFound) != 1 || s.LastError != nil || s.Head != "h1" {
		t.Fatalf("loop transition: %+v", s)
	}
	// second loop re-copies PrevUnfixed
	b.Event(TypeNodeOutput, out(`{"s":1}`), WithNode("v"))
	b.Event(TypeDeltaApplied, deltaFor(`{"s":1}`, Delta{Status: []BugStatus{{ID: "b1", StillPresent: false}}}), WithNode("v"))
	b.Event(TypeTransition, TransitionData{From: "discover", To: "discover", Loop: true, Head: "h2"})
	s = mustFold(t, b.Events())
	if *s.PrevUnfixed != 0 || s.Iteration != 2 {
		t.Fatalf("second loop: prev=%d iter=%d", *s.PrevUnfixed, s.Iteration)
	}
	// loop transition stamped with the current (not new) iteration is rejected
	b.Event(TypeTransition, TransitionData{From: "discover", To: "discover", Loop: true, Head: "h3"}, WithIter(2))
	expectReason(t, b.Events(), ReasonStamp, int64(len(b.Events())), TypeTransition)

	// agent-edit entry sets FixEntryHead only when not copied
	c := NewBuilder(runA)
	c.Init(baseInit())
	c.Event(TypeTransition, TransitionData{From: "discover", To: "fix", ToKind: KindAgentEdit, Head: "hh"})
	if s := mustFold(t, c.Events()); s.FixEntryHead != "hh" || s.StateKind != KindAgentEdit {
		t.Fatalf("fix entry: %+v", s)
	}
	c.Event(TypeTransition, TransitionData{From: "fix", To: "verify", Head: "hh2"})
	if s := mustFold(t, c.Events()); s.FixEntryHead != "hh" || s.StateKind != "" {
		t.Fatalf("non-edit transition must leave FixEntryHead: %+v", s)
	}
}

func TestFoldFixBaseline(t *testing.T) {
	mk := func() *Builder {
		b := NewBuilder(runA)
		b.Init(baseInit())
		b.Event(TypeTransition, TransitionData{From: "discover", To: "fix", ToKind: KindAgentEdit, Head: "h0"}, WithOrigin(&Origin{RunID: runB, Seq: 2, Version: 1, Hash: "x"}))
		return b
	}
	// copied transition never sets FixEntryHead (init below has ParentRunID so provenance passes)
	bp := NewBuilder(runA)
	d := baseInit()
	d.ParentRunID = runB
	d.Lineage = []string{runB}
	d.ForkedAtSeq = 2
	bp.Init(d)
	bp.Event(TypeTransition, TransitionData{From: "discover", To: "fix", ToKind: KindAgentEdit, Head: "h0"}, WithOrigin(&Origin{RunID: runB, Seq: 2, Version: 1, Hash: "x"}))
	s := mustFold(t, bp.Events())
	if s.FixEntryHead != "" || s.StateKind != KindAgentEdit {
		t.Fatalf("copied transition must not set FixEntryHead: %+v", s)
	}
	// tree then fix_baseline with the same head sets it
	bp.Event(TypeTree, TreeData{Head: "h9", TreeHash: "t"})
	bp.Event(TypeFixBaseline, FixBaselineData{Head: "h9"})
	s = mustFold(t, bp.Events())
	if s.FixEntryHead != "h9" || s.Head != "h9" {
		t.Fatalf("fix_baseline: %+v", s)
	}
	// order: no preceding tree
	b2 := NewBuilder(runA)
	d2 := baseInit()
	d2.ParentRunID = runB
	d2.Lineage = []string{runB}
	d2.ForkedAtSeq = 2
	b2.Init(d2)
	b2.Event(TypeTransition, TransitionData{From: "discover", To: "fix", ToKind: KindAgentEdit, Head: "h0"}, WithOrigin(&Origin{RunID: runB, Seq: 2, Version: 1, Hash: "x"}))
	b2.Event(TypeFixBaseline, FixBaselineData{Head: "h0"})
	expectReason(t, b2.Events(), ReasonFixBaselineOrder, 3, TypeFixBaseline)
	// head mismatch
	b3 := NewBuilder(runA)
	b3.Init(d2)
	b3.Event(TypeTransition, TransitionData{From: "discover", To: "fix", ToKind: KindAgentEdit, Head: "h0"}, WithOrigin(&Origin{RunID: runB, Seq: 2, Version: 1, Hash: "x"}))
	b3.Event(TypeTree, TreeData{Head: "h9", TreeHash: "t"})
	b3.Event(TypeFixBaseline, FixBaselineData{Head: "h8"})
	expectReason(t, b3.Events(), ReasonFixBaselineHead, 4, TypeFixBaseline)
	// wrong kind
	b4 := NewBuilder(runA)
	b4.Init(baseInit())
	b4.Event(TypeTree, TreeData{Head: "h9", TreeHash: "t"})
	b4.Event(TypeFixBaseline, FixBaselineData{Head: "h9"})
	expectReason(t, b4.Events(), ReasonFixBaselineKind, 3, TypeFixBaseline)
	_ = mk
}

func TestFoldTerminalAndMisc(t *testing.T) {
	base := func() *Builder {
		b := NewBuilder(runA)
		b.Init(baseInit())
		b.Event(TypeTransition, TransitionData{From: "discover", To: "done", Outcome: OutcomeOverflow, Head: "h"})
		return b
	}
	allowed := map[string]any{
		TypeOverflowHandler: OverflowHandlerData{Name: "notify", Argv: []string{"/bin/notify"}, ExitCode: 0},
		TypeCmdCall:         CmdCallData{Name: "notify", Argv: []string{"/bin/notify"}},
		TypeWarn:            WarnData{Code: "W"},
		TypeRecord:          RecordData{Name: "n", Data: json.RawMessage(`{}`)},
		TypeTokens:          TokenTotals{Input: 1},
		TypeTree:            TreeData{Head: "h", TreeHash: "t"},
		TypeFork:            ForkData{ChildRunID: runB, AtSeq: 1},
	}
	for typ, data := range allowed {
		b := base()
		b.Event(typ, data)
		s := mustFold(t, b.Events())
		if typ == TypeOverflowHandler && !s.OverflowHandled {
			t.Fatalf("overflow_handler must set OverflowHandled")
		}
		if typ == TypeWarn && (len(s.Warnings) != 1 || s.Warnings[0] != "W") {
			t.Fatalf("warn must append code")
		}
		if typ == TypeTokens && s.Tokens.Input != 1 {
			t.Fatalf("tokens after terminal must accumulate")
		}
	}
	rejected := map[string]any{
		TypeTransition:   TransitionData{From: "done", To: "x", Head: "h"},
		TypeGate:         GateData{Name: "g", Passed: true},
		TypeConverge:     ConvergeData{Atom: "a"},
		TypeNodeOutput:   out(`{}`),
		TypeDeltaApplied: deltaFor(`{}`, Delta{}),
		TypeLLMCall:      LLMCallData{Kind: "k", Verdict: json.RawMessage(`{}`)},
		TypeNeedsInput:   EmptyData{},
		TypeFixBaseline:  FixBaselineData{Head: "h"},
	}
	for typ, data := range rejected {
		b := base()
		b.Event(typ, data, WithNode("n"))
		expectReason(t, b.Events(), ReasonPostTerminal, 3, typ)
	}
	// gate rules
	b := NewBuilder(runA)
	b.Init(baseInit())
	b.Event(TypeGate, GateData{Name: "g", Passed: false})
	expectReason(t, b.Events(), ReasonBadPayload, 2, TypeGate)
	b = NewBuilder(runA)
	b.Init(baseInit())
	b.Event(TypeGate, GateData{Name: "g", Passed: false, Error: &GateError{Code: "E", Gate: "g"}})
	b.Event(TypeGate, GateData{Name: "g2", Passed: true})
	if s := mustFold(t, b.Events()); s.LastError == nil || s.LastError.Code != "E" {
		t.Fatalf("passing gate is a no-op for LastError")
	}
	// unsanctioned command
	b = NewBuilder(runA)
	b.Init(baseInit())
	b.Event(TypeCmdCall, CmdCallData{Name: "rogue", Argv: []string{"/x"}})
	expectReason(t, b.Events(), ReasonUnsanctionedCmd, 2, TypeCmdCall)
	b = NewBuilder(runA)
	b.Init(baseInit())
	b.Event(TypeOverflowHandler, OverflowHandlerData{Name: "rogue"})
	expectReason(t, b.Events(), ReasonUnsanctionedCmd, 2, TypeOverflowHandler)
	// converge stop sets StopReason; non-stop does not
	b = NewBuilder(runA)
	b.Init(baseInit())
	b.Event(TypeConverge, ConvergeData{Atom: "budget", Class: OutcomeOverflow, Stop: false})
	if s := mustFold(t, b.Events()); s.StopReason != "" {
		t.Fatalf("non-stop converge must not set StopReason")
	}
	b.Event(TypeConverge, ConvergeData{Atom: "budget", Class: OutcomeOverflow, Stop: true, Reason: "r"})
	if s := mustFold(t, b.Events()); s.StopReason != "budget" {
		t.Fatalf("stop converge must set StopReason")
	}
	// warnings cap
	b = NewBuilder(runA)
	b.Init(baseInit())
	for i := 0; i < MaxWarnings; i++ {
		b.Event(TypeWarn, WarnData{Code: "W"})
	}
	mustFold(t, b.Events())
	b.Event(TypeWarn, WarnData{Code: "W"})
	expectReason(t, b.Events(), ReasonOversize, int64(MaxWarnings+2), TypeWarn)
}

// R1b: no-op rows compare snapshots ignoring Seq.
func TestFoldNoOps(t *testing.T) {
	b := happyLog()
	// remove the terminal transition so more events are legal
	evs := b.Events()[:len(b.Events())-1]
	before := mustFold(t, evs)
	b2 := NewBuilder(runA)
	b2.events, b2.lines = evs, nil
	for _, e := range evs {
		b2.lines = append(b2.lines, marshalCanonical(e))
	}
	b2.state, b2.iter = "verify", 0
	steps := []func() Event{
		func() Event { return b2.Event(TypeRecord, RecordData{Name: "x", Data: json.RawMessage(`{"k":1}`)}) },
		func() Event { return b2.Event(TypeNeedsInput, EmptyData{}, WithNode("verify")) },
		func() Event { return b2.Event(TypeCmdCall, CmdCallData{Name: "notify", Argv: []string{"/bin/notify"}}) },
		func() Event { return b2.Event(TypeFork, ForkData{ChildRunID: runB, AtSeq: 3}) },
		func() Event { return b2.Event(TypeGate, GateData{Name: "g", Passed: true}) },
	}
	for _, step := range steps {
		e := step()
		after := mustFold(t, b2.Events())
		if !SnapshotEqualIgnoringSeq(before, after) || after.Seq != e.Seq {
			t.Fatalf("%s must be a no-op (seq %d)", e.Type, e.Seq)
		}
		if e.Type == TypeNeedsInput && !reflect.DeepEqual(after.NodesRun, before.NodesRun) {
			t.Fatalf("needs_input must not touch NodesRun")
		}
		before = after
	}
}

// R2: sequence-level invariants.
func TestFoldSequenceInvariants(t *testing.T) {
	if _, err := Fold(nil); err == nil || err.(*FoldError).Reason != ReasonEmpty || err.(*FoldError).Code != CodeAuditEmpty {
		t.Fatalf("empty: %v", err)
	}
	b := NewBuilder(runA)
	b.Init(baseInit())
	b.Event(TypeTree, TreeData{Head: "h", TreeHash: "t"}, WithVersion(2))
	expectReason(t, b.Events(), ReasonVersion, 2, TypeTree)
	b = NewBuilder(runA)
	b.Init(baseInit(), WithVersion(0))
	expectReason(t, b.Events(), ReasonVersion, 1, TypeInit)

	b = NewBuilder(runA)
	b.Event(TypeTree, TreeData{Head: "h", TreeHash: "t"})
	expectReason(t, b.Events(), ReasonFirstNotInit, 1, TypeTree)

	b = NewBuilder(runA)
	b.Init(baseInit())
	b.Init(baseInit())
	expectReason(t, b.Events(), ReasonSecondInit, 2, TypeInit)

	b = NewBuilder(runA)
	b.Init(baseInit())
	b.Event(TypeTree, TreeData{Head: "h", TreeHash: "t"}, WithSeq(5))
	expectReason(t, b.Events(), ReasonSeqGap, 5, TypeTree)

	b = NewBuilder(runA)
	b.Init(baseInit())
	b.Event(TypeTree, TreeData{Head: "h", TreeHash: "t"}, WithType("bogus"))
	expectReason(t, b.Events(), ReasonUnknownType, 2, "bogus")

	b = NewBuilder(runA)
	b.Init(baseInit())
	b.Event(TypeTree, TreeData{Head: "h", TreeHash: "t"}, WithRawData(`{"head":"h","extra":1}`))
	expectReason(t, b.Events(), ReasonBadPayload, 2, TypeTree)
	b = NewBuilder(runA)
	b.Init(baseInit())
	b.Event(TypeTree, TreeData{Head: "h", TreeHash: "t"}, WithRawData(`not json`))
	expectReason(t, b.Events(), ReasonBadPayload, 2, TypeTree)
	b = NewBuilder(runA)
	b.Init(baseInit(), WithRawData(`{"run_id":"`+runA+`","initial_state":"s","created_at":"2026-08-26T00:00:00Z","workflow":"w","allowed_cmds":[],"goldens":[],"lineage":[],"vars":{},"vars":{}}`))
	expectReason(t, b.Events(), ReasonBadPayload, 1, TypeInit)

	// stamps
	b = NewBuilder(runA)
	b.Init(baseInit(), WithState("discover"))
	expectReason(t, b.Events(), ReasonInitStamp, 1, TypeInit)
	b = NewBuilder(runA)
	b.Init(baseInit(), WithIter(1))
	expectReason(t, b.Events(), ReasonInitStamp, 1, TypeInit)
	b = NewBuilder(runA)
	b.Init(baseInit(), WithAt(time0()))
	expectReason(t, b.Events(), ReasonInitStamp, 1, TypeInit)
	b = NewBuilder(runA)
	b.Init(baseInit())
	b.Event(TypeTree, TreeData{Head: "h", TreeHash: "t"}, WithIter(1))
	expectReason(t, b.Events(), ReasonStamp, 2, TypeTree)
	b = NewBuilder(runA)
	b.Init(baseInit())
	b.Event(TypeTree, TreeData{Head: "h", TreeHash: "t"}, WithState("elsewhere"))
	expectReason(t, b.Events(), ReasonStamp, 2, TypeTree)
	b = NewBuilder(runA)
	b.Init(baseInit())
	b.Event(TypeTransition, TransitionData{From: "elsewhere", To: "x", Head: "h"})
	expectReason(t, b.Events(), ReasonStamp, 2, TypeTransition)
	b = NewBuilder(runA)
	b.Init(baseInit())
	b.Event(TypeTree, TreeData{Head: "h", TreeHash: "t"}, WithAt(time0()))
	expectReason(t, b.Events(), ReasonStamp, 2, TypeTree)
	// init payload mismatches
	b = NewBuilder(runA)
	d := baseInit()
	d.RunID = "mrv-" + strings.Repeat("z", 12)
	b.Init(d)
	if s := mustFold(t, b.Events()); s.RunID != d.RunID {
		t.Fatalf("run id from payload")
	}
}

func time0() (t timeT) { return }

// R10: provenance and mock stamping.
func TestFoldProvenance(t *testing.T) {
	parent := happyLog()
	pevs := parent.Events()
	child := NewBuilder(runA)
	cevs := child.Copy(pevs, 2, 7, runB, nil)
	cs := mustFold(t, cevs)
	ps := mustFold(t, pevs[:7])
	if cs.ParentRunID != runA || cs.ForkedAtSeq != 7 || !reflect.DeepEqual(cs.Lineage, []string{runA}) || cs.RunID != runB {
		t.Fatalf("child identity: %+v", cs)
	}
	// equal except identity fields and FixEntryHead
	ps.RunID, ps.ParentRunID, ps.Lineage, ps.ForkedAtSeq, ps.CreatedAt, ps.FixEntryHead = cs.RunID, cs.ParentRunID, cs.Lineage, cs.ForkedAtSeq, cs.CreatedAt, cs.FixEntryHead
	if !SnapshotEqualIgnoringSeq(ps, cs) {
		t.Fatalf("child prefix fold differs from parent prefix fold:\n%s\n%s", marshalCanonical(ps), marshalCanonical(cs))
	}
	// the child can continue natively
	child.Event(TypeLLMCall, LLMCallData{Kind: "adjudicate", Index: 0, Verdict: json.RawMessage(`{}`)}, WithNode("adjudicate"))
	mustFold(t, child.Events())

	violations := []struct {
		name   string
		mutate func(evs []Event) []Event
		seq    int64
	}{
		{"wrong parent", func(evs []Event) []Event { evs[2].Origin.RunID = runB; return evs }, 3},
		{"origin seq mismatch", func(evs []Event) []Event { evs[2].Origin.Seq = 9; return evs }, 3},
		{"missing origin in range", func(evs []Event) []Event { evs[3].Origin = nil; return evs }, 4},
		{"empty hash", func(evs []Event) []Event { evs[2].Origin.Hash = ""; return evs }, 3},
		{"bad version", func(evs []Event) []Event { evs[2].Origin.Version = 0; return evs }, 3},
	}
	for _, v := range violations {
		evs := v.mutate(fresh(cevs))
		expectReason(t, rechain(evs), ReasonProvenance, v.seq, evs[v.seq-1].Type)
	}
	// a copied init is a second init before provenance is even consulted
	ic := fresh(cevs)
	ic[1].Type = TypeInit
	expectReason(t, rechain(ic), ReasonSecondInit, 2, TypeInit)
	// origin outside the range
	child2 := NewBuilder(runA)
	c2 := child2.Copy(pevs, 2, 3, runB, nil)
	child2.Event(TypeTree, TreeData{Head: "h", TreeHash: "t"}, WithOrigin(&Origin{RunID: runA, Seq: 4, Version: 1, Hash: "x"}))
	expectReason(t, child2.Events(), ReasonProvenance, int64(len(c2)+1), TypeTree)
	// origin with no parent
	b := NewBuilder(runA)
	b.Init(baseInit())
	b.Event(TypeTree, TreeData{Head: "h", TreeHash: "t"}, WithOrigin(&Origin{RunID: runB, Seq: 2, Version: 1, Hash: "x"}))
	expectReason(t, b.Events(), ReasonProvenance, 2, TypeTree)
	// ForkedAtSeq larger than copies is accepted by Fold (spec-3 precondition)
	child3 := NewBuilder(runA)
	child3.Copy(pevs, 2, 3, runB, func(d *InitData) { d.ForkedAtSeq = 10 })
	mustFold(t, child3.Events())

	// mock stamping
	m := NewBuilder(runA)
	d := baseInit()
	d.Mock = "scen"
	m.Init(d)
	m.Event(TypeTree, TreeData{Head: "h", TreeHash: "t"})
	if s := mustFold(t, m.Events()); s.MockTainted || !m.Events()[1].Mock {
		t.Fatalf("mock run events are stamped and not tainted")
	}
	m.Event(TypeTree, TreeData{Head: "h", TreeHash: "t"}, WithMock(false))
	expectReason(t, m.Events(), ReasonMockStamp, 3, TypeTree)
	// copies into a mock child are exempt
	mc := NewBuilder(runA)
	mc.Copy(pevs, 2, 3, runB, func(d *InitData) { d.Mock = "scen" })
	if s := mustFold(t, mc.Events()); s.MockTainted {
		t.Fatalf("copied events into a mock child are exempt from mock_stamp")
	}
	// a mock event in a non-mock run taints it
	n := NewBuilder(runA)
	n.Init(baseInit())
	n.Event(TypeTree, TreeData{Head: "h", TreeHash: "t"}, WithMock(true))
	if s := mustFold(t, n.Events()); !s.MockTainted {
		t.Fatalf("mock event in a real run must taint")
	}
}

// R12: caps at-cap accepted, one-over rejected, measured on canonical bytes.
func TestFoldCaps(t *testing.T) {
	esc := func(n int) string { return strings.Repeat("<", n) } // 1 byte each, canonical keeps literal
	nl := func(n int) string { return strings.Repeat("\n", n) } // 2 canonical bytes each
	type row struct {
		name  string
		build func(n int) []Event
		max   int
	}
	rows := []row{
		{"IssueText", func(n int) []Event {
			b := NewBuilder(runA)
			b.Init(baseInit())
			b.Event(TypeNodeOutput, out(`{}`), WithNode("n"))
			b.Event(TypeDeltaApplied, deltaFor(`{}`, Delta{Findings: []Finding{{IssueText: esc(n)}}}), WithNode("n"))
			return b.Events()
		}, MaxText - 2},
		{"IssueText-escaped", func(n int) []Event {
			b := NewBuilder(runA)
			b.Init(baseInit())
			b.Event(TypeNodeOutput, out(`{}`), WithNode("n"))
			b.Event(TypeDeltaApplied, deltaFor(`{}`, Delta{Findings: []Finding{{IssueText: nl(n)}}}), WithNode("n"))
			return b.Events()
		}, (MaxText - 2) / 2},
		{"Desc", func(n int) []Event {
			b := NewBuilder(runA)
			b.Init(baseInit())
			b.Event(TypeNodeOutput, out(`{}`), WithNode("n"))
			b.Event(TypeDeltaApplied, deltaFor(`{}`, Delta{Confirmed: []Bug{{ID: "b", Desc: esc(n)}}}), WithNode("n"))
			return b.Events()
		}, MaxDesc - 2},
		{"Short-name", func(n int) []Event {
			b := NewBuilder(runA)
			b.Init(baseInit())
			b.Event(TypeWarn, WarnData{Code: esc(n)})
			return b.Events()
		}, MaxShort - 2},
		{"WarnDetail", func(n int) []Event {
			b := NewBuilder(runA)
			b.Init(baseInit())
			b.Event(TypeWarn, WarnData{Code: "c", Detail: esc(n)})
			return b.Events()
		}, MaxText - 2},
		{"GateDetail", func(n int) []Event {
			b := NewBuilder(runA)
			b.Init(baseInit())
			b.Event(TypeGate, GateData{Name: "g", Passed: false, Error: &GateError{Code: "E", Gate: "g", Detail: esc(n)}})
			return b.Events()
		}, MaxDetail - 2},
		{"Stderr", func(n int) []Event {
			b := NewBuilder(runA)
			b.Init(baseInit())
			b.Event(TypeCmdCall, CmdCallData{Name: "notify", Argv: []string{"/bin/notify"}, Stderr: esc(n)})
			return b.Events()
		}, MaxStderr - 2},
		{"TreeStatus", func(n int) []Event {
			b := NewBuilder(runA)
			b.Init(baseInit())
			b.Event(TypeTree, TreeData{Head: "h", TreeHash: "t", Status: esc(n)})
			return b.Events()
		}, MaxDetail - 2},
		{"Goldens-count", func(n int) []Event {
			b := NewBuilder(runA)
			d := baseInit()
			for i := 0; i < n; i++ {
				d.Goldens = append(d.Goldens, Golden{Comment: "c"})
			}
			b.Init(d)
			return b.Events()
		}, MaxGoldens},
		{"Vars-count", func(n int) []Event {
			b := NewBuilder(runA)
			d := baseInit()
			d.Vars = map[string]string{}
			for i := 0; i < n; i++ {
				d.Vars[strings.Repeat("k", i+1)] = "v"
			}
			b.Init(d)
			return b.Events()
		}, MaxVars},
		{"AllowedCmds-count", func(n int) []Event {
			b := NewBuilder(runA)
			d := baseInit()
			d.AllowedCmds = nil
			for i := 0; i < n; i++ {
				d.AllowedCmds = append(d.AllowedCmds, AllowedCmd{Name: "c", Argv: []string{"/c"}, FileHashes: map[string]string{}})
			}
			b.Init(d)
			return b.Events()
		}, MaxAllowedCmds},
		{"Argv-count", func(n int) []Event {
			b := NewBuilder(runA)
			d := baseInit()
			d.AllowedCmds = []AllowedCmd{{Name: "c", Argv: make([]string, n), FileHashes: map[string]string{}}}
			b.Init(d)
			return b.Events()
		}, MaxArgv},
		{"FileHashes-count", func(n int) []Event {
			b := NewBuilder(runA)
			d := baseInit()
			fh := map[string]string{}
			for i := 0; i < n; i++ {
				fh[strings.Repeat("p", i+1)] = "h"
			}
			d.AllowedCmds = []AllowedCmd{{Name: "c", Argv: []string{"/c"}, FileHashes: fh}}
			b.Init(d)
			return b.Events()
		}, MaxFileHashes},
		{"Env-count", func(n int) []Event {
			b := NewBuilder(runA)
			d := baseInit()
			d.AllowedCmds = []AllowedCmd{{Name: "c", Argv: []string{"/c"}, FileHashes: map[string]string{}, Env: make([]string, n)}}
			b.Init(d)
			return b.Events()
		}, MaxEnv},
		{"Delta-list-count", func(n int) []Event {
			b := NewBuilder(runA)
			b.Init(baseInit())
			b.Event(TypeNodeOutput, out(`{}`), WithNode("n"))
			fs := make([]Finding, n)
			for i := range fs {
				fs[i].IssueText = "f"
			}
			b.Event(TypeDeltaApplied, deltaFor(`{}`, Delta{Findings: fs}), WithNode("n"))
			return b.Events()
		}, MaxDeltaList},
		{"Payload", func(n int) []Event {
			b := NewBuilder(runA)
			b.Init(baseInit())
			b.Event(TypeRecord, RecordData{Name: "r", Data: json.RawMessage(`"` + esc(n) + `"`)})
			return b.Events()
		}, MaxPayload - len(`{"name":"r","data":""}`)},
	}
	for _, r := range rows {
		evs := r.build(r.max)
		if _, err := Fold(evs); err != nil {
			t.Fatalf("%s at cap must be accepted: %v", r.name, err)
		}
		over := r.build(r.max + 1)
		_, err := Fold(over)
		fe, ok := err.(*FoldError)
		if !ok || fe.Reason != ReasonOversize {
			t.Fatalf("%s one over cap must be oversize, got %v", r.name, err)
		}
	}
}

// R12 (continued): every remaining per-field cap has an over row (the at-cap side is the generic
// MaxShort/MaxText behavior already pinned above).
func TestFoldCapsPerField(t *testing.T) {
	big := strings.Repeat("x", MaxShort)    // MaxShort+2 canonical → over MaxShort
	bigText := strings.Repeat("x", MaxText) // over MaxText
	bigDetail := strings.Repeat("x", MaxDetail)
	bigStderr := strings.Repeat("x", MaxStderr)
	initWith := func(edit func(*InitData)) []Event {
		b := NewBuilder(runA)
		d := baseInit()
		edit(&d)
		b.Init(d)
		return b.Events()
	}
	ev := func(typ string, data any, o ...Override) []Event {
		b := NewBuilder(runA)
		b.Init(baseInit())
		b.Event(typ, data, o...)
		return b.Events()
	}
	delta := func(d Delta) []Event {
		b := NewBuilder(runA)
		b.Init(baseInit())
		b.Event(TypeNodeOutput, out(`{}`), WithNode("n"))
		b.Event(TypeDeltaApplied, deltaFor(`{}`, d), WithNode("n"))
		return b.Events()
	}
	cases := map[string][]Event{
		"init.workflow":        initWith(func(d *InitData) { d.Workflow = big }),
		"init.lineage":         initWith(func(d *InitData) { d.Lineage = []string{big} }),
		"init.vars.key":        initWith(func(d *InitData) { d.Vars = map[string]string{big: "v"} }),
		"init.vars.value":      initWith(func(d *InitData) { d.Vars = map[string]string{"k": bigText} }),
		"init.golden.comment":  initWith(func(d *InitData) { d.Goldens = []Golden{{Comment: bigText}} }),
		"init.golden.severity": initWith(func(d *InitData) { d.Goldens = []Golden{{Comment: "c", Severity: big}} }),
		"init.cmd.name":        initWith(func(d *InitData) { d.AllowedCmds = []AllowedCmd{{Name: big, Argv: []string{"/c"}}} }),
		"init.cmd.argv":        initWith(func(d *InitData) { d.AllowedCmds = []AllowedCmd{{Name: "c", Argv: []string{bigText}}} }),
		"init.cmd.filehash": initWith(func(d *InitData) {
			d.AllowedCmds = []AllowedCmd{{Name: "c", Argv: []string{"/c"}, FileHashes: map[string]string{big: "h"}}}
		}),
		"init.cmd.env":       initWith(func(d *InitData) { d.AllowedCmds = []AllowedCmd{{Name: "c", Argv: []string{"/c"}, Env: []string{big}}} }),
		"tree.head":          ev(TypeTree, TreeData{Head: big, TreeHash: "t"}),
		"delta.finding.file": delta(Delta{Findings: []Finding{{IssueText: "i", File: big}}}),
		"delta.bug.id":       delta(Delta{Confirmed: []Bug{{ID: big, Desc: "d"}}}),
		"delta.status.id":    delta(Delta{Confirmed: []Bug{{ID: "b"}}, Status: []BugStatus{{ID: big}}}),
		"delta.commit":       delta(Delta{Commit: big}),
		"llm.model":          ev(TypeLLMCall, LLMCallData{Kind: "k", Model: big, Verdict: json.RawMessage(`{}`)}, WithNode("n")),
		"cmd.name":           ev(TypeCmdCall, CmdCallData{Name: big, Argv: []string{"/c"}}),
		"cmd.argv":           ev(TypeCmdCall, CmdCallData{Name: "notify", Argv: []string{bigText}}),
		"cmd.stdout":         ev(TypeCmdCall, CmdCallData{Name: "notify", Argv: []string{"/c"}, Stdout: bigDetail}),
		"overflow.name":      ev(TypeOverflowHandler, OverflowHandlerData{Name: big}),
		"overflow.stdout":    ev(TypeOverflowHandler, OverflowHandlerData{Name: "notify", Stdout: bigDetail}),
		"overflow.stderr":    ev(TypeOverflowHandler, OverflowHandlerData{Name: "notify", Stderr: bigStderr}),
		"gate.name":          ev(TypeGate, GateData{Name: big, Passed: true}),
		"gate.error.code":    ev(TypeGate, GateData{Name: "g", Passed: false, Error: &GateError{Code: big, Gate: "g"}}),
		"converge.atom":      ev(TypeConverge, ConvergeData{Atom: big}),
		"converge.reason":    ev(TypeConverge, ConvergeData{Atom: "a", Reason: bigText}),
		"transition.to":      ev(TypeTransition, TransitionData{From: "discover", To: State(big), Head: "h"}),
		"fixbaseline.head":   ev(TypeFixBaseline, FixBaselineData{Head: big}),
		"record.name":        ev(TypeRecord, RecordData{Name: big, Data: json.RawMessage(`{}`)}),
		"fork.child":         ev(TypeFork, ForkData{ChildRunID: big}),
		"envelope.node":      ev(TypeNeedsInput, EmptyData{}, WithNode(big)),
		"envelope.state":     ev(TypeTree, TreeData{Head: "h", TreeHash: "t"}, WithState(State(big))),
	}
	for name, evs := range cases {
		_, err := Fold(evs)
		fe, ok := err.(*FoldError)
		if !ok || fe.Reason != ReasonOversize {
			t.Fatalf("%s: expected oversize, got %v", name, err)
		}
	}
	// a raw init without vars/lineage keys folds with empty containers
	b := NewBuilder(runA)
	b.Init(baseInit(), WithRawData(`{"run_id":"`+runA+`","created_at":"2026-08-26T00:00:00Z","workflow":"w","allowed_cmds":[],"goldens":[],"initial_state":"s","repo_root":"/r","work_dir":"/r","base_sha":"b","head":"h","workflow_hash":"x","repo_mode":"advisory"}`))
	s := mustFold(t, b.Events())
	if s.Vars == nil || s.Lineage == nil || len(s.Vars) != 0 || len(s.Lineage) != 0 {
		t.Fatalf("nil containers must fold to empty: %+v", s)
	}
}

// R11 is folded into TestFoldSequenceInvariants (version rows).

// R3: order and determinism.
func TestFoldOrderAndDeterminism(t *testing.T) {
	b := NewBuilder(runA)
	b.Init(baseInit())
	for i, n := range []string{"zeta", "alpha", "mid"} {
		raw := `{"i":` + string(rune('0'+i)) + `}`
		b.Event(TypeNodeOutput, out(raw), WithNode(n))
		b.Event(TypeDeltaApplied, deltaFor(raw, Delta{Confirmed: []Bug{bug(n)}}), WithNode(n))
	}
	for _, w := range []string{"w3", "w1", "w2"} {
		b.Event(TypeWarn, WarnData{Code: w})
	}
	s1 := mustFold(t, b.Events())
	s2 := mustFold(t, b.Events())
	if !reflect.DeepEqual(s1.NodesRun, []string{"zeta", "alpha", "mid"}) || !reflect.DeepEqual(s1.Warnings, []string{"w3", "w1", "w2"}) {
		t.Fatalf("order: %v %v", s1.NodesRun, s1.Warnings)
	}
	if string(marshalCanonical(s1)) != string(marshalCanonical(s2)) {
		t.Fatalf("fold not deterministic")
	}
	// FoldFull agrees with Fold and exposes ChainHead empty
	fs, err := FoldFull(b.Events())
	if err != nil || fs.ChainHead != "" || string(marshalCanonical(fs.Snapshot)) != string(marshalCanonical(s1)) {
		t.Fatalf("FoldFull: %v %q", err, fs.ChainHead)
	}
}

// R4: per-prefix composition and mutation classes.
func TestFoldPrefixesAndMutations(t *testing.T) {
	evs := happyLog().Events()
	var st FoldState
	for i, ev := range evs {
		next, err := Apply(st, ev)
		if err != nil {
			t.Fatalf("apply %d: %v", i, err)
		}
		full := mustFold(t, evs[:i+1])
		if string(marshalCanonical(next.Snapshot)) != string(marshalCanonical(full)) {
			t.Fatalf("prefix %d: Apply and Fold disagree", i)
		}
		// Apply must not mutate its input
		if i > 0 && string(marshalCanonical(st.Snapshot)) != string(marshalCanonical(mustFold(t, evs[:i]))) {
			t.Fatalf("Apply mutated its input at %d", i)
		}
		st = next
	}
	// deleting any event breaks the chain/seq before re-chaining
	for i := 1; i < len(evs)-1; i++ {
		cut := append(fresh(evs[:i]), evs[i+1:]...)
		if _, err := Fold(cut); err == nil || err.(*FoldError).Reason != ReasonSeqGap {
			t.Fatalf("deletion at %d must be seq_gap, got %v", i, err)
		}
	}
	// after re-chaining, deleting a node_output yields delta_without_output; deleting a warn is a valid different snapshot
	idxOut := indexOf(evs, TypeNodeOutput)
	cut := rechain(append(fresh(evs[:idxOut]), evs[idxOut+1:]...))
	if _, err := Fold(cut); err == nil || err.(*FoldError).Reason != ReasonDeltaWithoutOutput {
		t.Fatalf("re-chained deletion of node_output: %v", err)
	}
	// duplicating the terminal transition fails the From stamp (state is already done)
	dup := rechain(append(fresh(evs), evs[len(evs)-1]))
	if _, err := Fold(dup); err == nil || err.(*FoldError).Reason != ReasonStamp {
		t.Fatalf("duplicate terminal: %v", err)
	}
	// duplicating a post-terminal-legal event after the terminal transition is fine; a node event is not
	post := rechain(append(fresh(evs), evs[indexOf(evs, TypeNeedsInput)]))
	post[len(post)-1].State = "done"
	if _, err := Fold(rechain(post)); err == nil || err.(*FoldError).Reason != ReasonPostTerminal {
		t.Fatalf("node event after terminal: %v", err)
	}
	// swapping a gate and its transition changes the state stamp → stamp
	idxT := indexOf(evs, TypeTransition)
	sw := fresh(evs)
	sw[idxT-1], sw[idxT] = sw[idxT], sw[idxT-1]
	if _, err := Fold(rechain(sw)); err == nil || err.(*FoldError).Reason != ReasonStamp {
		t.Fatalf("swap: %v", err)
	}
}

// ---- helpers ------------------------------------------------------------------------------------

type timeT = time.Time

func fresh(evs []Event) []Event {
	out := make([]Event, len(evs))
	for i, e := range evs {
		out[i] = e
		if e.Origin != nil {
			o := *e.Origin
			out[i].Origin = &o
		}
	}
	return out
}

// rechain renumbers Seq contiguously and recomputes Prev over canonical lines, keeping everything else.
func rechain(evs []Event) []Event {
	prev := ""
	for i := range evs {
		evs[i].Seq = int64(i) + 1
		evs[i].Prev = prev
		prev = LineHash(marshalCanonical(evs[i]))
	}
	return evs
}

func indexOf(evs []Event, typ string) int {
	for i, e := range evs {
		if e.Type == typ {
			return i
		}
	}
	return -1
}

func TestFoldTokensNegativeAndNextIndex(t *testing.T) {
	b := NewBuilder(runA)
	b.Init(baseInit())
	b.Event(TypeNodeOutput, out(`{}`), WithNode("n"))
	b.Event(TypeLLMCall, LLMCallData{Kind: "k", Model: "m", Index: 0, Verdict: json.RawMessage(`{}`), Tokens: TokenTotals{Input: 5}}, WithNode("n"))
	st, err := FoldFull(b.Events())
	if err != nil {
		t.Fatal(err)
	}
	if st.NextIndex("n@0") != 1 || st.NextIndex("zzz@0") != 0 {
		t.Fatalf("NextIndex: %d", st.NextIndex("n@0"))
	}
	for name, tok := range map[string]TokenTotals{"input": {Input: -1}, "cache_read": {CacheRead: -1}, "cache_create": {CacheCreate: -1}, "output": {Output: -1}, "reasoning": {Reasoning: -1}} {
		b2 := NewBuilder(runA)
		b2.Init(baseInit())
		b2.Event(TypeTokens, tok)
		if _, err := Fold(b2.Events()); err == nil || err.(*FoldError).Reason != ReasonTokensNegative {
			t.Errorf("tokens %s: %v", name, err)
		}
		b3 := NewBuilder(runA)
		b3.Init(baseInit())
		b3.Event(TypeNodeOutput, out(`{}`), WithNode("n"))
		b3.Event(TypeLLMCall, LLMCallData{Kind: "k", Model: "m", Index: 0, Verdict: json.RawMessage(`{}`), Tokens: tok}, WithNode("n"))
		if _, err := Fold(b3.Events()); err == nil || err.(*FoldError).Reason != ReasonTokensNegative {
			t.Errorf("llm_call %s: %v", name, err)
		}
	}
	if (TokenTotals{}).Negative() || (TokenTotals{}).TooLarge() {
		t.Fatal("zero is neither negative nor too large")
	}
	for name, tok := range map[string]TokenTotals{"input": {Input: MaxTokenCounter + 1}, "cache_read": {CacheRead: MaxTokenCounter + 1}, "cache_create": {CacheCreate: MaxTokenCounter + 1}, "output": {Output: MaxTokenCounter + 1}, "reasoning": {Reasoning: MaxTokenCounter + 1}} {
		b2 := NewBuilder(runA)
		b2.Init(baseInit())
		b2.Event(TypeTokens, tok)
		if _, err := Fold(b2.Events()); err == nil || err.(*FoldError).Reason != ReasonTokensTooLarge {
			t.Errorf("tokens too large %s: %v", name, err)
		}
		b3 := NewBuilder(runA)
		b3.Init(baseInit())
		b3.Event(TypeNodeOutput, out(`{}`), WithNode("n"))
		b3.Event(TypeLLMCall, LLMCallData{Kind: "k", Model: "m", Index: 0, Verdict: json.RawMessage(`{}`), Tokens: tok}, WithNode("n"))
		if _, err := Fold(b3.Events()); err == nil || err.(*FoldError).Reason != ReasonTokensTooLarge {
			t.Errorf("llm_call too large %s: %v", name, err)
		}
	}
	// at the cap is accepted
	b4 := NewBuilder(runA)
	b4.Init(baseInit())
	b4.Event(TypeTokens, TokenTotals{Input: MaxTokenCounter})
	if _, err := Fold(b4.Events()); err != nil {
		t.Fatal(err)
	}
}

func TestWorkflowSourceFolds(t *testing.T) {
	d := baseInit()
	d.WorkflowSource = "path"
	b := NewBuilder(runA)
	b.Init(d)
	snap, err := Fold(b.Events())
	if err != nil || snap.WorkflowSource != "path" {
		t.Fatalf("workflow_source must fold: %v %+v", err, snap.WorkflowSource)
	}
	if snap.Clone().WorkflowSource != "path" {
		t.Fatal("Clone must copy WorkflowSource")
	}
	// legacy init without the field decodes and folds as ""
	b2 := NewBuilder(runA)
	b2.Init(baseInit())
	if snap, err := Fold(b2.Events()); err != nil || snap.WorkflowSource != "" {
		t.Fatalf("legacy: %v %q", err, snap.WorkflowSource)
	}
	// cap: over MaxShort is refused at fold
	d.WorkflowSource = strings.Repeat("x", MaxShort+1)
	b3 := NewBuilder(runA)
	b3.Init(d)
	if _, err := Fold(b3.Events()); err == nil {
		t.Fatal("over-cap workflow_source must be refused")
	}
}

// A node_output whose payload carries no output must be refused. The comment
// asserting p.Output is always a valid sub-document did not hold: Canonical(nil)
// fails, the error was discarded, and the fold stored a present key with an
// invalid document. The matching delta_applied then passed too, because
// OutputHash(nil) is the sha256 of the empty string — so a malformed event
// validated all the way through the chain and only surfaced much later, when
// kind.Decode met a nil document.
func TestFoldRefusesANodeOutputWithNoOutput(t *testing.T) {
	// The payload omits "output" entirely, so the decoded RawMessage is nil.
	// Canonical(nil) fails; the fold used to discard that error, store the empty
	// result under a present key, and let the matching delta_applied through —
	// OutputHash(nil) being the sha256 of the empty string.
	b := NewBuilder(runA)
	b.Init(baseInit())
	b.Event(TypeTree, TreeData{Head: "head0000", TreeHash: "t0", Status: ""})
	b.Event(TypeNeedsInput, EmptyData{}, WithNode("discover"))
	b.Event(TypeNodeOutput, out(`{"findings":[]}`), WithNode("discover"))

	evs := b.Events()
	evs[len(evs)-1].Data = json.RawMessage(`{}`) // no "output" key at all

	if _, err := Fold(evs); err == nil {
		t.Fatal("a node_output whose payload omits output was accepted")
	}
}
