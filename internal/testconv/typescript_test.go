package testconv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTSBasics(t *testing.T) {
	c := typeScriptConvention{}
	for _, f := range []string{"src/calc.test.ts", "a/b.spec.tsx", "x.test.js", "y.spec.jsx", "m.test.mts", "e.test.mjs", "c.spec.cjs"} {
		if !c.IsTestFile(f) {
			t.Fatalf("%s must be a test file", f)
		}
	}
	for _, f := range []string{"src/calc.ts", "a/b.tsx", "notes.md", "test.ts"} {
		if c.IsTestFile(f) {
			t.Fatalf("%s must NOT be a test file", f)
		}
	}
	run := c.RunArgs([]string{"npx", "jest"}, "Calculator adds")
	if got := run[len(run)-3:]; got[0] != "--testNamePattern" || got[1] != `^Calculator adds$` || got[2] != "--json" {
		t.Fatalf("RunArgs must anchor --testNamePattern and request --json, got %v", run)
	}
	base := []string{"npx", "jest"}
	_ = c.RunArgs(base, "x")
	if len(base) != 2 {
		t.Fatal("RunArgs must not mutate the base command")
	}
	suite := c.SuiteArgs([]string{"npx", "jest"})
	if suite[len(suite)-1] != "--json" || len(suite) != 3 {
		t.Fatalf("SuiteArgs must append only --json, got %v", suite)
	}
}

// Fixtures carry numTotalTestSuites — the sentinel findJestReport requires to distinguish the real
// report line from pretty-reporter braces in the combined output.
const (
	jestPass = `{"numTotalTestSuites":1,"numRuntimeErrorTestSuites":0,"testResults":[{"status":"passed","assertionResults":[{"fullName":"Calculator adds","status":"passed"}]}]}`
	jestFail = `{"numTotalTestSuites":1,"numRuntimeErrorTestSuites":0,"testResults":[{"status":"failed","assertionResults":[{"ancestorTitles":["Calculator"],"title":"adds","status":"failed"}]}]}`
	// A suite that failed to transpile/collect: a failed testResult with no assertions.
	jestTranspile = `{"numTotalTestSuites":1,"numRuntimeErrorTestSuites":0,"testResults":[{"status":"failed","message":"SyntaxError: x","assertionResults":[]}]}`
	// numRuntimeErrorTestSuites marks a suite that errored at collection time.
	jestRuntimeErr = `{"numTotalTestSuites":1,"numRuntimeErrorTestSuites":1,"testResults":[]}`
)

func TestTSParseReport(t *testing.T) {
	c := typeScriptConvention{}
	must := func(code int, out string) TestReport {
		r, err := c.ParseReport(code, out, "")
		if err != nil {
			t.Fatalf("ParseReport error: %v", err)
		}
		return r
	}
	if Classify(must(0, jestPass), "Calculator adds") != ClsPassed {
		t.Fatal("a passed assertion -> ClsPassed")
	}
	// fullName absent -> reconstructed from ancestorTitles + title.
	if Classify(must(1, jestFail), "Calculator adds") != ClsAssert {
		t.Fatal("a failed assertion -> ClsAssert (name reconstructed)")
	}
	if Classify(must(1, jestTranspile), "Calculator adds") != ClsCompile {
		t.Fatal("a failed suite with no assertions -> ClsCompile")
	}
	if Classify(must(1, jestRuntimeErr), "Calculator adds") != ClsCompile {
		t.Fatal("numRuntimeErrorTestSuites>0 -> ClsCompile")
	}
	// A leading console.log before the JSON is tolerated (first { .. last }).
	if Classify(must(0, "console noise\n"+jestPass+"\ntrailing\n"), "Calculator adds") != ClsPassed {
		t.Fatal("a JSON report preceded by log noise must still parse")
	}
	// A FAILED test must never be masked by a same-named pass in another suite.
	mixed := `{"numTotalTestSuites":2,"testResults":[{"status":"failed","assertionResults":[{"fullName":"T","status":"failed"}]},{"status":"passed","assertionResults":[{"fullName":"T","status":"passed"}]}]}`
	if Classify(must(1, mixed), "T") != ClsAssert {
		t.Fatal("a same-named pass must not mask a failure")
	}
	// Pretty-reporter braces on stderr (merged into the stream) must NOT be mistaken for the report:
	// the report line is found by its numTotalTestSuites sentinel, not by brace position.
	pretty := "  ● Calculator › adds\n    expect(received).toBe(expected)\n    { a: 1 }\n" + jestFail + "\n"
	if Classify(must(1, pretty), "Calculator adds") != ClsAssert {
		t.Fatal("pretty-reporter braces must not corrupt report extraction")
	}
}

func TestTSParseReportUnreadable(t *testing.T) {
	c := typeScriptConvention{}
	// No line carries the Jest report sentinel → no report found.
	if _, err := c.ParseReport(1, "no json here at all\n{ \"other\": 1 }\n", ""); err == nil {
		t.Fatal("output with no Jest report line must be an error")
	}
	// A line WITH the sentinel but a type-mismatched body → the Unmarshal error path.
	if _, err := c.ParseReport(1, `{"numTotalTestSuites":1,"testResults":"not-an-array"}`, ""); err == nil {
		t.Fatal("a report line that does not decode must be an error")
	}
}

func TestTSDeletesATest(t *testing.T) {
	c := typeScriptConvention{}
	// A removed line inside a test file's hunk.
	del := "diff --git a/calc.test.ts b/calc.test.ts\n--- a/calc.test.ts\n+++ b/calc.test.ts\n@@ -1,3 +1,2 @@\n it('x',()=>{})\n-it('gone',()=>{})\n"
	if !c.DeletesATest(del) {
		t.Fatal("a removed line in a test file must trigger the gate")
	}
	// A deleted test FILE (new side /dev/null) is recognized by its old path.
	delFile := "diff --git a/a.spec.ts b/a.spec.ts\n--- a/a.spec.ts\n+++ /dev/null\n@@ -1,1 +0,0 @@\n-it('x',()=>{})\n"
	if !c.DeletesATest(delFile) {
		t.Fatal("a deleted test file must trigger the gate")
	}
	// A removal in a NON-test file must not trigger.
	prod := "diff --git a/calc.ts b/calc.ts\n--- a/calc.ts\n+++ b/calc.ts\n@@ -1,2 +1,1 @@\n-const gone = 1\n"
	if c.DeletesATest(prod) {
		t.Fatal("a removal in a production file must not trigger the TS test-deletion gate")
	}
	// A pure ADDITION to a test file (no removed content line) must not trigger.
	add := "diff --git a/calc.test.ts b/calc.test.ts\n--- a/calc.test.ts\n+++ b/calc.test.ts\n@@ -1,1 +1,2 @@\n it('x',()=>{})\n+it('new',()=>{})\n"
	if c.DeletesATest(add) {
		t.Fatal("an addition-only test-file change must not trigger the gate")
	}
	// The --- header of a test file must not itself be read as a removed content line.
	headerOnly := "diff --git a/calc.test.ts b/calc.test.ts\n--- a/calc.test.ts\n+++ b/calc.test.ts\n@@ -1,1 +1,1 @@\n unchanged\n"
	if c.DeletesATest(headerOnly) {
		t.Fatal("a test-file diff with no removed content line must not trigger")
	}
}

func TestTSDirHasTests(t *testing.T) {
	c := typeScriptConvention{}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if c.DirHasTests(root, "src/calc.ts") {
		t.Fatal("a dir with no test file has no tests")
	}
	if err := os.WriteFile(filepath.Join(root, "src", "calc.spec.ts"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if !c.DirHasTests(root, "src/calc.ts") {
		t.Fatal("a dir with a *.spec.ts has tests")
	}
}

func TestTSSemanticallyNullIsAlwaysFalse(t *testing.T) {
	// Dependency-free: no stdlib JS tokenizer, so the pre-screen is skipped (the safe direction).
	c := typeScriptConvention{}
	if c.SemanticallyNull("const x = 1", "const x = 1") {
		t.Fatal("TS SemanticallyNull must be conservatively false, even for identical input")
	}
}
