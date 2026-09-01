# Evidence — the Vitest convention

**Task:** add Vitest support to the `testconv` seam (Dave asked, 2026-09-01, after the Jest-scoping of
PR-B #49 surfaced the gap). **Base:** `main` (`fdec33a`). **Design basis:** same seam as PR-A (#48) /
PR-B (#49); Dave's steer — runner's own structured output, no bespoke parser, no new dependency.

## Verified first (not assumed)

The PR-B post-mortem was that I *claimed* Vitest without checking. This time I verified Vitest's
reporter against its docs before building: Vitest's `json` reporter (`--reporter=json`) emits a
**Jest-compatible aggregated result** — the same `numTotalTestSuites` / `testResults[]` /
`assertionResults[]` (`fullName`, `status`, `title`, `ancestorTitles`) shape, "a simplified version of
the Jest format." Its results reporter flag is `--reporter=json` (NOT Jest's bare `--json`). Sources
below.

## What landed (`internal/testconv/vitest.go`)

- Extracted the Jest-format parser into a shared package-level `parseJSCompatReport(runner, code,
  stdout)` + `findJSCompatReportLine` + `jsCompatReport` (renamed from the Jest-only names, since two
  conventions now share it). The Jest convention delegates to it unchanged.
- `vitestConvention` **embeds `typeScriptConvention`**, so `IsTestFile`, `DirHasTests`, `DeletesATest`,
  and `SemanticallyNull` are literally the same code. It overrides only:
  - `Name()` → `"vitest"`.
  - `RunArgs` / `SuiteArgs` → `--testNamePattern ^<name>$ --reporter=json` / `--reporter=json`.
  - `ParseReport` → the shared parser with runner name `"vitest"` (for accurate error messages).
- The shared parser's transpile-failure signal is robust to Vitest's *simplified* format: Vitest may
  omit `numRuntimeErrorTestSuites`, so the **failed-suite-with-no-assertions** fallback is what marks a
  Vitest collection/transpile failure `BuildFailed`. The `numTotalTestSuites` sentinel (also present in
  Vitest) still isolates the report line from pretty-reporter braces in combined stdout+stderr.

## Selection

`test_convention: vitest` (registered alongside `go` / `typescript`). No workflow YAML change; default
stays `go`; an unknown name still aborts the node (fail closed). The base command must be a one-shot run
(`vitest run` / CI mode) — the convention adds only flags, matching how the Jest convention leaves
non-interactive invocation to the consent-hashed base command.

## Tests, coverage, mutation verification

- `internal/testconv` **100.0%** (Go + Jest + Vitest). `TestForSelectsAndFailsClosed` now asserts `go`,
  `typescript`, and `vitest` resolve; unknown/empty fail closed.
- Vitest tests cover the reporter flags, base-slice non-mutation, a Jest-compatible pass/fail report,
  the no-`numRuntimeErrorTestSuites` transpile fallback, inherited predicates (spot check), and the
  no-report-line fail-closed error.
- **Mutation-verified** (file-backup, line-targeted, re-run): all killed — `RunArgs` and `SuiteArgs`
  reporter flag (`--reporter=json`→`--json`), and the `ParseReport` shared-parser wiring. Tree confirmed
  clean afterward.
- `gofmt`/`go vet` clean; full `go test ./...` green.

## Shepherding round 1 (Cursor Bugbot — 1× High, corrected)

- **"Vitest JSON not emitted on stdout" — the premise was wrong.** Bugbot claimed Vitest's json reporter
  writes to a file *by default*. Vitest's docs say the opposite: "Can either be printed to the terminal
  or written to a file using the `outputFile` configuration option" — i.e., stdout by default, which is
  what `ParseReport` reads. No code change to the default path. The legitimate residual — a repo that
  hard-wires `outputFile` in its Vitest config sends the report to a file and leaves stdout empty — is
  now documented: it fails CLOSED (no report line → unverifiable), never silently wrong, the same edge
  as Jest's `--outputFile`. Verified against [Vitest Reporters](https://vitest.dev/guide/reporters).

## Now covered

`go` (go test -json), `typescript`/Jest (`--json`), and `vitest` (`--reporter=json`). A repository in
any of the three participates by selecting its `test_convention`. Python (pytest) is the next follow-up.

## Sources

- [Vitest — Reporters (json reporter, Jest-compatible)](https://vitest.dev/guide/reporters)
- [Vitest PR #489 — feat: json reporter](https://github.com/vitest-dev/vitest/pull/489)
