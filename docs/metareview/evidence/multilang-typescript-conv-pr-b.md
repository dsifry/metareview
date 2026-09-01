# Evidence — PR-B: the TypeScript convention (Jest/Vitest `--json`)

**Task:** multi-language support, PR-B — add `TypeScriptConvention` on the seam PR-A landed (#48).
**Design:** `docs/handoffs/handoff-2026-09-01-multilang-testconvention-seam.md`. **Decision (Dave,
2026-09-01):** each runner's OWN structured output, **no bespoke parser, no new dependency**. **Base:**
`main` (`623a255`).

## What landed (`internal/testconv/typescript.go`)

A `typeScriptConvention` registered as `"typescript"`, implementing the seam entirely on Jest/Vitest's
own machine-readable report — no source parsing, no regex over test syntax, no dependency:

- **`IsTestFile` / `DirHasTests`** — the `*.test.*` / `*.spec.*` suffixes (`ts`/`tsx`/`js`/`jsx`/`mts`/`cts`).
- **`RunArgs` / `SuiteArgs`** — `--testNamePattern ^<name>$ --json` (one test) / `--json` (whole suite).
  Test identity is the full `describe > it` name (space-joined), anchored and quoted so it selects one
  test and cannot inject a flag. Vitest's `--reporter=json` emits the same shape.
- **`ParseReport`** — normalizes Jest `--json` into the shared `TestReport`: each `assertionResult`
  (`fullName`, or `ancestorTitles + title`) → `Passed`/`Failed`; a suite Jest could not run
  (`numRuntimeErrorTestSuites > 0`, or a failed suite with zero assertions — a transpile/collection
  error) → `BuildFailed`. Classification then uses the **same generic `Classify`** as Go. Output with no
  decodable JSON object is an error (fail closed), never an empty report scored clean. A `Failed` result
  **sticks** — a same-named test in another suite can never mask a failure (matching the Go convention's
  fix). A stray log line before the JSON is tolerated (first `{` … last `}`).
- **`DeletesATest`** — TS has no single-line test-declaration rule (`it`/`test`/`describe` are
  aliasable), and a bespoke parser/regex is out (Dave's steer). So this is a **conservative structural
  signal**: a removed content line inside a hunk of a file whose path `IsTestFile` (either side of the
  `diff --git` line, so a deleted test file is caught by its old path). It over-triggers on any
  test-file line removal — SAFE: the §9.6 gate then re-checks proven pins and finds no regression when
  none exists; the cost is a recheck, never a missed one.
- **`SemanticallyNull`** — always `false`: no standard-library JS/TS tokenizer and the seam is
  dependency-free, so the trivial-pin pre-screen is skipped for TS (the safe direction — the full
  compile-then-break steps still run).

## Selection & wiring

No workflow YAML change: a TS repository selects the convention with the existing `test_convention:
typescript` node param (PR-A). `test_convention` absent still defaults to Go, and an unknown name still
aborts the node (fail closed). Nothing Go-specific changed.

## Tests, coverage, mutation verification

- `internal/testconv` **100.0%** (Go + TS). `TestForSelectsAndFailsClosed` now asserts both `go` and
  `typescript` resolve, and an unknown name (`cobol`) / empty name fail closed.
- Jest-`--json` fixtures cover pass, assertion-fail (name reconstructed from ancestorTitles+title),
  transpile failure (failed suite, no assertions), collection error (`numRuntimeErrorTestSuites`), a
  report preceded by log noise, and a same-named pass/fail across suites (failure wins). Unreadable
  output (no JSON object; an invalid object) is an error. `DeletesATest` covers a removed test-file
  line, a deleted test file, a production-file removal (no trigger), an addition-only change (no
  trigger), and a header-only change (no trigger).
- **Mutation-verified** (file-backup, line-targeted, re-run — [[mutation-verify-without-git-checkout]],
  [[verify-delegated-fixes]]): all killed — `ParseReport` failed→passed; `numRuntimeErrorTestSuites`
  build-fail disabled; transpile (no-assertions) build-fail disabled; the sticky-`Failed` guard;
  `DeletesATest` test-file gate forced true; `IsTestFile` forced false. Tree confirmed clean afterward.
- `gofmt`/`go vet` clean; full `go test ./...` green.

## Follow-ups (deferred)

- **Python (pytest)** next, then Java/Ruby/Rust as demand shows — each a new convention on the same seam.
- A TS trivial-pin pre-screen would need a pluggable JS/TS tokenizer — a separately-justified step, not
  a dependency added here.
- End-to-end execution against a real Jest/Vitest project is out of scope for a Go unit suite (no JS
  toolchain in this repo); `ParseReport` is tested against captured report shapes, the same way the Go
  convention is unit-tested against `go test -json` fixtures.
