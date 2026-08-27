# metareview task-done context

Run ID: `mrv-20260827-090548364954000-task-done-m1-m6-fsm-packages-a99c72f1`

## Task

# M1–M6: internal/fsm core packages

Implement `internal/fsm/{errs,converge,gate,workflow,machine,cmdexec,judge,mockai,kind}` per
`docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md` (r4) and `docs/specs/2026-08-27-metareview-0.9.0-fsm-judge-kinds.md`
(r5), test-first, under the combined coverage gate (`tests/coverage.sh`), reviewed per commit range (≤ 120 KB each).

## Acceptance

- Every §7/§8 test row has a discriminating test (literal pins; goldens regression-only behind an env flag).
- `go test ./internal/fsm/...` passes; every `internal/fsm/*` package at exactly 100% statements.
- `bash tests/coverage.sh` passes (legacy floor held).
- Dependency direction per spec 2 §1 (machine imports no kinds/judge/cmdexec/workflows).
- Every LLM/shell effect behind an interface; no shell, pinned argv, exact env in `cmdexec`.


## Git

- Base: `8a7454c0f095d4cf9b16a0d977ac724832f73824`
- Head: `1ea94b7cd6f8fe181cf185b9f66deba1f999314b`
- Branch: ``
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `150961`
- Filtered diff bytes: `100261`
- Risk level: `none`
- Generated files excluded: docs/metareview/FINDINGS.md, docs/metareview/context/mrv-20260827-090548007669000-task-done-m1-m6-fsm-packages-a99c72f1-context.md, docs/metareview/reviews/mrv-20260827-090548007669000-task-done-m1-m6-fsm-packages-a99c72f1.md



## Review Manifest

- Manifest verdict: `NEEDS_REVISION`
- Source manifest hash: `a92f91903553fcc4`
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- internal/fsm/machine/harness_test.go
- internal/fsm/machine/helpers_test.go
- internal/fsm/machine/machine_test.go

### Path Dispositions
- docs/metareview/FINDINGS.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/context/mrv-20260827-090548007669000-task-done-m1-m6-fsm-packages-a99c72f1-context.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/reviews/mrv-20260827-090548007669000-task-done-m1-m6-fsm-packages-a99c72f1.md: generated (metareview generated review artifact excluded from source manifest)

### Shards
- shard-01: internal/fsm/machine/machine_test.go
- shard-02: internal/fsm/machine/harness_test.go, internal/fsm/machine/helpers_test.go

### Manifest Blockers
- missing cross-shard result
- missing shard result for shard-01
- missing shard result for shard-02

## Changed Files

- internal/fsm/machine/harness_test.go
- internal/fsm/machine/helpers_test.go
- internal/fsm/machine/machine_test.go

## Diff

```diff
diff --git a/internal/fsm/machine/harness_test.go b/internal/fsm/machine/harness_test.go
new file mode 100644
index 0000000..346c2c9
--- /dev/null
+++ b/internal/fsm/machine/harness_test.go
@@ -0,0 +1,430 @@
+package machine
+
+import (
+	"context"
+	"encoding/json"
+	"errors"
+	"fmt"
+	"strings"
+	"sync"
+	"testing"
+	"time"
+
+	"github.com/dsifry/metareview/internal/fsm/converge"
+	"github.com/dsifry/metareview/internal/fsm/errs"
+	"github.com/dsifry/metareview/internal/fsm/gate"
+	"github.com/dsifry/metareview/internal/fsm/run"
+	"github.com/dsifry/metareview/internal/fsm/workflow"
+	"github.com/dsifry/metareview/workflows"
+)
+
+const (
+	shaBase = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
+	shaHead = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
+	shaFix  = "cccccccccccccccccccccccccccccccccccccccc"
+)
+
+// ---- kinds ----
+
+type fakeKind struct {
+	name    string
+	info    workflow.KindInfo
+	decode  func(json.RawMessage) (any, error)
+	reduce  func(run.Snapshot, any) (run.Delta, error)
+	instr   func(run.Snapshot, *workflow.Node, Diff, string) (Instructions, error)
+	seenIns []Instructions
+}
+
+func (k *fakeKind) Name() string            { return k.name }
+func (k *fakeKind) Info() workflow.KindInfo { return k.info }
+func (k *fakeKind) Decode(raw json.RawMessage) (any, error) {
+	if k.decode != nil {
+		return k.decode(raw)
+	}
+	var d run.Delta
+	dec := json.NewDecoder(strings.NewReader(string(raw)))
+	dec.DisallowUnknownFields()
+	if err := dec.Decode(&d); err != nil {
+		return nil, err
+	}
+	return d, nil
+}
+func (k *fakeKind) Reduce(s run.Snapshot, out any) (run.Delta, error) {
+	if k.reduce != nil {
+		return k.reduce(s, out)
+	}
+	return out.(run.Delta), nil
+}
+func (k *fakeKind) Instructions(s run.Snapshot, n *workflow.Node, d Diff, nonce string) (Instructions, error) {
+	if k.instr != nil {
+		return k.instr(s, n, d, nonce)
+	}
+	ins := Instructions{Text: "do " + n.Name + " nonce=" + nonce, Input: map[string]any{"diff": d.Text, "diff_truncated": d.Truncated, "iteration": s.Iteration}, Untrusted: []string{"diff"}, OutputSchema: json.RawMessage(`{}`)}
+	k.seenIns = append(k.seenIns, ins)
+	return ins, nil
+}
+
+type fakeExecutor struct {
+	fn    func(ExecInput) (json.RawMessage, error)
+	calls []ExecInput
+}
+
+func (e *fakeExecutor) Execute(_ context.Context, in ExecInput) (json.RawMessage, error) {
+	e.calls = append(e.calls, in)
+	return e.fn(in)
+}
+
+type fakeRegistry struct {
+	kinds map[string]*fakeKind
+	execs map[string]*fakeExecutor
+	mock  bool
+}
+
+func (r *fakeRegistry) Kind(n string) (NodeKind, bool) {
+	k, ok := r.kinds[n]
+	return k, ok
+}
+func (r *fakeRegistry) Executor(n string) (Executor, bool) {
+	e, ok := r.execs[n]
+	return e, ok
+}
+func (r *fakeRegistry) Info() map[string]workflow.KindInfo {
+	m := map[string]workflow.KindInfo{}
+	for n, k := range r.kinds {
+		m[n] = k.info
+	}
+	return m
+}
+func (r *fakeRegistry) Mock() bool { return r.mock }
+
+// llmCall emits one llm_call through the audit closure.
+func llmCall(in ExecInput, i int, tokens int64) error {
+	d := run.LLMCallData{Kind: "match", Model: in.Node.Model, Effort: in.Node.Effort, Index: in.StartIndex + i, InputHash: "h", Verdict: json.RawMessage(`{"match":true}`), Confidence: 0.9, Tokens: run.TokenTotals{Input: tokens}}
+	return in.Audit(run.Event{Type: run.TypeLLMCall, Data: run.MarshalCanonical(d)})
+}
+
+func newRegistry() *fakeRegistry {
+	r := &fakeRegistry{kinds: map[string]*fakeKind{}, execs: map[string]*fakeExecutor{}}
+	r.kinds["review-lenses"] = &fakeKind{name: "review-lenses", info: workflow.KindInfo{DefaultExec: "subagent", AllowedExec: []string{"inline", "subagent"}}}
+	r.kinds["match-then-adjudicate"] = &fakeKind{name: "match-then-adjudicate", info: workflow.KindInfo{DefaultExec: "fork", AllowedExec: []string{"fork"}}}
+	r.kinds["agent-edit"] = &fakeKind{name: "agent-edit", info: workflow.KindInfo{DefaultExec: "inline", AllowedExec: []string{"inline", "subagent"}}}
+	r.kinds["still-present"] = &fakeKind{name: "still-present", info: workflow.KindInfo{DefaultExec: "fork", AllowedExec: []string{"fork"}}}
+	r.kinds["cmd"] = &fakeKind{name: "cmd", info: workflow.KindInfo{DefaultExec: "fork", AllowedExec: []string{"fork"}}}
+	// agent-edit output is {commit, summary}; reduce to Commit
+	r.kinds["agent-edit"].decode = func(raw json.RawMessage) (any, error) {
+		var o struct {
+			Commit  string `json:"commit"`
+			Summary string `json:"summary"`
+		}
+		dec := json.NewDecoder(strings.NewReader(string(raw)))
+		dec.DisallowUnknownFields()
+		if err := dec.Decode(&o); err != nil {
+			return nil, err
+		}
+		return o.Commit, nil
+	}
+	r.kinds["agent-edit"].reduce = func(_ run.Snapshot, out any) (run.Delta, error) { return run.Delta{Commit: out.(string)}, nil }
+	// default executors: adjudicate confirms every finding after one llm_call each; verify says all fixed
+	r.execs["match-then-adjudicate"] = &fakeExecutor{fn: func(in ExecInput) (json.RawMessage, error) {
+		var bugs []run.Bug
+		for i, f := range in.Snap.Findings {
+			if err := llmCall(in, i, 10); err != nil {
+				return nil, err
+			}
+			bugs = append(bugs, run.Bug{ID: run.BugID(f.IssueText), Desc: f.IssueText, Verdict: "real_but_ungold", Confidence: 0.9})
+		}
+		return json.RawMessage(run.MarshalCanonical(run.Delta{Confirmed: bugs})), nil
+	}}
+	r.execs["still-present"] = &fakeExecutor{fn: func(in ExecInput) (json.RawMessage, error) {
+		var st []run.BugStatus
+		for i, b := range in.Snap.AllFound {
+			if err := llmCall(in, i, 5); err != nil {
+				return nil, err
+			}
+			st = append(st, run.BugStatus{ID: b.ID, StillPresent: false, Confidence: 1})
+		}
+		return json.RawMessage(run.MarshalCanonical(run.Delta{Status: st})), nil
+	}}
+	r.execs["cmd"] = &fakeExecutor{fn: func(in ExecInput) (json.RawMessage, error) { return json.RawMessage(`{}`), nil }}
+	return r
+}
+
+// ---- git ----
+
+// gitFakes returns the same fake for every dir unless overridden.
+type gitFakes struct {
+	byDir map[string]gate.Git
+	def   *gate.Fake
+}
+
+func (g *gitFakes) get(dir string) gate.Git {
+	if f, ok := g.byDir[dir]; ok {
+		return f
+	}
+	return g.def
+}
+
+// failingGit fails exactly one method with err.
+type failingGit struct {
+	gate.Git
+	at  string
+	err error
+}
+
+func (f *failingGit) Head(ctx context.Context) (string, error) {
+	if f.at == "Head" {
+		return "", f.err
+	}
+	return f.Git.Head(ctx)
+}
+func (f *failingGit) RevParse(ctx context.Context, r string) (string, error) {
+	if f.at == "RevParse" {
+		return "", f.err
+	}
+	return f.Git.RevParse(ctx, r)
+}
+func (f *failingGit) Status(ctx context.Context) (bool, string, error) {
+	if f.at == "Status" {
+		return false, "", f.err
+	}
+	return f.Git.Status(ctx)
+}
+func (f *failingGit) WorkTree(ctx context.Context) (string, error) {
+	if f.at == "WorkTree" {
+		return "", f.err
+	}
+	return f.Git.WorkTree(ctx)
+}
+func (f *failingGit) Diff(ctx context.Context, a, b string, n int) (string, bool, error) {
+	if f.at == "Diff" {
+		return "", false, f.err
+	}
+	return f.Git.Diff(ctx, a, b, n)
+}
+func (f *failingGit) CommonDir(ctx context.Context) (string, error) {
+	if f.at == "CommonDir" {
+		return "", f.err
+	}
+	return f.Git.CommonDir(ctx)
+}
+
+// ---- store wrappers ----
+
+// countingStore fails the Nth append (1-based) or a named method.
+type countingStore struct {
+	run.RunStore
+	mu       sync.Mutex
+	appends  int
+	failAt   int
+	failType string
+	failOp   string
+	events   int
+	failEvAt int // fail the Nth EventsWithLines call
+	err      error
+}
+
+func (c *countingStore) Append(id string, st run.FoldState, ev run.Event) (run.FoldState, error) {
+	c.mu.Lock()
+	c.appends++
+	n := c.appends
+	c.mu.Unlock()
+	if (c.failAt != 0 && n == c.failAt) || (c.failType != "" && ev.Type == c.failType) {
+		return run.FoldState{}, c.err
+	}
+	return c.RunStore.Append(id, st, ev)
+}
+func (c *countingStore) Lock(id string) (func(), error) {
+	if c.failOp == "Lock" {
+		return nil, c.err
+	}
+	return c.RunStore.Lock(id)
+}
+func (c *countingStore) EventsWithLines(id string) (run.Log, [][]byte, error) {
+	c.mu.Lock()
+	c.events++
+	n := c.events
+	c.mu.Unlock()
+	if c.failOp == "Events" || (c.failEvAt != 0 && n == c.failEvAt) {
+		return run.Log{}, nil, c.err
+	}
+	return c.RunStore.EventsWithLines(id)
+}
+func (c *countingStore) RepairTail(id string) error {
+	if c.failOp == "Repair" {
+		return c.err
+	}
+	return c.RunStore.RepairTail(id)
+}
+
+// ---- runner ----
+
+type fakeRunner struct {
+	calls    []string
+	stdins   [][]byte
+	res      converge.CmdResult
+	err      error
+	audit    func(run.Event) error
+	ordinal  func(string) int
+	ordinals []int
+}
+
+func (f *fakeRunner) Run(_ context.Context, name string, stdin []byte) (converge.CmdResult, error) {
+	f.calls = append(f.calls, name)
+	f.stdins = append(f.stdins, stdin)
+	if f.ordinal != nil {
+		f.ordinals = append(f.ordinals, f.ordinal(name))
+	}
+	if f.audit != nil {
+		d := run.CmdCallData{Name: name, Argv: []string{"/bin/true"}, InputHash: "x", ExitCode: f.res.ExitCode}
+		if err := f.audit(run.Event{Type: run.TypeCmdCall, Data: run.MarshalCanonical(d)}); err != nil {
+			return converge.CmdResult{}, err
+		}
+	}
+	return f.res, f.err
+}
+
+func (f *fakeRunner) Call(ctx context.Context, name string, stdin []byte, out any) error {
+	res, err := f.Run(ctx, name, stdin)
+	if err != nil {
+		return err
+	}
+	return json.Unmarshal(res.Stdout, out)
+}
+
+// ---- harness ----
+
+type harness struct {
+	t        *testing.T
+	store    *countingStore
+	sidecar  *MemSidecar
+	reg      *fakeRegistry
+	git      *gitFakes
+	runner   *fakeRunner
+	clock    int64
+	files    map[string][]byte
+	nonces   int
+	mockHash map[string]string
+	terminal []View
+	termErr  error
+	deps     Deps
+}
+
+func newHarness(t *testing.T) *harness {
+	t.Helper()
+	h := &harness{t: t, store: &countingStore{RunStore: run.NewMemStore(run.Options{})}, sidecar: &MemSidecar{}, reg: newRegistry(), files: map[string][]byte{}, mockHash: map[string]string{}}
+	h.git = &gitFakes{byDir: map[string]gate.Git{}, def: &gate.Fake{HeadSHA: shaHead, Refs: map[string]string{"main": shaBase}, Common: "/repo/.git", Clean: true, Tree: "t1", Diffs: map[string]string{shaBase + ".." + shaHead: "DIFF", shaHead + ".." + shaHead: "DIFF"}}}
+	h.runner = &fakeRunner{res: converge.CmdResult{Stdout: []byte(`{"stop": false, "reason": ""}`)}}
+	h.deps = Deps{
+		Store: h.store, Sidecar: h.sidecar, Kinds: h.reg,
+		Git: func(dir string) gate.Git { return h.git.get(dir) },
+		Runner: func(d RunnerDeps) converge.Caller {
+			h.runner.audit = d.Audit
+			h.runner.ordinal = d.CmdCalls
+			return h.runner
+		},
+		Clock: func() run.Time {
+			h.clock++
+			return run.Time{Time: time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC).Add(time.Duration(h.clock) * time.Second)}
+		},
+		LookPath: func(n string) (string, error) {
+			if n == "bash" {
+				return "/bin/bash", nil
+			}
+			return "", errors.New("not found")
+		},
+		FileHash: func(p string) (string, error) {
+			if p == "/bin/bash" {
+				return "hb", nil
+			}
+			return "", errors.New("no such file")
+		},
+		Workflows: workflows.Read,
+		ReadFile: func(p string) ([]byte, error) {
+			b, ok := h.files[p]
+			if !ok {
+				return nil, errors.New("open " + p + ": no such file")
+			}
+			return b, nil
+		},
+		Nonce: func() string { h.nonces++; return fmt.Sprintf("n%d", h.nonces) },
+		MockLoad: func(dir string) (string, error) {
+			if hsh, ok := h.mockHash[dir]; ok {
+				return hsh, nil
+			}
+			return "", errors.New("no scenario at " + dir)
+		},
+		Terminal: func(_ context.Context, v View) error { h.terminal = append(h.terminal, v); return h.termErr },
+	}
+	return h
+}
+
+func (h *harness) init(o InitOptions) (*Machine, error) {
+	if o.WorkDir == "" {
+		o.WorkDir = "/repo"
+	}
+	if o.RepoRoot == "" {
+		o.RepoRoot = "/repo"
+	}
+	return Init(context.Background(), h.deps, o)
+}
+
+func (h *harness) mustInit(o InitOptions) *Machine {
+	h.t.Helper()
+	m, err := h.init(o)
+	if err != nil {
+		h.t.Fatalf("init: %v", err)
+	}
+	return m
+}
+
+func (h *harness) events(m *Machine) []run.Event {
+	log, err := h.store.Events(m.runID)
+	if err != nil {
+		h.t.Fatal(err)
+	}
+	return log.Events
+}
+
+func (h *harness) types(m *Machine) []string {
+	var out []string
+	for _, ev := range h.events(m) {
+		out = append(out, ev.Type)
+	}
+	return out
+}
+
+func (h *harness) record(m *Machine, node string, data string) RecordResult {
+	h.t.Helper()
+	r, err := m.Record(context.Background(), RecordOptions{Kind: RecordNodeOutput, Node: node, Data: json.RawMessage(data)})
+	if err != nil {
+		h.t.Fatalf("record %s: %v", node, err)
+	}
+	return r
+}
+
+func (h *harness) advance(m *Machine) AdvanceResult {
+	h.t.Helper()
+	r, err := m.Advance(context.Background())
+	if err != nil {
+		h.t.Fatalf("advance: %v", err)
+	}
+	return r
+}
+
+func wantCode(t *testing.T, err error, code string) *errs.Error {
+	t.Helper()
+	if !errs.Is(err, code) {
+		t.Fatalf("want %s, got %v", code, err)
+	}
+	return errs.As(err)
+}
+
+func findings(n int) string {
+	var fs []run.Finding
+	for i := 0; i < n; i++ {
+		fs = append(fs, run.Finding{IssueText: fmt.Sprintf("bug %d", i), File: "f.go", Line: i + 1})
+	}
+	return string(run.MarshalCanonical(run.Delta{Findings: fs}))
+}
+
+var sdlcVars = map[string]string{"JUDGE": "gpt-5.2", "JUDGE_EFFORT": "medium"}
diff --git a/internal/fsm/machine/helpers_test.go b/internal/fsm/machine/helpers_test.go
new file mode 100644
index 0000000..0dc455c
--- /dev/null
+++ b/internal/fsm/machine/helpers_test.go
@@ -0,0 +1,87 @@
+package machine
+
+import (
+	"io"
+	"os"
+	"testing"
+	"time"
+)
+
+type timeDuration = time.Duration
+
+func appendBytes(t *testing.T, path, s string) {
+	t.Helper()
+	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if _, err := f.WriteString(s); err != nil {
+		t.Fatal(err)
+	}
+	_ = f.Close()
+}
+
+func writeBytes(t *testing.T, path, s string) {
+	t.Helper()
+	if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
+		t.Fatal(err)
+	}
+}
+
+func symlink(t *testing.T, target, link string) {
+	t.Helper()
+	if err := os.Symlink(target, link); err != nil {
+		t.Fatal(err)
+	}
+}
+
+func fileMode(t *testing.T, path string) os.FileMode {
+	t.Helper()
+	st, err := os.Stat(path)
+	if err != nil {
+		t.Fatal(err)
+	}
+	return st.Mode().Perm()
+}
+
+func chmod(t *testing.T, path string, mode os.FileMode) {
+	t.Helper()
+	if err := os.Chmod(path, mode); err != nil {
+		t.Fatal(err)
+	}
+}
+
+func fileOwnerCanChmod() bool { return os.Getuid() != 0 }
+
+func mkdir(t *testing.T, dir string) {
+	t.Helper()
+	if err := os.MkdirAll(dir, 0o700); err != nil {
+		t.Fatal(err)
+	}
+}
+
+func joinLines(lines [][]byte) []byte {
+	var out []byte
+	for _, l := range lines {
+		out = append(out, l...)
+		out = append(out, '\n')
+	}
+	return out
+}
+
+// badFile fails the chosen operation.
+type badFile struct{ rerr, werr, cerr error }
+
+func (b badFile) Read(p []byte) (int, error) {
+	if b.rerr != nil {
+		return 0, b.rerr
+	}
+	return 0, io.EOF
+}
+func (b badFile) Write(p []byte) (int, error) {
+	if b.werr != nil {
+		return 0, b.werr
+	}
+	return len(p), nil
+}
+func (b badFile) Close() error { return b.cerr }
diff --git a/internal/fsm/machine/machine_test.go b/internal/fsm/machine/machine_test.go
new file mode 100644
index 0000000..c371ffa
--- /dev/null
+++ b/internal/fsm/machine/machine_test.go
@@ -0,0 +1,1838 @@
+package machine
+
+import (
+	"context"
+	"encoding/json"
+	"errors"
+	"io"
+	"os"
+	"strings"
+	"testing"
+
+	"github.com/dsifry/metareview/internal/fsm/converge"
+	"github.com/dsifry/metareview/internal/fsm/errs"
+	"github.com/dsifry/metareview/internal/fsm/gate"
+	"github.com/dsifry/metareview/internal/fsm/run"
+	"github.com/dsifry/metareview/internal/fsm/workflow"
+	"github.com/dsifry/metareview/workflows"
+)
+
+// ---------------------------------------------------------------- M1 Init
+
+func TestM1Init(t *testing.T) {
+	h := newHarness(t)
+	m := h.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
+	evs := h.events(m)
+	if got := strings.Join(h.types(m), ","); got != "init,tree" {
+		t.Fatalf("sequence %s", got)
+	}
+	var init run.InitData
+	if err := json.Unmarshal(evs[0].Data, &init); err != nil {
+		t.Fatal(err)
+	}
+	raw, _ := workflows.Read("sdlc-loop")
+	w, _ := workflow.Parse(raw, workflow.Options{Kinds: h.reg.Info()})
+	if init.Workflow != "sdlc-loop" || init.WorkflowHash != w.Hash || init.BaseSHA != shaBase || init.Head != shaHead || init.InitialState != "discover" || init.InitialKind != "review-lenses" || init.RepoMode != "advisory" || init.RepoRoot != "/repo" || init.WorkDir != "/repo" || init.Vars["JUDGE"] != "gpt-5.2" || init.Vars["REVIEWER"] != "claude-opus-5" || init.Mock != "" || init.CmdsSHA256 != "" || len(init.AllowedCmds) != 0 || len(init.Goldens) != 0 || init.Calibration {
+		t.Fatalf("init data: %+v", init)
+	}
+	if evs[0].State != "" || evs[0].Iter != 0 || evs[0].Mock {
+		t.Fatal("init carries no stamps")
+	}
+	var tree run.TreeData
+	_ = json.Unmarshal(evs[1].Data, &tree)
+	if evs[1].State != "discover" || tree.Head != shaHead || tree.TreeHash != gate.TreeHash(shaHead, "t1") {
+		t.Fatalf("tree: %+v %+v", evs[1], tree)
+	}
+	side, err := h.sidecar.Read(m.runID, SidecarWorkflow)
+	if err != nil || string(side) != string(raw) {
+		t.Fatal("sidecar holds the raw workflow bytes")
+	}
+	if v := m.View(); v.NextAction != NextAdvance || v.Node == nil || v.Node.Name != "discover" || v.Node.Exec != "subagent" || v.Workflow != "sdlc-loop" || v.RunID != m.runID {
+		t.Fatalf("view: %+v", v)
+	}
+	// explicit run id, path workflow, warnings, calibration, goldens
+	h.files["/x/my.yaml"] = raw
+	h.files["/x/g.json"] = []byte(`[{"comment":"golden one"},{"comment":"golden two","severity":"high"}]`)
+	m2 := h.mustInit(InitOptions{Workflow: "/x/my.yaml", RunID: "mrv-explicit-run-1", Calibration: true, GoldensPath: "/x/g.json"})
+	if m2.runID != "mrv-explicit-run-1" || m2.View().Snapshot.Goldens[1].Severity != "high" || !m2.View().Snapshot.Calibration || m2.View().Snapshot.Vars["JUDGE"] != "gpt-5.2" {
+		t.Fatalf("m2: %+v", m2.View().Snapshot)
+	}
+	// ERR_RUN_EXISTS leaves the existing sidecar intact
+	if _, err := h.init(InitOptions{Workflow: "/x/my.yaml", RunID: "mrv-explicit-run-1", Calibration: true}); !isStoreCode(err, run.CodeRunExists) {
+		t.Fatalf("exists: %v", err)
+	}
+	if side, _ := h.sidecar.Read("mrv-explicit-run-1", SidecarWorkflow); string(side) != string(raw) {
+		t.Fatal("victim sidecar changed")
+	}
+	// warnings → warn events
+	noClean := strings.Replace(strings.Replace(string(raw), "  - {from: discover,   to: done,       gate: nothing_found,      outcome: clean}   # iteration 0 only: refuses once bugs are known\n", "", 1), "  - {from: adjudicate, to: done,       gate: nothing_confirmed,  outcome: clean}\n", "", 1)
+	h.files["/x/noclean.yaml"] = []byte(noClean)
+	m3 := h.mustInit(InitOptions{Workflow: "/x/noclean.yaml", Vars: sdlcVars})
+	if got := strings.Join(h.types(m3), ","); got != "init,tree,warn" {
+		t.Fatalf("warn sequence %s", got)
+	}
+	if !strings.HasPrefix(m3.View().Snapshot.Warnings[0], "WORKFLOW_WARNING") && !strings.Contains(m3.View().Snapshot.Warnings[0], "loop_without_clean_exit") {
+		t.Fatalf("warnings: %v", m3.View().Snapshot.Warnings)
+	}
+}
+
+func TestM1InitErrors(t *testing.T) {
+	h := newHarness(t)
+	raw, _ := workflows.Read("sdlc-loop")
+	cases := []struct {
+		name string
+		o    InitOptions
+		prep func()
+		code string
+	}{
+		{"not-found-name", InitOptions{Workflow: "nope", Vars: sdlcVars}, nil, CodeWorkflowNotFound},
+		{"not-found-path", InitOptions{Workflow: "/x/missing.yaml", Vars: sdlcVars}, nil, CodeWorkflowNotFound},
+		{"too-large", InitOptions{Workflow: "/x/big.yaml"}, func() { h.files["/x/big.yaml"] = make([]byte, MaxWorkflowBytes+1) }, CodeWorkflowTooLarge},
+		{"invalid", InitOptions{Workflow: "/x/bad.yaml"}, func() { h.files["/x/bad.yaml"] = []byte("workflow: x\nversion: 2\n") }, workflow.CodeWorkflowInvalid},
+		{"bad-repo-mode", InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, RepoMode: "advisory"}, nil, CodeBadRepoMode},
+		{"var-unset", InitOptions{Workflow: "sdlc-loop"}, nil, workflow.CodeVarUnset},
+		{"var-unknown", InitOptions{Workflow: "sdlc-loop", Vars: map[string]string{"JUDGE": "a", "JUDGE_EFFORT": "b", "FOO": "x"}}, nil, workflow.CodeVarUnknown},
+		{"goldens-missing", InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, GoldensPath: "/x/none.json"}, nil, CodeGoldensInvalid},
+		{"goldens-big", InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, GoldensPath: "/x/gbig.json"}, func() { h.files["/x/gbig.json"] = make([]byte, MaxGoldensBytes+1) }, CodeGoldensInvalid},
+		{"goldens-unknown-field", InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, GoldensPath: "/x/g1.json"}, func() { h.files["/x/g1.json"] = []byte(`[{"comment":"c","zzz":1}]`) }, CodeGoldensInvalid},
+		{"goldens-count", InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, GoldensPath: "/x/g2.json"}, func() {
+			var gs []run.Golden
+			for i := 0; i <= run.MaxGoldens; i++ {
+				gs = append(gs, run.Golden{Comment: strings.Repeat("g", i+1)})
+			}
+			h.files["/x/g2.json"] = run.MarshalCanonical(gs)
+		}, CodeGoldensInvalid},
+		{"goldens-empty-comment", InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, GoldensPath: "/x/g3.json"}, func() { h.files["/x/g3.json"] = []byte(`[{"comment":""}]`) }, CodeGoldensInvalid},
+		{"goldens-over-desc", InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, GoldensPath: "/x/g4.json"}, func() {
+			h.files["/x/g4.json"] = run.MarshalCanonical([]run.Golden{{Comment: strings.Repeat("x", run.MaxDesc+1)}})
+		}, CodeGoldensInvalid},
+		{"goldens-dup", InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, GoldensPath: "/x/g5.json"}, func() { h.files["/x/g5.json"] = []byte(`[{"comment":"c"},{"comment":"c"}]`) }, CodeGoldensInvalid},
+		{"mock-invalid", InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, MockDir: "scen/x"}, nil, CodeMockInvalid},
+		{"goldens-null-ok", InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, GoldensPath: "/x/gnull.json"}, func() { h.files["/x/gnull.json"] = []byte(`null`) }, ""},
+		{"mock-registry-mismatch", InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, MockDir: "scen/ok"}, func() { h.mockHash["/repo/scen/ok"] = strings.Repeat("a", 64) }, CodeMockMismatch},
+		{"base-unknown", InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "nope"}, nil, gate.CodeGit},
+		{"workdir-relative", InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, WorkDir: "rel"}, nil, CodeWorkdirForeign},
+		{"mock-outside-root", InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, MockDir: "/other/scen"}, func() { h.mockHash["/other/scen"] = strings.Repeat("b", 64) }, CodeMockInvalid},
+		{"mock-short-hash", InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, MockDir: "scen/short"}, func() { h.mockHash["/repo/scen/short"] = "abc" }, CodeMockInvalid},
+		{"workdir-foreign", InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, WorkDir: "/elsewhere"}, func() {
+			h.git.byDir["/elsewhere"] = &gate.Fake{HeadSHA: shaHead, Common: "/other/.git"}
+		}, CodeWorkdirForeign},
+	}
+	_ = raw
+	for _, c := range cases {
+		if c.prep != nil {
+			c.prep()
+		}
+		m, err := h.init(c.o)
+		if c.code == "" {
+			if err != nil || m.View().Snapshot.Goldens == nil {
+				t.Errorf("%s: %v", c.name, err)
+			}
+			continue
+		}
+		if !errs.Is(err, c.code) {
+			t.Errorf("%s: want %s got %v", c.name, c.code, err)
+		}
+	}
+	// registry mismatch the other way: mock registry without --mock-ai
+	h.reg.mock = true
+	if _, err := h.init(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars}); !errs.Is(err, CodeMockMismatch) {
+		t.Fatalf("mock registry: %v", err)
+	}
+	// mock ok → Mock = rel#hash[:16]
+	m := h.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, MockDir: "/repo/scen/ok"})
+	if m.View().Snapshot.Mock != "scen/ok#"+strings.Repeat("a", 16) {
+		t.Fatalf("mock id %q", m.View().Snapshot.Mock)
+	}
+	h.mockHash["/repo"] = strings.Repeat("c", 64)
+	if mr := h.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, MockDir: "."}); mr.View().Snapshot.Mock != ".#"+strings.Repeat("c", 16) {
+		t.Fatalf("mock at repo root: %q", mr.View().Snapshot.Mock)
+	}
+	if _, err := Open(context.Background(), h.deps, m.runID, OpenOptions{}); err != nil {
+		t.Fatal(err)
+	}
+	h.mockHash["/repo/scen/ok"] = "short"
+	if _, err := Open(context.Background(), h.deps, m.runID, OpenOptions{}); !errs.Is(err, CodeMockMismatch) {
+		t.Fatalf("short hash on open: %v", err)
+	}
+	h.mockHash["/repo/scen/ok"] = strings.Repeat("a", 64)
+	if !h.events(m)[1].Mock {
+		t.Fatal("mock runs stamp Mock on every later event")
+	}
+	h.reg.mock = false
+	// git failures at each Init call site are returned unchanged (ERR_GIT{op})
+	boom := errs.E(gate.CodeGit, "boom", "op", "x")
+	for _, at := range []string{"CommonDir", "Head", "RevParse", "Status", "WorkTree"} {
+		h.git.byDir["/repo"] = &failingGit{Git: h.git.def, at: at, err: boom}
+		if _, err := h.init(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars}); !errors.Is(err, boom) {
+			t.Errorf("git %s: %v", at, err)
+		}
+	}
+	delete(h.git.byDir, "/repo")
+	// store/sidecar failures
+	h.store.failOp = "Lock"
+	h.store.err = errors.New("locked")
+	if _, err := h.init(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars}); err == nil || err.Error() != "locked" {
+		t.Fatalf("lock: %v", err)
+	}
+	h.store.failOp = ""
+	h.store.appends, h.store.failAt, h.store.err = 0, 1, errors.New("append1")
+	if _, err := h.init(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars}); err == nil || err.Error() != "append1" {
+		t.Fatalf("append: %v", err)
+	}
+	h.store.failAt = 0
+	h.store.appends = 0
+	h.files["/x/w.yaml"] = raw
+	h.sidecar.Put("mrv-sidecar-exists", SidecarWorkflow, []byte("x"))
+	if _, err := h.init(InitOptions{Workflow: "/x/w.yaml", RunID: "mrv-sidecar-exists", Vars: sdlcVars}); !errs.Is(err, CodeSidecar) {
+		t.Fatalf("sidecar exists: %v", err)
+	}
+	// the run exists without a usable sidecar → Open reports ERR_WORKFLOW_CHANGED (bytes differ)
+	if _, err := Open(context.Background(), h.deps, "mrv-sidecar-exists", OpenOptions{}); !errs.Is(err, CodeWorkflowChanged) {
+		t.Fatalf("open after sidecar failure: %v", err)
+	}
+	// warn append failure on a workflow with warnings
+	noClean := strings.Replace(strings.Replace(string(raw), "  - {from: discover,   to: done,       gate: nothing_found,      outcome: clean}   # iteration 0 only: refuses once bugs are known\n", "", 1), "  - {from: adjudicate, to: done,       gate: nothing_confirmed,  outcome: clean}\n", "", 1)
+	h.files["/x/noclean.yaml"] = []byte(noClean)
+	h.store.appends, h.store.failAt, h.store.err = 0, 2, errors.New("append2")
+	if _, err := h.init(InitOptions{Workflow: "/x/noclean.yaml", Vars: sdlcVars}); err == nil || err.Error() != "append2" {
+		t.Fatalf("warn append: %v", err)
+	}
+	h.store.failAt = 0
+}
+
+func TestM1Consent(t *testing.T) {
+	h := newHarness(t)
+	raw, _ := workflows.Read("sdlc-loop")
+	withCmd := strings.Replace(string(raw), "repo_mode: advisory", "cmds:\n  notify: {argv: [bash, ./notify.sh, --tag, $JUDGE], timeout: 2, env: [SLACK_WEBHOOK]}\non_overflow: notify\nrepo_mode: advisory", 1)
+	h.files["/x/cmd.yaml"] = []byte(withCmd)
+	_, err := h.init(InitOptions{Workflow: "/x/cmd.yaml", Vars: sdlcVars})
+	e := wantCode(t, err, CodeCmdsNotAllowed)
+	sha := e.Field("sha")
+	if len(sha) != 64 || !strings.Contains(e.Detail, `notify: argv=["/bin/bash" "./notify.sh" "--tag" "gpt-5.2"] timeout=2000ms env=[SLACK_WEBHOOK]`) || !strings.Contains(e.Detail, "pinned: /bin/bash=hb") || !strings.Contains(e.Detail, "unpinned: ./notify.sh, --tag, gpt-5.2") {
+		t.Fatalf("consent detail:\n%s", e.Detail)
+	}
+	if _, err := h.init(InitOptions{Workflow: "/x/cmd.yaml", Vars: sdlcVars, AllowCustomCmds: "wrong"}); !errs.Is(err, CodeCmdsNotAllowed) {
+		t.Fatal("wrong sha")
+	}
+	m := h.mustInit(InitOptions{Workflow: "/x/cmd.yaml", Vars: sdlcVars, AllowCustomCmds: sha})
+	if snap := m.View().Snapshot; snap.CmdsSHA256 != sha || len(snap.AllowedCmds) != 1 || snap.AllowedCmds[0].TimeoutMS != 2000 {
+		t.Fatalf("allowed: %+v", snap.AllowedCmds)
+	}
+	// cmd not found
+	h.files["/x/cmd2.yaml"] = []byte(strings.Replace(withCmd, "argv: [bash,", "argv: [nope,", 1))
+	if _, err := h.init(InitOptions{Workflow: "/x/cmd2.yaml", Vars: sdlcVars}); !errs.Is(err, workflow.CodeCmdNotFound) {
+		t.Fatalf("cmd not found: %v", err)
+	}
+}
+
+// ---------------------------------------------------------------- M2 happy paths
+
+func TestM2ReviewLoop(t *testing.T) {
+	h := newHarness(t)
+	m := h.mustInit(InitOptions{Workflow: "review-loop", Vars: sdlcVars})
+	// discover is host-executed: NEEDS_INPUT, once
+	r := h.advance(m)
+	if r.Status != StatusNeedsInput || r.ExitCode != 3 || r.NeedsInput == nil || r.NeedsInput.Node != "discover" || r.NeedsInput.Exec != "subagent" || r.NeedsInput.Model != "claude-opus-5" || r.NeedsInput.Instructions.Input["diff"] != "DIFF" || r.NeedsInput.Instructions.Text != "do discover nonce=n1" || !strings.Contains(r.NeedsInput.Record, "--node discover") {
+		t.Fatalf("needs input: %+v", r)
+	}
+	if v := m.View(); v.NextAction != NextRecord || v.Node.HasOutput {
+		t.Fatalf("view after needs_input: %+v", v)
+	}
+	if r.NeedsInput.Instructions.Input["diff_truncated"] != false {
+		t.Fatal("not truncated")
+	}
+	MaxDiffBytes = 2
+	if r := h.advance(m); r.NeedsInput.Instructions.Input["diff"] != "DI" || r.NeedsInput.Instructions.Input["diff_truncated"] != true {
+		t.Fatalf("truncated diff: %v", r.NeedsInput.Instructions.Input)
+	}
+	MaxDiffBytes = 1 << 20
+	// a second advance and a tokens record do not re-append needs_input
+	h.advance(m)
+	if _, err := m.Record(context.Background(), RecordOptions{Kind: RecordTokens, Data: json.RawMessage(`{"input":7,"output":3}`)}); err != nil {
+		t.Fatal(err)
+	}
+	h.advance(m)
+	if got := strings.Join(h.types(m), ","); got != "init,tree,needs_input,tokens" {
+		t.Fatalf("needs_input once: %s", got)
+	}
+	// clean path: no findings
+	h.record(m, "discover", `{"findings":[]}`)
+	if v := m.View(); v.NextAction != NextAdvance || !v.Node.HasOutput || v.Node.Applied {
+		t.Fatalf("view after record: %+v", v)
+	}
+	r = h.advance(m)
+	if r.Status != StatusDone || r.Outcome != run.OutcomeClean || r.ExitCode != 0 || r.To != "done" {
+		t.Fatalf("clean: %+v", r)
+	}
+	if got := strings.Join(h.types(m), ","); got != "init,tree,needs_input,tokens,node_output,delta_applied,gate,transition" {
+		t.Fatalf("clean sequence: %s", got)
+	}
+	if len(h.terminal) != 1 || h.terminal[0].Snapshot.Outcome != run.OutcomeClean || m.View().NextAction != NextNone {
+		t.Fatalf("terminal hook: %d", len(h.terminal))
+	}
+	// terminal: Advance → ERR_RUN_TERMINAL, Terminal called again (idempotency is the hook's)
+	if _, err := m.Advance(context.Background()); !errs.Is(err, CodeRunTerminal) {
+		t.Fatalf("terminal advance: %v", err)
+	}
+	if len(h.terminal) != 2 {
+		t.Fatal("Terminal on every terminal advance")
+	}
+	// reviewed path
+	m2 := h.mustInit(InitOptions{Workflow: "review-loop", Vars: sdlcVars})
+	h.advance(m2)
+	h.record(m2, "discover", findings(2))
+	r = h.advance(m2) // discover → adjudicate (fork executes on the next advance? no: the node runs when entering the state)
+	if r.Status != StatusAdvanced || r.To != "adjudicate" || r.ExitCode != 0 {
+		t.Fatalf("advanced: %+v", r)
+	}
+	r = h.advance(m2)
+	if r.Status != StatusDone || r.Outcome != run.OutcomeReviewed || r.ExitCode != 1 {
+		t.Fatalf("reviewed: %+v", r)
+	}
+	want := "init,tree,needs_input,node_output,delta_applied,gate,gate,transition,llm_call,llm_call,node_output,delta_applied,gate,gate,transition"
+	if got := strings.Join(h.types(m2), ","); got != want {
+		t.Fatalf("reviewed sequence:\n%s\n%s", got, want)
+	}
+	ex := h.reg.execs["match-then-adjudicate"]
+	if len(ex.calls) != 1 || ex.calls[0].StartIndex != 0 || ex.calls[0].Diff.Text != "DIFF" || ex.calls[0].Node.Name != "adjudicate" || ex.calls[0].Node.Model != "gpt-5.2" || ex.calls[0].Runner == nil {
+		t.Fatalf("exec input: %+v", ex.calls)
+	}
+	evs := h.events(m2)
+	for _, ev := range evs {
+		if ev.Type == run.TypeLLMCall && (ev.Node != "adjudicate" || ev.State != "adjudicate") {
+			t.Fatalf("llm_call stamps: %+v", ev)
+		}
+		if ev.Mock {
+			t.Fatal("non-mock run must not stamp Mock")
+		}
+	}
+	if m2.View().Snapshot.MockTainted || m2.View().Snapshot.Tokens.Input != 20 {
+		t.Fatalf("tokens/taint: %+v", m2.View().Snapshot.Tokens)
+	}
+}
+
+func TestM2SdlcFixedAndLoop(t *testing.T) {
+	h := newHarness(t)
+	m := h.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars})
+	// iteration-0 clean exit at discover
+	h.advance(m)
+	h.record(m, "discover", `{"findings":[]}`)
+	if r := h.advance(m); r.Status != StatusDone || r.Outcome != run.OutcomeClean {
+		t.Fatalf("clean at discover: %+v", r)
+	}
+	// full fixed path
+	h.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+	m = h.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars})
+	h.advance(m)
+	h.record(m, "discover", findings(2))
+	if r := h.advance(m); r.To != "adjudicate" {
+		t.Fatalf("→adjudicate: %+v", r)
+	}
+	if r := h.advance(m); r.To != "fix" {
+		t.Fatalf("→fix: %+v", r)
+	}
+	if snap := m.View().Snapshot; snap.FixEntryHead != shaHead || len(snap.AllFound) != 2 {
+		t.Fatalf("fix entry: %+v", snap)
+	}
+	r := h.advance(m)
+	if r.Status != StatusNeedsInput || r.NeedsInput.Kind != "agent-edit" || r.NeedsInput.Exec != "inline" {
+		t.Fatalf("fix needs input: %+v", r)
+	}
+	h.record(m, "fix", `{"commit":"`+shaFix+`","summary":"fixed"}`)
+	if r := h.advance(m); r.To != "verify" {
+		t.Fatalf("→verify: %+v", r)
+	}
+	r = h.advance(m)
+	if r.Status != StatusDone || r.Outcome != run.OutcomeFixed || r.ExitCode != 0 {
+		t.Fatalf("fixed: %+v", r)
+	}
+	// no converge event on the gate-first path
+	for _, ty := range h.types(m) {
+		if ty == run.TypeConverge {
+			t.Fatal("gate-first: no converge event when all_fixed passes")
+		}
+	}
+	// loop once: verify says bug 0 still present at iteration 0, fixed at iteration 1
+	iter := 0
+	h.reg.execs["still-present"].fn = func(in ExecInput) (json.RawMessage, error) {
+		var st []run.BugStatus
+		for i, b := range in.Snap.AllFound {
+			if err := llmCall(in, i, 5); err != nil {
+				return nil, err
+			}
+			st = append(st, run.BugStatus{ID: b.ID, StillPresent: iter == 0 && i == 0, Confidence: 1})
+		}
+		return json.RawMessage(run.MarshalCanonical(run.Delta{Status: st})), nil
+	}
+	m = h.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars})
+	h.advance(m)
+	h.record(m, "discover", findings(2))
+	h.advance(m)
+	h.advance(m)
+	h.advance(m)
+	h.record(m, "fix", `{"commit":"`+shaFix+`","summary":"fixed"}`)
+	h.advance(m)
+	r = h.advance(m)
+	if r.Status != StatusAdvanced || r.To != "discover" || m.View().Snapshot.Iteration != 1 || m.View().Snapshot.Unfixed != 1 {
+		t.Fatalf("loop: %+v %+v", r, m.View().Snapshot)
+	}
+	evs := h.events(m)
+	last := evs[len(evs)-1]
+	if last.Type != run.TypeTransition || last.Iter != 1 || last.State != "verify" {
+		t.Fatalf("loop transition stamps: %+v", last)
+	}
+	// converge event present with stop=false
+	var sawConverge bool
+	for _, ev := range evs {
+		if ev.Type == run.TypeConverge {
+			sawConverge = true
+			var cd run.ConvergeData
+			_ = json.Unmarshal(ev.Data, &cd)
+			if cd.Stop || cd.Atom == "" {
+				t.Fatalf("converge: %+v", cd)
+			}
+		}
+	}
+	if !sawConverge {
+		t.Fatal("converge evaluated at the loop boundary")
+	}
+	iter = 1
+	// iteration 1: discover misses everything → nothing_found refuses (bugs known) → findings_nonempty fails → GATE_FAILED
+	h.advance(m) // needs input at discover@1
+	if got := countType(evs, run.TypeNeedsInput); got != 2 {
+		t.Fatalf("needs_input count before: %d", got)
+	}
+	h.record(m, "discover", `{"findings":[]}`)
+	r = h.advance(m)
+	if r.Status != StatusGateFailed || r.Gate == nil || r.Gate.Name != "nothing_found" || r.Gate.Error.Code != gate.CodeBugsKnown || r.ExitCode != 1 {
+		t.Fatalf("iteration ≥1 miss must be visible: %+v", r)
+	}
+	if v := m.View(); v.FailedGate == nil || v.FailedGate.Name != "findings_nonempty" {
+		t.Fatalf("FailedGate is the last failing gate: %+v", v.FailedGate)
+	}
+	if len(h.terminal) == 0 || h.terminal[len(h.terminal)-1].Snapshot.Outcome != run.OutcomeFailed {
+		t.Fatal("Terminal called for failed")
+	}
+}
+
+func countType(evs []run.Event, ty string) int {
+	n := 0
+	for _, ev := range evs {
+		if ev.Type == ty {
+			n++
+		}
+	}
+	return n
+}
+
+// ---------------------------------------------------------------- M3 failures
+
+func TestM3GateFailures(t *testing.T) {
+	h := newHarness(t)
+	// two failing gates at discover (review-loop with findings present? no: both fail when... use sdlc discover with findings but nothing_found refuses and findings_nonempty passes).
+	// Build a custom workflow whose discover has two gates that both fail on empty findings: findings_nonempty, confirmed_nonempty.
+	raw, _ := workflows.Read("review-loop")
+	custom := strings.Replace(string(raw), "  - {from: discover,   to: done,       gate: findings_empty,     outcome: clean}\n", "  - {from: discover,   to: done,       gate: confirmed_nonempty, outcome: clean}\n", 1)
+	h.files["/x/two.yaml"] = []byte(custom)
+	m := h.mustInit(InitOptions{Workflow: "/x/two.yaml", Vars: sdlcVars})
+	h.advance(m)
+	h.record(m, "discover", `{"findings":[]}`)
+	r := h.advance(m)
+	if r.Status != StatusGateFailed || r.Gate.Name != "confirmed_nonempty" || r.To != "failed" {
+		t.Fatalf("first failing gate: %+v", r)
+	}
+	evs := h.events(m)
+	var td run.TransitionData
+	_ = json.Unmarshal(evs[len(evs)-1].Data, &td)
+	if td.Gate != "confirmed_nonempty" || td.Outcome != run.OutcomeFailed || td.To != "failed" {
+		t.Fatalf("failed transition: %+v", td)
+	}
+	if r.Untrusted[0] != "gate.detail" {
+		t.Fatalf("untrusted: %v", r.Untrusted)
+	}
+	// executor failure → executor pseudo-gate; earlier llm_calls kept; StartIndex honoured on the next execution (fork-less: a fresh run)
+	h = newHarness(t)
+	boom := errors.New("provider down")
+	h.reg.execs["match-then-adjudicate"].fn = func(in ExecInput) (json.RawMessage, error) {
+		_ = llmCall(in, 0, 1)
+		return nil, boom
+	}
+	m = h.mustInit(InitOptions{Workflow: "review-loop", Vars: sdlcVars})
+	h.advance(m)
+	h.record(m, "discover", findings(1))
+	h.advance(m)
+	r = h.advance(m)
+	if r.Status != StatusGateFailed || r.Gate.Name != GateExecutor || r.Gate.Error.Code != CodeExecutorFailed || !strings.Contains(r.Gate.Error.Detail, "provider down") {
+		t.Fatalf("executor: %+v", r)
+	}
+	if got := strings.Join(h.types(m), ","); !strings.HasSuffix(got, "llm_call,gate,transition") {
+		t.Fatalf("executor sequence: %s", got)
+	}
+	// interrupted execution: a pre-seeded llm_call makes StartIndex 1; ctx cancel returns the error with no pseudo-gate
+	h = newHarness(t)
+	var seen []int
+	h.reg.execs["match-then-adjudicate"].fn = func(in ExecInput) (json.RawMessage, error) {
+		seen = append(seen, in.StartIndex)
+		if len(seen) == 1 {
+			_ = llmCall(in, 0, 1)
+			return nil, context.Canceled
+		}
+		_ = llmCall(in, 0, 1)
+		return json.RawMessage(`{"confirmed":[]}`), nil
+	}
+	m = h.mustInit(InitOptions{Workflow: "review-loop", Vars: sdlcVars})
+	h.advance(m)
+	h.record(m, "discover", findings(1))
+	h.advance(m)
+	ctx, cancel := context.WithCancel(context.Background())
+	cancel()
+	if _, err := m.Advance(ctx); !errors.Is(err, context.Canceled) {
+		t.Fatalf("interrupt: %v", err)
+	}
+	if countType(h.events(m), run.TypeGate) != 2 { // discover's two gates only
+		t.Fatal("no pseudo-gate on interrupt")
+	}
+	r = h.advance(m)
+	if seen[1] != 1 || r.Status != StatusDone || r.Outcome != run.OutcomeClean {
+		t.Fatalf("resume: %v %+v", seen, r)
+	}
+	// decode / reduce errors → node_output pseudo-gate
+	h = newHarness(t)
+	h.reg.execs["match-then-adjudicate"].fn = func(in ExecInput) (json.RawMessage, error) { return json.RawMessage(`{"nope":1}`), nil }
+	m = h.mustInit(InitOptions{Workflow: "review-loop", Vars: sdlcVars})
+	h.advance(m)
+	h.record(m, "discover", findings(1))
+	h.advance(m)
+	if r := h.advance(m); r.Status != StatusGateFailed || r.Gate.Name != GateNodeOutput || r.Gate.Error.Code != CodeNodeOutputInvalid {
+		t.Fatalf("decode error: %+v", r)
+	}
+	h = newHarness(t)
+	h.reg.kinds["match-then-adjudicate"].reduce = func(run.Snapshot, any) (run.Delta, error) { return run.Delta{}, errors.New("reduce boom") }
+	m = h.mustInit(InitOptions{Workflow: "review-loop", Vars: sdlcVars})
+	h.advance(m)
+	h.record(m, "discover", findings(1))
+	h.advance(m)
+	if r := h.advance(m); r.Status != StatusGateFailed || r.Gate.Name != GateNodeOutput || !strings.Contains(r.Gate.Error.Detail, "reduce boom") {
+		t.Fatalf("reduce error: %+v", r)
+	}
+	// rejected delta_applied (status subset) → node_output pseudo-gate
+	h = newHarness(t)
+	h.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+	h.reg.execs["still-present"].fn = func(in ExecInput) (json.RawMessage, error) {
+		return json.RawMessage(run.MarshalCanonical(run.Delta{Status: []run.BugStatus{{ID: in.Snap.AllFound[0].ID}}})), nil
+	}
+	m = h.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars})
+	h.advance(m)
+	h.record(m, "discover", findings(2))
+	h.advance(m)
+	h.advance(m)
+	h.advance(m)
+	h.record(m, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
+	h.advance(m)
+	if r := h.advance(m); r.Status != StatusGateFailed || r.Gate.Name != GateNodeOutput || !strings.Contains(r.Gate.Error.Detail, "status_incomplete") {
+		t.Fatalf("rejected delta: %+v", r)
+	}
+	// Instructions failure
+	h = newHarness(t)
+	h.reg.kinds["review-lenses"].instr = func(run.Snapshot, *workflow.Node, Diff, string) (Instructions, error) {
+		return Instructions{}, errors.New("no rubric")
+	}
+	m = h.mustInit(InitOptions{Workflow: "review-loop", Vars: sdlcVars})
+	if _, err := m.Advance(context.Background()); !errs.Is(err, CodeInstructionsFailed) {
+		t.Fatalf("instructions: %v", err)
+	}
+	if got := strings.Join(h.types(m), ","); got != "init,tree" {
+		t.Fatalf("nothing appended on instructions failure: %s", got)
+	}
+	// git failure at each Advance call site returned unchanged
+	h = newHarness(t)
+	m = h.mustInit(InitOptions{Workflow: "review-loop", Vars: sdlcVars})
+	boomGit := errs.E(gate.CodeGit, "git down", "op", "x")
+	for _, at := range []string{"Head", "Status", "WorkTree", "Diff"} {
+		h.git.byDir["/repo"] = &failingGit{Git: h.git.def, at: at, err: boomGit}
+		if _, err := m.Advance(context.Background()); !errors.Is(err, boomGit) {
+			t.Errorf("advance git %s: %v", at, err)
+		}
+	}
+}
+
+// ---------------------------------------------------------------- M4 loop / convergence
+
+// loopRun drives sdlc-loop to the first loop boundary with n findings, all still present.
+func loopRun(t *testing.T, h *harness, n int, wf string) *Machine {
+	t.Helper()
+	h.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+	h.reg.execs["still-present"].fn = func(in ExecInput) (json.RawMessage, error) {
+		var st []run.BugStatus
+		for i, b := range in.Snap.AllFound {
+			if err := llmCall(in, i, 5); err != nil {
+				return nil, err
+			}
+			st = append(st, run.BugStatus{ID: b.ID, StillPresent: true, Confidence: 1})
+		}
+		return json.RawMessage(run.MarshalCanonical(run.Delta{Status: st})), nil
+	}
+	m := h.mustInit(InitOptions{Workflow: wf, Vars: sdlcVars})
+	h.advance(m)
+	h.record(m, "discover", findings(n))
+	h.advance(m)
+	h.advance(m)
+	h.advance(m)
+	h.record(m, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
+	h.advance(m)
+	return m
+}
+
+func sdlcWith(t *testing.T, h *harness, name, old, new string) string {
+	t.Helper()
+	raw, _ := workflows.Read("sdlc-loop")
+	if !strings.Contains(string(raw), old) {
+		t.Fatalf("fixture target missing: %s", old)
+	}
+	h.files["/x/"+name] = []byte(strings.Replace(string(raw), old, new, 1))
+	return "/x/" + name
+}
+
+func TestM4Convergence(t *testing.T) {
+	// cumulative regression: iter 0 finds 8, all present; iter 1 discover finds a new one only, verify marks 7 present → not fixed, loop again
+	h := newHarness(t)
+	present := func(id string) bool { return true }
+	h.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+	h.reg.execs["still-present"].fn = func(in ExecInput) (json.RawMessage, error) {
+		var st []run.BugStatus
+		for i, b := range in.Snap.AllFound {
+			if err := llmCall(in, i, 5); err != nil {
+				return nil, err
+			}
+			st = append(st, run.BugStatus{ID: b.ID, StillPresent: present(b.ID), Confidence: 1})
+		}
+		return json.RawMessage(run.MarshalCanonical(run.Delta{Status: st})), nil
+	}
+	m := h.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars})
+	h.advance(m)
+	h.record(m, "discover", findings(8))
+	h.advance(m)
+	h.advance(m)
+	h.advance(m)
+	h.record(m, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
+	h.advance(m)
+	r := h.advance(m)
+	if r.To != "discover" || m.View().Snapshot.Unfixed != 8 {
+		t.Fatalf("loop 1: %+v", r)
+	}
+	// iteration 1: fix its own new bug and one old one → 7 of 9 remain: progress, not fixed, loop again
+	newBug := run.BugID("bug 8")
+	present = func(id string) bool { return id != newBug && id != run.BugID("bug 0") }
+	h.advance(m)
+	h.record(m, "discover", string(run.MarshalCanonical(run.Delta{Findings: []run.Finding{{IssueText: "bug 8"}}})))
+	h.advance(m)
+	h.advance(m)
+	h.advance(m)
+	h.record(m, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
+	h.advance(m)
+	r = h.advance(m)
+	snap := m.View().Snapshot
+	if r.Outcome == run.OutcomeFixed || len(snap.AllFound) != 9 || snap.Unfixed != 7 || r.To != "discover" || snap.Iteration != 2 {
+		t.Fatalf("cumulative: %+v all=%d unfixed=%d", r, len(snap.AllFound), snap.Unfixed)
+	}
+	// gate-first: max_iterations 1 with all fixed → fixed, zero converge events; one bug left → overflow
+	h = newHarness(t)
+	wf := sdlcWith(t, h, "max1.yaml", "{max_iterations: 5}", "{max_iterations: 1}")
+	h.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+	m = h.mustInit(InitOptions{Workflow: wf, Vars: sdlcVars})
+	h.advance(m)
+	h.record(m, "discover", findings(1))
+	h.advance(m)
+	h.advance(m)
+	h.advance(m)
+	h.record(m, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
+	h.advance(m)
+	if r := h.advance(m); r.Outcome != run.OutcomeFixed || countType(h.events(m), run.TypeConverge) != 0 {
+		t.Fatalf("gate-first: %+v", r)
+	}
+	h = newHarness(t)
+	wf = sdlcWith(t, h, "max1.yaml", "{max_iterations: 5}", "{max_iterations: 1}")
+	m = loopRun(t, h, 1, wf)
+	r = h.advance(m)
+	if r.Status != StatusStopped || r.Outcome != run.OutcomeOverflow || r.ExitCode != 1 || !strings.HasPrefix(r.StopReason, "max_iterations: ") || r.Untrusted[len(r.Untrusted)-1] != "stop_reason" {
+		t.Fatalf("overflow: %+v", r)
+	}
+	if h.terminal[len(h.terminal)-1].Snapshot.StopReason != "max_iterations" {
+		t.Fatal("fold keeps the atom as StopReason")
+	}
+	// stalled: nil-then-plateau (iteration 0 never stalls; iteration 1 with equal unfixed stalls)
+	h = newHarness(t)
+	m = loopRun(t, h, 2, "sdlc-loop")
+	if r := h.advance(m); r.To != "discover" {
+		t.Fatalf("first boundary never stalls: %+v", r)
+	}
+	h.advance(m)
+	h.record(m, "discover", findings(2))
+	h.advance(m)
+	h.advance(m)
+	h.advance(m)
+	h.record(m, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
+	h.advance(m)
+	if r := h.advance(m); r.Outcome != run.OutcomeStalled || !strings.HasPrefix(r.StopReason, "no_fixation_progress") {
+		t.Fatalf("stalled: %+v", r)
+	}
+	// budget via llm_call tokens and via record tokens
+	h = newHarness(t)
+	wf = sdlcWith(t, h, "budget.yaml", "{budget: {tokens: 4000000}}", "{budget: {tokens: 25}}")
+	m = loopRun(t, h, 2, wf) // adjudicate 2×10 + verify 2×5 = 30 ≥ 25
+	if r := h.advance(m); r.Outcome != run.OutcomeOverflow || !strings.HasPrefix(r.StopReason, "budget") {
+		t.Fatalf("budget llm: %+v", r)
+	}
+	h = newHarness(t)
+	wf = sdlcWith(t, h, "budget2.yaml", "{budget: {tokens: 4000000}}", "{budget: {tokens: 1000}}")
+	m = loopRun(t, h, 1, wf)
+	if _, err := m.Record(context.Background(), RecordOptions{Kind: RecordTokens, Data: json.RawMessage(`{"output":1000}`)}); err != nil {
+		t.Fatal(err)
+	}
+	if r := h.advance(m); r.Outcome != run.OutcomeOverflow {
+		t.Fatalf("budget record: %+v", r)
+	}
+	// custom via cmd atom + on_overflow handler NOT run for custom; converge error → converge pseudo-gate
+	h = newHarness(t)
+	wf = sdlcWith(t, h, "custom.yaml", "  any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]\nrepo_mode: advisory",
+		"  any: [no_fixation_progress, {cmd: notify}, {max_iterations: 5}]\ncmds:\n  notify: {argv: [bash, -c, echo]}\non_overflow: notify\nrepo_mode: advisory")
+	h.runner.res = converge.CmdResult{Stdout: []byte(`{"stop": true, "reason": "plateau"}`)}
+	_, sha, _ := workflow.ResolveCmds(mustResolve(t, h, wf), "/repo", h.deps.LookPath, h.deps.FileHash)
+	allPresent(h)
+	h.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+	m = h.mustInit(InitOptions{Workflow: wf, Vars: sdlcVars, AllowCustomCmds: sha})
+	h.advance(m)
+	h.record(m, "discover", findings(1))
+	h.advance(m)
+	h.advance(m)
+	h.advance(m)
+	h.record(m, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
+	h.advance(m)
+	r = h.advance(m)
+	if r.Outcome != run.OutcomeCustom || r.StopReason != "cmd:notify: plateau" || countType(h.events(m), run.TypeOverflowHandler) != 0 || countType(h.events(m), run.TypeCmdCall) != 1 {
+		t.Fatalf("custom: %+v handlers=%d", r, countType(h.events(m), run.TypeOverflowHandler))
+	}
+	if !strings.Contains(string(h.runner.stdins[0]), `"vars":{"JUDGE":"sha256:`) {
+		t.Fatal("cmd atom receives the redacted payload")
+	}
+	if len(h.runner.ordinals) != 1 || h.runner.ordinals[0] != 0 {
+		t.Fatalf("first call ordinal 0: %v", h.runner.ordinals)
+	}
+	// converge error
+	h = newHarness(t)
+	wf = sdlcWith(t, h, "custom2.yaml", "  any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]\nrepo_mode: advisory",
+		"  any: [{cmd: notify}, {max_iterations: 5}]\ncmds:\n  notify: {argv: [bash, -c, echo]}\nrepo_mode: advisory")
+	h.runner.res = converge.CmdResult{Stdout: []byte(`garbage`)}
+	_, sha, _ = workflow.ResolveCmds(mustResolve(t, h, wf), "/repo", h.deps.LookPath, h.deps.FileHash)
+	allPresent(h)
+	h.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+	m = h.mustInit(InitOptions{Workflow: wf, Vars: sdlcVars, AllowCustomCmds: sha})
+	h.advance(m)
+	h.record(m, "discover", findings(1))
+	h.advance(m)
+	h.advance(m)
+	h.advance(m)
+	h.record(m, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
+	h.advance(m)
+	r = h.advance(m)
+	if r.Status != StatusGateFailed || r.Gate.Name != GateConverge || r.Gate.Error.Code != CodeConvergeFailed || !strings.Contains(r.Gate.Error.Detail, "error: ") {
+		t.Fatalf("converge error: %+v", r)
+	}
+	// fixed_class guard: a fake predicate that classes a stop as fixed
+	h = newHarness(t)
+	m = loopRun(t, h, 1, "sdlc-loop")
+	sess, err := m.load(context.Background(), false)
+	if err != nil {
+		t.Fatal(err)
+	}
+	sess.pred = fixedPred{stop: true}
+	r, err = sess.advance()
+	sess.unlock()
+	if err != nil || r.Gate == nil || r.Gate.Name != GateConverge || !strings.Contains(r.Gate.Error.Detail, "fixed_class") {
+		t.Fatalf("fixed_class: %+v %v", r, err)
+	}
+	// a NON-firing fixed-class result (any: [all_fixed, …] with bugs remaining) is not a failure: the loop is taken
+	h = newHarness(t)
+	m = loopRun(t, h, 1, "sdlc-loop")
+	sess, _ = m.load(context.Background(), false)
+	sess.pred = fixedPred{stop: false}
+	r, err = sess.advance()
+	sess.unlock()
+	if err != nil || r.Status != StatusAdvanced || r.To != "discover" {
+		t.Fatalf("non-firing fixed class: %+v %v", r, err)
+	}
+	// the real all_fixed atom firing on a confirmed_empty terminal gate → fixed (design §9 example)
+	h = newHarness(t)
+	wf = sdlcWith(t, h, "af.yaml", "  - {from: verify,     to: done,       gate: all_fixed,   outcome: fixed}", "  - {from: verify,     to: done,       gate: confirmed_empty,   outcome: fixed}")
+	h.files["/x/af.yaml"] = []byte(strings.Replace(string(h.files["/x/af.yaml"]), "any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]", "any: [all_fixed, {max_iterations: 5}]", 1))
+	h.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+	m = h.mustInit(InitOptions{Workflow: wf, Vars: sdlcVars})
+	h.advance(m)
+	h.record(m, "discover", findings(1))
+	h.advance(m)
+	h.advance(m)
+	h.advance(m)
+	h.record(m, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
+	h.advance(m)
+	if r := h.advance(m); r.Outcome != run.OutcomeFixed || r.StopReason != "all_fixed: all bugs fixed" {
+		t.Fatalf("all_fixed atom: %+v", r)
+	}
+	// user workflow whose terminal gate is confirmed_empty: convergence still bounds the loop
+	h = newHarness(t)
+	wf = sdlcWith(t, h, "ce.yaml", "  - {from: verify,     to: done,       gate: all_fixed,   outcome: fixed}", "  - {from: verify,     to: done,       gate: confirmed_empty,   outcome: fixed}")
+	h.files["/x/ce.yaml"] = []byte(strings.Replace(string(h.files["/x/ce.yaml"]), "{max_iterations: 5}", "{max_iterations: 1}", 1))
+	h.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+	m = h.mustInit(InitOptions{Workflow: wf, Vars: sdlcVars})
+	h.advance(m)
+	h.record(m, "discover", findings(1))
+	h.advance(m)
+	h.advance(m)
+	h.advance(m)
+	h.record(m, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
+	h.advance(m)
+	if r := h.advance(m); r.Outcome != run.OutcomeOverflow {
+		t.Fatalf("bounded even when all fixed: %+v", r)
+	}
+	// Atom cap: an `all` of 32 long-named cmd atoms yields a >1 KB name; it is capped, never a store rejection
+	h = newHarness(t)
+	var names, decls []string
+	for i := 0; i < 32; i++ {
+		n := "cmd-" + strings.Repeat("x", 26) + string(rune('a'+i%16)) // 16 declared names, each referenced twice
+		names = append(names, "{cmd: "+n+"}")
+		if i < 16 {
+			decls = append(decls, "  "+n+": {argv: [bash, -c, echo]}")
+		}
+	}
+	wf = sdlcWith(t, h, "wide.yaml", "  any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]\nrepo_mode: advisory",
+		"  all: ["+strings.Join(names, ", ")+"]\ncmds:\n"+strings.Join(decls, "\n")+"\nrepo_mode: advisory")
+	h.runner.res = converge.CmdResult{Stdout: []byte(`{"stop": true, "reason": "x"}`)}
+	_, sha, _ = workflow.ResolveCmds(mustResolve(t, h, wf), "/repo", h.deps.LookPath, h.deps.FileHash)
+	allPresent(h)
+	h.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+	m = h.mustInit(InitOptions{Workflow: wf, Vars: sdlcVars, AllowCustomCmds: sha})
+	h.advance(m)
+	h.record(m, "discover", findings(1))
+	h.advance(m)
+	h.advance(m)
+	h.advance(m)
+	h.record(m, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
+	h.advance(m)
+	if r := h.advance(m); r.Outcome != run.OutcomeCustom || len(r.StopReason) > run.MaxShort+run.MaxText+8 {
+		t.Fatalf("wide all: %+v", r)
+	}
+	// emitter cap: 5 KB cmd reason capped in converge event and StopReason
+	h = newHarness(t)
+	wf = sdlcWith(t, h, "custom3.yaml", "  any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]\nrepo_mode: advisory",
+		"  any: [{cmd: notify}]\ncmds:\n  notify: {argv: [bash, -c, echo]}\nrepo_mode: advisory")
+	big := strings.Repeat("r", 5000)
+	h.runner.res = converge.CmdResult{Stdout: []byte(`{"stop": true, "reason": "` + big + `"}`)}
+	_, sha, _ = workflow.ResolveCmds(mustResolve(t, h, wf), "/repo", h.deps.LookPath, h.deps.FileHash)
+	allPresent(h)
+	h.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+	m = h.mustInit(InitOptions{Workflow: wf, Vars: sdlcVars, AllowCustomCmds: sha})
+	h.advance(m)
+	h.record(m, "discover", findings(1))
+	h.advance(m)
+	h.advance(m)
+	h.advance(m)
+	h.record(m, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
+	h.advance(m)
+	r = h.advance(m)
+	if r.Outcome != run.OutcomeCustom || len(r.StopReason) > run.MaxText+20 {
+		t.Fatalf("capped reason: %d", len(r.StopReason))
+	}
+}
+
+// allPresent makes verify report every bug still present (no llm_calls).
+func allPresent(h *harness) {
+	h.reg.execs["still-present"].fn = func(in ExecInput) (json.RawMessage, error) {
+		var st []run.BugStatus
+		for _, b := range in.Snap.AllFound {
+			st = append(st, run.BugStatus{ID: b.ID, StillPresent: true, Confidence: 1})
+		}
+		return json.RawMessage(run.MarshalCanonical(run.Delta{Status: st})), nil
+	}
+}
+
+type fixedPred struct{ stop bool }
+
+func (fixedPred) Name() string       { return "evil" }
+func (fixedPred) Class() run.Outcome { return run.OutcomeFixed }
+func (p fixedPred) Evaluate(context.Context, run.Snapshot) (converge.Result, error) {
+	return converge.Result{Stop: p.stop, Atom: "evil", Class: run.OutcomeFixed, Reason: "ha"}, nil
+}
+
+func mustResolve(t *testing.T, h *harness, path string) *workflow.Workflow {
+	t.Helper()
+	w, err := workflow.Parse(h.files[path], workflow.Options{Kinds: h.reg.Info()})
+	if err != nil {
+		t.Fatal(err)
+	}
+	r, _, err := w.Resolve(sdlcVars, false)
+	if err != nil {
+		t.Fatal(err)
+	}
+	return r
+}
+
+func TestM4OverflowHandler(t *testing.T) {
+	h := newHarness(t)
+	wf := sdlcWith(t, h, "ov.yaml", "  any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]\nrepo_mode: advisory",
+		"  any: [{max_iterations: 1}]\ncmds:\n  notify: {argv: [bash, -c, echo]}\non_overflow: notify\nrepo_mode: advisory")
+	_, sha, _ := workflow.ResolveCmds(mustResolve(t, h, wf), "/repo", h.deps.LookPath, h.deps.FileHash)
+	h.runner.res = converge.CmdResult{Stdout: []byte("notified"), Stderr: []byte("e"), ExitCode: 0}
+	allPresent(h)
+	h.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+	m := h.mustInit(InitOptions{Workflow: wf, Vars: sdlcVars, AllowCustomCmds: sha})
+	h.advance(m)
+	h.record(m, "discover", findings(1))
+	h.advance(m)
+	h.advance(m)
+	h.advance(m)
+	h.record(m, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
+	h.advance(m)
+	r := h.advance(m)
+	if r.Outcome != run.OutcomeOverflow || len(h.runner.calls) != 1 || h.runner.calls[0] != "notify" {
+		t.Fatalf("handler: %+v %v", r, h.runner.calls)
+	}
+	evs := h.events(m)
+	var oh run.OverflowHandlerData
+	_ = json.Unmarshal(evs[len(evs)-1].Data, &oh)
+	if evs[len(evs)-1].Type != run.TypeOverflowHandler || oh.Name != "notify" || oh.Argv[0] != "/bin/bash" || oh.Stdout != "notified" || oh.Stderr != "e" || oh.ExitCode != 0 || len(oh.InputHash) != 64 || oh.Error != "" {
+		t.Fatalf("overflow_handler: %+v", oh)
+	}
+	if !m.View().Snapshot.OverflowHandled || len(h.terminal) != 1 {
+		t.Fatal("handled + terminal once")
+	}
+	// terminal run: advance → ERR_RUN_TERMINAL (handled)
+	if _, err := m.Advance(context.Background()); !errs.Is(err, CodeRunTerminal) {
+		t.Fatal("terminal")
+	}
+	// failure warn (exit≠0) and runner error (exit −1)
+	for _, tc := range []struct {
+		res  converge.CmdResult
+		err  error
+		exit int
+		code string
+	}{{converge.CmdResult{ExitCode: 3}, nil, 3, ""}, {converge.CmdResult{}, errs.E("ERR_CMD_TIMEOUT", "slow"), -1, "ERR_CMD_TIMEOUT"}, {converge.CmdResult{}, errors.New("plain"), -1, "ERR_CMD_FAILED"}} {
+		h2 := newHarness(t)
+		h2.files["/x/ov.yaml"] = h.files["/x/ov.yaml"]
+		allPresent(h2)
+		h2.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+		h2.runner.res, h2.runner.err = tc.res, tc.err
+		m := h2.mustInit(InitOptions{Workflow: wf, Vars: sdlcVars, AllowCustomCmds: sha})
+		h2.advance(m)
+		h2.record(m, "discover", findings(1))
+		h2.advance(m)
+		h2.advance(m)
+		h2.advance(m)
+		h2.record(m, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
+		h2.advance(m)
+		r := h2.advance(m)
+		if len(r.Warnings) != 1 || !strings.HasPrefix(r.Warnings[0], WarnOverflowHandlerFailed) || r.Untrusted[0] != "warnings" {
+			t.Fatalf("handler failure warn: %+v", r)
+		}
+		evs := h2.events(m)
+		var oh run.OverflowHandlerData
+		_ = json.Unmarshal(evs[len(evs)-2].Data, &oh)
+		if oh.ExitCode != tc.exit || oh.Error != tc.code {
+			t.Fatalf("overflow_handler on failure: %+v", oh)
+		}
+	}
+	// crash-resume: a terminal overflow run whose handler never ran → Advance runs it, then ERR_RUN_TERMINAL
+	h3 := newHarness(t)
+	h3.files["/x/ov.yaml"] = h.files["/x/ov.yaml"]
+	allPresent(h3)
+	h3.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+	h3.runner.res = converge.CmdResult{Stdout: []byte("ok")}
+	h3.store.err = errors.New("crash")
+	m3 := h3.mustInit(InitOptions{Workflow: wf, Vars: sdlcVars, AllowCustomCmds: sha})
+	h3.advance(m3)
+	h3.record(m3, "discover", findings(1))
+	h3.advance(m3)
+	h3.advance(m3)
+	h3.advance(m3)
+	h3.record(m3, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
+	h3.advance(m3)
+	// fail the handler's own cmd_call append: the overflow transition is durable, the handler is not
+	h3.store.failType = run.TypeCmdCall
+	_, err := m3.Advance(context.Background())
+	if err == nil || err.Error() != "crash" {
+		t.Fatalf("expected crash, got %v (appends=%d)", err, h3.store.appends)
+	}
+	h3.store.failType = ""
+	h3.store.failAt = 0
+	if !strings.HasSuffix(strings.Join(h3.types(m3), ","), "converge,transition") {
+		t.Fatalf("crash before handler: %s", strings.Join(h3.types(m3), ","))
+	}
+	r = h3.advance(m3)
+	if r.Status != StatusStopped || r.Outcome != run.OutcomeOverflow || !m3.View().Snapshot.OverflowHandled || countType(h3.events(m3), run.TypeOverflowHandler) != 1 {
+		t.Fatalf("resume handler: %+v", r)
+	}
+	if o := h3.runner.ordinals; len(o) != 2 || o[0] != 0 || o[1] != 0 {
+		t.Fatalf("ordinals: the crashed call stored no cmd_call, so the resume is ordinal 0 again: %v", o)
+	}
+	if _, err := m3.Advance(context.Background()); !errs.Is(err, CodeRunTerminal) {
+		t.Fatal("terminal after resume")
+	}
+}
+
+// ---------------------------------------------------------------- M5 tree
+
+func TestM5Tree(t *testing.T) {
+	h := newHarness(t)
+	m := h.mustInit(InitOptions{Workflow: "review-loop", Vars: sdlcVars})
+	// unchanged → no tree
+	h.advance(m)
+	if countType(h.events(m), run.TypeTree) != 1 {
+		t.Fatal("tree only on change")
+	}
+	// content changes, porcelain identical → advisory warn + tree
+	h.git.def.Tree = "t2"
+	r := h.advance(m)
+	if len(r.Warnings) != 1 || !strings.HasPrefix(r.Warnings[0], WarnUnsanctionedEdit) || countType(h.events(m), run.TypeTree) != 2 || countType(h.events(m), run.TypeWarn) != 1 {
+		t.Fatalf("advisory: %+v", r)
+	}
+	// 5 KB porcelain: warn Detail capped at MaxText, tree status intact
+	h.git.def.Tree = "t3"
+	h.git.def.Porcelain = strings.Repeat("? f\n", 1300)
+	r = h.advance(m)
+	evs := h.events(m)
+	var wd run.WarnData
+	_ = json.Unmarshal(evs[len(evs)-2].Data, &wd)
+	var td run.TreeData
+	_ = json.Unmarshal(evs[len(evs)-1].Data, &td)
+	if len(wd.Detail) > run.MaxText || len(td.Status) != 5200 || td.StatusTruncated {
+		t.Fatalf("caps: warn=%d status=%d", len(wd.Detail), len(td.Status))
+	}
+	// 70 KB porcelain → tree status capped with flag
+	h.git.def.Tree = "t4"
+	h.git.def.Porcelain = strings.Repeat("? f\n", 18000)
+	h.advance(m)
+	evs = h.events(m)
+	_ = json.Unmarshal(evs[len(evs)-1].Data, &td)
+	if !td.StatusTruncated || len(td.Status) > run.MaxDetail {
+		t.Fatal("tree status cap")
+	}
+	// enforcing: gate + no tree; a second advance re-detects
+	h = newHarness(t)
+	m = h.mustInit(InitOptions{Workflow: "review-loop", Vars: sdlcVars, RepoMode: "enforcing"})
+	h.git.def.Tree = "t2"
+	r = h.advance(m)
+	if r.Status != StatusGateFailed || r.Gate.Name != GateRepoMode || r.Gate.Error.Code != CodeUnsanctionedEdit || countType(h.events(m), run.TypeTree) != 1 {
+		t.Fatalf("enforcing: %+v trees=%d", r, countType(h.events(m), run.TypeTree))
+	}
+	// agent-edit state is exempt
+	h = newHarness(t)
+	h.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+	m = h.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, RepoMode: "enforcing"})
+	h.advance(m)
+	h.record(m, "discover", findings(1))
+	h.advance(m)
+	h.advance(m)
+	h.git.def.Tree = "edited"
+	r = h.advance(m)
+	if r.Status != StatusNeedsInput || countType(h.events(m), run.TypeTree) != 2 || countType(h.events(m), run.TypeWarn) != 0 {
+		t.Fatalf("agent-edit exempt: %+v", r)
+	}
+	// baseline when TreeHash == "": fake a log without a tree by creating the run directly
+	h = newHarness(t)
+	m = h.mustInit(InitOptions{Workflow: "review-loop", Vars: sdlcVars})
+	raw, _ := workflows.Read("review-loop")
+	initEv := h.events(m)[0]
+	initEv.Seq, initEv.Prev = 0, ""
+	var initData run.InitData
+	_ = json.Unmarshal(initEv.Data, &initData)
+	initData.RunID = "mrv-no-tree-baseline"
+	first := run.Event{SchemaVersion: run.SchemaVersion, At: initEv.At, Type: run.TypeInit, Data: run.MarshalCanonical(initData)}
+	if _, err := h.store.Create("mrv-no-tree-baseline", first); err != nil {
+		t.Fatal(err)
+	}
+	if err := h.sidecar.Write("mrv-no-tree-baseline", SidecarWorkflow, raw); err != nil {
+		t.Fatal(err)
+	}
+	m2, err := Open(context.Background(), h.deps, "mrv-no-tree-baseline", OpenOptions{})
+	if err != nil {
+		t.Fatal(err)
+	}
+	h.advance(m2)
+	if got := strings.Join(h.types(m2), ","); got != "init,tree,needs_input" {
+		t.Fatalf("baseline: %s", got)
+	}
+}
+
+// ---------------------------------------------------------------- M6 Record
+
+func TestM6Record(t *testing.T) {
+	h := newHarness(t)
+	m := h.mustInit(InitOptions{Workflow: "review-loop", Vars: sdlcVars})
+	ctx := context.Background()
+	rec := func(o RecordOptions) error { _, err := m.Record(ctx, o); return err }
+	wantCode(t, rec(RecordOptions{Kind: RecordNodeOutput, Node: "adjudicate", Data: json.RawMessage(`{}`)}), CodeNodeMismatch)
+	wantCode(t, rec(RecordOptions{Kind: RecordNodeOutput, Node: "discover", Data: json.RawMessage(`{"nope":1}`)}), CodeNodeOutputInvalid)
+	if got := strings.Join(h.types(m), ","); got != "init,tree" {
+		t.Fatalf("invalid output leaves the log unchanged: %s", got)
+	}
+	if err := rec(RecordOptions{Kind: RecordNodeOutput, Node: "discover", Data: json.RawMessage(`{"findings":[{"issue_text":"a"}]}`)}); err != nil {
+		t.Fatal(err)
+	}
+	e := wantCode(t, rec(RecordOptions{Kind: RecordNodeOutput, Node: "discover", Data: json.RawMessage(`{"findings":[]}`)}), CodeNodeOutputExists)
+	if e.Field("key") != "discover@0" {
+		t.Fatal("key")
+	}
+	r, err := m.Record(ctx, RecordOptions{Kind: RecordNodeOutput, Node: "discover", Data: json.RawMessage(`{"findings":[]}`), Replace: true})
+	if err != nil || r.Key != "discover@0" || r.Type != run.TypeNodeOutput {
+		t.Fatalf("replace: %+v %v", r, err)
+	}
+	h.advance(m) // applied → clean → terminal
+	wantCode(t, rec(RecordOptions{Kind: RecordNodeOutput, Node: "discover", Data: json.RawMessage(`{"findings":[]}`)}), CodeRunTerminal)
+	// tokens/event allowed on a terminal run
+	if _, err := m.Record(ctx, RecordOptions{Kind: RecordTokens, Data: json.RawMessage(`{"input":1}`)}); err != nil {
+		t.Fatal(err)
+	}
+	if r, err := m.Record(ctx, RecordOptions{Kind: RecordEvent, Name: "my-note", Data: json.RawMessage(`{"x":1}`)}); err != nil || r.Type != run.TypeRecord || r.Key != "my-note" {
+		t.Fatalf("event: %+v %v", r, err)
+	}
+	// tokens errors
+	for _, d := range []string{`{"input":-1}`, `{"zzz":1}`, `not json`, `{"input":1099511627777}`} {
+		wantCode(t, rec(RecordOptions{Kind: RecordTokens, Data: json.RawMessage(d)}), CodeRecordTokens)
+	}
+	// record name sub-rules
+	for name, reason := range map[string]string{"Bad": "syntax", "transition": "event_type", "mrv_x": "reserved"} {
+		e := wantCode(t, rec(RecordOptions{Kind: RecordEvent, Name: name, Data: json.RawMessage(`{}`)}), CodeRecordName)
+		if e.Field("reason") != reason {
+			t.Errorf("%s: %s", name, e.Field("reason"))
+		}
+	}
+	wantCode(t, rec(RecordOptions{Kind: RecordEvent, Name: "ok", Data: json.RawMessage(`nope`)}), CodeRecordTooLarge)
+	wantCode(t, rec(RecordOptions{Kind: RecordEvent, Name: "ok", Data: json.RawMessage(`"` + strings.Repeat("x", run.MaxPayload) + `"`)}), CodeRecordTooLarge)
+	wantCode(t, rec(RecordOptions{Kind: "bogus"}), CodeRecordName)
+	// applied and not-host
+	m2 := h.mustInit(InitOptions{Workflow: "review-loop", Vars: sdlcVars})
+	h.advance(m2)
+	h.record(m2, "discover", findings(1))
+	h.advance(m2)
+	if _, err := m2.Record(ctx, RecordOptions{Kind: RecordNodeOutput, Node: "adjudicate", Data: json.RawMessage(`{}`)}); !errs.Is(err, CodeNodeNotHost) {
+		t.Fatalf("not host: %v", err)
+	}
+	m3 := h.mustInit(InitOptions{Workflow: "review-loop", Vars: sdlcVars})
+	h.advance(m3)
+	h.record(m3, "discover", findings(1))
+	// apply without transitioning: fail the gate append so the delta is applied but the state stays
+	h.store.appends, h.store.failAt, h.store.err = 0, 2, errors.New("stop")
+	_, _ = m3.Advance(ctx)
+	h.store.failAt = 0
+	if _, err := m3.Record(ctx, RecordOptions{Kind: RecordNodeOutput, Node: "discover", Data: json.RawMessage(`{"findings":[]}`), Replace: true}); !errs.Is(err, CodeNodeOutputApplied) {
+		t.Fatalf("applied: %v", err)
+	}
+	// append failure inside Record
+	h.store.appends, h.store.failAt, h.store.err = 0, 1, errors.New("disk")
+	if _, err := m.Record(ctx, RecordOptions{Kind: RecordTokens, Data: json.RawMessage(`{"input":1}`)}); err == nil || err.Error() != "disk" {
+		t.Fatalf("tokens append: %v", err)
+	}
+	h.store.appends, h.store.failAt = 0, 1
+	if _, err := m.Record(ctx, RecordOptions{Kind: RecordEvent, Name: "ok", Data: json.RawMessage(`{}`)}); err == nil || err.Error() != "disk" {
+		t.Fatalf("event append: %v", err)
+	}
+	h.store.failAt = 0
+	m4 := h.mustInit(InitOptions{Workflow: "review-loop", Vars: sdlcVars})
+	h.store.appends, h.store.failAt = 0, 1
+	if _, err := m4.Record(ctx, RecordOptions{Kind: RecordNodeOutput, Node: "discover", Data: json.RawMessage(`{"findings":[]}`)}); err == nil || err.Error() != "disk" {
+		t.Fatalf("node append: %v", err)
+	}
+	h.store.failAt = 0
+	// invalid JSON output that Decode accepts? Decode rejects first; canonical error path via a kind that accepts anything
+	h.reg.kinds["review-lenses"].decode = func(json.RawMessage) (any, error) { return run.Delta{}, nil }
+	if _, err := m4.Record(ctx, RecordOptions{Kind: RecordNodeOutput, Node: "discover", Data: json.RawMessage(`{"a":1,"a":2}`)}); !errs.Is(err, CodeNodeOutputInvalid) {
+		t.Fatalf("canonical: %v", err)
+	}
+	// load failure inside Record (lock)
+	h.store.failOp, h.store.err = "Lock", errors.New("locked")
+	if _, err := m.Record(ctx, RecordOptions{Kind: RecordTokens, Data: json.RawMessage(`{}`)}); err == nil || err.Error() != "locked" {
+		t.Fatal("record lock")
+	}
+	h.store.failOp = ""
+}
+
+// ---------------------------------------------------------------- M7 Open
+
+func TestM7Open(t *testing.T) {
+	h := newHarness(t)
+	ctx := context.Background()
+	m := h.mustInit(InitOptions{Workflow: "review-loop", Vars: sdlcVars})
+	if _, err := Open(ctx, h.deps, "mrv-does-not-exist", OpenOptions{}); !isStoreCode(err, run.CodeRunNotFound) {
+		t.Fatalf("not found: %v", err)
+	}
+	m2, err := Open(ctx, h.deps, m.runID, OpenOptions{})
+	if err != nil || m2.View().NextAction != NextAdvance {
+		t.Fatalf("open: %v", err)
+	}
+	// sidecar edit → ERR_WORKFLOW_CHANGED; embedded bytes replaced but sidecar intact → follows the sidecar
+	raw, _ := workflows.Read("review-loop")
+	h.sidecar.Put(m.runID, SidecarWorkflow, append([]byte(nil), append(raw, '\n')...))
+	if _, err := Open(ctx, h.deps, m.runID, OpenOptions{}); !errs.Is(err, CodeWorkflowChanged) {
+		t.Fatalf("changed: %v", err)
+	}
+	h.sidecar.Put(m.runID, SidecarWorkflow, raw)
+	h.deps.Workflows = func(string) ([]byte, error) { return []byte("garbage"), nil }
+	if _, err := Open(ctx, h.deps, m.runID, OpenOptions{}); err != nil {
+		t.Fatalf("embedded bytes are irrelevant after init: %v", err)
+	}
+	h.deps.Workflows = workflows.Read
+	h.sidecar.Delete(m.runID, SidecarWorkflow)
+	if e := wantCode(t, mustErr(Open(ctx, h.deps, m.runID, OpenOptions{})), CodeSidecar); e.Field("reason") != "missing" {
+		t.Fatal("missing sidecar")
+	}
+	h.sidecar.Put(m.runID, SidecarWorkflow, raw)
+	// sidecar parses but a vocabulary change makes it invalid → ERR_WORKFLOW_INVALID
+	delete(h.reg.kinds, "match-then-adjudicate")
+	if _, err := Open(ctx, h.deps, m.runID, OpenOptions{}); !errs.Is(err, workflow.CodeWorkflowInvalid) {
+		t.Fatalf("vocabulary: %v", err)
+	}
+	h.reg = newRegistry()
+	h.deps.Kinds = h.reg
+	// stored vars no longer resolve (var removed from the registry-independent workflow is impossible; simulate with a sidecar whose vars differ)
+	// ERR_CMD_CHANGED via the consent list
+	withCmd := strings.Replace(raw2(t, "sdlc-loop"), "repo_mode: advisory", "cmds:\n  notify: {argv: [bash]}\nrepo_mode: advisory", 1)
+	h.files["/x/cmd.yaml"] = []byte(withCmd)
+	_, cerr := h.init(InitOptions{Workflow: "/x/cmd.yaml", Vars: sdlcVars})
+	sha := errs.As(cerr).Field("sha")
+	mc := h.mustInit(InitOptions{Workflow: "/x/cmd.yaml", Vars: sdlcVars, AllowCustomCmds: sha})
+	h.deps.FileHash = func(p string) (string, error) { return "changed", nil }
+	if _, err := Open(ctx, h.deps, mc.runID, OpenOptions{}); !errs.Is(err, workflow.CodeCmdChanged) {
+		t.Fatalf("cmd changed: %v", err)
+	}
+	h.deps.FileHash = func(p string) (string, error) {
+		if p == "/bin/bash" {
+			return "hb", nil
+		}
+		return "", errors.New("no")
+	}
+	// mock mismatch via scenario edit and via registry
+	h.mockHash["/repo/scen"] = strings.Repeat("1", 64)
+	h.reg.mock = true
+	mm := h.mustInit(InitOptions{Workflow: "review-loop", Vars: sdlcVars, MockDir: "scen"})
+	h.mockHash["/repo/scen"] = strings.Repeat("2", 64)
+	if _, err := Open(ctx, h.deps, mm.runID, OpenOptions{}); !errs.Is(err, CodeMockMismatch) {
+		t.Fatalf("scenario edited: %v", err)
+	}
+	h.mockHash["/repo/scen"] = strings.Repeat("1", 64)
+	h.reg.mock = false
+	if _, err := Open(ctx, h.deps, mm.runID, OpenOptions{}); !errs.Is(err, CodeMockMismatch) {
+		t.Fatalf("registry mismatch: %v", err)
+	}
+	h.reg.mock = true
+	if _, err := Open(ctx, h.deps, mm.runID, OpenOptions{}); err != nil {
+		t.Fatal(err)
+	}
+	h.reg.mock = false
+	// stored vars that no longer resolve: a crafted init whose vars lack JUDGE (only reachable by hand-writing a log)
+	var initData run.InitData
+	_ = json.Unmarshal(h.events(m)[0].Data, &initData)
+	initData.RunID = "mrv-crafted-novars-log"
+	initData.Vars = map[string]string{}
+	first := run.Event{SchemaVersion: run.SchemaVersion, At: initData.CreatedAt, Type: run.TypeInit, Data: run.MarshalCanonical(initData)}
+	if _, err := h.store.Create("mrv-crafted-novars-log", first); err != nil {
+		t.Fatal(err)
+	}
+	h.sidecar.Put("mrv-crafted-novars-log", SidecarWorkflow, raw)
+	if _, err := Open(ctx, h.deps, "mrv-crafted-novars-log", OpenOptions{}); !errs.Is(err, workflow.CodeVarUnset) {
+		t.Fatalf("stored vars unresolvable: %v", err)
+	}
+	// convergence parse failure on open is impossible after Parse validated; runner wiring observed via Runner call count
+	// torn tail: only reachable with the JSONL store → covered in TestM7Torn
+	// lock / events failures
+	h.store.failOp, h.store.err = "Events", errors.New("events")
+	if _, err := Open(ctx, h.deps, m.runID, OpenOptions{}); err == nil || err.Error() != "events" {
+		t.Fatal("events failure")
+	}
+	h.store.failOp = ""
+}
+
+func mustErr(_ *Machine, err error) error { return err }
+
+func raw2(t *testing.T, name string) string {
+	t.Helper()
+	b, err := workflows.Read(name)
+	if err != nil {
+		t.Fatal(err)
+	}
+	return string(b)
+}
+
+func TestM7Torn(t *testing.T) {
+	ctx := context.Background()
+	h := newHarness(t)
+	root := t.TempDir()
+	h.store = &countingStore{RunStore: run.NewJSONLStore(root, run.Options{})}
+	h.deps.Store = h.store
+	h.deps.Sidecar = FSSidecar{Root: root}
+	m := h.mustInit(InitOptions{Workflow: "review-loop", Vars: sdlcVars})
+	h.advance(m)
+	// tear the tail
+	p := root + "/.metareview/runs/" + m.runID + "/audit.jsonl"
+	appendBytes(t, p, `{"seq":99,"garbage`)
+	mo, err := Open(ctx, h.deps, m.runID, OpenOptions{})
+	if err != nil || !mo.View().Torn {
+		t.Fatalf("open torn: %v", err)
+	}
+	if _, err := mo.Advance(ctx); !errs.Is(err, run.CodeAuditTorn) {
+		t.Fatalf("advance torn: %v", err)
+	}
+	if _, err := mo.Record(ctx, RecordOptions{Kind: RecordTokens, Data: json.RawMessage(`{}`)}); !errs.Is(err, run.CodeAuditTorn) {
+		t.Fatalf("record torn: %v", err)
+	}
+	// repair → warn with the literal detail, fold ok
+	mr, err := Open(ctx, h.deps, m.runID, OpenOptions{Repair: true})
+	if err != nil || mr.View().Torn {
+		t.Fatalf("repair: %v", err)
+	}
+	evs := h.events(mr)
+	var wd run.WarnData
+	_ = json.Unmarshal(evs[len(evs)-1].Data, &wd)
+	if evs[len(evs)-1].Type != run.TypeWarn || wd.Code != WarnAuditTornLineDropped || wd.Detail != "18 bytes dropped after seq 3 from audit.jsonl" {
+		t.Fatalf("repair warn: %+v", wd)
+	}
+	// repair on a clean log → ERR_AUDIT_NOT_TORN passes through
+	if _, err := Open(ctx, h.deps, m.runID, OpenOptions{Repair: true}); !isStoreCode(err, run.CodeAuditNotTorn) {
+		t.Fatalf("not torn: %v", err)
+	}
+	// repair at offset 0 removes the run → ERR_RUN_NOT_FOUND with the detail
+	h.deps.Sidecar = FSSidecar{Root: root}
+	m2 := h.mustInit(InitOptions{Workflow: "review-loop", Vars: sdlcVars})
+	p2 := root + "/.metareview/runs/" + m2.runID + "/audit.jsonl"
+	writeBytes(t, p2, `{"torn`)
+	err = mustErr(Open(ctx, h.deps, m2.runID, OpenOptions{Repair: true}))
+	if !errs.Is(err, run.CodeRunNotFound) || !strings.Contains(err.Error(), "torn bytes") {
+		t.Fatalf("offset 0: %v", err)
+	}
+	// repair failure (other) and events failure after repair
+	m3 := h.mustInit(InitOptions{Workflow: "review-loop", Vars: sdlcVars})
+	appendBytes(t, root+"/.metareview/runs/"+m3.runID+"/audit.jsonl", `{"x`)
+	h.store.failOp, h.store.err = "Repair", errors.New("repair boom")
+	if _, err := Open(ctx, h.deps, m3.runID, OpenOptions{Repair: true}); err == nil || err.Error() != "repair boom" {
+		t.Fatalf("repair error: %v", err)
+	}
+	h.store.failOp = ""
+	// warn append failure after repair
+	h.store.appends, h.store.failAt, h.store.err = 0, 1, errors.New("warn boom")
+	if _, err := Open(ctx, h.deps, m3.runID, OpenOptions{Repair: true}); err == nil || err.Error() != "warn boom" {
+		t.Fatalf("warn append: %v", err)
+	}
+	h.store.failAt = 0
+	// the re-read after the repair warn fails
+	appendBytes(t, root+"/.metareview/runs/"+m3.runID+"/audit.jsonl", `{"z`)
+	h.store.events, h.store.failEvAt, h.store.err = 0, 3, errors.New("reread")
+	if _, err := Open(ctx, h.deps, m3.runID, OpenOptions{Repair: true}); err == nil || err.Error() != "reread" {
+		t.Fatalf("reread: %v", err)
+	}
+	h.store.failEvAt = 0
+	// the read right after RepairTail fails (other than not-found)
+	m5 := h.mustInit(InitOptions{Workflow: "review-loop", Vars: sdlcVars})
+	appendBytes(t, root+"/.metareview/runs/"+m5.runID+"/audit.jsonl", `{"y`)
+	h.store.events, h.store.failEvAt, h.store.err = 0, 2, errors.New("postrepair")
+	if _, err := Open(ctx, h.deps, m5.runID, OpenOptions{Repair: true}); err == nil || err.Error() != "postrepair" {
+		t.Fatalf("post-repair read: %v", err)
+	}
+	h.store.failEvAt = 0
+	// chain-valid but fold-invalid log → FoldFull error surfaces from Open
+	b := run.NewBuilder("mrv-crafted-invalid-log")
+	var initData run.InitData
+	_ = json.Unmarshal(h.events(m)[0].Data, &initData)
+	initData.RunID = "mrv-crafted-invalid-log"
+	b.Init(initData)
+	b.Event(run.TypeTransition, run.TransitionData{From: "nowhere", To: "done", Gate: "x", Outcome: run.OutcomeClean, Head: shaHead})
+	dir := root + "/.metareview/runs/mrv-crafted-invalid-log"
+	mkdir(t, dir)
+	writeBytes(t, dir+"/audit.jsonl", string(joinLines(b.Lines())))
+	if _, err := Open(ctx, h.deps, "mrv-crafted-invalid-log", OpenOptions{}); err == nil || !strings.Contains(err.Error(), "stamp") {
+		t.Fatalf("fold-invalid log: %v", err)
+	}
+	// FS sidecar rows: symlink refused, exists, missing run
+	fs := FSSidecar{Root: root}
+	if err := fs.Write(m.runID, SidecarWorkflow, []byte("x")); !errs.Is(err, CodeSidecar) || errs.As(err).Field("reason") != "exists" {
+		t.Fatalf("exists: %v", err)
+	}
+	if err := fs.Write("mrv-no-such-run-here", "note.txt", []byte("x")); !errs.Is(err, CodeSidecar) || errs.As(err).Field("reason") != "path" {
+		t.Fatalf("missing run dir: %v", err)
+	}
+	if _, err := fs.Read(m.runID, "nothing.txt"); errs.As(err).Field("reason") != "missing" {
+		t.Fatalf("missing file: %v", err)
+	}
+	if err := fs.Write(m.runID, "audit.evil", nil); errs.As(err).Field("reason") != "name" {
+		t.Fatal("reserved name")
+	}
+	if err := fs.Write("bad id", "x", nil); errs.As(err).Field("reason") != "path" {
+		t.Fatal("bad run id")
+	}
+	dir = root + "/.metareview/runs/" + m.runID
+	symlink(t, "/etc/hosts", dir+"/link.txt")
+	if _, err := fs.Read(m.runID, "link.txt"); !errs.Is(err, CodeSidecar) {
+		t.Fatal("symlink read refused")
+	}
+	if err := fs.Write(m.runID, "link.txt", []byte("x")); errs.As(err).Field("reason") != "exists" {
+		t.Fatal("symlink write refused")
+	}
+	writeBytes(t, dir+"/big.bin", strings.Repeat("x", run.MaxPayload+1))
+	if _, err := fs.Read(m.runID, "big.bin"); errs.As(err).Field("reason") != "too_large" {
+		t.Fatal("too large")
+	}
+	names, err := fs.List(m.runID)
+	if err != nil || strings.Join(names, ",") != "big.bin,workflow.yaml" {
+		t.Fatalf("list: %v %v", names, err)
+	}
+	if _, err := fs.List("mrv-no-such-run-here"); err == nil {
+		t.Fatal("list missing")
+	}
+	if _, err := fs.List("bad"); err == nil {
+		t.Fatal("list bad id")
+	}
+	mode := fileMode(t, dir+"/workflow.yaml")
+	if mode != 0o600 {
+		t.Fatalf("mode %o", mode)
+	}
+	// unreadable file (permission) → path
+	if fileOwnerCanChmod() {
+		writeBytes(t, dir+"/noread.txt", "x")
+		chmod(t, dir+"/noread.txt", 0)
+		if _, err := fs.Read(m.runID, "noread.txt"); errs.As(err).Field("reason") != "path" {
+			t.Fatalf("unreadable: %v", err)
+		}
+	}
+	// injected file failures: write error, close error, read error
+	bad := FSSidecar{Root: root, Open: func(string, int, os.FileMode) (io.ReadWriteCloser, error) {
+		return badFile{werr: errors.New("wfail")}, nil
+	}}
+	if err := bad.Write(m.runID, "inj.txt", []byte("x")); errs.As(err).Field("reason") != "path" || !strings.Contains(err.Error(), "wfail") {
+		t.Fatalf("write error: %v", err)
+	}
+	bad.Open = func(string, int, os.FileMode) (io.ReadWriteCloser, error) {
+		return badFile{cerr: errors.New("cfail")}, nil
+	}
+	if err := bad.Write(m.runID, "inj.txt", []byte("x")); !strings.Contains(err.Error(), "cfail") {
+		t.Fatalf("close error: %v", err)
+	}
+	bad.Open = func(string, int, os.FileMode) (io.ReadWriteCloser, error) {
+		return badFile{rerr: errors.New("rfail")}, nil
+	}
+	if _, err := bad.Read(m.runID, "inj.txt"); !strings.Contains(err.Error(), "rfail") {
+		t.Fatalf("read error: %v", err)
+	}
+	// mem sidecar edge rows
+	ms := &MemSidecar{}
+	if err := ms.Write("bad id", "x", nil); err == nil {
+		t.Fatal("mem bad id")
+	}
+	if _, err := ms.Read("bad id", "x"); err == nil {
+		t.Fatal("mem read bad id")
+	}
+	if _, err := ms.List("bad id"); err == nil {
+		t.Fatal("mem list bad id")
+	}
+	ms.Put("mrv-mem-run-1", "big.bin", make([]byte, run.MaxPayload+1))
+	if _, err := ms.Read("mrv-mem-run-1", "big.bin"); errs.As(err).Field("reason") != "too_large" {
+		t.Fatal("mem too large")
+	}
+	if names, _ := ms.List("mrv-mem-run-1"); strings.Join(names, ",") != "big.bin" {
+		t.Fatal("mem list")
+	}
+}
+
+// ---------------------------------------------------------------- M8 stamps
+
+func TestM8Stamps(t *testing.T) {
+	h := newHarness(t)
+	h.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+	m := h.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars})
+	h.advance(m)
+	h.record(m, "discover", findings(1))
+	h.advance(m)
+	h.advance(m)
+	h.advance(m)
+	h.record(m, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
+	h.advance(m)
+	h.advance(m)
+	evs := h.events(m)
+	for i, ev := range evs {
+		if !ev.At.Time.Equal(evs[0].At.Time.Add(timeSeconds(i))) {
+			t.Fatalf("At sequence at %d: %v", i, ev.At)
+		}
+		if ev.Mock {
+			t.Fatal("no Mock stamp on a real run")
+		}
+	}
+	snap := m.View().Snapshot
+	st, err := run.Fold(evs)
+	if err != nil || !run.SnapshotEqualIgnoringSeq(snap, st) || snap.MockTainted {
+		t.Fatal("view == fold")
+	}
+}
+
+func timeSeconds(i int) timeDuration { return timeDuration(i) * timeDuration(1e9) }
+
+// ---------------------------------------------------------------- M9 exits / store failures
+
+func TestM9ExitsAndStoreFailures(t *testing.T) {
+	h := newHarness(t)
+	h.termErr = errors.New("record failed")
+	m := h.mustInit(InitOptions{Workflow: "review-loop", Vars: sdlcVars})
+	h.advance(m)
+	h.record(m, "discover", `{"findings":[]}`)
+	if _, err := m.Advance(context.Background()); err == nil || err.Error() != "record failed" {
+		t.Fatalf("terminal error surfaced: %v", err)
+	}
+	if m.View().Snapshot.Outcome != run.OutcomeClean {
+		t.Fatal("transition durable despite Terminal error")
+	}
+	h.termErr = nil
+	// nil Terminal is a no-op
+	h = newHarness(t)
+	h.deps.Terminal = nil
+	m = h.mustInit(InitOptions{Workflow: "review-loop", Vars: sdlcVars})
+	h.advance(m)
+	h.record(m, "discover", `{"findings":[]}`)
+	if r := h.advance(m); r.Status != StatusDone {
+		t.Fatal("nil terminal")
+	}
+	// MaxEvents small → ERR_AUDIT_FULL surfaced
+	h = newHarness(t)
+	h.store = &countingStore{RunStore: run.NewMemStore(run.Options{MaxEvents: 2})}
+	h.deps.Store = h.store
+	m = h.mustInit(InitOptions{Workflow: "review-loop", Vars: sdlcVars})
+	h.advance(m) // needs_input counted (1)
+	if _, err := m.Record(context.Background(), RecordOptions{Kind: RecordTokens, Data: json.RawMessage(`{"input":1}`)}); !isStoreCode(err, run.CodeAuditFull) {
+		t.Fatalf("audit full: %v", err)
+	}
+	// every append site in a happy review-loop returns the store error unchanged
+	base := newHarness(t)
+	base.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+	mb := base.mustInit(InitOptions{Workflow: "review-loop", Vars: sdlcVars})
+	base.advance(mb)
+	base.record(mb, "discover", findings(1))
+	base.advance(mb)
+	base.advance(mb)
+	total := base.store.appends
+	for n := 1; n <= total; n++ {
+		hh := newHarness(t)
+		hh.store.failAt, hh.store.err = n, errors.New("appendfail")
+		mm, err := hh.init(InitOptions{Workflow: "review-loop", Vars: sdlcVars})
+		var errs2 []error
+		if err != nil {
+			errs2 = append(errs2, err)
+		} else {
+			steps := []func() error{
+				func() error { _, e := mm.Advance(context.Background()); return e },
+				func() error {
+					_, e := mm.Record(context.Background(), RecordOptions{Kind: RecordNodeOutput, Node: "discover", Data: json.RawMessage(findings(1))})
+					return e
+				},
+				func() error { _, e := mm.Advance(context.Background()); return e },
+				func() error { _, e := mm.Advance(context.Background()); return e },
+			}
+			for _, s := range steps {
+				if e := s(); e != nil {
+					errs2 = append(errs2, e)
+					break
+				}
+			}
+		}
+		if len(errs2) != 1 || errs2[0].Error() != "appendfail" {
+			t.Fatalf("append #%d: %v", n, errs2)
+		}
+	}
+	// the same sweep over richer sdlc paths: advisory tree change, loop, overflow + handler, enforcing edit, executor/decode failures, converge error
+	for _, sc := range sweepScenarios(t) {
+		hb := newHarness(t)
+		sc.setup(hb)
+		mb := hb.mustInit(sc.opts(hb))
+		for _, step := range sc.steps {
+			if err := step(hb, mb); err != nil {
+				t.Fatalf("%s baseline: %v", sc.name, err)
+			}
+		}
+		total := hb.store.appends
+		for n := 1; n <= total; n++ {
+			hh := newHarness(t)
+			sc.setup(hh)
+			hh.store.failAt, hh.store.err = n, errors.New("appendfail")
+			var got error
+			mm, err := hh.init(sc.opts(hh))
+			if err != nil {
+				got = err
+			} else {
+				for _, step := range sc.steps {
+					if e := step(hh, mm); e != nil {
+						got = e
+						break
+					}
+				}
+			}
+			if got == nil || got.Error() != "appendfail" {
+				t.Fatalf("%s append #%d/%d: %v", sc.name, n, total, got)
+			}
+		}
+	}
+	// Advance load failure (lock)
+	h = newHarness(t)
+	m = h.mustInit(InitOptions{Workflow: "review-loop", Vars: sdlcVars})
+	h.store.failOp, h.store.err = "Lock", errors.New("locked")
+	if _, err := m.Advance(context.Background()); err == nil || err.Error() != "locked" {
+		t.Fatal("advance lock")
+	}
+	h.store.failOp = ""
+	if m.RunID() != m.runID {
+		t.Fatal("RunID")
+	}
+}
+
+type sweepScenario struct {
+	name  string
+	setup func(*harness)
+	opts  func(*harness) InitOptions
+	steps []func(*harness, *Machine) error
+}
+
+func adv(h *harness, m *Machine) error { _, err := m.Advance(context.Background()); return err }
+func rec(node, data string) func(*harness, *Machine) error {
+	return func(h *harness, m *Machine) error {
+		_, err := m.Record(context.Background(), RecordOptions{Kind: RecordNodeOutput, Node: node, Data: json.RawMessage(data)})
+		return err
+	}
+}
+
+func sweepScenarios(t *testing.T) []sweepScenario {
+	fixData := `{"commit":"` + shaFix + `","summary":"s"}`
+	loopSteps := []func(*harness, *Machine) error{adv, rec("discover", findings(1)), adv, adv, adv, rec("fix", fixData), adv, adv}
+	ovYAML := func(h *harness) string {
+		return sdlcWith(t, h, "sweep-ov.yaml", "  any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]\nrepo_mode: advisory",
+			"  any: [{max_iterations: 1}]\ncmds:\n  notify: {argv: [bash, -c, echo]}\non_overflow: notify\nrepo_mode: advisory")
+	}
+	return []sweepScenario{
+		{"overflow+handler+warn", func(h *harness) {
+			allPresent(h)
+			h.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+			h.runner.res = converge.CmdResult{ExitCode: 2}
+		}, func(h *harness) InitOptions {
+			wf := ovYAML(h)
+			_, sha, _ := workflow.ResolveCmds(mustResolve(t, h, wf), "/repo", h.deps.LookPath, h.deps.FileHash)
+			return InitOptions{Workflow: wf, Vars: sdlcVars, AllowCustomCmds: sha}
+		}, append([]func(*harness, *Machine) error{func(h *harness, m *Machine) error { h.git.def.Tree = "changed"; return nil }}, loopSteps...)},
+		{"enforcing", func(h *harness) {}, func(h *harness) InitOptions {
+			return InitOptions{Workflow: "review-loop", Vars: sdlcVars, RepoMode: "enforcing"}
+		},
+			[]func(*harness, *Machine) error{func(h *harness, m *Machine) error { h.git.def.Tree = "changed"; return nil }, adv}},
+		{"executor-fail", func(h *harness) {
+			h.reg.execs["match-then-adjudicate"].fn = func(in ExecInput) (json.RawMessage, error) { return nil, errors.New("down") }
+		}, func(h *harness) InitOptions { return InitOptions{Workflow: "review-loop", Vars: sdlcVars} },
+			[]func(*harness, *Machine) error{adv, rec("discover", findings(1)), adv, adv}},
+		{"decode-fail", func(h *harness) {
+			h.reg.execs["match-then-adjudicate"].fn = func(in ExecInput) (json.RawMessage, error) { return json.RawMessage(`{"zz":1}`), nil }
+		}, func(h *harness) InitOptions { return InitOptions{Workflow: "review-loop", Vars: sdlcVars} },
+			[]func(*harness, *Machine) error{adv, rec("discover", findings(1)), adv, adv}},
+		{"converge-error", func(h *harness) {
+			allPresent(h)
+			h.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+			h.runner.res = converge.CmdResult{Stdout: []byte("garbage")}
+		}, func(h *harness) InitOptions {
+			wf := sdlcWith(t, h, "sweep-cv.yaml", "  any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]\nrepo_mode: advisory",
+				"  any: [{cmd: notify}]\ncmds:\n  notify: {argv: [bash, -c, echo]}\nrepo_mode: advisory")
+			_, sha, _ := workflow.ResolveCmds(mustResolve(t, h, wf), "/repo", h.deps.LookPath, h.deps.FileHash)
+			return InitOptions{Workflow: wf, Vars: sdlcVars, AllowCustomCmds: sha}
+		}, loopSteps},
+	}
+}
+
+func TestM9Residue(t *testing.T) {
+	ctx := context.Background()
+	// Init: the repo root's CommonDir fails while the work dir's succeeds
+	h := newHarness(t)
+	h.git.byDir["/repo"] = &failingGit{Git: h.git.def, at: "CommonDir", err: errs.E(gate.CodeGit, "root down", "op", "x")}
+	h.git.byDir["/w"] = h.git.def
+	if _, err := h.init(InitOptions{Workflow: "review-loop", Vars: sdlcVars, WorkDir: "/w"}); !errs.Is(err, gate.CodeGit) {
+		t.Fatalf("root common dir: %v", err)
+	}
+	// terminal advance: Terminal error surfaces before ERR_RUN_TERMINAL
+	h = newHarness(t)
+	m := h.mustInit(InitOptions{Workflow: "review-loop", Vars: sdlcVars})
+	h.advance(m)
+	h.record(m, "discover", `{"findings":[]}`)
+	h.advance(m)
+	h.termErr = errors.New("hook down")
+	if _, err := m.Advance(ctx); err == nil || err.Error() != "hook down" {
+		t.Fatalf("terminal hook on a terminal run: %v", err)
+	}
+	// fail(): Terminal error after the failed transition
+	h = newHarness(t)
+	h.reg.execs["match-then-adjudicate"].fn = func(in ExecInput) (json.RawMessage, error) { return nil, errors.New("down") }
+	m = h.mustInit(InitOptions{Workflow: "review-loop", Vars: sdlcVars})
+	h.advance(m)
+	h.record(m, "discover", findings(1))
+	h.advance(m)
+	h.termErr = errors.New("hook down")
+	if _, err := m.Advance(ctx); err == nil || err.Error() != "hook down" {
+		t.Fatalf("terminal hook on failed: %v", err)
+	}
+	if m.View().Snapshot.Outcome != run.OutcomeFailed {
+		t.Fatal("failed transition durable")
+	}
+	// baseline tree append failure (TreeHash == "") and a tree append failure at an agent-edit state
+	h = newHarness(t)
+	m = h.mustInit(InitOptions{Workflow: "review-loop", Vars: sdlcVars})
+	raw, _ := workflows.Read("review-loop")
+	var initData run.InitData
+	_ = json.Unmarshal(h.events(m)[0].Data, &initData)
+	initData.RunID = "mrv-no-tree-baseline-2"
+	first := run.Event{SchemaVersion: run.SchemaVersion, At: initData.CreatedAt, Type: run.TypeInit, Data: run.MarshalCanonical(initData)}
+	if _, err := h.store.Create("mrv-no-tree-baseline-2", first); err != nil {
+		t.Fatal(err)
+	}
+	_ = h.sidecar.Write("mrv-no-tree-baseline-2", SidecarWorkflow, raw)
+	mb, err := Open(ctx, h.deps, "mrv-no-tree-baseline-2", OpenOptions{})
+	if err != nil {
+		t.Fatal(err)
+	}
+	h.store.failType, h.store.err = run.TypeTree, errors.New("treefail")
+	if _, err := mb.Advance(ctx); err == nil || err.Error() != "treefail" {
+		t.Fatalf("baseline tree append: %v", err)
+	}
+	h = newHarness(t)
+	h.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+	m = h.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars})
+	h.advance(m)
+	h.record(m, "discover", findings(1))
+	h.advance(m)
+	h.advance(m) // at fix (agent-edit)
+	h.git.def.Tree = "edited"
+	h.store.failType, h.store.err = run.TypeTree, errors.New("treefail")
+	if _, err := m.Advance(ctx); err == nil || err.Error() != "treefail" {
+		t.Fatalf("agent-edit tree append: %v", err)
+	}
+	// loop gate append failure at the boundary (convergence does not stop): fail the gate append after the converge event
+	h = newHarness(t)
+	m = loopRun(t, h, 1, "sdlc-loop")
+	h.store.failType, h.store.err = run.TypeGate, errors.New("gatefail")
+	// the first gate at verify is all_fixed; make only the second gate append fail by counting
+	h.store.failType = ""
+	h.store.appends = 0
+	h.store.failAt = 6 // node_output, delta_applied, gate(all_fixed), converge, gate(bugs_remain)=5 … use a sweep instead
+	h.store.failAt = 0
+	total := 0
+	{
+		hb := newHarness(t)
+		hb.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+		allPresent(hb)
+		mb := hb.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars})
+		for _, step := range []func(*harness, *Machine) error{adv, rec("discover", findings(1)), adv, adv, adv, rec("fix", `{"commit":"`+shaFix+`","summary":"s"}`), adv, adv} {
+			if err := step(hb, mb); err != nil {
+				t.Fatal(err)
+			}
+		}
+		total = hb.store.appends
+	}
+	for n := 1; n <= total; n++ {
+		hh := newHarness(t)
+		hh.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+		allPresent(hh)
+		hh.store.failAt, hh.store.err = n, errors.New("appendfail")
+		mm, err := hh.init(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars})
+		got := err
+		if got == nil {
+			for _, step := range []func(*harness, *Machine) error{adv, rec("discover", findings(1)), adv, adv, adv, rec("fix", `{"commit":"`+shaFix+`","summary":"s"}`), adv, adv} {
+				if e := step(hh, mm); e != nil {
+					got = e
+					break
+				}
+			}
+		}
+		if got == nil || got.Error() != "appendfail" {
+			t.Fatalf("loop sweep #%d/%d: %v", n, total, got)
+		}
+	}
+	// Repair on a crafted log that repairs to a fold-invalid prefix
+	root := t.TempDir()
+	hj := newHarness(t)
+	hj.store = &countingStore{RunStore: run.NewJSONLStore(root, run.Options{})}
+	hj.deps.Store = hj.store
+	hj.deps.Sidecar = FSSidecar{Root: root}
+	mj := hj.mustInit(InitOptions{Workflow: "review-loop", Vars: sdlcVars})
+	var initJ run.InitData
+	_ = json.Unmarshal(hj.events(mj)[0].Data, &initJ)
+	initJ.RunID = "mrv-crafted-invalid-torn"
+	b := run.NewBuilder("mrv-crafted-invalid-torn")
+	b.Init(initJ)
+	b.Event(run.TypeTransition, run.TransitionData{From: "nowhere", To: "done", Gate: "x", Outcome: run.OutcomeClean, Head: shaHead})
+	dir := root + "/.metareview/runs/mrv-crafted-invalid-torn"
+	mkdir(t, dir)
+	writeBytes(t, dir+"/audit.jsonl", string(joinLines(b.Lines()))+`{"torn`)
+	_ = hj.deps.Sidecar.Write("mrv-crafted-invalid-torn", SidecarWorkflow, raw)
+	if _, err := Open(ctx, hj.deps, "mrv-crafted-invalid-torn", OpenOptions{Repair: true}); err == nil || !strings.Contains(err.Error(), "stamp") {
+		t.Fatalf("fold-invalid after repair: %v", err)
+	}
+	// malformed mock identity in a crafted init → ERR_MOCK_MISMATCH, not a panic
+	initJ.RunID = "mrv-crafted-bad-mock"
+	initJ.Mock = "nohash"
+	if _, err := hj.store.Create("mrv-crafted-bad-mock", run.Event{SchemaVersion: run.SchemaVersion, At: initJ.CreatedAt, Type: run.TypeInit, Data: run.MarshalCanonical(initJ)}); err != nil {
+		t.Fatal(err)
+	}
+	_ = hj.deps.Sidecar.Write("mrv-crafted-bad-mock", SidecarWorkflow, raw)
+	hj.reg.mock = true
+	if _, err := Open(ctx, hj.deps, "mrv-crafted-bad-mock", OpenOptions{}); !errs.Is(err, CodeMockMismatch) {
+		t.Fatalf("malformed mock: %v", err)
+	}
+	hj.reg.mock = false
+	// on_overflow interrupted by the parent context: nothing recorded, handler retried on resume
+	ho := newHarness(t)
+	wf := sdlcWith(t, ho, "ov-cancel.yaml", "  any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]\nrepo_mode: advisory",
+		"  any: [{max_iterations: 1}]\ncmds:\n  notify: {argv: [bash, -c, echo]}\non_overflow: notify\nrepo_mode: advisory")
+	_, sha, _ := workflow.ResolveCmds(mustResolve(t, ho, wf), "/repo", ho.deps.LookPath, ho.deps.FileHash)
+	allPresent(ho)
+	ho.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+	ho.runner.err = context.Canceled
+	mo := ho.mustInit(InitOptions{Workflow: wf, Vars: sdlcVars, AllowCustomCmds: sha})
+	ho.advance(mo)
+	ho.record(mo, "discover", findings(1))
+	ho.advance(mo)
+	ho.advance(mo)
+	ho.advance(mo)
+	ho.record(mo, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
+	ho.advance(mo)
+	cctx, ccancel := context.WithCancel(ctx)
+	ccancel()
+	ho.runner.audit = nil
+	if _, err := mo.Advance(cctx); !errors.Is(err, context.Canceled) {
+		t.Fatalf("interrupted handler: %v", err)
+	}
+	if countType(ho.events(mo), run.TypeOverflowHandler) != 0 || mo.View().Snapshot.OverflowHandled {
+		t.Fatal("interrupted handler must not be recorded")
+	}
+	// interrupted inside a cmd atom: returned, never a converge pseudo-gate
+	hc := newHarness(t)
+	wfc := sdlcWith(t, hc, "cv-cancel.yaml", "  any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]\nrepo_mode: advisory",
+		"  any: [{cmd: notify}]\ncmds:\n  notify: {argv: [bash, -c, echo]}\nrepo_mode: advisory")
+	_, shac, _ := workflow.ResolveCmds(mustResolve(t, hc, wfc), "/repo", hc.deps.LookPath, hc.deps.FileHash)
+	allPresent(hc)
+	hc.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+	hc.runner.err = context.Canceled
+	mc := hc.mustInit(InitOptions{Workflow: wfc, Vars: sdlcVars, AllowCustomCmds: shac})
+	hc.advance(mc)
+	hc.record(mc, "discover", findings(1))
+	hc.advance(mc)
+	hc.advance(mc)
+	hc.advance(mc)
+	hc.record(mc, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
+	hc.advance(mc)
+	cctx2, ccancel2 := context.WithCancel(ctx)
+	ccancel2()
+	hc.runner.audit = nil
+	if _, err := mc.Advance(cctx2); !errors.Is(err, context.Canceled) {
+		t.Fatalf("interrupted atom: %v", err)
+	}
+	if mc.View().Snapshot.Outcome != "" {
+		t.Fatal("interrupted atom must not fail the run")
+	}
+	// FS Read with bad args
+	if _, err := (FSSidecar{Root: root}).Read("bad id", "x"); errs.As(err).Field("reason") != "path" {
+		t.Fatal("read bad id")
+	}
+}



```

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

coverage gate run after commit 1d6284b (M4/M5/M6 complete):
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

[exited with code 0]

