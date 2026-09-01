# Evidence — the Python / pytest convention

**Task:** the fourth language on the `testconv` seam (after Go #48, Jest #49, Vitest #50). A background
fork was meant to build this but never did — built here in the main session. **Base:** `main`
(`8c6f8a0`, post-Gap-#1). **Design basis:** Dave's steer — the runner's own robust structured output,
no bespoke parser, no new dependency.

## Verified first (the recurring lesson)

pytest has **no native stdout JSON**. Its native, dependency-free machine format is **JUnit XML**
(`--junit-xml`, built into pytest core; also the cross-language test-result standard). Verified against
the pytest/JUnit docs before building: `<testsuite>`/`<testcase>` with `classname`/`name`/`file`
attributes; a `<failure>` child = an assertion failed (test ran), `<error>` = a crash/collection/setup
failure (no assertion reached), `<skipped>` = not run; and that pytest ≥6.1 defaults to **xunit2**,
whose root is a bare `<testsuite>` (no `<testsuites>` wrapper) — both roots are handled. Sources below.

## What landed (`internal/testconv/python.go`)

`pythonConvention` registered as `"python"`:
- **`IsTestFile`/`DirHasTests`** — pytest discovery: `test_*.py` / `*_test.py`.
- **`RunArgs`/`SuiteArgs`** — `--junit-xml=/dev/stdout` (+ the **nodeid** as a positional for one test).
  The nodeid is the unambiguous selector pytest accepts (never `-k`, a substring match); passed as one
  argv element so it cannot inject a flag.
- **`ParseReport`** — extracts the JUnit XML block from pytest's combined console+XML stdout (the XML is
  one block at session end), parses it with `encoding/xml` under either root, and normalizes to the
  shared `TestReport`: `<failure>` → Failed, a bare testcase → Passed, `<error>` → `BuildFailed` (the
  assertion-vs-compile axis — a crash/collection error is not an assertion), `<skipped>` → absent. A
  **Failed result sticks** (never masked by a same-named pass). No XML block, or malformed XML, is an
  error → fail closed (never an empty report scored clean).
- **`pytestNodeID`** reconstructs the nodeid from a testcase's `file`/`classname`/`name`: the class
  segments are those in `classname` after the module (found as the segments following the last one
  equal to the file's stem), so a function is `file.py::name` and a method `file.py::Class::name`;
  parametrized `[params]` rides along in `name`. This is the one genuinely fiddly bit; a reconstruction
  miss fails closed (the target reads as absent → malformed), never a false proof.
- **`DeletesATest`** — the same conservative, parser-free structural signal as the TS/Vitest
  conventions (a removed content line in a `test_*`/`*_test` file). **`SemanticallyNull`** → false
  (no stdlib Python tokenizer; dependency-free rule).

## Selection & portability
`test_convention: python` (alongside `go`/`typescript`/`vitest`); default stays `go`; unknown aborts
(fail closed). `--junit-xml=/dev/stdout` is a **Unix** mechanism — on Windows the report needs a
file-based seam extension (a documented follow-up, the same shape as the Jest/Vitest `outputFile` edge).
If the XML does not reach stdout (e.g. a repo config redirects it to a file), `ParseReport` fails closed.

## Tests, coverage, mutation verification
- `internal/testconv` **100.0%** (go + jest + vitest + python). `TestForSelectsAndFailsClosed` now
  asserts `python` resolves; unknown/empty still fail closed.
- Fixtures cover both XML roots, pass/failure→Failed/error→BuildFailed/skipped→absent, method vs
  function vs parametrized nodeids, XML embedded in console noise, a same-named failure-wins case, and
  three unreadable/malformed fail-closed paths. `TestPyNodeIDReconstruction` pins the nodeid mapping.
- **Mutation-verified** (file-backup, line-targeted, re-run): all killed — `<failure>`→Passed,
  `<error>` not setting BuildFailed, the sticky-Failed guard, nodeid dropping class segments, RunArgs
  dropping the junit flag, and the `<testsuites>` extraction branch.
- `gofmt`/`go vet`/golangci-lint clean; full `go test ./...` green.

## Honest limits
The JUnit XML **shape** is verified against docs and the parser is unit-tested against captured
fixtures, exactly as the other three conventions are. End-to-end delivery via `--junit-xml=/dev/stdout`
against a live pytest run is **not** exercised here (no Python toolchain in this repo); it is the
documented pytest mechanism and fails closed if the XML is absent.

## Now covered
`go`, `typescript` (Jest), `vitest`, and `python` (pytest) — a repo in any of the four participates by
selecting its `test_convention`.

## Sources
- [pytest — JUnit XML (`--junit-xml`, xunit2 default)](https://docs.pytest.org/en/stable/how-to/output.html#creating-junitxml-format-files)
- [JUnit XML format — testsuite/testcase, failure vs error vs skipped](https://gaffer.sh/blog/junit-xml-format-guide/)
