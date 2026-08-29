# metareview task-done context

Run ID: `mrv-20260827-083954751710000-task-done-m1-m6-fsm-packages-a99c72f1`

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

- Base: `92ba42e55183b4d5ee522381f9db5eafd5f2d68f`
- Head: `72cdfce5e7d18b84a8a5b6246be5cdd2f9ca4a25`
- Branch: ``
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `35543`
- Filtered diff bytes: `12082`
- Risk level: `none`
- Generated files excluded: docs/metareview/context/mrv-20260827-064453390399000-artifact-2026-08-27-metareview-0-9-0-fsm-core-a0b8592f-context.md, docs/metareview/context/mrv-20260827-064453475472000-artifact-2026-08-27-metareview-0-9-0-fsm-judge-kinds-33d63bfb-context.md, docs/metareview/reviews/mrv-20260827-064453390399000-artifact-2026-08-27-metareview-0-9-0-fsm-core-a0b8592f.md, docs/metareview/reviews/mrv-20260827-064453475472000-artifact-2026-08-27-metareview-0-9-0-fsm-judge-kinds-33d63bfb.md



## Review Manifest

- Manifest verdict: `NEEDS_REVISION`
- Source manifest hash: `22290e2fc7ea3d64`
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- docs/tasks/m1-m6-fsm-packages.md
- internal/fsm/converge/converge.go
- internal/fsm/converge/converge_test.go
- internal/fsm/gate/fake.go
- internal/fsm/gate/gate_test.go
- internal/fsm/gate/git.go
- internal/fsm/run/errors.go
- internal/fsm/run/fold.go
- internal/fsm/run/fold_test.go
- internal/fsm/run/types.go
- workflows/sdlc-loop.yaml

### Path Dispositions
- docs/metareview/context/mrv-20260827-064453390399000-artifact-2026-08-27-metareview-0-9-0-fsm-core-a0b8592f-context.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/context/mrv-20260827-064453475472000-artifact-2026-08-27-metareview-0-9-0-fsm-judge-kinds-33d63bfb-context.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/reviews/mrv-20260827-064453390399000-artifact-2026-08-27-metareview-0-9-0-fsm-core-a0b8592f.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/reviews/mrv-20260827-064453475472000-artifact-2026-08-27-metareview-0-9-0-fsm-judge-kinds-33d63bfb.md: generated (metareview generated review artifact excluded from source manifest)

### Shards
- shard-01: docs/tasks/m1-m6-fsm-packages.md, internal/fsm/converge/converge.go, internal/fsm/converge/converge_test.go, internal/fsm/gate/fake.go, internal/fsm/gate/gate_test.go, internal/fsm/gate/git.go, internal/fsm/run/errors.go, internal/fsm/run/fold.go, internal/fsm/run/fold_test.go, internal/fsm/run/types.go, workflows/sdlc-loop.yaml

### Manifest Blockers
- missing shard result for shard-01

## Changed Files

- internal/fsm/converge/converge.go
- internal/fsm/converge/converge_test.go
- internal/fsm/gate/fake.go
- internal/fsm/gate/gate_test.go
- internal/fsm/gate/git.go
- internal/fsm/run/errors.go
- internal/fsm/run/fold.go
- internal/fsm/run/fold_test.go
- internal/fsm/run/types.go
- workflows/sdlc-loop.yaml
- docs/tasks/m1-m6-fsm-packages.md

## Diff

```diff
diff --git a/internal/fsm/converge/converge.go b/internal/fsm/converge/converge.go
index d3f881e..9b9ddab 100644
--- a/internal/fsm/converge/converge.go
+++ b/internal/fsm/converge/converge.go
@@ -245,9 +245,11 @@ func (c *cmdAtom) Evaluate(ctx context.Context, s run.Snapshot) (Result, error)
 
 // Payload is the JSON handed to sanctioned commands: the snapshot with var
 // values replaced by their sha256 (commands are consented to run, not to
-// receive credentials).
+// receive credentials) and node outputs omitted (they are not the
+// convergence question and can be megabytes).
 func Payload(s run.Snapshot) []byte {
 	c := s.Clone()
+	c.NodeOutputs = nil
 	for k, v := range c.Vars {
 		sum := sha256.Sum256([]byte(v))
 		c.Vars[k] = "sha256:" + hex.EncodeToString(sum[:])
diff --git a/internal/fsm/converge/converge_test.go b/internal/fsm/converge/converge_test.go
index 73e28ac..b30e948 100644
--- a/internal/fsm/converge/converge_test.go
+++ b/internal/fsm/converge/converge_test.go
@@ -99,6 +99,7 @@ func TestC2CmdAtom(t *testing.T) {
 	ctx := context.Background()
 	s := snap(2, 1, nil, 7, 1)
 	s.Vars = map[string]string{"JUDGE": "secret-model"}
+	s.NodeOutputs = map[string]json.RawMessage{"n@0": json.RawMessage(`{"big":true}`)}
 	fr := &fakeRunner{res: CmdResult{Stdout: []byte(`{"stop": true, "reason": "plateau"}`)}}
 	p, err := Parse(node(t, "{cmd: notify}"), fr)
 	if err != nil {
@@ -126,6 +127,9 @@ func TestC2CmdAtom(t *testing.T) {
 	if got.Iteration != 2 || got.Tokens.Input != 7 {
 		t.Fatal("payload carries the snapshot")
 	}
+	if strings.Contains(string(fr.stdins[0]), `"big"`) {
+		t.Fatal("payload must omit node outputs")
+	}
 	if s.Vars["JUDGE"] != "secret-model" {
 		t.Fatal("Payload must not mutate the snapshot")
 	}
diff --git a/internal/fsm/gate/fake.go b/internal/fsm/gate/fake.go
index 30d0635..5175a98 100644
--- a/internal/fsm/gate/fake.go
+++ b/internal/fsm/gate/fake.go
@@ -16,6 +16,7 @@ type Fake struct {
 	Porcelain string
 	Diffs     map[string]string // "from..to" → diff; "HEAD" → working diff
 	Tree      string            // WorkTree answer
+	Common    string            // CommonDir answer
 	Err       error
 	Calls     []string
 }
@@ -80,3 +81,8 @@ func (f *Fake) WorkTree(context.Context) (string, error) {
 	f.call("WorkTree")
 	return f.Tree, f.Err
 }
+
+func (f *Fake) CommonDir(context.Context) (string, error) {
+	f.call("CommonDir")
+	return f.Common, f.Err
+}
diff --git a/internal/fsm/gate/gate_test.go b/internal/fsm/gate/gate_test.go
index 8287020..868d8e8 100644
--- a/internal/fsm/gate/gate_test.go
+++ b/internal/fsm/gate/gate_test.go
@@ -261,6 +261,10 @@ func TestG2RealGit(t *testing.T) {
 	if out := git(t, dir, "diff", "--cached", "--name-only"); out != "" {
 		t.Fatalf("real index touched: %s", out)
 	}
+	common, err := g.CommonDir(ctx)
+	if err != nil || !filepath.IsAbs(common) || filepath.Base(common) != ".git" {
+		t.Fatalf("common dir %q %v", common, err)
+	}
 	wd, tr, err := g.WorkingDiff(ctx, 1<<20)
 	if err != nil || tr || !strings.Contains(wd, "+changed") {
 		t.Fatalf("working diff: %v %v", tr, err)
@@ -322,6 +326,9 @@ func TestG2ExecErrorBranches(t *testing.T) {
 	if _, err := g.WorkTree(ctx); !errs.Is(err, CodeGit) {
 		t.Fatal("worktree exit 128")
 	}
+	if _, err := g.CommonDir(ctx); !errs.Is(err, CodeGit) {
+		t.Fatal("common exit 128")
+	}
 	// exit 1 where it is not a legal answer, and malformed stdout
 	exit1 := NewExec("/", func(_ context.Context, _ string, _ []string, args ...string) ([]byte, []byte, int, error) {
 		return []byte("garbage"), nil, 1, nil
@@ -344,6 +351,9 @@ func TestG2ExecErrorBranches(t *testing.T) {
 	if _, err := exit1.WorkTree(ctx); !errs.Is(err, CodeGit) {
 		t.Fatal("worktree add exit 1")
 	}
+	if _, err := exit1.CommonDir(ctx); !errs.Is(err, CodeGit) {
+		t.Fatal("common exit 1")
+	}
 	// write-tree exit 1 / short output after a successful add
 	calls := 0
 	wt := NewExec("/", func(_ context.Context, _ string, env []string, args ...string) ([]byte, []byte, int, error) {
@@ -447,7 +457,11 @@ func TestG4FakeContract(t *testing.T) {
 	if tr, _ := f.WorkTree(ctx); tr != "t" {
 		t.Fatal("tree")
 	}
-	if len(f.Calls) != 10 {
+	f.Common = "/r/.git"
+	if c, _ := f.CommonDir(ctx); c != "/r/.git" {
+		t.Fatal("common")
+	}
+	if len(f.Calls) != 11 {
 		t.Fatalf("calls %v", f.Calls)
 	}
 	boom := errors.New("boom")
diff --git a/internal/fsm/gate/git.go b/internal/fsm/gate/git.go
index 0062d1c..78db26e 100644
--- a/internal/fsm/gate/git.go
+++ b/internal/fsm/gate/git.go
@@ -39,6 +39,9 @@ type Git interface {
 	Diff(ctx context.Context, from, to string, max int) (diff string, truncated bool, err error)
 	// WorkingDiff returns `git diff HEAD` cut like Diff.
 	WorkingDiff(ctx context.Context, max int) (string, bool, error)
+	// CommonDir returns the absolute path of the repository's common git dir
+	// (shared by all worktrees), for the "same repository" check.
+	CommonDir(ctx context.Context) (string, error)
 	// WorkTree returns a content hash of the working tree (tracked + untracked,
 	// ignored excluded): `git add -A` into a scratch index, then `write-tree`.
 	WorkTree(ctx context.Context) (string, error)
@@ -226,6 +229,19 @@ func (g *execGit) WorkingDiff(ctx context.Context, max int) (string, bool, error
 	return d, t, nil
 }
 
+// CommonDir resolves `git rev-parse --path-format=absolute --git-common-dir`.
+func (g *execGit) CommonDir(ctx context.Context) (string, error) {
+	out, code, err := g.run(ctx, "rev-parse", "--path-format=absolute", "--git-common-dir")
+	if err != nil {
+		return "", err
+	}
+	dir := strings.TrimSpace(out)
+	if code != 0 || dir == "" {
+		return "", errs.E(CodeGit, "git-common-dir unavailable", "op", "rev-parse")
+	}
+	return dir, nil
+}
+
 // WorkTree hashes the working tree through a scratch index so content
 // changes (not just paths) move the hash. The scratch index lives in the
 // OS temp dir and is removed afterwards.
diff --git a/internal/fsm/run/errors.go b/internal/fsm/run/errors.go
index 4c35a5b..368b951 100644
--- a/internal/fsm/run/errors.go
+++ b/internal/fsm/run/errors.go
@@ -37,6 +37,7 @@ const (
 	ReasonFixBaselineOrder   = "fix_baseline_order"
 	ReasonUnsanctionedCmd    = "unsanctioned_cmd"
 	ReasonBadOutcome         = "bad_outcome"
+	ReasonTokensNegative     = "tokens_negative"
 )
 
 // CodeFor returns the FoldError code for a reason (§2.4 table).
diff --git a/internal/fsm/run/fold.go b/internal/fsm/run/fold.go
index c952828..dbdb84a 100644
--- a/internal/fsm/run/fold.go
+++ b/internal/fsm/run/fold.go
@@ -145,9 +145,15 @@ func Apply(st FoldState, ev Event) (FoldState, error) {
 		if p.Index != st.indexes[k] {
 			return FoldState{}, foldErr(ReasonStamp, ev)
 		}
+		if p.Tokens.Negative() {
+			return FoldState{}, foldErr(ReasonTokensNegative, ev)
+		}
 		next.indexes[k] = p.Index + 1
 		next.Tokens = next.Tokens.Add(p.Tokens)
 	case *TokenTotals:
+		if p.Negative() {
+			return FoldState{}, foldErr(ReasonTokensNegative, ev)
+		}
 		next.Tokens = next.Tokens.Add(*p)
 	case *CmdCallData:
 		if !sanctioned(st.AllowedCmds, p.Name) {
@@ -237,6 +243,9 @@ func FoldFull(events []Event) (FoldState, error) {
 
 // cow returns a state whose containers are fresh so that mutations never reach st. RawMessage
 // values (node outputs) are shared: they are never mutated in place.
+// NextIndex returns the llm_call index the next call under key must carry.
+func (st FoldState) NextIndex(key string) int { return st.indexes[key] }
+
 func (st FoldState) cow() FoldState {
 	next := st
 	next.Vars = cloneStringMap(st.Vars)
diff --git a/internal/fsm/run/fold_test.go b/internal/fsm/run/fold_test.go
index 1711ddb..126b624 100644
--- a/internal/fsm/run/fold_test.go
+++ b/internal/fsm/run/fold_test.go
@@ -1038,3 +1038,35 @@ func indexOf(evs []Event, typ string) int {
 	}
 	return -1
 }
+
+func TestFoldTokensNegativeAndNextIndex(t *testing.T) {
+	b := NewBuilder(runA)
+	b.Init(baseInit())
+	b.Event(TypeNodeOutput, out(`{}`), WithNode("n"))
+	b.Event(TypeLLMCall, LLMCallData{Kind: "k", Model: "m", Index: 0, Verdict: json.RawMessage(`{}`), Tokens: TokenTotals{Input: 5}}, WithNode("n"))
+	st, err := FoldFull(b.Events())
+	if err != nil {
+		t.Fatal(err)
+	}
+	if st.NextIndex("n@0") != 1 || st.NextIndex("zzz@0") != 0 {
+		t.Fatalf("NextIndex: %d", st.NextIndex("n@0"))
+	}
+	for name, tok := range map[string]TokenTotals{"input": {Input: -1}, "cache_read": {CacheRead: -1}, "cache_create": {CacheCreate: -1}, "output": {Output: -1}, "reasoning": {Reasoning: -1}} {
+		b2 := NewBuilder(runA)
+		b2.Init(baseInit())
+		b2.Event(TypeTokens, tok)
+		if _, err := Fold(b2.Events()); err == nil || err.(*FoldError).Reason != ReasonTokensNegative {
+			t.Errorf("tokens %s: %v", name, err)
+		}
+		b3 := NewBuilder(runA)
+		b3.Init(baseInit())
+		b3.Event(TypeNodeOutput, out(`{}`), WithNode("n"))
+		b3.Event(TypeLLMCall, LLMCallData{Kind: "k", Model: "m", Index: 0, Verdict: json.RawMessage(`{}`), Tokens: tok}, WithNode("n"))
+		if _, err := Fold(b3.Events()); err == nil || err.(*FoldError).Reason != ReasonTokensNegative {
+			t.Errorf("llm_call %s: %v", name, err)
+		}
+	}
+	if (TokenTotals{}).Negative() {
+		t.Fatal("zero is not negative")
+	}
+}
diff --git a/internal/fsm/run/types.go b/internal/fsm/run/types.go
index c03135b..36bf3fa 100644
--- a/internal/fsm/run/types.go
+++ b/internal/fsm/run/types.go
@@ -103,6 +103,12 @@ type TokenTotals struct {
 }
 
 // Add returns the field-wise sum.
+// Negative reports whether any counter is below zero (rejected by Apply so a
+// driver cannot pay down a budget with negative records).
+func (t TokenTotals) Negative() bool {
+	return t.Input < 0 || t.CacheRead < 0 || t.CacheCreate < 0 || t.Output < 0 || t.Reasoning < 0
+}
+
 func (t TokenTotals) Add(u TokenTotals) TokenTotals {
 	return TokenTotals{Input: t.Input + u.Input, CacheRead: t.CacheRead + u.CacheRead, CacheCreate: t.CacheCreate + u.CacheCreate, Output: t.Output + u.Output, Reasoning: t.Reasoning + u.Reasoning}
 }
diff --git a/workflows/sdlc-loop.yaml b/workflows/sdlc-loop.yaml
index 4b2ee3c..1845616 100644
--- a/workflows/sdlc-loop.yaml
+++ b/workflows/sdlc-loop.yaml
@@ -7,7 +7,9 @@ vars:
   JUDGE_EFFORT: {required: true}   # Pareto-selected with JUDGE (spec §17); pinned to medium under --calibration
 states: [discover, adjudicate, fix, verify, done, failed]
 transitions:                                  # ordered
+  - {from: discover,   to: done,       gate: findings_empty,     outcome: clean}
   - {from: discover,   to: adjudicate, gate: findings_nonempty}
+  - {from: adjudicate, to: done,       gate: confirmed_empty,    outcome: clean}
   - {from: adjudicate, to: fix,        gate: confirmed_nonempty}
   - {from: fix,        to: verify,     gate: commit_exists}
   - {from: verify,     to: done,       gate: all_fixed,   outcome: fixed}
@@ -20,4 +22,7 @@ nodes:
 convergence:
   any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]
 repo_mode: advisory
-# on_overflow: {cmd: ["./notify-overflow.sh"], timeout: 30}   # requires --allow-custom-cmds <sha256>
+# Escape hatch (requires --allow-custom-cmds <sha256>; see docs/fsm):
+# cmds:
+#   notify: {argv: [bash, ./notify-overflow.sh], timeout: 30, env: [SLACK_WEBHOOK]}
+# on_overflow: notify


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

