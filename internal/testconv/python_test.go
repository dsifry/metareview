package testconv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPyBasics(t *testing.T) {
	c := pythonConvention{}
	for _, f := range []string{"test_calc.py", "tests/test_mod.py", "calc_test.py", "pkg/a_test.py"} {
		if !c.IsTestFile(f) {
			t.Fatalf("%s must be a pytest test file", f)
		}
	}
	for _, f := range []string{"calc.py", "conftest.py", "test_notpy.txt", "protest.py"} {
		if c.IsTestFile(f) {
			t.Fatalf("%s must NOT be a test file", f)
		}
	}
	run := c.RunArgs([]string{"pytest"}, "tests/test_mod.py::TestC::test_x")
	// pytest ≥6.1's default (xunit2) drops the file attr, so we pin xunit1 to keep nodeid identity.
	if !strings.Contains(strings.Join(run, " "), "junit_family=xunit1") {
		t.Fatalf("RunArgs must pin xunit1 (for the file attr), got %v", run)
	}
	if got := run[len(run)-2:]; got[0] != "--junit-xml=/dev/stdout" || got[1] != "tests/test_mod.py::TestC::test_x" {
		t.Fatalf("RunArgs must add junit-xml=/dev/stdout and the nodeid positional last, got %v", run)
	}
	base := []string{"pytest"}
	_ = c.RunArgs(base, "x")
	if len(base) != 1 {
		t.Fatal("RunArgs must not mutate the base command")
	}
	suite := c.SuiteArgs([]string{"pytest"})
	if !strings.Contains(strings.Join(suite, " "), "junit_family=xunit1") || suite[len(suite)-1] != "--junit-xml=/dev/stdout" {
		t.Fatalf("SuiteArgs must pin xunit1 and end with the junit flag, got %v", suite)
	}
}

func TestPyNodeIDReconstruction(t *testing.T) {
	// A module-level function: no class segment.
	fn := junitCase{Classname: "tests.test_mod", Name: "test_foo", File: "tests/test_mod.py"}
	if got := pytestNodeID(fn); got != "tests/test_mod.py::test_foo" {
		t.Fatalf("function nodeid: %q", got)
	}
	// A class method: the class segment(s) after the module stem are kept.
	m := junitCase{Classname: "tests.test_mod.TestCalc", Name: "test_bar", File: "tests/test_mod.py"}
	if got := pytestNodeID(m); got != "tests/test_mod.py::TestCalc::test_bar" {
		t.Fatalf("method nodeid: %q", got)
	}
	// Parametrized: the [params] suffix rides along in name.
	p := junitCase{Classname: "test_mod", Name: "test_foo[1-2]", File: "test_mod.py"}
	if got := pytestNodeID(p); got != "test_mod.py::test_foo[1-2]" {
		t.Fatalf("parametrized nodeid: %q", got)
	}
	// No file attribute → degrade to classname::name.
	if got := pytestNodeID(junitCase{Classname: "test_mod", Name: "test_x"}); got != "test_mod::test_x" {
		t.Fatalf("no-file nodeid: %q", got)
	}
}

const (
	// classic <testsuites> root
	pyPass = `<?xml version="1.0"?><testsuites><testsuite name="pytest" tests="1"><testcase classname="tests.test_mod" name="test_foo" file="tests/test_mod.py"/></testsuite></testsuites>`
	// bare <testsuite> root (xunit2 default)
	pyFail = `<testsuite name="pytest" tests="1" failures="1"><testcase classname="tests.test_mod.TestCalc" name="test_bar" file="tests/test_mod.py"><failure message="assert 1 == 2">details</failure></testcase></testsuite>`
	// a collection/crash error → BuildFailed; skipped → absent
	pyError   = `<testsuite name="pytest" errors="1"><testcase classname="tests.test_mod" name="test_foo" file="tests/test_mod.py"><error message="ImportError">boom</error></testcase></testsuite>`
	pySkipped = `<testsuite name="pytest"><testcase classname="tests.test_mod" name="test_foo" file="tests/test_mod.py"><skipped/></testcase></testsuite>`
)

func TestPyParseReport(t *testing.T) {
	c := pythonConvention{}
	must := func(out string) TestReport {
		r, err := c.ParseReport(1, out, "")
		if err != nil {
			t.Fatalf("ParseReport error: %v", err)
		}
		return r
	}
	if Classify(must(pyPass), "tests/test_mod.py::test_foo") != ClsPassed {
		t.Fatal("a passing testcase -> ClsPassed (classic <testsuites> root)")
	}
	if Classify(must(pyFail), "tests/test_mod.py::TestCalc::test_bar") != ClsAssert {
		t.Fatal("a <failure> -> ClsAssert (bare <testsuite> root, method nodeid)")
	}
	if Classify(must(pyError), "tests/test_mod.py::test_foo") != ClsCompile {
		t.Fatal("an <error> (collection/crash) -> ClsCompile")
	}
	if Classify(must(pySkipped), "tests/test_mod.py::test_foo") != ClsNoTest {
		t.Fatal("a <skipped> testcase is absent -> ClsNoTest")
	}
	// XML embedded in console noise is still extracted.
	if Classify(must("=== test session starts ===\nblah\n"+pyPass+"\n1 passed\n"), "tests/test_mod.py::test_foo") != ClsPassed {
		t.Fatal("XML must be extracted from surrounding console output")
	}
	// A FAILED result must not be masked by a same-named pass in another suite.
	mixed := `<testsuites><testsuite><testcase classname="test_mod" name="t" file="test_mod.py"><failure/></testcase></testsuite>` +
		`<testsuite><testcase classname="test_mod" name="t" file="test_mod.py"/></testsuite></testsuites>`
	if Classify(must(mixed), "test_mod.py::t") != ClsAssert {
		t.Fatal("a same-named pass must not mask a failure")
	}
}

func TestPyParseReportUnreadable(t *testing.T) {
	c := pythonConvention{}
	if _, err := c.ParseReport(1, "no xml here, just console output\n", ""); err == nil {
		t.Fatal("output with no JUnit XML must be an error")
	}
	// An opening tag with no matching close → no extractable block → error (not a clean report).
	if _, err := c.ParseReport(1, "<testsuite name=\"pytest\"> truncated, never closed\n", ""); err == nil {
		t.Fatal("an unterminated JUnit block must be an error")
	}
	// A block that opens a testsuite but is not well-formed must be a parse error, not a clean report.
	if _, err := c.ParseReport(1, "<testsuite><testcase name=></testsuite>", ""); err == nil {
		t.Fatal("malformed XML must be an error")
	}
	// The <testsuites> root parse-error path.
	if _, err := c.ParseReport(1, "<testsuites><bad</testsuites>", ""); err == nil {
		t.Fatal("malformed <testsuites> must be an error")
	}
}

func TestPyDeletesATest(t *testing.T) {
	c := pythonConvention{}
	del := "diff --git a/test_mod.py b/test_mod.py\n--- a/test_mod.py\n+++ b/test_mod.py\n@@ -1,3 +1,2 @@\n def test_keep(): pass\n-def test_gone(): pass\n"
	if !c.DeletesATest(del) {
		t.Fatal("a removed line in a test file must trigger the gate")
	}
	delFile := "diff --git a/a_test.py b/a_test.py\n--- a/a_test.py\n+++ /dev/null\n@@ -1,1 +0,0 @@\n-def test_x(): pass\n"
	if !c.DeletesATest(delFile) {
		t.Fatal("a deleted test file must trigger the gate (old path)")
	}
	prod := "diff --git a/calc.py b/calc.py\n--- a/calc.py\n+++ b/calc.py\n@@ -1,2 +1,1 @@\n-x = 1\n"
	if c.DeletesATest(prod) {
		t.Fatal("a removal in a production file must not trigger")
	}
	add := "diff --git a/test_mod.py b/test_mod.py\n--- a/test_mod.py\n+++ b/test_mod.py\n@@ -1,1 +1,2 @@\n def test_x(): pass\n+def test_new(): pass\n"
	if c.DeletesATest(add) {
		t.Fatal("an addition-only test-file change must not trigger")
	}
}

func TestPyDirHasTestsAndNull(t *testing.T) {
	c := pythonConvention{}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if c.DirHasTests(root, "pkg/calc.py") {
		t.Fatal("a dir with no test file has no tests")
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "calc_test.py"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if !c.DirHasTests(root, "pkg/calc.py") {
		t.Fatal("a dir with a *_test.py has tests")
	}
	if c.SemanticallyNull("x", "x") {
		t.Fatal("python SemanticallyNull must be conservatively false")
	}
}
