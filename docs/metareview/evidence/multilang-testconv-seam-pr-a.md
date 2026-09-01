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

## Shepherding round 1 (Cursor Bugbot — 2× High, fixed)

- **`validateProve` rejected `test_convention`.** The param was read at Execute but the validator
  accepted only `test_cmd`, so a workflow selecting a language would fail to load. Fixed: the validator
  accepts and type-checks `test_convention` (an unknown NAME still aborts at Execute, fail closed).
- **`ParseReport` same-named tests across packages could mask a failure.** Keyed by test name with
  last-write-wins, a same-named test in a second package (both matched by `-run ^Name$`) could let a
  later pass hide an earlier failure. Fixed: a recorded `Failed` sticks; a pass only sets an
  absent/passed entry. A failure is never hidden; the residual can only refuse to prove, never mint a
  false proof, and §9.2 independently checks the fail-before. Both fixed with tests; mutation-verified.

## Scope / follow-ups (deferred, with rationale)

- **PR-B** adds `TypeScriptConvention` (Jest/Vitest `--json`) + per-target selection, and the runner
  executed-test **set-diff** for TS `DeletesTests` (no single-line rule exists for `it`/`test`).
- `ScopeSuite`/`suiteRun` still keys on exit code alone (its documented all-or-nothing contract);
  `SuiteArgs` is defined for PR-B's set-diff. `Verifier.BuildCmd`'s Go default is untouched (a possible
  later convention method). The §9.2 `FailBefore` now carries the raw `go test -json` stream (the
  assertion text is present in the output events); prettifying it for the reviewer is a possible refinement.
