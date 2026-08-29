package machine

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/gate"
	"github.com/dsifry/metareview/internal/fsm/run"
	"github.com/dsifry/metareview/internal/fsm/workflow"
	"github.com/dsifry/metareview/workflows"
)

// sdlcDone drives a one-iteration sdlc run to DONE fixed.
func sdlcDone(h *harness, o InitOptions) *Machine {
	h.t.Helper()
	h.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
	m := h.mustInit(o)
	h.advance(m)
	h.record(m, "discover", findings(2))
	h.advance(m) // → adjudicate
	h.advance(m) // → fix
	h.advance(m) // needs input
	h.record(m, "fix", `{"commit":"`+shaFix+`","summary":"fixed"}`)
	h.advance(m) // → verify
	if r := h.advance(m); r.Status != StatusDone || r.Outcome != run.OutcomeFixed {
		h.t.Fatalf("sdlcDone: %+v", r)
	}
	return m
}

func seqOfTransitionInto(t *testing.T, evs []run.Event, to run.State, iter int) (int64, string) {
	t.Helper()
	for _, ev := range evs {
		if ev.Type != run.TypeTransition || ev.Iter != iter {
			continue
		}
		var td run.TransitionData
		_ = json.Unmarshal(ev.Data, &td)
		if td.To == to {
			return ev.Seq, td.Head
		}
	}
	t.Fatalf("no transition into %s@%d", to, iter)
	return 0, ""
}

func intp(i int) *int { return &i }

func decode[T any](t *testing.T, ev run.Event) T {
	t.Helper()
	var d T
	if err := json.Unmarshal(ev.Data, &d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestF1ForkCheckpointAndCopy(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	p := sdlcDone(h, InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
	pevs := h.events(p)
	praw, _ := h.sidecar.Read(p.runID, SidecarWorkflow)
	wantSeq, wantHead := seqOfTransitionInto(t, pevs, "adjudicate", 0)

	child, res, err := p.Fork(ctx, ForkOptions{From: "adjudicate"})
	if err != nil {
		t.Fatal(err)
	}
	if res.ForkedAtSeq != wantSeq || res.Copied != int(wantSeq-1) || res.ChildRunID != child.RunID() || res.CmdsSHA256 != "" || len(res.DroppedVars) != 0 {
		t.Fatalf("result: %+v", res)
	}
	cevs := h.events(child)
	// child init literal
	cd := decode[run.InitData](t, cevs[0])
	pd := decode[run.InitData](t, pevs[0])
	if cd.ParentRunID != p.runID || len(cd.Lineage) != 1 || cd.Lineage[0] != p.runID || cd.ForkedAtSeq != wantSeq || cd.WorkDir != pd.WorkDir || cd.Head != wantHead || cd.Head != shaHead || cd.WorkflowHash != pd.WorkflowHash || cd.WorkflowSource != pd.WorkflowSource || cd.Mock != "" || cd.CmdsSHA256 != "" || cd.Vars["JUDGE"] != "gpt-5.2" || cd.RunID != child.runID {
		t.Fatalf("child init: %+v", cd)
	}
	// copied prefix equals parent [2..seq] modulo origin and prev
	_, plines, _ := h.store.EventsWithLines(p.runID)
	for i := int64(2); i <= wantSeq; i++ {
		pe, ce := pevs[i-1], cevs[i-1]
		if ce.Origin == nil || ce.Origin.RunID != p.runID || ce.Origin.Seq != pe.Seq || ce.Origin.Version != 1 || ce.Origin.Hash != run.LineHash(plines[i-1]) {
			t.Fatalf("origin at %d: %+v", i, ce.Origin)
		}
		if string(ce.Data) != string(pe.Data) || !ce.At.Equal(pe.At.Time) || ce.State != pe.State || ce.Iter != pe.Iter || ce.Node != pe.Node || ce.Mock != pe.Mock || ce.Type != pe.Type {
			t.Fatalf("copy at %d differs", i)
		}
	}
	// rebaseline tree with checkpoint stamps, no fix_baseline for adjudicate
	tree := cevs[wantSeq]
	if tree.Type != run.TypeTree || tree.State != "adjudicate" || tree.Iter != 0 || tree.Mock || tree.Origin != nil {
		t.Fatalf("tree stamps: %+v", tree)
	}
	td := decode[run.TreeData](t, tree)
	if td.Head != shaHead || td.TreeHash != gate.TreeHash(shaHead, "t1") {
		t.Fatalf("tree data: %+v", td)
	}
	if int64(len(cevs)) != wantSeq+1 {
		t.Fatalf("child length %d", len(cevs))
	}
	// child sidecar == parent's; child folds and is not incomplete
	if craw, _ := h.sidecar.Read(child.runID, SidecarWorkflow); string(craw) != string(praw) {
		t.Fatal("sidecar bytes")
	}
	if child.View().Snapshot.State != "adjudicate" || child.View().Snapshot.Head != shaHead {
		t.Fatalf("child view: %+v", child.View().Snapshot)
	}
	// parent: byte-identical plus one fork event
	pafter := h.events(p)
	if len(pafter) != len(pevs)+1 {
		t.Fatalf("parent grew by %d", len(pafter)-len(pevs))
	}
	for i := range pevs {
		if string(run.MarshalCanonical(pafter[i])) != string(run.MarshalCanonical(pevs[i])) {
			t.Fatalf("parent event %d changed", i+1)
		}
	}
	fd := decode[run.ForkData](t, pafter[len(pafter)-1])
	if pafter[len(pafter)-1].Type != run.TypeFork || fd.ChildRunID != child.runID || fd.AtSeq != wantSeq {
		t.Fatalf("fork event: %+v", fd)
	}
	// child's first advance re-runs adjudicate from index 0 (the executor is invoked; discover is not)
	if r := h.advance(child); r.Status != StatusAdvanced || r.To != "fix" {
		t.Fatalf("child advance: %+v", r)
	}
	// fix checkpoint gets a fix_baseline
	fixSeq, _ := seqOfTransitionInto(t, pevs, "fix", 0)
	c2, res2, err := p.Fork(ctx, ForkOptions{From: "fix"})
	if err != nil || res2.ForkedAtSeq != fixSeq {
		t.Fatalf("fork at fix: %v %+v", err, res2)
	}
	c2evs := h.events(c2)
	if c2evs[len(c2evs)-1].Type != run.TypeFixBaseline || c2evs[len(c2evs)-2].Type != run.TypeTree {
		t.Fatalf("fix baseline order: %v", h.types(c2))
	}
	if c2.View().Snapshot.FixEntryHead != shaHead {
		t.Fatalf("FixEntryHead: %+v", c2.View().Snapshot)
	}
	// restart: initial with --at-iter 0 → seq 1, nothing copied
	c3, res3, err := p.Fork(ctx, ForkOptions{From: "discover", AtIter: intp(0)})
	if err != nil || res3.ForkedAtSeq != 1 || res3.Copied != 0 {
		t.Fatalf("restart: %v %+v", err, res3)
	}
	if evs := h.events(c3); len(evs) != 2 || evs[1].Type != run.TypeTree || evs[1].State != "discover" {
		t.Fatalf("restart events: %v", h.types(c3))
	}
	// fork of a fork: lineage grows, origin names the immediate parent
	c4, _, err := c2.Fork(ctx, ForkOptions{From: "adjudicate"})
	if err != nil {
		t.Fatal(err)
	}
	c4d := decode[run.InitData](t, h.events(c4)[0])
	if len(c4d.Lineage) != 2 || c4d.Lineage[0] != p.runID || c4d.Lineage[1] != c2.runID || h.events(c4)[1].Origin.RunID != c2.runID {
		t.Fatalf("lineage: %+v", c4d)
	}
	// refusals
	if _, _, err := p.Fork(ctx, ForkOptions{From: "nope"}); !errs.Is(err, CodeCheckpointNotFound) || errs.As(err).Fields["reason"] != "unknown_state" {
		t.Fatalf("unknown state: %v", err)
	}
	if _, _, err := p.Fork(ctx, ForkOptions{From: "done"}); !errs.Is(err, CodeCheckpointNotFound) || errs.As(err).Fields["reason"] != "terminal_state" {
		t.Fatalf("terminal state: %v", err)
	}
	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate", AtIter: intp(5)}); !errs.Is(err, CodeCheckpointNotFound) || errs.As(err).Fields["at_iter"] != "5" {
		t.Fatalf("at_iter out of range: %v", err)
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, err := p.Fork(cctx, ForkOptions{From: "adjudicate"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("ctx: %v", err)
	}
	h.store.torn = true
	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate"}); !errs.Is(err, run.CodeAuditTorn) {
		t.Fatalf("torn: %v", err)
	}
	h.store.torn = false
}

// twoIterParent builds a sdlc run whose head advances at the iteration-0 fix and loops once.
func twoIterParent(t *testing.T, h *harness) *Machine {
	t.Helper()
	const h1 = "1111111111111111111111111111111111111111"
	iter := 0
	h.reg.execs["still-present"].fn = func(in ExecInput) (json.RawMessage, error) {
		var st []run.BugStatus
		for i, b := range in.Snap.AllFound {
			if err := llmCall(in, i, 5); err != nil {
				return nil, err
			}
			st = append(st, run.BugStatus{ID: b.ID, StillPresent: iter == 0 && i == 0, Confidence: 1})
		}
		return json.RawMessage(run.MarshalCanonical(run.Delta{Status: st})), nil
	}
	h.git.def.Counts = map[string]int{shaHead + ".." + h1: 1, h1 + ".." + h1: 1}
	h.git.def.Diffs[shaBase+".."+h1] = "DIFF2"
	h.git.def.Diffs[h1+".."+h1] = "DIFF2"
	m := h.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
	h.advance(m)
	h.record(m, "discover", findings(2))
	h.advance(m)
	h.advance(m)
	h.advance(m)
	h.git.def.HeadSHA = h1 // the fix commit moves HEAD
	h.record(m, "fix", `{"commit":"`+shaFix+`","summary":"fixed"}`)
	h.advance(m)
	if r := h.advance(m); r.To != "discover" || m.View().Snapshot.Iteration != 1 {
		t.Fatalf("loop: %+v", r)
	}
	iter = 1
	h.advance(m)
	h.record(m, "discover", findings(1))
	h.advance(m)
	h.advance(m)
	h.advance(m)
	h.record(m, "fix", `{"commit":"`+shaFix+`","summary":"fixed"}`)
	h.advance(m)
	if r := h.advance(m); r.Status != StatusDone || r.Outcome != run.OutcomeFixed {
		t.Fatalf("two-iter done: %+v", r)
	}
	return m
}

func TestF1TwoIterationCheckpoints(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	p := twoIterParent(t, h)
	pevs := h.events(p)
	const h1 = "1111111111111111111111111111111111111111"
	adj0, _ := seqOfTransitionInto(t, pevs, "adjudicate", 0)
	adj1, _ := seqOfTransitionInto(t, pevs, "adjudicate", 1)
	disc1, _ := seqOfTransitionInto(t, pevs, "discover", 1)
	// --from adjudicate --at-iter 0 with HEAD at h1 → refused, expected shaHead (the checkpoint's head, not the latest)
	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate", AtIter: intp(0)}); !errs.Is(err, CodeTreeNotAtCheckpoint) || errs.As(err).Fields["expected"] != shaHead || errs.As(err).Fields["got"] != h1 || !strings.Contains(err.Error(), "git worktree add") {
		t.Fatalf("ch discrimination: %v", err)
	}
	// worktree at shaHead accepted; child head/tree.Head == shaHead; not a restart
	h.git.byDir["/wt0"] = &gate.Fake{HeadSHA: shaHead, Common: "/repo/.git", Clean: true, Tree: "t0", Refs: map[string]string{"main": shaBase}, Diffs: map[string]string{shaBase + ".." + shaHead: "DIFF", shaHead + ".." + shaHead: "DIFF"}}
	c, res, err := p.Fork(ctx, ForkOptions{From: "adjudicate", AtIter: intp(0), WorkDir: "/wt0"})
	if err != nil || res.ForkedAtSeq != adj0 || res.Copied == 0 {
		t.Fatalf("at-iter 0: %v %+v", err, res)
	}
	cevs := h.events(c)
	td := decode[run.TreeData](t, cevs[len(cevs)-1])
	if td.Head != shaHead || td.TreeHash != gate.TreeHash(shaHead, "t0") || decode[run.InitData](t, cevs[0]).Head != shaHead || decode[run.InitData](t, cevs[0]).WorkDir != "/wt0" {
		t.Fatalf("child at h0: %+v", td)
	}
	before := len(h.reg.execs["match-then-adjudicate"].calls)
	h.advance(c) // adjudicate re-runs; discover does not
	if len(h.reg.execs["match-then-adjudicate"].calls) != before+1 {
		t.Fatal("adjudicate must re-run on the child")
	}
	// nil at-iter → the latest re-entry
	if _, res, err := p.Fork(ctx, ForkOptions{From: "adjudicate"}); err != nil || res.ForkedAtSeq != adj1 {
		t.Fatalf("latest: %v %+v", err, res)
	}
	if _, res, err := p.Fork(ctx, ForkOptions{From: "discover"}); err != nil || res.ForkedAtSeq != disc1 {
		t.Fatalf("initial nil → latest re-entry: %v %+v", err, res)
	}
	if _, res, err := p.Fork(ctx, ForkOptions{From: "adjudicate", AtIter: intp(1)}); err != nil || res.ForkedAtSeq != adj1 {
		t.Fatalf("at-iter 1: %v %+v", err, res)
	}
	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate", AtIter: intp(2)}); !errs.Is(err, CodeCheckpointNotFound) || errs.As(err).Fields["at_iter"] != "2" {
		t.Fatalf("at-iter 2: %v", err)
	}
	// restart: expected == InitData.Head
	if _, _, err := p.Fork(ctx, ForkOptions{From: "discover", AtIter: intp(0)}); !errs.Is(err, CodeTreeNotAtCheckpoint) || errs.As(err).Fields["expected"] != shaHead {
		t.Fatalf("restart expected init head: %v", err)
	}
}

func TestF2NoCommitRecovery(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	m := h.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
	h.advance(m)
	h.record(m, "discover", findings(1))
	h.advance(m)
	h.advance(m)
	h.advance(m)
	h.record(m, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
	r := h.advance(m)
	if r.Status != StatusGateFailed || r.Gate.Error.Code != gate.CodeNoCommit {
		t.Fatalf("expected ERR_NO_COMMIT: %+v", r)
	}
	adjCalls := len(h.reg.execs["match-then-adjudicate"].calls)
	c, _, err := m.Fork(ctx, ForkOptions{From: "fix"})
	if err != nil {
		t.Fatal(err)
	}
	// copied llm_calls carry Origin; discover/adjudicate are not re-run
	for _, ev := range h.events(c) {
		if ev.Type == run.TypeLLMCall && ev.Origin == nil {
			t.Fatal("copied llm_call without origin")
		}
	}
	if r := h.advance(c); r.Status != StatusNeedsInput || r.NeedsInput.Node != "fix" {
		t.Fatalf("child re-runs fix: %+v", r)
	}
	h.record(c, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
	if r := h.advance(c); r.Status != StatusGateFailed || r.Gate.Error.Code != gate.CodeNoCommit {
		t.Fatalf("negative control: %+v", r)
	}
	c2, _, err := m.Fork(ctx, ForkOptions{From: "fix"})
	if err != nil {
		t.Fatal(err)
	}
	h.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
	h.advance(c2)
	h.record(c2, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
	if r := h.advance(c2); r.To != "verify" {
		t.Fatalf("commit_exists passes on the child: %+v", r)
	}
	if len(h.reg.execs["match-then-adjudicate"].calls) != adjCalls {
		t.Fatal("adjudicate must not re-run")
	}
}

func TestF3JudgeSwapAndFreeze(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	p := sdlcDone(h, InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
	// iter 0 swap at adjudicate is allowed: adjudicate has not run in the copied prefix
	c, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate", Vars: map[string]string{"JUDGE": "b"}})
	if err != nil {
		t.Fatal(err)
	}
	if decode[run.InitData](t, h.events(c)[0]).Vars["JUDGE"] != "b" || c.View().Snapshot.Vars["JUDGE"] != "b" {
		t.Fatal("swapped var must be the child's")
	}
	// after adjudicate ran: frozen, first frozen state in States order is adjudicate (verify also references JUDGE)
	if _, _, err := p.Fork(ctx, ForkOptions{From: "verify", Vars: map[string]string{"JUDGE": "b"}}); !errs.Is(err, CodeVarFrozen) || errs.As(err).Fields["state"] != "adjudicate" || errs.As(err).Fields["name"] != "JUDGE" {
		t.Fatalf("frozen: %v", err)
	}
	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate", Vars: map[string]string{"REVIEWER": "x"}}); !errs.Is(err, CodeVarFrozen) || errs.As(err).Fields["state"] != "discover" {
		t.Fatalf("reviewer frozen: %v", err)
	}
	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate", Vars: map[string]string{"NOPE": "x"}}); !errs.Is(err, workflow.CodeVarUnknown) {
		t.Fatalf("undeclared: %v", err)
	}
	// review-loop: judge swap re-runs adjudicate with the new model
	h2 := newHarness(t)
	rl := h2.mustInit(InitOptions{Workflow: "review-loop", Vars: sdlcVars, Base: "main"})
	h2.advance(rl)
	h2.record(rl, "discover", findings(1))
	h2.advance(rl)
	if r := h2.advance(rl); r.Status != StatusDone {
		t.Fatalf("review-loop: %+v", r)
	}
	rc, _, err := rl.Fork(ctx, ForkOptions{From: "adjudicate", Vars: map[string]string{"JUDGE": "b"}})
	if err != nil {
		t.Fatal(err)
	}
	h2.reg.execs["match-then-adjudicate"].calls = nil
	h2.advance(rc)
	if calls := h2.reg.execs["match-then-adjudicate"].calls; len(calls) != 1 || calls[0].Node.Model != "b" {
		t.Fatalf("swapped model must reach the executor: %+v", calls)
	}
	// calibration parent: no override ok (pins re-applied); override refused
	h3 := newHarness(t)
	cal := sdlcDone(h3, InitOptions{Workflow: "sdlc-loop", Base: "main", Calibration: true})
	if cc, _, err := cal.Fork(ctx, ForkOptions{From: "adjudicate"}); err != nil || !cc.View().Snapshot.Calibration || cc.View().Snapshot.Vars["JUDGE"] == "" {
		t.Fatalf("calibration fork: %v", err)
	}
	if _, _, err := cal.Fork(ctx, ForkOptions{From: "adjudicate", Vars: map[string]string{"JUDGE": "b"}}); !errs.Is(err, workflow.CodeCalibrationPinned) {
		t.Fatalf("calibration override: %v", err)
	}
	// convergence cmd freeze: a var referenced only by a cmd that ran is frozen; one whose cmd never ran is not
	h4 := newHarness(t)
	wf := sdlcWith(t, h4, "cmdvars.yaml", "  any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]\nrepo_mode: advisory",
		"  any: [no_fixation_progress, {cmd: chk}, {max_iterations: 5}]\ncmds:\n  chk: {argv: [bash, -c, $XRAN]}\n  idle: {argv: [bash, -c, $XIDLE]}\non_overflow: idle\nrepo_mode: advisory")
	h4.files[wf] = []byte(strings.Replace(string(h4.files[wf]), "vars:\n", "vars:\n  XRAN: {default: a}\n  XIDLE: {default: b}\n", 1))
	_, sha, _ := workflow.ResolveCmds(mustResolve(t, h4, wf), "/repo", h4.deps.LookPath, h4.deps.FileHash)
	h4.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
	iter := 0
	h4.reg.execs["still-present"].fn = func(in ExecInput) (json.RawMessage, error) {
		var st []run.BugStatus
		for i, b := range in.Snap.AllFound {
			if err := llmCall(in, i, 5); err != nil {
				return nil, err
			}
			st = append(st, run.BugStatus{ID: b.ID, StillPresent: iter == 0, Confidence: 1})
		}
		return json.RawMessage(run.MarshalCanonical(run.Delta{Status: st})), nil
	}
	pm := h4.mustInit(InitOptions{Workflow: wf, Vars: sdlcVars, AllowCustomCmds: sha})
	h4.advance(pm)
	h4.record(pm, "discover", findings(1))
	h4.advance(pm)
	h4.advance(pm)
	h4.advance(pm)
	h4.record(pm, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
	h4.advance(pm)
	if r := h4.advance(pm); r.To != "discover" || countType(h4.events(pm), run.TypeCmdCall) != 1 {
		t.Fatalf("loop with cmd atom: %+v", r)
	}
	if _, _, err := pm.Fork(ctx, ForkOptions{From: "discover", Vars: map[string]string{"XRAN": "z"}, AllowCustomCmds: ""}); !errs.Is(err, CodeVarFrozen) || errs.As(err).Fields["state"] != "verify" {
		t.Fatalf("cmd var frozen with the converge state: %v", err)
	}
	// XIDLE's cmd never ran → not frozen (consent needed for the changed argv)
	_, _, err = pm.Fork(ctx, ForkOptions{From: "discover", Vars: map[string]string{"XIDLE": "z"}})
	if !errs.Is(err, CodeCmdsNotAllowed) || errs.As(err).Fields["cmds_json"] == "" {
		t.Fatalf("unfrozen var changes the consent sha: %v", err)
	}
	newSha := errs.As(err).Fields["sha"]
	if _, res, err := pm.Fork(ctx, ForkOptions{From: "discover", Vars: map[string]string{"XIDLE": "z"}, AllowCustomCmds: newSha}); err != nil || res.CmdsSHA256 != newSha {
		t.Fatalf("with consent: %v %+v", err, res)
	}
}

func TestF4GitPreconditions(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	p := sdlcDone(h, InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate", WorkDir: "rel/dir"}); !errs.Is(err, CodeWorkdirForeign) || errs.As(err).Fields["reason"] != "relative" {
		t.Fatalf("relative: %v", err)
	}
	h.git.byDir["/elsewhere"] = &gate.Fake{HeadSHA: shaHead, Common: "/other/.git", Tree: "t9"}
	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate", WorkDir: "/elsewhere"}); !errs.Is(err, CodeWorkdirForeign) || errs.As(err).Fields["reason"] != "other_repo" {
		t.Fatalf("foreign: %v", err)
	}
	h.git.byDir["/wt"] = &gate.Fake{HeadSHA: shaHead, Common: "/repo/.git", Clean: false, Porcelain: " M x.go\n", Tree: "t2", Refs: map[string]string{"main": shaBase}, Diffs: map[string]string{shaBase + ".." + shaHead: "DIFF", shaHead + ".." + shaHead: "DIFF"}}
	c, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate", WorkDir: "/wt"})
	if err != nil {
		t.Fatal(err)
	}
	td := decode[run.TreeData](t, h.events(c)[len(h.events(c))-1])
	if td.TreeHash != gate.TreeHash(shaHead, "t2") || td.Status != " M x.go\n" || c.View().Snapshot.TreeHash != gate.TreeHash(shaHead, "t2") {
		t.Fatalf("worktree baseline: %+v", td)
	}
	// git failures pass through
	for _, method := range []string{"CommonDir", "Head", "Status", "WorkTree"} {
		h.git.byDir["/wt"] = &failingGit{Git: h.git.def, at: method, err: errors.New("git broke " + method)}
		_, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate", WorkDir: "/wt"})
		if err == nil || !strings.Contains(err.Error(), "git broke") {
			t.Fatalf("%s: %v", method, err)
		}
	}
	delete(h.git.byDir, "/wt")
	// mock parent forks as mock and the child open re-verifies the scenario
	hm := newHarness(t)
	hm.reg.mock = true
	hm.mockHash["/repo/mock"] = strings.Repeat("m", 16)
	pm := sdlcDone(hm, InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main", MockDir: "mock"})
	before := 0
	loads := &before
	orig := hm.deps.MockLoad
	hm.deps.MockLoad = func(d string) (string, error) { *loads++; return orig(d) }
	pm.deps.MockLoad = hm.deps.MockLoad
	cm, _, err := pm.Fork(ctx, ForkOptions{From: "adjudicate"})
	if err != nil || cm.View().Snapshot.Mock != pm.View().Snapshot.Mock || *loads == 0 {
		t.Fatalf("mock fork: %v", err)
	}
	if evs := hm.events(cm); !evs[len(evs)-1].Mock {
		t.Fatal("the fork's own tree must carry the mock stamp")
	}
	hm.reg.mock = false
	if _, _, err := pm.Fork(ctx, ForkOptions{From: "adjudicate"}); !errs.Is(err, CodeMockMismatch) {
		t.Fatalf("registry mismatch: %v", err)
	}
	// fork after fix in the same iteration: FixEntryHead is empty and commit_exists fails closed
	h5 := newHarness(t)
	p5 := sdlcDone(h5, InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
	c5, _, err := p5.Fork(ctx, ForkOptions{From: "verify"})
	if err != nil || c5.View().Snapshot.FixEntryHead != "" {
		t.Fatalf("verify fork: %v %+v", err, c5.View().Snapshot)
	}
	sn := c5.View().Snapshot
	ce, _ := gate.Builtin("commit_exists")
	if gerr := ce(ctx, sn, h5.git.def); gerr == nil || gerr.Code != gate.CodeGateInapplicable {
		t.Fatalf("commit_exists must fail closed: %+v", gerr)
	}
}

func TestF5WorkflowChangeAndCopyValidation(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	p := sdlcDone(h, InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
	raw, _ := workflows.Read("sdlc-loop")
	changed := []byte(strings.Replace(string(raw), "effort: $REV_EFFORT", "effort: high", 1))
	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate", WorkflowBytes: changed}); !errs.Is(err, CodeWorkflowChanged) {
		t.Fatalf("without the flag: %v", err)
	}
	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate", AcceptWorkflowChange: true, WorkflowBytes: []byte(strings.Repeat("#", MaxWorkflowBytes+1))}); !errs.Is(err, CodeWorkflowTooLarge) {
		t.Fatalf("too large: %v", err)
	}
	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate", AcceptWorkflowChange: true, WorkflowBytes: []byte("workflow: [")}); !errs.Is(err, workflow.CodeWorkflowInvalid) {
		t.Fatalf("invalid: %v", err)
	}
	incompatible := map[string]string{
		"name":    strings.Replace(string(raw), "workflow: sdlc-loop", "workflow: other-loop", 1),
		"initial": strings.Replace(string(raw), "states: [discover, adjudicate, fix, verify, done, failed]", "states: [adjudicate, discover, fix, verify, done, failed]", 1),
		"state":   strings.Replace(string(raw), "adjudicate: {kind: match-then-adjudicate, exec: fork,", "adjudicate: {kind: still-present, exec: fork,", 1),
	}
	for reason, body := range incompatible {
		_, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate", AcceptWorkflowChange: true, WorkflowBytes: []byte(body)})
		if !errs.Is(err, CodeWorkflowIncompatible) || errs.As(err).Fields["reason"] != reason {
			t.Fatalf("%s: %v", reason, err)
		}
	}
	// changed exec on a copied state
	execChanged := strings.Replace(string(raw), "discover:   {kind: review-lenses,        exec: subagent,", "discover:   {kind: review-lenses,        exec: inline,", 1)
	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate", AcceptWorkflowChange: true, WorkflowBytes: []byte(execChanged)}); !errs.Is(err, CodeWorkflowIncompatible) || errs.As(err).Fields["state"] != "discover" {
		t.Fatalf("exec: %v", err)
	}
	// no child directory after any refusal
	if list, _ := h.store.List(); len(list) != 1 {
		t.Fatalf("refusals must not create runs: %d", len(list))
	}
	// Byte-identical workflow bytes are not a change: no flag needed, source stays "embedded".
	same, sameRes, err := p.Fork(ctx, ForkOptions{From: "adjudicate", WorkflowBytes: append([]byte(nil), raw...)})
	if err != nil || same.View().Snapshot.State != "adjudicate" {
		t.Fatalf("identical bytes: %v %+v", err, sameRes)
	}
	if src := same.View().Snapshot.WorkflowSource; src != "embedded" {
		t.Fatalf("identical bytes: source %q", src)
	}
	// positive controls: changed gate/model on a not-yet-run state, dropped var → accepted, source path
	accepted := strings.Replace(string(raw), "effort: $REV_EFFORT", "effort: high", 1)
	accepted = strings.Replace(accepted, "  REV_EFFORT:   {default: low}\n", "", 1)
	c, res, err := p.Fork(ctx, ForkOptions{From: "adjudicate", AcceptWorkflowChange: true, WorkflowBytes: []byte(accepted)})
	if err != nil {
		t.Fatal(err)
	}
	cd := decode[run.InitData](t, h.events(c)[0])
	if cd.WorkflowSource != "path" || len(res.DroppedVars) != 1 || res.DroppedVars[0] != "REV_EFFORT" || cd.WorkflowHash == p.View().Snapshot.WorkflowHash {
		t.Fatalf("accepted change: %+v %+v", res, cd)
	}
	if craw, _ := h.sidecar.Read(c.runID, SidecarWorkflow); string(craw) != accepted {
		t.Fatal("child sidecar must be the new bytes")
	}
	// freeze rule evaluated on P's resolved workflow, not the new one
	noJudge := strings.Replace(string(raw), "adjudicate: {kind: match-then-adjudicate, exec: fork,     model: $JUDGE,", "adjudicate: {kind: match-then-adjudicate, exec: fork,     model: fixed-model,", 1)
	if _, _, err := p.Fork(ctx, ForkOptions{From: "verify", AcceptWorkflowChange: true, WorkflowBytes: []byte(noJudge), Vars: map[string]string{"JUDGE": "b"}}); !errs.Is(err, CodeVarFrozen) || errs.As(err).Fields["state"] != "adjudicate" {
		t.Fatalf("freeze on Pw: %v", err)
	}
	// tightened Decode → ERR_COPY_INVALID{decode}, no child dir
	h.reg.kinds["review-lenses"].decode = func(json.RawMessage) (any, error) { return nil, errors.New("tightened") }
	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate"}); !errs.Is(err, CodeCopyInvalid) || errs.As(err).Fields["reason"] != "decode" {
		t.Fatalf("decode: %v", err)
	}
	h.reg.kinds["review-lenses"].decode = nil
	// max_events: the fork's count check, boundary exact
	counted := 0
	pevs := h.events(p)
	adj, _ := seqOfTransitionInto(t, pevs, "adjudicate", 0)
	for _, ev := range pevs[:adj] {
		if run.Counted(ev.Type) {
			counted++
		}
	}
	h.store.maxEvents = counted + 1 // parent prefix + the child's own tree
	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate"}); err != nil {
		t.Fatalf("boundary accepted: %v", err)
	}
	h.store.maxEvents = counted
	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate"}); !errs.Is(err, CodeCopyInvalid) || errs.As(err).Fields["reason"] != "max_events" {
		t.Fatalf("max_events: %v", err)
	}
	h.store.maxEvents = 0
	if list, _ := h.store.List(); len(list) != 4 {
		t.Fatalf("only accepted (or identical-bytes) forks create runs: %d", len(list))
	}
}

func TestF5Escalation(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	failAtFix := func(m *Machine) {
		h.git.def.Counts = map[string]int{}
		h.advance(m)
		if r := h.advance(m); r.Status == StatusNeedsInput {
			h.record(m, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
			h.advance(m)
		}
		if m.View().Snapshot.Outcome != run.OutcomeFailed {
			t.Fatalf("expected failed: %+v", m.View().Snapshot)
		}
	}
	p := h.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
	h.advance(p)
	h.record(p, "discover", findings(1))
	h.advance(p)
	h.advance(p)
	failAtFix(p) // attempt 1, failed
	c1, _, err := p.Fork(ctx, ForkOptions{From: "fix"})
	if err != nil {
		t.Fatal(err)
	}
	failAtFix(c1) // attempt 2, failed — still forkable
	c2, _, err := c1.Fork(ctx, ForkOptions{From: "fix"})
	if err != nil {
		t.Fatal(err)
	}
	failAtFix(c2) // attempt 3, failed → escalated leaf
	if _, _, err := c2.Fork(ctx, ForkOptions{From: "fix"}); !errs.Is(err, CodeRunEscalated) || errs.As(err).Fields["attempt"] != "3" {
		t.Fatalf("escalated: %v", err)
	}
	if _, _, err := c1.Fork(ctx, ForkOptions{From: "fix"}); err != nil {
		t.Fatalf("non-escalated parent still forkable: %v", err)
	}
	// a PASS attempt-3 leaf is forkable; its non-PASS child (attempt 4) escalates
	c2b, _, err := c1.Fork(ctx, ForkOptions{From: "fix"})
	if err != nil {
		t.Fatal(err)
	}
	h.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
	h.advance(c2b)
	h.record(c2b, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
	h.advance(c2b)
	if r := h.advance(c2b); r.Outcome != run.OutcomeFixed {
		t.Fatalf("pass leaf: %+v", r)
	}
	c3, _, err := c2b.Fork(ctx, ForkOptions{From: "fix"})
	if err != nil {
		t.Fatalf("pass leaf forkable: %v", err)
	}
	failAtFix(c3)
	if _, _, err := c3.Fork(ctx, ForkOptions{From: "fix"}); !errs.Is(err, CodeRunEscalated) || errs.As(err).Fields["attempt"] != "4" {
		t.Fatalf("attempt 4: %v", err)
	}
	// a non-terminal attempt-3 parent is forkable
	c3n, _, err := c2b.Fork(ctx, ForkOptions{From: "fix"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := c3n.Fork(ctx, ForkOptions{From: "fix"}); err != nil {
		t.Fatalf("non-terminal attempt-3: %v", err)
	}
}

func TestF6VerifyOrigin(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	p := sdlcDone(h, InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
	c, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate"})
	if err != nil {
		t.Fatal(err)
	}
	clog, _ := h.store.Events(c.runID)
	h.store.events = 0
	checks := VerifyOrigin(ctx, h.store, clog)
	if len(checks) == 0 || h.store.events != 1 {
		t.Fatalf("parent read once: %d reads, %d checks", h.store.events, len(checks))
	}
	for _, ck := range checks {
		if !ck.OK || ck.Reason != "ok" {
			t.Fatalf("expected ok: %+v", ck)
		}
	}
	mutate := func(f func(ev *run.Event)) run.Log {
		l := run.Log{Events: append([]run.Event{}, clog.Events...)}
		o := *l.Events[1].Origin
		l.Events[1].Origin = &o
		f(&l.Events[1])
		return l
	}
	cases := []struct {
		name, want string
		f          func(ev *run.Event)
	}{
		{"parent_missing", "parent_missing", func(ev *run.Event) { ev.Origin.RunID = "mrv-missing-parent-1" }},
		{"version_unavailable", "version_unavailable", func(ev *run.Event) { ev.Origin.Version = 2 }},
		{"hash_mismatch", "hash_mismatch", func(ev *run.Event) { ev.Origin.Hash = "0000" }},
		{"hash seq out of range", "hash_mismatch", func(ev *run.Event) { ev.Origin.Seq = 999 }},
		{"content_mismatch", "content_mismatch", func(ev *run.Event) { ev.Data = json.RawMessage(`{"x":1}`) }},
		{"precedence version before hash", "version_unavailable", func(ev *run.Event) { ev.Origin.Version = 2; ev.Origin.Hash = "0000"; ev.Data = json.RawMessage(`{}`) }},
	}
	for _, cs := range cases {
		got := VerifyOrigin(ctx, h.store, mutate(cs.f))
		if got[0].OK || got[0].Reason != cs.want || got[0].Seq != 2 {
			t.Fatalf("%s: %+v", cs.name, got[0])
		}
	}
	h.store.failOp, h.store.err = "Events", errors.New("disk")
	if got := VerifyOrigin(ctx, h.store, clog); got[0].Reason != "parent_unreadable" {
		t.Fatalf("unreadable: %+v", got[0])
	}
	h.store.failOp = ""
	if got := VerifyOrigin(ctx, h.store, run.Log{Events: clog.Events[:1]}); len(got) != 0 {
		t.Fatal("no origins → no checks")
	}
}

func TestF7DiffRuns(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	p := sdlcDone(h, InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
	c, res, err := p.Fork(ctx, ForkOptions{From: "adjudicate", Vars: map[string]string{"JUDGE": "b"}})
	if err != nil {
		t.Fatal(err)
	}
	// the child's adjudicate rejects finding 0 with a lower confidence and different verdict
	h.reg.execs["match-then-adjudicate"].fn = func(in ExecInput) (json.RawMessage, error) {
		var bugs []run.Bug
		for i, f := range in.Snap.Findings {
			if err := llmCall(in, i, 10); err != nil {
				return nil, err
			}
			bugs = append(bugs, run.Bug{ID: run.BugID(f.IssueText), Desc: f.IssueText, Verdict: "real_but_ungold", Confidence: 0.9})
		}
		return json.RawMessage(run.MarshalCanonical(run.Delta{Confirmed: bugs})), nil
	}
	h.advance(c)
	plog, _ := h.store.Events(p.runID)
	clog, _ := h.store.Events(c.runID)
	decide := func(kind string, v json.RawMessage) Decision {
		var d struct {
			IsReal     *bool   `json:"is_real"`
			Confidence float64 `json:"confidence"`
		}
		_ = json.Unmarshal(v, &d)
		if d.IsReal == nil {
			return Decision{}
		}
		eff := *d.IsReal && d.Confidence >= 0.7
		return Decision{Raw: d.IsReal, Effective: &eff}
	}
	r, err := DiffRuns(plog, clog, decide)
	if err != nil {
		t.Fatal(err)
	}
	if r.A != p.runID || r.B != c.runID || !r.SameWorkflow || r.CommonPrefixSeq != res.ForkedAtSeq || r.Outcomes[0] != "fixed" || r.Outcomes[1] != "" {
		t.Fatalf("report: %+v", r)
	}
	if len(r.Calls) == 0 || len(r.Transitions) == 0 {
		t.Fatalf("rows: %+v", r)
	}
	// determinism and sort order
	r2, _ := DiffRuns(plog, clog, decide)
	if string(run.MarshalCanonical(r)) != string(run.MarshalCanonical(r2)) {
		t.Fatal("deterministic")
	}
	for i := 1; i < len(r.Calls); i++ {
		x, y := r.Calls[i-1], r.Calls[i]
		// Equal input hashes within one node@iter are legal — several calls of one
		// kind against the same input — and are ordered by an unexported
		// occurrence counter, so only a strict decrease is a failure here.
		if x.Node > y.Node ||
			(x.Node == y.Node && x.Iter > y.Iter) ||
			(x.Node == y.Node && x.Iter == y.Iter && x.Kind > y.Kind) ||
			(x.Node == y.Node && x.Iter == y.Iter && x.Kind == y.Kind && x.InputHash > y.InputHash) {
			t.Fatalf("sort order at %d", i)
		}
	}
	// hand-built logs pin the row semantics
	mk := func(id string, calls []run.LLMCallData, ats []int) run.Log {
		b := run.NewBuilder(id)
		d := run.InitData{RunID: id, Workflow: "w", WorkflowHash: "h", Vars: map[string]string{}, RepoMode: "advisory", AllowedCmds: []run.AllowedCmd{}, RepoRoot: "/r", WorkDir: "/r", BaseSHA: shaBase, Head: shaHead, InitialState: "s", Lineage: []string{}, Goldens: []run.Golden{}}
		b.Init(d)
		for i, c := range calls {
			b.Event(run.TypeLLMCall, c, run.WithNode("n"), run.WithIter(ats[i]), run.WithState("s"))
		}
		return run.Log{Events: b.Events()}
	}
	bp := func(v bool) *bool { return &v }
	raw := func(real bool, conf float64) json.RawMessage {
		return json.RawMessage(run.MarshalCanonical(map[string]any{"is_real": real, "confidence": conf}))
	}
	a := mk("mrv-diff-a-0000001", []run.LLMCallData{
		{Kind: "adjudicate", Model: "m1", Index: 0, InputHash: "i1", Verdict: raw(true, 0.9), Confidence: 0.9},
		{Kind: "adjudicate", Model: "m1", Index: 1, InputHash: "i2", Verdict: raw(true, 0.9), Confidence: 0.9},
		{Kind: "adjudicate", Model: "m1", Index: 2, InputHash: "i3", Verdict: json.RawMessage(`null`)},
		{Kind: "adjudicate", Model: "m1", Index: 3, InputHash: "i4", Verdict: raw(true, 0.9), Confidence: 0.9, Error: "ERR_X"},
		{Kind: "adjudicate", Model: "m1", Index: 4, InputHash: "i5", Verdict: raw(true, 0.9), Confidence: 0.9},
	}, []int{0, 0, 0, 0, 0})
	b := mk("mrv-diff-b-0000001", []run.LLMCallData{
		{Kind: "adjudicate", Model: "m2", Index: 0, InputHash: "i2", Verdict: raw(true, 0.8), Confidence: 0.8},                 // decision same, confidence differs
		{Kind: "adjudicate", Model: "m2", Index: 1, InputHash: "i3", Verdict: raw(false, 0.5), Confidence: 0.5},                // nil vs false
		{Kind: "adjudicate", Model: "m2", Index: 2, InputHash: "i4", Verdict: raw(true, 0.9), Confidence: 0.9, Error: "ERR_X"}, // identical errors never Same
		{Kind: "adjudicate", Model: "m2", Index: 3, InputHash: "i9", Verdict: raw(true, 0.9), Confidence: 0.9},                 // one-sided
		{Kind: "adjudicate", Model: "m2", Index: 4, InputHash: "i1", Verdict: raw(true, 0.6), Confidence: 0.6},                 // raw same, effective flips; aligned by hash
	}, []int{0, 0, 0, 0, 0})
	rep, err := DiffRuns(a, b, decide)
	if err != nil {
		t.Fatal(err)
	}
	byHash := map[string]CallRow{}
	for _, row := range rep.Calls {
		byHash[row.InputHash] = row
	}
	if r1 := byHash["i1"]; !r1.RawSame || r1.DecisionSame || r1.ConfidenceSame || r1.Same || r1.A.Index != 0 || r1.B.Index != 4 {
		t.Fatalf("i1: %+v", r1)
	}
	if r2 := byHash["i2"]; !r2.RawSame || !r2.DecisionSame || r2.ConfidenceSame || r2.Same {
		t.Fatalf("i2: %+v", r2)
	}
	if r3 := byHash["i3"]; r3.RawSame || r3.DecisionSame || r3.A.Raw != nil || *r3.B.Raw {
		t.Fatalf("i3: %+v", r3)
	}
	if r4 := byHash["i4"]; !r4.DecisionSame || !r4.ConfidenceSame || r4.Same {
		t.Fatalf("i4 identical errors: %+v", r4)
	}
	if r5, r9 := byHash["i5"], byHash["i9"]; r5.B != nil || r9.A != nil || r5.Same || r9.Same {
		t.Fatalf("one-sided: %+v %+v", r5, r9)
	}
	_ = bp
	if rep.CommonPrefixSeq != 1 {
		t.Fatalf("diverge at seq 2: %d", rep.CommonPrefixSeq)
	}
	// nil vs nil is DecisionSame; a type-equal data-equal event with a different At does not extend the prefix
	a2 := mk("mrv-diff-a-0000002", []run.LLMCallData{{Kind: "adjudicate", InputHash: "i3", Verdict: json.RawMessage(`null`)}}, []int{0})
	b2 := mk("mrv-diff-b-0000002", []run.LLMCallData{{Kind: "adjudicate", InputHash: "i3", Verdict: json.RawMessage(`null`)}}, []int{0})
	rep2, _ := DiffRuns(a2, b2, decide)
	if !rep2.Calls[0].DecisionSame || !rep2.Calls[0].RawSame || rep2.CommonPrefixSeq != 2 {
		t.Fatalf("nil/nil: %+v", rep2)
	}
	// incompatible workflows; fold errors
	other := mk("mrv-diff-c-0000001", nil, nil)
	var od run.InitData
	_ = json.Unmarshal(other.Events[0].Data, &od)
	od.Workflow = "different"
	other.Events[0].Data = run.MarshalCanonical(od)
	if _, err := DiffRuns(a, other, decide); !errs.Is(err, CodeDiffIncompatible) {
		t.Fatalf("incompatible: %v", err)
	}
	bad := run.Log{Events: []run.Event{{Type: "bogus"}}}
	if _, err := DiffRuns(bad, a, decide); err == nil {
		t.Fatal("fold error a")
	}
	if _, err := DiffRuns(a, bad, decide); err == nil {
		t.Fatal("fold error b")
	}
	// transitions: same / diverging / one-sided
	tr := func(id string, tos []run.State) run.Log {
		b := run.NewBuilder(id)
		b.Init(run.InitData{RunID: id, Workflow: "w", WorkflowHash: "h", Vars: map[string]string{}, RepoMode: "advisory", AllowedCmds: []run.AllowedCmd{}, RepoRoot: "/r", WorkDir: "/r", BaseSHA: shaBase, Head: shaHead, InitialState: "s", Lineage: []string{}, Goldens: []run.Golden{}})
		from := run.State("s")
		for _, to := range tos {
			b.Event(run.TypeTransition, run.TransitionData{From: from, To: to, Gate: "g", Head: shaHead}, run.WithState(from))
			from = to
		}
		return run.Log{Events: b.Events()}
	}
	ta := tr("mrv-diff-t-0000001", []run.State{"x", "y"})
	tb := tr("mrv-diff-t-0000002", []run.State{"x", "z", "q"})
	rt, err := DiffRuns(ta, tb, decide)
	if err != nil || len(rt.Transitions) != 3 || !rt.Transitions[0].Same || rt.Transitions[1].Same || rt.Transitions[2].Same || rt.Transitions[2].To != "q" || rt.Transitions[2].SeqA != 0 {
		t.Fatalf("transitions: %v %+v", err, rt.Transitions)
	}
}

func TestF10ForkFailureSweeps(t *testing.T) {
	ctx := context.Background()
	snapshotOf := func(h *harness, m *Machine) string {
		var b strings.Builder
		for _, ev := range h.events(m) {
			b.Write(run.MarshalCanonical(ev))
		}
		return b.String()
	}
	// every refusal in steps 0–7 leaves P byte-identical and creates no child
	h := newHarness(t)
	p := sdlcDone(h, InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
	before := snapshotOf(h, p)
	refusals := []ForkOptions{
		{From: "nope"}, {From: "done"}, {From: "adjudicate", AtIter: intp(7)},
		{From: "adjudicate", WorkDir: "rel"}, {From: "verify", Vars: map[string]string{"JUDGE": "b"}},
		{From: "adjudicate", Vars: map[string]string{"NOPE": "b"}}, {From: "adjudicate", WorkflowBytes: []byte("x")},
	}
	for _, o := range refusals {
		if _, _, err := p.Fork(ctx, o); err == nil {
			t.Fatalf("expected refusal for %+v", o)
		}
	}
	if snapshotOf(h, p) != before {
		t.Fatal("parent changed on refusal")
	}
	if list, _ := h.store.List(); len(list) != 1 {
		t.Fatal("child created on refusal")
	}
	// step-8 seam failures: sidecar write, child create, child lock, each append; P stays byte-identical
	h.sidecar.WriteErr = errors.New("sidecar write failed")
	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate"}); err == nil || !strings.Contains(err.Error(), "sidecar write failed") {
		t.Fatalf("sidecar: %v", err)
	}
	h.sidecar.WriteErr = nil
	if snapshotOf(h, p) != before {
		t.Fatal("parent changed on sidecar failure")
	}
	h.store.failOp, h.store.err = "Create", errors.New("create failed")
	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate"}); err == nil || !strings.Contains(err.Error(), "create failed") {
		t.Fatalf("create: %v", err)
	}
	h.store.failOp = ""
	pevs := h.events(p)
	adj, _ := seqOfTransitionInto(t, pevs, "adjudicate", 0)
	h.store.failLockRun = "child"
	h.store.err = errors.New("child lock failed")
	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate"}); err == nil || !strings.Contains(err.Error(), "child lock failed") {
		t.Fatalf("child lock: %v", err)
	}
	h.store.failLockRun = ""
	// an append failure mid-copy leaves an incomplete child and P untouched (no fork event)
	h.store.appends = 0
	h.store.failAt, h.store.err = 2, errors.New("append failed")
	_, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate"})
	if err == nil || !strings.Contains(err.Error(), "append failed") {
		t.Fatalf("append: %v", err)
	}
	h.store.failAt = 0
	if snapshotOf(h, p) != before {
		t.Fatal("parent changed on append failure")
	}
	list, _ := h.store.List()
	var incomplete string
	for _, s := range list {
		if s.RunID != p.runID {
			incomplete = s.RunID
			if !strings.Contains(s.Error, "incomplete fork") {
				t.Fatalf("incomplete child must be flagged: %+v", s)
			}
		}
	}
	if _, err := Open(ctx, h.deps, incomplete, OpenOptions{}); !errs.Is(err, CodeForkIncomplete) {
		t.Fatalf("open incomplete: %v", err)
	}
	// a failure at the parent's fork append: child complete, P without the fork event
	h.store.failType, h.store.err = run.TypeFork, errors.New("fork append failed")
	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate"}); err == nil || !strings.Contains(err.Error(), "fork append failed") {
		t.Fatalf("fork append: %v", err)
	}
	h.store.failType = ""
	if snapshotOf(h, p) != before {
		t.Fatal("parent changed")
	}
	// the child machine's own load failure is returned with the result
	h.store.failEvAt, h.store.err = 3, errors.New("events failed")
	h.store.events = 0
	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate"}); err == nil || !strings.Contains(err.Error(), "events failed") {
		t.Fatalf("child load: %v", err)
	}
	h.store.failEvAt = 0
	// parent lock / events failures
	h.store.failOp = "Lock"
	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate"}); err == nil {
		t.Fatal("parent lock")
	}
	h.store.failOp = ""
	h.store.failEvAt, h.store.events = 2, 0
	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate"}); err == nil {
		t.Fatal("second events read")
	}
	h.store.failEvAt = 0
	// ordinal continuity: copied cmd_calls count toward the child's ordinals
	h4 := newHarness(t)
	wf := sdlcWith(t, h4, "ord.yaml", "  any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]\nrepo_mode: advisory",
		"  any: [no_fixation_progress, {cmd: chk}, {max_iterations: 5}]\ncmds:\n  chk: {argv: [bash, -c, echo]}\nrepo_mode: advisory")
	_, sha, _ := workflow.ResolveCmds(mustResolve(t, h4, wf), "/repo", h4.deps.LookPath, h4.deps.FileHash)
	h4.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
	iter := 0
	h4.reg.execs["still-present"].fn = func(in ExecInput) (json.RawMessage, error) {
		var st []run.BugStatus
		for i, b := range in.Snap.AllFound {
			if err := llmCall(in, i, 5); err != nil {
				return nil, err
			}
			st = append(st, run.BugStatus{ID: b.ID, StillPresent: iter == 0, Confidence: 1})
		}
		return json.RawMessage(run.MarshalCanonical(run.Delta{Status: st})), nil
	}
	pm := h4.mustInit(InitOptions{Workflow: wf, Vars: sdlcVars, AllowCustomCmds: sha})
	h4.advance(pm)
	h4.record(pm, "discover", findings(1))
	h4.advance(pm)
	h4.advance(pm)
	h4.advance(pm)
	h4.record(pm, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
	h4.advance(pm)
	h4.advance(pm) // loop: cmd_call #1
	c, _, err := pm.Fork(ctx, ForkOptions{From: "discover", AtIter: intp(1)})
	if err != nil {
		t.Fatal(err)
	}
	if got := h4.runner.ordinal("chk"); got != 1 {
		t.Fatalf("copied cmd_call must count: ordinal %d", got)
	}
	_ = c
	_ = adj
}

func TestF7DiffSortAndPrefixEdges(t *testing.T) {
	decide := func(kind string, v json.RawMessage) Decision { return Decision{} }
	mk := func(id string, calls []run.LLMCallData, nodes []string) run.Log {
		b := run.NewBuilder(id)
		b.Init(run.InitData{RunID: id, Workflow: "w", WorkflowHash: "h", Vars: map[string]string{}, RepoMode: "advisory", AllowedCmds: []run.AllowedCmd{}, RepoRoot: "/r", WorkDir: "/r", BaseSHA: shaBase, Head: shaHead, InitialState: "s", Lineage: []string{}, Goldens: []run.Golden{}})
		for i, c := range calls {
			b.Event(run.TypeLLMCall, c, run.WithNode(nodes[i]), run.WithState("s"))
		}
		return run.Log{Events: b.Events()}
	}
	// rows differing only in kind, then only in input hash, then node: literal order on out-of-order insertion
	a := mk("mrv-diff-s-0000001", []run.LLMCallData{
		{Kind: "still-present", Index: 0, InputHash: "z"},
		{Kind: "adjudicate", Index: 1, InputHash: "b"},
		{Kind: "adjudicate", Index: 2, InputHash: "a"},
		{Kind: "adjudicate", Index: 0, InputHash: "a"},
	}, []string{"n", "n", "n", "m"})
	rep, err := DiffRuns(a, mk("mrv-diff-s-0000002", nil, nil), decide)
	if err != nil {
		t.Fatal(err)
	}
	got := ""
	for _, r := range rep.Calls {
		got += r.Node + "/" + r.Kind + "/" + r.InputHash + " "
	}
	if got != "m/adjudicate/a n/adjudicate/a n/adjudicate/b n/still-present/z " {
		t.Fatalf("order: %s", got)
	}
	// iteration ordering through a loop transition
	b := run.NewBuilder("mrv-diff-s-0000003")
	b.Init(run.InitData{RunID: "mrv-diff-s-0000003", Workflow: "w", WorkflowHash: "h", Vars: map[string]string{}, RepoMode: "advisory", AllowedCmds: []run.AllowedCmd{}, RepoRoot: "/r", WorkDir: "/r", BaseSHA: shaBase, Head: shaHead, InitialState: "s", Lineage: []string{}, Goldens: []run.Golden{}})
	b.Event(run.TypeTransition, run.TransitionData{From: "s", To: "s", Gate: "g", Head: shaHead, Loop: true}, run.WithState("s"))
	b.Event(run.TypeLLMCall, run.LLMCallData{Kind: "adjudicate", Index: 0, InputHash: "a"}, run.WithNode("n"), run.WithState("s"), run.WithIter(1))
	looped := run.Log{Events: b.Events()}
	rep2, err := DiffRuns(looped, a, decide)
	if err != nil || rep2.Calls[len(rep2.Calls)-1].Iter != 1 || rep2.Calls[len(rep2.Calls)-1].Node != "n" {
		t.Fatalf("iter order: %v %+v", err, rep2.Calls)
	}
}

func TestF10ForkSeamErrors(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	p := sdlcDone(h, InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
	// sidecar read failure (after load verified it once)
	reads := 0
	p.deps.Sidecar = &failingSidecar{Sidecar: h.sidecar, readErr: errors.New("sidecar read failed"), after: &reads, failAt: 2}
	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate"}); err == nil || !strings.Contains(err.Error(), "sidecar read failed") {
		t.Fatalf("sidecar read: %v", err)
	}
	p.deps.Sidecar = h.sidecar
	// repo root's CommonDir fails while the work dir's succeeds
	h.git.byDir["/wt"] = &gate.Fake{HeadSHA: shaHead, Common: "/repo/.git", Clean: true, Tree: "t2"}
	h.git.byDir["/repo"] = &failingGit{Git: h.git.def, at: "CommonDir", err: errors.New("root common failed")}
	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate", WorkDir: "/wt"}); err == nil || !strings.Contains(err.Error(), "root common failed") {
		t.Fatalf("root common: %v", err)
	}
	delete(h.git.byDir, "/repo")
	delete(h.git.byDir, "/wt")
	// ResolveCmds failure on a workflow with commands
	h2 := newHarness(t)
	wf := sdlcWith(t, h2, "cmds.yaml", "  any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]\nrepo_mode: advisory",
		"  any: [no_fixation_progress, {cmd: chk}, {max_iterations: 5}]\ncmds:\n  chk: {argv: [bash, -c, echo]}\nrepo_mode: advisory")
	_, sha, _ := workflow.ResolveCmds(mustResolve(t, h2, wf), "/repo", h2.deps.LookPath, h2.deps.FileHash)
	p2 := sdlcDone(h2, InitOptions{Workflow: wf, Vars: sdlcVars, AllowCustomCmds: sha})
	p2.deps.LookPath = func(string) (string, error) { return "", errors.New("lookpath failed") }
	if _, _, err := p2.Fork(ctx, ForkOptions{From: "adjudicate"}); err == nil || !strings.Contains(err.Error(), "bash not found") {
		t.Fatalf("resolve cmds: %v", err)
	}
	p2.deps.LookPath = h2.deps.LookPath
	// a workflow change that drops a command whose cmd_call was copied → reason cmd (needs a copied cmd_call: loop once)
	raw := h2.files[wf]
	noCmd := strings.Replace(string(raw), "  any: [no_fixation_progress, {cmd: chk}, {max_iterations: 5}]\ncmds:\n  chk: {argv: [bash, -c, echo]}\n", "  any: [no_fixation_progress, {max_iterations: 5}]\n", 1)
	h3 := newHarness(t)
	wf3 := sdlcWith(t, h3, "cmds3.yaml", "  any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]\nrepo_mode: advisory",
		"  any: [no_fixation_progress, {cmd: chk}, {max_iterations: 5}]\ncmds:\n  chk: {argv: [bash, -c, echo]}\nrepo_mode: advisory")
	_, sha3, _ := workflow.ResolveCmds(mustResolve(t, h3, wf3), "/repo", h3.deps.LookPath, h3.deps.FileHash)
	h3.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
	iter := 0
	h3.reg.execs["still-present"].fn = func(in ExecInput) (json.RawMessage, error) {
		var st []run.BugStatus
		for i, b := range in.Snap.AllFound {
			if err := llmCall(in, i, 5); err != nil {
				return nil, err
			}
			st = append(st, run.BugStatus{ID: b.ID, StillPresent: iter == 0, Confidence: 1})
		}
		return json.RawMessage(run.MarshalCanonical(run.Delta{Status: st})), nil
	}
	pm := h3.mustInit(InitOptions{Workflow: wf3, Vars: sdlcVars, AllowCustomCmds: sha3})
	h3.advance(pm)
	h3.record(pm, "discover", findings(1))
	h3.advance(pm)
	h3.advance(pm)
	h3.advance(pm)
	h3.record(pm, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
	h3.advance(pm)
	h3.advance(pm) // loop → cmd_call
	if _, _, err := pm.Fork(ctx, ForkOptions{From: "discover", AtIter: intp(1), AcceptWorkflowChange: true, WorkflowBytes: []byte(noCmd)}); !errs.Is(err, CodeWorkflowIncompatible) || errs.As(err).Fields["reason"] != "cmd" {
		t.Fatalf("removed cmd: %v", err)
	}
	// a renamed state: From no longer exists → reason state
	h4 := newHarness(t)
	p4 := sdlcDone(h4, InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
	base, _ := workflows.Read("sdlc-loop")
	renamed := string(base)
	for _, r := range [][2]string{{"from: adjudicate", "from: review"}, {"to: adjudicate", "to: review"}, {"adjudicate: {kind", "review: {kind"}, {"[discover, adjudicate,", "[discover, review,"}} {
		renamed = strings.ReplaceAll(renamed, r[0], r[1])
	}
	if _, _, err := p4.Fork(ctx, ForkOptions{From: "adjudicate", AcceptWorkflowChange: true, WorkflowBytes: []byte(renamed)}); !errs.Is(err, CodeWorkflowIncompatible) || errs.As(err).Fields["reason"] != "state" || errs.As(err).Fields["state"] != "adjudicate" {
		t.Fatalf("renamed state: %v", err)
	}
	// validateChild: a log that does not fold is ERR_COPY_INVALID with the fold's seq and reason
	if err := validateChild([]run.Event{{Type: "bogus"}}, 100); !errs.Is(err, CodeCopyInvalid) || errs.As(err).Fields["seq"] == "" {
		t.Fatalf("validateChild fold: %v", err)
	}
	// copyInvalid maps fold errors and plain errors
	if e := errs.As(copyInvalid(&run.FoldError{Code: run.CodeAuditInvalid, Reason: "x", Seq: 4})); e.Code != CodeCopyInvalid || e.Fields["seq"] != "4" || e.Fields["reason"] != "x" {
		t.Fatalf("copyInvalid fold: %+v", e)
	}
	if e := errs.As(copyInvalid(errors.New("plain"))); e.Code != CodeCopyInvalid || e.Fields["reason"] != "fold" {
		t.Fatalf("copyInvalid plain: %+v", e)
	}
}

type failingSidecar struct {
	Sidecar
	readErr error
	after   *int
	failAt  int
}

func (f *failingSidecar) Read(runID, name string) ([]byte, error) {
	*f.after++
	if *f.after == f.failAt {
		return nil, f.readErr
	}
	return f.Sidecar.Read(runID, name)
}

// Two calls of one kind in the same node@iter with the same input — the same
// question put to two models — must both appear in the report. Keying rows on
// (node, iter, kind, input_hash) alone overwrote row.A and the earlier call
// vanished, which is the one thing a diff of two runs must never do.
func TestDiffRunsKeepsRepeatedCallsWithTheSameInput(t *testing.T) {
	mk := func(id string, calls []run.LLMCallData) run.Log {
		b := run.NewBuilder(id)
		b.Init(run.InitData{RunID: id, Workflow: "w", WorkflowHash: "h", Vars: map[string]string{},
			RepoMode: "advisory", AllowedCmds: []run.AllowedCmd{}, RepoRoot: "/r", WorkDir: "/r",
			BaseSHA: shaBase, Head: shaHead, InitialState: "s", Lineage: []string{}, Goldens: []run.Golden{}})
		for _, c := range calls {
			b.Event(run.TypeLLMCall, c, run.WithNode("n"))
		}
		return run.Log{Events: b.Events()}
	}
	raw := func(real bool, conf float64) json.RawMessage {
		return json.RawMessage(run.MarshalCanonical(map[string]any{"is_real": real, "confidence": conf}))
	}
	same := "same-input-hash"
	a := mk("mrv-diff-rep-a-001", []run.LLMCallData{
		{Kind: "adjudicate", Model: "m1", Index: 0, InputHash: same, Verdict: raw(true, 0.9), Confidence: 0.9},
		{Kind: "adjudicate", Model: "m2", Index: 1, InputHash: same, Verdict: raw(false, 0.4), Confidence: 0.4},
	})
	b := mk("mrv-diff-rep-b-001", []run.LLMCallData{
		{Kind: "adjudicate", Model: "m3", Index: 0, InputHash: same, Verdict: raw(true, 0.8), Confidence: 0.8},
		{Kind: "adjudicate", Model: "m4", Index: 1, InputHash: same, Verdict: raw(false, 0.3), Confidence: 0.3},
	})

	decide := func(kind string, v json.RawMessage) Decision {
		var d struct {
			IsReal     *bool   `json:"is_real"`
			Confidence float64 `json:"confidence"`
		}
		_ = json.Unmarshal(v, &d)
		if d.IsReal == nil {
			return Decision{}
		}
		eff := *d.IsReal && d.Confidence >= 0.7
		return Decision{Raw: d.IsReal, Effective: &eff}
	}
	rep, err := DiffRuns(a, b, decide)
	if err != nil {
		t.Fatal(err)
	}
	rows := 0
	for _, row := range rep.Calls {
		if row.InputHash == same {
			rows++
			if row.A == nil || row.B == nil {
				t.Fatalf("both sides must be present on every repeated row: %+v", row)
			}
		}
	}
	if rows != 2 {
		t.Fatalf("both calls must survive: got %d rows for the repeated input, want 2", rows)
	}
	// And the nth call on each side pairs with the nth on the other.
	if rep.Calls[0].A.Model != "m1" || rep.Calls[0].B.Model != "m3" {
		t.Fatalf("first row must pair the first call of each side: %+v / %+v", rep.Calls[0].A, rep.Calls[0].B)
	}
}

// A non-terminal state may legally carry no node: validateGraph forbids a node only on
// terminal states (workflow.go "terminal_with_node"). Events are stamped with the state
// they occurred in, so such a state reaches compatible()'s copied-prefix loop. All four
// nil combinations must be decided, and none may panic.
func TestForkCompatibleNodelessState(t *testing.T) {
	const tmpl = `workflow: nodeless
version: 1
states: [a, b, done, failed]
transitions:
  - {from: a, to: b, gate: findings_nonempty}
  - {from: b, to: done, gate: all_fixed, outcome: fixed}
nodes:
NODES
convergence:
  any: [{max_iterations: 5}]
repo_mode: advisory
`
	h := newHarness(t)
	parse := func(nodes string) *workflow.Workflow {
		t.Helper()
		w, err := workflow.Parse([]byte(strings.Replace(tmpl, "NODES", nodes, 1)), workflow.Options{Kinds: h.reg.Info()})
		if err != nil {
			t.Fatalf("parse %q: %v", nodes, err)
		}
		return w
	}
	const (
		bare  = "  a: {kind: agent-edit}"
		withB = "  a: {kind: agent-edit}\n  b: {kind: agent-edit}"
	)
	// an event stamped with the nodeless state, so "b" enters the copied-prefix set
	copied := []run.Event{{Type: run.TypeNodeOutput, State: "b"}}

	if err := compatible(parse(bare), parse(bare), copied, "a"); err != nil {
		t.Fatalf("nodeless in both is an unchanged state, want nil, got %v", err)
	}
	if err := compatible(parse(bare), parse(withB), copied, "a"); !errs.Is(err, CodeWorkflowIncompatible) || errs.As(err).Fields["reason"] != "state" {
		t.Fatalf("a node added on a copied nodeless state must be incompatible, got %v", err)
	}
	if err := compatible(parse(withB), parse(bare), copied, "a"); !errs.Is(err, CodeWorkflowIncompatible) || errs.As(err).Fields["reason"] != "state" {
		t.Fatalf("a node removed from a copied state must be incompatible, got %v", err)
	}
}

// A verdict reached by browsing a materialized tree and one reached from excerpts in the
// prompt are answers to different questions. DiffRuns must say so rather than report them as
// agreeing or disagreeing, which would read as a model difference.
func TestReportCompareFlagsCrossEvidenceRows(t *testing.T) {
	yes, no := true, false
	side := func(evidence string, real *bool) *CallSide {
		return &CallSide{Model: "m", Raw: real, Effective: real, Confidence: 0.9, Evidence: evidence}
	}
	t.Run("same evidence compares normally", func(t *testing.T) {
		r := &Report{}
		row := &CallRow{A: side(run.EvidenceExcerpt, &yes), B: side(run.EvidenceExcerpt, &yes)}
		r.compare(row)
		if !row.EvidenceSame || !row.Same || r.EvidenceMismatch != 0 {
			t.Fatalf("row=%+v mismatch=%d", row, r.EvidenceMismatch)
		}
	})
	t.Run("empty evidence means excerpt", func(t *testing.T) {
		r := &Report{}
		row := &CallRow{A: side("", &yes), B: side(run.EvidenceExcerpt, &yes)}
		r.compare(row)
		if !row.EvidenceSame || r.EvidenceMismatch != 0 {
			t.Fatal("a pre-field row must still compare against an explicit excerpt row")
		}
	})
	t.Run("different evidence is never Same, even on equal verdicts", func(t *testing.T) {
		r := &Report{}
		row := &CallRow{A: side(run.EvidenceExcerpt, &yes), B: side(run.EvidenceSandbox, &yes)}
		r.compare(row)
		if row.EvidenceSame {
			t.Error("EvidenceSame must be false")
		}
		if row.Same {
			t.Error("equal verdicts from unequal evidence are not agreement")
		}
		if !row.DecisionSame {
			t.Error("the underlying decision comparison must still be reported")
		}
		if r.EvidenceMismatch != 1 {
			t.Errorf("EvidenceMismatch = %d, want 1", r.EvidenceMismatch)
		}
	})
	t.Run("disagreeing verdicts on the same evidence", func(t *testing.T) {
		r := &Report{}
		row := &CallRow{A: side(run.EvidenceSandbox, &yes), B: side(run.EvidenceSandbox, &no)}
		r.compare(row)
		if row.Same || !row.EvidenceSame || r.EvidenceMismatch != 0 {
			t.Fatalf("row=%+v mismatch=%d", row, r.EvidenceMismatch)
		}
	})
	t.Run("a one-sided row is left alone", func(t *testing.T) {
		r := &Report{}
		row := &CallRow{A: side(run.EvidenceSandbox, &yes)}
		r.compare(row)
		if row.Same || row.EvidenceSame || r.EvidenceMismatch != 0 {
			t.Fatalf("row=%+v mismatch=%d", row, r.EvidenceMismatch)
		}
	})
}

// CallRow.Index shipped as a constant 0 in every fsm diff envelope while its doc comment
// claimed it distinguished repeated calls. The unexported occurrence counter does that.
func TestCallRowCarriesNoDeadIndexField(t *testing.T) {
	b, err := json.Marshal(CallRow{Node: "n", Kind: "adjudicate", A: &CallSide{Index: 3}})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if _, present := m["index"]; present {
		t.Errorf("CallRow still ships a row-level index: %s", b)
	}
	// Marshalling one instance cannot see the defect's actual shape: re-adding
	// `Index int `+"`"+`json:"index,omitempty"`+"`"+` to CallRow left the full suite green, because the row above
	// has a zero Index and omitempty drops it. The type is what the property is about.
	for i := 0; i < reflect.TypeOf(CallRow{}).NumField(); i++ {
		if f := reflect.TypeOf(CallRow{}).Field(i); f.Name == "Index" {
			t.Errorf("CallRow declares a row-level Index field (json tag %q); the index belongs to each side", f.Tag.Get("json"))
		}
	}
	side := m["a"].(map[string]any)
	if side["index"] != float64(3) {
		t.Errorf("the per-side index must survive: %v", side["index"])
	}
}
