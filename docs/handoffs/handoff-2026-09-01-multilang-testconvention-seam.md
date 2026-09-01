# Handoff — the `TestConvention` seam (multi-language: Go reference, then TypeScript)

**Date:** 2026-09-01. **Follow-up of:** the pins-and-bug-class Execution Loop (T1.3–T2.2, merged
#41–#46) and the 0.10.0 candidate "The pins/bug-class engine is Go-only at the language seams"
(`docs/0.10.0-candidates.md`, merged #47). **Spec basis:** §4.2 frames the test convention as
pluggable. **Decision (Dave, 2026-09-01):** build the seam, TypeScript first.

## Goal

The reproduction/deletion/trivial-pin machinery has a language-agnostic spine but four leaf sites
hard-wired to Go. Extract a `TestConvention` seam, keep Go behavior byte-identical behind it, then add
a TypeScript convention (Jest/Vitest). Everything else — FSM/gates, the real `git worktree` engine,
consent-hashed `cmd`, unified-diff parsing, the exit-code scope check, mutation-kill re-verification —
is already language-neutral and MUST NOT change behavior.

## The four Go-hardwired sites (exact locations)

1. **`internal/mutation/reproduction.go`**
   - `isTestFile(path)` (l.139) — `strings.HasSuffix(path, "_test.go")`.
   - `runTest` (l.381) — appends **`-run ^<test>$ -v`** to `TestCmd`; the whole Go single-test
     narrowing + verbose convention.
   - `classify(test, code, out)` (l.454) — keys on **`=== RUN   <test>`** and **`--- FAIL: <test>`**.
   - `isCompileError(out)` (l.473) — **`[build failed]` / `[setup failed]` / `[vet failed]`**.
2. **`internal/mutation/prescreen.go`** — `semanticallyNull` / `goTokens` / `directive`: the §9.8 R7
   trivial-pin pre-screen, a `go/scanner` token walk with Go directive rules (`//go:build`, `//export`,
   `//extern`, `// +build`, `//line`).
3. **`internal/fsm/kind/prove.go`**
   - `deletesATest(diff)` / `testFuncRemovedRe` (l.267/272) — Go's `^-func (Test|Benchmark|Fuzz|Example)…`
     removal rule (§9.6 gate).
   - `packageHasTests(dir, file)` (l.442) — globs **`*_test.go`** in the file's directory.
4. (`internal/mutation/verify.go` — the pin `Verifier` — is ALREADY language-agnostic: pins are
   proven by exit code / build-vs-test split, no marker parsing. Leave it. Confirm during build.)

## The seam

New leaf package **`internal/testconv`** (zero internal deps, so both `internal/mutation` and
`internal/fsm/kind` can import it without a layering cycle — mirrors reproduction.go's "this package
may not import the FSM" boundary):

```go
package testconv

// TestReport is the normalized result of running the suite (or one test), across runners — the
// analogue of mutation.Report. Built ONLY from a runner's structured output; never source-parsed.
type TestReport struct {
    Tests       map[string]Outcome // test id → Passed|Failed (the SET that executed; drives deletion detection)
    BuildFailed bool               // a compile/transpile/setup failure — no assertion was reached
}
type Outcome int
const ( Passed Outcome = iota; Failed )

// Class is the assertion-vs-compile axis for ONE target test, DERIVED generically from a TestReport
// (was mutation.failClass; no per-language string scraping).
type Class int
const ( ClsAssert Class = iota; ClsPassed; ClsNoTest; ClsCompile; ClsOther )

type Convention interface {
    Name() string                                    // "go" | "typescript"
    IsTestFile(path string) bool                     // reproduction partition + DirHasTests
    RunArgs(base []string, test string) []string      // narrow base cmd to ONE test, STRUCTURED output
    SuiteArgs(base []string) []string                 // whole suite, STRUCTURED output (deletion set + ScopeSuite)
    ParseReport(code int, stdout, stderr string) (TestReport, error) // runner's -json → normalized; unreadable → error
    DirHasTests(root, file string) bool               // §3.1 owesPin gate — glob the runner's test-file pattern
    SemanticallyNull(orig, mutated string) bool       // §9.8 R7 trivial-pin pre-screen (Go: go/scanner)
}

func For(name string) (Convention, bool)             // registry; "" / unknown → (nil,false), FAIL CLOSED
```

**Classification** is **generic** over `TestReport`: `Classify(report, test) Class` (target
Passed→ClsPassed; Failed→ClsAssert; BuildFailed & absent→ClsCompile; absent→ClsNoTest; else ClsOther)
— one function, no per-language string scraping. **Test-deletion** stays a per-language
`DeletesATest(diff) bool` behind the seam (the §9.6 trigger is a CHEAP diff check — computing it from a
runner set-diff would force two extra whole-suite runs on every prove). Each convention uses its most
robust AVAILABLE mechanism: Go matches its `testing` package's exact name rule (a language spec, not a
hand-rolled parser); TS — which has no single-line rule (`it`/`test`/`describe` are aliasable) — will
use the **runner's executed-test set-diff** across the pre/post suite reports it already produces (a
`DeletesTests(pre, post TestReport)` path added in PR-B, no regex, no parser). The genuinely fragile
site, classification, is what `go test -json` fixes now.

`GoConvention.ParseReport` reads the **`go test -json`** event stream (test2json): filter to the
target `Package`+`Test`; a `pass`/`fail` action sets the outcome; a package `fail` with
`build`/`setup`/`vet` output (or a `build-fail` action) sets `BuildFailed`. This replaces the current
`=== RUN`/`--- FAIL:` scrape AND kills the multi-package sibling-noise bug at the root — siblings are
just different `Package` values, ignored structurally.

## Threading (how each site gets its Convention)

- **Selection:** a new workflow node param **`test_convention`** (string), resolved in
  `proveExec.Execute` beside `testCmdParam`, **defaulting to `"go"`** when absent (every existing
  workflow keeps working unchanged). Put the resolved `Convention` on **`ProveSpec.Convention`**.
- `ProveSpec.Convention` → `mutation.Reproducer.Convention`; `Reproducer` runs `RunArgs`/`SuiteArgs`,
  calls `ParseReport`, derives `Class` via `testconv.Classify`. `isTestFile` → `Convention.IsTestFile`.
- `ProveSpec.Convention` → `deletesATest` → `Convention.DeletesATest(diff)` (Go keeps its exact
  name-rule matcher) and `owesPin`/`packageHasTests` → `Convention.DirHasTests`.
- `Verifier`/`Reproducer` gain a `Convention` field; `SemanticallyNull` routes through it. A nil
  Convention in `mutation` defaults to Go (test ergonomics); the FSM path ALWAYS sets it.
- **Fail closed:** an unknown/empty convention name where a proof needs one → `PinUnverifiable`
  ("no test convention for <name>"), NEVER a fall-through to Go. `ParseReport` on output it cannot
  read returns an error (→ unverifiable), never an empty report scored clean (the mutation package's
  refuse-unreadable rule).

## Decomposition — TWO PRs (methodology: one language per PR)

**PR-A — the seam + `GoConvention` on `go test -json`; rework Go onto the robust path** (Dave chose
"uniform: rework Go too, dep-free", 2026-09-01). Introduce `internal/testconv` with the normalized
`TestReport` model, generic `Classify`, and `GoConvention` reading `go test -json`;
rewire reproduction.go, prove.go's `deletesATest`/`packageHasTests`, and the `SemanticallyNull` call
through the seam (Go's `DeletesATest` keeps its merged name-rule matcher unchanged).
Existing reproduction/deletion tests move from console-text inputs to `go test -json` fixtures (the
*behavior* is identical or strictly more robust; the *inputs* change). Default `test_convention:"go"`.
Mutation-verified the seam is load-bearing (stub `GoConvention.ParseReport` to always-Passed → a
reproduction test reddens). `SemanticallyNull` stays `go/scanner`. **No bespoke parser, no new dep.**

**PR-B — add `TypeScriptConvention` (Jest/Vitest) + per-target selection.**
- `IsTestFile`/`DirHasTests`: `*.test.ts(x)` / `*.spec.ts(x)` (+ `.js`/`.jsx`).
- `RunArgs`/`SuiteArgs`: `--json` (Jest) / `--reporter=json` (Vitest). **Test identity is the
  `describe > it` STRING**, not a function name — `Proof.Test` carries that.
- `ParseReport`: normalize Jest `--json` (`testResults[].assertionResults[].status`
  "passed"/"failed"; runtime/transpile error + no assertions → `BuildFailed`) into `TestReport`. Same
  generic `Classify`/deletion-diff as Go — NO new parser, NO regex.
- `SemanticallyNull`: no stdlib JS/TS tokenizer, and PR-B adds NO parser dependency (the dep-free
  rule). Return `false` conservatively → always run the full compile/break steps (safe; skips only the
  trivial-pin optimization for TS). A pluggable tokenizer is a later, separately-justified step.
- **Multi-package Jest fixtures required** before trusting `ParseReport`
  ([[single-package-fixtures-hide-classify-bugs]]).

## Constraints (all from the memories / prior rounds)

- **TDD red-first, mutation-verified, `internal/fsm/*` at 100%.** New `internal/testconv` and
  `internal/mutation` additions carry their own tests to their floor.
- **Classification is the per-language-fragile part.** The Go classifier already burned a round on
  multi-package `go test ./...` sibling noise — see [[single-package-fixtures-hide-classify-bugs]].
  The TS classifier needs monorepo/multi-package Jest fixtures (JSON with several `testResults`) before
  it is trusted.
- One language per PR through `review task-done` → PR → bot-shepherd → squash-merge → sync main.
- Watch the task-done self-reference trap ([[task-done-todo-self-reference-trap]]) and unique agent
  scratch paths ([[unique-agent-scratch-paths]]) if any subagents run mutations.

## Related memories

[[pins-bug-class-execution-loop-status]], [[single-package-fixtures-hide-classify-bugs]],
[[mock-judge-hides-node-wiring-bugs]], [[verify-delegated-fixes]], [[task-done-todo-self-reference-trap]].
