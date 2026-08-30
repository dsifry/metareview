package machine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dsifry/metareview/internal/fsm/converge"
	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/gate"
	"github.com/dsifry/metareview/internal/fsm/run"
	"github.com/dsifry/metareview/internal/fsm/workflow"
	"github.com/dsifry/metareview/workflows"
)

const (
	shaBase = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	shaHead = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	shaFix  = "cccccccccccccccccccccccccccccccccccccccc"
)

// ---- kinds ----

type fakeKind struct {
	name    string
	info    workflow.KindInfo
	decode  func(json.RawMessage) (any, error)
	reduce  func(run.Snapshot, any) (run.Delta, error)
	instr   func(run.Snapshot, *workflow.Node, Diff, string) (Instructions, error)
	seenIns []Instructions
}

func (k *fakeKind) Name() string            { return k.name }
func (k *fakeKind) Info() workflow.KindInfo { return k.info }
func (k *fakeKind) Decode(raw json.RawMessage) (any, error) {
	if k.decode != nil {
		return k.decode(raw)
	}
	var d run.Delta
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		return nil, err
	}
	return d, nil
}
func (k *fakeKind) Reduce(s run.Snapshot, out any) (run.Delta, error) {
	if k.reduce != nil {
		return k.reduce(s, out)
	}
	return out.(run.Delta), nil
}
func (k *fakeKind) Instructions(s run.Snapshot, n *workflow.Node, d Diff, nonce string) (Instructions, error) {
	if k.instr != nil {
		return k.instr(s, n, d, nonce)
	}
	ins := Instructions{Text: "do " + n.Name + " nonce=" + nonce, Input: map[string]any{"diff": d.Text, "diff_truncated": d.Truncated, "iteration": s.Iteration}, Untrusted: []string{"diff"}, OutputSchema: json.RawMessage(`{}`)}
	k.seenIns = append(k.seenIns, ins)
	return ins, nil
}

type fakeExecutor struct {
	fn    func(ExecInput) (json.RawMessage, error)
	calls []ExecInput
}

func (e *fakeExecutor) Execute(_ context.Context, in ExecInput) (json.RawMessage, error) {
	e.calls = append(e.calls, in)
	return e.fn(in)
}

type fakeRegistry struct {
	kinds map[string]*fakeKind
	execs map[string]*fakeExecutor
	mock  bool
}

func (r *fakeRegistry) Kind(n string) (NodeKind, bool) {
	k, ok := r.kinds[n]
	return k, ok
}
func (r *fakeRegistry) Executor(n string) (Executor, bool) {
	e, ok := r.execs[n]
	return e, ok
}
func (r *fakeRegistry) Info() map[string]workflow.KindInfo {
	m := map[string]workflow.KindInfo{}
	for n, k := range r.kinds {
		m[n] = k.info
	}
	return m
}
func (r *fakeRegistry) Mock() bool { return r.mock }

// llmCall emits one llm_call through the audit closure.
func llmCall(in ExecInput, i int, tokens int64) error {
	d := run.LLMCallData{Kind: "match", Model: in.Node.Model, Effort: in.Node.Effort, Index: in.StartIndex + i, InputHash: "h", Verdict: json.RawMessage(`{"match":true}`), Confidence: 0.9, Tokens: run.TokenTotals{Input: tokens}}
	return in.Audit(run.Event{Type: run.TypeLLMCall, Data: run.MarshalCanonical(d)})
}

func newRegistry() *fakeRegistry {
	r := &fakeRegistry{kinds: map[string]*fakeKind{}, execs: map[string]*fakeExecutor{}}
	r.kinds["review-lenses"] = &fakeKind{name: "review-lenses", info: workflow.KindInfo{DefaultExec: "subagent", AllowedExec: []string{"inline", "subagent"}}}
	r.kinds["match-then-adjudicate"] = &fakeKind{name: "match-then-adjudicate", info: workflow.KindInfo{DefaultExec: "fork", AllowedExec: []string{"fork"}, NeedsJudge: true}}
	r.kinds["agent-edit"] = &fakeKind{name: "agent-edit", info: workflow.KindInfo{DefaultExec: "inline", AllowedExec: []string{"inline", "subagent"}}}
	r.kinds["still-present"] = &fakeKind{name: "still-present", info: workflow.KindInfo{DefaultExec: "fork", AllowedExec: []string{"fork"}, NeedsJudge: true}}
	// mutation-verify: deterministic, so NeedsJudge is false and pre-flight skips it.
	r.kinds["mutation-verify"] = &fakeKind{name: "mutation-verify", info: workflow.KindInfo{DefaultExec: "fork", AllowedExec: []string{"fork"}}}
	r.kinds["cmd"] = &fakeKind{name: "cmd", info: workflow.KindInfo{DefaultExec: "fork", AllowedExec: []string{"fork"}}}
	// agent-edit output is {commit, summary}; reduce to Commit
	r.kinds["agent-edit"].decode = func(raw json.RawMessage) (any, error) {
		var o struct {
			Commit  string `json:"commit"`
			Summary string `json:"summary"`
		}
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&o); err != nil {
			return nil, err
		}
		return o.Commit, nil
	}
	r.kinds["agent-edit"].reduce = func(_ run.Snapshot, out any) (run.Delta, error) { return run.Delta{Commit: out.(string)}, nil }
	// default executors: adjudicate confirms every finding after one llm_call each; verify says all fixed
	r.execs["match-then-adjudicate"] = &fakeExecutor{fn: func(in ExecInput) (json.RawMessage, error) {
		var bugs []run.Bug
		for i, f := range in.Snap.Findings {
			if err := llmCall(in, i, 10); err != nil {
				return nil, err
			}
			bugs = append(bugs, run.Bug{ID: run.BugID(f.IssueText), Desc: f.IssueText, Verdict: "real_but_ungold", Confidence: 0.9})
		}
		return json.RawMessage(run.MarshalCanonical(run.Delta{Confirmed: bugs})), nil
	}}
	r.execs["still-present"] = &fakeExecutor{fn: func(in ExecInput) (json.RawMessage, error) {
		var st []run.BugStatus
		for i, b := range in.Snap.AllFound {
			if err := llmCall(in, i, 5); err != nil {
				return nil, err
			}
			st = append(st, run.BugStatus{ID: b.ID, StillPresent: false, Confidence: 1})
		}
		return json.RawMessage(run.MarshalCanonical(run.Delta{Status: st})), nil
	}}
	r.execs["cmd"] = &fakeExecutor{fn: func(in ExecInput) (json.RawMessage, error) { return json.RawMessage(`{}`), nil }}
	// mutation-verify with nothing to prove reports no findings, which is what a fix that
	// declared no pins produces.
	r.execs["mutation-verify"] = &fakeExecutor{fn: func(ExecInput) (json.RawMessage, error) { return json.RawMessage(`{"findings":[]}`), nil }}
	return r
}

// ---- git ----

// gitFakes returns the same fake for every dir unless overridden.
type gitFakes struct {
	byDir map[string]gate.Git
	def   *gate.Fake
}

func (g *gitFakes) get(dir string) gate.Git {
	if f, ok := g.byDir[dir]; ok {
		return f
	}
	return g.def
}

// failingGit fails exactly one method with err.
type failingGit struct {
	gate.Git
	at  string
	err error
}

func (f *failingGit) Head(ctx context.Context) (string, error) {
	if f.at == "Head" {
		return "", f.err
	}
	return f.Git.Head(ctx)
}
func (f *failingGit) RevParse(ctx context.Context, r string) (string, error) {
	if f.at == "RevParse" {
		return "", f.err
	}
	return f.Git.RevParse(ctx, r)
}
func (f *failingGit) Status(ctx context.Context) (bool, string, error) {
	if f.at == "Status" {
		return false, "", f.err
	}
	return f.Git.Status(ctx)
}
func (f *failingGit) WorkTree(ctx context.Context) (string, error) {
	if f.at == "WorkTree" {
		return "", f.err
	}
	return f.Git.WorkTree(ctx)
}
func (f *failingGit) Diff(ctx context.Context, a, b string, n int) (string, bool, error) {
	if f.at == "Diff" {
		return "", false, f.err
	}
	return f.Git.Diff(ctx, a, b, n)
}
func (f *failingGit) CommonDir(ctx context.Context) (string, error) {
	if f.at == "CommonDir" {
		return "", f.err
	}
	return f.Git.CommonDir(ctx)
}

// ---- store wrappers ----

// countingStore fails the Nth append (1-based) or a named method.
type countingStore struct {
	run.RunStore
	mu          sync.Mutex
	appends     int
	failAt      int
	failType    string
	failOp      string
	events      int
	failEvAt    int // fail the Nth EventsWithLines call
	err         error
	failLockRun string // "child": fail Lock for any run other than the first locked one
	firstLock   string
	maxEvents   int  // overrides MaxEvents() when non-zero (the fork's in-memory count check)
	torn        bool // report a torn tail on every read (the memory store is never torn)
}

func (c *countingStore) MaxEvents() int {
	if c.maxEvents != 0 {
		return c.maxEvents
	}
	return c.RunStore.MaxEvents()
}
func (c *countingStore) Create(id string, first run.Event) (run.FoldState, error) {
	if c.failOp == "Create" {
		return run.FoldState{}, c.err
	}
	return c.RunStore.Create(id, first)
}

func (c *countingStore) Append(id string, st run.FoldState, ev run.Event) (run.FoldState, error) {
	c.mu.Lock()
	c.appends++
	n := c.appends
	c.mu.Unlock()
	if (c.failAt != 0 && n == c.failAt) || (c.failType != "" && ev.Type == c.failType) {
		return run.FoldState{}, c.err
	}
	return c.RunStore.Append(id, st, ev)
}
func (c *countingStore) Lock(id string) (func(), error) {
	if c.failOp == "Lock" {
		return nil, c.err
	}
	if c.firstLock == "" {
		c.firstLock = id
	}
	if c.failLockRun == "child" && id != c.firstLock {
		return nil, c.err
	}
	return c.RunStore.Lock(id)
}
func (c *countingStore) EventsWithLines(id string) (run.Log, [][]byte, error) {
	c.mu.Lock()
	c.events++
	n := c.events
	c.mu.Unlock()
	if c.failOp == "Events" || (c.failEvAt != 0 && n == c.failEvAt) {
		return run.Log{}, nil, c.err
	}
	log, lines, err := c.RunStore.EventsWithLines(id)
	if c.torn && err == nil {
		log.Torn = &run.TornTail{Offset: 1, Bytes: []byte("{")}
	}
	return log, lines, err
}
func (c *countingStore) RepairTail(id string) error {
	if c.failOp == "Repair" {
		return c.err
	}
	return c.RunStore.RepairTail(id)
}

// ---- runner ----

type fakeRunner struct {
	calls    []string
	stdins   [][]byte
	res      converge.CmdResult
	err      error
	audit    func(run.Event) error
	ordinal  func(string) int
	ordinals []int
}

func (f *fakeRunner) Run(_ context.Context, name string, stdin []byte) (converge.CmdResult, error) {
	f.calls = append(f.calls, name)
	f.stdins = append(f.stdins, stdin)
	if f.ordinal != nil {
		f.ordinals = append(f.ordinals, f.ordinal(name))
	}
	if f.audit != nil {
		d := run.CmdCallData{Name: name, Argv: []string{"/bin/true"}, InputHash: "x", ExitCode: f.res.ExitCode}
		if err := f.audit(run.Event{Type: run.TypeCmdCall, Data: run.MarshalCanonical(d)}); err != nil {
			return converge.CmdResult{}, err
		}
	}
	return f.res, f.err
}

func (f *fakeRunner) Call(ctx context.Context, name string, stdin []byte, out any) error {
	res, err := f.Run(ctx, name, stdin)
	if err != nil {
		return err
	}
	return json.Unmarshal(res.Stdout, out)
}

// ---- harness ----

type harness struct {
	t        *testing.T
	store    *countingStore
	sidecar  *MemSidecar
	reg      *fakeRegistry
	git      *gitFakes
	runner   *fakeRunner
	clock    int64
	files    map[string][]byte
	nonces   int
	mockHash map[string]string
	terminal []View
	termErr  error
	deps     Deps
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{t: t, store: &countingStore{RunStore: run.NewMemStore(run.Options{})}, sidecar: &MemSidecar{}, reg: newRegistry(), files: map[string][]byte{}, mockHash: map[string]string{}}
	h.git = &gitFakes{byDir: map[string]gate.Git{}, def: &gate.Fake{HeadSHA: shaHead, Refs: map[string]string{"main": shaBase}, Common: "/repo/.git", Clean: true, Tree: "t1", Diffs: map[string]string{shaBase + ".." + shaHead: "DIFF", shaHead + ".." + shaHead: "DIFF"}}}
	h.runner = &fakeRunner{res: converge.CmdResult{Stdout: []byte(`{"stop": false, "reason": ""}`)}}
	h.deps = Deps{
		Store: h.store, Sidecar: h.sidecar, Kinds: h.reg,
		Git: func(dir string) gate.Git { return h.git.get(dir) },
		Runner: func(d RunnerDeps) converge.Caller {
			h.runner.audit = d.Audit
			h.runner.ordinal = d.CmdCalls
			return h.runner
		},
		Clock: func() run.Time {
			h.clock++
			return run.Time{Time: time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC).Add(time.Duration(h.clock) * time.Second)}
		},
		LookPath: func(n string) (string, error) {
			if n == "bash" {
				return "/bin/bash", nil
			}
			return "", errors.New("not found")
		},
		FileHash: func(p string) (string, error) {
			if p == "/bin/bash" {
				return "hb", nil
			}
			return "", errors.New("no such file")
		},
		Workflows: workflows.Read,
		ReadFile: func(p string) ([]byte, error) {
			b, ok := h.files[p]
			if !ok {
				return nil, errors.New("open " + p + ": no such file")
			}
			return b, nil
		},
		Nonce: func() string { h.nonces++; return fmt.Sprintf("n%d", h.nonces) },
		MockLoad: func(dir string) (string, error) {
			if hsh, ok := h.mockHash[dir]; ok {
				return hsh, nil
			}
			return "", errors.New("no scenario at " + dir)
		},
		Terminal: func(_ context.Context, v View) error { h.terminal = append(h.terminal, v); return h.termErr },
	}
	return h
}

func (h *harness) init(o InitOptions) (*Machine, error) {
	if o.WorkDir == "" {
		o.WorkDir = "/repo"
	}
	if o.RepoRoot == "" {
		o.RepoRoot = "/repo"
	}
	return Init(context.Background(), h.deps, o)
}

func (h *harness) mustInit(o InitOptions) *Machine {
	h.t.Helper()
	m, err := h.init(o)
	if err != nil {
		h.t.Fatalf("init: %v", err)
	}
	return m
}

func (h *harness) events(m *Machine) []run.Event {
	log, err := h.store.Events(m.runID)
	if err != nil {
		h.t.Fatal(err)
	}
	return log.Events
}

func (h *harness) types(m *Machine) []string {
	var out []string
	for _, ev := range h.events(m) {
		out = append(out, ev.Type)
	}
	return out
}

func (h *harness) record(m *Machine, node string, data string) RecordResult {
	h.t.Helper()
	r, err := m.Record(context.Background(), RecordOptions{Kind: RecordNodeOutput, Node: node, Data: json.RawMessage(data)})
	if err != nil {
		h.t.Fatalf("record %s: %v", node, err)
	}
	return r
}

func (h *harness) advance(m *Machine) AdvanceResult {
	h.t.Helper()
	r, err := m.Advance(context.Background())
	if err != nil {
		h.t.Fatalf("advance: %v", err)
	}
	return r
}

// wantCode asserts that err carries code. Callers that go on to inspect the
// error use wantCodeE; the bare assertion returns nothing so that an ignored
// return cannot look like a discarded failure.
func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	_ = wantCodeE(t, err, code)
}

func wantCodeE(t *testing.T, err error, code string) *errs.Error {
	t.Helper()
	if !errs.Is(err, code) {
		t.Fatalf("want %s, got %v", code, err)
	}
	return errs.As(err)
}

func findings(n int) string {
	var fs []run.Finding
	for i := 0; i < n; i++ {
		fs = append(fs, run.Finding{IssueText: fmt.Sprintf("bug %d", i), File: "f.go", Line: i + 1})
	}
	return string(run.MarshalCanonical(run.Delta{Findings: fs}))
}

var sdlcVars = map[string]string{"JUDGE": "gpt-5.2", "JUDGE_EFFORT": "medium"}
