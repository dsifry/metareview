# metareview task-done context

Run ID: `mrv-20260827-082412402343000-task-done-m1-m6-fsm-packages-a99c72f1`

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

- Base: `10980df410f0b1ccc7876b5d90f4acbf15d1602f`
- Head: `92ba42e55183b4d5ee522381f9db5eafd5f2d68f`
- Branch: ``
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `62858`
- Filtered diff bytes: `62858`
- Risk level: `none`



## Review Manifest

- Manifest verdict: `NEEDS_REVISION`
- Source manifest hash: `18fb3926baadcb9b`
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- docs/tasks/m1-m6-fsm-packages.md
- go.mod
- go.sum
- internal/fsm/converge/converge.go
- internal/fsm/converge/converge_test.go
- internal/fsm/errs/errs.go
- internal/fsm/errs/errs_test.go
- internal/fsm/gate/fake.go
- internal/fsm/gate/gate_test.go
- internal/fsm/gate/gates.go
- internal/fsm/gate/git.go
- internal/fsm/run/fold.go
- internal/fsm/run/fold_test.go
- internal/fsm/run/types.go
- internal/fsm/run/types_test.go

### Shards
- shard-01: go.mod, internal/fsm/converge/converge.go, internal/fsm/converge/converge_test.go, internal/fsm/errs/errs.go, internal/fsm/errs/errs_test.go, internal/fsm/gate/fake.go, internal/fsm/gate/gate_test.go, internal/fsm/gate/gates.go, internal/fsm/gate/git.go, internal/fsm/run/fold.go, internal/fsm/run/types.go
- shard-02: docs/tasks/m1-m6-fsm-packages.md, go.sum, internal/fsm/run/fold_test.go, internal/fsm/run/types_test.go

### Manifest Blockers
- missing cross-shard result
- missing shard result for shard-01
- missing shard result for shard-02

## Changed Files

- go.mod
- go.sum
- internal/fsm/converge/converge.go
- internal/fsm/converge/converge_test.go
- internal/fsm/errs/errs.go
- internal/fsm/errs/errs_test.go
- internal/fsm/gate/fake.go
- internal/fsm/gate/gate_test.go
- internal/fsm/gate/gates.go
- internal/fsm/gate/git.go
- internal/fsm/run/fold.go
- internal/fsm/run/fold_test.go
- internal/fsm/run/types.go
- internal/fsm/run/types_test.go
- docs/tasks/m1-m6-fsm-packages.md

## Diff

```diff
diff --git a/go.mod b/go.mod
index a98dbdc..e9838d5 100644
--- a/go.mod
+++ b/go.mod
@@ -1,3 +1,5 @@
 module github.com/dsifry/metareview
 
 go 1.26
+
+require gopkg.in/yaml.v3 v3.0.1 // indirect
diff --git a/go.sum b/go.sum
new file mode 100644
index 0000000..4bc0337
--- /dev/null
+++ b/go.sum
@@ -0,0 +1,3 @@
+gopkg.in/check.v1 v0.0.0-20161208181325-20d25e280405/go.mod h1:Co6ibVJAznAaIkqp8huTwlJQCZ016jof/cbN4VW5Yz0=
+gopkg.in/yaml.v3 v3.0.1 h1:fxVm/GzAzEWqLHuvctI91KS9hhNmmWOoWu0XTYJS7CA=
+gopkg.in/yaml.v3 v3.0.1/go.mod h1:K4uyk7z7BCEPqu6E+C64Yfv1cQ7kz7rIZviUmN+EgEM=
diff --git a/internal/fsm/converge/converge.go b/internal/fsm/converge/converge.go
new file mode 100644
index 0000000..d3f881e
--- /dev/null
+++ b/internal/fsm/converge/converge.go
@@ -0,0 +1,315 @@
+// Package converge implements the convergence predicate tree evaluated at a
+// loop boundary: atoms (all_fixed, no_fixation_progress, max_iterations,
+// budget, cmd) composed with any/all/not.
+//
+// Deterministic workflow structure; the cmd atom's result is whatever the
+// sanctioned command says — auditable, never "deterministic results".
+package converge
+
+import (
+	"bytes"
+	"context"
+	"crypto/sha256"
+	"encoding/hex"
+	"encoding/json"
+	"fmt"
+	"strings"
+	"time"
+
+	"gopkg.in/yaml.v3"
+
+	"github.com/dsifry/metareview/internal/fsm/errs"
+	"github.com/dsifry/metareview/internal/fsm/run"
+)
+
+// AllFixed reports whether every bug found so far is fixed. Nothing found is
+// NOT fixed (review-loop expresses "nothing to fix" with findings_empty).
+func AllFixed(s run.Snapshot) bool { return len(s.AllFound) > 0 && s.Unfixed == 0 }
+
+// CmdResult is what a sanctioned command produced.
+type CmdResult struct {
+	Stdout, Stderr []byte
+	ExitCode       int
+	Duration       time.Duration
+}
+
+// Runner executes a declared command by name (argv is pinned at consent time
+// and never supplied here). Implemented by cmdexec.Guarded, which also audits.
+type Runner interface {
+	Run(ctx context.Context, name string, stdin []byte) (CmdResult, error)
+}
+
+// Result is what an evaluation decided. Atom/Class name the deciding atom
+// (for `any`, the first child that fired; for `all`, the joined names and the
+// first child's class).
+type Result struct {
+	Stop   bool
+	Atom   string
+	Class  run.Outcome
+	Reason string
+}
+
+// Predicate is one node of the convergence tree.
+type Predicate interface {
+	Name() string
+	Class() run.Outcome
+	Evaluate(ctx context.Context, s run.Snapshot) (Result, error)
+}
+
+// Error codes produced by this package.
+const (
+	CodeBadConvergence   = "ERR_BAD_CONVERGENCE"
+	CodeCmdOutputInvalid = "ERR_CMD_OUTPUT_INVALID"
+	CodeCmdFailed        = "ERR_CMD_FAILED"
+)
+
+// Validate checks the structure of a convergence tree without binding it.
+// cmdNames lists the declared command names a cmd atom may reference.
+func Validate(node *yaml.Node, cmdNames []string) error {
+	_, err := parse(node, nil, cmdNames)
+	return err
+}
+
+// Parse validates and binds a convergence tree; cmd atoms call runner.
+func Parse(node *yaml.Node, runner Runner) (Predicate, error) {
+	return parse(node, runner, nil)
+}
+
+func bad(detail string) error {
+	return errs.E(CodeBadConvergence, detail, "detail", detail)
+}
+
+// parse walks the tree. When cmdNames is nil every cmd name is accepted
+// (Parse trusts workflow.Parse's earlier Validate); otherwise names must be
+// declared.
+func parse(node *yaml.Node, runner Runner, cmdNames []string) (Predicate, error) {
+	if node == nil || node.Kind == 0 {
+		return nil, bad("empty convergence")
+	}
+	if node.Kind == yaml.DocumentNode {
+		if len(node.Content) != 1 {
+			return nil, bad("empty convergence")
+		}
+		return parse(node.Content[0], runner, cmdNames)
+	}
+	if node.Kind == yaml.ScalarNode {
+		// Bare form: `no_fixation_progress` / `all_fixed` (the shipped YAMLs use it).
+		switch node.Value {
+		case "all_fixed":
+			return allFixed{}, nil
+		case "no_fixation_progress":
+			return noProgress{}, nil
+		}
+		return nil, bad(fmt.Sprintf("line %d: unknown atom %q", node.Line, node.Value))
+	}
+	if node.Kind != yaml.MappingNode || len(node.Content) != 2 {
+		return nil, bad(fmt.Sprintf("line %d: an atom is a mapping with exactly one key", node.Line))
+	}
+	key, val := node.Content[0].Value, node.Content[1]
+	switch key {
+	case "all_fixed", "no_fixation_progress":
+		var on bool
+		if err := val.Decode(&on); err != nil || !on {
+			return nil, bad(fmt.Sprintf("line %d: %s must be true", node.Line, key))
+		}
+		if key == "all_fixed" {
+			return allFixed{}, nil
+		}
+		return noProgress{}, nil
+	case "max_iterations":
+		var n int
+		if err := val.Decode(&n); err != nil || n <= 0 {
+			return nil, bad(fmt.Sprintf("line %d: max_iterations must be a positive integer", node.Line))
+		}
+		return maxIter{n: n}, nil
+	case "budget":
+		var tokens int64
+		if val.Kind != yaml.MappingNode || len(val.Content) != 2 || val.Content[0].Value != "tokens" || val.Content[1].Decode(&tokens) != nil || tokens <= 0 {
+			return nil, bad(fmt.Sprintf("line %d: budget must be {tokens: positive integer}", node.Line))
+		}
+		return budget{tokens: tokens}, nil
+	case "cmd":
+		var name string
+		if err := val.Decode(&name); err != nil || name == "" {
+			return nil, bad(fmt.Sprintf("line %d: cmd must name a declared command", node.Line))
+		}
+		if cmdNames != nil && !contains(cmdNames, name) {
+			return nil, bad(fmt.Sprintf("line %d: unknown cmd %q", node.Line, name))
+		}
+		return &cmdAtom{name: name, runner: runner}, nil
+	case "any", "all":
+		if val.Kind != yaml.SequenceNode || len(val.Content) == 0 {
+			return nil, bad(fmt.Sprintf("line %d: %s must be a non-empty list", node.Line, key))
+		}
+		kids := make([]Predicate, 0, len(val.Content))
+		for _, c := range val.Content {
+			p, err := parse(c, runner, cmdNames)
+			if err != nil {
+				return nil, err
+			}
+			kids = append(kids, p)
+		}
+		return &compound{op: key, kids: kids}, nil
+	case "not":
+		inner, err := parse(val, runner, cmdNames)
+		if err != nil {
+			return nil, err
+		}
+		return &not{inner: inner}, nil
+	}
+	return nil, bad(fmt.Sprintf("line %d: unknown atom %q", node.Line, key))
+}
+
+func contains(xs []string, s string) bool {
+	for _, x := range xs {
+		if x == s {
+			return true
+		}
+	}
+	return false
+}
+
+type allFixed struct{}
+
+func (allFixed) Name() string       { return "all_fixed" }
+func (allFixed) Class() run.Outcome { return run.OutcomeFixed }
+func (a allFixed) Evaluate(_ context.Context, s run.Snapshot) (Result, error) {
+	return decide(a, AllFixed(s), "all bugs fixed"), nil
+}
+
+// decide builds a Result for a leaf atom.
+func decide(p Predicate, stop bool, reason string) Result {
+	r := Result{Stop: stop, Atom: p.Name(), Class: p.Class()}
+	if stop {
+		r.Reason = reason
+	}
+	return r
+}
+
+type noProgress struct{}
+
+func (noProgress) Name() string       { return "no_fixation_progress" }
+func (noProgress) Class() run.Outcome { return run.OutcomeStalled }
+func (n noProgress) Evaluate(_ context.Context, s run.Snapshot) (Result, error) {
+	stop := s.PrevUnfixed != nil && s.Unfixed >= *s.PrevUnfixed
+	reason := ""
+	if stop {
+		reason = fmt.Sprintf("unfixed %d >= previous %d", s.Unfixed, *s.PrevUnfixed)
+	}
+	return decide(n, stop, reason), nil
+}
+
+type maxIter struct{ n int }
+
+func (maxIter) Name() string       { return "max_iterations" }
+func (maxIter) Class() run.Outcome { return run.OutcomeOverflow }
+func (m maxIter) Evaluate(_ context.Context, s run.Snapshot) (Result, error) {
+	return decide(m, s.Iteration+1 >= m.n, fmt.Sprintf("iteration %d reached max_iterations %d", s.Iteration, m.n)), nil
+}
+
+type budget struct{ tokens int64 }
+
+func (budget) Name() string       { return "budget" }
+func (budget) Class() run.Outcome { return run.OutcomeOverflow }
+func (b budget) Evaluate(_ context.Context, s run.Snapshot) (Result, error) {
+	t := s.Tokens.Total()
+	return decide(b, t >= b.tokens, fmt.Sprintf("tokens %d >= budget %d", t, b.tokens)), nil
+}
+
+type cmdAtom struct {
+	name   string
+	runner Runner
+}
+
+func (c *cmdAtom) Name() string       { return "cmd:" + c.name }
+func (c *cmdAtom) Class() run.Outcome { return run.OutcomeCustom }
+func (c *cmdAtom) Evaluate(ctx context.Context, s run.Snapshot) (Result, error) {
+	res, err := c.runner.Run(ctx, c.name, Payload(s))
+	if err != nil {
+		return Result{}, err
+	}
+	if res.ExitCode != 0 {
+		return Result{}, errs.E(CodeCmdFailed, fmt.Sprintf("cmd %s exited %d", c.name, res.ExitCode), "name", c.name, "exit", fmt.Sprint(res.ExitCode))
+	}
+	var out struct {
+		Stop   bool   `json:"stop"`
+		Reason string `json:"reason"`
+	}
+	dec := json.NewDecoder(bytes.NewReader(res.Stdout))
+	dec.DisallowUnknownFields()
+	if err := dec.Decode(&out); err != nil {
+		return Result{}, errs.E(CodeCmdOutputInvalid, fmt.Sprintf("cmd %s stdout is not {stop, reason}: %v", c.name, err), "name", c.name)
+	}
+	return decide(c, out.Stop, out.Reason), nil
+}
+
+// Payload is the JSON handed to sanctioned commands: the snapshot with var
+// values replaced by their sha256 (commands are consented to run, not to
+// receive credentials).
+func Payload(s run.Snapshot) []byte {
+	c := s.Clone()
+	for k, v := range c.Vars {
+		sum := sha256.Sum256([]byte(v))
+		c.Vars[k] = "sha256:" + hex.EncodeToString(sum[:])
+	}
+	return run.MarshalCanonical(c)
+}
+
+type compound struct {
+	op   string
+	kids []Predicate
+}
+
+func (c *compound) Name() string {
+	names := make([]string, len(c.kids))
+	for i, k := range c.kids {
+		names[i] = k.Name()
+	}
+	return c.op + "(" + strings.Join(names, "+") + ")"
+}
+
+func (c *compound) Class() run.Outcome { return c.kids[0].Class() }
+
+// Evaluate: any stops with the first firing child's Result; all stops only
+// when every child fires (Atom = names joined by "+", Class = the first's).
+// Errors abort evaluation at the first failing child.
+func (c *compound) Evaluate(ctx context.Context, s run.Snapshot) (Result, error) {
+	var names, reasons []string
+	for _, k := range c.kids {
+		r, err := k.Evaluate(ctx, s)
+		if err != nil {
+			return Result{}, err
+		}
+		if c.op == "any" {
+			if r.Stop {
+				return r, nil
+			}
+			continue
+		}
+		if !r.Stop {
+			return Result{Atom: c.Name(), Class: c.Class()}, nil
+		}
+		names = append(names, r.Atom)
+		reasons = append(reasons, r.Reason)
+	}
+	if c.op == "any" {
+		return Result{Atom: c.Name(), Class: c.Class()}, nil
+	}
+	return Result{Stop: true, Atom: strings.Join(names, "+"), Class: c.Class(), Reason: strings.Join(reasons, "; ")}, nil
+}
+
+type not struct{ inner Predicate }
+
+func (n *not) Name() string       { return "not(" + n.inner.Name() + ")" }
+func (n *not) Class() run.Outcome { return n.inner.Class() }
+func (n *not) Evaluate(ctx context.Context, s run.Snapshot) (Result, error) {
+	r, err := n.inner.Evaluate(ctx, s)
+	if err != nil {
+		return Result{}, err
+	}
+	out := Result{Stop: !r.Stop, Atom: n.Name(), Class: n.Class()}
+	if out.Stop {
+		out.Reason = "inner predicate did not fire"
+	}
+	return out, nil
+}
diff --git a/internal/fsm/converge/converge_test.go b/internal/fsm/converge/converge_test.go
new file mode 100644
index 0000000..73e28ac
--- /dev/null
+++ b/internal/fsm/converge/converge_test.go
@@ -0,0 +1,271 @@
+package converge
+
+import (
+	"context"
+	"crypto/sha256"
+	"encoding/hex"
+	"encoding/json"
+	"errors"
+	"strings"
+	"testing"
+
+	"gopkg.in/yaml.v3"
+
+	"github.com/dsifry/metareview/internal/fsm/errs"
+	"github.com/dsifry/metareview/internal/fsm/run"
+)
+
+func node(t *testing.T, src string) *yaml.Node {
+	t.Helper()
+	var n yaml.Node
+	if err := yaml.Unmarshal([]byte(src), &n); err != nil {
+		t.Fatal(err)
+	}
+	return &n
+}
+
+type fakeRunner struct {
+	calls  []string
+	stdins [][]byte
+	res    CmdResult
+	err    error
+}
+
+func (f *fakeRunner) Run(_ context.Context, name string, stdin []byte) (CmdResult, error) {
+	f.calls = append(f.calls, name)
+	f.stdins = append(f.stdins, stdin)
+	return f.res, f.err
+}
+
+func intp(i int) *int { return &i }
+
+func snap(iter int, unfixed int, prev *int, tokens int64, found int) run.Snapshot {
+	s := run.Snapshot{Iteration: iter, Unfixed: unfixed, PrevUnfixed: prev, Tokens: run.TokenTotals{Input: tokens}}
+	for i := 0; i < found; i++ {
+		s.AllFound = append(s.AllFound, run.Bug{ID: strings.Repeat("a", 12), Desc: "d"})
+	}
+	return s
+}
+
+func TestC1AllFixed(t *testing.T) {
+	if AllFixed(snap(0, 0, nil, 0, 0)) {
+		t.Fatal("empty AllFound must not be fixed")
+	}
+	if !AllFixed(snap(0, 0, nil, 0, 1)) {
+		t.Fatal("found + unfixed 0 → fixed")
+	}
+	if AllFixed(snap(0, 1, nil, 0, 1)) {
+		t.Fatal("unfixed 1 → not fixed")
+	}
+}
+
+func TestC2Atoms(t *testing.T) {
+	ctx := context.Background()
+	cases := []struct {
+		name string
+		yaml string
+		s    run.Snapshot
+		stop bool
+		atom string
+		cls  run.Outcome
+	}{
+		{"all_fixed-bare-true", "all_fixed", snap(0, 0, nil, 0, 2), true, "all_fixed", run.OutcomeFixed},
+		{"all_fixed-map-false", "{all_fixed: true}", snap(0, 1, nil, 0, 2), false, "all_fixed", run.OutcomeFixed},
+		{"nfp-nil-prev", "no_fixation_progress", snap(1, 5, nil, 0, 5), false, "no_fixation_progress", run.OutcomeStalled},
+		{"nfp-equal", "{no_fixation_progress: true}", snap(1, 5, intp(5), 0, 5), true, "no_fixation_progress", run.OutcomeStalled},
+		{"nfp-less", "no_fixation_progress", snap(1, 4, intp(5), 0, 5), false, "no_fixation_progress", run.OutcomeStalled},
+		{"nfp-more", "no_fixation_progress", snap(1, 6, intp(5), 0, 6), true, "no_fixation_progress", run.OutcomeStalled},
+		{"max-iter-3", "{max_iterations: 5}", snap(3, 1, nil, 0, 1), false, "max_iterations", run.OutcomeOverflow},
+		{"max-iter-4", "{max_iterations: 5}", snap(4, 1, nil, 0, 1), true, "max_iterations", run.OutcomeOverflow},
+		{"budget-under", "{budget: {tokens: 100}}", snap(0, 1, nil, 99, 1), false, "budget", run.OutcomeOverflow},
+		{"budget-at", "{budget: {tokens: 100}}", snap(0, 1, nil, 100, 1), true, "budget", run.OutcomeOverflow},
+	}
+	for _, c := range cases {
+		p, err := Parse(node(t, c.yaml), nil)
+		if err != nil {
+			t.Fatalf("%s: %v", c.name, err)
+		}
+		r, err := p.Evaluate(ctx, c.s)
+		if err != nil || r.Stop != c.stop || r.Atom != c.atom || r.Class != c.cls || (r.Stop && r.Reason == "") || (!r.Stop && r.Reason != "") {
+			t.Errorf("%s: got %+v err=%v", c.name, r, err)
+		}
+		if p.Name() != c.atom || p.Class() != c.cls {
+			t.Errorf("%s: Name/Class", c.name)
+		}
+	}
+}
+
+func TestC2CmdAtom(t *testing.T) {
+	ctx := context.Background()
+	s := snap(2, 1, nil, 7, 1)
+	s.Vars = map[string]string{"JUDGE": "secret-model"}
+	fr := &fakeRunner{res: CmdResult{Stdout: []byte(`{"stop": true, "reason": "plateau"}`)}}
+	p, err := Parse(node(t, "{cmd: notify}"), fr)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if p.Name() != "cmd:notify" || p.Class() != run.OutcomeCustom {
+		t.Fatal("cmd atom name/class")
+	}
+	r, err := p.Evaluate(ctx, s)
+	if err != nil || !r.Stop || r.Reason != "plateau" || r.Atom != "cmd:notify" || r.Class != run.OutcomeCustom {
+		t.Fatalf("got %+v %v", r, err)
+	}
+	if fr.calls[0] != "notify" {
+		t.Fatal("name passed")
+	}
+	// stdin is the redacted payload: vars hashed, never the raw value.
+	var got run.Snapshot
+	if err := json.Unmarshal(fr.stdins[0], &got); err != nil {
+		t.Fatal(err)
+	}
+	sum := sha256.Sum256([]byte("secret-model"))
+	if got.Vars["JUDGE"] != "sha256:"+hex.EncodeToString(sum[:]) || strings.Contains(string(fr.stdins[0]), "secret-model") {
+		t.Fatalf("payload not redacted: %s", fr.stdins[0])
+	}
+	if got.Iteration != 2 || got.Tokens.Input != 7 {
+		t.Fatal("payload carries the snapshot")
+	}
+	if s.Vars["JUDGE"] != "secret-model" {
+		t.Fatal("Payload must not mutate the snapshot")
+	}
+
+	// stop:false path
+	fr.res = CmdResult{Stdout: []byte(`{"stop": false, "reason": ""}`)}
+	if r, err := p.Evaluate(ctx, s); err != nil || r.Stop || r.Reason != "" {
+		t.Fatalf("stop false: %+v %v", r, err)
+	}
+	// invalid outputs
+	for _, out := range []string{`{"stop": "yes"}`, `not json`, `{"stop": true, "extra": 1}`, ``} {
+		fr.res = CmdResult{Stdout: []byte(out)}
+		_, err := p.Evaluate(ctx, s)
+		if !errs.Is(err, CodeCmdOutputInvalid) {
+			t.Errorf("%q: %v", out, err)
+		}
+	}
+	fr.res = CmdResult{Stdout: []byte(`{"stop": true, "reason": "x"}`), ExitCode: 3}
+	if _, err := p.Evaluate(ctx, s); !errs.Is(err, CodeCmdFailed) || errs.As(err).Field("exit") != "3" {
+		t.Fatalf("exit: %v", err)
+	}
+	boom := errors.New("boom")
+	fr.err = boom
+	if _, err := p.Evaluate(ctx, s); !errors.Is(err, boom) {
+		t.Fatalf("runner error must propagate: %v", err)
+	}
+}
+
+func TestC3Compose(t *testing.T) {
+	ctx := context.Background()
+	fired := snap(4, 5, intp(5), 0, 5) // nfp fires, max_iterations 5 fires, budget doesn't
+	quiet := snap(0, 1, nil, 0, 1)
+
+	anyP, err := Parse(node(t, "any: [{budget: {tokens: 1000}}, no_fixation_progress, {max_iterations: 5}]"), nil)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if anyP.Name() != "any(budget+no_fixation_progress+max_iterations)" || anyP.Class() != run.OutcomeOverflow {
+		t.Fatalf("any name/class: %s %s", anyP.Name(), anyP.Class())
+	}
+	r, _ := anyP.Evaluate(ctx, fired)
+	if !r.Stop || r.Atom != "no_fixation_progress" || r.Class != run.OutcomeStalled {
+		t.Fatalf("any must report the first firing child: %+v", r)
+	}
+	r, _ = anyP.Evaluate(ctx, quiet)
+	if r.Stop || r.Atom != anyP.Name() || r.Class != run.OutcomeOverflow || r.Reason != "" {
+		t.Fatalf("any quiet: %+v", r)
+	}
+
+	allP, err := Parse(node(t, "all: [no_fixation_progress, {max_iterations: 5}]"), nil)
+	if err != nil {
+		t.Fatal(err)
+	}
+	r, _ = allP.Evaluate(ctx, fired)
+	if !r.Stop || r.Atom != "no_fixation_progress+max_iterations" || r.Class != run.OutcomeStalled || !strings.Contains(r.Reason, "; ") {
+		t.Fatalf("all fired: %+v", r)
+	}
+	r, _ = allP.Evaluate(ctx, snap(4, 4, intp(5), 0, 5)) // nfp quiet, max fires
+	if r.Stop || r.Atom != allP.Name() || r.Reason != "" {
+		t.Fatalf("all partial: %+v", r)
+	}
+
+	notP, err := Parse(node(t, "not: {budget: {tokens: 10}}"), nil)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if notP.Name() != "not(budget)" || notP.Class() != run.OutcomeOverflow {
+		t.Fatal("not name/class")
+	}
+	r, _ = notP.Evaluate(ctx, quiet)
+	if !r.Stop || r.Atom != "not(budget)" || r.Reason == "" {
+		t.Fatalf("not inverts: %+v", r)
+	}
+	r, _ = notP.Evaluate(ctx, snap(0, 1, nil, 10, 1))
+	if r.Stop || r.Reason != "" {
+		t.Fatalf("not inverts (2): %+v", r)
+	}
+
+	// Error abort: the failing cmd atom stops evaluation; later atoms are not visited.
+	boom := errors.New("boom")
+	fr := &fakeRunner{err: boom}
+	for _, src := range []string{"any: [{cmd: c}, {cmd: d}]", "all: [{cmd: c}, {cmd: d}]", "not: {cmd: c}"} {
+		fr.calls = nil
+		p, err := Parse(node(t, src), fr)
+		if err != nil {
+			t.Fatal(err)
+		}
+		if _, err := p.Evaluate(ctx, quiet); !errors.Is(err, boom) {
+			t.Fatalf("%s: %v", src, err)
+		}
+		if len(fr.calls) != 1 {
+			t.Fatalf("%s: evaluated %d atoms after the error", src, len(fr.calls))
+		}
+	}
+}
+
+func TestC4ValidateAndParseErrors(t *testing.T) {
+	bad := []struct{ name, yaml string }{
+		{"empty", ""},
+		{"unknown-atom", "{frobnicate: 1}"},
+		{"unknown-scalar", "frobnicate"},
+		{"two-keys", "{all_fixed: true, max_iterations: 2}"},
+		{"list-not-atom", "[1, 2]"},
+		{"all_fixed-false", "{all_fixed: false}"},
+		{"nfp-string", "{no_fixation_progress: yes please}"},
+		{"max-iter-zero", "{max_iterations: 0}"},
+		{"max-iter-string", "{max_iterations: five}"},
+		{"budget-no-tokens", "{budget: {}}"},
+		{"budget-extra", "{budget: {tokens: 5, dollars: 1}}"},
+		{"budget-scalar", "{budget: 5}"},
+		{"cmd-empty", "{cmd: ''}"},
+		{"cmd-list", "{cmd: [a, b]}"},
+		{"cmd-unknown", "{cmd: nope}"},
+		{"any-empty", "any: []"},
+		{"any-scalar", "any: x"},
+		{"all-bad-child", "all: [{max_iterations: -1}]"},
+		{"not-bad-child", "not: {budget: 0}"},
+	}
+	for _, c := range bad {
+		err := Validate(node(t, c.yaml), []string{"notify"})
+		if !errs.Is(err, CodeBadConvergence) || errs.As(err).Field("detail") == "" {
+			t.Errorf("%s: want ERR_BAD_CONVERGENCE with detail, got %v", c.name, err)
+		}
+	}
+	if err := Validate(node(t, "{cmd: notify}"), []string{"notify"}); err != nil {
+		t.Fatal(err)
+	}
+	if err := Validate(nil, nil); !errs.Is(err, CodeBadConvergence) {
+		t.Fatal("nil node")
+	}
+	// A multi-document node is rejected too.
+	multi := &yaml.Node{Kind: yaml.DocumentNode}
+	if err := Validate(multi, nil); !errs.Is(err, CodeBadConvergence) {
+		t.Fatal("empty document")
+	}
+	// Parse (cmdNames nil) accepts any cmd name — workflow.Parse validated it earlier.
+	if _, err := Parse(node(t, "{cmd: anything}"), &fakeRunner{}); err != nil {
+		t.Fatal(err)
+	}
+	if _, err := Parse(node(t, "{cmd: [x]}"), nil); !errs.Is(err, CodeBadConvergence) {
+		t.Fatal("non-string cmd via Parse")
+	}
+}
diff --git a/internal/fsm/errs/errs.go b/internal/fsm/errs/errs.go
new file mode 100644
index 0000000..ae1edfa
--- /dev/null
+++ b/internal/fsm/errs/errs.go
@@ -0,0 +1,118 @@
+// Package errs is the one error type shared by every internal/fsm package.
+//
+// Every failure a caller can act on is an *Error carrying an ERR_* Code, a
+// human Detail, and structured Fields (reason, name, path, expected, got, …).
+package errs
+
+import (
+	"errors"
+	"sort"
+	"strings"
+)
+
+// Error is a coded error. Code is an ERR_* constant; Detail is human text;
+// Fields carries structured context for callers and the CLI envelope.
+type Error struct {
+	Code   string
+	Detail string
+	Fields map[string]string
+	cause  error
+}
+
+// E builds an *Error. kv must be an even-length list of key/value strings;
+// an odd trailing key is stored with an empty value.
+func E(code, detail string, kv ...string) *Error {
+	e := &Error{Code: code, Detail: detail}
+	if len(kv) > 0 {
+		e.Fields = make(map[string]string, len(kv)/2+1)
+		for i := 0; i < len(kv); i += 2 {
+			v := ""
+			if i+1 < len(kv) {
+				v = kv[i+1]
+			}
+			e.Fields[kv[i]] = v
+		}
+	}
+	return e
+}
+
+// Wrap returns a copy of e carrying cause as its Unwrap target. A nil cause
+// returns e itself.
+func Wrap(e *Error, cause error) *Error {
+	if cause == nil {
+		return e
+	}
+	c := *e
+	c.Fields = copyFields(e.Fields)
+	c.cause = cause
+	return &c
+}
+
+// Error renders "CODE: detail" followed by sorted fields as " (k=v, k=v)".
+func (e *Error) Error() string {
+	var b strings.Builder
+	b.WriteString(e.Code)
+	if e.Detail != "" {
+		b.WriteString(": ")
+		b.WriteString(e.Detail)
+	}
+	if len(e.Fields) > 0 {
+		keys := make([]string, 0, len(e.Fields))
+		for k := range e.Fields {
+			keys = append(keys, k)
+		}
+		sort.Strings(keys)
+		b.WriteString(" (")
+		for i, k := range keys {
+			if i > 0 {
+				b.WriteString(", ")
+			}
+			b.WriteString(k)
+			b.WriteString("=")
+			b.WriteString(e.Fields[k])
+		}
+		b.WriteString(")")
+	}
+	return b.String()
+}
+
+// Unwrap exposes the wrapped cause for errors.Is/As.
+func (e *Error) Unwrap() error { return e.cause }
+
+// Field returns Fields[k] or "".
+func (e *Error) Field(k string) string { return e.Fields[k] }
+
+// Is reports whether err is (or wraps) an *Error with the given code.
+func Is(err error, code string) bool {
+	var e *Error
+	return errors.As(err, &e) && e.Code == code
+}
+
+// Code returns the ERR_* code of err, or "" when err is not an *Error.
+func Code(err error) string {
+	var e *Error
+	if errors.As(err, &e) {
+		return e.Code
+	}
+	return ""
+}
+
+// As returns the *Error inside err, or nil.
+func As(err error) *Error {
+	var e *Error
+	if errors.As(err, &e) {
+		return e
+	}
+	return nil
+}
+
+func copyFields(m map[string]string) map[string]string {
+	if m == nil {
+		return nil
+	}
+	c := make(map[string]string, len(m))
+	for k, v := range m {
+		c[k] = v
+	}
+	return c
+}
diff --git a/internal/fsm/errs/errs_test.go b/internal/fsm/errs/errs_test.go
new file mode 100644
index 0000000..6dcb0f3
--- /dev/null
+++ b/internal/fsm/errs/errs_test.go
@@ -0,0 +1,60 @@
+package errs
+
+import (
+	"errors"
+	"fmt"
+	"io"
+	"testing"
+)
+
+func TestErrorFormat(t *testing.T) {
+	cases := []struct {
+		e    *Error
+		want string
+	}{
+		{E("ERR_X", ""), "ERR_X"},
+		{E("ERR_X", "boom"), "ERR_X: boom"},
+		{E("ERR_X", "boom", "b", "2", "a", "1"), "ERR_X: boom (a=1, b=2)"},
+		{E("ERR_X", "", "k"), "ERR_X (k=)"},
+	}
+	for _, c := range cases {
+		if got := c.e.Error(); got != c.want {
+			t.Errorf("%q != %q", got, c.want)
+		}
+	}
+	if E("ERR_X", "").Fields != nil {
+		t.Fatal("no kv → nil Fields")
+	}
+}
+
+func TestIsCodeAsWrap(t *testing.T) {
+	base := E("ERR_A", "a", "name", "n")
+	wrapped := Wrap(base, io.EOF)
+	outer := fmt.Errorf("ctx: %w", wrapped)
+	if !Is(outer, "ERR_A") || Is(outer, "ERR_B") || Is(io.EOF, "ERR_A") || Is(nil, "ERR_A") {
+		t.Fatal("Is")
+	}
+	if Code(outer) != "ERR_A" || Code(io.EOF) != "" {
+		t.Fatal("Code")
+	}
+	if As(outer) != wrapped || As(io.EOF) != nil {
+		t.Fatal("As")
+	}
+	if !errors.Is(outer, io.EOF) {
+		t.Fatal("Unwrap chain")
+	}
+	if wrapped.Field("name") != "n" || wrapped.Field("zz") != "" {
+		t.Fatal("Field")
+	}
+	// Wrap copies fields; the base must be untouched and nil cause returns base.
+	wrapped.Fields["name"] = "changed"
+	if base.Fields["name"] != "n" || base.Unwrap() != nil {
+		t.Fatal("Wrap must copy")
+	}
+	if Wrap(base, nil) != base {
+		t.Fatal("Wrap(nil) returns e")
+	}
+	if Wrap(E("ERR_N", ""), io.EOF).Fields != nil {
+		t.Fatal("copyFields(nil) stays nil")
+	}
+}
diff --git a/internal/fsm/gate/fake.go b/internal/fsm/gate/fake.go
new file mode 100644
index 0000000..30d0635
--- /dev/null
+++ b/internal/fsm/gate/fake.go
@@ -0,0 +1,82 @@
+package gate
+
+import (
+	"context"
+	"fmt"
+)
+
+// Fake is a scripted Git for tests. Unset answers return zero values; Err
+// (when non-nil) is returned by every method. Calls records method names.
+type Fake struct {
+	HeadSHA   string
+	Refs      map[string]string // RevParse answers
+	Ancestors map[string]bool   // "a b" → answer
+	Counts    map[string]int    // "from..to" → count
+	Clean     bool
+	Porcelain string
+	Diffs     map[string]string // "from..to" → diff; "HEAD" → working diff
+	Tree      string            // WorkTree answer
+	Err       error
+	Calls     []string
+}
+
+func (f *Fake) call(name string, args ...string) {
+	f.Calls = append(f.Calls, fmt.Sprint(append([]string{name}, args...)))
+}
+
+func (f *Fake) Head(context.Context) (string, error) {
+	f.call("Head")
+	return f.HeadSHA, f.Err
+}
+
+func (f *Fake) RevParse(_ context.Context, ref string) (string, error) {
+	f.call("RevParse", ref)
+	if f.Err != nil {
+		return "", f.Err
+	}
+	if ref == "HEAD" {
+		return f.HeadSHA, nil
+	}
+	if sha, ok := f.Refs[ref]; ok {
+		return sha, nil
+	}
+	return "", fmt.Errorf("%s: unknown ref %q", CodeGit, ref)
+}
+
+func (f *Fake) IsAncestor(_ context.Context, a, b string) (bool, error) {
+	f.call("IsAncestor", a, b)
+	return f.Ancestors[a+" "+b], f.Err
+}
+
+func (f *Fake) CommitCount(_ context.Context, from, to string) (int, error) {
+	f.call("CommitCount", from, to)
+	return f.Counts[from+".."+to], f.Err
+}
+
+func (f *Fake) Status(context.Context) (bool, string, error) {
+	f.call("Status")
+	return f.Clean, f.Porcelain, f.Err
+}
+
+func (f *Fake) Diff(_ context.Context, from, to string, max int) (string, bool, error) {
+	f.call("Diff", from, to)
+	if f.Err != nil {
+		return "", false, f.Err
+	}
+	d, t := Cut(f.Diffs[from+".."+to], max)
+	return d, t, nil
+}
+
+func (f *Fake) WorkingDiff(_ context.Context, max int) (string, bool, error) {
+	f.call("WorkingDiff")
+	if f.Err != nil {
+		return "", false, f.Err
+	}
+	d, t := Cut(f.Diffs["HEAD"], max)
+	return d, t, nil
+}
+
+func (f *Fake) WorkTree(context.Context) (string, error) {
+	f.call("WorkTree")
+	return f.Tree, f.Err
+}
diff --git a/internal/fsm/gate/gate_test.go b/internal/fsm/gate/gate_test.go
new file mode 100644
index 0000000..8287020
--- /dev/null
+++ b/internal/fsm/gate/gate_test.go
@@ -0,0 +1,474 @@
+package gate
+
+import (
+	"context"
+	"errors"
+	"os"
+	"os/exec"
+	"path/filepath"
+	"strings"
+	"testing"
+
+	"github.com/dsifry/metareview/internal/fsm/errs"
+	"github.com/dsifry/metareview/internal/fsm/run"
+)
+
+const (
+	shaA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
+	shaB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
+	shaF = "ffffffffffffffffffffffffffffffffffffffff"
+)
+
+func bugs(n int) []run.Bug {
+	out := make([]run.Bug, n)
+	for i := range out {
+		out[i] = run.Bug{ID: strings.Repeat("a", 12), Desc: "d"}
+	}
+	return out
+}
+
+func TestG1Gates(t *testing.T) {
+	ctx := context.Background()
+	fnd := run.Snapshot{Findings: []run.Finding{{IssueText: "x"}}}
+	cnf := run.Snapshot{Confirmed: bugs(2)}
+	fixed := run.Snapshot{AllFound: bugs(3), Unfixed: 0}
+	remain := run.Snapshot{AllFound: bugs(3), Unfixed: 2}
+	cases := []struct {
+		gate string
+		s    run.Snapshot
+		code string // "" → pass
+	}{
+		{"findings_nonempty", fnd, ""}, {"findings_nonempty", run.Snapshot{}, CodeNoFindings},
+		{"findings_empty", run.Snapshot{}, ""}, {"findings_empty", fnd, CodeFindingsPresent},
+		{"confirmed_nonempty", cnf, ""}, {"confirmed_nonempty", run.Snapshot{}, CodeNoConfirmed},
+		{"confirmed_empty", run.Snapshot{}, ""}, {"confirmed_empty", cnf, CodeConfirmedPresent},
+		{"all_fixed", fixed, ""}, {"all_fixed", remain, CodeBugsRemain}, {"all_fixed", run.Snapshot{}, CodeBugsRemain},
+		{"bugs_remain", remain, ""}, {"bugs_remain", run.Snapshot{}, ""}, {"bugs_remain", fixed, CodeAllFixed},
+	}
+	for _, c := range cases {
+		g, ok := Builtin(c.gate)
+		if !ok {
+			t.Fatalf("missing gate %s", c.gate)
+		}
+		err := g(ctx, c.s, &Fake{})
+		if c.code == "" {
+			if err != nil {
+				t.Errorf("%s: unexpected %+v", c.gate, err)
+			}
+			continue
+		}
+		if err == nil || err.Code != c.code || err.Gate != c.gate || err.Detail == "" {
+			t.Errorf("%s: got %+v want %s", c.gate, err, c.code)
+		}
+	}
+	if _, ok := Builtin("nope"); ok {
+		t.Fatal("unknown gate")
+	}
+	want := []string{"all_fixed", "bugs_remain", "commit_exists", "confirmed_empty", "confirmed_nonempty", "findings_empty", "findings_nonempty"}
+	if strings.Join(Names(), ",") != strings.Join(want, ",") {
+		t.Fatalf("Names: %v", Names())
+	}
+}
+
+func TestG1CommitExists(t *testing.T) {
+	ctx := context.Background()
+	g, _ := Builtin("commit_exists")
+	// inapplicable before the first fix entry
+	if err := g(ctx, run.Snapshot{BaseSHA: shaB}, &Fake{HeadSHA: shaA}); err == nil || err.Code != CodeGateInapplicable {
+		t.Fatalf("inapplicable: %+v", err)
+	}
+	snap := run.Snapshot{BaseSHA: shaB, FixEntryHead: shaF}
+	// counts are keyed by (from, to): the gate must count from FixEntryHead, not BaseSHA
+	f := &Fake{HeadSHA: shaA, Counts: map[string]int{shaF + ".." + shaA: 1, shaB + ".." + shaA: 0}, Clean: true}
+	if err := g(ctx, snap, f); err != nil {
+		t.Fatalf("pass: %+v", err)
+	}
+	if strings.Join(f.Calls, ";") != "[Head];[CommitCount "+shaF+" "+shaA+"];[Status]" {
+		t.Fatalf("calls: %v", f.Calls)
+	}
+	// commits but dirty → ERR_NO_COMMIT with porcelain + working diff, capped
+	big := strings.Repeat("x", run.MaxDetail)
+	f = &Fake{HeadSHA: shaA, Counts: map[string]int{shaF + ".." + shaA: 2}, Porcelain: "1 .M N... 100644 100644 100644 0 0 f.go\n", Diffs: map[string]string{"HEAD": big}}
+	err := g(ctx, snap, f)
+	if err == nil || err.Code != CodeNoCommit || !strings.Contains(err.Detail, "f.go") || !strings.Contains(err.Detail, "2 commits") || !err.DetailTruncated || len(err.Detail) > run.MaxDetail {
+		t.Fatalf("dirty: %+v", err)
+	}
+	// zero commits, clean
+	f = &Fake{HeadSHA: shaA, Clean: true}
+	if err := g(ctx, snap, f); err == nil || err.Code != CodeNoCommit || !strings.Contains(err.Detail, "0 commits since "+shaF) {
+		t.Fatalf("no commits: %+v", err)
+	}
+	// each git call site failing individually → ERR_GIT
+	boom := errors.New("boom")
+	for _, failAt := range []string{"Head", "CommitCount", "Status", "WorkingDiff"} {
+		f := &failingFake{Fake: Fake{HeadSHA: shaA, Counts: map[string]int{shaF + ".." + shaA: 0}}, failAt: failAt, err: boom}
+		err := g(ctx, snap, f)
+		if err == nil || err.Code != CodeGit || !strings.Contains(err.Detail, "boom") {
+			t.Fatalf("fail at %s: %+v", failAt, err)
+		}
+	}
+}
+
+// failingFake fails exactly one method.
+type failingFake struct {
+	Fake
+	failAt string
+	err    error
+}
+
+func (f *failingFake) Head(ctx context.Context) (string, error) {
+	if f.failAt == "Head" {
+		return "", f.err
+	}
+	return f.Fake.Head(ctx)
+}
+func (f *failingFake) CommitCount(ctx context.Context, a, b string) (int, error) {
+	if f.failAt == "CommitCount" {
+		return 0, f.err
+	}
+	return f.Fake.CommitCount(ctx, a, b)
+}
+func (f *failingFake) Status(ctx context.Context) (bool, string, error) {
+	if f.failAt == "Status" {
+		return false, "", f.err
+	}
+	return f.Fake.Status(ctx)
+}
+func (f *failingFake) WorkingDiff(ctx context.Context, max int) (string, bool, error) {
+	if f.failAt == "WorkingDiff" {
+		return "", false, f.err
+	}
+	return f.Fake.WorkingDiff(ctx, max)
+}
+
+// ---- real git ----
+
+func git(t *testing.T, dir string, args ...string) string {
+	t.Helper()
+	cmd := exec.Command("git", args...)
+	cmd.Dir = dir
+	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t", "GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z")
+	out, err := cmd.CombinedOutput()
+	if err != nil {
+		t.Fatalf("git %v: %v\n%s", args, err, out)
+	}
+	return strings.TrimSpace(string(out))
+}
+
+func write(t *testing.T, dir, name, content string) {
+	t.Helper()
+	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
+		t.Fatal(err)
+	}
+	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
+		t.Fatal(err)
+	}
+}
+
+func repo(t *testing.T) (string, string, string) {
+	t.Helper()
+	dir := t.TempDir()
+	git(t, dir, "init", "-q", "-b", "main")
+	write(t, dir, "a.txt", "one\n")
+	git(t, dir, "add", "-A")
+	git(t, dir, "commit", "-qm", "c1")
+	c1 := git(t, dir, "rev-parse", "HEAD")
+	write(t, dir, "a.txt", "one\ntwo é\n")
+	git(t, dir, "commit", "-qam", "c2")
+	c2 := git(t, dir, "rev-parse", "HEAD")
+	return dir, c1, c2
+}
+
+func TestG2RealGit(t *testing.T) {
+	ctx := context.Background()
+	dir, c1, c2 := repo(t)
+	g := NewExec(dir, RealExec)
+	head, err := g.Head(ctx)
+	if err != nil || head != c2 {
+		t.Fatalf("head %s %v", head, err)
+	}
+	git(t, dir, "tag", "-a", "v1", "-m", "tag")
+	for ref, want := range map[string]string{"main": c2, "HEAD~1": c1, c1[:7]: c1, "v1": c2} {
+		if got, err := g.RevParse(ctx, ref); err != nil || got != want {
+			t.Errorf("RevParse %s: %s %v", ref, got, err)
+		}
+	}
+	if _, err := g.RevParse(ctx, "nope"); !errs.Is(err, CodeGit) || errs.As(err).Field("ref") != "nope" {
+		t.Fatalf("unknown ref: %v", err)
+	}
+	for _, bad := range []string{"", "-bad", "a b", "a\x00b", "a\nb", "x\x7f"} {
+		if _, err := g.RevParse(ctx, bad); !errs.Is(err, CodeGitRef) {
+			t.Errorf("ref %q: %v", bad, err)
+		}
+	}
+	// ancestor: true, false (exit 1)
+	if ok, err := g.IsAncestor(ctx, c1, c2); err != nil || !ok {
+		t.Fatal("c1 ancestor of c2")
+	}
+	if ok, err := g.IsAncestor(ctx, c2, c1); err != nil || ok {
+		t.Fatal("c2 not ancestor of c1")
+	}
+	if n, err := g.CommitCount(ctx, c1, c2); err != nil || n != 1 {
+		t.Fatalf("count %d %v", n, err)
+	}
+	if n, err := g.CommitCount(ctx, c2, c2); err != nil || n != 0 {
+		t.Fatalf("count0 %d %v", n, err)
+	}
+	// sha validation on every sha-taking method (HEAD~1 is refused; RevParse accepts it)
+	for _, bad := range []string{"HEAD~1", "main", "abcdef", strings.Repeat("a", 41), "ABCDEFA", ""} {
+		if _, err := g.IsAncestor(ctx, bad, c2); !errs.Is(err, CodeGitRef) {
+			t.Errorf("IsAncestor %q: %v", bad, err)
+		}
+		if _, err := g.CommitCount(ctx, c1, bad); !errs.Is(err, CodeGitRef) {
+			t.Errorf("CommitCount %q: %v", bad, err)
+		}
+		if _, _, err := g.Diff(ctx, bad, c2, 10); !errs.Is(err, CodeGitRef) {
+			t.Errorf("Diff %q: %v", bad, err)
+		}
+	}
+	for _, ok := range []string{"abcdefa", strings.Repeat("a", 40), "HEAD"} {
+		if !ValidSHA(ok) {
+			t.Errorf("ValidSHA %q", ok)
+		}
+	}
+	// status clean, then dirty incl. a file in a new untracked directory
+	clean, porcelain, err := g.Status(ctx)
+	if err != nil || !clean || porcelain != "" {
+		t.Fatalf("clean: %v %q %v", clean, porcelain, err)
+	}
+	tree1, err := g.WorkTree(ctx)
+	if err != nil || len(tree1) != 40 {
+		t.Fatalf("worktree %s %v", tree1, err)
+	}
+	write(t, dir, "new/dir/u.txt", "u")
+	write(t, dir, "a.txt", "changed\n")
+	clean, porcelain, err = g.Status(ctx)
+	if err != nil || clean || !strings.Contains(porcelain, "? new/dir/u.txt") || !strings.Contains(porcelain, "a.txt") {
+		t.Fatalf("dirty: %v %q %v", clean, porcelain, err)
+	}
+	tree2, _ := g.WorkTree(ctx)
+	if tree2 == tree1 {
+		t.Fatal("worktree hash must move with content")
+	}
+	// content change to the untracked file: porcelain identical, tree moves
+	write(t, dir, "new/dir/u.txt", "v")
+	_, porcelain2, _ := g.Status(ctx)
+	tree3, _ := g.WorkTree(ctx)
+	if porcelain2 != porcelain || tree3 == tree2 {
+		t.Fatalf("content-blind: %v %v", porcelain2 == porcelain, tree3 == tree2)
+	}
+	// the scratch index must not touch the real index
+	if out := git(t, dir, "diff", "--cached", "--name-only"); out != "" {
+		t.Fatalf("real index touched: %s", out)
+	}
+	wd, tr, err := g.WorkingDiff(ctx, 1<<20)
+	if err != nil || tr || !strings.Contains(wd, "+changed") {
+		t.Fatalf("working diff: %v %v", tr, err)
+	}
+	// diff + truncation at a rune boundary (é is 2 bytes)
+	full, tr, err := g.Diff(ctx, c1, c2, 1<<20)
+	if err != nil || tr || !strings.Contains(full, "+two é") {
+		t.Fatalf("diff: %q %v %v", full, tr, err)
+	}
+	idx := strings.Index(full, "é")
+	cut, tr, err := g.Diff(ctx, c1, c2, idx+1)
+	if err != nil || !tr || cut != full[:idx] {
+		t.Fatalf("rune cut: %q %v", cut, tr)
+	}
+}
+
+func TestG3TreeHashPin(t *testing.T) {
+	// sha256("abc\ndef") — hand-computed, the authority for the preimage layout.
+	if got := TreeHash("abc", "def"); got != "d53d6b91af7caf8fe3d8021f116270137c0079d579a1e16965da80c2ed138ffb" {
+		t.Fatalf("TreeHash preimage changed: %s", got)
+	}
+	if TreeHash("abc", "def") == TreeHash("abcd", "ef") {
+		t.Fatal("separator must keep head and tree apart")
+	}
+}
+
+func TestG2ExecErrorBranches(t *testing.T) {
+	ctx := context.Background()
+	boom := errors.New("spawn failed")
+	// process cannot run
+	g := NewExec("/", func(context.Context, string, []string, ...string) ([]byte, []byte, int, error) {
+		return nil, nil, 0, boom
+	})
+	if _, err := g.Head(ctx); !errs.Is(err, CodeGit) || !errors.Is(err, boom) {
+		t.Fatalf("spawn: %v", err)
+	}
+	// exit code ≥ 2 anywhere → ERR_GIT with stderr in Detail
+	g = NewExec("/", func(context.Context, string, []string, ...string) ([]byte, []byte, int, error) {
+		return nil, []byte("fatal: broken\n"), 128, nil
+	})
+	if _, err := g.RevParse(ctx, "main"); !errs.Is(err, CodeGit) || !strings.Contains(err.Error(), "fatal: broken") || errs.As(err).Field("exit") != "128" {
+		t.Fatalf("exit 128: %v", err)
+	}
+	if _, err := g.IsAncestor(ctx, shaA, shaB); !errs.Is(err, CodeGit) {
+		t.Fatal("ancestor exit 128")
+	}
+	if _, err := g.CommitCount(ctx, shaA, shaB); !errs.Is(err, CodeGit) {
+		t.Fatal("count exit 128")
+	}
+	if _, _, err := g.Status(ctx); !errs.Is(err, CodeGit) {
+		t.Fatal("status exit 128")
+	}
+	if _, _, err := g.Diff(ctx, shaA, shaB, 1); !errs.Is(err, CodeGit) {
+		t.Fatal("diff exit 128")
+	}
+	if _, _, err := g.WorkingDiff(ctx, 1); !errs.Is(err, CodeGit) {
+		t.Fatal("wdiff exit 128")
+	}
+	if _, err := g.WorkTree(ctx); !errs.Is(err, CodeGit) {
+		t.Fatal("worktree exit 128")
+	}
+	// exit 1 where it is not a legal answer, and malformed stdout
+	exit1 := NewExec("/", func(_ context.Context, _ string, _ []string, args ...string) ([]byte, []byte, int, error) {
+		return []byte("garbage"), nil, 1, nil
+	})
+	if _, err := exit1.RevParse(ctx, "main"); !errs.Is(err, CodeGit) {
+		t.Fatal("rev-parse exit 1")
+	}
+	if _, err := exit1.CommitCount(ctx, shaA, shaB); !errs.Is(err, CodeGit) {
+		t.Fatal("count exit 1")
+	}
+	if _, _, err := exit1.Status(ctx); !errs.Is(err, CodeGit) {
+		t.Fatal("status exit 1")
+	}
+	if _, _, err := exit1.Diff(ctx, shaA, shaB, 1); !errs.Is(err, CodeGit) {
+		t.Fatal("diff exit 1")
+	}
+	if _, _, err := exit1.WorkingDiff(ctx, 1); !errs.Is(err, CodeGit) {
+		t.Fatal("wdiff exit 1")
+	}
+	if _, err := exit1.WorkTree(ctx); !errs.Is(err, CodeGit) {
+		t.Fatal("worktree add exit 1")
+	}
+	// write-tree exit 1 / short output after a successful add
+	calls := 0
+	wt := NewExec("/", func(_ context.Context, _ string, env []string, args ...string) ([]byte, []byte, int, error) {
+		calls++
+		if args[0] == "add" {
+			if len(env) != 1 || !strings.HasPrefix(env[0], "GIT_INDEX_FILE=") {
+				t.Fatalf("scratch index env: %v", env)
+			}
+			return nil, nil, 0, nil
+		}
+		return []byte("short"), nil, 0, nil
+	})
+	if _, err := wt.WorkTree(ctx); !errs.Is(err, CodeGit) {
+		t.Fatal("write-tree short")
+	}
+	// write-tree spawn failure after a successful add, and scratch-index creation failure
+	n := 0
+	wt2 := NewExec("/", func(_ context.Context, _ string, _ []string, args ...string) ([]byte, []byte, int, error) {
+		n++
+		if n == 1 {
+			return nil, nil, 0, nil
+		}
+		return nil, nil, 0, boom
+	})
+	if _, err := wt2.WorkTree(ctx); !errors.Is(err, boom) {
+		t.Fatal("write-tree spawn failure")
+	}
+	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
+	if _, err := wt2.WorkTree(ctx); !errs.Is(err, CodeGit) || !strings.Contains(err.Error(), "scratch index") {
+		t.Fatalf("scratch index failure: %v", err)
+	}
+	t.Setenv("TMPDIR", os.TempDir())
+	// rev-parse returning a non-sha with exit 0, and count with non-integer stdout
+	odd := NewExec("/", func(_ context.Context, _ string, _ []string, args ...string) ([]byte, []byte, int, error) {
+		return []byte("not-a-sha\n"), nil, 0, nil
+	})
+	if _, err := odd.RevParse(ctx, "main"); !errs.Is(err, CodeGit) {
+		t.Fatal("rev-parse odd")
+	}
+	if _, err := odd.CommitCount(ctx, shaA, shaB); !errs.Is(err, CodeGit) {
+		t.Fatal("count odd")
+	}
+	// argument shape observed through the seam: --end-of-options before user data
+	var seen [][]string
+	spy := NewExec("/", func(_ context.Context, _ string, _ []string, args ...string) ([]byte, []byte, int, error) {
+		seen = append(seen, args)
+		return []byte(shaA + "\n"), nil, 0, nil
+	})
+	_, _ = spy.RevParse(ctx, "main")
+	_, _ = spy.IsAncestor(ctx, shaA, shaB)
+	_, _, _ = spy.Diff(ctx, shaA, shaB, 10)
+	for _, a := range seen {
+		if !strings.Contains(strings.Join(a, " "), "--end-of-options") {
+			t.Fatalf("missing --end-of-options: %v", a)
+		}
+	}
+	if !strings.HasSuffix(seen[0][len(seen[0])-1], "^{commit}") {
+		t.Fatal("rev-parse must peel to a commit")
+	}
+	// RealExec: spawn failure branch via a bogus dir
+	if _, _, _, err := RealExec(ctx, "/nonexistent-dir-xyz", nil, "status"); err == nil {
+		t.Fatal("RealExec must report an unrunnable process")
+	}
+	// scrubEnv drops GIT_* overrides
+	got := scrubEnv([]string{"GIT_DIR=x", "GIT_CONFIG_COUNT=1", "GIT_WORK_TREE=y", "GIT_INDEX_FILE=z", "GIT_EXTERNAL_DIFF=e", "GIT_OBJECT_DIRECTORY=o", "GIT_ALTERNATE_OBJECT_DIRECTORIES=a", "PATH=p", "HOME=h"})
+	if strings.Join(got, ",") != "PATH=p,HOME=h" {
+		t.Fatalf("scrub: %v", got)
+	}
+}
+
+func TestG4FakeContract(t *testing.T) {
+	ctx := context.Background()
+	f := &Fake{HeadSHA: shaA, Refs: map[string]string{"main": shaA}, Ancestors: map[string]bool{shaB + " " + shaA: true}, Counts: map[string]int{shaB + ".." + shaA: 3}, Clean: false, Porcelain: "p", Diffs: map[string]string{shaB + ".." + shaA: "diff é", "HEAD": "wd"}, Tree: "t"}
+	if h, _ := f.Head(ctx); h != shaA {
+		t.Fatal("head")
+	}
+	if r, _ := f.RevParse(ctx, "HEAD"); r != shaA {
+		t.Fatal("revparse HEAD")
+	}
+	if r, _ := f.RevParse(ctx, "main"); r != shaA {
+		t.Fatal("revparse main")
+	}
+	if _, err := f.RevParse(ctx, "nope"); err == nil {
+		t.Fatal("unknown ref")
+	}
+	if ok, _ := f.IsAncestor(ctx, shaB, shaA); !ok {
+		t.Fatal("ancestor")
+	}
+	if n, _ := f.CommitCount(ctx, shaB, shaA); n != 3 {
+		t.Fatal("count")
+	}
+	if c, p, _ := f.Status(ctx); c || p != "p" {
+		t.Fatal("status")
+	}
+	if d, tr, _ := f.Diff(ctx, shaB, shaA, 6); d != "diff " || !tr {
+		t.Fatalf("diff cut %q", d)
+	}
+	if d, _, _ := f.WorkingDiff(ctx, 100); d != "wd" {
+		t.Fatal("wd")
+	}
+	if tr, _ := f.WorkTree(ctx); tr != "t" {
+		t.Fatal("tree")
+	}
+	if len(f.Calls) != 10 {
+		t.Fatalf("calls %v", f.Calls)
+	}
+	boom := errors.New("boom")
+	f.Err = boom
+	if _, err := f.Head(ctx); err != boom {
+		t.Fatal("err head")
+	}
+	if _, err := f.RevParse(ctx, "HEAD"); err != boom {
+		t.Fatal("err revparse")
+	}
+	if _, _, err := f.Diff(ctx, shaB, shaA, 1); err != boom {
+		t.Fatal("err diff")
+	}
+	if _, _, err := f.WorkingDiff(ctx, 1); err != boom {
+		t.Fatal("err wd")
+	}
+	// Cut edge cases
+	if s, tr := Cut("abc", -1); s != "abc" || tr {
+		t.Fatal("negative max keeps all")
+	}
+	if s, tr := Cut("é", 1); s != "" || !tr {
+		t.Fatal("cut before a partial rune")
+	}
+}
diff --git a/internal/fsm/gate/gates.go b/internal/fsm/gate/gates.go
new file mode 100644
index 0000000..c098bcf
--- /dev/null
+++ b/internal/fsm/gate/gates.go
@@ -0,0 +1,116 @@
+package gate
+
+import (
+	"context"
+	"fmt"
+	"sort"
+
+	"github.com/dsifry/metareview/internal/fsm/converge"
+	"github.com/dsifry/metareview/internal/fsm/run"
+)
+
+// Gate codes.
+const (
+	CodeNoFindings       = "ERR_NO_FINDINGS"
+	CodeFindingsPresent  = "ERR_FINDINGS_PRESENT"
+	CodeNoConfirmed      = "ERR_NO_CONFIRMED"
+	CodeConfirmedPresent = "ERR_CONFIRMED_PRESENT"
+	CodeNoCommit         = "ERR_NO_COMMIT"
+	CodeGateInapplicable = "ERR_GATE_INAPPLICABLE"
+	CodeBugsRemain       = "ERR_BUGS_REMAIN"
+	CodeAllFixed         = "ERR_ALL_FIXED"
+)
+
+// Gate evaluates a snapshot; nil means pass.
+type Gate func(ctx context.Context, s run.Snapshot, g Git) *run.GateError
+
+func fail(name, code, detail string) *run.GateError {
+	d, truncated := run.CapDetail(detail)
+	return &run.GateError{Code: code, Gate: name, Detail: d, DetailTruncated: truncated}
+}
+
+var builtin = map[string]Gate{
+	"findings_nonempty": func(_ context.Context, s run.Snapshot, _ Git) *run.GateError {
+		if len(s.Findings) > 0 {
+			return nil
+		}
+		return fail("findings_nonempty", CodeNoFindings, "no findings this iteration")
+	},
+	"findings_empty": func(_ context.Context, s run.Snapshot, _ Git) *run.GateError {
+		if len(s.Findings) == 0 {
+			return nil
+		}
+		return fail("findings_empty", CodeFindingsPresent, fmt.Sprintf("%d findings present", len(s.Findings)))
+	},
+	"confirmed_nonempty": func(_ context.Context, s run.Snapshot, _ Git) *run.GateError {
+		if len(s.Confirmed) > 0 {
+			return nil
+		}
+		return fail("confirmed_nonempty", CodeNoConfirmed, "no confirmed bugs this iteration")
+	},
+	"confirmed_empty": func(_ context.Context, s run.Snapshot, _ Git) *run.GateError {
+		if len(s.Confirmed) == 0 {
+			return nil
+		}
+		return fail("confirmed_empty", CodeConfirmedPresent, fmt.Sprintf("%d confirmed bugs present", len(s.Confirmed)))
+	},
+	"commit_exists": commitExists,
+	"all_fixed": func(_ context.Context, s run.Snapshot, _ Git) *run.GateError {
+		if converge.AllFixed(s) {
+			return nil
+		}
+		return fail("all_fixed", CodeBugsRemain, fmt.Sprintf("%d of %d bugs unfixed", s.Unfixed, len(s.AllFound)))
+	},
+	"bugs_remain": func(_ context.Context, s run.Snapshot, _ Git) *run.GateError {
+		if !converge.AllFixed(s) {
+			return nil
+		}
+		return fail("bugs_remain", CodeAllFixed, fmt.Sprintf("all %d bugs fixed", len(s.AllFound)))
+	},
+}
+
+// commitExists passes when at least one commit landed since the fix entry
+// and the tree is clean. Git failures are reported as ERR_GIT gate errors so
+// the audit shows them.
+func commitExists(ctx context.Context, s run.Snapshot, g Git) *run.GateError {
+	const name = "commit_exists"
+	if s.FixEntryHead == "" {
+		return fail(name, CodeGateInapplicable, "no fix entry recorded for this iteration")
+	}
+	head, err := g.Head(ctx)
+	if err != nil {
+		return fail(name, CodeGit, err.Error())
+	}
+	n, err := g.CommitCount(ctx, s.FixEntryHead, head)
+	if err != nil {
+		return fail(name, CodeGit, err.Error())
+	}
+	clean, porcelain, err := g.Status(ctx)
+	if err != nil {
+		return fail(name, CodeGit, err.Error())
+	}
+	if n > 0 && clean {
+		return nil
+	}
+	wd, _, err := g.WorkingDiff(ctx, run.MaxDetail)
+	if err != nil {
+		return fail(name, CodeGit, err.Error())
+	}
+	return fail(name, CodeNoCommit, fmt.Sprintf("%d commits since %s; clean=%v\n--- status ---\n%s--- working diff ---\n%s", n, s.FixEntryHead, clean, porcelain, wd))
+}
+
+// Names lists the built-in gate names, sorted.
+func Names() []string {
+	names := make([]string, 0, len(builtin))
+	for n := range builtin {
+		names = append(names, n)
+	}
+	sort.Strings(names)
+	return names
+}
+
+// Builtin returns the gate by name.
+func Builtin(name string) (Gate, bool) {
+	g, ok := builtin[name]
+	return g, ok
+}
diff --git a/internal/fsm/gate/git.go b/internal/fsm/gate/git.go
new file mode 100644
index 0000000..0062d1c
--- /dev/null
+++ b/internal/fsm/gate/git.go
@@ -0,0 +1,276 @@
+// Package gate holds the deterministic gates evaluated between states and the
+// narrow Git interface they need. Every gate returns a coded *run.GateError
+// with evidence in Detail; nothing here calls an LLM.
+package gate
+
+import (
+	"context"
+	"crypto/sha256"
+	"encoding/hex"
+	"fmt"
+	"os"
+	"os/exec"
+	"regexp"
+	"strconv"
+	"strings"
+	"unicode/utf8"
+
+	"github.com/dsifry/metareview/internal/fsm/errs"
+)
+
+// Error codes produced by the Git implementations.
+const (
+	CodeGit    = "ERR_GIT"
+	CodeGitRef = "ERR_GIT_REF"
+)
+
+// Git is the read-only view of a work tree the gates need.
+type Git interface {
+	Head(ctx context.Context) (string, error)
+	// RevParse resolves any ref (branch, HEAD~1, sha) to a full commit sha.
+	RevParse(ctx context.Context, ref string) (string, error)
+	// IsAncestor reports whether a is an ancestor of b (exit 1 → false, nil).
+	IsAncestor(ctx context.Context, a, b string) (bool, error)
+	// CommitCount counts from..to.
+	CommitCount(ctx context.Context, from, to string) (int, error)
+	// Status returns clean and the porcelain v2 status incl. untracked files.
+	Status(ctx context.Context) (clean bool, porcelain string, err error)
+	// Diff returns `git diff from..to` cut at a rune boundary ≤ max bytes.
+	Diff(ctx context.Context, from, to string, max int) (diff string, truncated bool, err error)
+	// WorkingDiff returns `git diff HEAD` cut like Diff.
+	WorkingDiff(ctx context.Context, max int) (string, bool, error)
+	// WorkTree returns a content hash of the working tree (tracked + untracked,
+	// ignored excluded): `git add -A` into a scratch index, then `write-tree`.
+	WorkTree(ctx context.Context) (string, error)
+}
+
+// Exec runs git in dir with extra env entries and args; it returns stdout,
+// stderr, the exit code, and a non-nil err only when the process could not
+// be run at all.
+type Exec func(ctx context.Context, dir string, env []string, args ...string) (stdout, stderr []byte, code int, err error)
+
+// RealExec shells out to git with prompts, external diff drivers, and
+// config injection disabled.
+func RealExec(ctx context.Context, dir string, env []string, args ...string) ([]byte, []byte, int, error) {
+	cmd := exec.CommandContext(ctx, "git", append([]string{"-c", "core.fsmonitor=false", "-c", "diff.external=", "--no-pager"}, args...)...)
+	cmd.Dir = dir
+	cmd.Env = append(scrubEnv(cmd.Environ()), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C", "GIT_EXTERNAL_DIFF=", "GIT_CONFIG_NOSYSTEM=1")
+	cmd.Env = append(cmd.Env, env...)
+	var out, errb strings.Builder
+	cmd.Stdout, cmd.Stderr = &out, &errb
+	err := cmd.Run()
+	code := 0
+	if err != nil {
+		var ee *exec.ExitError
+		if ok := asExit(err, &ee); ok {
+			code, err = ee.ExitCode(), nil
+		}
+	}
+	return []byte(out.String()), []byte(errb.String()), code, err
+}
+
+// scrubEnv drops GIT_* overrides that could redirect git to another
+// repository or inject configuration.
+func scrubEnv(environ []string) []string {
+	out := environ[:0:0]
+	for _, kv := range environ {
+		k, _, _ := strings.Cut(kv, "=")
+		switch {
+		case k == "GIT_DIR", k == "GIT_WORK_TREE", k == "GIT_INDEX_FILE", k == "GIT_EXTERNAL_DIFF", k == "GIT_OBJECT_DIRECTORY", k == "GIT_ALTERNATE_OBJECT_DIRECTORIES", strings.HasPrefix(k, "GIT_CONFIG"):
+			continue
+		}
+		out = append(out, kv)
+	}
+	return out
+}
+
+func asExit(err error, target **exec.ExitError) bool {
+	ee, ok := err.(*exec.ExitError)
+	if ok {
+		*target = ee
+	}
+	return ok
+}
+
+var shaPattern = regexp.MustCompile(`^[0-9a-f]{7,40}$|^HEAD$`)
+
+// ValidSHA reports whether s is a sha argument the gates may pass to git.
+func ValidSHA(s string) bool { return shaPattern.MatchString(s) }
+
+// ValidRef reports whether ref is safe to hand to rev-parse: non-empty, no
+// leading '-', no control characters or whitespace.
+func ValidRef(ref string) bool {
+	if ref == "" || strings.HasPrefix(ref, "-") || !utf8.ValidString(ref) {
+		return false
+	}
+	for _, r := range ref {
+		if r < 0x20 || r == 0x7f || r == ' ' {
+			return false
+		}
+	}
+	return true
+}
+
+type execGit struct {
+	dir string
+	x   Exec
+}
+
+// NewExec returns a Git backed by x in dir.
+func NewExec(dir string, x Exec) Git { return &execGit{dir: dir, x: x} }
+
+func (g *execGit) run(ctx context.Context, args ...string) (string, int, error) {
+	return g.runEnv(ctx, nil, args...)
+}
+
+func (g *execGit) runEnv(ctx context.Context, env []string, args ...string) (string, int, error) {
+	out, stderr, code, err := g.x(ctx, g.dir, env, args...)
+	if err != nil {
+		return "", 0, errs.Wrap(errs.E(CodeGit, fmt.Sprintf("git %s: %v", args[0], err), "op", args[0]), err)
+	}
+	if code != 0 && code != 1 {
+		return "", code, errs.E(CodeGit, fmt.Sprintf("git %s exited %d: %s", args[0], code, strings.TrimSpace(string(stderr))), "op", args[0], "exit", strconv.Itoa(code))
+	}
+	return string(out), code, nil
+}
+
+func (g *execGit) Head(ctx context.Context) (string, error) {
+	return g.RevParse(ctx, "HEAD")
+}
+
+func (g *execGit) RevParse(ctx context.Context, ref string) (string, error) {
+	if !ValidRef(ref) {
+		return "", errs.E(CodeGitRef, "invalid ref", "ref", ref)
+	}
+	out, code, err := g.run(ctx, "rev-parse", "--verify", "--quiet", "--end-of-options", ref+"^{commit}")
+	if err != nil {
+		return "", err
+	}
+	sha := strings.TrimSpace(out)
+	if code != 0 || !shaPattern.MatchString(sha) || len(sha) != 40 {
+		return "", errs.E(CodeGit, "unknown ref", "ref", ref, "op", "rev-parse")
+	}
+	return sha, nil
+}
+
+func shaArgs(shas ...string) error {
+	for _, s := range shas {
+		if !ValidSHA(s) {
+			return errs.E(CodeGitRef, "invalid sha", "ref", s)
+		}
+	}
+	return nil
+}
+
+func (g *execGit) IsAncestor(ctx context.Context, a, b string) (bool, error) {
+	if err := shaArgs(a, b); err != nil {
+		return false, err
+	}
+	_, code, err := g.run(ctx, "merge-base", "--is-ancestor", "--end-of-options", a, b)
+	if err != nil {
+		return false, err
+	}
+	return code == 0, nil
+}
+
+func (g *execGit) CommitCount(ctx context.Context, from, to string) (int, error) {
+	if err := shaArgs(from, to); err != nil {
+		return 0, err
+	}
+	out, code, err := g.run(ctx, "rev-list", "--count", "--end-of-options", from+".."+to)
+	if err != nil {
+		return 0, err
+	}
+	n, perr := strconv.Atoi(strings.TrimSpace(out))
+	if code != 0 || perr != nil {
+		return 0, errs.E(CodeGit, "rev-list --count produced "+strings.TrimSpace(out), "op", "rev-list")
+	}
+	return n, nil
+}
+
+func (g *execGit) Status(ctx context.Context) (bool, string, error) {
+	out, code, err := g.run(ctx, "status", "--porcelain=v2", "--untracked-files=all")
+	if err != nil {
+		return false, "", err
+	}
+	if code != 0 {
+		return false, "", errs.E(CodeGit, "git status exited 1", "op", "status")
+	}
+	return strings.TrimSpace(out) == "", out, nil
+}
+
+func (g *execGit) Diff(ctx context.Context, from, to string, max int) (string, bool, error) {
+	if err := shaArgs(from, to); err != nil {
+		return "", false, err
+	}
+	out, code, err := g.run(ctx, "diff", "--no-ext-diff", "--no-textconv", "--end-of-options", from+".."+to)
+	if err != nil {
+		return "", false, err
+	}
+	if code != 0 {
+		return "", false, errs.E(CodeGit, "git diff exited 1", "op", "diff")
+	}
+	d, t := Cut(out, max)
+	return d, t, nil
+}
+
+func (g *execGit) WorkingDiff(ctx context.Context, max int) (string, bool, error) {
+	out, code, err := g.run(ctx, "diff", "--no-ext-diff", "--no-textconv", "HEAD")
+	if err != nil {
+		return "", false, err
+	}
+	if code != 0 {
+		return "", false, errs.E(CodeGit, "git diff HEAD exited 1", "op", "diff")
+	}
+	d, t := Cut(out, max)
+	return d, t, nil
+}
+
+// WorkTree hashes the working tree through a scratch index so content
+// changes (not just paths) move the hash. The scratch index lives in the
+// OS temp dir and is removed afterwards.
+func (g *execGit) WorkTree(ctx context.Context) (string, error) {
+	f, err := os.CreateTemp("", "mrv-index-*")
+	if err != nil {
+		return "", errs.Wrap(errs.E(CodeGit, "scratch index: "+err.Error(), "op", "write-tree"), err)
+	}
+	name := f.Name()
+	_ = f.Close()
+	_ = os.Remove(name)
+	defer os.Remove(name)
+	env := []string{"GIT_INDEX_FILE=" + name}
+	if _, code, err := g.runEnv(ctx, env, "add", "-A", "--"); err != nil {
+		return "", err
+	} else if code != 0 {
+		return "", errs.E(CodeGit, "git add -A exited 1", "op", "add")
+	}
+	out, code, err := g.runEnv(ctx, env, "write-tree")
+	if err != nil {
+		return "", err
+	}
+	id := strings.TrimSpace(out)
+	if code != 0 || len(id) != 40 {
+		return "", errs.E(CodeGit, "write-tree produced "+id, "op", "write-tree")
+	}
+	return id, nil
+}
+
+// Cut truncates s to at most max bytes at a rune boundary.
+func Cut(s string, max int) (string, bool) {
+	if max < 0 || len(s) <= max {
+		return s, false
+	}
+	i := max
+	for i > 0 && !utf8.RuneStart(s[i]) {
+		i--
+	}
+	return s[:i], true
+}
+
+// TreeHash is the working-tree fingerprint: sha256(head + "\n" + workTree),
+// where workTree is Git.WorkTree's content hash; the porcelain status is
+// stored beside it as evidence but does not feed the hash.
+func TreeHash(head, workTree string) string {
+	sum := sha256.Sum256([]byte(head + "\n" + workTree))
+	return hex.EncodeToString(sum[:])
+}
diff --git a/internal/fsm/run/fold.go b/internal/fsm/run/fold.go
index 21ee696..c952828 100644
--- a/internal/fsm/run/fold.go
+++ b/internal/fsm/run/fold.go
@@ -502,7 +502,7 @@ func withinCaps(p any) bool {
 			}
 		}
 		for _, c := range d.AllowedCmds {
-			if !shortOK(c.Name) || !argvOK(c.Argv) || len(c.FileHashes) > MaxFileHashes {
+			if !shortOK(c.Name) || !argvOK(c.Argv) || len(c.FileHashes) > MaxFileHashes || len(c.Env) > MaxEnv || !shortOK(c.Env...) {
 				return false
 			}
 			for k, v := range c.FileHashes {
diff --git a/internal/fsm/run/fold_test.go b/internal/fsm/run/fold_test.go
index 9d5f32a..1711ddb 100644
--- a/internal/fsm/run/fold_test.go
+++ b/internal/fsm/run/fold_test.go
@@ -804,6 +804,13 @@ func TestFoldCaps(t *testing.T) {
 			b.Init(d)
 			return b.Events()
 		}, MaxFileHashes},
+		{"Env-count", func(n int) []Event {
+			b := NewBuilder(runA)
+			d := baseInit()
+			d.AllowedCmds = []AllowedCmd{{Name: "c", Argv: []string{"/c"}, FileHashes: map[string]string{}, Env: make([]string, n)}}
+			b.Init(d)
+			return b.Events()
+		}, MaxEnv},
 		{"Delta-list-count", func(n int) []Event {
 			b := NewBuilder(runA)
 			b.Init(baseInit())
@@ -875,6 +882,7 @@ func TestFoldCapsPerField(t *testing.T) {
 		"init.cmd.filehash": initWith(func(d *InitData) {
 			d.AllowedCmds = []AllowedCmd{{Name: "c", Argv: []string{"/c"}, FileHashes: map[string]string{big: "h"}}}
 		}),
+		"init.cmd.env":       initWith(func(d *InitData) { d.AllowedCmds = []AllowedCmd{{Name: "c", Argv: []string{"/c"}, Env: []string{big}}} }),
 		"tree.head":          ev(TypeTree, TreeData{Head: big, TreeHash: "t"}),
 		"delta.finding.file": delta(Delta{Findings: []Finding{{IssueText: "i", File: big}}}),
 		"delta.bug.id":       delta(Delta{Confirmed: []Bug{{ID: big, Desc: "d"}}}),
diff --git a/internal/fsm/run/types.go b/internal/fsm/run/types.go
index 8715738..c03135b 100644
--- a/internal/fsm/run/types.go
+++ b/internal/fsm/run/types.go
@@ -51,6 +51,7 @@ const (
 	MaxAllowedCmds = 16
 	MaxArgv        = 32
 	MaxFileHashes  = 64
+	MaxEnv         = 16
 	MaxDeltaList   = 256
 	MaxWarnings    = 1024
 
@@ -126,6 +127,8 @@ type AllowedCmd struct {
 	Name       string            `json:"name"`
 	Argv       []string          `json:"argv"`
 	FileHashes map[string]string `json:"file_hashes"`
+	TimeoutMS  int64             `json:"timeout_ms,omitempty"`
+	Env        []string          `json:"env,omitempty"` // extra environment names passed through (consent-covered)
 }
 
 // Delta is what a node's Reduce produced; Fold applies it (§4.3). It carries no tokens.
@@ -300,6 +303,10 @@ func SnapshotEqualIgnoringSeq(a, b Snapshot) bool {
 	return bytes.Equal(marshalCanonical(a), marshalCanonical(b))
 }
 
+// MarshalCanonical encodes v with HTML escaping disabled and no trailing
+// newline — the encoding every struct→JSON path in internal/fsm must use.
+func MarshalCanonical(v any) []byte { return marshalCanonical(v) }
+
 // marshalCanonical encodes v with HTML escaping disabled and no trailing newline.
 func marshalCanonical(v any) []byte {
 	var buf bytes.Buffer
diff --git a/internal/fsm/run/types_test.go b/internal/fsm/run/types_test.go
index 61976aa..e370483 100644
--- a/internal/fsm/run/types_test.go
+++ b/internal/fsm/run/types_test.go
@@ -209,3 +209,9 @@ func TestReasonCodeTable(t *testing.T) {
 		}
 	}
 }
+
+func TestMarshalCanonicalExported(t *testing.T) {
+	if got := string(MarshalCanonical(map[string]string{"a": "<b>"})); got != `{"a":"<b>"}` {
+		t.Fatalf("got %s", got)
+	}
+}


--- docs/tasks/m1-m6-fsm-packages.md
+# M1–M6: internal/fsm core packages
+
+Implement `internal/fsm/{errs,converge,gate,workflow,machine,cmdexec,judge,mockai,kind}` per
+`docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md` (r4) and `docs/specs/2026-08-27-metareview-0.9.0-fsm-judge-kinds.md`
+(r5), test-first, under the combined coverage gate (`tests/coverage.sh`), reviewed per commit range (≤ 120 KB each).
+
+## Acceptance
+
+- Every §7/§8 test row has a discriminating test (literal pins; goldens regression-only behind an env flag).
+- `go test ./internal/fsm/...` passes; every `internal/fsm/*` package at exactly 100% statements.
+- `bash tests/coverage.sh` passes (legacy floor held).
+- Dependency direction per spec 2 §1 (machine imports no kinds/judge/cmdexec/workflows).
+- Every LLM/shell effect behind an interface; no shell, pinned argv, exact env in `cmdexec`.
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

