# metareview task-done context

Run ID: `mrv-20260901-172216437441000-task-done-multilang-typescript-conv-6cdb50c2`

## Task

Advisory task target: multilang-typescript-conv

## Git

- Base: `623a2557d4ea11443891026354e659597cdd251e`
- Head: `d55378fb4d1014fcdadfefd4c65b62ff9f1fa1a4`
- Branch: `multilang-typescript-conv`
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `46168`
- Filtered diff bytes: `16532`
- Risk level: `none`
- Generated files excluded: docs/metareview/context/mrv-20260901-171104038410000-task-done-multilang-typescript-conv-6cdb50c2-context.md, docs/metareview/evidence/multilang-typescript-conv-pr-b.md, docs/metareview/reviews/mrv-20260901-171104038410000-task-done-multilang-typescript-conv-6cdb50c2.md

## Context Shard Plan

Not sharded.

## Review Manifest

- Manifest verdict: `PASS`
- Source manifest hash: not sharded
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- internal/testconv/testconv_test.go
- internal/testconv/typescript.go
- internal/testconv/typescript_test.go

### Path Dispositions
- docs/metareview/context/mrv-20260901-171104038410000-task-done-multilang-typescript-conv-6cdb50c2-context.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/evidence/multilang-typescript-conv-pr-b.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/reviews/mrv-20260901-171104038410000-task-done-multilang-typescript-conv-6cdb50c2.md: generated (metareview generated review artifact excluded from source manifest)

### Manifest Blockers
No manifest blockers.

## Changed Files

- internal/testconv/testconv_test.go
- internal/testconv/typescript.go
- internal/testconv/typescript_test.go

## Diff

```diff
diff --git a/internal/testconv/testconv_test.go b/internal/testconv/testconv_test.go
index 284f2c2..fbd1abe 100644
--- a/internal/testconv/testconv_test.go
+++ b/internal/testconv/testconv_test.go
@@ -11,9 +11,12 @@ func TestForSelectsAndFailsClosed(t *testing.T) {
 	if !ok || c == nil || c.Name() != "go" {
 		t.Fatalf("For(go) must return the Go convention, got %v ok=%v", c, ok)
 	}
+	if c, ok := For("typescript"); !ok || c == nil || c.Name() != "typescript" {
+		t.Fatalf("For(typescript) must return the TS convention, got %v ok=%v", c, ok)
+	}
 	// The load-bearing negative: an unknown or empty name returns (nil,false) so the caller fails
-	// closed — never a silent default to Go.
-	if c, ok := For("typescript"); ok || c != nil {
+	// closed — never a silent default to a language.
+	if c, ok := For("cobol"); ok || c != nil {
 		t.Fatal("an unregistered convention must return (nil,false)")
 	}
 	if c, ok := For(""); ok || c != nil {
diff --git a/internal/testconv/typescript.go b/internal/testconv/typescript.go
new file mode 100644
index 0000000..9a37995
--- /dev/null
+++ b/internal/testconv/typescript.go
@@ -0,0 +1,185 @@
+package testconv
+
+import (
+	"encoding/json"
+	"fmt"
+	"path/filepath"
+	"regexp"
+	"strings"
+)
+
+func init() { register(typeScriptConvention{}) }
+
+// typeScriptConvention supports TypeScript/JavaScript repositories driven by JEST. Like the Go
+// convention it reads the runner's OWN structured output — Jest `--json` — and normalizes it; it parses
+// no source. It carries no state.
+//
+// It is scoped to Jest on purpose: Vitest's flags and output differ (its results reporter is
+// `--reporter=json`, not the bare `--json` Jest uses), so a correct Vitest convention is a separate
+// registration with its own report fixtures — a follow-up, not a claim made here. A Vitest repo that
+// selected this convention would run the wrong flag and fail closed (ParseReport finds no Jest report),
+// never silently wrong.
+type typeScriptConvention struct{}
+
+func (typeScriptConvention) Name() string { return "typescript" }
+
+// testFileSuffixes are the conventional TS/JS test-file endings (shared across Jest and Vitest).
+var testFileSuffixes = []string{
+	".test.ts", ".test.tsx", ".test.js", ".test.jsx", ".test.mts", ".test.cts",
+	".spec.ts", ".spec.tsx", ".spec.js", ".spec.jsx", ".spec.mts", ".spec.cts",
+}
+
+// IsTestFile reports whether path ends in a `*.test.*` / `*.spec.*` suffix.
+func (typeScriptConvention) IsTestFile(path string) bool {
+	for _, s := range testFileSuffixes {
+		if strings.HasSuffix(path, s) {
+			return true
+		}
+	}
+	return false
+}
+
+// RunArgs narrows the base Jest command to the single named test and asks for the JSON report. The test
+// identity is the full `describe > it` name (space-joined), matched by Jest's `--testNamePattern`; it is
+// anchored and regexp-quoted so it selects one test and cannot inject a flag.
+func (typeScriptConvention) RunArgs(base []string, test string) []string {
+	return append(append([]string(nil), base...), "--testNamePattern", "^"+regexp.QuoteMeta(test)+"$", "--json")
+}
+
+// SuiteArgs runs the whole suite (no name filter) with Jest's JSON report.
+func (typeScriptConvention) SuiteArgs(base []string) []string {
+	return append(append([]string(nil), base...), "--json")
+}
+
+// jestReport is the subset of Jest's `--json` output this reads. Each assertionResult is one test
+// (`fullName` = ancestorTitles + title); a testResult with no assertions but a failed status is a suite
+// that failed to transpile/execute — a build failure, not an assertion.
+type jestReport struct {
+	NumRuntimeErrorTestSuites int `json:"numRuntimeErrorTestSuites"`
+	TestResults               []struct {
+		Status           string `json:"status"`
+		AssertionResults []struct {
+			FullName       string   `json:"fullName"`
+			Title          string   `json:"title"`
+			AncestorTitles []string `json:"ancestorTitles"`
+			Status         string   `json:"status"`
+		} `json:"assertionResults"`
+	} `json:"testResults"`
+}
+
+// ParseReport normalizes a Jest `--json` run. A build/transpile failure (a suite Jest could not
+// run: numRuntimeErrorTestSuites, or a failed suite with zero assertions) sets BuildFailed — the same
+// assertion-vs-compile distinction the Go convention draws. As with the mutation-report readers, output
+// it cannot decode is an error (the caller fails closed), never an empty report scored as a clean run.
+// A FAILED test is authoritative and is never masked by a same-named pass (matching the Go convention).
+func (typeScriptConvention) ParseReport(code int, stdout, stderr string) (TestReport, error) {
+	data, ok := findJestReport(stdout)
+	if !ok {
+		return TestReport{}, fmt.Errorf("testconv(typescript): no Jest --json report line in the runner output (exit %d)", code)
+	}
+	var raw jestReport
+	if err := json.Unmarshal(data, &raw); err != nil {
+		return TestReport{}, fmt.Errorf("testconv(typescript): parsing the runner's --json report: %w", err)
+	}
+	rep := TestReport{Tests: map[string]Outcome{}}
+	if raw.NumRuntimeErrorTestSuites > 0 {
+		rep.BuildFailed = true
+	}
+	for _, tr := range raw.TestResults {
+		if tr.Status == "failed" && len(tr.AssertionResults) == 0 {
+			rep.BuildFailed = true // a suite that failed to run at all (transpile/collection error)
+		}
+		for _, ar := range tr.AssertionResults {
+			name := ar.FullName
+			if name == "" {
+				name = strings.TrimSpace(strings.Join(ar.AncestorTitles, " ") + " " + ar.Title)
+			}
+			switch ar.Status {
+			case "failed":
+				rep.Tests[name] = Failed
+			case "passed":
+				if rep.Tests[name] != Failed {
+					rep.Tests[name] = Passed
+				}
+			}
+			// A status that did not actually execute (pending, skipped, or not-yet-implemented) leaves
+			// the test absent from the report.
+		}
+	}
+	return rep, nil
+}
+
+// findJestReport locates Jest's `--json` report line in the runner output. Jest prints that report as a
+// SINGLE-LINE JSON object to stdout, but its default (pretty) reporter also prints failing-test blocks
+// full of braces to stderr — and the engine feeds combined stdout+stderr here, so a "first `{` to last
+// `}`" span would splice pretty output into the JSON. Instead this scans line by line and returns the
+// one line that is a Jest aggregated result, identified by the always-present `numTotalTestSuites`
+// field (a brace from pretty output never has it). No bespoke parser — the runner's own JSON, isolated.
+func findJestReport(s string) ([]byte, bool) {
+	for _, line := range strings.Split(s, "\n") {
+		line = strings.TrimSpace(line)
+		if !strings.HasPrefix(line, "{") {
+			continue
+		}
+		var probe struct {
+			N *int `json:"numTotalTestSuites"`
+		}
+		if json.Unmarshal([]byte(line), &probe) == nil && probe.N != nil {
+			return []byte(line), true
+		}
+	}
+	return nil, false
+}
+
+// DeletesATest reports whether the diff removes test content from a TS/JS test file. TypeScript has no
+// single-line "test declaration" rule (a test is `it(...)`/`test(...)`, and those are aliasable), so
+// rather than parse test syntax (no bespoke parser, no dependency — Dave, 2026-09-01) this is a
+// conservative structural signal: a removed content line inside a hunk of a file whose path is a test
+// file. It over-triggers on any test-file line removal, which is SAFE — the §9.6 gate then re-checks
+// proven pins and finds no regression when none exists; the cost is a recheck, never a missed one.
+func (c typeScriptConvention) DeletesATest(diff string) bool {
+	inTestFile, inHunk := false, false
+	for _, line := range strings.Split(diff, "\n") {
+		switch {
+		case strings.HasPrefix(line, "diff --git "):
+			inTestFile = c.diffGitTouchesTest(line)
+			inHunk = false
+		case strings.HasPrefix(line, "@@"):
+			inHunk = true
+		case strings.HasPrefix(line, "---"), strings.HasPrefix(line, "+++"):
+			// file headers, not content
+		case inHunk && inTestFile && strings.HasPrefix(line, "-"):
+			return true
+		}
+	}
+	return false
+}
+
+// diffGitTouchesTest reports whether either path on a `diff --git a/… b/…` line is a test file (so a
+// deleted test file, whose new side is /dev/null, is still recognized by its old path).
+func (c typeScriptConvention) diffGitTouchesTest(line string) bool {
+	fields := strings.Fields(strings.TrimPrefix(line, "diff --git "))
+	for _, f := range fields {
+		f = strings.TrimPrefix(strings.TrimPrefix(f, "a/"), "b/")
+		if c.IsTestFile(f) {
+			return true
+		}
+	}
+	return false
+}
+
+// DirHasTests reports whether the directory holding file contains any TS/JS test file.
+func (c typeScriptConvention) DirHasTests(root, file string) bool {
+	dir := filepath.Join(root, filepath.Dir(file))
+	for _, s := range testFileSuffixes {
+		if matches, err := filepath.Glob(filepath.Join(dir, "*"+s)); err == nil && len(matches) > 0 {
+			return true
+		}
+	}
+	return false
+}
+
+// SemanticallyNull always reports false: there is no standard-library JS/TS tokenizer, and the seam is
+// deliberately dependency-free (Dave, 2026-09-01), so the trivial-pin pre-screen is skipped for TS. The
+// full compile-then-break steps still run — the safe direction, at worst one wasted mutation.
+func (typeScriptConvention) SemanticallyNull(orig, mutated string) bool { return false }
diff --git a/internal/testconv/typescript_test.go b/internal/testconv/typescript_test.go
new file mode 100644
index 0000000..04ea4b1
--- /dev/null
+++ b/internal/testconv/typescript_test.go
@@ -0,0 +1,150 @@
+package testconv
+
+import (
+	"os"
+	"path/filepath"
+	"testing"
+)
+
+func TestTSBasics(t *testing.T) {
+	c := typeScriptConvention{}
+	for _, f := range []string{"src/calc.test.ts", "a/b.spec.tsx", "x.test.js", "y.spec.jsx", "m.test.mts"} {
+		if !c.IsTestFile(f) {
+			t.Fatalf("%s must be a test file", f)
+		}
+	}
+	for _, f := range []string{"src/calc.ts", "a/b.tsx", "notes.md", "test.ts"} {
+		if c.IsTestFile(f) {
+			t.Fatalf("%s must NOT be a test file", f)
+		}
+	}
+	run := c.RunArgs([]string{"npx", "jest"}, "Calculator adds")
+	if got := run[len(run)-3:]; got[0] != "--testNamePattern" || got[1] != `^Calculator adds$` || got[2] != "--json" {
+		t.Fatalf("RunArgs must anchor --testNamePattern and request --json, got %v", run)
+	}
+	base := []string{"npx", "jest"}
+	_ = c.RunArgs(base, "x")
+	if len(base) != 2 {
+		t.Fatal("RunArgs must not mutate the base command")
+	}
+	suite := c.SuiteArgs([]string{"npx", "jest"})
+	if suite[len(suite)-1] != "--json" || len(suite) != 3 {
+		t.Fatalf("SuiteArgs must append only --json, got %v", suite)
+	}
+}
+
+// Fixtures carry numTotalTestSuites — the sentinel findJestReport requires to distinguish the real
+// report line from pretty-reporter braces in the combined output.
+const (
+	jestPass = `{"numTotalTestSuites":1,"numRuntimeErrorTestSuites":0,"testResults":[{"status":"passed","assertionResults":[{"fullName":"Calculator adds","status":"passed"}]}]}`
+	jestFail = `{"numTotalTestSuites":1,"numRuntimeErrorTestSuites":0,"testResults":[{"status":"failed","assertionResults":[{"ancestorTitles":["Calculator"],"title":"adds","status":"failed"}]}]}`
+	// A suite that failed to transpile/collect: a failed testResult with no assertions.
+	jestTranspile = `{"numTotalTestSuites":1,"numRuntimeErrorTestSuites":0,"testResults":[{"status":"failed","message":"SyntaxError: x","assertionResults":[]}]}`
+	// numRuntimeErrorTestSuites marks a suite that errored at collection time.
+	jestRuntimeErr = `{"numTotalTestSuites":1,"numRuntimeErrorTestSuites":1,"testResults":[]}`
+)
+
+func TestTSParseReport(t *testing.T) {
+	c := typeScriptConvention{}
+	must := func(code int, out string) TestReport {
+		r, err := c.ParseReport(code, out, "")
+		if err != nil {
+			t.Fatalf("ParseReport error: %v", err)
+		}
+		return r
+	}
+	if Classify(must(0, jestPass), "Calculator adds") != ClsPassed {
+		t.Fatal("a passed assertion -> ClsPassed")
+	}
+	// fullName absent -> reconstructed from ancestorTitles + title.
+	if Classify(must(1, jestFail), "Calculator adds") != ClsAssert {
+		t.Fatal("a failed assertion -> ClsAssert (name reconstructed)")
+	}
+	if Classify(must(1, jestTranspile), "Calculator adds") != ClsCompile {
+		t.Fatal("a failed suite with no assertions -> ClsCompile")
+	}
+	if Classify(must(1, jestRuntimeErr), "Calculator adds") != ClsCompile {
+		t.Fatal("numRuntimeErrorTestSuites>0 -> ClsCompile")
+	}
+	// A leading console.log before the JSON is tolerated (first { .. last }).
+	if Classify(must(0, "console noise\n"+jestPass+"\ntrailing\n"), "Calculator adds") != ClsPassed {
+		t.Fatal("a JSON report preceded by log noise must still parse")
+	}
+	// A FAILED test must never be masked by a same-named pass in another suite.
+	mixed := `{"numTotalTestSuites":2,"testResults":[{"status":"failed","assertionResults":[{"fullName":"T","status":"failed"}]},{"status":"passed","assertionResults":[{"fullName":"T","status":"passed"}]}]}`
+	if Classify(must(1, mixed), "T") != ClsAssert {
+		t.Fatal("a same-named pass must not mask a failure")
+	}
+	// Pretty-reporter braces on stderr (merged into the stream) must NOT be mistaken for the report:
+	// the report line is found by its numTotalTestSuites sentinel, not by brace position.
+	pretty := "  ● Calculator › adds\n    expect(received).toBe(expected)\n    { a: 1 }\n" + jestFail + "\n"
+	if Classify(must(1, pretty), "Calculator adds") != ClsAssert {
+		t.Fatal("pretty-reporter braces must not corrupt report extraction")
+	}
+}
+
+func TestTSParseReportUnreadable(t *testing.T) {
+	c := typeScriptConvention{}
+	// No line carries the Jest report sentinel → no report found.
+	if _, err := c.ParseReport(1, "no json here at all\n{ \"other\": 1 }\n", ""); err == nil {
+		t.Fatal("output with no Jest report line must be an error")
+	}
+	// A line WITH the sentinel but a type-mismatched body → the Unmarshal error path.
+	if _, err := c.ParseReport(1, `{"numTotalTestSuites":1,"testResults":"not-an-array"}`, ""); err == nil {
+		t.Fatal("a report line that does not decode must be an error")
+	}
+}
+
+func TestTSDeletesATest(t *testing.T) {
+	c := typeScriptConvention{}
+	// A removed line inside a test file's hunk.
+	del := "diff --git a/calc.test.ts b/calc.test.ts\n--- a/calc.test.ts\n+++ b/calc.test.ts\n@@ -1,3 +1,2 @@\n it('x',()=>{})\n-it('gone',()=>{})\n"
+	if !c.DeletesATest(del) {
+		t.Fatal("a removed line in a test file must trigger the gate")
+	}
+	// A deleted test FILE (new side /dev/null) is recognized by its old path.
+	delFile := "diff --git a/a.spec.ts b/a.spec.ts\n--- a/a.spec.ts\n+++ /dev/null\n@@ -1,1 +0,0 @@\n-it('x',()=>{})\n"
+	if !c.DeletesATest(delFile) {
+		t.Fatal("a deleted test file must trigger the gate")
+	}
+	// A removal in a NON-test file must not trigger.
+	prod := "diff --git a/calc.ts b/calc.ts\n--- a/calc.ts\n+++ b/calc.ts\n@@ -1,2 +1,1 @@\n-const gone = 1\n"
+	if c.DeletesATest(prod) {
+		t.Fatal("a removal in a production file must not trigger the TS test-deletion gate")
+	}
+	// A pure ADDITION to a test file (no removed content line) must not trigger.
+	add := "diff --git a/calc.test.ts b/calc.test.ts\n--- a/calc.test.ts\n+++ b/calc.test.ts\n@@ -1,1 +1,2 @@\n it('x',()=>{})\n+it('new',()=>{})\n"
+	if c.DeletesATest(add) {
+		t.Fatal("an addition-only test-file change must not trigger the gate")
+	}
+	// The --- header of a test file must not itself be read as a removed content line.
+	headerOnly := "diff --git a/calc.test.ts b/calc.test.ts\n--- a/calc.test.ts\n+++ b/calc.test.ts\n@@ -1,1 +1,1 @@\n unchanged\n"
+	if c.DeletesATest(headerOnly) {
+		t.Fatal("a test-file diff with no removed content line must not trigger")
+	}
+}
+
+func TestTSDirHasTests(t *testing.T) {
+	c := typeScriptConvention{}
+	root := t.TempDir()
+	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
+		t.Fatal(err)
+	}
+	if c.DirHasTests(root, "src/calc.ts") {
+		t.Fatal("a dir with no test file has no tests")
+	}
+	if err := os.WriteFile(filepath.Join(root, "src", "calc.spec.ts"), []byte(""), 0o644); err != nil {
+		t.Fatal(err)
+	}
+	if !c.DirHasTests(root, "src/calc.ts") {
+		t.Fatal("a dir with a *.spec.ts has tests")
+	}
+}
+
+func TestTSSemanticallyNullIsAlwaysFalse(t *testing.T) {
+	// Dependency-free: no stdlib JS tokenizer, so the pre-screen is skipped (the safe direction).
+	c := typeScriptConvention{}
+	if c.SemanticallyNull("const x = 1", "const x = 1") {
+		t.Fatal("TS SemanticallyNull must be conservatively false, even for identical input")
+	}
+}



```

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

# Evidence — PR-B: the TypeScript convention (Jest/Vitest `--json`)

**Task:** multi-language support, PR-B — add `TypeScriptConvention` on the seam PR-A landed (#48).
**Design:** `docs/handoffs/handoff-2026-09-01-multilang-testconvention-seam.md`. **Decision (Dave,
2026-09-01):** each runner's OWN structured output, **no bespoke parser, no new dependency**. **Base:**
`main` (`623a255`).

## What landed (`internal/testconv/typescript.go`)

A `typeScriptConvention` registered as `"typescript"`, implementing the seam entirely on Jest/Vitest's
own machine-readable report — no source parsing, no regex over test syntax, no dependency:

- **`IsTestFile` / `DirHasTests`** — the `*.test.*` / `*.spec.*` suffixes (`ts`/`tsx`/`js`/`jsx`/`mts`/`cts`).
- **`RunArgs` / `SuiteArgs`** — `--testNamePattern ^<name>$ --json` (one test) / `--json` (whole suite),
  **Jest's** flags. Test identity is the full `describe > it` name (space-joined), anchored and quoted so
  it selects one test and cannot inject a flag. **Scoped to Jest on purpose** (see round 2): Vitest's
  results reporter is `--reporter=json`, not bare `--json`, so a correct Vitest convention is a separate
  registration — a follow-up, not claimed here.
- **`ParseReport`** — normalizes Jest `--json` into the shared `TestReport`: each `assertionResult`
  (`fullName`, or `ancestorTitles + title`) → `Passed`/`Failed`; a suite Jest could not run
  (`numRuntimeErrorTestSuites > 0`, or a failed suite with zero assertions — a transpile/collection
  error) → `BuildFailed`. Classification then uses the **same generic `Classify`** as Go. A `Failed`
  result **sticks** — a same-named test in another suite can never mask a failure (matching the Go
  convention's fix). The report line is found by its always-present `numTotalTestSuites` sentinel, so
  pretty-reporter braces merged in from stderr can't corrupt extraction (see round 2); output with no
  such report line, or one that does not decode, is an error (fail closed), never an empty report scored
  clean.
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

## Shepherding round 2 (Cursor Bugbot — 2× High, fixed)

- **JSON extractor broke on failing tests.** `runTest` feeds combined stdout+stderr, and Jest's default
  pretty reporter prints brace-laden failure blocks to stderr; the original "first `{` to last `}`"
  span spliced those into the JSON. Fixed: `findJestReport` scans line by line and returns the one line
  that is a Jest aggregated result, identified by the always-present `numTotalTestSuites` field — a
  pretty-output brace never has it. Regression test added (pretty braces before the report).
- **Overclaimed Vitest.** `RunArgs`/`SuiteArgs` emit Jest's `--json`; Vitest's results reporter is
  `--reporter=json` (bare `--json` is a different flag) and its report differs. Fixed by **scoping the
  convention to Jest** explicitly (docs + comments); a Vitest repo selecting it runs the wrong flag and
  fails closed (no Jest report line → unverifiable), never silently wrong. A dedicated Vitest convention
  is a follow-up with its own report fixtures.

Both fixed with tests; `internal/testconv` stays 100%; the `numTotalTestSuites` sentinel gate is
mutation-verified.

## Follow-ups (deferred)

- **Python (pytest)** next, then Java/Ruby/Rust as demand shows — each a new convention on the same seam.
- A TS trivial-pin pre-screen would need a pluggable JS/TS tokenizer — a separately-justified step, not
  a dependency added here.
- End-to-end execution against a real Jest/Vitest project is out of scope for a Go unit suite (no JS
  toolchain in this repo); `ParseReport` is tested against captured report shapes, the same way the Go
  convention is unit-tested against `go test -json` fixtures.

