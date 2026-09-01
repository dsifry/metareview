package testconv

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

func init() { register(typeScriptConvention{}) }

// typeScriptConvention supports TypeScript/JavaScript repositories driven by Jest or Vitest. Like the
// Go convention it reads the runner's OWN structured output — Jest `--json` (Vitest's `--reporter=json`
// is the same shape) — and normalizes it; it parses no source. It carries no state.
type typeScriptConvention struct{}

func (typeScriptConvention) Name() string { return "typescript" }

// testFileSuffixes are the Jest/Vitest conventional test-file endings.
var testFileSuffixes = []string{
	".test.ts", ".test.tsx", ".test.js", ".test.jsx", ".test.mts", ".test.cts",
	".spec.ts", ".spec.tsx", ".spec.js", ".spec.jsx", ".spec.mts", ".spec.cts",
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

// RunArgs narrows the base runner command to the single named test and asks for the JSON report. The
// test identity is the full `describe > it` name (space-joined), matched by Jest/Vitest
// `--testNamePattern`; it is anchored and regexp-quoted so it selects one test and cannot inject a flag.
func (typeScriptConvention) RunArgs(base []string, test string) []string {
	return append(append([]string(nil), base...), "--testNamePattern", "^"+regexp.QuoteMeta(test)+"$", "--json")
}

// SuiteArgs runs the whole suite (no name filter) with the JSON report.
func (typeScriptConvention) SuiteArgs(base []string) []string {
	return append(append([]string(nil), base...), "--json")
}

// jestReport is the subset of Jest's `--json` output this reads (Vitest's json reporter matches). Each
// assertionResult is one test (`fullName` = ancestorTitles + title); a testResult with no assertions
// but a failed status is a suite that failed to transpile/execute — a build failure, not an assertion.
type jestReport struct {
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

// ParseReport normalizes a Jest/Vitest `--json` run. A build/transpile failure (a suite Jest could not
// run: numRuntimeErrorTestSuites, or a failed suite with zero assertions) sets BuildFailed — the same
// assertion-vs-compile distinction the Go convention draws. As with the mutation-report readers, output
// it cannot decode is an error (the caller fails closed), never an empty report scored as a clean run.
// A FAILED test is authoritative and is never masked by a same-named pass (matching the Go convention).
func (typeScriptConvention) ParseReport(code int, stdout, stderr string) (TestReport, error) {
	data, ok := extractJSONObject(stdout)
	if !ok {
		return TestReport{}, fmt.Errorf("testconv(typescript): no JSON report object in the runner output (exit %d)", code)
	}
	var raw jestReport
	if err := json.Unmarshal(data, &raw); err != nil {
		return TestReport{}, fmt.Errorf("testconv(typescript): parsing the runner's --json report: %w", err)
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

// extractJSONObject returns the outermost `{…}` in s. Jest normally writes clean JSON to stdout under
// --json, but a stray console.log or ts diagnostic can precede it; taking the first `{` through the last
// `}` recovers the report without a bespoke parser. It reports false when there is no object at all.
func extractJSONObject(s string) ([]byte, bool) {
	i := strings.IndexByte(s, '{')
	j := strings.LastIndexByte(s, '}')
	if i < 0 || j < i {
		return nil, false
	}
	return []byte(s[i : j+1]), true
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
