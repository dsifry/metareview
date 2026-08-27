package machine

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/run"
	"github.com/dsifry/metareview/internal/fsm/workflow"
	"github.com/dsifry/metareview/workflows"
)

func TestReadOnlyOpen(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	m := h.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
	h.advance(m)
	// a held lock, a re-pinned binary, and a missing scenario do not stop a read-only open
	unlock, err := h.store.Lock(m.runID)
	if err != nil {
		t.Fatal(err)
	}
	h.deps.FileHash = func(string) (string, error) { return "", errors.New("binary changed") }
	loads := 0
	h.deps.MockLoad = func(string) (string, error) { loads++; return "", errors.New("gone") }
	ro, err := Open(ctx, h.deps, m.runID, OpenOptions{ReadOnly: true})
	unlock()
	if err != nil || loads != 0 {
		t.Fatalf("read-only open: %v (mock loads %d)", err, loads)
	}
	v := ro.View()
	if v.Node == nil || v.Node.Model != "claude-opus-5" || v.Node.Effort != "low" || v.NextAction != NextRecord {
		t.Fatalf("node view: %+v", v.Node)
	}
	if len(v.Outgoing) != 2 || v.Outgoing[0].To != "done" || v.Outgoing[0].Gate != "nothing_found" || v.Outgoing[1].To != "adjudicate" {
		t.Fatalf("outgoing: %+v", v.Outgoing)
	}
	// a full open takes the lock: it fails while another session holds it
	unlock2, _ := h.store.Lock(m.runID)
	if _, err := Open(ctx, h.deps, m.runID, OpenOptions{}); err == nil {
		t.Fatal("full open must take the lock")
	}
	unlock2()
	// terminal state has no outgoing edges
	h2 := newHarness(t)
	done := sdlcDone(h2, InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
	if v := done.View(); len(v.Outgoing) != 0 || v.Node != nil {
		t.Fatalf("terminal view: %+v", v)
	}
}

func TestPreflightSeam(t *testing.T) {
	h := newHarness(t)
	var calls []string
	var preErr error
	h.deps.Preflight = func(n *workflow.Node, cal bool) error {
		calls = append(calls, n.Name+"/"+n.Kind+"/"+n.Model+"/"+n.Effort+"/"+map[bool]string{true: "cal", false: "prod"}[cal])
		return preErr
	}
	// init: every fork node, after Resolve, before Create; an error creates nothing
	preErr = errs.E("ERR_JUDGE_KEY", "no key")
	if _, err := h.init(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"}); !errs.Is(err, "ERR_JUDGE_KEY") {
		t.Fatalf("preflight at init: %v", err)
	}
	if list, _ := h.store.List(); len(list) != 0 {
		t.Fatal("nothing created when pre-flight refuses")
	}
	preErr = nil
	calls = nil
	m := h.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
	if strings.Join(calls, " ") != "adjudicate/match-then-adjudicate/gpt-5.2/medium/prod verify/still-present/gpt-5.2/medium/prod" {
		t.Fatalf("init preflight calls: %v", calls)
	}
	// advance: not for host nodes; once for the fork node before it runs; not when its output exists
	calls = nil
	h.advance(m) // discover needs input
	h.record(m, "discover", findings(1))
	h.advance(m) // → adjudicate
	if len(calls) != 0 {
		t.Fatalf("host nodes never pre-flight: %v", calls)
	}
	preErr = errs.E("ERR_JUDGE_MODEL", "bogus")
	appends := h.store.appends
	if _, err := m.Advance(context.Background()); !errs.Is(err, "ERR_JUDGE_MODEL") {
		t.Fatalf("advance preflight: %v", err)
	}
	if h.store.appends != appends {
		t.Fatal("no append when pre-flight refuses")
	}
	preErr = nil
	calls = nil
	if r := h.advance(m); r.To != "fix" || strings.Join(calls, " ") != "adjudicate/match-then-adjudicate/gpt-5.2/medium/prod" {
		t.Fatalf("fork node preflight once: %+v %v", r, calls)
	}
	// calibration flag reaches the seam
	h3 := newHarness(t)
	var cal []bool
	h3.deps.Preflight = func(_ *workflow.Node, c bool) error { cal = append(cal, c); return nil }
	h3.mustInit(InitOptions{Workflow: "sdlc-loop", Base: "main", Calibration: true})
	if len(cal) != 2 || !cal[0] {
		t.Fatalf("calibration flag: %v", cal)
	}
}

func TestInitWorkflowSourceAndReservedName(t *testing.T) {
	h := newHarness(t)
	m := h.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
	if m.View().Snapshot.WorkflowSource != "embedded" {
		t.Fatalf("embedded source: %+v", m.View().Snapshot.WorkflowSource)
	}
	raw, _ := workflows.Read("sdlc-loop")
	// a path with the embedded bytes is accepted as a path workflow
	h.files["/x/same.yaml"] = raw
	m2 := h.mustInit(InitOptions{Workflow: "/x/same.yaml", Vars: sdlcVars, Base: "main"})
	if m2.View().Snapshot.WorkflowSource != "path" {
		t.Fatal("path source")
	}
	// different bytes under an embedded name are refused before anything is created
	h.files["/x/fake.yaml"] = []byte(strings.Replace(string(raw), "effort: $REV_EFFORT", "effort: high", 1))
	n := len(mustList(t, h))
	if _, err := h.init(InitOptions{Workflow: "/x/fake.yaml", Vars: sdlcVars, Base: "main"}); !errs.Is(err, workflow.CodeWorkflowInvalid) || errs.As(err).Fields["reason"] != "reserved_name" {
		t.Fatalf("reserved name: %v", err)
	}
	if len(mustList(t, h)) != n {
		t.Fatal("refusal created a run")
	}
	// a path workflow with its own name is fine
	h.files["/x/own.yaml"] = renamed(strings.Replace(string(raw), "effort: $REV_EFFORT", "effort: high", 1))
	if _, err := h.init(InitOptions{Workflow: "/x/own.yaml", Vars: sdlcVars, Base: "main"}); err != nil {
		t.Fatal(err)
	}
	// consent refusals carry the structured list
	wf := sdlcWith(t, h, "cj.yaml", "repo_mode: advisory", "cmds:\n  notify: {argv: [bash, ./n.sh]}\non_overflow: notify\nrepo_mode: advisory")
	_, err := h.init(InitOptions{Workflow: wf, Vars: sdlcVars, Base: "main"})
	if !errs.Is(err, CodeCmdsNotAllowed) {
		t.Fatalf("consent: %v", err)
	}
	var cmds []run.AllowedCmd
	if json.Unmarshal([]byte(errs.As(err).Fields["cmds_json"]), &cmds) != nil || len(cmds) != 1 || cmds[0].Name != "notify" {
		t.Fatalf("cmds_json: %v", errs.As(err).Fields["cmds_json"])
	}
}

func mustList(t *testing.T, h *harness) []run.RunSummary {
	t.Helper()
	l, err := h.store.List()
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestRecordLLMCall(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	m := h.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
	h.advance(m)
	var seen []Stamp
	call := func(kind string, err error) func(context.Context, Stamp) (run.LLMCallData, error) {
		return func(_ context.Context, st Stamp) (run.LLMCallData, error) {
			seen = append(seen, st)
			return run.LLMCallData{Kind: kind, Model: "m", Effort: "low", InputHash: "ih", Verdict: json.RawMessage(`{"is_real":true}`), Confidence: 0.9}, err
		}
	}
	seq, err := m.RecordLLMCall(ctx, call("adjudicate", nil))
	if err != nil || seq == 0 {
		t.Fatalf("first: %v %d", err, seq)
	}
	seq2, err := m.RecordLLMCall(ctx, call("adjudicate", nil))
	if err != nil || seq2 != seq+1 {
		t.Fatalf("second: %v %d", err, seq2)
	}
	if len(seen) != 2 || seen[0].Index != 0 || seen[1].Index != 1 || seen[0].State != "discover" || seen[0].Iter != 0 || seen[0].Calibration || !seen[0].Fence {
		t.Fatalf("stamps: %+v", seen)
	}
	evs := h.events(m)
	last := evs[len(evs)-1]
	var d run.LLMCallData
	_ = json.Unmarshal(last.Data, &d)
	if last.Type != run.TypeLLMCall || last.Node != JudgeNode || last.State != "discover" || d.Index != 1 {
		t.Fatalf("appended llm_call: %+v %+v", last, d)
	}
	// a call error with the data set is appended and returned; Kind == "" appends nothing
	n := len(h.events(m))
	if _, err := m.RecordLLMCall(ctx, call("adjudicate", errors.New("http 500"))); err == nil || err.Error() != "http 500" || len(h.events(m)) != n+1 {
		t.Fatalf("error appended: %v", err)
	}
	if _, err := m.RecordLLMCall(ctx, func(context.Context, Stamp) (run.LLMCallData, error) { return run.LLMCallData{}, context.Canceled }); !errors.Is(err, context.Canceled) || len(h.events(m)) != n+1 {
		t.Fatalf("nothing appended on cancel: %v", err)
	}
	// the machine's own work continues: the next index for judge@0 is 3
	if r := h.record(m, "discover", findings(1)); r.Seq == 0 {
		t.Fatal("record")
	}
	// calibration run: Fence false
	hc := newHarness(t)
	mc := hc.mustInit(InitOptions{Workflow: "sdlc-loop", Base: "main", Calibration: true})
	var st Stamp
	_, _ = mc.RecordLLMCall(ctx, func(_ context.Context, s Stamp) (run.LLMCallData, error) { st = s; return run.LLMCallData{}, nil })
	if !st.Calibration || st.Fence {
		t.Fatalf("calibration stamp: %+v", st)
	}
	// terminal run refused; torn refused; lock/append failures pass through
	done := sdlcDone(newHarness(t), InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
	if _, err := done.RecordLLMCall(ctx, call("adjudicate", nil)); !errs.Is(err, CodeRunTerminal) {
		t.Fatalf("terminal: %v", err)
	}
	h.store.torn = true
	if _, err := m.RecordLLMCall(ctx, call("adjudicate", nil)); !errs.Is(err, run.CodeAuditTorn) {
		t.Fatalf("torn: %v", err)
	}
	h.store.torn = false
	h.store.failOp, h.store.err = "Lock", errors.New("locked")
	if _, err := m.RecordLLMCall(ctx, call("adjudicate", nil)); err == nil {
		t.Fatal("lock")
	}
	h.store.failOp = ""
	h.store.failType = run.TypeLLMCall
	if _, err := m.RecordLLMCall(ctx, call("adjudicate", nil)); err == nil || err.Error() != "locked" {
		t.Fatalf("append: %v", err)
	}
	h.store.failType = ""
}
