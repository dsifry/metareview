# metareview task-done context

Run ID: `mrv-20260827-083955673209000-task-done-m1-m6-fsm-packages-a99c72f1`

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

- Base: `147e6633dcd16f9cb3cbdfdc472f2499b48f591b`
- Head: `1823bcfa2a86965a6435d4e64e698bb4041fcb98`
- Branch: ``
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `76504`
- Filtered diff bytes: `76504`
- Risk level: `none`



## Review Manifest

- Manifest verdict: `NEEDS_REVISION`
- Source manifest hash: `6db31002ad80ce48`
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- docs/tasks/m1-m6-fsm-packages.md
- internal/fsm/converge/converge.go
- internal/fsm/converge/converge_test.go
- internal/fsm/gate/fake.go
- internal/fsm/gate/gate_test.go
- internal/fsm/gate/gates.go
- internal/fsm/gate/git.go
- internal/fsm/run/errors.go
- internal/fsm/run/fold.go
- internal/fsm/run/fold_test.go
- internal/fsm/run/types.go
- internal/fsm/run/types_test.go
- internal/fsm/workflow/resolve.go
- internal/fsm/workflow/testdata/cmds-preimage.json
- internal/fsm/workflow/testdata/cmds-preimage.sha256
- internal/fsm/workflow/workflow.go
- internal/fsm/workflow/workflow_test.go
- workflows/sdlc-loop.yaml

### Shards
- shard-01: internal/fsm/converge/converge.go, internal/fsm/workflow/resolve.go, internal/fsm/workflow/workflow.go, internal/fsm/workflow/workflow_test.go
- shard-02: docs/tasks/m1-m6-fsm-packages.md, internal/fsm/converge/converge_test.go, internal/fsm/gate/fake.go, internal/fsm/gate/gate_test.go, internal/fsm/gate/gates.go, internal/fsm/gate/git.go, internal/fsm/run/errors.go, internal/fsm/run/fold.go, internal/fsm/run/fold_test.go, internal/fsm/run/types.go, internal/fsm/run/types_test.go, internal/fsm/workflow/testdata/cmds-preimage.json, internal/fsm/workflow/testdata/cmds-preimage.sha256, workflows/sdlc-loop.yaml

### Manifest Blockers
- missing cross-shard result
- missing shard result for shard-01
- missing shard result for shard-02

## Changed Files

- internal/fsm/converge/converge.go
- internal/fsm/converge/converge_test.go
- internal/fsm/gate/fake.go
- internal/fsm/gate/gate_test.go
- internal/fsm/gate/gates.go
- internal/fsm/gate/git.go
- internal/fsm/run/errors.go
- internal/fsm/run/fold.go
- internal/fsm/run/fold_test.go
- internal/fsm/run/types.go
- internal/fsm/run/types_test.go
- internal/fsm/workflow/resolve.go
- internal/fsm/workflow/testdata/cmds-preimage.json
- internal/fsm/workflow/testdata/cmds-preimage.sha256
- internal/fsm/workflow/workflow.go
- internal/fsm/workflow/workflow_test.go
- workflows/sdlc-loop.yaml
- docs/tasks/m1-m6-fsm-packages.md

## Diff

```diff
diff --git a/internal/fsm/converge/converge.go b/internal/fsm/converge/converge.go
index 9b9ddab..64962be 100644
--- a/internal/fsm/converge/converge.go
+++ b/internal/fsm/converge/converge.go
@@ -64,15 +64,16 @@ const (
 )
 
 // Validate checks the structure of a convergence tree without binding it.
-// cmdNames lists the declared command names a cmd atom may reference.
+// cmdNames lists the declared command names a cmd atom may reference (nil
+// accepts any name; workflow.Parse always passes the declared list).
 func Validate(node *yaml.Node, cmdNames []string) error {
-	_, err := parse(node, nil, cmdNames)
+	_, err := parse(node, nil, cmdNames, true, 0)
 	return err
 }
 
 // Parse validates and binds a convergence tree; cmd atoms call runner.
 func Parse(node *yaml.Node, runner Runner) (Predicate, error) {
-	return parse(node, runner, nil)
+	return parse(node, runner, nil, true, 0)
 }
 
 func bad(detail string) error {
@@ -81,8 +82,10 @@ func bad(detail string) error {
 
 // parse walks the tree. When cmdNames is nil every cmd name is accepted
 // (Parse trusts workflow.Parse's earlier Validate); otherwise names must be
-// declared.
-func parse(node *yaml.Node, runner Runner, cmdNames []string) (Predicate, error) {
+// declared. top is true at the root and for direct children of a root-level
+// `any` (depth tracks nesting): `all_fixed` is legal only there, so a give-up
+// can never carry the class `fixed` through `all`/`not` (plan C3).
+func parse(node *yaml.Node, runner Runner, cmdNames []string, top bool, depth int) (Predicate, error) {
 	if node == nil || node.Kind == 0 {
 		return nil, bad("empty convergence")
 	}
@@ -90,12 +93,15 @@ func parse(node *yaml.Node, runner Runner, cmdNames []string) (Predicate, error)
 		if len(node.Content) != 1 {
 			return nil, bad("empty convergence")
 		}
-		return parse(node.Content[0], runner, cmdNames)
+		return parse(node.Content[0], runner, cmdNames, true, 0)
 	}
 	if node.Kind == yaml.ScalarNode {
 		// Bare form: `no_fixation_progress` / `all_fixed` (the shipped YAMLs use it).
 		switch node.Value {
 		case "all_fixed":
+			if !top {
+				return nil, bad(fmt.Sprintf("line %d: all_fixed may only appear at the top level or directly under a top-level any", node.Line))
+			}
 			return allFixed{}, nil
 		case "no_fixation_progress":
 			return noProgress{}, nil
@@ -113,6 +119,9 @@ func parse(node *yaml.Node, runner Runner, cmdNames []string) (Predicate, error)
 			return nil, bad(fmt.Sprintf("line %d: %s must be true", node.Line, key))
 		}
 		if key == "all_fixed" {
+			if !top {
+				return nil, bad(fmt.Sprintf("line %d: all_fixed may only appear at the top level or directly under a top-level any", node.Line))
+			}
 			return allFixed{}, nil
 		}
 		return noProgress{}, nil
@@ -143,7 +152,7 @@ func parse(node *yaml.Node, runner Runner, cmdNames []string) (Predicate, error)
 		}
 		kids := make([]Predicate, 0, len(val.Content))
 		for _, c := range val.Content {
-			p, err := parse(c, runner, cmdNames)
+			p, err := parse(c, runner, cmdNames, key == "any" && depth == 0, depth+1)
 			if err != nil {
 				return nil, err
 			}
@@ -151,7 +160,7 @@ func parse(node *yaml.Node, runner Runner, cmdNames []string) (Predicate, error)
 		}
 		return &compound{op: key, kids: kids}, nil
 	case "not":
-		inner, err := parse(val, runner, cmdNames)
+		inner, err := parse(val, runner, cmdNames, false, depth+1)
 		if err != nil {
 			return nil, err
 		}
diff --git a/internal/fsm/converge/converge_test.go b/internal/fsm/converge/converge_test.go
index b30e948..709edf6 100644
--- a/internal/fsm/converge/converge_test.go
+++ b/internal/fsm/converge/converge_test.go
@@ -247,6 +247,16 @@ func TestC4ValidateAndParseErrors(t *testing.T) {
 		{"any-scalar", "any: x"},
 		{"all-bad-child", "all: [{max_iterations: -1}]"},
 		{"not-bad-child", "not: {budget: 0}"},
+		{"all_fixed-under-not", "not: all_fixed"},
+		{"all_fixed-under-all", "all: [{not: all_fixed}, {max_iterations: 5}]"},
+		{"all_fixed-map-under-all", "all: [{all_fixed: true}, {max_iterations: 5}]"},
+		{"all_fixed-nested-any", "any: [{any: [all_fixed]}]"},
+	}
+	// all_fixed is legal at the top level and directly under a top-level any
+	for _, ok := range []string{"all_fixed", "{all_fixed: true}", "any: [all_fixed, {max_iterations: 5}]"} {
+		if err := Validate(node(t, ok), nil); err != nil {
+			t.Errorf("%s: %v", ok, err)
+		}
 	}
 	for _, c := range bad {
 		err := Validate(node(t, c.yaml), []string{"notify"})
diff --git a/internal/fsm/gate/fake.go b/internal/fsm/gate/fake.go
index 5175a98..24ac121 100644
--- a/internal/fsm/gate/fake.go
+++ b/internal/fsm/gate/fake.go
@@ -3,6 +3,8 @@ package gate
 import (
 	"context"
 	"fmt"
+
+	"github.com/dsifry/metareview/internal/fsm/errs"
 )
 
 // Fake is a scripted Git for tests. Unset answers return zero values; Err
@@ -41,7 +43,7 @@ func (f *Fake) RevParse(_ context.Context, ref string) (string, error) {
 	if sha, ok := f.Refs[ref]; ok {
 		return sha, nil
 	}
-	return "", fmt.Errorf("%s: unknown ref %q", CodeGit, ref)
+	return "", errs.E(CodeGit, "unknown ref", "ref", ref, "op", "rev-parse")
 }
 
 func (f *Fake) IsAncestor(_ context.Context, a, b string) (bool, error) {
diff --git a/internal/fsm/gate/gate_test.go b/internal/fsm/gate/gate_test.go
index 868d8e8..5aca880 100644
--- a/internal/fsm/gate/gate_test.go
+++ b/internal/fsm/gate/gate_test.go
@@ -44,6 +44,8 @@ func TestG1Gates(t *testing.T) {
 		{"confirmed_empty", run.Snapshot{}, ""}, {"confirmed_empty", cnf, CodeConfirmedPresent},
 		{"all_fixed", fixed, ""}, {"all_fixed", remain, CodeBugsRemain}, {"all_fixed", run.Snapshot{}, CodeBugsRemain},
 		{"bugs_remain", remain, ""}, {"bugs_remain", run.Snapshot{}, ""}, {"bugs_remain", fixed, CodeAllFixed},
+		{"nothing_found", run.Snapshot{}, ""}, {"nothing_found", fnd, CodeFindingsPresent}, {"nothing_found", run.Snapshot{AllFound: bugs(1)}, CodeBugsKnown},
+		{"nothing_confirmed", run.Snapshot{}, ""}, {"nothing_confirmed", cnf, CodeConfirmedPresent}, {"nothing_confirmed", run.Snapshot{AllFound: bugs(1), Unfixed: 1}, CodeBugsKnown},
 	}
 	for _, c := range cases {
 		g, ok := Builtin(c.gate)
@@ -64,7 +66,7 @@ func TestG1Gates(t *testing.T) {
 	if _, ok := Builtin("nope"); ok {
 		t.Fatal("unknown gate")
 	}
-	want := []string{"all_fixed", "bugs_remain", "commit_exists", "confirmed_empty", "confirmed_nonempty", "findings_empty", "findings_nonempty"}
+	want := []string{"all_fixed", "bugs_remain", "commit_exists", "confirmed_empty", "confirmed_nonempty", "findings_empty", "findings_nonempty", "nothing_confirmed", "nothing_found"}
 	if strings.Join(Names(), ",") != strings.Join(want, ",") {
 		t.Fatalf("Names: %v", Names())
 	}
@@ -418,7 +420,7 @@ func TestG2ExecErrorBranches(t *testing.T) {
 		t.Fatal("RealExec must report an unrunnable process")
 	}
 	// scrubEnv drops GIT_* overrides
-	got := scrubEnv([]string{"GIT_DIR=x", "GIT_CONFIG_COUNT=1", "GIT_WORK_TREE=y", "GIT_INDEX_FILE=z", "GIT_EXTERNAL_DIFF=e", "GIT_OBJECT_DIRECTORY=o", "GIT_ALTERNATE_OBJECT_DIRECTORIES=a", "PATH=p", "HOME=h"})
+	got := scrubEnv([]string{"GIT_DIR=x", "GIT_CONFIG_COUNT=1", "GIT_WORK_TREE=y", "GIT_INDEX_FILE=z", "GIT_EXTERNAL_DIFF=e", "GIT_OBJECT_DIRECTORY=o", "GIT_ALTERNATE_OBJECT_DIRECTORIES=a", "GIT_COMMON_DIR=c", "GIT_TRACE=1", "PATH=p", "HOME=h"})
 	if strings.Join(got, ",") != "PATH=p,HOME=h" {
 		t.Fatalf("scrub: %v", got)
 	}
@@ -436,7 +438,7 @@ func TestG4FakeContract(t *testing.T) {
 	if r, _ := f.RevParse(ctx, "main"); r != shaA {
 		t.Fatal("revparse main")
 	}
-	if _, err := f.RevParse(ctx, "nope"); err == nil {
+	if _, err := f.RevParse(ctx, "nope"); !errs.Is(err, CodeGit) {
 		t.Fatal("unknown ref")
 	}
 	if ok, _ := f.IsAncestor(ctx, shaB, shaA); !ok {
diff --git a/internal/fsm/gate/gates.go b/internal/fsm/gate/gates.go
index c098bcf..6ba3433 100644
--- a/internal/fsm/gate/gates.go
+++ b/internal/fsm/gate/gates.go
@@ -19,6 +19,7 @@ const (
 	CodeGateInapplicable = "ERR_GATE_INAPPLICABLE"
 	CodeBugsRemain       = "ERR_BUGS_REMAIN"
 	CodeAllFixed         = "ERR_ALL_FIXED"
+	CodeBugsKnown        = "ERR_BUGS_KNOWN"
 )
 
 // Gate evaluates a snapshot; nil means pass.
@@ -54,6 +55,27 @@ var builtin = map[string]Gate{
 		}
 		return fail("confirmed_empty", CodeConfirmedPresent, fmt.Sprintf("%d confirmed bugs present", len(s.Confirmed)))
 	},
+	// nothing_found / nothing_confirmed are the iteration-0 clean exits: they
+	// refuse once any bug is known, so a later discovery miss cannot end a
+	// loop as clean while bugs remain.
+	"nothing_found": func(_ context.Context, s run.Snapshot, _ Git) *run.GateError {
+		if len(s.AllFound) > 0 {
+			return fail("nothing_found", CodeBugsKnown, fmt.Sprintf("%d bugs already known (%d unfixed)", len(s.AllFound), s.Unfixed))
+		}
+		if len(s.Findings) > 0 {
+			return fail("nothing_found", CodeFindingsPresent, fmt.Sprintf("%d findings present", len(s.Findings)))
+		}
+		return nil
+	},
+	"nothing_confirmed": func(_ context.Context, s run.Snapshot, _ Git) *run.GateError {
+		if len(s.AllFound) > 0 {
+			return fail("nothing_confirmed", CodeBugsKnown, fmt.Sprintf("%d bugs already known (%d unfixed)", len(s.AllFound), s.Unfixed))
+		}
+		if len(s.Confirmed) > 0 {
+			return fail("nothing_confirmed", CodeConfirmedPresent, fmt.Sprintf("%d confirmed bugs present", len(s.Confirmed)))
+		}
+		return nil
+	},
 	"commit_exists": commitExists,
 	"all_fixed": func(_ context.Context, s run.Snapshot, _ Git) *run.GateError {
 		if converge.AllFixed(s) {
diff --git a/internal/fsm/gate/git.go b/internal/fsm/gate/git.go
index 78db26e..371fee3 100644
--- a/internal/fsm/gate/git.go
+++ b/internal/fsm/gate/git.go
@@ -55,7 +55,7 @@ type Exec func(ctx context.Context, dir string, env []string, args ...string) (s
 // RealExec shells out to git with prompts, external diff drivers, and
 // config injection disabled.
 func RealExec(ctx context.Context, dir string, env []string, args ...string) ([]byte, []byte, int, error) {
-	cmd := exec.CommandContext(ctx, "git", append([]string{"-c", "core.fsmonitor=false", "-c", "diff.external=", "--no-pager"}, args...)...)
+	cmd := exec.CommandContext(ctx, "git", append([]string{"-c", "core.fsmonitor=false", "-c", "diff.external=", "-c", "core.excludesFile=", "-c", "core.attributesFile=", "--no-pager"}, args...)...)
 	cmd.Dir = dir
 	cmd.Env = append(scrubEnv(cmd.Environ()), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C", "GIT_EXTERNAL_DIFF=", "GIT_CONFIG_NOSYSTEM=1")
 	cmd.Env = append(cmd.Env, env...)
@@ -72,14 +72,13 @@ func RealExec(ctx context.Context, dir string, env []string, args ...string) ([]
 	return []byte(out.String()), []byte(errb.String()), code, err
 }
 
-// scrubEnv drops GIT_* overrides that could redirect git to another
-// repository or inject configuration.
+// scrubEnv drops every GIT_* variable so nothing in the caller's environment
+// can redirect git to another repository, inject configuration, or change
+// object/ref resolution. RealExec re-adds the few it needs.
 func scrubEnv(environ []string) []string {
 	out := environ[:0:0]
 	for _, kv := range environ {
-		k, _, _ := strings.Cut(kv, "=")
-		switch {
-		case k == "GIT_DIR", k == "GIT_WORK_TREE", k == "GIT_INDEX_FILE", k == "GIT_EXTERNAL_DIFF", k == "GIT_OBJECT_DIRECTORY", k == "GIT_ALTERNATE_OBJECT_DIRECTORIES", strings.HasPrefix(k, "GIT_CONFIG"):
+		if strings.HasPrefix(kv, "GIT_") {
 			continue
 		}
 		out = append(out, kv)
diff --git a/internal/fsm/run/errors.go b/internal/fsm/run/errors.go
index 368b951..b863a39 100644
--- a/internal/fsm/run/errors.go
+++ b/internal/fsm/run/errors.go
@@ -38,6 +38,7 @@ const (
 	ReasonUnsanctionedCmd    = "unsanctioned_cmd"
 	ReasonBadOutcome         = "bad_outcome"
 	ReasonTokensNegative     = "tokens_negative"
+	ReasonTokensTooLarge     = "tokens_too_large"
 )
 
 // CodeFor returns the FoldError code for a reason (§2.4 table).
diff --git a/internal/fsm/run/fold.go b/internal/fsm/run/fold.go
index dbdb84a..fe6cbca 100644
--- a/internal/fsm/run/fold.go
+++ b/internal/fsm/run/fold.go
@@ -148,12 +148,18 @@ func Apply(st FoldState, ev Event) (FoldState, error) {
 		if p.Tokens.Negative() {
 			return FoldState{}, foldErr(ReasonTokensNegative, ev)
 		}
+		if p.Tokens.TooLarge() {
+			return FoldState{}, foldErr(ReasonTokensTooLarge, ev)
+		}
 		next.indexes[k] = p.Index + 1
 		next.Tokens = next.Tokens.Add(p.Tokens)
 	case *TokenTotals:
 		if p.Negative() {
 			return FoldState{}, foldErr(ReasonTokensNegative, ev)
 		}
+		if p.TooLarge() {
+			return FoldState{}, foldErr(ReasonTokensTooLarge, ev)
+		}
 		next.Tokens = next.Tokens.Add(*p)
 	case *CmdCallData:
 		if !sanctioned(st.AllowedCmds, p.Name) {
diff --git a/internal/fsm/run/fold_test.go b/internal/fsm/run/fold_test.go
index 126b624..c867e03 100644
--- a/internal/fsm/run/fold_test.go
+++ b/internal/fsm/run/fold_test.go
@@ -1066,7 +1066,29 @@ func TestFoldTokensNegativeAndNextIndex(t *testing.T) {
 			t.Errorf("llm_call %s: %v", name, err)
 		}
 	}
-	if (TokenTotals{}).Negative() {
-		t.Fatal("zero is not negative")
+	if (TokenTotals{}).Negative() || (TokenTotals{}).TooLarge() {
+		t.Fatal("zero is neither negative nor too large")
+	}
+	for name, tok := range map[string]TokenTotals{"input": {Input: MaxTokenCounter + 1}, "cache_read": {CacheRead: MaxTokenCounter + 1}, "cache_create": {CacheCreate: MaxTokenCounter + 1}, "output": {Output: MaxTokenCounter + 1}, "reasoning": {Reasoning: MaxTokenCounter + 1}} {
+		b2 := NewBuilder(runA)
+		b2.Init(baseInit())
+		b2.Event(TypeTokens, tok)
+		if _, err := Fold(b2.Events()); err == nil || err.(*FoldError).Reason != ReasonTokensTooLarge {
+			t.Errorf("tokens too large %s: %v", name, err)
+		}
+		b3 := NewBuilder(runA)
+		b3.Init(baseInit())
+		b3.Event(TypeNodeOutput, out(`{}`), WithNode("n"))
+		b3.Event(TypeLLMCall, LLMCallData{Kind: "k", Model: "m", Index: 0, Verdict: json.RawMessage(`{}`), Tokens: tok}, WithNode("n"))
+		if _, err := Fold(b3.Events()); err == nil || err.(*FoldError).Reason != ReasonTokensTooLarge {
+			t.Errorf("llm_call too large %s: %v", name, err)
+		}
+	}
+	// at the cap is accepted
+	b4 := NewBuilder(runA)
+	b4.Init(baseInit())
+	b4.Event(TypeTokens, TokenTotals{Input: MaxTokenCounter})
+	if _, err := Fold(b4.Events()); err != nil {
+		t.Fatal(err)
 	}
 }
diff --git a/internal/fsm/run/types.go b/internal/fsm/run/types.go
index 36bf3fa..f6bc894 100644
--- a/internal/fsm/run/types.go
+++ b/internal/fsm/run/types.go
@@ -103,12 +103,20 @@ type TokenTotals struct {
 }
 
 // Add returns the field-wise sum.
+// MaxTokenCounter bounds every counter in one record so sums can never wrap.
+const MaxTokenCounter = int64(1) << 40
+
 // Negative reports whether any counter is below zero (rejected by Apply so a
 // driver cannot pay down a budget with negative records).
 func (t TokenTotals) Negative() bool {
 	return t.Input < 0 || t.CacheRead < 0 || t.CacheCreate < 0 || t.Output < 0 || t.Reasoning < 0
 }
 
+// TooLarge reports whether any counter exceeds MaxTokenCounter.
+func (t TokenTotals) TooLarge() bool {
+	return t.Input > MaxTokenCounter || t.CacheRead > MaxTokenCounter || t.CacheCreate > MaxTokenCounter || t.Output > MaxTokenCounter || t.Reasoning > MaxTokenCounter
+}
+
 func (t TokenTotals) Add(u TokenTotals) TokenTotals {
 	return TokenTotals{Input: t.Input + u.Input, CacheRead: t.CacheRead + u.CacheRead, CacheCreate: t.CacheCreate + u.CacheCreate, Output: t.Output + u.Output, Reasoning: t.Reasoning + u.Reasoning}
 }
@@ -224,7 +232,7 @@ func (s Snapshot) Clone() Snapshot {
 	if s.AllowedCmds != nil {
 		c.AllowedCmds = make([]AllowedCmd, len(s.AllowedCmds))
 		for i, a := range s.AllowedCmds {
-			c.AllowedCmds[i] = AllowedCmd{Name: a.Name, Argv: cloneStrings(a.Argv), FileHashes: cloneStringMap(a.FileHashes)}
+			c.AllowedCmds[i] = AllowedCmd{Name: a.Name, Argv: cloneStrings(a.Argv), FileHashes: cloneStringMap(a.FileHashes), TimeoutMS: a.TimeoutMS, Env: cloneStrings(a.Env)}
 		}
 	}
 	if s.Goldens != nil {
diff --git a/internal/fsm/run/types_test.go b/internal/fsm/run/types_test.go
index e370483..8b789a8 100644
--- a/internal/fsm/run/types_test.go
+++ b/internal/fsm/run/types_test.go
@@ -215,3 +215,15 @@ func TestMarshalCanonicalExported(t *testing.T) {
 		t.Fatalf("got %s", got)
 	}
 }
+
+func TestCloneKeepsAllowedCmdScalars(t *testing.T) {
+	s := Snapshot{AllowedCmds: []AllowedCmd{{Name: "c", Argv: []string{"/c"}, FileHashes: map[string]string{}, TimeoutMS: 1500, Env: []string{"A"}}}}
+	c := s.Clone()
+	if c.AllowedCmds[0].TimeoutMS != 1500 || len(c.AllowedCmds[0].Env) != 1 || c.AllowedCmds[0].Env[0] != "A" {
+		t.Fatalf("clone dropped fields: %+v", c.AllowedCmds[0])
+	}
+	c.AllowedCmds[0].Env[0] = "B"
+	if s.AllowedCmds[0].Env[0] != "A" {
+		t.Fatal("env must be copied")
+	}
+}
diff --git a/internal/fsm/workflow/resolve.go b/internal/fsm/workflow/resolve.go
new file mode 100644
index 0000000..16912dc
--- /dev/null
+++ b/internal/fsm/workflow/resolve.go
@@ -0,0 +1,200 @@
+package workflow
+
+import (
+	"crypto/sha256"
+	"encoding/hex"
+	"encoding/json"
+	"os"
+	"path/filepath"
+	"sort"
+	"strings"
+
+	"github.com/dsifry/metareview/internal/fsm/errs"
+	"github.com/dsifry/metareview/internal/fsm/run"
+)
+
+// Resolve substitutes $VAR tokens from vars (caller) over defaults and returns
+// a resolved copy plus the effective vars. calibration pins JUDGE/JUDGE_EFFORT
+// for declared vars and refuses caller values for them.
+func (w *Workflow) Resolve(vars map[string]string, calibration bool) (*Workflow, map[string]string, error) {
+	for name := range vars {
+		if _, ok := w.Vars[name]; !ok {
+			return nil, nil, errs.E(CodeVarUnknown, "unknown var "+name, "name", name)
+		}
+	}
+	effective := map[string]string{}
+	names := make([]string, 0, len(w.Vars))
+	for n := range w.Vars {
+		names = append(names, n)
+	}
+	sort.Strings(names)
+	pins := map[string]string{"JUDGE": CalibrationJudge, "JUDGE_EFFORT": CalibrationEffort}
+	for _, name := range names {
+		spec := w.Vars[name]
+		if pin, pinned := pins[name]; calibration && pinned {
+			if _, given := vars[name]; given {
+				return nil, nil, errs.E(CodeCalibrationPinned, "--calibration pins "+name, "name", name)
+			}
+			effective[name] = pin
+			continue
+		}
+		if v, ok := vars[name]; ok {
+			effective[name] = v
+			continue
+		}
+		if spec.Required {
+			return nil, nil, errs.E(CodeVarUnset, "required var "+name+" is unset", "name", name)
+		}
+		effective[name] = spec.Default
+	}
+	sub := func(s string) string {
+		return varRef.ReplaceAllStringFunc(s, func(m string) string {
+			if m == "$$" {
+				return "$"
+			}
+			return effective[m[1:]]
+		})
+	}
+	c := *w
+	c.Nodes = make(map[run.State]*Node, len(w.Nodes))
+	for s, n := range w.Nodes {
+		nn := *n
+		nn.Model, nn.Effort = sub(n.Model), sub(n.Effort)
+		nn.Params = make(map[string]any, len(n.Params))
+		for k, v := range n.Params {
+			switch x := v.(type) {
+			case string:
+				nn.Params[k] = sub(x)
+			case []any:
+				l := make([]any, len(x))
+				for i, e := range x {
+					if s, ok := e.(string); ok {
+						l[i] = sub(s)
+					} else {
+						l[i] = e
+					}
+				}
+				nn.Params[k] = l
+			default:
+				nn.Params[k] = v
+			}
+		}
+		c.Nodes[s] = &nn
+	}
+	c.Cmds = make(map[string]*CmdDecl, len(w.Cmds))
+	for name, d := range w.Cmds {
+		dd := *d
+		dd.Argv = make([]string, len(d.Argv))
+		for i, a := range d.Argv {
+			dd.Argv[i] = sub(a)
+		}
+		c.Cmds[name] = &dd
+	}
+	return &c, effective, nil
+}
+
+// ResolveCmds pins every declared command: argv[0] to an absolute path, every
+// argv element that names a regular file to its sha256. Returns the consent
+// list (sorted by name) and cmds_sha256 = sha256(Canonical(json)).
+func ResolveCmds(w *Workflow, workDir string, lookPath func(string) (string, error), hash func(string) (string, error)) ([]run.AllowedCmd, string, error) {
+	var out []run.AllowedCmd
+	for _, name := range w.cmdNames() {
+		d := w.Cmds[name]
+		argv := append([]string{}, d.Argv...)
+		var abs string
+		var err error
+		if strings.Contains(argv[0], "/") {
+			abs = argv[0]
+			if !filepath.IsAbs(abs) {
+				abs = filepath.Join(workDir, abs)
+			}
+			abs, err = lookPath(abs)
+		} else {
+			abs, err = lookPath(argv[0])
+		}
+		if err != nil || !filepath.IsAbs(abs) {
+			return nil, "", errs.E(CodeCmdNotFound, "cmd "+name+": "+argv[0]+" not found", "name", name)
+		}
+		argv[0] = abs
+		fh := map[string]string{}
+		for _, a := range argv {
+			p := candidatePath(workDir, a)
+			if h, err := hash(p); err == nil {
+				fh[p] = h
+			}
+		}
+		out = append(out, run.AllowedCmd{Name: name, Argv: argv, FileHashes: fh, TimeoutMS: d.Timeout.Milliseconds(), Env: append([]string(nil), d.Env...)})
+	}
+	if out == nil {
+		return nil, "", nil
+	}
+	return out, CmdsSHA256(out), nil
+}
+
+// CmdsSHA256 is the consent digest of an allowed-command list.
+func CmdsSHA256(cmds []run.AllowedCmd) string {
+	sorted := append([]run.AllowedCmd{}, cmds...)
+	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
+	raw, _ := json.Marshal(sorted)
+	canon, _ := run.Canonical(raw)
+	sum := sha256.Sum256(canon)
+	return hex.EncodeToString(sum[:])
+}
+
+func candidatePath(workDir, elem string) string {
+	if filepath.IsAbs(elem) {
+		return elem
+	}
+	return filepath.Join(workDir, elem)
+}
+
+// VerifyCmds re-hashes every pinned file and refuses newly-appeared files.
+func VerifyCmds(allowed []run.AllowedCmd, workDir string, hash func(string) (string, error)) error {
+	for _, c := range allowed {
+		for p, want := range c.FileHashes {
+			got, err := hash(p)
+			if err != nil {
+				return errs.E(CodeCmdChanged, "pinned file missing: "+p, "path", p, "reason", "missing", "name", c.Name)
+			}
+			if got != want {
+				return errs.E(CodeCmdChanged, "pinned file changed: "+p, "path", p, "reason", "mismatch", "name", c.Name)
+			}
+		}
+		for _, a := range c.Argv {
+			p := candidatePath(workDir, a)
+			if _, pinned := c.FileHashes[p]; pinned {
+				continue
+			}
+			if _, err := hash(p); err == nil {
+				return errs.E(CodeCmdChanged, "unpinned argv element now names a file: "+p, "path", p, "reason", "appeared", "name", c.Name)
+			}
+		}
+	}
+	return nil
+}
+
+// FileSHA256 hashes a regular file; directories and missing paths error.
+func FileSHA256(path string) (string, error) {
+	st, err := os.Lstat(path)
+	if err != nil {
+		return "", err
+	}
+	if !st.Mode().IsRegular() {
+		return "", errs.E(CodeCmdChanged, "not a regular file: "+path, "path", path, "reason", "irregular")
+	}
+	b, err := os.ReadFile(path)
+	if err != nil {
+		return "", err
+	}
+	sum := sha256.Sum256(b)
+	return hex.EncodeToString(sum[:]), nil
+}
+
+func containsStr(xs []string, s string) bool {
+	for _, x := range xs {
+		if x == s {
+			return true
+		}
+	}
+	return false
+}
diff --git a/internal/fsm/workflow/testdata/cmds-preimage.json b/internal/fsm/workflow/testdata/cmds-preimage.json
new file mode 100644
index 0000000..2f1cedc
--- /dev/null
+++ b/internal/fsm/workflow/testdata/cmds-preimage.json
@@ -0,0 +1 @@
+[{"name":"notify","argv":["/bin/bash","./scripts/notify.sh","--model","gpt"],"file_hashes":{"/bin/bash":"hb","WORK/scripts/notify.sh":"hs"},"timeout_ms":2000,"env":["SLACK_WEBHOOK"]},{"name":"zeta","argv":["WORK/scripts/notify.sh","x"],"file_hashes":{"WORK/scripts/notify.sh":"hs"},"timeout_ms":60000}]
diff --git a/internal/fsm/workflow/testdata/cmds-preimage.sha256 b/internal/fsm/workflow/testdata/cmds-preimage.sha256
new file mode 100644
index 0000000..9e5ebc0
--- /dev/null
+++ b/internal/fsm/workflow/testdata/cmds-preimage.sha256
@@ -0,0 +1 @@
+d5f22e88404dc2366f7168b75ad299af15f45c53068f0abe4fe39c901e6804e3
diff --git a/internal/fsm/workflow/workflow.go b/internal/fsm/workflow/workflow.go
new file mode 100644
index 0000000..f0fec0b
--- /dev/null
+++ b/internal/fsm/workflow/workflow.go
@@ -0,0 +1,744 @@
+// Package workflow turns a workflow YAML into a validated, resolvable
+// Workflow: states, transitions, nodes, declared commands, and the
+// convergence tree. Every rule is static; nothing here runs anything.
+package workflow
+
+import (
+	"crypto/sha256"
+	"encoding/hex"
+	"fmt"
+	"regexp"
+	"sort"
+	"strings"
+	"time"
+
+	"gopkg.in/yaml.v3"
+
+	"github.com/dsifry/metareview/internal/fsm/converge"
+	"github.com/dsifry/metareview/internal/fsm/errs"
+	"github.com/dsifry/metareview/internal/fsm/gate"
+	"github.com/dsifry/metareview/internal/fsm/run"
+)
+
+// Error codes.
+const (
+	CodeWorkflowInvalid   = "ERR_WORKFLOW_INVALID"
+	CodeVarUnset          = "ERR_VAR_UNSET"
+	CodeVarUnknown        = "ERR_VAR_UNKNOWN"
+	CodeCalibrationPinned = "ERR_CALIBRATION_PINNED"
+	CodeCmdNotFound       = "ERR_CMD_NOT_FOUND"
+	CodeCmdChanged        = "ERR_CMD_CHANGED"
+)
+
+// Calibration pins (design §17: the reference judge and effort).
+const (
+	CalibrationJudge  = "gpt-5.2"
+	CalibrationEffort = "medium"
+)
+
+// Reserved names.
+const (
+	FailedState    = run.State("failed")
+	JudgeNodeName  = "judge" // spec 5's `fsm judge --run` appends llm_call{Node: "judge"}
+	DefaultTimeout = 60 * time.Second
+	MaxTimeout     = 3600 * time.Second
+)
+
+// VarSpec declares a workflow variable.
+type VarSpec struct {
+	Default  string
+	Required bool
+}
+
+// CmdDecl is a declared command (the only place argv is written).
+type CmdDecl struct {
+	Name    string
+	Argv    []string
+	Timeout time.Duration
+	Env     []string
+}
+
+// Node is a state's work: a kind plus its exec mode and parameters.
+type Node struct {
+	Name   string
+	Kind   string
+	Exec   string
+	Model  string
+	Effort string
+	Params map[string]any
+	Cmd    string
+}
+
+// Transition is one edge; Loop marks the single loop-closing edge.
+type Transition struct {
+	From, To run.State
+	Gate     string
+	Outcome  run.Outcome
+	Loop     bool
+}
+
+// KindInfo is what Parse needs to know about a kind (from the registry).
+type KindInfo struct {
+	DefaultExec    string
+	AllowedExec    []string
+	ValidateParams func(map[string]any) error
+}
+
+// Options parameterizes Parse. Kinds is required.
+type Options struct {
+	Kinds map[string]KindInfo
+}
+
+// Workflow is the parsed, validated (and possibly resolved) workflow.
+type Workflow struct {
+	Name        string
+	Version     int
+	Vars        map[string]VarSpec
+	States      []run.State
+	Initial     run.State
+	Transitions []Transition
+	Nodes       map[run.State]*Node
+	Cmds        map[string]*CmdDecl
+	Convergence *yaml.Node
+	RepoMode    string
+	OnOverflow  string
+	Hash        string
+	Refs        map[run.State][]string
+	CmdRefs     map[string][]string
+	Warnings    []string
+}
+
+var (
+	statePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
+	varPattern   = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
+	cmdPattern   = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
+	varRef       = regexp.MustCompile(`\$(\$|[A-Z_][A-Z0-9_]*)`)
+	execModes    = map[string]bool{"inline": true, "subagent": true, "fork": true}
+	reservedEnv  = map[string]bool{"PATH": true, "HOME": true, "LANG": true, "TMPDIR": true, "BASH_ENV": true, "ENV": true, "PYTHONPATH": true, "NODE_OPTIONS": true, "PERL5OPT": true}
+	reservedPfx  = []string{"MRV_", "LD_", "DYLD_", "GIT_"}
+)
+
+func invalid(reason, at, detail string) error {
+	return errs.E(CodeWorkflowInvalid, detail, "reason", reason, "at", at)
+}
+
+// ---- raw YAML shapes ----
+
+type rawWorkflow struct {
+	Workflow    string                    `yaml:"workflow"`
+	Version     int                       `yaml:"version"`
+	Vars        map[string]rawVar         `yaml:"vars"`
+	States      []string                  `yaml:"states"`
+	Cmds        yaml.Node                 `yaml:"cmds"`
+	Nodes       map[string]map[string]any `yaml:"nodes"`
+	Transitions yaml.Node                 `yaml:"transitions"`
+	Convergence yaml.Node                 `yaml:"convergence"`
+	RepoMode    string                    `yaml:"repo_mode"`
+	OnOverflow  string                    `yaml:"on_overflow"`
+}
+
+type rawVar struct {
+	Default  *string `yaml:"default"`
+	Required bool    `yaml:"required"`
+}
+
+type rawCmd struct {
+	Argv    []any    `yaml:"argv"`
+	Timeout *int     `yaml:"timeout"`
+	Env     []string `yaml:"env"`
+}
+
+type rawTransition struct {
+	From    string `yaml:"from"`
+	To      string `yaml:"to"`
+	Gate    string `yaml:"gate"`
+	Outcome string `yaml:"outcome"`
+	Loop    bool   `yaml:"loop"`
+	On      string `yaml:"on"`
+}
+
+// Parse decodes and statically validates raw. $VAR tokens stay unresolved.
+func Parse(raw []byte, opts Options) (*Workflow, error) {
+	if len(opts.Kinds) == 0 {
+		return nil, invalid("missing_kinds", "options", "Options.Kinds is required")
+	}
+	var rw rawWorkflow
+	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
+	dec.KnownFields(true)
+	if err := dec.Decode(&rw); err != nil {
+		return nil, invalid("unknown_key", "document", err.Error())
+	}
+	sum := sha256.Sum256(raw)
+	w := &Workflow{
+		Name: rw.Workflow, Version: rw.Version, Vars: map[string]VarSpec{}, Nodes: map[run.State]*Node{},
+		Cmds: map[string]*CmdDecl{}, RepoMode: rw.RepoMode, OnOverflow: rw.OnOverflow,
+		Hash: hex.EncodeToString(sum[:]), Refs: map[run.State][]string{}, CmdRefs: map[string][]string{},
+	}
+	if w.Name == "" {
+		return nil, invalid("missing_name", "workflow", "workflow name is required")
+	}
+	if w.Version != 1 {
+		return nil, invalid("bad_version", "version", fmt.Sprintf("version must be 1, got %d", w.Version))
+	}
+	if len(rw.States) == 0 {
+		return nil, invalid("no_initial", "states", "states must be non-empty")
+	}
+	seen := map[string]bool{}
+	for _, s := range rw.States {
+		if !statePattern.MatchString(s) || s == JudgeNodeName || seen[s] {
+			return nil, invalid("bad_state", "states."+s, "state names must match ^[a-z][a-z0-9_-]{0,31}$, be unique, and not be `judge`")
+		}
+		seen[s] = true
+		w.States = append(w.States, run.State(s))
+	}
+	w.Initial = w.States[0]
+	if err := w.parseVars(rw.Vars); err != nil {
+		return nil, err
+	}
+	if err := w.parseCmds(&rw.Cmds); err != nil {
+		return nil, err
+	}
+	if err := w.parseNodes(rw.Nodes, opts.Kinds); err != nil {
+		return nil, err
+	}
+	if err := w.parseTransitions(&rw.Transitions); err != nil {
+		return nil, err
+	}
+	if err := w.validateGraph(); err != nil {
+		return nil, err
+	}
+	if rw.Convergence.Kind != 0 {
+		w.Convergence = &rw.Convergence
+		if err := converge.Validate(w.Convergence, w.cmdNames()); err != nil {
+			return nil, invalid("bad_convergence", "convergence", errs.As(err).Field("detail"))
+		}
+	} else if w.LoopTransition() != nil {
+		return nil, invalid("missing_convergence", "convergence", "a loop requires a convergence predicate")
+	}
+	if w.RepoMode == "" {
+		w.RepoMode = "advisory"
+	}
+	if w.RepoMode != "advisory" && w.RepoMode != "enforcing" {
+		return nil, invalid("bad_repo_mode", "repo_mode", "repo_mode must be advisory or enforcing")
+	}
+	if w.OnOverflow != "" && w.Cmds[w.OnOverflow] == nil {
+		return nil, invalid("unknown_cmd", "on_overflow", "on_overflow names an undeclared cmd "+w.OnOverflow)
+	}
+	if err := w.collectRefs(); err != nil {
+		return nil, err
+	}
+	w.warn()
+	return w, nil
+}
+
+func (w *Workflow) parseVars(vars map[string]rawVar) error {
+	if len(vars) > run.MaxVars {
+		return invalid("bad_var", "vars", fmt.Sprintf("more than %d vars", run.MaxVars))
+	}
+	for name, v := range vars {
+		if !varPattern.MatchString(name) {
+			return invalid("bad_var", "vars."+name, "var names must match ^[A-Z_][A-Z0-9_]*$")
+		}
+		if v.Required && v.Default != nil {
+			return invalid("bad_var", "vars."+name, "a required var cannot carry a default")
+		}
+		spec := VarSpec{Required: v.Required}
+		if v.Default != nil {
+			spec.Default = *v.Default
+		}
+		w.Vars[name] = spec
+	}
+	return nil
+}
+
+func (w *Workflow) parseCmds(n *yaml.Node) error {
+	if n.Kind == 0 {
+		return nil
+	}
+	if n.Kind != yaml.MappingNode {
+		return invalid("bad_cmd", "cmds", "cmds must be a mapping")
+	}
+	for i := 0; i+1 < len(n.Content); i += 2 {
+		name := n.Content[i].Value
+		at := "cmds." + name
+		if !cmdPattern.MatchString(name) {
+			return invalid("bad_cmd", at, "cmd names must match ^[a-z][a-z0-9_-]{0,31}$")
+		}
+		if w.Cmds[name] != nil {
+			return invalid("duplicate_cmd", at, "cmd declared twice")
+		}
+		var rc rawCmd
+		if err := strictNode(n.Content[i+1], &rc, "argv", "timeout", "env"); err != nil {
+			return invalid("bad_cmd", at, err.Error())
+		}
+		if len(rc.Argv) == 0 || len(rc.Argv) > run.MaxArgv {
+			return invalid("bad_cmd", at, fmt.Sprintf("argv must have 1..%d elements", run.MaxArgv))
+		}
+		d := &CmdDecl{Name: name, Timeout: DefaultTimeout}
+		for _, a := range rc.Argv {
+			s, ok := a.(string)
+			if !ok || s == "" {
+				return invalid("bad_cmd", at, "argv elements must be non-empty strings")
+			}
+			d.Argv = append(d.Argv, s)
+		}
+		if rc.Timeout != nil {
+			if *rc.Timeout < 1 || *rc.Timeout > 3600 {
+				return invalid("bad_cmd", at, "timeout must be 1..3600 seconds")
+			}
+			d.Timeout = time.Duration(*rc.Timeout) * time.Second
+		}
+		if len(rc.Env) > run.MaxEnv {
+			return invalid("bad_env", at, fmt.Sprintf("more than %d env names", run.MaxEnv))
+		}
+		envSeen := map[string]bool{}
+		for _, e := range rc.Env {
+			if !varPattern.MatchString(e) || envSeen[e] || reservedEnvName(e) {
+				return invalid("bad_env", at, "env name "+e+" is invalid, duplicate, or reserved")
+			}
+			envSeen[e] = true
+			d.Env = append(d.Env, e)
+		}
+		w.Cmds[name] = d
+	}
+	if len(w.Cmds) > run.MaxAllowedCmds {
+		return invalid("bad_cmd", "cmds", fmt.Sprintf("more than %d cmds", run.MaxAllowedCmds))
+	}
+	return nil
+}
+
+func reservedEnvName(e string) bool {
+	if reservedEnv[e] {
+		return true
+	}
+	for _, p := range reservedPfx {
+		if strings.HasPrefix(e, p) {
+			return true
+		}
+	}
+	return false
+}
+
+// strictNode decodes a mapping node into out, refusing keys outside allowed.
+func strictNode(n *yaml.Node, out any, allowed ...string) error {
+	if n.Kind != yaml.MappingNode {
+		return fmt.Errorf("line %d: expected a mapping", n.Line)
+	}
+	for i := 0; i+1 < len(n.Content); i += 2 {
+		if !containsStr(allowed, n.Content[i].Value) {
+			return fmt.Errorf("line %d: unknown field %q", n.Line, n.Content[i].Value)
+		}
+	}
+	return n.Decode(out)
+}
+
+func (w *Workflow) parseNodes(nodes map[string]map[string]any, kinds map[string]KindInfo) error {
+	names := make([]string, 0, len(nodes))
+	for n := range nodes {
+		names = append(names, n)
+	}
+	sort.Strings(names)
+	for _, name := range names {
+		raw := nodes[name]
+		at := "nodes." + name
+		if !w.hasState(run.State(name)) {
+			return invalid("unknown_state", at, "node for undeclared state "+name)
+		}
+		node := &Node{Name: name, Params: map[string]any{}}
+		for k, v := range raw {
+			s, isStr := v.(string)
+			switch k {
+			case "kind", "exec", "model", "effort", "cmd":
+				if !isStr {
+					return invalid("unknown_key", at+"."+k, k+" must be a string")
+				}
+				switch k {
+				case "kind":
+					node.Kind = s
+				case "exec":
+					node.Exec = s
+				case "model":
+					node.Model = s
+				case "effort":
+					node.Effort = s
+				case "cmd":
+					node.Cmd = s
+				}
+			default:
+				node.Params[k] = v
+			}
+		}
+		if node.Kind == "" {
+			return invalid("node_without_kind", at, "node needs a kind")
+		}
+		info, ok := kinds[node.Kind]
+		if !ok {
+			return invalid("unknown_kind", at, "unknown kind "+node.Kind)
+		}
+		if node.Exec == "" {
+			node.Exec = info.DefaultExec
+		}
+		if !execModes[node.Exec] {
+			return invalid("unknown_exec", at, "exec must be inline, subagent, or fork")
+		}
+		if !containsStr(info.AllowedExec, node.Exec) {
+			return invalid("exec_kind_mismatch", at, fmt.Sprintf("kind %s does not allow exec %s", node.Kind, node.Exec))
+		}
+		if info.ValidateParams != nil {
+			if err := info.ValidateParams(node.Params); err != nil {
+				return invalid("bad_params", at, err.Error())
+			}
+		}
+		if (node.Kind == "cmd") != (node.Cmd != "") {
+			return invalid("cmd_without_kind", at, "cmd: is required on cmd kinds and forbidden elsewhere")
+		}
+		if node.Cmd != "" && w.Cmds[node.Cmd] == nil {
+			return invalid("unknown_cmd", at, "node references undeclared cmd "+node.Cmd)
+		}
+		w.Nodes[run.State(name)] = node
+	}
+	return nil
+}
+
+func (w *Workflow) parseTransitions(n *yaml.Node) error {
+	switch n.Kind {
+	case yaml.SequenceNode:
+		for i, c := range n.Content {
+			var rt rawTransition
+			if err := strictNode(c, &rt, "from", "to", "gate", "outcome", "loop"); err != nil {
+				return invalid("unknown_key", fmt.Sprintf("transitions[%d]", i), err.Error())
+			}
+			if err := w.addTransition(rt, fmt.Sprintf("transitions[%d]", i)); err != nil {
+				return err
+			}
+		}
+	case yaml.MappingNode:
+		for i := 0; i+1 < len(n.Content); i += 2 {
+			key := n.Content[i].Value
+			from, to, ok := splitArrow(key)
+			if !ok {
+				return invalid("unknown_key", "transitions."+key, "mapping-form keys look like from→to")
+			}
+			if from == "*" {
+				continue // the implicit *→failed rule
+			}
+			var rt rawTransition
+			if err := strictNode(n.Content[i+1], &rt, "gate", "outcome", "loop", "on"); err != nil {
+				return invalid("unknown_key", "transitions."+key, err.Error())
+			}
+			rt.From, rt.To = from, to
+			if err := w.addTransition(rt, "transitions."+key); err != nil {
+				return err
+			}
+		}
+	default:
+		return invalid("unknown_key", "transitions", "transitions must be a list or a mapping")
+	}
+	return nil
+}
+
+func splitArrow(key string) (string, string, bool) {
+	for _, sep := range []string{"→", "->"} {
+		if i := strings.Index(key, sep); i > 0 {
+			return strings.TrimSpace(key[:i]), strings.TrimSpace(key[i+len(sep):]), true
+		}
+	}
+	return "", "", false
+}
+
+func (w *Workflow) addTransition(rt rawTransition, at string) error {
+	t := Transition{From: run.State(rt.From), To: run.State(rt.To), Gate: rt.Gate, Outcome: run.Outcome(rt.Outcome), Loop: rt.Loop}
+	if !w.hasState(t.From) || !w.hasState(t.To) {
+		return invalid("unknown_state", at, "transition references an undeclared state")
+	}
+	if t.From == FailedState || t.To == FailedState {
+		return invalid("failed_reserved", at, "failed is the implicit *→failed target and appears in no transition")
+	}
+	if _, ok := gate.Builtin(t.Gate); !ok {
+		return invalid("unknown_gate", at, "unknown gate "+t.Gate)
+	}
+	for _, o := range w.Transitions {
+		if o.From == t.From && o.Gate == t.Gate {
+			return invalid("duplicate_transition", at, fmt.Sprintf("second transition from %s with gate %s", t.From, t.Gate))
+		}
+	}
+	w.Transitions = append(w.Transitions, t)
+	return nil
+}
+
+func (w *Workflow) validateGraph() error {
+	if !w.hasState(FailedState) || w.Nodes[FailedState] != nil {
+		return invalid("failed_reserved", "states", "failed must be declared and carry no node")
+	}
+	for s, n := range w.Nodes {
+		if w.IsTerminal(s) {
+			return invalid("terminal_with_node", "nodes."+n.Name, "terminal states carry no node")
+		}
+	}
+	for i, t := range w.Transitions {
+		at := fmt.Sprintf("transitions[%d]", i)
+		if w.IsTerminal(t.To) {
+			if t.Outcome == "" {
+				return invalid("terminal_without_outcome", at, "a transition into a terminal state needs an outcome")
+			}
+			if !validOutcome(t.Outcome) {
+				return invalid("bad_outcome", at, "unknown outcome "+string(t.Outcome))
+			}
+		} else if t.Outcome != "" {
+			return invalid("outcome_on_nonterminal", at, "only transitions into terminal states carry an outcome")
+		}
+	}
+	incoming := map[run.State]int{}
+	for _, t := range w.Transitions {
+		incoming[t.To]++
+	}
+	for _, s := range w.States[1:] {
+		if s != FailedState && incoming[s] == 0 {
+			return invalid("unreachable_state", "states."+string(s), "state has no incoming transition")
+		}
+	}
+	loops := 0
+	var loop *Transition
+	for i := range w.Transitions {
+		if w.Transitions[i].Loop {
+			loops++
+			loop = &w.Transitions[i]
+		}
+	}
+	if loops > 1 {
+		return invalid("loop_count", "transitions", "more than one loop transition")
+	}
+	if loop != nil {
+		if !w.reaches(loop.To, loop.From) {
+			return invalid("loop_not_cycle", "transitions", "the loop's target must reach its source")
+		}
+		terminals := 0
+		for _, t := range w.Outgoing(loop.From) {
+			if !t.Loop && w.IsTerminal(t.To) && t.Outcome != "" {
+				terminals++
+			}
+		}
+		if terminals != 1 {
+			return invalid("loop_terminal", "transitions", "the loop-carrying state needs exactly one outcome-bearing terminal transition")
+		}
+	}
+	if w.hasCycleWithoutLoop() {
+		return invalid("cycle_without_loop", "transitions", "non-loop transitions form a cycle")
+	}
+	return nil
+}
+
+func validOutcome(o run.Outcome) bool {
+	for _, x := range run.Outcomes {
+		if x == o && o != run.OutcomeFailed {
+			return true
+		}
+	}
+	return false
+}
+
+// reaches reports whether from reaches to via non-loop transitions.
+func (w *Workflow) reaches(from, to run.State) bool {
+	seen := map[run.State]bool{}
+	stack := []run.State{from}
+	for len(stack) > 0 {
+		s := stack[len(stack)-1]
+		stack = stack[:len(stack)-1]
+		if s == to {
+			return true
+		}
+		if seen[s] {
+			continue
+		}
+		seen[s] = true
+		for _, t := range w.Outgoing(s) {
+			if !t.Loop {
+				stack = append(stack, t.To)
+			}
+		}
+	}
+	return false
+}
+
+func (w *Workflow) hasCycleWithoutLoop() bool {
+	const (
+		white = 0
+		gray  = 1
+		black = 2
+	)
+	color := map[run.State]int{}
+	var visit func(run.State) bool
+	visit = func(s run.State) bool {
+		color[s] = gray
+		for _, t := range w.Outgoing(s) {
+			if t.Loop {
+				continue
+			}
+			switch color[t.To] {
+			case gray:
+				return true
+			case white:
+				if visit(t.To) {
+					return true
+				}
+			}
+		}
+		color[s] = black
+		return false
+	}
+	for _, s := range w.States {
+		if color[s] == white && visit(s) {
+			return true
+		}
+	}
+	return false
+}
+
+func (w *Workflow) collectRefs() error {
+	for s, n := range w.Nodes {
+		refs := refsIn(n.Model, n.Effort)
+		refs = append(refs, paramRefs(n.Params)...)
+		for _, r := range refs {
+			if _, ok := w.Vars[r]; !ok {
+				return invalid("unknown_var", "nodes."+n.Name, "$"+r+" is not a declared var")
+			}
+		}
+		w.Refs[s] = uniqSorted(refs)
+	}
+	for name, c := range w.Cmds {
+		refs := refsIn(c.Argv...)
+		for _, r := range refs {
+			if _, ok := w.Vars[r]; !ok {
+				return invalid("unknown_var", "cmds."+name, "$"+r+" is not a declared var")
+			}
+		}
+		w.CmdRefs[name] = uniqSorted(refs)
+	}
+	return nil
+}
+
+func refsIn(ss ...string) []string {
+	var out []string
+	for _, s := range ss {
+		for _, m := range varRef.FindAllStringSubmatch(s, -1) {
+			if m[1] != "$" {
+				out = append(out, m[1])
+			}
+		}
+	}
+	return out
+}
+
+func paramRefs(params map[string]any) []string {
+	var out []string
+	for _, v := range params {
+		switch x := v.(type) {
+		case string:
+			out = append(out, refsIn(x)...)
+		case []any:
+			for _, e := range x {
+				if s, ok := e.(string); ok {
+					out = append(out, refsIn(s)...)
+				}
+			}
+		}
+	}
+	return out
+}
+
+func uniqSorted(in []string) []string {
+	m := map[string]bool{}
+	out := make([]string, 0, len(in))
+	for _, s := range in {
+		if !m[s] {
+			m[s] = true
+			out = append(out, s)
+		}
+	}
+	sort.Strings(out)
+	return out
+}
+
+func (w *Workflow) warn() {
+	if w.LoopTransition() == nil {
+		return
+	}
+	for _, t := range w.Transitions {
+		if t.Outcome == run.OutcomeClean {
+			return
+		}
+	}
+	w.Warnings = append(w.Warnings, "loop_without_clean_exit: no transition carries outcome clean; a loop that finds nothing ends overflow (all_fixed needs a non-empty AllFound)")
+}
+
+// ---- accessors ----
+
+func (w *Workflow) hasState(s run.State) bool {
+	for _, x := range w.States {
+		if x == s {
+			return true
+		}
+	}
+	return false
+}
+
+func (w *Workflow) cmdNames() []string {
+	names := make([]string, 0, len(w.Cmds))
+	for n := range w.Cmds {
+		names = append(names, n)
+	}
+	sort.Strings(names)
+	return names
+}
+
+// NodeFor returns the node of s, or nil.
+func (w *Workflow) NodeFor(s run.State) *Node { return w.Nodes[s] }
+
+// Outgoing returns s's transitions in declaration order.
+func (w *Workflow) Outgoing(s run.State) []Transition {
+	var out []Transition
+	for _, t := range w.Transitions {
+		if t.From == s {
+			out = append(out, t)
+		}
+	}
+	return out
+}
+
+// IsTerminal reports whether s has no outgoing transition.
+func (w *Workflow) IsTerminal(s run.State) bool { return len(w.Outgoing(s)) == 0 }
+
+// LoopTransition returns the loop edge, or nil.
+func (w *Workflow) LoopTransition() *Transition {
+	for i := range w.Transitions {
+		if w.Transitions[i].Loop {
+			return &w.Transitions[i]
+		}
+	}
+	return nil
+}
+
+// TerminalFor returns the loop-carrying state's outcome-bearing terminal
+// transition (validated unique), or nil for any other state.
+func (w *Workflow) TerminalFor(s run.State) *Transition {
+	loop := w.LoopTransition()
+	if loop == nil || loop.From != s {
+		return nil
+	}
+	var found *Transition
+	for i, t := range w.Transitions {
+		if t.From == s && !t.Loop && w.IsTerminal(t.To) && t.Outcome != "" {
+			found = &w.Transitions[i]
+		}
+	}
+	return found
+}
+
+// VarsReferencedBy lists the $VARs a state's node (and its cmd) reference.
+func (w *Workflow) VarsReferencedBy(s run.State) []string {
+	refs := append([]string{}, w.Refs[s]...)
+	if n := w.Nodes[s]; n != nil && n.Cmd != "" {
+		refs = append(refs, w.CmdRefs[n.Cmd]...)
+	}
+	return uniqSorted(refs)
+}
diff --git a/internal/fsm/workflow/workflow_test.go b/internal/fsm/workflow/workflow_test.go
new file mode 100644
index 0000000..c42ddc0
--- /dev/null
+++ b/internal/fsm/workflow/workflow_test.go
@@ -0,0 +1,530 @@
+package workflow
+
+import (
+	"crypto/sha256"
+	"encoding/hex"
+	"encoding/json"
+	"errors"
+	"os"
+	"path/filepath"
+	"sort"
+	"strings"
+	"testing"
+	"time"
+
+	"github.com/dsifry/metareview/internal/fsm/errs"
+	"github.com/dsifry/metareview/internal/fsm/run"
+	"github.com/dsifry/metareview/workflows"
+)
+
+func kinds() map[string]KindInfo {
+	lenses := func(p map[string]any) error {
+		if v, ok := p["lenses"]; ok {
+			n, isInt := v.(int)
+			if !isInt || n < 1 || n > 8 {
+				return errors.New("lenses must be 1..8")
+			}
+		}
+		return nil
+	}
+	return map[string]KindInfo{
+		"review-lenses":         {DefaultExec: "subagent", AllowedExec: []string{"inline", "subagent"}, ValidateParams: lenses},
+		"match-then-adjudicate": {DefaultExec: "fork", AllowedExec: []string{"fork"}},
+		"agent-edit":            {DefaultExec: "inline", AllowedExec: []string{"inline", "subagent"}},
+		"still-present":         {DefaultExec: "fork", AllowedExec: []string{"fork"}},
+		"cmd":                   {DefaultExec: "fork", AllowedExec: []string{"fork"}},
+	}
+}
+
+const example = `workflow: example
+version: 1
+vars: { JUDGE: {required: true}, JUDGE_EFFORT: {required: true}, REVIEWER: {default: claude-opus-5} }
+states: [discover, adjudicate, fix, verify, done, failed]
+cmds:
+  notify: { argv: [bash, ./scripts/notify.sh, --model, $JUDGE], timeout: 30, env: [SLACK_WEBHOOK] }
+nodes:
+  discover:   { kind: review-lenses, exec: subagent, model: $REVIEWER, lenses: 8 }
+  adjudicate: { kind: match-then-adjudicate, exec: fork, model: $JUDGE, effort: $JUDGE_EFFORT }
+  fix:        { kind: agent-edit }
+  verify:     { kind: still-present, model: $JUDGE, effort: $JUDGE_EFFORT }
+transitions:
+  - { from: discover, to: done, gate: findings_empty, outcome: clean }
+  - { from: discover, to: adjudicate, gate: findings_nonempty }
+  - { from: adjudicate, to: done, gate: confirmed_empty, outcome: clean }
+  - { from: adjudicate, to: fix, gate: confirmed_nonempty }
+  - { from: fix, to: verify, gate: commit_exists }
+  - { from: verify, to: done, gate: all_fixed, outcome: fixed }
+  - { from: verify, to: discover, gate: bugs_remain, loop: true }
+convergence: { any: [ no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}, {cmd: notify} ] }
+repo_mode: advisory
+on_overflow: notify
+`
+
+func mustParse(t *testing.T, src string) *Workflow {
+	t.Helper()
+	w, err := Parse([]byte(src), Options{Kinds: kinds()})
+	if err != nil {
+		t.Fatalf("parse: %v", err)
+	}
+	return w
+}
+
+func reasonOf(t *testing.T, src string) (string, string) {
+	t.Helper()
+	_, err := Parse([]byte(src), Options{Kinds: kinds()})
+	if !errs.Is(err, CodeWorkflowInvalid) {
+		t.Fatalf("expected ERR_WORKFLOW_INVALID, got %v", err)
+	}
+	e := errs.As(err)
+	return e.Field("reason"), e.Field("at")
+}
+
+func TestW1Shipped(t *testing.T) {
+	for _, name := range workflows.Names() {
+		raw, err := workflows.Read(name)
+		if err != nil {
+			t.Fatal(err)
+		}
+		w := mustParse(t, string(raw))
+		if w.Name != name || len(w.Warnings) != 0 {
+			t.Fatalf("%s: name %q warnings %v", name, w.Name, w.Warnings)
+		}
+		sum := sha256.Sum256(raw)
+		if w.Hash != hex.EncodeToString(sum[:]) {
+			t.Fatal("hash is sha256 of the raw bytes")
+		}
+	}
+	sdlc := mustParse(t, string(must(workflows.Read("sdlc-loop"))))
+	if len(sdlc.Transitions) != 7 || sdlc.LoopTransition() == nil || sdlc.LoopTransition().From != "verify" {
+		t.Fatalf("sdlc transitions: %+v", sdlc.Transitions)
+	}
+	if tt := sdlc.TerminalFor("verify"); tt == nil || tt.To != "done" || tt.Gate != "all_fixed" || tt.Outcome != run.OutcomeFixed {
+		t.Fatalf("TerminalFor: %+v", tt)
+	}
+	if sdlc.TerminalFor("discover") != nil {
+		t.Fatal("TerminalFor is nil off the loop state")
+	}
+	if strings.Join(sdlc.Refs["adjudicate"], ",") != "JUDGE,JUDGE_EFFORT" || strings.Join(sdlc.Refs["discover"], ",") != "REVIEWER,REV_EFFORT" {
+		t.Fatalf("refs: %v", sdlc.Refs)
+	}
+	if sdlc.Nodes["fix"].Exec != "inline" || sdlc.Nodes["fix"].Params["x"] != nil {
+		t.Fatal("fix node")
+	}
+	if !sdlc.IsTerminal("done") || !sdlc.IsTerminal("failed") || sdlc.IsTerminal("verify") {
+		t.Fatal("IsTerminal")
+	}
+	rl := mustParse(t, string(must(workflows.Read("review-loop"))))
+	if rl.LoopTransition() != nil || rl.Convergence != nil || len(rl.Outgoing("adjudicate")) != 2 {
+		t.Fatal("review-loop shape")
+	}
+
+	ex := mustParse(t, example)
+	if ex.Nodes["fix"].Exec != "inline" || ex.Nodes["verify"].Exec != "fork" {
+		t.Fatal("exec defaulted from the kind")
+	}
+	c := ex.Cmds["notify"]
+	if c == nil || c.Timeout != 30*time.Second || strings.Join(c.Env, ",") != "SLACK_WEBHOOK" || strings.Join(c.Argv, " ") != "bash ./scripts/notify.sh --model $JUDGE" {
+		t.Fatalf("cmd decl: %+v", c)
+	}
+	if strings.Join(ex.CmdRefs["notify"], ",") != "JUDGE" || ex.OnOverflow != "notify" || ex.RepoMode != "advisory" {
+		t.Fatal("cmd refs / on_overflow")
+	}
+	if ex.Nodes["discover"].Params["lenses"] != 8 {
+		t.Fatal("params kept")
+	}
+	// default timeout and default repo_mode
+	w := mustParse(t, strings.Replace(strings.Replace(example, ", timeout: 30", "", 1), "repo_mode: advisory\n", "", 1))
+	if w.Cmds["notify"].Timeout != DefaultTimeout || w.RepoMode != "advisory" {
+		t.Fatal("defaults")
+	}
+	// mapping form, both arrows, *→failed ignored, order preserved
+	m := mustParse(t, mappingDoc)
+	if len(m.Transitions) != 4 || m.Transitions[1].To != "adjudicate" || m.Transitions[3].Outcome != run.OutcomeReviewed {
+		t.Fatalf("mapping form: %+v", m.Transitions)
+	}
+	if strings.Join(m.VarsReferencedBy("adjudicate"), ",") != "JUDGE" || len(m.VarsReferencedBy("discover")) != 0 {
+		t.Fatal("VarsReferencedBy")
+	}
+}
+
+// scalarTransitions replaces the whole transitions block with a scalar.
+func scalarTransitions() string {
+	i := strings.Index(example, "transitions:")
+	j := strings.Index(example, "convergence:")
+	return example[:i] + "transitions: nope\n" + example[j:]
+}
+
+func must(b []byte, err error) []byte {
+	if err != nil {
+		panic(err)
+	}
+	return b
+}
+
+// edit applies one textual edit to the example.
+func edit(old, new string) string {
+	if !strings.Contains(example, old) {
+		panic("edit target missing: " + old)
+	}
+	return strings.Replace(example, old, new, 1)
+}
+
+func TestW2Reasons(t *testing.T) {
+	cases := []struct {
+		name, reason, at string
+		src              string
+	}{
+		{"unknown-top-key", "unknown_key", "document", example + "extra: 1\n"},
+		{"transitions-scalar", "unknown_key", "transitions", scalarTransitions()},
+		{"node-kind-not-string", "unknown_key", "nodes.fix.kind", edit("fix:        { kind: agent-edit }", "fix:        { kind: [agent-edit] }")},
+		{"transition-unknown-field", "unknown_key", "transitions[0]", edit("gate: findings_empty, outcome: clean }", "gate: findings_empty, outcome: clean, extra: 1 }")},
+		{"missing-name", "missing_name", "workflow", edit("workflow: example", "workflow: ''")},
+		{"bad-version", "bad_version", "version", edit("version: 1", "version: 2")},
+		{"no-initial", "no_initial", "states", edit("states: [discover, adjudicate, fix, verify, done, failed]", "states: []")},
+		{"bad-state-charset", "bad_state", "states.Discover", edit("states: [discover,", "states: [Discover, discover,")},
+		{"bad-state-judge", "bad_state", "states.judge", edit("states: [discover,", "states: [judge, discover,")},
+		{"bad-state-dup", "bad_state", "states.discover", edit("states: [discover,", "states: [discover, discover,")},
+		{"bad-var-name", "bad_var", "vars.judge", edit("REVIEWER: {default: claude-opus-5}", "judge: {default: x}")},
+		{"bad-var-required-default", "bad_var", "vars.JUDGE", edit("JUDGE: {required: true}", "JUDGE: {required: true, default: x}")},
+		{"bad-cmd-argv-empty", "bad_cmd", "cmds.notify", edit("argv: [bash, ./scripts/notify.sh, --model, $JUDGE]", "argv: []")},
+		{"bad-cmd-argv-nonstring", "bad_cmd", "cmds.notify", edit("argv: [bash, ./scripts/notify.sh, --model, $JUDGE]", "argv: [bash, 3]")},
+		{"bad-cmd-argv-emptyelem", "bad_cmd", "cmds.notify", edit("argv: [bash, ./scripts/notify.sh, --model, $JUDGE]", "argv: [bash, '']")},
+		{"bad-cmd-name", "bad_cmd", "cmds.Notify", edit("  notify: { argv:", "  Notify: { argv:")},
+		{"bad-cmd-timeout-high", "bad_cmd", "cmds.notify", edit("timeout: 30", "timeout: 3601")},
+		{"bad-cmd-timeout-zero", "bad_cmd", "cmds.notify", edit("timeout: 30", "timeout: 0")},
+		{"bad-cmd-timeout-string", "bad_cmd", "cmds.notify", edit("timeout: 30", "timeout: soon")},
+		{"bad-cmd-unknown-field", "bad_cmd", "cmds.notify", edit("timeout: 30", "timeout: 30, shell: true")},
+		{"bad-cmd-not-mapping", "bad_cmd", "cmds", edit("cmds:\n  notify: { argv: [bash, ./scripts/notify.sh, --model, $JUDGE], timeout: 30, env: [SLACK_WEBHOOK] }", "cmds: [notify]")},
+		{"bad-env-charset", "bad_env", "cmds.notify", edit("env: [SLACK_WEBHOOK]", "env: [slack]")},
+		{"bad-env-dup", "bad_env", "cmds.notify", edit("env: [SLACK_WEBHOOK]", "env: [SLACK_WEBHOOK, SLACK_WEBHOOK]")},
+		{"bad-env-path", "bad_env", "cmds.notify", edit("env: [SLACK_WEBHOOK]", "env: [PATH]")},
+		{"bad-env-mrv", "bad_env", "cmds.notify", edit("env: [SLACK_WEBHOOK]", "env: [MRV_X]")},
+		{"bad-env-ld", "bad_env", "cmds.notify", edit("env: [SLACK_WEBHOOK]", "env: [LD_PRELOAD]")},
+		{"bad-env-count", "bad_env", "cmds.notify", edit("env: [SLACK_WEBHOOK]", "env: [A1, A2, A3, A4, A5, A6, A7, A8, A9, A10, A11, A12, A13, A14, A15, A16, A17]")},
+		{"duplicate-cmd", "duplicate_cmd", "cmds.notify", edit("  notify: { argv: [bash, ./scripts/notify.sh, --model, $JUDGE], timeout: 30, env: [SLACK_WEBHOOK] }", "  notify: { argv: [a] }\n  notify: { argv: [b] }")},
+		{"unknown-state-node", "unknown_state", "nodes.zzz", edit("  fix:        { kind: agent-edit }", "  fix:        { kind: agent-edit }\n  zzz: { kind: agent-edit }")},
+		{"unknown-state-transition", "unknown_state", "transitions[0]", edit("{ from: discover, to: done, gate: findings_empty, outcome: clean }", "{ from: discover, to: nowhere, gate: findings_empty, outcome: clean }")},
+		{"failed-in-transition", "failed_reserved", "transitions[0]", edit("{ from: discover, to: done, gate: findings_empty, outcome: clean }", "{ from: discover, to: failed, gate: findings_empty, outcome: failed }")},
+		{"failed-undeclared", "failed_reserved", "states", edit("done, failed]", "done]")},
+		{"failed-with-node", "failed_reserved", "states", edit("  fix:        { kind: agent-edit }", "  fix:        { kind: agent-edit }\n  failed: { kind: agent-edit }")},
+		{"node-without-kind", "node_without_kind", "nodes.fix", edit("fix:        { kind: agent-edit }", "fix:        { exec: inline }")},
+		{"unknown-kind", "unknown_kind", "nodes.fix", edit("fix:        { kind: agent-edit }", "fix:        { kind: wizard }")},
+		{"unknown-exec", "unknown_exec", "nodes.fix", edit("fix:        { kind: agent-edit }", "fix:        { kind: agent-edit, exec: remote }")},
+		{"exec-kind-mismatch", "exec_kind_mismatch", "nodes.verify", edit("verify:     { kind: still-present, model: $JUDGE, effort: $JUDGE_EFFORT }", "verify:     { kind: still-present, exec: inline, model: $JUDGE, effort: $JUDGE_EFFORT }")},
+		{"bad-params", "bad_params", "nodes.discover", edit("lenses: 8 }", "lenses: 9 }")},
+		{"cmd-on-non-cmd-kind", "cmd_without_kind", "nodes.fix", edit("fix:        { kind: agent-edit }", "fix:        { kind: agent-edit, cmd: notify }")},
+		{"cmd-kind-without-cmd", "cmd_without_kind", "nodes.fix", edit("fix:        { kind: agent-edit }", "fix:        { kind: cmd }")},
+		{"unknown-cmd-node", "unknown_cmd", "nodes.fix", edit("fix:        { kind: agent-edit }", "fix:        { kind: cmd, cmd: nope }")},
+		{"unknown-cmd-on-overflow", "unknown_cmd", "on_overflow", edit("on_overflow: notify", "on_overflow: nope")},
+		{"unknown-cmd-atom", "bad_convergence", "convergence", edit("{cmd: notify}", "{cmd: nope}")},
+		{"terminal-with-node", "terminal_with_node", "nodes.done", edit("  fix:        { kind: agent-edit }", "  fix:        { kind: agent-edit }\n  done: { kind: agent-edit }")},
+		{"unknown-gate", "unknown_gate", "transitions[0]", edit("gate: findings_empty, outcome: clean }", "gate: vibes, outcome: clean }")},
+		{"duplicate-transition", "duplicate_transition", "transitions[3]", edit("{ from: adjudicate, to: done, gate: confirmed_empty, outcome: clean }", "{ from: adjudicate, to: fix, gate: findings_nonempty }\n  - { from: discover, to: fix, gate: findings_nonempty }")},
+		{"terminal-without-outcome", "terminal_without_outcome", "transitions[0]", edit("gate: findings_empty, outcome: clean }", "gate: findings_empty }")},
+		{"outcome-on-nonterminal", "outcome_on_nonterminal", "transitions[1]", edit("{ from: discover, to: adjudicate, gate: findings_nonempty }", "{ from: discover, to: adjudicate, gate: findings_nonempty, outcome: clean }")},
+		{"bad-outcome", "bad_outcome", "transitions[0]", edit("gate: findings_empty, outcome: clean }", "gate: findings_empty, outcome: great }")},
+		{"bad-outcome-failed", "bad_outcome", "transitions[0]", edit("gate: findings_empty, outcome: clean }", "gate: findings_empty, outcome: failed }")},
+		{"unreachable-state", "unreachable_state", "states.fix", edit("  - { from: adjudicate, to: fix, gate: confirmed_nonempty }\n", "")},
+		{"loop-count", "loop_count", "transitions", edit("{ from: adjudicate, to: fix, gate: confirmed_nonempty }", "{ from: adjudicate, to: fix, gate: confirmed_nonempty, loop: true }")},
+		{"loop-not-cycle", "loop_not_cycle", "transitions", strings.Replace(edit("{ from: verify, to: discover, gate: bugs_remain, loop: true }", "{ from: verify, to: side, gate: bugs_remain, loop: true }\n  - { from: side, to: done, gate: findings_empty, outcome: clean }"), "verify, done, failed]", "verify, side, done, failed]", 1)},
+		{"loop-terminal-zero", "loop_terminal", "transitions", edit("{ from: verify, to: done, gate: all_fixed, outcome: fixed }", "{ from: verify, to: fix, gate: all_fixed }")},
+		{"loop-terminal-two", "loop_terminal", "transitions", edit("{ from: verify, to: done, gate: all_fixed, outcome: fixed }", "{ from: verify, to: done, gate: all_fixed, outcome: fixed }\n  - { from: verify, to: done, gate: findings_empty, outcome: clean }")},
+		{"missing-convergence", "missing_convergence", "convergence", edit("convergence: { any: [ no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}, {cmd: notify} ] }\n", "")},
+		{"cycle-without-loop", "cycle_without_loop", "transitions", edit("{ from: fix, to: verify, gate: commit_exists }", "{ from: fix, to: verify, gate: commit_exists }\n  - { from: fix, to: adjudicate, gate: findings_nonempty }")},
+		{"bad-convergence", "bad_convergence", "convergence", edit("{max_iterations: 5}", "{max_iterations: -5}")},
+		{"bad-repo-mode", "bad_repo_mode", "repo_mode", edit("repo_mode: advisory", "repo_mode: strict")},
+		{"unknown-var-node", "unknown_var", "nodes.discover", edit("model: $REVIEWER, lenses: 8", "model: $REVIEWERX, lenses: 8")},
+		{"unknown-var-cmd", "unknown_var", "cmds.notify", edit("--model, $JUDGE]", "--model, $NOPE]")},
+		{"unknown-var-param", "unknown_var", "nodes.discover", edit("lenses: 8 }", "lenses: 8, tags: [$NOPE] }")},
+	}
+	for _, c := range cases {
+		reason, at := reasonOf(t, c.src)
+		if reason != c.reason || at != c.at {
+			t.Errorf("%s: got %s@%s want %s@%s", c.name, reason, at, c.reason, c.at)
+		}
+	}
+	// missing kinds / over-cap vars, cmds, argv
+	if _, err := Parse([]byte(example), Options{}); errs.As(err).Field("reason") != "missing_kinds" {
+		t.Fatal("missing_kinds")
+	}
+	var vars []string
+	for i := 0; i <= run.MaxVars; i++ {
+		vars = append(vars, "V"+strings.Repeat("A", i+1)+": {default: x}")
+	}
+	if r, _ := reasonOf(t, edit("vars: { JUDGE: {required: true}, JUDGE_EFFORT: {required: true}, REVIEWER: {default: claude-opus-5} }", "vars: { JUDGE: {required: true}, JUDGE_EFFORT: {required: true}, REVIEWER: {default: claude-opus-5}, "+strings.Join(vars, ", ")+" }")); r != "bad_var" {
+		t.Errorf("MaxVars: %s", r)
+	}
+	var cmds []string
+	for i := 0; i <= run.MaxAllowedCmds; i++ {
+		cmds = append(cmds, "  c"+strings.Repeat("x", i)+": { argv: [a] }")
+	}
+	if r, _ := reasonOf(t, edit("  notify: { argv: [bash, ./scripts/notify.sh, --model, $JUDGE], timeout: 30, env: [SLACK_WEBHOOK] }", "  notify: { argv: [bash] }\n"+strings.Join(cmds, "\n"))); r != "bad_cmd" {
+		t.Errorf("MaxAllowedCmds: %s", r)
+	}
+	if r, _ := reasonOf(t, edit("argv: [bash, ./scripts/notify.sh, --model, $JUDGE]", "argv: ["+strings.Repeat("a, ", run.MaxArgv)+"a]")); r != "bad_cmd" {
+		t.Errorf("MaxArgv: %s", r)
+	}
+	// mapping-form errors: key without an arrow, unknown field, and a transition error inside the mapping form
+	for name, rep := range map[string][2]string{
+		"no-arrow":      {`"discover→done": { gate: findings_empty, outcome: clean }`, `"discover": { gate: findings_empty, outcome: clean }`},
+		"unknown-field": {`"discover→done": { gate: findings_empty, outcome: clean }`, `"discover→done": { gate: findings_empty, outcome: clean, extra: 1 }`},
+		"scalar-value":  {`"discover→done": { gate: findings_empty, outcome: clean }`, `"discover→done": clean`},
+	} {
+		if r, at := reasonOf(t, strings.Replace(mappingDoc, rep[0], rep[1], 1)); r != "unknown_key" || !strings.HasPrefix(at, "transitions.") {
+			t.Errorf("%s: %s@%s", name, r, at)
+		}
+	}
+	if r, at := reasonOf(t, strings.Replace(mappingDoc, `"discover→done": { gate: findings_empty, outcome: clean }`, `"discover→done": { gate: vibes, outcome: clean }`, 1)); r != "unknown_gate" || at != "transitions.discover→done" {
+		t.Errorf("mapping transition error: %s@%s", r, at)
+	}
+	if r, _ := reasonOf(t, edit("timeout: 30", "timeout: 30, shell: true")); r != "bad_cmd" {
+		t.Errorf("cmd unknown field: %s", r)
+	}
+}
+
+const mappingDoc = `workflow: rl
+version: 1
+vars: { JUDGE: {required: true} }
+states: [discover, adjudicate, done, failed]
+nodes:
+  discover:   { kind: review-lenses }
+  adjudicate: { kind: match-then-adjudicate, model: $JUDGE }
+transitions:
+  "discover→done": { gate: findings_empty, outcome: clean }
+  "discover -> adjudicate": { gate: findings_nonempty }
+  "adjudicate→done": { gate: confirmed_empty, outcome: clean }
+  "adjudicate->done ": { gate: confirmed_nonempty, outcome: reviewed }
+  "*→failed": { on: gate_error }
+`
+
+func TestW2Warnings(t *testing.T) {
+	src := edit("  - { from: discover, to: done, gate: findings_empty, outcome: clean }\n", "")
+	src = strings.Replace(src, "  - { from: adjudicate, to: done, gate: confirmed_empty, outcome: clean }\n", "", 1)
+	w := mustParse(t, src)
+	if len(w.Warnings) != 1 || !strings.HasPrefix(w.Warnings[0], "loop_without_clean_exit") {
+		t.Fatalf("warnings: %v", w.Warnings)
+	}
+}
+
+func TestW3Resolve(t *testing.T) {
+	w := mustParse(t, example)
+	r, eff, err := w.Resolve(map[string]string{"JUDGE": "a", "JUDGE_EFFORT": "b"}, false)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if r.Nodes["adjudicate"].Model != "a" || r.Nodes["adjudicate"].Effort != "b" || r.Nodes["discover"].Model != "claude-opus-5" {
+		t.Fatalf("prefix pair resolution: %+v", r.Nodes["adjudicate"])
+	}
+	if strings.Join(r.Cmds["notify"].Argv, " ") != "bash ./scripts/notify.sh --model a" || w.Cmds["notify"].Argv[3] != "$JUDGE" {
+		t.Fatal("argv substitution must not mutate the original")
+	}
+	if eff["REVIEWER"] != "claude-opus-5" || eff["JUDGE"] != "a" || len(eff) != 3 {
+		t.Fatalf("effective: %v", eff)
+	}
+	if strings.Join(r.VarsReferencedBy("adjudicate"), ",") != "JUDGE,JUDGE_EFFORT" || r.Hash != w.Hash {
+		t.Fatal("resolved copy keeps Refs and Hash")
+	}
+	// $$ literal and list params
+	src := edit("lenses: 8 }", "lenses: 8, tags: [$JUDGE, 3, '$$x'], note: '$$' }")
+	w2 := mustParse(t, src)
+	r2, _, _ := w2.Resolve(map[string]string{"JUDGE": "j", "JUDGE_EFFORT": "e"}, false)
+	tags := r2.Nodes["discover"].Params["tags"].([]any)
+	if tags[0] != "j" || tags[1] != 3 || tags[2] != "$x" || r2.Nodes["discover"].Params["note"] != "$" || r2.Nodes["discover"].Params["lenses"] != 8 {
+		t.Fatalf("params: %v", r2.Nodes["discover"].Params)
+	}
+	// errors
+	if _, _, err := w.Resolve(map[string]string{"JUDGE": "a"}, false); !errs.Is(err, CodeVarUnset) || errs.As(err).Field("name") != "JUDGE_EFFORT" {
+		t.Fatalf("unset: %v", err)
+	}
+	if _, _, err := w.Resolve(map[string]string{"JUDGE": "a", "JUDGE_EFFORT": "b", "FOO": "x"}, false); !errs.Is(err, CodeVarUnknown) || errs.As(err).Field("name") != "FOO" {
+		t.Fatalf("unknown caller var: %v", err)
+	}
+	// calibration
+	r3, eff3, err := w.Resolve(map[string]string{"REVIEWER": "r"}, true)
+	if err != nil || eff3["JUDGE"] != CalibrationJudge || eff3["JUDGE_EFFORT"] != CalibrationEffort || r3.Nodes["adjudicate"].Model != CalibrationJudge {
+		t.Fatalf("calibration pin: %v %v", eff3, err)
+	}
+	for _, name := range []string{"JUDGE", "JUDGE_EFFORT"} {
+		if _, _, err := w.Resolve(map[string]string{name: "x"}, true); !errs.Is(err, CodeCalibrationPinned) || errs.As(err).Field("name") != name {
+			t.Fatalf("pinned %s: %v", name, err)
+		}
+	}
+	// re-resolve of stored (pinned) vars with calibration=false succeeds
+	if _, _, err := w.Resolve(eff3, false); err != nil {
+		t.Fatal("re-resolve stored vars")
+	}
+	// calibration on a workflow without JUDGE is a no-op
+	noJudge := mustParse(t, strings.Replace(strings.Replace(strings.Replace(example, "JUDGE: {required: true}, JUDGE_EFFORT: {required: true}, ", "", 1), "model: $JUDGE, effort: $JUDGE_EFFORT", "model: x", -1), "--model, $JUDGE", "--model, x", 1))
+	if _, eff4, err := noJudge.Resolve(nil, true); err != nil || len(eff4) != 1 {
+		t.Fatalf("no-judge calibration: %v %v", eff4, err)
+	}
+}
+
+func TestW4ResolveCmds(t *testing.T) {
+	work := t.TempDir()
+	if err := os.MkdirAll(filepath.Join(work, "scripts"), 0o755); err != nil {
+		t.Fatal(err)
+	}
+	script := filepath.Join(work, "scripts", "notify.sh")
+	if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0o755); err != nil {
+		t.Fatal(err)
+	}
+	lookPath := func(name string) (string, error) {
+		switch name {
+		case "bash":
+			return "/bin/bash", nil
+		case "rel":
+			return "bin/rel", nil
+		}
+		if filepath.IsAbs(name) {
+			if _, err := os.Stat(name); err == nil {
+				return name, nil
+			}
+		}
+		return "", errors.New("not found")
+	}
+	hashes := map[string]string{"/bin/bash": "hb", script: "hs"}
+	hash := func(p string) (string, error) {
+		if h, ok := hashes[p]; ok {
+			return h, nil
+		}
+		return "", errors.New("no such file")
+	}
+	src := edit("cmds:\n  notify: { argv: [bash, ./scripts/notify.sh, --model, $JUDGE], timeout: 30, env: [SLACK_WEBHOOK] }",
+		"cmds:\n  zeta: { argv: [./scripts/notify.sh, x] }\n  notify: { argv: [bash, ./scripts/notify.sh, --model, $JUDGE], timeout: 30, env: [SLACK_WEBHOOK] }")
+	src = strings.Replace(src, "timeout: 30", "timeout: 2", 1) // 1500 ms cannot be expressed; use 2 s → 2000 ms
+	w := mustParse(t, src)
+	r, _, err := w.Resolve(map[string]string{"JUDGE": "gpt", "JUDGE_EFFORT": "low"}, false)
+	if err != nil {
+		t.Fatal(err)
+	}
+	allowed, sha, err := ResolveCmds(r, work, lookPath, hash)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if len(allowed) != 2 || allowed[0].Name != "notify" || allowed[1].Name != "zeta" {
+		t.Fatalf("sorted by name: %+v", allowed)
+	}
+	n := allowed[0]
+	if n.Argv[0] != "/bin/bash" || n.Argv[3] != "gpt" || n.TimeoutMS != 2000 || strings.Join(n.Env, ",") != "SLACK_WEBHOOK" {
+		t.Fatalf("notify: %+v", n)
+	}
+	if n.FileHashes["/bin/bash"] != "hb" || n.FileHashes[script] != "hs" || len(n.FileHashes) != 2 {
+		t.Fatalf("file hashes: %v", n.FileHashes)
+	}
+	z := allowed[1]
+	if z.Argv[0] != script || z.FileHashes[script] != "hs" || len(z.FileHashes) != 1 || z.Env != nil || z.TimeoutMS != 60000 {
+		t.Fatalf("zeta: %+v", z)
+	}
+	// hand-authored preimage: the fixture is the canonical JSON of this list; its sha is computed by shasum (see testdata/cmds-preimage.sha256)
+	fixture, err := os.ReadFile("testdata/cmds-preimage.json")
+	if err != nil {
+		t.Fatal(err)
+	}
+	wantSha := strings.TrimSpace(string(must(os.ReadFile("testdata/cmds-preimage.sha256"))))
+	got, _ := json.Marshal(allowed)
+	canon, _ := run.Canonical(got)
+	if string(canon) != strings.TrimSpace(strings.ReplaceAll(string(fixture), "WORK", work)) {
+		t.Fatalf("preimage drift:\n%s\n%s", canon, fixture)
+	}
+	// the sha over the WORK-substituted fixture must equal CmdsSHA256 (the .sha256 file pins the fixture with WORK=/w)
+	if sha != CmdsSHA256(allowed) {
+		t.Fatal("ResolveCmds sha == CmdsSHA256")
+	}
+	sum := sha256.Sum256([]byte(strings.TrimSpace(string(fixture))))
+	if hex.EncodeToString(sum[:]) != wantSha {
+		t.Fatalf("fixture sha256 %s != pinned %s", hex.EncodeToString(sum[:]), wantSha)
+	}
+	// declaration order independent
+	rev := append([]run.AllowedCmd{}, allowed[1], allowed[0])
+	if CmdsSHA256(rev) != sha {
+		t.Fatal("order independent")
+	}
+	// one-byte edit moves the sha
+	mod := append([]run.AllowedCmd{}, allowed...)
+	mod[0].Env = []string{"SLACK_WEBHOOK", "X"}
+	if CmdsSHA256(mod) == sha {
+		t.Fatal("env is part of the preimage")
+	}
+	mod = append([]run.AllowedCmd{}, allowed...)
+	mod[0].TimeoutMS = 2001
+	if CmdsSHA256(mod) == sha {
+		t.Fatal("timeout is part of the preimage")
+	}
+	// no cmds → empty
+	if a, s, err := ResolveCmds(mustParse(t, string(must(workflows.Read("review-loop")))), work, lookPath, hash); a != nil || s != "" || err != nil {
+		t.Fatal("no cmds")
+	}
+	// not found / relative lookPath result / missing relative script
+	for _, bad := range []string{"argv: [nope]", "argv: [rel]", "argv: [./missing.sh]"} {
+		wb := mustParse(t, edit("argv: [bash, ./scripts/notify.sh, --model, $JUDGE]", bad))
+		rb, _, _ := wb.Resolve(map[string]string{"JUDGE": "g", "JUDGE_EFFORT": "l"}, false)
+		if _, _, err := ResolveCmds(rb, work, lookPath, hash); !errs.Is(err, CodeCmdNotFound) || errs.As(err).Field("name") != "notify" {
+			t.Fatalf("%s: %v", bad, err)
+		}
+	}
+	// VerifyCmds: ok, mismatch, missing, appeared
+	if err := VerifyCmds(allowed, work, hash); err != nil {
+		t.Fatal(err)
+	}
+	hashes[script] = "changed"
+	if err := VerifyCmds(allowed, work, hash); !errs.Is(err, CodeCmdChanged) || errs.As(err).Field("reason") != "mismatch" {
+		t.Fatalf("mismatch: %v", err)
+	}
+	delete(hashes, script)
+	if err := VerifyCmds(allowed, work, hash); !errs.Is(err, CodeCmdChanged) || errs.As(err).Field("reason") != "missing" {
+		t.Fatalf("missing: %v", err)
+	}
+	hashes[script] = "hs"
+	hashes[filepath.Join(work, "--model")] = "hm" // an unpinned element now names a file
+	if err := VerifyCmds(allowed, work, hash); !errs.Is(err, CodeCmdChanged) || errs.As(err).Field("reason") != "appeared" || errs.As(err).Field("path") != filepath.Join(work, "--model") {
+		t.Fatalf("appeared: %v", err)
+	}
+	// FileSHA256: regular, directory, missing
+	h, err := FileSHA256(script)
+	if err != nil || h != hexSum("#!/bin/sh\n") {
+		t.Fatalf("FileSHA256: %s %v", h, err)
+	}
+	if _, err := FileSHA256(work); !errs.Is(err, CodeCmdChanged) {
+		t.Fatal("dir is irregular")
+	}
+	if _, err := FileSHA256(filepath.Join(work, "nope")); err == nil {
+		t.Fatal("missing")
+	}
+	unreadable := filepath.Join(work, "unreadable")
+	if err := os.WriteFile(unreadable, []byte("x"), 0o000); err != nil {
+		t.Fatal(err)
+	}
+	if os.Getuid() != 0 {
+		if _, err := FileSHA256(unreadable); err == nil {
+			t.Fatal("unreadable")
+		}
+	}
+}
+
+func hexSum(s string) string {
+	sum := sha256.Sum256([]byte(s))
+	return hex.EncodeToString(sum[:])
+}
+
+func TestW5Accessors(t *testing.T) {
+	w := mustParse(t, example)
+	out := w.Outgoing("discover")
+	if len(out) != 2 || out[0].Gate != "findings_empty" || out[1].Gate != "findings_nonempty" {
+		t.Fatalf("Outgoing order: %+v", out)
+	}
+	if w.NodeFor("done") != nil || w.NodeFor("fix") == nil {
+		t.Fatal("NodeFor")
+	}
+	// VarsReferencedBy on a cmd node unions the cmd's refs
+	src := edit("  fix:        { kind: agent-edit }", "  fix:        { kind: cmd, cmd: notify, tag: $REVIEWER }")
+	wc := mustParse(t, src)
+	if strings.Join(wc.VarsReferencedBy("fix"), ",") != "JUDGE,REVIEWER" {
+		t.Fatalf("VarsReferencedBy cmd node: %v", wc.VarsReferencedBy("fix"))
+	}
+	names := w.cmdNames()
+	sort.Strings(names)
+	if strings.Join(names, ",") != "notify" {
+		t.Fatal("cmdNames")
+	}
+}
diff --git a/workflows/sdlc-loop.yaml b/workflows/sdlc-loop.yaml
index 1845616..6c5a50d 100644
--- a/workflows/sdlc-loop.yaml
+++ b/workflows/sdlc-loop.yaml
@@ -7,9 +7,9 @@ vars:
   JUDGE_EFFORT: {required: true}   # Pareto-selected with JUDGE (spec §17); pinned to medium under --calibration
 states: [discover, adjudicate, fix, verify, done, failed]
 transitions:                                  # ordered
-  - {from: discover,   to: done,       gate: findings_empty,     outcome: clean}
+  - {from: discover,   to: done,       gate: nothing_found,      outcome: clean}   # iteration 0 only: refuses once bugs are known
   - {from: discover,   to: adjudicate, gate: findings_nonempty}
-  - {from: adjudicate, to: done,       gate: confirmed_empty,    outcome: clean}
+  - {from: adjudicate, to: done,       gate: nothing_confirmed,  outcome: clean}
   - {from: adjudicate, to: fix,        gate: confirmed_nonempty}
   - {from: fix,        to: verify,     gate: commit_exists}
   - {from: verify,     to: done,       gate: all_fixed,   outcome: fixed}
@@ -17,8 +17,8 @@ transitions:                                  # ordered
 nodes:
   discover:   {kind: review-lenses,        exec: subagent, lenses: 8, model: $REVIEWER, effort: $REV_EFFORT}
   adjudicate: {kind: match-then-adjudicate, exec: fork,     model: $JUDGE, effort: $JUDGE_EFFORT}
-  fix:        {kind: agent-edit,            exec: inline}
-  verify:     {kind: still-present,         exec: fork,     model: $JUDGE, effort: $JUDGE_EFFORT}
+  fix:        {kind: agent-edit}                                   # exec inferred from the kind (inline)
+  verify:     {kind: still-present,         model: $JUDGE, effort: $JUDGE_EFFORT}   # exec inferred (fork)
 convergence:
   any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]
 repo_mode: advisory


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

