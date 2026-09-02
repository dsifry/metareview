# metareview task-done context

Run ID: `mrv-20260902-051859934731000-task-done-typescript-4dea666f`

## Task

package testconv

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

func init() { register(typeScriptConvention{}) }

// typeScriptConvention supports TypeScript/JavaScript repositories driven by JEST. Like the Go
// convention it reads the runner's OWN structured output — Jest `--json` — and normalizes it; it parses
// no source. It carries no state.
//
// It is scoped to Jest on purpose: Vitest's flags and output differ (its results reporter is
// `--reporter=json`, not the bare `--json` Jest uses), so a correct Vitest convention is a separate
// registration with its own report fixtures — a follow-up, not a claim made here. A Vitest repo that
// selected this convention would run the wrong flag and fail closed (ParseReport finds no Jest report),
// never silently wrong.
type typeScriptConvention struct{}

func (typeScriptConvention) Name() string { return "typescript" }

// testFileSuffixes are the conventional TS/JS test-file endings (shared across Jest and Vitest).
var testFileSuffixes = []string{
	".test.ts", ".test.tsx", ".test.js", ".test.jsx", ".test.mjs", ".test.cjs", ".test.mts", ".test.cts",
	".spec.ts", ".spec.tsx", ".spec.js", ".spec.jsx", ".spec.mjs", ".spec.cjs", ".spec.mts", ".spec.cts",
}

// IsTestFile reports whether path ends in a `*.test.*` / `*.spec.*` suffix.
func (typeScriptConvention) IsTestFile(path string) bool {
	for _, s := range testFileSuffixes {
		if strings.HasSuffix(path, s) {
			return true
		}
	}
	return false
}

// RunArgs narrows the base Jest command to the single named test and asks for the JSON report. The test
// identity is the full `describe > it` name (space-joined), matched by Jest's `--testNamePattern`; it is
// anchored and regexp-quoted so it selects one test and cannot inject a flag.
func (typeScriptConvention) RunArgs(base []string, test string) []string {
	return append(append([]string(nil), base...), "--testNamePattern", "^"+regexp.QuoteMeta(test)+"$", "--json")
}

// SuiteArgs runs the whole suite (no name filter) with Jest's JSON report.
func (typeScriptConvention) SuiteArgs(base []string) []string {
	return append(append([]string(nil), base...), "--json")
}

// ParseReport normalizes a Jest `--json` run (see parseJSCompatReport).
func (typeScriptConvention) ParseReport(code int, stdout, stderr string) (TestReport, error) {
	return parseJSCompatReport("typescript", code, stdout)
}

// jsCompatReport is the subset of the Jest `--json` aggregated result this reads. Vitest's `json`
// reporter emits the SAME (Jest-compatible) shape, so both conventions share this parser. Each
// assertionResult is one test (`fullName` = ancestorTitles + title); a testResult with no assertions
// but a failed status is a suite that failed to transpile/execute — a build failure, not an assertion.
type jsCompatReport struct {
	NumRuntimeErrorTestSuites int `json:"numRuntimeErrorTestSuites"`
	TestResults               []struct {
		Status           string `json:"status"`
		AssertionResults []struct {
			FullName       string   `json:"fullName"`
			Title          string   `json:"title"`
			AncestorTitles []string `json:"ancestorTitles"`
			Status         string   `json:"status"`
		} `json:"assertionResults"`
	} `json:"testResults"`
}

// parseJSCompatReport normalizes a Jest-format `--json` run (Jest or Vitest) into a TestReport. A
// build/transpile failure (numRuntimeErrorTestSuites, or a failed suite with zero assertions — Vitest's
// simplified format may omit the former, so the latter is the load-bearing signal there) sets
// BuildFailed — the assertion-vs-compile distinction the Go convention draws. As with the
// mutation-report readers, output it cannot decode is an error (the caller fails closed), never an empty
// report scored as a clean run. A FAILED test is authoritative and is never masked by a same-named pass.
// runner names the convention in error messages ("typescript"/"vitest").
func parseJSCompatReport(runner string, code int, stdout string) (TestReport, error) {
	data, ok := findJSCompatReportLine(stdout)
	if !ok {
		return TestReport{}, fmt.Errorf("testconv(%s): no Jest-format --json report line in the runner output (exit %d)", runner, code)
	}
	var raw jsCompatReport
	if err := json.Unmarshal(data, &raw); err != nil {
		return TestReport{}, fmt.Errorf("testconv(%s): parsing the runner's --json report: %w", runner, err)
	}
	rep := TestReport{Tests: map[string]Outcome{}}
	if raw.NumRuntimeErrorTestSuites > 0 {
		rep.BuildFailed = true
	}
	for _, tr := range raw.TestResults {
		if tr.Status == "failed" && len(tr.AssertionResults) == 0 {
			rep.BuildFailed = true // a suite that failed to run at all (transpile/collection error)
		}
		for _, ar := range tr.AssertionResults {
			name := ar.FullName
			if name == "" {
				name = strings.TrimSpace(strings.Join(ar.AncestorTitles, " ") + " " + ar.Title)
			}
			switch ar.Status {
			case "failed":
				rep.Tests[name] = Failed
			case "passed":
				if rep.Tests[name] != Failed {
					rep.Tests[name] = Passed
				}
			}
			// A status that did not actually execute (pending, skipped, or not-yet-implemented) leaves
			// the test absent from the report.
		}
	}
	return rep, nil
}

// findJSCompatReportLine locates the Jest-format `--json` report line in the runner output. Jest and
// Vitest print that report as a SINGLE-LINE JSON object to stdout, but the default (pretty) reporter
// also prints failing-test blocks full of braces to stderr — and the engine feeds combined stdout+stderr
// here, so a "first `{` to last `}`" span would splice pretty output into the JSON. Instead this scans
// line by line and returns the one line that is an aggregated result, identified by the always-present
// `numTotalTestSuites` field (a brace from pretty output never has it). No bespoke parser — the runner's
// own JSON, isolated.
func findJSCompatReportLine(s string) ([]byte, bool) {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var probe struct {
			N *int `json:"numTotalTestSuites"`
		}
		if json.Unmarshal([]byte(line), &probe) == nil && probe.N != nil {
			return []byte(line), true
		}
	}
	return nil, false
}

// DeletesATest reports whether the diff removes test content from a TS/JS test file. TypeScript has no
// single-line "test declaration" rule (a test is `it(...)`/`test(...)`, and those are aliasable), so
// rather than parse test syntax (no bespoke parser, no dependency — Dave, 2026-09-01) this is a
// conservative structural signal: a removed content line inside a hunk of a file whose path is a test
// file. It over-triggers on any test-file line removal, which is SAFE — the §9.6 gate then re-checks
// proven pins and finds no regression when none exists; the cost is a recheck, never a missed one.
func (c typeScriptConvention) DeletesATest(diff string) bool {
	inTestFile, inHunk := false, false
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			inTestFile = c.diffGitTouchesTest(line)
			inHunk = false
		case strings.HasPrefix(line, "@@"):
			inHunk = true
		case strings.HasPrefix(line, "---"), strings.HasPrefix(line, "+++"):
			// file headers, not content
		case inHunk && inTestFile && strings.HasPrefix(line, "-"):
			return true
		}
	}
	return false
}

// diffGitTouchesTest reports whether either path on a `diff --git a/… b/…` line is a test file (so a
// deleted test file, whose new side is /dev/null, is still recognized by its old path).
func (c typeScriptConvention) diffGitTouchesTest(line string) bool {
	fields := strings.Fields(strings.TrimPrefix(line, "diff --git "))
	for _, f := range fields {
		f = strings.TrimPrefix(strings.TrimPrefix(f, "a/"), "b/")
		if c.IsTestFile(f) {
			return true
		}
	}
	return false
}

// DirHasTests reports whether the directory holding file contains any TS/JS test file.
func (c typeScriptConvention) DirHasTests(root, file string) bool {
	dir := filepath.Join(root, filepath.Dir(file))
	for _, s := range testFileSuffixes {
		if matches, err := filepath.Glob(filepath.Join(dir, "*"+s)); err == nil && len(matches) > 0 {
			return true
		}
	}
	return false
}

// SemanticallyNull always reports false: there is no standard-library JS/TS tokenizer, and the seam is
// deliberately dependency-free (Dave, 2026-09-01), so the trivial-pin pre-screen is skipped for TS. The
// full compile-then-break steps still run — the safe direction, at worst one wasted mutation.
func (typeScriptConvention) SemanticallyNull(orig, mutated string) bool { return false }


## Git

- Base: `846d0ea1da8134f4e35e8cb658cb6f8ef9c702a3`
- Head: `afd0a6129e27e813e7ecc246b30a6d894062d62f`
- Branch: `testconv-mjs-cjs`
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `1463`
- Filtered diff bytes: `1463`
- Risk level: `none`

## Context Shard Plan

Not sharded.

## Review Manifest

- Manifest verdict: `PASS`
- Source manifest hash: not sharded
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- internal/testconv/typescript.go
- internal/testconv/typescript_test.go

### Manifest Blockers
No manifest blockers.

## Changed Files

- internal/testconv/typescript.go
- internal/testconv/typescript_test.go

## Diff

```diff
diff --git a/internal/testconv/typescript.go b/internal/testconv/typescript.go
index 58ef523..c201e80 100644
--- a/internal/testconv/typescript.go
+++ b/internal/testconv/typescript.go
@@ -25,8 +25,8 @@ func (typeScriptConvention) Name() string { return "typescript" }
 
 // testFileSuffixes are the conventional TS/JS test-file endings (shared across Jest and Vitest).
 var testFileSuffixes = []string{
-	".test.ts", ".test.tsx", ".test.js", ".test.jsx", ".test.mts", ".test.cts",
-	".spec.ts", ".spec.tsx", ".spec.js", ".spec.jsx", ".spec.mts", ".spec.cts",
+	".test.ts", ".test.tsx", ".test.js", ".test.jsx", ".test.mjs", ".test.cjs", ".test.mts", ".test.cts",
+	".spec.ts", ".spec.tsx", ".spec.js", ".spec.jsx", ".spec.mjs", ".spec.cjs", ".spec.mts", ".spec.cts",
 }
 
 // IsTestFile reports whether path ends in a `*.test.*` / `*.spec.*` suffix.
diff --git a/internal/testconv/typescript_test.go b/internal/testconv/typescript_test.go
index 04ea4b1..0d46efc 100644
--- a/internal/testconv/typescript_test.go
+++ b/internal/testconv/typescript_test.go
@@ -8,7 +8,7 @@ import (
 
 func TestTSBasics(t *testing.T) {
 	c := typeScriptConvention{}
-	for _, f := range []string{"src/calc.test.ts", "a/b.spec.tsx", "x.test.js", "y.spec.jsx", "m.test.mts"} {
+	for _, f := range []string{"src/calc.test.ts", "a/b.spec.tsx", "x.test.js", "y.spec.jsx", "m.test.mts", "e.test.mjs", "c.spec.cjs"} {
 		if !c.IsTestFile(f) {
 			t.Fatalf("%s must be a test file", f)
 		}



```

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

{"schemaVersion":1,"kind":"validation","command":["go","test","./internal/testconv/"],"cwd":".","exitCode":0,"startedAt":"2026-09-02T05:18:59.845699Z","finishedAt":"2026-09-02T05:18:59.926721Z","stdoutSha256":"fc0b56768ed3f840beef0f981d0f9c4f383ff53c7e99186b5a8f51072edef17d","stderrSha256":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","summary":"go test ./internal/testconv/ exited 0"}

