# metareview task-done context

Run ID: `mrv-20260901-164813816597000-task-done-multilang-testconv-seam-01cc5b3d`

## Task

Advisory task target: multilang-testconv-seam

## Git

- Base: `0ea89c9270e89690b2fb63019382ee408855d63d`
- Head: `6e22efe3da4b914e931324db788ed7037a232a73`
- Branch: `multilang-testconv-seam`
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `88916`
- Filtered diff bytes: `83151`
- Risk level: `none`
- Generated files excluded: docs/metareview/evidence/multilang-testconv-seam-pr-a.md

## Context Shard Plan

Not sharded.

## Review Manifest

- Manifest verdict: `PASS`
- Source manifest hash: not sharded
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- docs/handoffs/handoff-2026-09-01-multilang-testconvention-seam.md
- internal/fsm/kind/prove.go
- internal/fsm/kind/prove_mutation.go
- internal/fsm/kind/prove_reproduction.go
- internal/fsm/kind/prove_reproduction_test.go
- internal/fsm/kind/prove_test.go
- internal/mutation/prescreen.go
- internal/mutation/reproduction.go
- internal/mutation/reproduction_test.go
- internal/mutation/verify.go
- internal/mutation/verify_test.go
- internal/testconv/goconv.go
- internal/testconv/testconv.go
- internal/testconv/testconv_test.go
- tests/coverage-floor.txt

### Path Dispositions
- docs/metareview/evidence/multilang-testconv-seam-pr-a.md: generated (metareview generated review artifact excluded from source manifest)

### Manifest Blockers
No manifest blockers.

## Changed Files

- docs/handoffs/handoff-2026-09-01-multilang-testconvention-seam.md
- internal/fsm/kind/prove.go
- internal/fsm/kind/prove_mutation.go
- internal/fsm/kind/prove_reproduction.go
- internal/fsm/kind/prove_reproduction_test.go
- internal/fsm/kind/prove_test.go
- internal/mutation/prescreen.go
- internal/mutation/reproduction.go
- internal/mutation/reproduction_test.go
- internal/mutation/verify.go
- internal/mutation/verify_test.go
- internal/testconv/goconv.go
- internal/testconv/testconv.go
- internal/testconv/testconv_test.go
- tests/coverage-floor.txt

## Diff

````diff
diff --git a/docs/handoffs/handoff-2026-09-01-multilang-testconvention-seam.md b/docs/handoffs/handoff-2026-09-01-multilang-testconvention-seam.md
new file mode 100644
index 0000000..1aa4ce6
--- /dev/null
+++ b/docs/handoffs/handoff-2026-09-01-multilang-testconvention-seam.md
@@ -0,0 +1,143 @@
+# Handoff — the `TestConvention` seam (multi-language: Go reference, then TypeScript)
+
+**Date:** 2026-09-01. **Follow-up of:** the pins-and-bug-class Execution Loop (T1.3–T2.2, merged
+#41–#46) and the 0.10.0 candidate "The pins/bug-class engine is Go-only at the language seams"
+(`docs/0.10.0-candidates.md`, merged #47). **Spec basis:** §4.2 frames the test convention as
+pluggable. **Decision (Dave, 2026-09-01):** build the seam, TypeScript first.
+
+## Goal
+
+The reproduction/deletion/trivial-pin machinery has a language-agnostic spine but four leaf sites
+hard-wired to Go. Extract a `TestConvention` seam, keep Go behavior byte-identical behind it, then add
+a TypeScript convention (Jest/Vitest). Everything else — FSM/gates, the real `git worktree` engine,
+consent-hashed `cmd`, unified-diff parsing, the exit-code scope check, mutation-kill re-verification —
+is already language-neutral and MUST NOT change behavior.
+
+## The four Go-hardwired sites (exact locations)
+
+1. **`internal/mutation/reproduction.go`**
+   - `isTestFile(path)` (l.139) — `strings.HasSuffix(path, "_test.go")`.
+   - `runTest` (l.381) — appends **`-run ^<test>$ -v`** to `TestCmd`; the whole Go single-test
+     narrowing + verbose convention.
+   - `classify(test, code, out)` (l.454) — keys on **`=== RUN   <test>`** and **`--- FAIL: <test>`**.
+   - `isCompileError(out)` (l.473) — **`[build failed]` / `[setup failed]` / `[vet failed]`**.
+2. **`internal/mutation/prescreen.go`** — `semanticallyNull` / `goTokens` / `directive`: the §9.8 R7
+   trivial-pin pre-screen, a `go/scanner` token walk with Go directive rules (`//go:build`, `//export`,
+   `//extern`, `// +build`, `//line`).
+3. **`internal/fsm/kind/prove.go`**
+   - `deletesATest(diff)` / `testFuncRemovedRe` (l.267/272) — Go's `^-func (Test|Benchmark|Fuzz|Example)…`
+     removal rule (§9.6 gate).
+   - `packageHasTests(dir, file)` (l.442) — globs **`*_test.go`** in the file's directory.
+4. (`internal/mutation/verify.go` — the pin `Verifier` — is ALREADY language-agnostic: pins are
+   proven by exit code / build-vs-test split, no marker parsing. Leave it. Confirm during build.)
+
+## The seam
+
+New leaf package **`internal/testconv`** (zero internal deps, so both `internal/mutation` and
+`internal/fsm/kind` can import it without a layering cycle — mirrors reproduction.go's "this package
+may not import the FSM" boundary):
+
+```go
+package testconv
+
+// TestReport is the normalized result of running the suite (or one test), across runners — the
+// analogue of mutation.Report. Built ONLY from a runner's structured output; never source-parsed.
+type TestReport struct {
+    Tests       map[string]Outcome // test id → Passed|Failed (the SET that executed; drives deletion detection)
+    BuildFailed bool               // a compile/transpile/setup failure — no assertion was reached
+}
+type Outcome int
+const ( Passed Outcome = iota; Failed )
+
+// Class is the assertion-vs-compile axis for ONE target test, DERIVED generically from a TestReport
+// (was mutation.failClass; no per-language string scraping).
+type Class int
+const ( ClsAssert Class = iota; ClsPassed; ClsNoTest; ClsCompile; ClsOther )
+
+type Convention interface {
+    Name() string                                    // "go" | "typescript"
+    IsTestFile(path string) bool                     // reproduction partition + DirHasTests
+    RunArgs(base []string, test string) []string      // narrow base cmd to ONE test, STRUCTURED output
+    SuiteArgs(base []string) []string                 // whole suite, STRUCTURED output (deletion set + ScopeSuite)
+    ParseReport(code int, stdout, stderr string) (TestReport, error) // runner's -json → normalized; unreadable → error
+    DirHasTests(root, file string) bool               // §3.1 owesPin gate — glob the runner's test-file pattern
+    SemanticallyNull(orig, mutated string) bool       // §9.8 R7 trivial-pin pre-screen (Go: go/scanner)
+}
+
+func For(name string) (Convention, bool)             // registry; "" / unknown → (nil,false), FAIL CLOSED
+```
+
+**Classification** is **generic** over `TestReport`: `Classify(report, test) Class` (target
+Passed→ClsPassed; Failed→ClsAssert; BuildFailed & absent→ClsCompile; absent→ClsNoTest; else ClsOther)
+— one function, no per-language string scraping. **Test-deletion** stays a per-language
+`DeletesATest(diff) bool` behind the seam (the §9.6 trigger is a CHEAP diff check — computing it from a
+runner set-diff would force two extra whole-suite runs on every prove). Each convention uses its most
+robust AVAILABLE mechanism: Go matches its `testing` package's exact name rule (a language spec, not a
+hand-rolled parser); TS — which has no single-line rule (`it`/`test`/`describe` are aliasable) — will
+use the **runner's executed-test set-diff** across the pre/post suite reports it already produces (a
+`DeletesTests(pre, post TestReport)` path added in PR-B, no regex, no parser). The genuinely fragile
+site, classification, is what `go test -json` fixes now.
+
+`GoConvention.ParseReport` reads the **`go test -json`** event stream (test2json): filter to the
+target `Package`+`Test`; a `pass`/`fail` action sets the outcome; a package `fail` with
+`build`/`setup`/`vet` output (or a `build-fail` action) sets `BuildFailed`. This replaces the current
+`=== RUN`/`--- FAIL:` scrape AND kills the multi-package sibling-noise bug at the root — siblings are
+just different `Package` values, ignored structurally.
+
+## Threading (how each site gets its Convention)
+
+- **Selection:** a new workflow node param **`test_convention`** (string), resolved in
+  `proveExec.Execute` beside `testCmdParam`, **defaulting to `"go"`** when absent (every existing
+  workflow keeps working unchanged). Put the resolved `Convention` on **`ProveSpec.Convention`**.
+- `ProveSpec.Convention` → `mutation.Reproducer.Convention`; `Reproducer` runs `RunArgs`/`SuiteArgs`,
+  calls `ParseReport`, derives `Class` via `testconv.Classify`. `isTestFile` → `Convention.IsTestFile`.
+- `ProveSpec.Convention` → `deletesATest` → `Convention.DeletesATest(diff)` (Go keeps its exact
+  name-rule matcher) and `owesPin`/`packageHasTests` → `Convention.DirHasTests`.
+- `Verifier`/`Reproducer` gain a `Convention` field; `SemanticallyNull` routes through it. A nil
+  Convention in `mutation` defaults to Go (test ergonomics); the FSM path ALWAYS sets it.
+- **Fail closed:** an unknown/empty convention name where a proof needs one → `PinUnverifiable`
+  ("no test convention for <name>"), NEVER a fall-through to Go. `ParseReport` on output it cannot
+  read returns an error (→ unverifiable), never an empty report scored clean (the mutation package's
+  refuse-unreadable rule).
+
+## Decomposition — TWO PRs (methodology: one language per PR)
+
+**PR-A — the seam + `GoConvention` on `go test -json`; rework Go onto the robust path** (Dave chose
+"uniform: rework Go too, dep-free", 2026-09-01). Introduce `internal/testconv` with the normalized
+`TestReport` model, generic `Classify`, and `GoConvention` reading `go test -json`;
+rewire reproduction.go, prove.go's `deletesATest`/`packageHasTests`, and the `SemanticallyNull` call
+through the seam (Go's `DeletesATest` keeps its merged name-rule matcher unchanged).
+Existing reproduction/deletion tests move from console-text inputs to `go test -json` fixtures (the
+*behavior* is identical or strictly more robust; the *inputs* change). Default `test_convention:"go"`.
+Mutation-verified the seam is load-bearing (stub `GoConvention.ParseReport` to always-Passed → a
+reproduction test reddens). `SemanticallyNull` stays `go/scanner`. **No bespoke parser, no new dep.**
+
+**PR-B — add `TypeScriptConvention` (Jest/Vitest) + per-target selection.**
+- `IsTestFile`/`DirHasTests`: `*.test.ts(x)` / `*.spec.ts(x)` (+ `.js`/`.jsx`).
+- `RunArgs`/`SuiteArgs`: `--json` (Jest) / `--reporter=json` (Vitest). **Test identity is the
+  `describe > it` STRING**, not a function name — `Proof.Test` carries that.
+- `ParseReport`: normalize Jest `--json` (`testResults[].assertionResults[].status`
+  "passed"/"failed"; runtime/transpile error + no assertions → `BuildFailed`) into `TestReport`. Same
+  generic `Classify`/deletion-diff as Go — NO new parser, NO regex.
+- `SemanticallyNull`: no stdlib JS/TS tokenizer, and PR-B adds NO parser dependency (the dep-free
+  rule). Return `false` conservatively → always run the full compile/break steps (safe; skips only the
+  trivial-pin optimization for TS). A pluggable tokenizer is a later, separately-justified step.
+- **Multi-package Jest fixtures required** before trusting `ParseReport`
+  ([[single-package-fixtures-hide-classify-bugs]]).
+
+## Constraints (all from the memories / prior rounds)
+
+- **TDD red-first, mutation-verified, `internal/fsm/*` at 100%.** New `internal/testconv` and
+  `internal/mutation` additions carry their own tests to their floor.
+- **Classification is the per-language-fragile part.** The Go classifier already burned a round on
+  multi-package `go test ./...` sibling noise — see [[single-package-fixtures-hide-classify-bugs]].
+  The TS classifier needs monorepo/multi-package Jest fixtures (JSON with several `testResults`) before
+  it is trusted.
+- One language per PR through `review task-done` → PR → bot-shepherd → squash-merge → sync main.
+- Watch the task-done self-reference trap ([[task-done-todo-self-reference-trap]]) and unique agent
+  scratch paths ([[unique-agent-scratch-paths]]) if any subagents run mutations.
+
+## Related memories
+
+[[pins-bug-class-execution-loop-status]], [[single-package-fixtures-hide-classify-bugs]],
+[[mock-judge-hides-node-wiring-bugs]], [[verify-delegated-fixes]], [[task-done-todo-self-reference-trap]].
diff --git a/internal/fsm/kind/prove.go b/internal/fsm/kind/prove.go
index 599bcd2..c3a2b31 100644
--- a/internal/fsm/kind/prove.go
+++ b/internal/fsm/kind/prove.go
@@ -4,8 +4,6 @@ import (
 	"context"
 	"encoding/json"
 	"fmt"
-	"path/filepath"
-	"regexp"
 	"strings"
 	"time"
 
@@ -14,6 +12,7 @@ import (
 	"github.com/dsifry/metareview/internal/fsm/machine"
 	"github.com/dsifry/metareview/internal/fsm/run"
 	"github.com/dsifry/metareview/internal/fsm/workflow"
+	"github.com/dsifry/metareview/internal/testconv"
 )
 
 // Prove is the mutation-verify node kind (spec §3.1 / §9.1): the only deterministic non-gate node.
@@ -40,6 +39,10 @@ type ProveSpec struct {
 	Timeout    time.Duration
 	PreFixSHA  string // Snapshot.FixEntryHead — the pre-fix tree for reproduction/deletion proofs
 	PostFixSHA string // Snapshot.Head — the post-fix tree
+	// Convention is the language seam (spec §4.2): the test-file predicate, the test runner and its
+	// report reader, the removed-test rule, and the trivial-pin pre-screen. It is resolved from the
+	// node's `test_convention` param (default "go"); an unknown name aborts the node (fail closed).
+	Convention testconv.Convention
 }
 
 // Prover verifies a fix's differential proofs and returns one ProofResult per input proof, in order,
@@ -132,6 +135,22 @@ func testCmdParam(snap run.Snapshot, node *workflow.Node) []string {
 	return nil
 }
 
+// conventionParam resolves the node's `test_convention` param to a language convention. An absent or
+// empty param defaults to "go", so every existing workflow keeps working unchanged. An unknown name is
+// a configuration error that ABORTS the node (fail closed) — never a silent fall-through to Go, which
+// would score a non-Go repository by Go rules.
+func conventionParam(node *workflow.Node) (testconv.Convention, error) {
+	name, _ := node.Params["test_convention"].(string)
+	if name == "" {
+		name = "go"
+	}
+	conv, ok := testconv.For(name)
+	if !ok {
+		return nil, errs.E(machine.CodeExecutorFailed, "prove: unknown test_convention "+name, "reason", "unknown_test_convention")
+	}
+	return conv, nil
+}
+
 func (e *proveExec) Execute(ctx context.Context, in machine.ExecInput) (json.RawMessage, error) {
 	if e.prover == nil {
 		return nil, errNoProver()
@@ -176,7 +195,11 @@ func (e *proveExec) Execute(ctx context.Context, in machine.ExecInput) (json.Raw
 		}
 	}
 	dir := proveDir(in.Snap)
-	spec := ProveSpec{TestCmd: testCmdParam(in.Snap, in.Node), Dir: dir, PreFixSHA: in.Snap.FixEntryHead, PostFixSHA: in.Snap.Head}
+	conv, err := conventionParam(in.Node)
+	if err != nil {
+		return nil, err
+	}
+	spec := ProveSpec{TestCmd: testCmdParam(in.Snap, in.Node), Dir: dir, PreFixSHA: in.Snap.FixEntryHead, PostFixSHA: in.Snap.Head, Convention: conv}
 	verified, err := e.prover.ProvePins(ctx, toProve, spec)
 	if err != nil {
 		return nil, err
@@ -237,7 +260,7 @@ func (e *proveExec) Execute(ctx context.Context, in machine.ExecInput) (json.Raw
 		if settled[b.ID] || blocked[b.ID] {
 			continue
 		}
-		if owesPin(in.Diff.Text, dir, b.File) {
+		if owesPin(spec.Convention, in.Diff.Text, dir, b.File) {
 			delta.Findings = append(delta.Findings, owedPinMarker(b))
 		}
 	}
@@ -259,28 +282,6 @@ func (e *proveExec) Execute(ctx context.Context, in machine.ExecInput) (json.Raw
 	return raw, nil
 }
 
-// testFuncRemovedRe matches a removed Go test declaration, following the testing package's naming
-// rule: TestXxx/BenchmarkXxx/FuzzXxx/ExampleXxx where Xxx is EMPTY or starts with a non-lowercase rune
-// (so `Test`/`TestFoo`/`Test_x`/`TestÜ` match but `Testhelper` does not), with optional whitespace
-// before the parameter list (`func TestFoo (t ...)` is legal). Suffix runes are Unicode identifier
-// characters, not ASCII-only.
-var testFuncRemovedRe = regexp.MustCompile(`^-func (Test|Benchmark|Fuzz|Example)([^\p{Ll}\s(][\p{L}\p{N}_]*)?\s*\(`)
-
-// deletesATest reports whether the diff removes a Go test function — a removed `func Test.../
-// Benchmark.../Fuzz.../Example...` line, which covers both a deleted *_test.go file (its func lines
-// appear as removed) and a test removed from a surviving file. Header lines ("--- a/…") are skipped.
-func deletesATest(diff string) bool {
-	for _, line := range strings.Split(diff, "\n") {
-		if strings.HasPrefix(line, "---") {
-			continue
-		}
-		if testFuncRemovedRe.MatchString(line) {
-			return true
-		}
-	}
-	return false
-}
-
 // testDeletionRegressions runs the §9.6 mutation-kill non-regression check. It is a no-op unless the
 // fix deleted a test. Then it re-verifies every previously-PROVEN pin on the current post-deletion
 // tree; a pin that now SURVIVES (the mutation still applies but the tests no longer catch it) means the
@@ -290,7 +291,7 @@ func deletesATest(diff string) bool {
 // removed" pre-filter was tried and removed: a substring match on removed lines dropped still-live pins
 // like From "n < 10" against a removed "n < 100", silently bypassing the gate.)
 func (e *proveExec) testDeletionRegressions(ctx context.Context, in machine.ExecInput, spec ProveSpec) ([]run.Finding, error) {
-	if !deletesATest(in.Diff.Text) {
+	if !spec.Convention.DeletesATest(in.Diff.Text) {
 		return nil, nil
 	}
 	var recheck []run.DifferentialProof
@@ -431,16 +432,8 @@ func isAddedLineInFile(diff, file, from string) bool {
 // added a line in the finding's OWN file AND that file's package has tests. A cross-file remedy (no
 // added line in the finding's file) or a no-test package owes no pin — auditable exemptions, never a
 // silent pass.
-func owesPin(diff, dir, file string) bool {
-	return file != "" && len(judge.AddedLinesInFile(diff, file)) > 0 && packageHasTests(dir, file)
-}
-
-// packageHasTests reports whether the directory holding file contains Go test files (*_test.go). It
-// is the machine-determinable "package that has test files" of §4.2; a non-Go partition would key on
-// its own test convention here.
-func packageHasTests(dir, file string) bool {
-	matches, err := filepath.Glob(filepath.Join(dir, filepath.Dir(file), "*_test.go"))
-	return err == nil && len(matches) > 0
+func owesPin(conv testconv.Convention, diff, dir, file string) bool {
+	return file != "" && len(judge.AddedLinesInFile(diff, file)) > 0 && conv.DirHasTests(dir, file)
 }
 
 // owedPinMarker is the blocking finding for a confirmed finding that owes a pin the fix never
diff --git a/internal/fsm/kind/prove_mutation.go b/internal/fsm/kind/prove_mutation.go
index a37911c..7c3a7be 100644
--- a/internal/fsm/kind/prove_mutation.go
+++ b/internal/fsm/kind/prove_mutation.go
@@ -29,7 +29,7 @@ func (mp MutationProver) ProvePins(ctx context.Context, proofs []run.Differentia
 	for i, p := range proofs {
 		pins[i] = mutation.Pin{File: p.Pin.File, From: p.Pin.From, To: p.Pin.To, Test: p.Pin.Test}
 	}
-	v := mutation.Verifier{Dir: spec.Dir, TestCmd: spec.TestCmd, BuildCmd: spec.BuildCmd, Timeout: spec.Timeout, Run: mp.Run}
+	v := mutation.Verifier{Dir: spec.Dir, TestCmd: spec.TestCmd, BuildCmd: spec.BuildCmd, Timeout: spec.Timeout, Run: mp.Run, Convention: spec.Convention}
 	mrs, err := v.Verify(ctx, pins)
 	if err != nil {
 		return nil, err
diff --git a/internal/fsm/kind/prove_reproduction.go b/internal/fsm/kind/prove_reproduction.go
index f0d6e05..fe4cd92 100644
--- a/internal/fsm/kind/prove_reproduction.go
+++ b/internal/fsm/kind/prove_reproduction.go
@@ -39,6 +39,7 @@ func (rp ReproductionProver) engine(spec ProveSpec) (mutation.Reproducer, gitFn)
 	return mutation.Reproducer{
 		Dir: spec.Dir, PreFixSHA: spec.PreFixSHA, PostFixSHA: spec.PostFixSHA,
 		TestCmd: spec.TestCmd, Timeout: spec.Timeout, Git: git, Run: rp.Run,
+		Convention: spec.Convention,
 	}, git
 }
 
diff --git a/internal/fsm/kind/prove_reproduction_test.go b/internal/fsm/kind/prove_reproduction_test.go
index 0862107..d3fbfdf 100644
--- a/internal/fsm/kind/prove_reproduction_test.go
+++ b/internal/fsm/kind/prove_reproduction_test.go
@@ -52,9 +52,9 @@ func TestReproductionProverMapsProven(t *testing.T) {
 		Run: func(_ context.Context, _ string, _ []string) (int, string, error) {
 			calls++
 			if calls == 1 {
-				return 1, "=== RUN   T\n--- FAIL: T\n", nil
+				return 1, "{\"Action\":\"run\",\"Test\":\"T\"}\n{\"Action\":\"output\",\"Test\":\"T\",\"Output\":\"assertion failed\\n\"}\n{\"Action\":\"fail\",\"Test\":\"T\"}\n", nil
 			}
-			return 0, "=== RUN   T\n--- PASS: T\nok\n", nil
+			return 0, "{\"Action\":\"run\",\"Test\":\"T\"}\n{\"Action\":\"pass\",\"Test\":\"T\"}\n", nil
 		},
 	}
 	spec := ProveSpec{Dir: t.TempDir(), TestCmd: []string{"go", "test", "./..."}, PreFixSHA: "pre", PostFixSHA: "post"}
@@ -91,9 +91,9 @@ func TestProversRoutesByKind(t *testing.T) {
 			Run: func(_ context.Context, _ string, _ []string) (int, string, error) {
 				reproCalls++
 				if reproCalls == 1 {
-					return 1, "=== RUN   T\n--- FAIL: T\n", nil
+					return 1, "{\"Action\":\"run\",\"Test\":\"T\"}\n{\"Action\":\"output\",\"Test\":\"T\",\"Output\":\"assertion failed\\n\"}\n{\"Action\":\"fail\",\"Test\":\"T\"}\n", nil
 				}
-				return 0, "=== RUN   T\n--- PASS: T\nok\n", nil
+				return 0, "{\"Action\":\"run\",\"Test\":\"T\"}\n{\"Action\":\"pass\",\"Test\":\"T\"}\n", nil
 			},
 		},
 	}
@@ -160,9 +160,9 @@ func delReviewerRun() (func(context.Context, string, []string) (int, string, err
 		if hasRun {
 			repro++
 			if repro == 1 {
-				return 1, "=== RUN   T\n--- FAIL: T\n", nil
+				return 1, "{\"Action\":\"run\",\"Test\":\"T\"}\n{\"Action\":\"output\",\"Test\":\"T\",\"Output\":\"assertion failed\\n\"}\n{\"Action\":\"fail\",\"Test\":\"T\"}\n", nil
 			}
-			return 0, "=== RUN   T\n--- PASS: T\nok\n", nil
+			return 0, "{\"Action\":\"run\",\"Test\":\"T\"}\n{\"Action\":\"pass\",\"Test\":\"T\"}\n", nil
 		}
 		return 0, "ok\n", nil // whole-suite scope run: green
 	}
@@ -196,7 +196,7 @@ func TestProveDeletionProven(t *testing.T) {
 	if len(rs) != 1 || !rs[0].Proven || rs[0].Outcome != run.PinProven {
 		t.Fatalf("a well-formed deletion must be proven: %+v", rs)
 	}
-	if !strings.Contains(rs[0].FailBefore, "--- FAIL: T") {
+	if !strings.Contains(rs[0].FailBefore, "assertion failed") {
 		t.Fatalf("a proven deletion must carry the fail-before output for the §9.2 reviewer: %q", rs[0].FailBefore)
 	}
 }
@@ -260,7 +260,7 @@ func TestProveDeletionReproductionNotProven(t *testing.T) {
 	runFn := func(_ context.Context, _ string, argv []string) (int, string, error) {
 		for _, x := range argv {
 			if x == "-run" {
-				return 0, "=== RUN   T\n--- PASS: T\nok\n", nil // passes pre-fix → survived
+				return 0, "{\"Action\":\"run\",\"Test\":\"T\"}\n{\"Action\":\"pass\",\"Test\":\"T\"}\n", nil // passes pre-fix → survived
 			}
 		}
 		return 0, "ok\n", nil
@@ -279,9 +279,9 @@ func TestProveDeletionScopeRegresses(t *testing.T) {
 			if x == "-run" { // reproduction: fail-before then pass-after
 				repro++
 				if repro == 1 {
-					return 1, "=== RUN   T\n--- FAIL: T\n", nil
+					return 1, "{\"Action\":\"run\",\"Test\":\"T\"}\n{\"Action\":\"output\",\"Test\":\"T\",\"Output\":\"assertion failed\\n\"}\n{\"Action\":\"fail\",\"Test\":\"T\"}\n", nil
 				}
-				return 0, "=== RUN   T\n--- PASS: T\nok\n", nil
+				return 0, "{\"Action\":\"run\",\"Test\":\"T\"}\n{\"Action\":\"pass\",\"Test\":\"T\"}\n", nil
 			}
 		}
 		scope++ // whole suite: baseline green, post-fix RED → over-deletion regression
diff --git a/internal/fsm/kind/prove_test.go b/internal/fsm/kind/prove_test.go
index 0079a49..a67da1a 100644
--- a/internal/fsm/kind/prove_test.go
+++ b/internal/fsm/kind/prove_test.go
@@ -103,6 +103,22 @@ func (f *fakeReviewer) Call(_ context.Context, r judge.Request) (judge.Verdict,
 	return judge.Verdict{Kind: r.Kind, Decision: f.decision}, nil
 }
 
+// An unknown test_convention aborts the node (fail closed): a non-Go repository must never be scored by
+// Go's rules via a silent default. The default (absent param) is exercised by every other prove test.
+func TestProveUnknownConventionAborts(t *testing.T) {
+	r := mustNew(t, judge.NewMock(judge.Script{}), true)
+	r.execs[Prove] = &proveExec{prover: &mockProver{}, symptom: &fakeReviewer{decision: true}}
+	ex, _ := r.Executor(Prove)
+	node := &workflow.Node{Name: "prove", Kind: Prove, Exec: "fork", Params: map[string]any{"test_convention": "ruby"}}
+	_, err := ex.Execute(context.Background(), machine.ExecInput{Snap: run.Snapshot{}, Node: node, Diff: machine.Diff{Text: "@@\n+x\n"}, Audit: (&audits{}).fn})
+	if err == nil {
+		t.Fatal("an unknown test_convention must abort the node, not fall through to Go")
+	}
+	if !strings.Contains(err.Error(), "unknown test_convention") {
+		t.Fatalf("the abort must name the unknown convention: %v", err)
+	}
+}
+
 func runProve(t *testing.T, snap run.Snapshot, diff string, prover Prover) run.Delta {
 	t.Helper()
 	// A matches-by-default reviewer: it runs only on a Proven reproduction and lets it stand, so a
@@ -716,28 +732,3 @@ func TestProveTestDeletionRecheckErrorAborts(t *testing.T) {
 		t.Fatal("a re-verification error must abort the run")
 	}
 }
-
-func TestDeletesATestAndRemovedLine(t *testing.T) {
-	yes := "diff --git a/x_test.go b/x_test.go\n--- a/x_test.go\n+++ b/x_test.go\n@@ -1,1 +0,0 @@\n-func TestFoo(t *testing.T) {}\n"
-	if !deletesATest(yes) {
-		t.Fatal("a removed test func must be detected")
-	}
-	for _, kind := range []string{"Benchmark", "Fuzz", "Example"} {
-		if !deletesATest("@@\n-func " + kind + "Bar(") {
-			t.Fatalf("a removed %s func must be detected", kind)
-		}
-	}
-	// Go-legal forms that must be DETECTED: whitespace before the paren, an empty suffix (`Test`),
-	// an underscore/Unicode suffix.
-	for _, ok := range []string{"-func TestFoo (t *testing.T) {", "-func Test(t *testing.T){", "-func Test_x(t *testing.T){", "-func TestÜ(t *testing.T){", "-func TestMain(m *testing.M){"} {
-		if !deletesATest(ok) {
-			t.Fatalf("a Go-legal removed test decl must be detected: %q", ok)
-		}
-	}
-	// Must NOT count: a lowercase suffix (not a test per Go's rule), a non-test func, the "---" header.
-	for _, no := range []string{"-func Testhelper(t *testing.T) {", "-func helper() {}", "--- a/x_test.go"} {
-		if deletesATest(no) {
-			t.Fatalf("a non-test removed line must not count: %q", no)
-		}
-	}
-}
diff --git a/internal/mutation/prescreen.go b/internal/mutation/prescreen.go
deleted file mode 100644
index 68037c5..0000000
--- a/internal/mutation/prescreen.go
+++ /dev/null
@@ -1,103 +0,0 @@
-package mutation
-
-import (
-	"go/scanner"
-	"go/token"
-	"strings"
-)
-
-// semanticallyNull reports whether orig and mutated are the SAME Go program ignoring comments and
-// whitespace — i.e. the pin's mutation is a no-op the compiler cannot tell apart (spec §9.8 R7). It is
-// the trivial-pin pre-screen: a comment- or whitespace-only mutation compiles and breaks no test, so
-// without this it would surface as a phantom `survived`.
-//
-// It compares the two token streams (comments dropped by the scanner, whitespace irrelevant between
-// tokens), which is position-independent and pure. If EITHER body fails to scan cleanly, it returns
-// false (NOT null): a mutation that breaks scanning is the compile step's job, not this pre-screen's.
-// It does not attempt dead-code / reachability detection — no pure syntactic method covers it, and the
-// spec states no reachability contract, so that case falls through to the ordinary compile-then-break
-// steps.
-func semanticallyNull(orig, mutated string) bool {
-	a, ok := goTokens(orig)
-	if !ok {
-		return false
-	}
-	b, ok := goTokens(mutated)
-	if !ok {
-		return false
-	}
-	if len(a) != len(b) {
-		return false
-	}
-	for i := range a {
-		if a[i] != b[i] {
-			return false
-		}
-	}
-	return true
-}
-
-// goTokens scans src into its Go token stream, returning one "tok lit" string per token (the literal
-// distinguishes identifiers/literals whose token class is the same). Ordinary comments are dropped
-// (they are semantically null), but a comment DIRECTIVE (//go:build, //go:embed, //line, …) is KEPT:
-// it affects build selection, embedded data, or linker behaviour, so a change to one is NOT null.
-// ok is false if the source did not scan cleanly.
-func goTokens(src string) (toks []string, ok bool) {
-	var fset token.FileSet
-	file := fset.AddFile("", fset.Base(), len(src))
-	var s scanner.Scanner
-	ok = true
-	s.Init(file, []byte(src), func(token.Position, string) { ok = false }, scanner.ScanComments)
-	for {
-		_, tok, lit := s.Scan()
-		if tok == token.EOF {
-			break
-		}
-		if tok == token.COMMENT {
-			if d, isDir := directive(lit); isDir {
-				toks = append(toks, "//dir "+d)
-			}
-			continue
-		}
-		if lit != "" {
-			toks = append(toks, tok.String()+" "+lit)
-		} else {
-			toks = append(toks, tok.String())
-		}
-	}
-	return toks, ok
-}
-
-// directive reports whether a line comment is a meaningful Go comment directive (returning its text
-// without the leading //). It covers go/ast's isDirective form — a "//line " directive, or "//name:arg"
-// with a lowercase/digit name and no space before the colon (e.g. //go:build, //go:embed, //go:noescape)
-// — PLUS the other comments that change linkage or build selection: cgo "//export Name", gccgo
-// "//extern name", and the legacy "// +build" constraint. A change to any of these is NOT semantically
-// null, so it must not be classified a trivial pin. An ordinary comment (even one with a colon like
-// "// note: x", or a block comment) is not a directive.
-func directive(comment string) (string, bool) {
-	if !strings.HasPrefix(comment, "//") {
-		return "", false // block comments are never directives
-	}
-	c := comment[2:]
-	// //line (line directive), //export (cgo), and //extern (gccgo) have the keyword immediately after
-	// the //; the legacy build constraint is "//" then any run of spaces/tabs then "+build" (the Go
-	// toolchain tolerates extra whitespace there). Each changes linkage or build selection.
-	if strings.HasPrefix(c, "line ") || strings.HasPrefix(c, "export ") || strings.HasPrefix(c, "extern ") ||
-		strings.HasPrefix(strings.TrimLeft(c, " \t"), "+build") {
-		return c, true
-	}
-	// //name:arg — a lowercase/digit name, no space before the colon (e.g. //go:build).
-	colon := strings.Index(c, ":")
-	if colon <= 0 || colon+1 >= len(c) {
-		return "", false
-	}
-	for i := 0; i < colon; i++ {
-		b := c[i]
-		lowerOrDigit := b >= 'a' && b <= 'z' || b >= '0' && b <= '9'
-		if !lowerOrDigit {
-			return "", false
-		}
-	}
-	return c, true
-}
diff --git a/internal/mutation/reproduction.go b/internal/mutation/reproduction.go
index 31fe785..4a54c9e 100644
--- a/internal/mutation/reproduction.go
+++ b/internal/mutation/reproduction.go
@@ -6,9 +6,10 @@ import (
 	"os"
 	"os/exec"
 	"path/filepath"
-	"regexp"
 	"strings"
 	"time"
+
+	"github.com/dsifry/metareview/internal/testconv"
 )
 
 // Proof is a reproduction-form differential claim (spec §9.1): one committed test that must FAIL on
@@ -57,6 +58,10 @@ type Reproducer struct {
 	PostFixSHA string        // the post-fix tree (run.Snapshot.Head)
 	TestCmd    []string      // the consent-hashed base test command, e.g. [go test ./...]; never the agent's
 	Timeout    time.Duration // per test run; <=0 uses defaultVerifyTimeout
+	// Convention is the language seam (spec §4.2): which files are tests, how to run one and read the
+	// result, and the trivial-pin pre-screen. Nil defaults to Go — the FSM path always sets it
+	// explicitly, so nil is only ever the in-package/test default, never a production fall-through.
+	Convention testconv.Convention
 	// Git runs a hardened git in Dir and returns stdout, the exit code, and a non-nil error only
 	// when the process could not be run at all. nil uses a real shell-out. A seam for tests.
 	Git func(ctx context.Context, args ...string) (stdout string, code int, err error)
@@ -134,9 +139,16 @@ type partition struct {
 	delFiles  []string // non-test files the fix deletes — removed in step (d)
 }
 
-// isTestFile is the test-vs-production predicate (spec §9.1): the Go `*_test.go` convention, the same
-// one packageHasTests keys on. A non-Go partition would key on its own test convention here.
-func isTestFile(path string) bool { return strings.HasSuffix(path, "_test.go") }
+// convOrGo returns c, or the Go convention when c is nil. A nil Convention is the in-package/test
+// default only; the FSM path always sets one explicitly (and fails closed on an unknown name upstream).
+func convOrGo(c testconv.Convention) testconv.Convention {
+	if c == nil {
+		c, _ = testconv.For("go")
+	}
+	return c
+}
+
+func (r Reproducer) conv() testconv.Convention { return convOrGo(r.Convention) }
 
 // changedPartition lists the fix's changed files (PreFixSHA→PostFixSHA) split by role. Rename
 // detection is disabled so a rename is a delete plus an add, which materialize correctly.
@@ -161,7 +173,7 @@ func (r Reproducer) changedPartition(ctx context.Context) (partition, error) {
 			continue
 		}
 		status, path := fields[0][0], fields[1]
-		test := isTestFile(path)
+		test := r.conv().IsTestFile(path)
 		switch status {
 		case 'D':
 			if test {
@@ -232,20 +244,19 @@ func (r Reproducer) reproduceOne(ctx context.Context, p Proof, part partition) R
 	}
 
 	// (c) fail-before: the target test must FAIL with an assertion, not a compile/import, error.
-	code, before, err := r.runTest(ctx, wt, p.Test)
+	rep, before, err := r.runTest(ctx, wt, p.Test)
 	if err != nil {
 		return fail(PinUnverifiable, "the pre-fix test run failed to execute: %v", err)
 	}
-	switch classify(p.Test, code, before) {
-	case clsPassed:
+	switch testconv.Classify(rep, p.Test) {
+	case testconv.ClsPassed:
 		return fail(PinSurvived, "the test passes on the pre-fix tree, so it does not exercise the fault: %s", tail(before))
-	case clsNoTest:
+	case testconv.ClsNoTest:
 		return fail(PinMalformed, "the target test %q was not found in the pre-fix tree", p.Test)
-	case clsCompile:
+	case testconv.ClsCompile:
 		return fail(PinMalformed, "the pre-fix failure is a compile/import error, not an assertion, so it proves nothing: %s", tail(before))
-	case clsOther:
-		return fail(PinMalformed, "the pre-fix failure was not a recognizable test assertion: %s", tail(before))
 	}
+	// testconv.ClsAssert: a valid assertion fail-before — continue to (d).
 
 	// (d) apply the full fix: bring every changed production file to its post-fix content and remove
 	//     what the fix deletes. Test files are already overlaid, so the tree now equals PostFixSHA.
@@ -261,23 +272,24 @@ func (r Reproducer) reproduceOne(ctx context.Context, p Proof, part partition) R
 	}
 
 	// (e) pass-after: the target test must now PASS.
-	code, after, err := r.runTest(ctx, wt, p.Test)
+	rep, after, err := r.runTest(ctx, wt, p.Test)
 	if err != nil {
 		return fail(PinUnverifiable, "the post-fix test run failed to execute: %v", err)
 	}
-	switch classify(p.Test, code, after) {
-	case clsPassed:
+	switch testconv.Classify(rep, p.Test) {
+	case testconv.ClsPassed:
 		return ReproResult{Proof: p, Proven: true, Outcome: PinProven,
 			Detail: fmt.Sprintf("%q fails on the pre-fix tree (assertion) and passes once the fix is applied", p.Test),
 			// Store a COPIED tail, not the whole output: a test that logs a lot before its assertion
 			// would otherwise keep the full buffer alive through post-fix execution and symptom review,
 			// and several proofs could exhaust memory. strings.Clone frees the original backing array.
 			FailBefore: tailClone(before, maxFailBeforeBytes)}
-	case clsNoTest:
+	case testconv.ClsNoTest:
 		return fail(PinUnverifiable, "the target test %q vanished from the post-fix tree", p.Test)
-	case clsCompile:
+	case testconv.ClsCompile:
 		return fail(PinUnverifiable, "the post-fix tree did not build, so nothing can be concluded: %s", tail(after))
 	default:
+		// testconv.ClsAssert: the test still fails an assertion, so the differential did not hold.
 		return fail(PinSurvived, "applying the fix did not make the test pass; the differential did not hold: %s", tail(after))
 	}
 }
@@ -374,16 +386,30 @@ func removeInTree(wt, path string) error {
 	return nil
 }
 
-// runTest runs the consent-hashed test command narrowed to the single target test, verbosely. The
-// test name is regexp-quoted and anchored so -run selects exactly that test and cannot inject a flag
-// (it is one argv element, never a shell string); -v makes the target's own `=== RUN`/`--- PASS`/
-// `--- FAIL` markers appear, so classify keys on the target rather than inferring from suite noise.
-func (r Reproducer) runTest(ctx context.Context, dir, test string) (int, string, error) {
-	argv := append(append([]string(nil), r.TestCmd...), "-run", "^"+regexp.QuoteMeta(test)+"$", "-v")
+// runTest runs the consent-hashed test command narrowed to the single target test (the convention
+// builds the argv, so the test name cannot inject a flag and the runner emits its structured report),
+// then normalizes the run to a testconv.TestReport for classification. It returns the raw combined
+// output too — reproduceOne uses it for the human-readable Detail tail and the §9.2 FailBefore. A run
+// whose output the convention cannot read is an error (the caller reports unverifiable), never an
+// empty report scored as a clean run.
+func (r Reproducer) runTest(ctx context.Context, dir, test string) (testconv.TestReport, string, error) {
+	argv := r.conv().RunArgs(r.TestCmd, test)
+	var code int
+	var out string
+	var err error
 	if r.Run != nil {
-		return r.Run(ctx, dir, argv)
+		code, out, err = r.Run(ctx, dir, argv)
+	} else {
+		code, out, err = runProc(ctx, dir, argv, r.Timeout)
 	}
-	return runProc(ctx, dir, argv, r.Timeout)
+	if err != nil {
+		return testconv.TestReport{}, out, err
+	}
+	rep, perr := r.conv().ParseReport(code, out, "")
+	if perr != nil {
+		return testconv.TestReport{}, out, perr
+	}
+	return rep, out, nil
 }
 
 func (r Reproducer) mkWork() (string, error) {
@@ -432,50 +458,6 @@ func asExitErr(err error, target **exec.ExitError) bool {
 	return ok
 }
 
-// failClass is how a test run's exit code and output classify against the assertion-vs-compile axis.
-type failClass int
-
-const (
-	clsAssert  failClass = iota // exited non-zero with a real "--- FAIL:" assertion — a valid fail-before
-	clsPassed                   // exited zero with tests actually run
-	clsNoTest                   // exited zero but the -run filter matched nothing — the test is absent
-	clsCompile                  // a compile/import/setup failure — NOT an assertion, so it proves nothing
-	clsOther                    // exited non-zero but no assertion marker — fail closed on the assertion axis
-)
-
-// classify reads a verbose `go test -run ^<test>$ -v` run, keyed on the TARGET test's own markers.
-// Because TestCmd is the repo-wide command (e.g. `go test ./...`), sibling packages print
-// "[no test files]" and "no tests to run" for reasons unrelated to the target — so inferring the
-// target's fate from those strings misfires in any multi-package repo. Instead:
-//   - `=== RUN   <test>` proves the target actually ran;
-//   - the compile check comes BEFORE the assertion check, so a build-failed run that still prints a
-//     `--- FAIL:` from another package is compile, not a valid fail-before (the hole spec §9.3(b) closes);
-//   - `--- FAIL: <test>` proves the target itself failed an assertion.
-func classify(test string, code int, out string) failClass {
-	ran := strings.Contains(out, "=== RUN   "+test)
-	if code == 0 {
-		if ran {
-			return clsPassed
-		}
-		return clsNoTest // exit 0 but the target never ran → it is absent from the tree
-	}
-	if isCompileError(out) {
-		return clsCompile
-	}
-	if ran && strings.Contains(out, "--- FAIL: "+test) {
-		return clsAssert
-	}
-	return clsOther
-}
-
-// isCompileError reports a `go test` build/setup/vet failure — the failure modes that make a
-// non-zero exit mean "the code did not compile", never "an assertion failed".
-func isCompileError(out string) bool {
-	return strings.Contains(out, "[build failed]") ||
-		strings.Contains(out, "[setup failed]") ||
-		strings.Contains(out, "[vet failed]")
-}
-
 // runProc runs argv in dir with a timeout, returning the exit code and combined output. A run killed
 // by the deadline or a signal is an error, never a non-zero exit that a caller could read as a
 // legitimate test failure (the same trap Verifier.exec documents).
diff --git a/internal/mutation/reproduction_test.go b/internal/mutation/reproduction_test.go
index f615d0e..aea6c21 100644
--- a/internal/mutation/reproduction_test.go
+++ b/internal/mutation/reproduction_test.go
@@ -2,6 +2,7 @@ package mutation
 
 import (
 	"context"
+	"encoding/json"
 	"errors"
 	"os"
 	"path/filepath"
@@ -17,6 +18,35 @@ type runResp struct {
 	err  error
 }
 
+// go test -json event-stream builders. reproduceOne runs a mock's output through
+// GoConvention.ParseReport, so mocks emit test2json events, not console text.
+func jRun(test string) string { return `{"Action":"run","Package":"pkg","Test":"` + test + `"}` + "\n" }
+func jTerm(action, test string) string {
+	return `{"Action":"` + action + `","Package":"pkg","Test":"` + test + `"}` + "\n"
+}
+func jOut(test, s string) string {
+	b, _ := json.Marshal(s)
+	return `{"Action":"output","Package":"pkg","Test":"` + test + `","Output":` + string(b) + `}` + "\n"
+}
+
+// jFailBefore is a target that ran and failed an assertion (a valid fail-before); its output carries a
+// recognizable "assertion failed" line for the FailBefore checks. jPassAfter is a target that passed.
+func jFailBefore(test string) string {
+	return jRun(test) + jOut(test, "    x_test.go:1: assertion failed\n") + jTerm("fail", test)
+}
+func jPassAfter(test string) string { return jRun(test) + jTerm("pass", test) }
+
+// jBuildFail is a package that failed to build (no per-test events). jNoTests is a run in which only a
+// sibling package appeared and the target never ran.
+func jBuildFail() string {
+	return `{"Action":"output","Package":"pkg","Output":"# pkg [pkg.test]\n"}` + "\n" +
+		`{"Action":"output","Package":"pkg","Output":"FAIL\tpkg [build failed]\n"}` + "\n" +
+		`{"Action":"fail","Package":"pkg"}` + "\n"
+}
+func jNoTests() string {
+	return `{"Action":"output","Package":"sib","Output":"testing: warning: no tests to run\n"}` + "\n"
+}
+
 // fakeGit answers the exact git verbs the engine issues. Every response is a field so a test dials
 // in one edge without a real repository.
 type fakeGit struct {
@@ -150,25 +180,25 @@ func TestReproduceProvenPath(t *testing.T) {
 		calls++
 		if calls == 1 {
 			argvAtFailBefore = argv
-			return 1, "=== RUN   TestX\n--- FAIL: TestX (0.00s)\nFAIL\tpkg\t0.1s\n", nil // fail-before: assertion
+			return 1, jFailBefore("TestX"), nil // fail-before: assertion
 		}
 		_, err := os.Stat(filepath.Join(dir, "pkg", "prod.go"))
 		prodPresent = err == nil
 		prodAtPassAfter = argv
-		return 0, "=== RUN   TestX\n--- PASS: TestX (0.00s)\nok  \tpkg\t0.1s\n", nil // pass-after
+		return 0, jPassAfter("TestX"), nil // pass-after
 	}
 	r := newReproducer(t, g, run)
 	res := reproduceOne(t, r, "TestX")
 	mustOutcome(t, res, PinProven)
 	// A Proven result must carry the pre-fix assertion output for the §9.2 symptom reviewer.
-	if !strings.Contains(res.FailBefore, "--- FAIL: TestX") {
+	if !strings.Contains(res.FailBefore, "assertion failed") {
 		t.Fatalf("Proven result must carry the fail-before output: %q", res.FailBefore)
 	}
 
-	// The pre-fix run must be narrowed to the target test with an anchored -run, verbosely (-v so the
-	// target's own markers appear).
-	if !hasFlagValue(argvAtFailBefore, "-run", "^TestX$") || argvAtFailBefore[len(argvAtFailBefore)-1] != "-v" {
-		t.Fatalf("target test must be selected with an anchored -run and -v: %v", argvAtFailBefore)
+	// The pre-fix run must be narrowed to the target test with an anchored -run, and request the
+	// structured report (-json) that classification reads.
+	if !hasFlagValue(argvAtFailBefore, "-run", "^TestX$") || argvAtFailBefore[len(argvAtFailBefore)-1] != "-json" {
+		t.Fatalf("target test must be selected with an anchored -run and -json: %v", argvAtFailBefore)
 	}
 	if !prodPresent {
 		t.Fatalf("the fix's production file must be applied before the pass-after run (argv %v)", prodAtPassAfter)
@@ -184,15 +214,17 @@ func TestReproduceProvenPath(t *testing.T) {
 // A Proven result stores only a bounded, copied tail of the pre-fix output — a test that logs a lot
 // before its assertion must not keep the whole buffer alive for the §9.2 reviewer.
 func TestReproduceCapsFailBefore(t *testing.T) {
-	huge := "=== RUN   T\n" + strings.Repeat("x", maxFailBeforeBytes+5000) + "\n--- FAIL: T\n"
+	// A huge output event, then a late "assertion failed" line and the terminal fail — so the assertion
+	// lives in the tail the cap keeps.
+	huge := jRun("T") + jOut("T", strings.Repeat("x", maxFailBeforeBytes+5000)) + jOut("T", "assertion failed\n") + jTerm("fail", "T")
 	g := &fakeGit{partition: "A\tpkg/new_test.go\n", showBody: map[string]string{"pkg/new_test.go": "t"}}
-	run := &seqRunner{resp: []runResp{{code: 1, out: huge}, {code: 0, out: "=== RUN   T\n--- PASS: T\nok\n"}}}
+	run := &seqRunner{resp: []runResp{{code: 1, out: huge}, {code: 0, out: jPassAfter("T")}}}
 	res := reproduceOne(t, newReproducer(t, g, run.run), "T")
 	mustOutcome(t, res, PinProven)
 	if len(res.FailBefore) > maxFailBeforeBytes {
 		t.Fatalf("FailBefore must be capped to its tail: %d > %d", len(res.FailBefore), maxFailBeforeBytes)
 	}
-	if !strings.Contains(res.FailBefore, "--- FAIL: T") {
+	if !strings.Contains(res.FailBefore, "assertion failed") {
 		t.Fatal("the cap must keep the tail (where the assertion is)")
 	}
 }
@@ -209,11 +241,11 @@ func TestReproduceAppliesDeletion(t *testing.T) {
 	run := func(_ context.Context, dir string, _ []string) (int, string, error) {
 		calls++
 		if calls == 1 {
-			return 1, "=== RUN   TestX\n--- FAIL: TestX\n", nil
+			return 1, jFailBefore("TestX"), nil
 		}
 		_, err := os.Stat(filepath.Join(dir, "pkg", "gone.go"))
 		goneAbsent = os.IsNotExist(err)
-		return 0, "=== RUN   TestX\n--- PASS: TestX\nok\n", nil
+		return 0, jPassAfter("TestX"), nil
 	}
 	r := newReproducer(t, g, run)
 	// Pre-create the file the fix deletes so step (d) has something to remove.
@@ -234,7 +266,7 @@ func TestReproduceAppliesDeletion(t *testing.T) {
 // A deletion whose target is a non-empty directory cannot be removed: unverifiable, not a crash.
 func TestReproduceDeletionRemoveError(t *testing.T) {
 	g := &fakeGit{partition: "A\tpkg/new_test.go\nD\tpkg/busy\n", showBody: map[string]string{"pkg/new_test.go": "t"}}
-	run := &seqRunner{resp: []runResp{{code: 1, out: "=== RUN   TestX\n--- FAIL: TestX\n"}}}
+	run := &seqRunner{resp: []runResp{{code: 1, out: jFailBefore("TestX")}}}
 	r := newReproducer(t, g, run.run)
 	wtParent := t.TempDir()
 	r.MkWork = func() (string, error) { return wtParent, nil }
@@ -278,8 +310,8 @@ func TestReproducePrunesWhenRemoveFails(t *testing.T) {
 		return "", 0, nil
 	}
 	run := &seqRunner{resp: []runResp{
-		{code: 1, out: "=== RUN   TestX\n--- FAIL: TestX\n"},
-		{code: 0, out: "=== RUN   TestX\n--- PASS: TestX\nok\n"},
+		{code: 1, out: jFailBefore("TestX")},
+		{code: 0, out: jPassAfter("TestX")},
 	}}
 	r := Reproducer{Dir: t.TempDir(), PreFixSHA: "pre", PostFixSHA: "post", TestCmd: []string{"go", "test", "./..."},
 		Git: git, Run: run.run, MkWork: func() (string, error) { return parent, nil }}
@@ -301,10 +333,10 @@ func TestReproduceFailBeforeClassification(t *testing.T) {
 		want Outcome
 		hint string
 	}{
-		{"pass-before is a test gap", runResp{code: 0, out: "=== RUN   TestX\n--- PASS: TestX\nok\tpkg\t0.1s\n"}, PinSurvived, "does not exercise the fault"},
-		{"filter matched nothing", runResp{code: 0, out: "testing: warning: no tests to run\nPASS\n"}, PinMalformed, "was not found"},
-		{"compile error is not an assertion", runResp{code: 1, out: "# pkg\n./x_test.go:1:1: undefined: Z\nFAIL\tpkg [build failed]\n"}, PinMalformed, "compile/import error"},
-		{"non-assertion failure fails closed", runResp{code: 1, out: "some unexpected failure\n"}, PinMalformed, "not a recognizable test assertion"},
+		{"pass-before is a test gap", runResp{code: 0, out: jPassAfter("TestX")}, PinSurvived, "does not exercise the fault"},
+		{"filter matched nothing", runResp{code: 0, out: jNoTests()}, PinMalformed, "was not found"},
+		{"compile error is not an assertion", runResp{code: 1, out: jBuildFail()}, PinMalformed, "compile/import error"},
+		{"unreadable failing output is unverifiable", runResp{code: 1, out: "some unexpected failure\n"}, PinUnverifiable, "failed to execute"},
 	} {
 		t.Run(tc.name, func(t *testing.T) {
 			g := &fakeGit{partition: "A\tpkg/new_test.go\n", showBody: map[string]string{"pkg/new_test.go": "t"}}
@@ -327,13 +359,13 @@ func TestReproducePassAfterClassification(t *testing.T) {
 		want  Outcome
 		hint  string
 	}{
-		{"still failing means the fix did not hold", runResp{code: 1, out: "=== RUN   TestX\n--- FAIL: TestX\nFAIL\n"}, PinSurvived, "did not make the test pass"},
-		{"vanished test is unverifiable", runResp{code: 0, out: "no tests to run\n"}, PinUnverifiable, "vanished"},
-		{"post-fix build failure is unverifiable", runResp{code: 1, out: "FAIL\tpkg [build failed]\n"}, PinUnverifiable, "did not build"},
+		{"still failing means the fix did not hold", runResp{code: 1, out: jFailBefore("TestX")}, PinSurvived, "did not make the test pass"},
+		{"vanished test is unverifiable", runResp{code: 0, out: jNoTests()}, PinUnverifiable, "vanished"},
+		{"post-fix build failure is unverifiable", runResp{code: 1, out: jBuildFail()}, PinUnverifiable, "did not build"},
 	} {
 		t.Run(tc.name, func(t *testing.T) {
 			g := &fakeGit{partition: "A\tpkg/new_test.go\n", showBody: map[string]string{"pkg/new_test.go": "t"}}
-			run := &seqRunner{resp: []runResp{{code: 1, out: "=== RUN   TestX\n--- FAIL: TestX\n"}, tc.after}}
+			run := &seqRunner{resp: []runResp{{code: 1, out: jFailBefore("TestX")}, tc.after}}
 			got := reproduceOne(t, newReproducer(t, g, run.run), "TestX")
 			mustOutcome(t, got, tc.want)
 			if !strings.Contains(got.Detail, tc.hint) {
@@ -461,7 +493,7 @@ func TestReproduceFailBeforeRunError(t *testing.T) {
 
 func TestReproducePassAfterRunError(t *testing.T) {
 	g := &fakeGit{partition: "A\tpkg/new_test.go\n", showBody: map[string]string{"pkg/new_test.go": "t"}}
-	run := &seqRunner{resp: []runResp{{code: 1, out: "=== RUN   T\n--- FAIL: T\n"}, {err: errors.New("exec failed")}}}
+	run := &seqRunner{resp: []runResp{{code: 1, out: jFailBefore("T")}, {err: errors.New("exec failed")}}}
 	got := reproduceOne(t, newReproducer(t, g, run.run), "T")
 	mustOutcome(t, got, PinUnverifiable)
 	if !strings.Contains(got.Detail, "post-fix test run failed to execute") {
@@ -478,7 +510,7 @@ func TestReproduceProdOverlayFailsClosed(t *testing.T) {
 		showErrOnce: map[string]bool{"pkg/prod.go": true},
 		showErr:     errors.New("boom"),
 	}
-	run := &seqRunner{resp: []runResp{{code: 1, out: "=== RUN   T\n--- FAIL: T\n"}}}
+	run := &seqRunner{resp: []runResp{{code: 1, out: jFailBefore("T")}}}
 	got := reproduceOne(t, newReproducer(t, g, run.run), "T")
 	mustOutcome(t, got, PinUnverifiable)
 	if !strings.Contains(got.Detail, "could not apply the fix") {
@@ -486,44 +518,6 @@ func TestReproduceProdOverlayFailsClosed(t *testing.T) {
 	}
 }
 
-func TestClassifyAndPredicates(t *testing.T) {
-	if !isTestFile("a/b_test.go") || isTestFile("a/b.go") {
-		t.Fatal("isTestFile must key on the _test.go suffix")
-	}
-	const run = "=== RUN   TestX\n"
-	// Keyed on the target: exit 0 WITH the target's RUN marker is passed; without it the target never
-	// ran, so it is absent (clsNoTest) — even amid sibling-package "no test files"/"no tests to run".
-	if classify("TestX", 0, run+"--- PASS: TestX\nok\n") != clsPassed {
-		t.Fatal("zero exit with the target's RUN marker is passed")
-	}
-	if classify("TestX", 0, "?   sibling [no test files]\nok  other\ttesting: warning: no tests to run\n") != clsNoTest {
-		t.Fatal("zero exit without the target's RUN marker is clsNoTest, ignoring sibling-package noise")
-	}
-	// A build/setup/vet failure is compile — before the assertion check.
-	if classify("TestX", 1, "FAIL [build failed]") != clsCompile || classify("TestX", 1, "[setup failed]") != clsCompile || classify("TestX", 1, "[vet failed]") != clsCompile {
-		t.Fatal("a build/setup/vet failure is clsCompile")
-	}
-	// Compile beats assertion: a build-failed run that also prints the target's --- FAIL is still compile.
-	if classify("TestX", 1, run+"--- FAIL: TestX\nFAIL\tpkg [build failed]\n") != clsCompile {
-		t.Fatal("a compile failure must win over an assertion marker")
-	}
-	if classify("TestX", 1, run+"--- FAIL: TestX\n") != clsAssert {
-		t.Fatal("the target's assertion failure is clsAssert")
-	}
-	// A non-zero exit whose target neither ran nor failed (e.g. a sibling package failed) is clsOther.
-	if classify("TestX", 1, "mystery\n") != clsOther {
-		t.Fatal("a nonzero exit with no target marker is clsOther")
-	}
-	// Ran but no target FAIL line (an odd non-assertion failure) is also clsOther, not a valid assertion.
-	if classify("TestX", 1, run+"panic: boom\n") != clsOther {
-		t.Fatal("ran but no target --- FAIL line is clsOther")
-	}
-	// A SIBLING test's failure is not the target's assertion: the FAIL check is keyed on the target.
-	if classify("TestX", 1, run+"--- FAIL: TestOther (0.00s)\n") != clsOther {
-		t.Fatal("a non-target --- FAIL line must not count as the target's assertion")
-	}
-}
-
 // ScopeSuite runs the full test command (no -run) on both trees: green→green passes, green→red blocks,
 // a red or unrunnable baseline is unverifiable.
 func TestScopeSuite(t *testing.T) {
diff --git a/internal/mutation/verify.go b/internal/mutation/verify.go
index 30bfde9..33f5206 100644
--- a/internal/mutation/verify.go
+++ b/internal/mutation/verify.go
@@ -11,6 +11,7 @@ import (
 	"time"
 
 	"github.com/dsifry/metareview/internal/findings"
+	"github.com/dsifry/metareview/internal/testconv"
 )
 
 // Pin is a claim that one test holds one line of production code: replacing From with To at File
@@ -75,10 +76,15 @@ type Verifier struct {
 	// questions, and a language whose build check is not a test invocation needs to say so.
 	BuildCmd []string
 	Timeout  time.Duration
+	// Convention is the language seam (spec §4.2); here it supplies the trivial-pin pre-screen. Nil
+	// defaults to Go — the FSM path always sets it explicitly.
+	Convention testconv.Convention
 	// Now and Run are seams for tests; nil means the real thing.
 	Run func(ctx context.Context, dir string, argv []string) (int, string, error)
 }
 
+func (v Verifier) conv() testconv.Convention { return convOrGo(v.Convention) }
+
 const defaultVerifyTimeout = 10 * time.Minute
 
 // Verify checks every pin and returns one result each, in order. A pin that cannot be checked is
@@ -149,7 +155,7 @@ func (v Verifier) verifyOne(ctx context.Context, p Pin) PinResult {
 	//     triviality is out of scope: no pure syntactic method detects reachability, and the spec states
 	//     no reachability contract; the compile-then-break steps handle everything the token check does
 	//     not.)
-	if semanticallyNull(body, mutated) {
+	if v.conv().SemanticallyNull(body, mutated) {
 		return fail(PinMalformed, "the mutation %q -> %q changes only comments or whitespace; it is semantically null, so no test could catch it — rewrite the pin to mutate real behaviour", p.From, p.To)
 	}
 
diff --git a/internal/mutation/verify_test.go b/internal/mutation/verify_test.go
index 648dfc2..901de24 100644
--- a/internal/mutation/verify_test.go
+++ b/internal/mutation/verify_test.go
@@ -531,65 +531,3 @@ func TestVerifyRejectsATrivialWhitespacePin(t *testing.T) {
 		t.Fatalf("a whitespace-only pin must be malformed (never survived): %+v", res[0])
 	}
 }
-
-func TestSemanticallyNull(t *testing.T) {
-	// comment-only and whitespace-only changes are null.
-	if !semanticallyNull("package p\n// one\nvar x = 1\n", "package p\n// two\nvar x = 1\n") {
-		t.Fatal("a comment-only change must be null")
-	}
-	if !semanticallyNull("package p\nvar x = 1\n", "package p\nvar   x =  1\n") {
-		t.Fatal("a whitespace-only change must be null")
-	}
-	// a value change is NOT null (same token count, differing literal).
-	if semanticallyNull("package p\nvar x = 1\n", "package p\nvar x = 2\n") {
-		t.Fatal("a value change must not be null")
-	}
-	// a token-count change is NOT null.
-	if semanticallyNull("package p\nvar x = 1\n", "package p\nvar x = 1 + y\n") {
-		t.Fatal("added tokens must not be null")
-	}
-	// an unscannable mutation (unterminated string) is NOT treated as null — the compile step handles it.
-	if semanticallyNull("package p\nvar x = 1\n", "package p\nvar x = \"unterminated\n") {
-		t.Fatal("an unscannable mutation must not be null")
-	}
-	// an unscannable ORIGINAL is likewise not null.
-	if semanticallyNull("package p\nvar x = `unterminated", "package p\nvar x = 1\n") {
-		t.Fatal("an unscannable original must not be null")
-	}
-	// A Go DIRECTIVE comment is semantically meaningful — a change to one is NOT null.
-	if semanticallyNull("//go:build linux\npackage p\n", "//go:build windows\npackage p\n") {
-		t.Fatal("a //go:build directive change must NOT be null")
-	}
-	if semanticallyNull("package p\n//go:noinline\nfunc f() {}\n", "package p\n//go:noescape\nfunc f() {}\n") {
-		t.Fatal("a //go: directive change must NOT be null")
-	}
-	// Adding/removing a directive is not null.
-	if semanticallyNull("package p\nfunc f() {}\n", "package p\n//go:noinline\nfunc f() {}\n") {
-		t.Fatal("adding a directive must NOT be null")
-	}
-	// An ordinary comment that merely CONTAINS a colon is not a directive → still null.
-	if !semanticallyNull("package p\n// note: one\nvar x = 1\n", "package p\n// note: two\nvar x = 1\n") {
-		t.Fatal("an ordinary comment with a colon is still null")
-	}
-	// A block comment is never a directive → still null.
-	if !semanticallyNull("package p\n/* one */\nvar x = 1\n", "package p\n/* two */\nvar x = 1\n") {
-		t.Fatal("a block comment change is still null")
-	}
-	// cgo //export, gccgo //extern, and the legacy // +build constraint are meaningful → NOT null.
-	if semanticallyNull("package p\n//export One\nfunc f() {}\n", "package p\n//export Two\nfunc f() {}\n") {
-		t.Fatal("a //export change must NOT be null")
-	}
-	if semanticallyNull("package p\n//extern one\nfunc f()\n", "package p\n//extern two\nfunc f()\n") {
-		t.Fatal("a //extern change must NOT be null")
-	}
-	if semanticallyNull("// +build linux\npackage p\n", "// +build windows\npackage p\n") {
-		t.Fatal("a legacy // +build change must NOT be null")
-	}
-	// The Go toolchain tolerates extra whitespace (or a tab) between // and +build.
-	if semanticallyNull("//   +build linux\npackage p\n", "//   +build windows\npackage p\n") {
-		t.Fatal("a // +build with extra spaces must NOT be null")
-	}
-	if semanticallyNull("//\t+build linux\npackage p\n", "//\t+build windows\npackage p\n") {
-		t.Fatal("a // +build after a tab must NOT be null")
-	}
-}
diff --git a/internal/testconv/goconv.go b/internal/testconv/goconv.go
new file mode 100644
index 0000000..cfe205d
--- /dev/null
+++ b/internal/testconv/goconv.go
@@ -0,0 +1,215 @@
+package testconv
+
+import (
+	"encoding/json"
+	"fmt"
+	"go/scanner"
+	"go/token"
+	"path/filepath"
+	"regexp"
+	"strings"
+)
+
+func init() { register(goConvention{}) }
+
+// goConvention is the reference Convention: Go's `*_test.go` files, `go test -json` for structured
+// results, the `testing` package's test-name rule for deletion, and a `go/scanner` token walk for the
+// trivial-pin pre-screen. It carries no state.
+type goConvention struct{}
+
+func (goConvention) Name() string { return "go" }
+
+// IsTestFile is Go's `*_test.go` convention (spec §9.1) — the same one DirHasTests globs on.
+func (goConvention) IsTestFile(path string) bool { return strings.HasSuffix(path, "_test.go") }
+
+// RunArgs narrows the base command to exactly one test and asks for the JSON event stream. The name is
+// regexp-quoted and anchored so -run selects that one test and cannot inject a flag (argv, never a
+// shell string). -json subsumes -v: test2json emits per-test run/pass/fail events without it.
+func (goConvention) RunArgs(base []string, test string) []string {
+	return append(append([]string(nil), base...), "-run", "^"+regexp.QuoteMeta(test)+"$", "-json")
+}
+
+// SuiteArgs runs the whole suite (no -run filter) with the JSON event stream.
+func (goConvention) SuiteArgs(base []string) []string {
+	return append(append([]string(nil), base...), "-json")
+}
+
+// goEvent is the subset of a `go test -json` (test2json) event this reads: the per-test pass/fail
+// actions carry Test; package-level output (Test == "") carries the build/setup/vet failure text.
+type goEvent struct {
+	Action  string `json:"Action"`
+	Test    string `json:"Test"`
+	Output  string `json:"Output"`
+	Package string `json:"Package"`
+}
+
+// ParseReport normalizes a `go test -json` run. Each line is one event; a per-test pass/fail action
+// records the test's Outcome (keyed by the target's own name, so a sibling package's noise is a
+// different Test/Package and simply never recorded — the multi-package hole the console-scrape had is
+// gone structurally). Package-level output plus any non-JSON line (an early compiler error test2json
+// did not wrap) is scanned for a build/setup/vet-failure marker, which sets BuildFailed. A failed run
+// that emitted no JSON events at all and no recognizable build marker is unreadable — an error, so the
+// caller fails closed rather than treat a garbage run as a clean (empty) report.
+func (goConvention) ParseReport(code int, stdout, stderr string) (TestReport, error) {
+	rep := TestReport{Tests: map[string]Outcome{}}
+	var buildText strings.Builder
+	sawEvent := false
+	for _, raw := range strings.Split(stdout, "\n") {
+		line := strings.TrimSpace(raw)
+		if line == "" {
+			continue
+		}
+		var ev goEvent
+		if err := json.Unmarshal([]byte(line), &ev); err != nil {
+			// Not a test2json line — an early compiler error printed before the stream. Keep it for the
+			// build-marker scan; it is not a test outcome.
+			buildText.WriteString(line + "\n")
+			continue
+		}
+		sawEvent = true
+		switch ev.Action {
+		case "pass":
+			if ev.Test != "" {
+				rep.Tests[ev.Test] = Passed
+			}
+		case "fail":
+			if ev.Test != "" {
+				rep.Tests[ev.Test] = Failed
+			}
+		case "output":
+			if ev.Test == "" { // package-level output — where "FAIL pkg [build failed]" lands
+				buildText.WriteString(ev.Output)
+			}
+		}
+	}
+	if isBuildFailure(buildText.String()) {
+		rep.BuildFailed = true
+	}
+	// A nonzero exit with neither a JSON stream nor a build marker is unreadable: something ran that was
+	// not `go test -json`, or it died before emitting anything. Refuse rather than report a clean run.
+	if !sawEvent && !rep.BuildFailed && code != 0 {
+		return TestReport{}, fmt.Errorf("testconv(go): the test run produced no go-test-json events and no build-failure marker (exit %d)", code)
+	}
+	return rep, nil
+}
+
+// isBuildFailure reports a `go test` build/setup/vet failure — the modes that make a non-zero exit mean
+// "the code did not build", never "an assertion failed".
+func isBuildFailure(out string) bool {
+	return strings.Contains(out, "[build failed]") ||
+		strings.Contains(out, "[setup failed]") ||
+		strings.Contains(out, "[vet failed]")
+}
+
+// testFuncRemovedRe matches a removed Go test declaration, following the testing package's naming rule:
+// TestXxx/BenchmarkXxx/FuzzXxx/ExampleXxx where Xxx is EMPTY or starts with a non-lowercase rune (so
+// `Test`/`TestFoo`/`Test_x`/`TestÜ` match but `Testhelper` does not), with optional whitespace before
+// the parameter list. Suffix runes are Unicode identifier characters, not ASCII-only.
+var testFuncRemovedRe = regexp.MustCompile(`^-func (Test|Benchmark|Fuzz|Example)([^\p{Ll}\s(][\p{L}\p{N}_]*)?\s*\(`)
+
+// DeletesATest reports whether the diff removes a Go test function — a removed `func Test.../Benchmark
+// .../Fuzz.../Example...` line, covering both a deleted *_test.go file (its func lines appear removed)
+// and a test removed from a surviving file. This is the `testing` package's own name rule, matched
+// exactly on the diff's removed lines — a language spec, not a hand-rolled parser. Header lines are
+// skipped.
+func (goConvention) DeletesATest(diff string) bool {
+	for _, line := range strings.Split(diff, "\n") {
+		if strings.HasPrefix(line, "---") {
+			continue
+		}
+		if testFuncRemovedRe.MatchString(line) {
+			return true
+		}
+	}
+	return false
+}
+
+// DirHasTests reports whether the directory holding file (relative to root) contains Go test files.
+func (goConvention) DirHasTests(root, file string) bool {
+	matches, err := filepath.Glob(filepath.Join(root, filepath.Dir(file), "*_test.go"))
+	return err == nil && len(matches) > 0
+}
+
+// SemanticallyNull reports whether orig and mutated are the SAME Go program ignoring comments and
+// whitespace — a no-op mutation the compiler cannot tell apart (spec §9.8 R7). It compares the two
+// token streams (comments dropped by the scanner, whitespace irrelevant between tokens): position-
+// independent and pure. If EITHER body fails to scan, it returns false (a mutation that breaks scanning
+// is the compile step's job, not this pre-screen's). No dead-code/reachability detection is attempted.
+func (goConvention) SemanticallyNull(orig, mutated string) bool {
+	a, ok := goTokens(orig)
+	if !ok {
+		return false
+	}
+	b, ok := goTokens(mutated)
+	if !ok {
+		return false
+	}
+	if len(a) != len(b) {
+		return false
+	}
+	for i := range a {
+		if a[i] != b[i] {
+			return false
+		}
+	}
+	return true
+}
+
+// goTokens scans src into its Go token stream, one "tok lit" string per token (the literal distinguishes
+// identifiers/literals of the same class). Ordinary comments are dropped (semantically null), but a
+// comment DIRECTIVE (//go:build, //go:embed, //line, //export, //extern, // +build) is KEPT: it affects
+// build selection, embedded data, or linkage, so a change to one is NOT null. ok is false if the source
+// did not scan cleanly.
+func goTokens(src string) (toks []string, ok bool) {
+	var fset token.FileSet
+	file := fset.AddFile("", fset.Base(), len(src))
+	var s scanner.Scanner
+	ok = true
+	s.Init(file, []byte(src), func(token.Position, string) { ok = false }, scanner.ScanComments)
+	for {
+		_, tok, lit := s.Scan()
+		if tok == token.EOF {
+			break
+		}
+		if tok == token.COMMENT {
+			if d, isDir := directive(lit); isDir {
+				toks = append(toks, "//dir "+d)
+			}
+			continue
+		}
+		if lit != "" {
+			toks = append(toks, tok.String()+" "+lit)
+		} else {
+			toks = append(toks, tok.String())
+		}
+	}
+	return toks, ok
+}
+
+// directive reports whether a line comment is a meaningful Go comment directive (returning its text
+// without the leading //). It covers go/ast's isDirective form — a "//line " directive, or "//name:arg"
+// with a lowercase/digit name and no space before the colon (e.g. //go:build) — PLUS the comments that
+// change linkage or build selection: cgo "//export Name", gccgo "//extern name", and the legacy
+// "// +build" constraint. An ordinary comment (even one with a colon, or a block comment) is not one.
+func directive(comment string) (string, bool) {
+	if !strings.HasPrefix(comment, "//") {
+		return "", false // block comments are never directives
+	}
+	c := comment[2:]
+	if strings.HasPrefix(c, "line ") || strings.HasPrefix(c, "export ") || strings.HasPrefix(c, "extern ") ||
+		strings.HasPrefix(strings.TrimLeft(c, " \t"), "+build") {
+		return c, true
+	}
+	colon := strings.Index(c, ":")
+	if colon <= 0 || colon+1 >= len(c) {
+		return "", false
+	}
+	for i := 0; i < colon; i++ {
+		b := c[i]
+		lowerOrDigit := b >= 'a' && b <= 'z' || b >= '0' && b <= '9'
+		if !lowerOrDigit {
+			return "", false
+		}
+	}
+	return c, true
+}
diff --git a/internal/testconv/testconv.go b/internal/testconv/testconv.go
new file mode 100644
index 0000000..314fcd0
--- /dev/null
+++ b/internal/testconv/testconv.go
@@ -0,0 +1,115 @@
+// Package testconv is metareview's pluggable test-convention seam (spec §4.2). The differential-proof
+// engine (reproduction, deletion, trivial-pin) has a language-agnostic spine but a handful of leaf
+// decisions that are language-specific: which files are tests, how to run one test and the whole
+// suite, how to read the result, whether a directory has tests, and whether a mutation is a no-op.
+// A Convention answers exactly those, so a repository in any language participates by selecting one.
+//
+// It mirrors the mutation-report idiom (internal/mutation/{parse,stryker}.go): metareview reads a
+// robust EXTERNAL tool's own structured output — here each test runner's machine-readable report
+// (`go test -json`, Jest `--json`, Vitest `--reporter=json`) — and normalizes it. It never parses
+// source and never scrapes console text. As there, an outcome the normaliser does not recognise is
+// never scored as success, and output it cannot read is refused rather than treated as an empty
+// (clean-looking) result.
+package testconv
+
+// Outcome is what became of one test in a run. Only the two decided states exist: a test that did not
+// run leaves no entry in TestReport.Tests at all (its absence is the signal), so there is no third
+// "unknown" value to be mistaken for a pass.
+type Outcome int
+
+const (
+	Passed Outcome = iota // the test ran and passed
+	Failed                // the test ran and failed an assertion
+)
+
+// TestReport is the normalized result of running the suite (or a single narrowed test) — the analogue
+// of mutation.Report, built ONLY from a runner's structured output. Tests maps a test's identity (the
+// runner's own test id: a Go test function name, a Jest `describe > it` string) to its Outcome; a test
+// that did not run is simply absent. BuildFailed records that the target's own package failed to
+// compile/transpile or its setup failed, so no assertion was ever reached — the load-bearing
+// distinction (spec §9.2/§9.3): a "failure" that is really a build error proves nothing about a fault.
+type TestReport struct {
+	Tests       map[string]Outcome
+	BuildFailed bool
+}
+
+// Class is the assertion-vs-compile axis for ONE target test, the five-way result reproduceOne
+// switches on (formerly mutation.failClass). It is DERIVED generically from a TestReport by Classify —
+// no per-language logic — so every runner shares one classification once its report is normalized.
+type Class int
+
+const (
+	ClsAssert  Class = iota // the target ran and failed an assertion — a valid fail-before
+	ClsPassed               // the target ran and passed
+	ClsNoTest               // the target did not run and the build was fine — it is absent from the tree
+	ClsCompile              // the target's package did not build — NOT an assertion, so it proves nothing
+)
+
+// Classify maps a normalized report onto the target test's Class. It is the whole point of normalizing
+// to a report first: the decision is one language-independent function.
+//
+//   - target present & Passed  -> ClsPassed
+//   - target present & Failed  -> ClsAssert  (it genuinely ran and failed; a sibling package's build
+//     failure is irrelevant because the report attributes the failure to THIS test — the precision the
+//     old console-scrape lacked, so it had to conservatively call any build marker a compile error)
+//   - target absent & BuildFailed -> ClsCompile (the target's package did not build, so it could not run)
+//   - target absent & built       -> ClsNoTest  (the run was clean but this test is not in the tree)
+func Classify(r TestReport, test string) Class {
+	if o, ok := r.Tests[test]; ok {
+		if o == Passed {
+			return ClsPassed
+		}
+		return ClsAssert
+	}
+	if r.BuildFailed {
+		return ClsCompile
+	}
+	return ClsNoTest
+}
+
+// Convention is the per-language seam. Implementations are pure and cheap except ParseReport, which
+// only reads bytes a runner already produced. Everything the engine needs that is language-specific
+// lives here; the engine itself is language-agnostic.
+type Convention interface {
+	// Name is the stable identifier a workflow selects it by ("go", "typescript").
+	Name() string
+	// IsTestFile reports whether a repository-relative path is a test file (the reproduction partition
+	// and DirHasTests both key on this).
+	IsTestFile(path string) bool
+	// RunArgs builds the argv that runs EXACTLY the one named test, emitting the runner's structured
+	// report, from the consent-hashed base command. The test id is quoted/anchored by the convention so
+	// it selects one test and cannot inject a flag.
+	RunArgs(base []string, test string) []string
+	// SuiteArgs builds the argv that runs the WHOLE suite (no test filter), emitting the structured
+	// report — used by the over-deletion scope check.
+	SuiteArgs(base []string) []string
+	// ParseReport normalizes one run's (exit code, stdout, stderr) into a TestReport. Output it cannot
+	// read is an error (the caller fails closed), never an empty report — an empty report would score a
+	// broken run as a clean one (the refuse-unreadable rule from the mutation package).
+	ParseReport(code int, stdout, stderr string) (TestReport, error)
+	// DeletesATest reports whether a unified diff removes a test (the §9.6 gate trigger). Each language
+	// uses its most robust available mechanism; it is a cheap diff check, never a runner invocation.
+	DeletesATest(diff string) bool
+	// DirHasTests reports whether the directory holding a repository-relative file contains test files
+	// (the §3.1 owed-pin gate). root is the repository/worktree root.
+	DirHasTests(root, file string) bool
+	// SemanticallyNull reports whether a from->to mutation is a no-op the compiler cannot tell apart —
+	// the §9.8 R7 trivial-pin pre-screen. A convention with no robust tokenizer returns false (the safe
+	// direction: the full compile-then-break steps still run).
+	SemanticallyNull(orig, mutated string) bool
+}
+
+// For returns the convention selected by name. An empty or unknown name returns (nil, false): the
+// caller MUST fail closed (a proof it cannot classify is unverifiable), never fall through to a
+// default language — a non-Go repository silently scored by Go rules would mint false proofs and miss
+// test deletions, which is worse than declining to prove.
+func For(name string) (Convention, bool) {
+	c, ok := registry[name]
+	return c, ok
+}
+
+// registry is the fixed set of built-in conventions. It is populated by each convention's file init;
+// there is no dynamic registration, so the set is auditable at compile time.
+var registry = map[string]Convention{}
+
+func register(c Convention) { registry[c.Name()] = c }
diff --git a/internal/testconv/testconv_test.go b/internal/testconv/testconv_test.go
new file mode 100644
index 0000000..e5f7b6a
--- /dev/null
+++ b/internal/testconv/testconv_test.go
@@ -0,0 +1,213 @@
+package testconv
+
+import (
+	"os"
+	"path/filepath"
+	"testing"
+)
+
+func TestForSelectsAndFailsClosed(t *testing.T) {
+	c, ok := For("go")
+	if !ok || c == nil || c.Name() != "go" {
+		t.Fatalf("For(go) must return the Go convention, got %v ok=%v", c, ok)
+	}
+	// The load-bearing negative: an unknown or empty name returns (nil,false) so the caller fails
+	// closed — never a silent default to Go.
+	if c, ok := For("typescript"); ok || c != nil {
+		t.Fatal("an unregistered convention must return (nil,false)")
+	}
+	if c, ok := For(""); ok || c != nil {
+		t.Fatal("the empty name must return (nil,false)")
+	}
+}
+
+func TestClassify(t *testing.T) {
+	pass := TestReport{Tests: map[string]Outcome{"TestX": Passed}}
+	if Classify(pass, "TestX") != ClsPassed {
+		t.Fatal("a passed target is ClsPassed")
+	}
+	fail := TestReport{Tests: map[string]Outcome{"TestX": Failed}}
+	if Classify(fail, "TestX") != ClsAssert {
+		t.Fatal("a failed target is ClsAssert")
+	}
+	// Precision the console-scrape lacked: the target genuinely failed, so a co-occurring build failure
+	// (a sibling package) is irrelevant — it is still a valid assertion fail-before.
+	if Classify(TestReport{Tests: map[string]Outcome{"TestX": Failed}, BuildFailed: true}, "TestX") != ClsAssert {
+		t.Fatal("a target that itself failed is ClsAssert even when some package build-failed")
+	}
+	// Absent + build failed = the target's package did not build.
+	if Classify(TestReport{Tests: map[string]Outcome{}, BuildFailed: true}, "TestX") != ClsCompile {
+		t.Fatal("an absent target with a build failure is ClsCompile")
+	}
+	// Absent + clean build = the test is simply not in the tree.
+	if Classify(TestReport{Tests: map[string]Outcome{"TestOther": Passed}}, "TestX") != ClsNoTest {
+		t.Fatal("an absent target with a clean build is ClsNoTest")
+	}
+}
+
+func TestGoConventionBasics(t *testing.T) {
+	c := goConvention{}
+	if !c.IsTestFile("a/b_test.go") || c.IsTestFile("a/b.go") {
+		t.Fatal("IsTestFile keys on the _test.go suffix")
+	}
+	run := c.RunArgs([]string{"go", "test", "./..."}, "TestX")
+	if got := run[len(run)-3:]; got[0] != "-run" || got[1] != "^TestX$" || got[2] != "-json" {
+		t.Fatalf("RunArgs must anchor -run and request -json, got %v", run)
+	}
+	// The base is copied, not aliased: appending must not scribble on the caller's slice.
+	base := []string{"go", "test", "./..."}
+	_ = c.RunArgs(base, "TestX")
+	if len(base) != 3 {
+		t.Fatal("RunArgs must not mutate the base command")
+	}
+	suite := c.SuiteArgs([]string{"go", "test", "./..."})
+	if suite[len(suite)-1] != "-json" || len(suite) != 4 {
+		t.Fatalf("SuiteArgs must append only -json, got %v", suite)
+	}
+	// A name with regexp metacharacters must be quoted so -run matches it literally.
+	q := c.RunArgs([]string{"go", "test"}, "Test.$")
+	if q[len(q)-2] != `^Test\.\$$` {
+		t.Fatalf("RunArgs must regexp-quote the test name, got %q", q[len(q)-2])
+	}
+}
+
+// go test -json fixtures: one JSON object per line.
+const (
+	evPass    = `{"Action":"run","Package":"p","Test":"TestX"}` + "\n" + `{"Action":"pass","Package":"p","Test":"TestX"}` + "\n"
+	evFail    = `{"Action":"run","Package":"p","Test":"TestX"}` + "\n" + `{"Action":"fail","Package":"p","Test":"TestX"}` + "\n"
+	evSibling = `{"Action":"output","Package":"sib","Output":"?   sib [no test files]\n"}` + "\n"
+	evBuild   = `{"Action":"output","Package":"p","Output":"# p [p.test]\n"}` + "\n" +
+		`{"Action":"output","Package":"p","Output":"./a_test.go:5:2: undefined: foo\n"}` + "\n" +
+		`{"Action":"output","Package":"p","Output":"FAIL\tp [build failed]\n"}` + "\n" +
+		`{"Action":"fail","Package":"p"}` + "\n"
+)
+
+func TestGoParseReport(t *testing.T) {
+	c := goConvention{}
+	must := func(code int, out string) TestReport {
+		r, err := c.ParseReport(code, out, "")
+		if err != nil {
+			t.Fatalf("ParseReport unexpected error: %v", err)
+		}
+		return r
+	}
+	// A passing target, amid sibling "[no test files]" noise, classifies as passed.
+	if Classify(must(0, evSibling+evPass), "TestX") != ClsPassed {
+		t.Fatal("pass event -> ClsPassed, sibling noise ignored")
+	}
+	// A failing target is an assertion fail-before.
+	if Classify(must(1, evFail), "TestX") != ClsAssert {
+		t.Fatal("fail event -> ClsAssert")
+	}
+	// Exit 0, target never ran (only a sibling) -> absent, no build failure.
+	if Classify(must(0, evSibling), "TestX") != ClsNoTest {
+		t.Fatal("no target event, clean build -> ClsNoTest")
+	}
+	// The target's package failed to build -> ClsCompile.
+	if Classify(must(1, evBuild), "TestX") != ClsCompile {
+		t.Fatal("build-failed package -> ClsCompile")
+	}
+	// Precision: the TARGET failed an assertion while a SIBLING build-failed -> still ClsAssert.
+	mixed := evFail + `{"Action":"output","Package":"sib","Output":"FAIL\tsib [build failed]\n"}` + "\n" + `{"Action":"fail","Package":"sib"}` + "\n"
+	r := must(1, mixed)
+	if !r.BuildFailed {
+		t.Fatal("a sibling build failure must be recorded")
+	}
+	if Classify(r, "TestX") != ClsAssert {
+		t.Fatal("the target's own assertion failure wins over a sibling build failure")
+	}
+	// An early, non-JSON compiler error line (test2json did not wrap it) still sets BuildFailed.
+	if Classify(must(2, "# p\n./a_test.go:1:1: syntax error\nFAIL\tp [build failed]\n"), "TestX") != ClsCompile {
+		t.Fatal("a non-JSON build-failure line must set BuildFailed")
+	}
+}
+
+func TestGoParseReportUnreadable(t *testing.T) {
+	c := goConvention{}
+	// A nonzero exit with neither JSON events nor a build marker is unreadable — an error, so the
+	// caller fails closed rather than treat garbage as a clean (empty) report.
+	if _, err := c.ParseReport(1, "totally not json and no marker\n", ""); err == nil {
+		t.Fatal("unreadable failing output must be an error")
+	}
+	// Exit 0 with empty output is a legitimately empty run (nothing matched -run), not an error.
+	if r, err := c.ParseReport(0, "", ""); err != nil || len(r.Tests) != 0 || r.BuildFailed {
+		t.Fatalf("empty clean output must be an empty report, got %+v err=%v", r, err)
+	}
+}
+
+func TestGoDeletesATest(t *testing.T) {
+	c := goConvention{}
+	if !c.DeletesATest("@@\n-func TestFoo(t *testing.T) {") {
+		t.Fatal("a removed TestFoo must be detected")
+	}
+	for _, kind := range []string{"Test", "Benchmark", "Fuzz", "Example"} {
+		if !c.DeletesATest("@@\n-func " + kind + "Bar(") {
+			t.Fatalf("a removed %sBar must be detected", kind)
+		}
+	}
+	// A bare "Test(" and "Test_x(" match the naming rule; a lowercase suffix (Testhelper) does not.
+	if !c.DeletesATest("-func Test(") || !c.DeletesATest("-func Test_x(") {
+		t.Fatal("Test( and Test_x( follow the naming rule")
+	}
+	if c.DeletesATest("-func Testhelper(") {
+		t.Fatal("Testhelper is not a test function")
+	}
+	// A "---" diff header line that happens to start with a removed marker must be skipped.
+	if c.DeletesATest("--- a/foo_test.go\n") {
+		t.Fatal("a diff header must not be read as a removed test")
+	}
+	if c.DeletesATest("@@\n context line\n+func TestAdded(") {
+		t.Fatal("an added test is not a deletion")
+	}
+}
+
+func TestGoDirHasTests(t *testing.T) {
+	c := goConvention{}
+	root := t.TempDir()
+	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
+		t.Fatal(err)
+	}
+	if c.DirHasTests(root, "pkg/a.go") {
+		t.Fatal("a package with no _test.go has no tests")
+	}
+	if err := os.WriteFile(filepath.Join(root, "pkg", "a_test.go"), []byte("package pkg\n"), 0o644); err != nil {
+		t.Fatal(err)
+	}
+	if !c.DirHasTests(root, "pkg/a.go") {
+		t.Fatal("a package with a _test.go has tests")
+	}
+}
+
+func TestGoSemanticallyNull(t *testing.T) {
+	c := goConvention{}
+	null := [][2]string{
+		{"package p\n// one\nvar x = 1\n", "package p\n// two\nvar x = 1\n"},             // comment change
+		{"package p\nvar x = 1\n", "package p\nvar   x =  1\n"},                          // whitespace
+		{"package p\n// note: one\nvar x = 1\n", "package p\n// note: two\nvar x = 1\n"}, // colon in a plain comment
+		{"package p\n/* one */\nvar x = 1\n", "package p\n/* two */\nvar x = 1\n"},       // block comment
+	}
+	for _, p := range null {
+		if !c.SemanticallyNull(p[0], p[1]) {
+			t.Fatalf("expected null: %q vs %q", p[0], p[1])
+		}
+	}
+	notNull := [][2]string{
+		{"package p\nvar x = 1\n", "package p\nvar x = 2\n"},                 // real change
+		{"package p\nvar x = 1\n", "package p\nvar x = 1 + y\n"},             // added token
+		{"package p\nvar x = 1\n", "package p\nvar x = \"unterminated\n"},    // mutated does not scan
+		{"package p\nvar x = `unterminated", "package p\nvar x = 1\n"},       // orig does not scan
+		{"//go:build linux\npackage p\n", "//go:build windows\npackage p\n"}, // directive
+		{"package p\n//go:noinline\nfunc f() {}\n", "package p\n//go:noescape\nfunc f() {}\n"},
+		{"package p\nfunc f() {}\n", "package p\n//go:noinline\nfunc f() {}\n"}, // directive added
+		{"package p\n//export One\nfunc f() {}\n", "package p\n//export Two\nfunc f() {}\n"},
+		{"package p\n//extern one\nfunc f()\n", "package p\n//extern two\nfunc f()\n"},
+		{"// +build linux\npackage p\n", "// +build windows\npackage p\n"},
+		{"//   +build linux\npackage p\n", "//   +build windows\npackage p\n"},
+		{"//\t+build linux\npackage p\n", "//\t+build windows\npackage p\n"},
+	}
+	for _, p := range notNull {
+		if c.SemanticallyNull(p[0], p[1]) {
+			t.Fatalf("expected NOT null: %q vs %q", p[0], p[1])
+		}
+	}
+}
diff --git a/tests/coverage-floor.txt b/tests/coverage-floor.txt
index f396b99..a9dd9d0 100644
--- a/tests/coverage-floor.txt
+++ b/tests/coverage-floor.txt
@@ -39,3 +39,4 @@ internal/state 84.9
 internal/status 100.0
 internal/taskdone 89.4
 internal/tasksource 100.0
+internal/testconv 100.0



````

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

# Evidence — PR-A: the `TestConvention` seam + `GoConvention` on `go test -json`

**Task:** multi-language support, PR-A (the seam; Go reworked onto the runner's structured output; no
new language yet). **Design:** `docs/handoffs/handoff-2026-09-01-multilang-testconvention-seam.md`.
**Decision (Dave, 2026-09-01):** option 3 — uniform rework onto each runner's own `--json`/test2json
output, **no bespoke parser, no new dependency**; mirror the mutation-report idiom
(`internal/mutation/{parse,stryker}.go`). **Base:** `main` (`0ea89c9`).

## What landed

New leaf package **`internal/testconv`** (zero internal deps):

- `TestReport{Tests map[string]Outcome; BuildFailed bool}` — the normalized result of a run, built
  ONLY from a runner's structured output (the analogue of `mutation.Report`).
- `Classify(report, test) Class` — **one generic function**, no per-language string scraping: target
  Passed→`ClsPassed`, Failed→`ClsAssert`, absent+BuildFailed→`ClsCompile`, absent+clean→`ClsNoTest`.
- `Convention` interface + a compile-time `For(name)` registry that **fails closed** on an unknown/empty
  name (`(nil,false)`), never a silent default.
- `GoConvention`: `IsTestFile` (`*_test.go`), `RunArgs`/`SuiteArgs` (append `-run ^X$ -json` / `-json`),
  `ParseReport` (reads the **`go test -json`** / test2json event stream), `DeletesATest` (the `testing`
  package's exact name rule — a language spec, not a hand-rolled parser), `DirHasTests` (`*_test.go`
  glob), `SemanticallyNull` (`go/scanner` token walk — already the robust form, moved verbatim).

`ParseReport` is the crux: it records per-test `pass`/`fail` events keyed by the target's own name, so a
**sibling package's noise is a different `Package`/`Test` and is never recorded** — the multi-package
`go test ./...` hole that cost a bot round in T1.3 is now closed *structurally*, not by string-filtering
([[single-package-fixtures-hide-classify-bugs]]). Output it cannot read (a nonzero run with no JSON
events and no build marker) is an **error**, so the caller fails closed — never an empty report scored
as a clean run (the mutation package's refuse-unreadable rule).

A precision gain falls out for free: when the target test genuinely `fail`s while a *sibling* package
build-fails, the report attributes the failure to the target → `ClsAssert` (a valid fail-before). The
old console-scrape could not attribute the `[build failed]` marker, so it conservatively called any such
run a compile error (spec §9.3(b) worked around a limitation the structured report removes).

## Rewired callers (behavior identical or strictly more robust)

- `internal/mutation/reproduction.go` — `Reproducer` gains a `Convention` field; `runTest` builds argv
  via `RunArgs`, runs, and normalizes via `ParseReport`; the two `reproduceOne` switches classify via
  `testconv.Classify`. The Go-specific `classify`/`isCompileError`/`failClass`/`isTestFile` are gone.
- `internal/mutation/verify.go` — the trivial-pin pre-screen routes through `Convention.SemanticallyNull`
  (`prescreen.go` deleted; its logic and tests moved to `internal/testconv`).
- `internal/fsm/kind/prove.go` — `ProveSpec` gains `Convention`, resolved from a new node param
  **`test_convention`** (default `"go"`; unknown → node aborts, fail closed). `deletesATest`/
  `packageHasTests` are removed and routed through the convention; `owesPin` takes the convention.
- `prove_reproduction.go` / `prove_mutation.go` — the engine and the pin Verifier receive the convention.

A nil `Convention` in the `mutation` package defaults to Go (test ergonomics only); the FSM path always
sets it explicitly and fails closed on an unknown name upstream.

## Tests, coverage, mutation verification

- `internal/testconv` **100.0%** (added to `tests/coverage-floor.txt`). `internal/fsm/kind` and all
  `internal/fsm/*` + `workflows` remain **100.0%**. `internal/mutation` **95.6%** (floor 94.2).
- Existing reproduction/deletion tests were moved from console-text mock outputs to `go test -json`
  fixtures; the moved unit tests for `SemanticallyNull` and `deletesATest` now live in
  `internal/testconv` (relocated alongside their code, assertions unchanged).
- The unknown-`test_convention` abort has its own test (`TestProveUnknownConventionAborts`).
- **Mutation-verified** (file-backup, line-targeted, re-run independently —
  [[mutation-verify-without-git-checkout]], [[verify-delegated-fixes]]): all killed —
  `Classify` fail→pass; `ParseReport` fail-event→pass; `isBuildFailure`→false; and the cross-package
  wiring `GoConvention.DeletesATest`→false (killed by an `internal/fsm/kind` §9.6 test). Tree confirmed
  clean of mutations afterward.
- `gofmt`/`go vet` clean; full `go test ./...` green.

## Scope / follow-ups (deferred, with rationale)

- **PR-B** adds `TypeScriptConvention` (Jest/Vitest `--json`) + per-target selection, and the runner
  executed-test **set-diff** for TS `DeletesTests` (no single-line rule exists for `it`/`test`).
- `ScopeSuite`/`suiteRun` still keys on exit code alone (its documented all-or-nothing contract);
  `SuiteArgs` is defined for PR-B's set-diff. `Verifier.BuildCmd`'s Go default is untouched (a possible
  later convention method). The §9.2 `FailBefore` now carries the raw `go test -json` stream (the
  assertion text is present in the output events); prettifying it for the reviewer is a possible refinement.

