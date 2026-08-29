# metareview task-done context

Run ID: `mrv-20260827-082928631887000-task-done-m1-m6-fsm-packages-a99c72f1`

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

- Base: `1823bcfa2a86965a6435d4e64e698bb4041fcb98`
- Head: `07bdc1f4b47740066107f86a6c53ee35c103936b`
- Branch: ``
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `34393`
- Filtered diff bytes: `11330`
- Risk level: `none`
- Generated files excluded: docs/metareview/context/mrv-20260827-070456832125000-artifact-2026-08-27-metareview-0-9-0-fsm-core-a0b8592f-context.md, docs/metareview/context/mrv-20260827-070456908813000-artifact-2026-08-27-metareview-0-9-0-fsm-judge-kinds-33d63bfb-context.md, docs/metareview/reviews/mrv-20260827-070456832125000-artifact-2026-08-27-metareview-0-9-0-fsm-core-a0b8592f.md, docs/metareview/reviews/mrv-20260827-070456908813000-artifact-2026-08-27-metareview-0-9-0-fsm-judge-kinds-33d63bfb.md



## Review Manifest

- Manifest verdict: `NEEDS_REVISION`
- Source manifest hash: `89ba64d5377e1e0d`
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- docs/tasks/m1-m6-fsm-packages.md
- internal/fsm/converge/converge.go
- internal/fsm/converge/converge_test.go
- internal/fsm/workflow/resolve.go
- internal/fsm/workflow/workflow.go
- internal/fsm/workflow/workflow_test.go

### Path Dispositions
- docs/metareview/context/mrv-20260827-070456832125000-artifact-2026-08-27-metareview-0-9-0-fsm-core-a0b8592f-context.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/context/mrv-20260827-070456908813000-artifact-2026-08-27-metareview-0-9-0-fsm-judge-kinds-33d63bfb-context.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/reviews/mrv-20260827-070456832125000-artifact-2026-08-27-metareview-0-9-0-fsm-core-a0b8592f.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/reviews/mrv-20260827-070456908813000-artifact-2026-08-27-metareview-0-9-0-fsm-judge-kinds-33d63bfb.md: generated (metareview generated review artifact excluded from source manifest)

### Shards
- shard-01: docs/tasks/m1-m6-fsm-packages.md, internal/fsm/converge/converge.go, internal/fsm/converge/converge_test.go, internal/fsm/workflow/resolve.go, internal/fsm/workflow/workflow.go, internal/fsm/workflow/workflow_test.go

### Manifest Blockers
- missing shard result for shard-01

## Changed Files

- internal/fsm/converge/converge.go
- internal/fsm/converge/converge_test.go
- internal/fsm/workflow/resolve.go
- internal/fsm/workflow/workflow.go
- internal/fsm/workflow/workflow_test.go
- docs/tasks/m1-m6-fsm-packages.md

## Diff

```diff
diff --git a/internal/fsm/converge/converge.go b/internal/fsm/converge/converge.go
index 64962be..4e203fd 100644
--- a/internal/fsm/converge/converge.go
+++ b/internal/fsm/converge/converge.go
@@ -56,6 +56,12 @@ type Predicate interface {
 	Evaluate(ctx context.Context, s run.Snapshot) (Result, error)
 }
 
+// MaxDepth bounds nesting so composite names stay within run.MaxShort.
+const MaxDepth = 4
+
+// MaxAtoms bounds the number of leaves for the same reason.
+const MaxAtoms = 32
+
 // Error codes produced by this package.
 const (
 	CodeBadConvergence   = "ERR_BAD_CONVERGENCE"
@@ -95,6 +101,9 @@ func parse(node *yaml.Node, runner Runner, cmdNames []string, top bool, depth in
 		}
 		return parse(node.Content[0], runner, cmdNames, true, 0)
 	}
+	if depth > MaxDepth {
+		return nil, bad(fmt.Sprintf("line %d: convergence tree deeper than %d", node.Line, MaxDepth))
+	}
 	if node.Kind == yaml.ScalarNode {
 		// Bare form: `no_fixation_progress` / `all_fixed` (the shipped YAMLs use it).
 		switch node.Value {
@@ -147,8 +156,8 @@ func parse(node *yaml.Node, runner Runner, cmdNames []string, top bool, depth in
 		}
 		return &cmdAtom{name: name, runner: runner}, nil
 	case "any", "all":
-		if val.Kind != yaml.SequenceNode || len(val.Content) == 0 {
-			return nil, bad(fmt.Sprintf("line %d: %s must be a non-empty list", node.Line, key))
+		if val.Kind != yaml.SequenceNode || len(val.Content) == 0 || len(val.Content) > MaxAtoms {
+			return nil, bad(fmt.Sprintf("line %d: %s must be a list of 1..%d predicates", node.Line, key, MaxAtoms))
 		}
 		kids := make([]Predicate, 0, len(val.Content))
 		for _, c := range val.Content {
@@ -311,8 +320,11 @@ func (c *compound) Evaluate(ctx context.Context, s run.Snapshot) (Result, error)
 
 type not struct{ inner Predicate }
 
-func (n *not) Name() string       { return "not(" + n.inner.Name() + ")" }
-func (n *not) Class() run.Outcome { return n.inner.Class() }
+func (n *not) Name() string { return "not(" + n.inner.Name() + ")" }
+
+// Class of a negation is always custom: inverting an atom must never mint a
+// `fixed` or `stalled` classification (plan C3).
+func (n *not) Class() run.Outcome { return run.OutcomeCustom }
 func (n *not) Evaluate(ctx context.Context, s run.Snapshot) (Result, error) {
 	r, err := n.inner.Evaluate(ctx, s)
 	if err != nil {
diff --git a/internal/fsm/converge/converge_test.go b/internal/fsm/converge/converge_test.go
index 709edf6..aa843a9 100644
--- a/internal/fsm/converge/converge_test.go
+++ b/internal/fsm/converge/converge_test.go
@@ -40,7 +40,8 @@ func (f *fakeRunner) Run(_ context.Context, name string, stdin []byte) (CmdResul
 func intp(i int) *int { return &i }
 
 func snap(iter int, unfixed int, prev *int, tokens int64, found int) run.Snapshot {
-	s := run.Snapshot{Iteration: iter, Unfixed: unfixed, PrevUnfixed: prev, Tokens: run.TokenTotals{Input: tokens}}
+	// tokens are spread over the non-Input counters so a Total() that only reads Input fails.
+	s := run.Snapshot{Iteration: iter, Unfixed: unfixed, PrevUnfixed: prev, Tokens: run.TokenTotals{Output: tokens / 2, CacheRead: tokens - tokens/2 - tokens/4, Reasoning: tokens / 4}}
 	for i := 0; i < found; i++ {
 		s.AllFound = append(s.AllFound, run.Bug{ID: strings.Repeat("a", 12), Desc: "d"})
 	}
@@ -124,7 +125,7 @@ func TestC2CmdAtom(t *testing.T) {
 	if got.Vars["JUDGE"] != "sha256:"+hex.EncodeToString(sum[:]) || strings.Contains(string(fr.stdins[0]), "secret-model") {
 		t.Fatalf("payload not redacted: %s", fr.stdins[0])
 	}
-	if got.Iteration != 2 || got.Tokens.Input != 7 {
+	if got.Iteration != 2 || got.Tokens.Total() != 7 {
 		t.Fatal("payload carries the snapshot")
 	}
 	if strings.Contains(string(fr.stdins[0]), `"big"`) {
@@ -134,13 +135,17 @@ func TestC2CmdAtom(t *testing.T) {
 		t.Fatal("Payload must not mutate the snapshot")
 	}
 
-	// stop:false path
+	// stop:false path, and a missing reason is legal
 	fr.res = CmdResult{Stdout: []byte(`{"stop": false, "reason": ""}`)}
 	if r, err := p.Evaluate(ctx, s); err != nil || r.Stop || r.Reason != "" {
 		t.Fatalf("stop false: %+v %v", r, err)
 	}
+	fr.res = CmdResult{Stdout: []byte(`{"stop": true}`)}
+	if r, err := p.Evaluate(ctx, s); err != nil || !r.Stop || r.Reason != "" {
+		t.Fatalf("reason optional: %+v %v", r, err)
+	}
 	// invalid outputs
-	for _, out := range []string{`{"stop": "yes"}`, `not json`, `{"stop": true, "extra": 1}`, ``} {
+	for _, out := range []string{`{"stop": "yes"}`, `not json`, `{"stop": true, "extra": 1}`, ``, `{"stop": true, "reason": 5}`} {
 		fr.res = CmdResult{Stdout: []byte(out)}
 		_, err := p.Evaluate(ctx, s)
 		if !errs.Is(err, CodeCmdOutputInvalid) {
@@ -196,11 +201,11 @@ func TestC3Compose(t *testing.T) {
 	if err != nil {
 		t.Fatal(err)
 	}
-	if notP.Name() != "not(budget)" || notP.Class() != run.OutcomeOverflow {
-		t.Fatal("not name/class")
+	if notP.Name() != "not(budget)" || notP.Class() != run.OutcomeCustom {
+		t.Fatal("not name/class: negation is always custom")
 	}
 	r, _ = notP.Evaluate(ctx, quiet)
-	if !r.Stop || r.Atom != "not(budget)" || r.Reason == "" {
+	if !r.Stop || r.Atom != "not(budget)" || r.Class != run.OutcomeCustom || r.Reason == "" {
 		t.Fatalf("not inverts: %+v", r)
 	}
 	r, _ = notP.Evaluate(ctx, snap(0, 1, nil, 10, 1))
@@ -251,6 +256,14 @@ func TestC4ValidateAndParseErrors(t *testing.T) {
 		{"all_fixed-under-all", "all: [{not: all_fixed}, {max_iterations: 5}]"},
 		{"all_fixed-map-under-all", "all: [{all_fixed: true}, {max_iterations: 5}]"},
 		{"all_fixed-nested-any", "any: [{any: [all_fixed]}]"},
+		{"too-deep", "not: {not: {not: {not: {not: {max_iterations: 1}}}}}"},
+		{"too-wide", "any: [" + strings.Repeat("{max_iterations: 1}, ", MaxAtoms) + "{max_iterations: 1}]"},
+	}
+	if err := Validate(node(t, "any: ["+strings.Repeat("{max_iterations: 1}, ", MaxAtoms-1)+"{max_iterations: 1}]"), nil); err != nil {
+		t.Fatalf("at MaxAtoms: %v", err)
+	}
+	if err := Validate(node(t, "not: {not: {not: {not: {max_iterations: 1}}}}"), nil); err != nil {
+		t.Fatalf("at MaxDepth: %v", err)
 	}
 	// all_fixed is legal at the top level and directly under a top-level any
 	for _, ok := range []string{"all_fixed", "{all_fixed: true}", "any: [all_fixed, {max_iterations: 5}]"} {
diff --git a/internal/fsm/workflow/resolve.go b/internal/fsm/workflow/resolve.go
index 16912dc..8c22ca6 100644
--- a/internal/fsm/workflow/resolve.go
+++ b/internal/fsm/workflow/resolve.go
@@ -3,7 +3,6 @@ package workflow
 import (
 	"crypto/sha256"
 	"encoding/hex"
-	"encoding/json"
 	"os"
 	"path/filepath"
 	"sort"
@@ -135,9 +134,7 @@ func ResolveCmds(w *Workflow, workDir string, lookPath func(string) (string, err
 func CmdsSHA256(cmds []run.AllowedCmd) string {
 	sorted := append([]run.AllowedCmd{}, cmds...)
 	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
-	raw, _ := json.Marshal(sorted)
-	canon, _ := run.Canonical(raw)
-	sum := sha256.Sum256(canon)
+	sum := sha256.Sum256(run.MarshalCanonical(sorted))
 	return hex.EncodeToString(sum[:])
 }
 
diff --git a/internal/fsm/workflow/workflow.go b/internal/fsm/workflow/workflow.go
index f0fec0b..f6052ed 100644
--- a/internal/fsm/workflow/workflow.go
+++ b/internal/fsm/workflow/workflow.go
@@ -114,7 +114,7 @@ var (
 	cmdPattern   = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
 	varRef       = regexp.MustCompile(`\$(\$|[A-Z_][A-Z0-9_]*)`)
 	execModes    = map[string]bool{"inline": true, "subagent": true, "fork": true}
-	reservedEnv  = map[string]bool{"PATH": true, "HOME": true, "LANG": true, "TMPDIR": true, "BASH_ENV": true, "ENV": true, "PYTHONPATH": true, "NODE_OPTIONS": true, "PERL5OPT": true}
+	reservedEnv  = map[string]bool{"PATH": true, "HOME": true, "LANG": true, "TMPDIR": true, "BASH_ENV": true, "ENV": true, "PYTHONPATH": true, "PYTHONSTARTUP": true, "PYTHONHOME": true, "NODE_OPTIONS": true, "NODE_PATH": true, "PERL5OPT": true, "PERL5LIB": true, "RUBYOPT": true, "RUBYLIB": true, "JAVA_TOOL_OPTIONS": true, "SHELLOPTS": true, "PS4": true, "IFS": true, "CDPATH": true, "GLOBIGNORE": true, "PROMPT_COMMAND": true}
 	reservedPfx  = []string{"MRV_", "LD_", "DYLD_", "GIT_"}
 )
 
@@ -470,6 +470,9 @@ func (w *Workflow) validateGraph() error {
 	if !w.hasState(FailedState) || w.Nodes[FailedState] != nil {
 		return invalid("failed_reserved", "states", "failed must be declared and carry no node")
 	}
+	if len(w.Outgoing(w.Initial)) == 0 {
+		return invalid("initial_terminal", "states."+string(w.Initial), "the initial state needs an outgoing transition")
+	}
 	for s, n := range w.Nodes {
 		if w.IsTerminal(s) {
 			return invalid("terminal_with_node", "nodes."+n.Name, "terminal states carry no node")
diff --git a/internal/fsm/workflow/workflow_test.go b/internal/fsm/workflow/workflow_test.go
index c42ddc0..15d9423 100644
--- a/internal/fsm/workflow/workflow_test.go
+++ b/internal/fsm/workflow/workflow_test.go
@@ -224,6 +224,8 @@ func TestW2Reasons(t *testing.T) {
 		{"outcome-on-nonterminal", "outcome_on_nonterminal", "transitions[1]", edit("{ from: discover, to: adjudicate, gate: findings_nonempty }", "{ from: discover, to: adjudicate, gate: findings_nonempty, outcome: clean }")},
 		{"bad-outcome", "bad_outcome", "transitions[0]", edit("gate: findings_empty, outcome: clean }", "gate: findings_empty, outcome: great }")},
 		{"bad-outcome-failed", "bad_outcome", "transitions[0]", edit("gate: findings_empty, outcome: clean }", "gate: findings_empty, outcome: failed }")},
+		{"initial-terminal", "initial_terminal", "states.discover", strings.Replace(strings.Replace(example, "  - { from: discover, to: done, gate: findings_empty, outcome: clean }\n", "", 1), "  - { from: discover, to: adjudicate, gate: findings_nonempty }\n", "  - { from: fix, to: adjudicate, gate: findings_nonempty }\n", 1)},
+		{"bad-env-shellopts", "bad_env", "cmds.notify", edit("env: [SLACK_WEBHOOK]", "env: [SHELLOPTS]")},
 		{"unreachable-state", "unreachable_state", "states.fix", edit("  - { from: adjudicate, to: fix, gate: confirmed_nonempty }\n", "")},
 		{"loop-count", "loop_count", "transitions", edit("{ from: adjudicate, to: fix, gate: confirmed_nonempty }", "{ from: adjudicate, to: fix, gate: confirmed_nonempty, loop: true }")},
 		{"loop-not-cycle", "loop_not_cycle", "transitions", strings.Replace(edit("{ from: verify, to: discover, gate: bugs_remain, loop: true }", "{ from: verify, to: side, gate: bugs_remain, loop: true }\n  - { from: side, to: done, gate: findings_empty, outcome: clean }"), "verify, done, failed]", "verify, side, done, failed]", 1)},


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

