# metareview task-done context

Run ID: `mrv-20260901-174949441189000-task-done-multilang-vitest-conv-56254690`

## Task

Advisory task target: multilang-vitest-conv

## Git

- Base: `fdec33a9b92c002f111d20bb326da67dde40fbb1`
- Head: `82e1ed59ec255d6c7ba9ad58ae6a2a9a9bf703d1`
- Branch: `multilang-vitest-conv`
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `34770`
- Filtered diff bytes: `11870`
- Risk level: `none`
- Generated files excluded: docs/metareview/context/mrv-20260901-173913335292000-task-done-multilang-vitest-conv-56254690-context.md, docs/metareview/evidence/multilang-vitest-conv.md, docs/metareview/reviews/mrv-20260901-173913335292000-task-done-multilang-vitest-conv-56254690.md

## Context Shard Plan

Not sharded.

## Review Manifest

- Manifest verdict: `PASS`
- Source manifest hash: not sharded
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- internal/testconv/testconv_test.go
- internal/testconv/typescript.go
- internal/testconv/vitest.go
- internal/testconv/vitest_test.go

### Path Dispositions
- docs/metareview/context/mrv-20260901-173913335292000-task-done-multilang-vitest-conv-56254690-context.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/evidence/multilang-vitest-conv.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/reviews/mrv-20260901-173913335292000-task-done-multilang-vitest-conv-56254690.md: generated (metareview generated review artifact excluded from source manifest)

### Manifest Blockers
No manifest blockers.

## Changed Files

- internal/testconv/testconv_test.go
- internal/testconv/typescript.go
- internal/testconv/vitest.go
- internal/testconv/vitest_test.go

## Diff

```diff
diff --git a/internal/testconv/testconv_test.go b/internal/testconv/testconv_test.go
index fbd1abe..6154797 100644
--- a/internal/testconv/testconv_test.go
+++ b/internal/testconv/testconv_test.go
@@ -14,6 +14,9 @@ func TestForSelectsAndFailsClosed(t *testing.T) {
 	if c, ok := For("typescript"); !ok || c == nil || c.Name() != "typescript" {
 		t.Fatalf("For(typescript) must return the TS convention, got %v ok=%v", c, ok)
 	}
+	if c, ok := For("vitest"); !ok || c == nil || c.Name() != "vitest" {
+		t.Fatalf("For(vitest) must return the Vitest convention, got %v ok=%v", c, ok)
+	}
 	// The load-bearing negative: an unknown or empty name returns (nil,false) so the caller fails
 	// closed — never a silent default to a language.
 	if c, ok := For("cobol"); ok || c != nil {
diff --git a/internal/testconv/typescript.go b/internal/testconv/typescript.go
index 9a37995..58ef523 100644
--- a/internal/testconv/typescript.go
+++ b/internal/testconv/typescript.go
@@ -51,10 +51,16 @@ func (typeScriptConvention) SuiteArgs(base []string) []string {
 	return append(append([]string(nil), base...), "--json")
 }
 
-// jestReport is the subset of Jest's `--json` output this reads. Each assertionResult is one test
-// (`fullName` = ancestorTitles + title); a testResult with no assertions but a failed status is a suite
-// that failed to transpile/execute — a build failure, not an assertion.
-type jestReport struct {
+// ParseReport normalizes a Jest `--json` run (see parseJSCompatReport).
+func (typeScriptConvention) ParseReport(code int, stdout, stderr string) (TestReport, error) {
+	return parseJSCompatReport("typescript", code, stdout)
+}
+
+// jsCompatReport is the subset of the Jest `--json` aggregated result this reads. Vitest's `json`
+// reporter emits the SAME (Jest-compatible) shape, so both conventions share this parser. Each
+// assertionResult is one test (`fullName` = ancestorTitles + title); a testResult with no assertions
+// but a failed status is a suite that failed to transpile/execute — a build failure, not an assertion.
+type jsCompatReport struct {
 	NumRuntimeErrorTestSuites int `json:"numRuntimeErrorTestSuites"`
 	TestResults               []struct {
 		Status           string `json:"status"`
@@ -67,19 +73,21 @@ type jestReport struct {
 	} `json:"testResults"`
 }
 
-// ParseReport normalizes a Jest `--json` run. A build/transpile failure (a suite Jest could not
-// run: numRuntimeErrorTestSuites, or a failed suite with zero assertions) sets BuildFailed — the same
-// assertion-vs-compile distinction the Go convention draws. As with the mutation-report readers, output
-// it cannot decode is an error (the caller fails closed), never an empty report scored as a clean run.
-// A FAILED test is authoritative and is never masked by a same-named pass (matching the Go convention).
-func (typeScriptConvention) ParseReport(code int, stdout, stderr string) (TestReport, error) {
-	data, ok := findJestReport(stdout)
+// parseJSCompatReport normalizes a Jest-format `--json` run (Jest or Vitest) into a TestReport. A
+// build/transpile failure (numRuntimeErrorTestSuites, or a failed suite with zero assertions — Vitest's
+// simplified format may omit the former, so the latter is the load-bearing signal there) sets
+// BuildFailed — the assertion-vs-compile distinction the Go convention draws. As with the
+// mutation-report readers, output it cannot decode is an error (the caller fails closed), never an empty
+// report scored as a clean run. A FAILED test is authoritative and is never masked by a same-named pass.
+// runner names the convention in error messages ("typescript"/"vitest").
+func parseJSCompatReport(runner string, code int, stdout string) (TestReport, error) {
+	data, ok := findJSCompatReportLine(stdout)
 	if !ok {
-		return TestReport{}, fmt.Errorf("testconv(typescript): no Jest --json report line in the runner output (exit %d)", code)
+		return TestReport{}, fmt.Errorf("testconv(%s): no Jest-format --json report line in the runner output (exit %d)", runner, code)
 	}
-	var raw jestReport
+	var raw jsCompatReport
 	if err := json.Unmarshal(data, &raw); err != nil {
-		return TestReport{}, fmt.Errorf("testconv(typescript): parsing the runner's --json report: %w", err)
+		return TestReport{}, fmt.Errorf("testconv(%s): parsing the runner's --json report: %w", runner, err)
 	}
 	rep := TestReport{Tests: map[string]Outcome{}}
 	if raw.NumRuntimeErrorTestSuites > 0 {
@@ -109,13 +117,14 @@ func (typeScriptConvention) ParseReport(code int, stdout, stderr string) (TestRe
 	return rep, nil
 }
 
-// findJestReport locates Jest's `--json` report line in the runner output. Jest prints that report as a
-// SINGLE-LINE JSON object to stdout, but its default (pretty) reporter also prints failing-test blocks
-// full of braces to stderr — and the engine feeds combined stdout+stderr here, so a "first `{` to last
-// `}`" span would splice pretty output into the JSON. Instead this scans line by line and returns the
-// one line that is a Jest aggregated result, identified by the always-present `numTotalTestSuites`
-// field (a brace from pretty output never has it). No bespoke parser — the runner's own JSON, isolated.
-func findJestReport(s string) ([]byte, bool) {
+// findJSCompatReportLine locates the Jest-format `--json` report line in the runner output. Jest and
+// Vitest print that report as a SINGLE-LINE JSON object to stdout, but the default (pretty) reporter
+// also prints failing-test blocks full of braces to stderr — and the engine feeds combined stdout+stderr
+// here, so a "first `{` to last `}`" span would splice pretty output into the JSON. Instead this scans
+// line by line and returns the one line that is an aggregated result, identified by the always-present
+// `numTotalTestSuites` field (a brace from pretty output never has it). No bespoke parser — the runner's
+// own JSON, isolated.
+func findJSCompatReportLine(s string) ([]byte, bool) {
 	for _, line := range strings.Split(s, "\n") {
 		line = strings.TrimSpace(line)
 		if !strings.HasPrefix(line, "{") {
diff --git a/internal/testconv/vitest.go b/internal/testconv/vitest.go
new file mode 100644
index 0000000..b443c5a
--- /dev/null
+++ b/internal/testconv/vitest.go
@@ -0,0 +1,42 @@
+package testconv
+
+import "regexp"
+
+func init() { register(vitestConvention{}) }
+
+// vitestConvention supports TS/JS repositories driven by VITEST. It shares everything with the Jest
+// convention except the runner invocation: it embeds typeScriptConvention (so IsTestFile, DirHasTests,
+// DeletesATest, and SemanticallyNull are identical) and overrides only the flags and the report-name.
+//
+// Vitest's results reporter is `--reporter=json` (NOT Jest's bare `--json`), and its output is a
+// Jest-compatible aggregated result — the same `numTotalTestSuites` / `testResults` / `assertionResults`
+// shape, a simplified version of Jest's — so the shared parseJSCompatReport reads it directly
+// (verified against Vitest's reporter docs, 2026-09-01). The base command must be a one-shot run
+// (`vitest run`, or CI mode) — the convention does not add a subcommand, matching how the Jest
+// convention leaves non-interactive invocation to the consent-hashed base command.
+//
+// The json reporter prints to STDOUT by default (Vitest docs: "Can either be printed to the terminal or
+// written to a file using the outputFile configuration option"), which is what ParseReport reads. If a
+// repository hard-wires `outputFile` in its Vitest config, the report goes to that file and stdout is
+// empty — ParseReport then finds no report line and fails CLOSED (unverifiable), never silently wrong.
+// That is the same misconfiguration edge as Jest's `--outputFile`; such a repo must not redirect the
+// json reporter away from stdout in its consent-hashed base command / config.
+type vitestConvention struct{ typeScriptConvention }
+
+func (vitestConvention) Name() string { return "vitest" }
+
+// RunArgs narrows the base Vitest command to the single named test and asks for the JSON reporter. The
+// test identity is the full name, matched by Vitest's `--testNamePattern`, anchored and regexp-quoted.
+func (vitestConvention) RunArgs(base []string, test string) []string {
+	return append(append([]string(nil), base...), "--testNamePattern", "^"+regexp.QuoteMeta(test)+"$", "--reporter=json")
+}
+
+// SuiteArgs runs the whole suite (no name filter) with Vitest's JSON reporter.
+func (vitestConvention) SuiteArgs(base []string) []string {
+	return append(append([]string(nil), base...), "--reporter=json")
+}
+
+// ParseReport reads Vitest's Jest-compatible json report through the shared parser.
+func (vitestConvention) ParseReport(code int, stdout, stderr string) (TestReport, error) {
+	return parseJSCompatReport("vitest", code, stdout)
+}
diff --git a/internal/testconv/vitest_test.go b/internal/testconv/vitest_test.go
new file mode 100644
index 0000000..9dd69d7
--- /dev/null
+++ b/internal/testconv/vitest_test.go
@@ -0,0 +1,62 @@
+package testconv
+
+import "testing"
+
+func TestVitestRunArgs(t *testing.T) {
+	c := vitestConvention{}
+	run := c.RunArgs([]string{"npx", "vitest", "run"}, "Calculator adds")
+	if got := run[len(run)-3:]; got[0] != "--testNamePattern" || got[1] != `^Calculator adds$` || got[2] != "--reporter=json" {
+		t.Fatalf("Vitest RunArgs must use --testNamePattern and --reporter=json, got %v", run)
+	}
+	base := []string{"npx", "vitest", "run"}
+	_ = c.RunArgs(base, "x")
+	if len(base) != 3 {
+		t.Fatal("RunArgs must not mutate the base command")
+	}
+	suite := c.SuiteArgs([]string{"npx", "vitest", "run"})
+	if suite[len(suite)-1] != "--reporter=json" || len(suite) != 4 {
+		t.Fatalf("Vitest SuiteArgs must append only --reporter=json, got %v", suite)
+	}
+}
+
+// Vitest shares IsTestFile/DirHasTests/DeletesATest/SemanticallyNull with the TS convention (embedded);
+// a spot check confirms the promotion.
+func TestVitestSharesTSPredicates(t *testing.T) {
+	c := vitestConvention{}
+	if !c.IsTestFile("a/b.spec.ts") || c.IsTestFile("a/b.ts") {
+		t.Fatal("Vitest must inherit the TS test-file rule")
+	}
+	if c.SemanticallyNull("x", "x") {
+		t.Fatal("Vitest must inherit the dependency-free (false) trivial-pin pre-screen")
+	}
+}
+
+func TestVitestParseReport(t *testing.T) {
+	c := vitestConvention{}
+	must := func(out string) TestReport {
+		r, err := c.ParseReport(1, out, "")
+		if err != nil {
+			t.Fatalf("ParseReport error: %v", err)
+		}
+		return r
+	}
+	// Vitest's Jest-compatible json (note: its simplified format may OMIT numRuntimeErrorTestSuites — the
+	// failed-suite-with-no-assertions fallback is what catches a transpile failure there).
+	vitestPass := `{"numTotalTestSuites":1,"testResults":[{"status":"passed","assertionResults":[{"fullName":"Calculator adds","status":"passed"}]}]}`
+	if Classify(must(vitestPass), "Calculator adds") != ClsPassed {
+		t.Fatal("a Vitest passed assertion -> ClsPassed")
+	}
+	vitestFail := `{"numTotalTestSuites":1,"testResults":[{"status":"failed","assertionResults":[{"ancestorTitles":["Calculator"],"title":"adds","status":"failed"}]}]}`
+	if Classify(must(vitestFail), "Calculator adds") != ClsAssert {
+		t.Fatal("a Vitest failed assertion -> ClsAssert")
+	}
+	// A suite that failed to load with no assertions -> BuildFailed even without numRuntimeErrorTestSuites.
+	vitestTranspile := `{"numTotalTestSuites":1,"testResults":[{"status":"failed","message":"Error: cannot resolve","assertionResults":[]}]}`
+	if Classify(must(vitestTranspile), "Calculator adds") != ClsCompile {
+		t.Fatal("a Vitest failed suite with no assertions -> ClsCompile")
+	}
+	// Same fail-closed contract as Jest: no report line is an error.
+	if _, err := c.ParseReport(1, "no report here\n", ""); err == nil {
+		t.Fatal("output with no report line must be an error")
+	}
+}



```

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

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

