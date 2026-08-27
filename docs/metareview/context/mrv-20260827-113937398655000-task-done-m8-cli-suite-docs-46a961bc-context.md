# metareview task-done context

Run ID: `mrv-20260827-113937398655000-task-done-m8-cli-suite-docs-46a961bc`

## Task

# M8 — CLI, judge/mockai/converge handoffs, black-box suite, docs

Implements spec 4 r5 (`docs/specs/2026-08-27-metareview-0.9.0-fsm-cli.md`) plus the spec 2/5 handoffs:
`judge.Preflight`, `mockai.MaxFileBytes`, `converge.Describe`; `machine` `OpenOptions`, `Deps.Preflight`, `NodeView`,
`View.Outgoing`, `RecordLLMCall`, `Init` workflow-source stamps; `internal/fsm/cli` (`Deps` seams, `RealDeps`, `Run`,
envelopes, `exitFor`, `StatusLines`, `AgentPrompt`) wired into `cmd/metareview` (`fsm` branch, status section);
`tests/go/test-fsm.sh` over the mock scenarios under `testdata/fsm/scenarios`; `/fsm` skill, `commands/fsm.md`,
`docs/fsm/`, README/INSTALL/quickstart/AGENTS/CLAUDE/CHANGELOG/manifest amendments.

Done when every `internal/fsm/*` package and `workflows/` is at exactly 100% statement coverage and the legacy
packages hold their recorded floor (`tests/coverage.sh`), `tests/run-all.sh` is green, and `go vet` is clean.


## Git

- Base: `cc5c34cbb11e1260616614aef28e17238d4201ad`
- Head: `d0ea1d9cdb8a5981602f3d453857c36a1873a349`
- Branch: ``
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `26789`
- Filtered diff bytes: `26789`
- Risk level: `none`



## Review Manifest

- Manifest verdict: `NEEDS_REVISION`
- Source manifest hash: `95dd14023a874e36`
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- docs/tasks/m8-cli-suite-docs.md
- internal/fsm/machine/handoffs_test.go
- internal/fsm/machine/machine.go
- internal/fsm/machine/machine_test.go
- internal/fsm/machine/types.go

### Shards
- shard-01: docs/tasks/m8-cli-suite-docs.md, internal/fsm/machine/handoffs_test.go, internal/fsm/machine/machine.go, internal/fsm/machine/machine_test.go, internal/fsm/machine/types.go

### Manifest Blockers
- missing shard result for shard-01

## Changed Files

- internal/fsm/machine/handoffs_test.go
- internal/fsm/machine/machine.go
- internal/fsm/machine/machine_test.go
- internal/fsm/machine/types.go
- docs/tasks/m8-cli-suite-docs.md

## Diff

```diff
diff --git a/internal/fsm/machine/handoffs_test.go b/internal/fsm/machine/handoffs_test.go
new file mode 100644
index 0000000..8baf9b7
--- /dev/null
+++ b/internal/fsm/machine/handoffs_test.go
@@ -0,0 +1,226 @@
+package machine
+
+import (
+	"context"
+	"encoding/json"
+	"errors"
+	"strings"
+	"testing"
+
+	"github.com/dsifry/metareview/internal/fsm/errs"
+	"github.com/dsifry/metareview/internal/fsm/run"
+	"github.com/dsifry/metareview/internal/fsm/workflow"
+	"github.com/dsifry/metareview/workflows"
+)
+
+func TestReadOnlyOpen(t *testing.T) {
+	h := newHarness(t)
+	ctx := context.Background()
+	m := h.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
+	h.advance(m)
+	// a held lock, a re-pinned binary, and a missing scenario do not stop a read-only open
+	unlock, err := h.store.Lock(m.runID)
+	if err != nil {
+		t.Fatal(err)
+	}
+	h.deps.FileHash = func(string) (string, error) { return "", errors.New("binary changed") }
+	loads := 0
+	h.deps.MockLoad = func(string) (string, error) { loads++; return "", errors.New("gone") }
+	ro, err := Open(ctx, h.deps, m.runID, OpenOptions{ReadOnly: true})
+	unlock()
+	if err != nil || loads != 0 {
+		t.Fatalf("read-only open: %v (mock loads %d)", err, loads)
+	}
+	v := ro.View()
+	if v.Node == nil || v.Node.Model != "claude-opus-5" || v.Node.Effort != "low" || v.NextAction != NextRecord {
+		t.Fatalf("node view: %+v", v.Node)
+	}
+	if len(v.Outgoing) != 2 || v.Outgoing[0].To != "done" || v.Outgoing[0].Gate != "nothing_found" || v.Outgoing[1].To != "adjudicate" {
+		t.Fatalf("outgoing: %+v", v.Outgoing)
+	}
+	// a full open takes the lock: it fails while another session holds it
+	unlock2, _ := h.store.Lock(m.runID)
+	if _, err := Open(ctx, h.deps, m.runID, OpenOptions{}); err == nil {
+		t.Fatal("full open must take the lock")
+	}
+	unlock2()
+	// terminal state has no outgoing edges
+	h2 := newHarness(t)
+	done := sdlcDone(h2, InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
+	if v := done.View(); len(v.Outgoing) != 0 || v.Node != nil {
+		t.Fatalf("terminal view: %+v", v)
+	}
+}
+
+func TestPreflightSeam(t *testing.T) {
+	h := newHarness(t)
+	var calls []string
+	var preErr error
+	h.deps.Preflight = func(n *workflow.Node, cal bool) error {
+		calls = append(calls, n.Name+"/"+n.Kind+"/"+n.Model+"/"+n.Effort+"/"+map[bool]string{true: "cal", false: "prod"}[cal])
+		return preErr
+	}
+	// init: every fork node, after Resolve, before Create; an error creates nothing
+	preErr = errs.E("ERR_JUDGE_KEY", "no key")
+	if _, err := h.init(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"}); !errs.Is(err, "ERR_JUDGE_KEY") {
+		t.Fatalf("preflight at init: %v", err)
+	}
+	if list, _ := h.store.List(); len(list) != 0 {
+		t.Fatal("nothing created when pre-flight refuses")
+	}
+	preErr = nil
+	calls = nil
+	m := h.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
+	if strings.Join(calls, " ") != "adjudicate/match-then-adjudicate/gpt-5.2/medium/prod verify/still-present/gpt-5.2/medium/prod" {
+		t.Fatalf("init preflight calls: %v", calls)
+	}
+	// advance: not for host nodes; once for the fork node before it runs; not when its output exists
+	calls = nil
+	h.advance(m) // discover needs input
+	h.record(m, "discover", findings(1))
+	h.advance(m) // → adjudicate
+	if len(calls) != 0 {
+		t.Fatalf("host nodes never pre-flight: %v", calls)
+	}
+	preErr = errs.E("ERR_JUDGE_MODEL", "bogus")
+	appends := h.store.appends
+	if _, err := m.Advance(context.Background()); !errs.Is(err, "ERR_JUDGE_MODEL") {
+		t.Fatalf("advance preflight: %v", err)
+	}
+	if h.store.appends != appends {
+		t.Fatal("no append when pre-flight refuses")
+	}
+	preErr = nil
+	calls = nil
+	if r := h.advance(m); r.To != "fix" || strings.Join(calls, " ") != "adjudicate/match-then-adjudicate/gpt-5.2/medium/prod" {
+		t.Fatalf("fork node preflight once: %+v %v", r, calls)
+	}
+	// calibration flag reaches the seam
+	h3 := newHarness(t)
+	var cal []bool
+	h3.deps.Preflight = func(_ *workflow.Node, c bool) error { cal = append(cal, c); return nil }
+	h3.mustInit(InitOptions{Workflow: "sdlc-loop", Base: "main", Calibration: true})
+	if len(cal) != 2 || !cal[0] {
+		t.Fatalf("calibration flag: %v", cal)
+	}
+}
+
+func TestInitWorkflowSourceAndReservedName(t *testing.T) {
+	h := newHarness(t)
+	m := h.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
+	if m.View().Snapshot.WorkflowSource != "embedded" {
+		t.Fatalf("embedded source: %+v", m.View().Snapshot.WorkflowSource)
+	}
+	raw, _ := workflows.Read("sdlc-loop")
+	// a path with the embedded bytes is accepted as a path workflow
+	h.files["/x/same.yaml"] = raw
+	m2 := h.mustInit(InitOptions{Workflow: "/x/same.yaml", Vars: sdlcVars, Base: "main"})
+	if m2.View().Snapshot.WorkflowSource != "path" {
+		t.Fatal("path source")
+	}
+	// different bytes under an embedded name are refused before anything is created
+	h.files["/x/fake.yaml"] = []byte(strings.Replace(string(raw), "effort: $REV_EFFORT", "effort: high", 1))
+	n := len(mustList(t, h))
+	if _, err := h.init(InitOptions{Workflow: "/x/fake.yaml", Vars: sdlcVars, Base: "main"}); !errs.Is(err, workflow.CodeWorkflowInvalid) || errs.As(err).Fields["reason"] != "reserved_name" {
+		t.Fatalf("reserved name: %v", err)
+	}
+	if len(mustList(t, h)) != n {
+		t.Fatal("refusal created a run")
+	}
+	// a path workflow with its own name is fine
+	h.files["/x/own.yaml"] = renamed(strings.Replace(string(raw), "effort: $REV_EFFORT", "effort: high", 1))
+	if _, err := h.init(InitOptions{Workflow: "/x/own.yaml", Vars: sdlcVars, Base: "main"}); err != nil {
+		t.Fatal(err)
+	}
+	// consent refusals carry the structured list
+	wf := sdlcWith(t, h, "cj.yaml", "repo_mode: advisory", "cmds:\n  notify: {argv: [bash, ./n.sh]}\non_overflow: notify\nrepo_mode: advisory")
+	_, err := h.init(InitOptions{Workflow: wf, Vars: sdlcVars, Base: "main"})
+	if !errs.Is(err, CodeCmdsNotAllowed) {
+		t.Fatalf("consent: %v", err)
+	}
+	var cmds []run.AllowedCmd
+	if json.Unmarshal([]byte(errs.As(err).Fields["cmds_json"]), &cmds) != nil || len(cmds) != 1 || cmds[0].Name != "notify" {
+		t.Fatalf("cmds_json: %v", errs.As(err).Fields["cmds_json"])
+	}
+}
+
+func mustList(t *testing.T, h *harness) []run.RunSummary {
+	t.Helper()
+	l, err := h.store.List()
+	if err != nil {
+		t.Fatal(err)
+	}
+	return l
+}
+
+func TestRecordLLMCall(t *testing.T) {
+	h := newHarness(t)
+	ctx := context.Background()
+	m := h.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
+	h.advance(m)
+	var seen []Stamp
+	call := func(kind string, err error) func(context.Context, Stamp) (run.LLMCallData, error) {
+		return func(_ context.Context, st Stamp) (run.LLMCallData, error) {
+			seen = append(seen, st)
+			return run.LLMCallData{Kind: kind, Model: "m", Effort: "low", InputHash: "ih", Verdict: json.RawMessage(`{"is_real":true}`), Confidence: 0.9}, err
+		}
+	}
+	seq, err := m.RecordLLMCall(ctx, call("adjudicate", nil))
+	if err != nil || seq == 0 {
+		t.Fatalf("first: %v %d", err, seq)
+	}
+	seq2, err := m.RecordLLMCall(ctx, call("adjudicate", nil))
+	if err != nil || seq2 != seq+1 {
+		t.Fatalf("second: %v %d", err, seq2)
+	}
+	if len(seen) != 2 || seen[0].Index != 0 || seen[1].Index != 1 || seen[0].State != "discover" || seen[0].Iter != 0 || seen[0].Calibration || !seen[0].Fence {
+		t.Fatalf("stamps: %+v", seen)
+	}
+	evs := h.events(m)
+	last := evs[len(evs)-1]
+	var d run.LLMCallData
+	_ = json.Unmarshal(last.Data, &d)
+	if last.Type != run.TypeLLMCall || last.Node != JudgeNode || last.State != "discover" || d.Index != 1 {
+		t.Fatalf("appended llm_call: %+v %+v", last, d)
+	}
+	// a call error with the data set is appended and returned; Kind == "" appends nothing
+	n := len(h.events(m))
+	if _, err := m.RecordLLMCall(ctx, call("adjudicate", errors.New("http 500"))); err == nil || err.Error() != "http 500" || len(h.events(m)) != n+1 {
+		t.Fatalf("error appended: %v", err)
+	}
+	if _, err := m.RecordLLMCall(ctx, func(context.Context, Stamp) (run.LLMCallData, error) { return run.LLMCallData{}, context.Canceled }); !errors.Is(err, context.Canceled) || len(h.events(m)) != n+1 {
+		t.Fatalf("nothing appended on cancel: %v", err)
+	}
+	// the machine's own work continues: the next index for judge@0 is 3
+	if r := h.record(m, "discover", findings(1)); r.Seq == 0 {
+		t.Fatal("record")
+	}
+	// calibration run: Fence false
+	hc := newHarness(t)
+	mc := hc.mustInit(InitOptions{Workflow: "sdlc-loop", Base: "main", Calibration: true})
+	var st Stamp
+	_, _ = mc.RecordLLMCall(ctx, func(_ context.Context, s Stamp) (run.LLMCallData, error) { st = s; return run.LLMCallData{}, nil })
+	if !st.Calibration || st.Fence {
+		t.Fatalf("calibration stamp: %+v", st)
+	}
+	// terminal run refused; torn refused; lock/append failures pass through
+	done := sdlcDone(newHarness(t), InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
+	if _, err := done.RecordLLMCall(ctx, call("adjudicate", nil)); !errs.Is(err, CodeRunTerminal) {
+		t.Fatalf("terminal: %v", err)
+	}
+	h.store.torn = true
+	if _, err := m.RecordLLMCall(ctx, call("adjudicate", nil)); !errs.Is(err, run.CodeAuditTorn) {
+		t.Fatalf("torn: %v", err)
+	}
+	h.store.torn = false
+	h.store.failOp, h.store.err = "Lock", errors.New("locked")
+	if _, err := m.RecordLLMCall(ctx, call("adjudicate", nil)); err == nil {
+		t.Fatal("lock")
+	}
+	h.store.failOp = ""
+	h.store.failType = run.TypeLLMCall
+	if _, err := m.RecordLLMCall(ctx, call("adjudicate", nil)); err == nil || err.Error() != "locked" {
+		t.Fatalf("append: %v", err)
+	}
+	h.store.failType = ""
+}
diff --git a/internal/fsm/machine/machine.go b/internal/fsm/machine/machine.go
index 5e9359c..c7c09a5 100644
--- a/internal/fsm/machine/machine.go
+++ b/internal/fsm/machine/machine.go
@@ -60,7 +60,9 @@ func Init(ctx context.Context, deps Deps, o InitOptions) (*Machine, error) {
 	// 1. load + parse + resolve
 	var raw []byte
 	var err error
+	source := "embedded"
 	if strings.Contains(o.Workflow, "/") || strings.HasSuffix(o.Workflow, ".yaml") {
+		source = "path"
 		raw, err = deps.ReadFile(o.Workflow)
 		if err != nil {
 			return nil, errs.E(CodeWorkflowNotFound, err.Error(), "workflow", o.Workflow)
@@ -78,6 +80,11 @@ func Init(ctx context.Context, deps Deps, o InitOptions) (*Machine, error) {
 	if err != nil {
 		return nil, err
 	}
+	if source == "path" {
+		if emb, err := deps.Workflows(parsed.Name); err == nil && !bytes.Equal(emb, raw) {
+			return nil, errs.E(workflow.CodeWorkflowInvalid, "a path workflow may not reuse the embedded name "+parsed.Name+" with different bytes", "reason", "reserved_name", "workflow", o.Workflow)
+		}
+	}
 	switch o.RepoMode {
 	case "":
 	case "enforcing":
@@ -89,6 +96,15 @@ func Init(ctx context.Context, deps Deps, o InitOptions) (*Machine, error) {
 	if err != nil {
 		return nil, err
 	}
+	if deps.Preflight != nil {
+		for _, st := range w.States {
+			if n := w.NodeFor(st); n != nil && n.Exec == "fork" {
+				if err := deps.Preflight(n, o.Calibration); err != nil {
+					return nil, err
+				}
+			}
+		}
+	}
 	if !filepath.IsAbs(o.WorkDir) || !filepath.IsAbs(o.RepoRoot) {
 		return nil, errs.E(CodeWorkdirForeign, "work dir and repo root must be absolute", "reason", "relative", "work_dir", o.WorkDir, "repo_root", o.RepoRoot)
 	}
@@ -98,7 +114,7 @@ func Init(ctx context.Context, deps Deps, o InitOptions) (*Machine, error) {
 		return nil, err
 	}
 	if len(allowed) > 0 && o.AllowCustomCmds != sha {
-		return nil, errs.E(CodeCmdsNotAllowed, consentList(allowed, o.WorkDir), "sha", sha)
+		return nil, errs.E(CodeCmdsNotAllowed, consentList(allowed, o.WorkDir), "sha", sha, "cmds_json", string(run.MarshalCanonical(allowed)))
 	}
 	// 3. git
 	g := deps.Git(o.WorkDir)
@@ -177,7 +193,7 @@ func Init(ctx context.Context, deps Deps, o InitOptions) (*Machine, error) {
 	initData := run.InitData{
 		RunID: runID, CreatedAt: now, Workflow: w.Name, WorkflowHash: w.Hash, Vars: vars, Calibration: o.Calibration,
 		Mock: mock, RepoMode: w.RepoMode, AllowedCmds: allowed, CmdsSHA256: sha, RepoRoot: o.RepoRoot, WorkDir: o.WorkDir,
-		BaseSHA: baseSHA, Head: head, InitialState: w.Initial, InitialKind: initialKind, Goldens: goldens, Lineage: []string{},
+		BaseSHA: baseSHA, Head: head, InitialState: w.Initial, InitialKind: initialKind, Goldens: goldens, Lineage: []string{}, WorkflowSource: source,
 	}
 	m := &Machine{deps: deps, runID: runID}
 	first := run.Event{SchemaVersion: run.SchemaVersion, At: now, Type: run.TypeInit, Data: run.MarshalCanonical(initData)}
@@ -284,7 +300,7 @@ func readGoldens(deps Deps, path string) ([]run.Golden, error) {
 // Open loads an existing run (spec 2 §5.3b).
 func Open(ctx context.Context, deps Deps, runID string, o OpenOptions) (*Machine, error) {
 	m := &Machine{deps: deps, runID: runID}
-	sess, err := m.load(ctx, o.Repair)
+	sess, err := m.loadWith(ctx, o.Repair, o.ReadOnly)
 	if err != nil {
 		return nil, err
 	}
@@ -295,10 +311,18 @@ func Open(ctx context.Context, deps Deps, runID string, o OpenOptions) (*Machine
 
 // load performs §5.3b steps 1–2 and returns a locked session.
 func (m *Machine) load(ctx context.Context, repair bool) (*session, error) {
+	return m.loadWith(ctx, repair, false)
+}
+
+// loadWith is load with the read-only variant: no lock, and it stops after the fold + sidecar parse.
+func (m *Machine) loadWith(ctx context.Context, repair, readOnly bool) (*session, error) {
 	deps := m.deps
-	unlock, err := deps.Store.Lock(m.runID)
-	if err != nil {
-		return nil, err
+	unlock := func() {}
+	if !readOnly {
+		var err error
+		if unlock, err = deps.Store.Lock(m.runID); err != nil {
+			return nil, err
+		}
 	}
 	sess := &session{m: m, ctx: ctx, unlock: unlock}
 	ok := false
@@ -366,6 +390,10 @@ func (m *Machine) load(ctx context.Context, repair bool) (*session, error) {
 		return nil, err
 	}
 	sess.w = w
+	if readOnly {
+		ok = true
+		return sess, nil
+	}
 	if err := workflow.VerifyCmds(snap.AllowedCmds, snap.WorkDir, deps.FileHash); err != nil {
 		return nil, err
 	}
@@ -482,7 +510,10 @@ func pseudoGate(name, code, detail string) *run.GateError {
 // viewOf computes the read model.
 func (s *session) viewOf() View {
 	snap := s.st.Snapshot
-	v := View{RunID: s.m.runID, Workflow: snap.Workflow, Snapshot: snap, NextAction: NextAdvance, Torn: s.m.torn}
+	v := View{RunID: s.m.runID, Workflow: snap.Workflow, Snapshot: snap, NextAction: NextAdvance, Torn: s.m.torn, Outgoing: []Edge{}}
+	for _, tr := range s.w.Outgoing(snap.State) {
+		v.Outgoing = append(v.Outgoing, Edge{To: tr.To, Gate: tr.Gate})
+	}
 	if snap.Outcome != "" {
 		v.NextAction = NextNone
 	}
@@ -493,7 +524,7 @@ func (s *session) viewOf() View {
 	if n := s.w.NodeFor(snap.State); n != nil {
 		k := run.Key(n.Name, snap.Iteration)
 		_, has := snap.NodeOutputs[k]
-		v.Node = &NodeView{Name: n.Name, Kind: n.Kind, Exec: n.Exec, HasOutput: has, Applied: snap.Applied[k]}
+		v.Node = &NodeView{Name: n.Name, Kind: n.Kind, Exec: n.Exec, Model: n.Model, Effort: n.Effort, HasOutput: has, Applied: snap.Applied[k]}
 		if snap.Outcome == "" && n.Exec != "fork" && !has && hasNeedsInput(events, n.Name, snap.Iteration) {
 			v.NextAction = NextRecord
 		}
@@ -592,6 +623,11 @@ func (s *session) advance() (AdvanceResult, error) {
 	}
 	// 5. node
 	if node != nil {
+		if _, has := snap.NodeOutputs[run.Key(node.Name, snap.Iteration)]; node.Exec == "fork" && !has && s.m.deps.Preflight != nil {
+			if err := s.m.deps.Preflight(node, snap.Calibration); err != nil {
+				return AdvanceResult{}, err
+			}
+		}
 		res, done, err := s.runNode(node, head)
 		if done || err != nil {
 			return res, err
@@ -1003,3 +1039,33 @@ func incompleteFork(snap run.Snapshot) bool {
 	}
 	return snap.Seq <= snap.ForkedAtSeq || (snap.Seq == snap.ForkedAtSeq+1 && snap.StateKind == run.KindAgentEdit)
 }
+
+// RecordLLMCall appends one llm_call under the run's lock for fsm judge --run (spec 5 §2): the machine stamps
+// State/Iter/Mock, uses the reserved node name "judge" with Index = NextIndex("judge@<iter>"), Fence = !Calibration,
+// and appends whatever the closure returns when its Kind is set (a call error is appended with error set); Kind == ""
+// means nothing to append (ctx cancellation). The closure's error is returned either way.
+func (m *Machine) RecordLLMCall(ctx context.Context, call func(context.Context, Stamp) (run.LLMCallData, error)) (int64, error) {
+	sess, err := m.load(ctx, false)
+	if err != nil {
+		return 0, err
+	}
+	defer sess.unlock()
+	defer func() { m.view = sess.viewOf() }()
+	if m.torn {
+		return 0, errs.E(run.CodeAuditTorn, "audit.jsonl has a torn tail; open with --repair", "run", m.runID)
+	}
+	snap := sess.st.Snapshot
+	if snap.Outcome != "" {
+		return 0, errs.E(CodeRunTerminal, "run is terminal; re-judge through a fork", "outcome", string(snap.Outcome))
+	}
+	stamp := Stamp{State: snap.State, Iter: snap.Iteration, Index: sess.st.NextIndex(run.Key(JudgeNode, snap.Iteration)), Calibration: snap.Calibration, Fence: !snap.Calibration}
+	data, cerr := call(ctx, stamp)
+	if data.Kind == "" {
+		return 0, cerr
+	}
+	data.Index = stamp.Index
+	if err := sess.append(run.TypeLLMCall, data, JudgeNode); err != nil {
+		return 0, err
+	}
+	return sess.st.Seq, cerr
+}
diff --git a/internal/fsm/machine/machine_test.go b/internal/fsm/machine/machine_test.go
index 48027c6..dbd6b37 100644
--- a/internal/fsm/machine/machine_test.go
+++ b/internal/fsm/machine/machine_test.go
@@ -66,7 +66,7 @@ func TestM1Init(t *testing.T) {
 	}
 	// warnings → warn events
 	noClean := strings.Replace(strings.Replace(string(raw), "  - {from: discover,   to: done,       gate: nothing_found,      outcome: clean}   # iteration 0 only: refuses once bugs are known\n", "", 1), "  - {from: adjudicate, to: done,       gate: nothing_confirmed,  outcome: clean}\n", "", 1)
-	h.files["/x/noclean.yaml"] = []byte(noClean)
+	h.files["/x/noclean.yaml"] = renamed(noClean)
 	m3 := h.mustInit(InitOptions{Workflow: "/x/noclean.yaml", Vars: sdlcVars})
 	if got := strings.Join(h.types(m3), ","); got != "init,tree,warn" {
 		t.Fatalf("warn sequence %s", got)
@@ -193,7 +193,7 @@ func TestM1InitErrors(t *testing.T) {
 	}
 	// warn append failure on a workflow with warnings
 	noClean := strings.Replace(strings.Replace(string(raw), "  - {from: discover,   to: done,       gate: nothing_found,      outcome: clean}   # iteration 0 only: refuses once bugs are known\n", "", 1), "  - {from: adjudicate, to: done,       gate: nothing_confirmed,  outcome: clean}\n", "", 1)
-	h.files["/x/noclean.yaml"] = []byte(noClean)
+	h.files["/x/noclean.yaml"] = renamed(noClean)
 	h.store.appends, h.store.failAt, h.store.err = 0, 2, errors.New("append2")
 	if _, err := h.init(InitOptions{Workflow: "/x/noclean.yaml", Vars: sdlcVars}); err == nil || err.Error() != "append2" {
 		t.Fatalf("warn append: %v", err)
@@ -205,7 +205,7 @@ func TestM1Consent(t *testing.T) {
 	h := newHarness(t)
 	raw, _ := workflows.Read("sdlc-loop")
 	withCmd := strings.Replace(string(raw), "repo_mode: advisory", "cmds:\n  notify: {argv: [bash, ./notify.sh, --tag, $JUDGE], timeout: 2, env: [SLACK_WEBHOOK]}\non_overflow: notify\nrepo_mode: advisory", 1)
-	h.files["/x/cmd.yaml"] = []byte(withCmd)
+	h.files["/x/cmd.yaml"] = renamed(withCmd)
 	_, err := h.init(InitOptions{Workflow: "/x/cmd.yaml", Vars: sdlcVars})
 	e := wantCode(t, err, CodeCmdsNotAllowed)
 	sha := e.Field("sha")
@@ -220,7 +220,7 @@ func TestM1Consent(t *testing.T) {
 		t.Fatalf("allowed: %+v", snap.AllowedCmds)
 	}
 	// cmd not found
-	h.files["/x/cmd2.yaml"] = []byte(strings.Replace(withCmd, "argv: [bash,", "argv: [nope,", 1))
+	h.files["/x/cmd2.yaml"] = renamed(strings.Replace(withCmd, "argv: [bash,", "argv: [nope,", 1))
 	if _, err := h.init(InitOptions{Workflow: "/x/cmd2.yaml", Vars: sdlcVars}); !errs.Is(err, workflow.CodeCmdNotFound) {
 		t.Fatalf("cmd not found: %v", err)
 	}
@@ -434,7 +434,7 @@ func TestM3GateFailures(t *testing.T) {
 	// Build a custom workflow whose discover has two gates that both fail on empty findings: findings_nonempty, confirmed_nonempty.
 	raw, _ := workflows.Read("review-loop")
 	custom := strings.Replace(string(raw), "  - {from: discover,   to: done,       gate: findings_empty,     outcome: clean}\n", "  - {from: discover,   to: done,       gate: confirmed_nonempty, outcome: clean}\n", 1)
-	h.files["/x/two.yaml"] = []byte(custom)
+	h.files["/x/two.yaml"] = renamed(custom)
 	m := h.mustInit(InitOptions{Workflow: "/x/two.yaml", Vars: sdlcVars})
 	h.advance(m)
 	h.record(m, "discover", `{"findings":[]}`)
@@ -590,7 +590,9 @@ func sdlcWith(t *testing.T, h *harness, name, old, new string) string {
 	if !strings.Contains(string(raw), old) {
 		t.Fatalf("fixture target missing: %s", old)
 	}
-	h.files["/x/"+name] = []byte(strings.Replace(string(raw), old, new, 1))
+	body := strings.Replace(string(raw), old, new, 1)
+	body = strings.Replace(body, "workflow: sdlc-loop", "workflow: sdlc-loop-test", 1)
+	h.files["/x/"+name] = []byte(body)
 	return "/x/" + name
 }
 
@@ -765,7 +767,7 @@ func TestM4Convergence(t *testing.T) {
 	// the real all_fixed atom firing on a confirmed_empty terminal gate → fixed (design §9 example)
 	h = newHarness(t)
 	wf = sdlcWith(t, h, "af.yaml", "  - {from: verify,     to: done,       gate: all_fixed,   outcome: fixed}", "  - {from: verify,     to: done,       gate: confirmed_empty,   outcome: fixed}")
-	h.files["/x/af.yaml"] = []byte(strings.Replace(string(h.files["/x/af.yaml"]), "any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]", "any: [all_fixed, {max_iterations: 5}]", 1))
+	h.files["/x/af.yaml"] = renamed(strings.Replace(string(h.files["/x/af.yaml"]), "any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]", "any: [all_fixed, {max_iterations: 5}]", 1))
 	h.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
 	m = h.mustInit(InitOptions{Workflow: wf, Vars: sdlcVars})
 	h.advance(m)
@@ -781,7 +783,7 @@ func TestM4Convergence(t *testing.T) {
 	// user workflow whose terminal gate is confirmed_empty: convergence still bounds the loop
 	h = newHarness(t)
 	wf = sdlcWith(t, h, "ce.yaml", "  - {from: verify,     to: done,       gate: all_fixed,   outcome: fixed}", "  - {from: verify,     to: done,       gate: confirmed_empty,   outcome: fixed}")
-	h.files["/x/ce.yaml"] = []byte(strings.Replace(string(h.files["/x/ce.yaml"]), "{max_iterations: 5}", "{max_iterations: 1}", 1))
+	h.files["/x/ce.yaml"] = renamed(strings.Replace(string(h.files["/x/ce.yaml"]), "{max_iterations: 5}", "{max_iterations: 1}", 1))
 	h.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
 	m = h.mustInit(InitOptions{Workflow: wf, Vars: sdlcVars})
 	h.advance(m)
@@ -1195,7 +1197,7 @@ func TestM7Open(t *testing.T) {
 	// stored vars no longer resolve (var removed from the registry-independent workflow is impossible; simulate with a sidecar whose vars differ)
 	// ERR_CMD_CHANGED via the consent list
 	withCmd := strings.Replace(raw2(t, "sdlc-loop"), "repo_mode: advisory", "cmds:\n  notify: {argv: [bash]}\nrepo_mode: advisory", 1)
-	h.files["/x/cmd.yaml"] = []byte(withCmd)
+	h.files["/x/cmd.yaml"] = renamed(withCmd)
 	_, cerr := h.init(InitOptions{Workflow: "/x/cmd.yaml", Vars: sdlcVars})
 	sha := errs.As(cerr).Field("sha")
 	mc := h.mustInit(InitOptions{Workflow: "/x/cmd.yaml", Vars: sdlcVars, AllowCustomCmds: sha})
@@ -1878,3 +1880,18 @@ func TestOpenIncompleteFork(t *testing.T) {
 		t.Fatal(err)
 	}
 }
+
+// renamed gives a path fixture derived from an embedded workflow its own name: a path may not reuse an embedded
+// name with different bytes (reserved_name).
+func renamed(body any) []byte {
+	var text string
+	switch b := body.(type) {
+	case string:
+		text = b
+	case []byte:
+		text = string(b)
+	}
+	text = strings.Replace(text, "workflow: sdlc-loop", "workflow: sdlc-loop-test", 1)
+	text = strings.Replace(text, "workflow: review-loop", "workflow: review-loop-test", 1)
+	return []byte(text)
+}
diff --git a/internal/fsm/machine/types.go b/internal/fsm/machine/types.go
index 90b5efc..659552a 100644
--- a/internal/fsm/machine/types.go
+++ b/internal/fsm/machine/types.go
@@ -97,6 +97,9 @@ type Deps struct {
 	Nonce     func() string
 	MockLoad  func(dir string) (hash string, err error)
 	Terminal  func(ctx context.Context, v View) error
+	// Preflight (optional) runs before Create for every exec: fork node at Init, and on the Advance that would run a
+	// fork node, before any append of that node's work (spec 5 §8: judge pre-flight).
+	Preflight func(node *workflow.Node, calibration bool) error
 }
 
 // InitOptions parameterizes Init.
@@ -116,7 +119,8 @@ type InitOptions struct {
 
 // OpenOptions parameterizes Open.
 type OpenOptions struct {
-	Repair bool
+	Repair   bool
+	ReadOnly bool // no lock; load stops after the fold + sidecar parse (spec 5: fsm state and the read-only commands)
 }
 
 // Statuses of an Advance.
@@ -192,6 +196,25 @@ type View struct {
 	NextAction string
 	Torn       bool
 	FailedGate *run.GateData
+	Outgoing   []Edge // the current state's transitions in declaration order
+}
+
+// Edge is one outgoing transition of the current state.
+type Edge struct {
+	To   run.State
+	Gate string
+}
+
+// JudgeNode is the reserved node name of fsm judge --run's llm_calls.
+const JudgeNode = "judge"
+
+// Stamp is what RecordLLMCall hands its closure (spec 5 §2).
+type Stamp struct {
+	State       run.State
+	Iter        int
+	Index       int
+	Calibration bool
+	Fence       bool
 }
 
 // Decision is a judge verdict's decision as spec 3 §4 compares it: Raw is the kind's decision field, Effective the
@@ -201,6 +224,7 @@ type Decision struct{ Raw, Effective *bool }
 // NodeView describes the current state's node.
 type NodeView struct {
 	Name, Kind, Exec   string
+	Model, Effort      string
 	HasOutput, Applied bool
 }


--- docs/tasks/m8-cli-suite-docs.md
+# M8 — CLI, judge/mockai/converge handoffs, black-box suite, docs
+
+Implements spec 4 r5 (`docs/specs/2026-08-27-metareview-0.9.0-fsm-cli.md`) plus the spec 2/5 handoffs:
+`judge.Preflight`, `mockai.MaxFileBytes`, `converge.Describe`; `machine` `OpenOptions`, `Deps.Preflight`, `NodeView`,
+`View.Outgoing`, `RecordLLMCall`, `Init` workflow-source stamps; `internal/fsm/cli` (`Deps` seams, `RealDeps`, `Run`,
+envelopes, `exitFor`, `StatusLines`, `AgentPrompt`) wired into `cmd/metareview` (`fsm` branch, status section);
+`tests/go/test-fsm.sh` over the mock scenarios under `testdata/fsm/scenarios`; `/fsm` skill, `commands/fsm.md`,
+`docs/fsm/`, README/INSTALL/quickstart/AGENTS/CLAUDE/CHANGELOG/manifest amendments.
+
+Done when every `internal/fsm/*` package and `workflows/` is at exactly 100% statement coverage and the legacy
+packages hold their recorded floor (`tests/coverage.sh`), `tests/run-all.sh` is green, and `go vet` is clean.
```

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

# unit statement coverage at 22cd870266b3bd18540b8a18a495fbc834542326 (2026-08-27T11:38:49Z)
ok  	github.com/dsifry/metareview/internal/fsm/cli	(cached)	coverage: 100.0% of statements
ok  	github.com/dsifry/metareview/internal/fsm/cmdexec	1.133s	coverage: 100.0% of statements
ok  	github.com/dsifry/metareview/internal/fsm/converge	(cached)	coverage: 100.0% of statements
ok  	github.com/dsifry/metareview/internal/fsm/errs	(cached)	coverage: 100.0% of statements
ok  	github.com/dsifry/metareview/internal/fsm/export	1.189s	coverage: 100.0% of statements
ok  	github.com/dsifry/metareview/internal/fsm/gate	2.080s	coverage: 100.0% of statements
ok  	github.com/dsifry/metareview/internal/fsm/judge	(cached)	coverage: 100.0% of statements
ok  	github.com/dsifry/metareview/internal/fsm/kind	1.496s	coverage: 100.0% of statements
ok  	github.com/dsifry/metareview/internal/fsm/machine	2.137s	coverage: 99.9% of statements
ok  	github.com/dsifry/metareview/internal/fsm/mockai	(cached)	coverage: 100.0% of statements
ok  	github.com/dsifry/metareview/internal/fsm/record	2.215s	coverage: 100.0% of statements
ok  	github.com/dsifry/metareview/internal/fsm/run	(cached)	coverage: 100.0% of statements
ok  	github.com/dsifry/metareview/internal/fsm/workflow	1.757s	coverage: 100.0% of statements
ok  	github.com/dsifry/metareview/workflows	(cached)	coverage: 100.0% of statements
	github.com/dsifry/metareview/cmd/metareview		coverage: 0.0% of statements

go vet: clean

