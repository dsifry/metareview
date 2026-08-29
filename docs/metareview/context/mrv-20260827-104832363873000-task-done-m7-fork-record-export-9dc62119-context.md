# metareview task-done context

Run ID: `mrv-20260827-104832363873000-task-done-m7-fork-record-export-9dc62119`

## Task

# M7 — fork/resume, diff, export, runs.jsonl record

Implements spec 3 r5 (`docs/specs/2026-08-27-metareview-0.9.0-fsm-fork.md`): the `run` amendments (`WorkflowSource`,
`TornFiles`/`MaxEvents`/`Counted`, incomplete-fork rule), `machine.Fork`/`VerifyOrigin`/`DiffRuns` + `machine.Decision` +
`ERR_FORK_INCOMPLETE`, `kind.Decision` + judge-less registries, `internal/fsm/record` (terminal recorder, `Exists`,
torn-safe writer), `internal/fsm/export` (redaction table, redacted snapshot, manifest, `FS` seam).

Done when every touched `internal/fsm/*` package is at exactly 100% statement coverage (`tests/coverage.sh`) and
`go vet` is clean.


## Git

- Base: `a5a78ee6f1fa0a93669f61553b4ad57b7621ee40`
- Head: `f5e09af38bc15a1ec08cabe14fd069309917ced2`
- Branch: ``
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `11282`
- Filtered diff bytes: `11282`
- Risk level: `none`



## Review Manifest

- Manifest verdict: `NEEDS_REVISION`
- Source manifest hash: `374947e37f4c29fd`
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- docs/tasks/m7-fork-record-export.md
- internal/fsm/kind/decision.go
- internal/fsm/kind/decision_test.go
- internal/fsm/kind/kind.go
- internal/fsm/kind/kind_test.go
- internal/fsm/machine/machine.go
- internal/fsm/machine/machine_test.go
- internal/fsm/machine/types.go

### Shards
- shard-01: docs/tasks/m7-fork-record-export.md, internal/fsm/kind/decision.go, internal/fsm/kind/decision_test.go, internal/fsm/kind/kind.go, internal/fsm/kind/kind_test.go, internal/fsm/machine/machine.go, internal/fsm/machine/machine_test.go, internal/fsm/machine/types.go

### Manifest Blockers
- missing shard result for shard-01

## Changed Files

- internal/fsm/kind/decision.go
- internal/fsm/kind/decision_test.go
- internal/fsm/kind/kind.go
- internal/fsm/kind/kind_test.go
- internal/fsm/machine/machine.go
- internal/fsm/machine/machine_test.go
- internal/fsm/machine/types.go
- docs/tasks/m7-fork-record-export.md

## Diff

```diff
diff --git a/internal/fsm/kind/decision.go b/internal/fsm/kind/decision.go
new file mode 100644
index 0000000..7965e1f
--- /dev/null
+++ b/internal/fsm/kind/decision.go
@@ -0,0 +1,51 @@
+package kind
+
+import (
+	"encoding/json"
+
+	"github.com/dsifry/metareview/internal/fsm/errs"
+	"github.com/dsifry/metareview/internal/fsm/judge"
+	"github.com/dsifry/metareview/internal/fsm/machine"
+)
+
+func errNoJudge() error {
+	return errs.E(machine.CodeExecutorFailed, "this registry has no judge", "reason", "no_judge")
+}
+
+// Decision extracts a stored judge verdict's decision (spec 3 §1): Raw is the kind's decision field (match / is_real /
+// still_present), nil when the verdict is null, the field is absent or null, or the kind is unknown; Effective is the
+// kind's per-call rule — is_real ∧ confidence ≥ AdjudicateThreshold for adjudicate, still_present for still-present,
+// and Raw for match (the match rule is a relative argmax across candidates, so a single verdict has no other rule).
+func Decision(kind string, verdict json.RawMessage) machine.Decision {
+	var v struct {
+		Match        *bool    `json:"match"`
+		IsReal       *bool    `json:"is_real"`
+		StillPresent *bool    `json:"still_present"`
+		Confidence   *float64 `json:"confidence"`
+	}
+	if len(verdict) == 0 || json.Unmarshal(verdict, &v) != nil {
+		return machine.Decision{}
+	}
+	var raw *bool
+	switch kind {
+	case judge.KindMatch:
+		raw = v.Match
+	case judge.KindAdjudicate:
+		raw = v.IsReal
+	case judge.KindStillPresent:
+		raw = v.StillPresent
+	}
+	if raw == nil {
+		return machine.Decision{}
+	}
+	r := *raw
+	eff := r
+	if kind == judge.KindAdjudicate {
+		conf := 0.0
+		if v.Confidence != nil {
+			conf = *v.Confidence
+		}
+		eff = r && conf >= AdjudicateThreshold
+	}
+	return machine.Decision{Raw: &r, Effective: &eff}
+}
diff --git a/internal/fsm/kind/decision_test.go b/internal/fsm/kind/decision_test.go
new file mode 100644
index 0000000..390406d
--- /dev/null
+++ b/internal/fsm/kind/decision_test.go
@@ -0,0 +1,61 @@
+package kind
+
+import (
+	"context"
+	"encoding/json"
+	"testing"
+
+	"github.com/dsifry/metareview/internal/fsm/errs"
+	"github.com/dsifry/metareview/internal/fsm/judge"
+	"github.com/dsifry/metareview/internal/fsm/machine"
+)
+
+func bp(b bool) *bool { return &b }
+
+func TestDecision(t *testing.T) {
+	eq := func(a, b *bool) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }
+	cases := []struct {
+		name, kind, verdict string
+		raw, eff            *bool
+	}{
+		{"match true", judge.KindMatch, `{"match":true,"confidence":0.2}`, bp(true), bp(true)},
+		{"match false", judge.KindMatch, `{"match":false,"confidence":0.9}`, bp(false), bp(false)},
+		{"adjudicate real above threshold", judge.KindAdjudicate, `{"is_real":true,"confidence":0.7}`, bp(true), bp(true)},
+		{"adjudicate real below threshold", judge.KindAdjudicate, `{"is_real":true,"confidence":0.6}`, bp(true), bp(false)},
+		{"adjudicate real no confidence", judge.KindAdjudicate, `{"is_real":true}`, bp(true), bp(false)},
+		{"adjudicate not real", judge.KindAdjudicate, `{"is_real":false,"confidence":0.99}`, bp(false), bp(false)},
+		{"still-present", judge.KindStillPresent, `{"still_present":true,"confidence":0.5}`, bp(true), bp(true)},
+		{"still-present null bool", judge.KindStillPresent, `{"still_present":null,"confidence":0.5}`, nil, nil},
+		{"null verdict", judge.KindAdjudicate, `null`, nil, nil},
+		{"empty verdict", judge.KindAdjudicate, ``, nil, nil},
+		{"absent field", judge.KindAdjudicate, `{"match":true}`, nil, nil},
+		{"unknown kind", "other", `{"match":true}`, nil, nil},
+		{"garbage", judge.KindMatch, `{`, nil, nil},
+	}
+	for _, c := range cases {
+		d := Decision(c.kind, json.RawMessage(c.verdict))
+		if !eq(d.Raw, c.raw) || !eq(d.Effective, c.eff) {
+			t.Fatalf("%s: got raw=%v eff=%v", c.name, d.Raw, d.Effective)
+		}
+	}
+}
+
+func TestNilJudgeExecutors(t *testing.T) {
+	r, err := New(Deps{})
+	if err != nil {
+		t.Fatal(err)
+	}
+	for _, name := range []string{MatchThenAdjudicate, StillPresent} {
+		e, ok := r.Executor(name)
+		if !ok {
+			t.Fatalf("%s executor", name)
+		}
+		_, err := e.Execute(context.Background(), machine.ExecInput{})
+		if !errs.Is(err, machine.CodeExecutorFailed) {
+			t.Fatalf("%s without a judge: %v", name, err)
+		}
+		if e2 := errs.As(err); e2 == nil || e2.Fields["reason"] != "no_judge" {
+			t.Fatalf("reason: %v", err)
+		}
+	}
+}
diff --git a/internal/fsm/kind/kind.go b/internal/fsm/kind/kind.go
index 11b8513..d68a7a2 100644
--- a/internal/fsm/kind/kind.go
+++ b/internal/fsm/kind/kind.go
@@ -66,10 +66,11 @@ type Registry struct {
 	mock  bool
 }
 
-// New builds the registry; Mock must agree with the judge's type.
+// New builds the registry; Mock must agree with the judge's type. A nil judge is allowed (judge-less commands, spec 5
+// r4) with Mock false: executors reached without a judge fail ERR_EXECUTOR_FAILED{reason: no_judge}.
 func New(d Deps) (*Registry, error) {
 	_, isMock := d.Judge.(*judge.MockJudge)
-	if d.Judge == nil || isMock != d.Mock {
+	if isMock != d.Mock {
 		return nil, errs.E(CodeMockMismatch, "Mock must be true exactly when the judge is a MockJudge", "mock", fmt.Sprint(d.Mock))
 	}
 	r := &Registry{mock: d.Mock, kinds: map[string]machine.NodeKind{}, execs: map[string]machine.Executor{}}
@@ -400,6 +401,9 @@ func dedupCandidates(fs []run.Finding) []run.Finding {
 }
 
 func (e *adjudicateExec) Execute(ctx context.Context, in machine.ExecInput) (json.RawMessage, error) {
+	if e.judge == nil {
+		return nil, errNoJudge()
+	}
 	snap := in.Snap
 	cands := dedupCandidates(snap.Findings)
 	goldens := snap.Goldens
@@ -570,6 +574,9 @@ func (stillPresentKind) Reduce(_ run.Snapshot, out any) (run.Delta, error) {
 type stillPresentExec struct{ judge judge.Judge }
 
 func (e *stillPresentExec) Execute(ctx context.Context, in machine.ExecInput) (json.RawMessage, error) {
+	if e.judge == nil {
+		return nil, errNoJudge()
+	}
 	if len(in.Snap.AllFound) > run.MaxDeltaList {
 		return nil, errs.E(CodeTooManyBugs, fmt.Sprintf("%d bugs known (max %d)", len(in.Snap.AllFound), run.MaxDeltaList))
 	}
diff --git a/internal/fsm/kind/kind_test.go b/internal/fsm/kind/kind_test.go
index 37b928a..86a2a09 100644
--- a/internal/fsm/kind/kind_test.go
+++ b/internal/fsm/kind/kind_test.go
@@ -92,8 +92,11 @@ func TestK7Registry(t *testing.T) {
 	if _, err := New(Deps{Judge: realStub{}, Mock: true}); !errs.Is(err, CodeMockMismatch) {
 		t.Fatal("real judge with Mock")
 	}
-	if _, err := New(Deps{}); !errs.Is(err, CodeMockMismatch) {
-		t.Fatal("nil judge")
+	if _, err := New(Deps{Mock: true}); !errs.Is(err, CodeMockMismatch) {
+		t.Fatal("nil judge with Mock")
+	}
+	if nj, err := New(Deps{}); err != nil || nj.Mock() {
+		t.Fatalf("nil judge without Mock must build a judge-less registry: %v", err)
 	}
 	r := mustNew(t, m, true)
 	if !r.Mock() {
diff --git a/internal/fsm/machine/machine.go b/internal/fsm/machine/machine.go
index ed2ef8b..5e9359c 100644
--- a/internal/fsm/machine/machine.go
+++ b/internal/fsm/machine/machine.go
@@ -345,6 +345,9 @@ func (m *Machine) load(ctx context.Context, repair bool) (*session, error) {
 	st.ChainHead = log.Head
 	sess.st, sess.log = st, log
 	snap := st.Snapshot
+	if incompleteFork(snap) {
+		return nil, errs.E(CodeForkIncomplete, "fork was not completed; delete the run directory", "run", m.runID, "parent", snap.ParentRunID)
+	}
 	raw, err := deps.Sidecar.Read(m.runID, SidecarWorkflow)
 	if err != nil {
 		return nil, err
@@ -991,3 +994,12 @@ var sortedEventTypes = func() []string {
 	sort.Strings(s)
 	return s
 }()
+
+// incompleteFork is spec 3 §2 step 8: a forked child whose step-8 write did not finish (no rebaseline tree, or a tree
+// without the fix_baseline an agent-edit checkpoint requires). Checked before the sidecar read.
+func incompleteFork(snap run.Snapshot) bool {
+	if snap.ParentRunID == "" {
+		return false
+	}
+	return snap.Seq <= snap.ForkedAtSeq || (snap.Seq == snap.ForkedAtSeq+1 && snap.StateKind == run.KindAgentEdit)
+}
diff --git a/internal/fsm/machine/machine_test.go b/internal/fsm/machine/machine_test.go
index c371ffa..48027c6 100644
--- a/internal/fsm/machine/machine_test.go
+++ b/internal/fsm/machine/machine_test.go
@@ -1836,3 +1836,45 @@ func TestM9Residue(t *testing.T) {
 		t.Fatal("read bad id")
 	}
 }
+
+func TestOpenIncompleteFork(t *testing.T) {
+	h := newHarness(t)
+	ctx := context.Background()
+	m := h.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
+	h.advance(m) // discover NEEDS_INPUT
+	parent := h.events(m)
+	child := "mrv-crafted-incomplete-fork"
+	evs := run.NewBuilder(child).Copy(parent, 2, int64(len(parent)), child, func(d *run.InitData) {
+		d.ParentRunID = m.runID
+		d.Lineage = []string{m.runID}
+		d.ForkedAtSeq = int64(len(parent))
+	})
+	st, err := h.store.Create(child, evs[0])
+	if err != nil {
+		t.Fatal(err)
+	}
+	unlock, err := h.store.Lock(child)
+	if err != nil {
+		t.Fatal(err)
+	}
+	for _, ev := range evs[1:] {
+		if st, err = h.store.Append(child, st, ev); err != nil {
+			t.Fatal(err)
+		}
+	}
+	unlock()
+	// no sidecar was written: the incomplete-fork check must fire before the sidecar read
+	if _, err := Open(ctx, h.deps, child, OpenOptions{}); !errs.Is(err, CodeForkIncomplete) {
+		t.Fatalf("expected ERR_FORK_INCOMPLETE, got %v", err)
+	}
+	list, _ := h.store.List()
+	for _, s := range list {
+		if s.RunID == child && !strings.Contains(s.Error, "incomplete fork") {
+			t.Fatalf("List must flag it: %+v", s)
+		}
+	}
+	// a root run is unaffected
+	if _, err := Open(ctx, h.deps, m.runID, OpenOptions{}); err != nil {
+		t.Fatal(err)
+	}
+}
diff --git a/internal/fsm/machine/types.go b/internal/fsm/machine/types.go
index fd01205..90b5efc 100644
--- a/internal/fsm/machine/types.go
+++ b/internal/fsm/machine/types.go
@@ -194,6 +194,10 @@ type View struct {
 	FailedGate *run.GateData
 }
 
+// Decision is a judge verdict's decision as spec 3 §4 compares it: Raw is the kind's decision field, Effective the
+// kind's per-call rule applied to it (nil whenever Raw is nil). Declared here because kind imports machine.
+type Decision struct{ Raw, Effective *bool }
+
 // NodeView describes the current state's node.
 type NodeView struct {
 	Name, Kind, Exec   string
@@ -211,6 +215,7 @@ const (
 	CodeMockMismatch       = "ERR_MOCK_MISMATCH"
 	CodeBadRepoMode        = "ERR_BAD_REPO_MODE"
 	CodeSidecar            = "ERR_SIDECAR"
+	CodeForkIncomplete     = "ERR_FORK_INCOMPLETE"
 	CodeRunTerminal        = "ERR_RUN_TERMINAL"
 	CodeWorkflowChanged    = "ERR_WORKFLOW_CHANGED"
 	CodeNodeMismatch       = "ERR_NODE_MISMATCH"


--- docs/tasks/m7-fork-record-export.md
+# M7 — fork/resume, diff, export, runs.jsonl record
+
+Implements spec 3 r5 (`docs/specs/2026-08-27-metareview-0.9.0-fsm-fork.md`): the `run` amendments (`WorkflowSource`,
+`TornFiles`/`MaxEvents`/`Counted`, incomplete-fork rule), `machine.Fork`/`VerifyOrigin`/`DiffRuns` + `machine.Decision` +
+`ERR_FORK_INCOMPLETE`, `kind.Decision` + judge-less registries, `internal/fsm/record` (terminal recorder, `Exists`,
+torn-safe writer), `internal/fsm/export` (redaction table, redacted snapshot, manifest, `FS` seam).
+
+Done when every touched `internal/fsm/*` package is at exactly 100% statement coverage (`tests/coverage.sh`) and
+`go vet` is clean.
```

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence


> metareview@0.8.2 prepack
> npm run build


> metareview@0.8.2 build
> go build -o bin/metareview ./cmd/metareview

cmd/metareview                                      80.4%  ok
internal/artifactreview                             80.4%  ok
internal/contextpack                                76.1%  ok
internal/contextprofile                             84.6%  ok
internal/epicready                                  81.6%  ok
internal/epicsource                                 83.1%  ok
internal/evidence                                   85.2%  ok
internal/findings                                   90.5%  ok
internal/fsm/cmdexec                               100.0%  ok
internal/fsm/converge                              100.0%  ok
internal/fsm/errs                                  100.0%  ok
internal/fsm/export                                100.0%  ok
internal/fsm/gate                                  100.0%  ok
internal/fsm/judge                                 100.0%  ok
internal/fsm/kind                                  100.0%  ok
internal/fsm/machine                               100.0%  ok
internal/fsm/mockai                                100.0%  ok
internal/fsm/record                                100.0%  ok
internal/fsm/run                                   100.0%  ok
internal/fsm/workflow                              100.0%  ok
internal/gitcontext                                 83.7%  ok
internal/githubcontext                              95.9%  ok
internal/integration                               100.0%  ok
internal/knowledge                                  77.8%  ok
internal/learning                                   88.0%  ok
internal/learnsource                                70.8%  ok
internal/markdown                                   70.0%  ok
internal/prready                                    85.7%  ok
internal/repo                                       87.9%  ok
internal/reviewers                                  97.2%  ok
internal/reviewlog                                  90.2%  ok
internal/reviewmanifest                             90.5%  ok
internal/reviewstate                                92.1%  ok
internal/runchain                                   90.1%  ok
internal/sessionhistory                             86.2%  ok
internal/setup                                      88.5%  ok
internal/state                                      81.6%  ok
internal/taskdone                                   87.0%  ok
internal/tasksource                                 79.2%  ok
workflows                                          100.0%  ok
coverage gate passed
coverage exit=0

