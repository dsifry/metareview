# metareview task-done context

Run ID: `mrv-20260901-190331877901000-task-done-multilang-pytest-conv-1f2195e7`

## Task

Advisory task target: multilang-pytest-conv

## Git

- Base: `8c6f8a0f6b0e3ae946baf61f7c0da312f22cbce1`
- Head: `9c599afca7c8bab66e974eb0c7bbe4f194ccc179`
- Branch: `multilang-pytest-conv`
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `49404`
- Filtered diff bytes: `17940`
- Risk level: `none`
- Generated files excluded: docs/metareview/context/mrv-20260901-185610756923000-task-done-multilang-pytest-conv-1f2195e7-context.md, docs/metareview/evidence/multilang-pytest-conv.md, docs/metareview/reviews/mrv-20260901-185610756923000-task-done-multilang-pytest-conv-1f2195e7.md

## Context Shard Plan

Not sharded.

## Review Manifest

- Manifest verdict: `PASS`
- Source manifest hash: not sharded
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- internal/testconv/python.go
- internal/testconv/python_test.go
- internal/testconv/testconv_test.go

### Local changes (not sharded)
- .claude/worktrees/agent-af9d648e34ca9450a/

### Path Dispositions
- docs/metareview/context/mrv-20260901-185610756923000-task-done-multilang-pytest-conv-1f2195e7-context.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/evidence/multilang-pytest-conv.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/reviews/mrv-20260901-185610756923000-task-done-multilang-pytest-conv-1f2195e7.md: generated (metareview generated review artifact excluded from source manifest)

### Manifest Blockers
No manifest blockers.

## Changed Files

- internal/testconv/python.go
- internal/testconv/python_test.go
- internal/testconv/testconv_test.go
- .claude/worktrees/agent-af9d648e34ca9450a/

## Diff

```diff
diff --git a/internal/testconv/python.go b/internal/testconv/python.go
new file mode 100644
index 0000000..1f657ed
--- /dev/null
+++ b/internal/testconv/python.go
@@ -0,0 +1,211 @@
+package testconv
+
+import (
+	"encoding/xml"
+	"fmt"
+	"path/filepath"
+	"strings"
+)
+
+func init() { register(pythonConvention{}) }
+
+// pythonConvention supports Python repositories driven by PYTEST. pytest has no native stdout JSON, so
+// this reads its native, dependency-free machine format — **JUnit XML** (`--junit-xml`, built into
+// pytest core; also the cross-language test-result standard) — written to `/dev/stdout` so the engine's
+// stdout-only contract holds, and extracted from the combined output. It parses that XML with
+// encoding/xml; it never parses Python source. It carries no state.
+//
+// Portability: `--junit-xml=/dev/stdout` is a Unix mechanism. On Windows the report would need a
+// file-based seam extension (a documented follow-up, the same shape as the Jest/Vitest outputFile
+// edge). If the XML does not reach stdout, ParseReport fails CLOSED (unverifiable), never silently wrong.
+type pythonConvention struct{}
+
+func (pythonConvention) Name() string { return "python" }
+
+// IsTestFile is pytest's default discovery convention: a basename `test_*.py` or `*_test.py`.
+func (pythonConvention) IsTestFile(path string) bool {
+	base := filepath.Base(path)
+	return strings.HasSuffix(base, "_test.py") || (strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py"))
+}
+
+// pinXunit1 forces the xunit1 JUnit family. pytest ≥6.1 defaults to xunit2, which OMITS the `file`
+// attribute on `<testcase>` — and without `file` the nodeid cannot be reconstructed, so `Classify`
+// would never match the id `RunArgs` selects by. xunit1 carries `file` (and `line`), so pinning it on
+// the command line (overriding any repo ini/pyproject setting) makes test identity reliable. It is not
+// a dependency — just the format variant that reports the identity this seam needs.
+var pinXunit1 = []string{"-o", "junit_family=xunit1"}
+
+// RunArgs narrows the base pytest command to exactly one test by its nodeid (the unambiguous selector
+// pytest accepts as a positional — never `-k`, which is a substring match), pins xunit1, and writes the
+// JUnit XML to stdout. The nodeid is passed verbatim as one argv element, so it cannot inject a flag.
+func (pythonConvention) RunArgs(base []string, test string) []string {
+	out := append(append([]string(nil), base...), pinXunit1...)
+	return append(out, "--junit-xml=/dev/stdout", test)
+}
+
+// SuiteArgs runs the whole suite (xunit1, JUnit XML on stdout).
+func (pythonConvention) SuiteArgs(base []string) []string {
+	out := append(append([]string(nil), base...), pinXunit1...)
+	return append(out, "--junit-xml=/dev/stdout")
+}
+
+// junit* mirror the subset of pytest's JUnit XML this reads. The root is `<testsuites>` (classic) or a
+// bare `<testsuite>` (xunit2, pytest ≥6.1's default) — both are handled. A testcase's child element is
+// the outcome: `<failure>` = an assertion failed (the test ran); `<error>` = it crashed or failed to
+// collect/setup (never reached an assertion); `<skipped>` = it did not run.
+type junitCase struct {
+	Classname string     `xml:"classname,attr"`
+	Name      string     `xml:"name,attr"`
+	File      string     `xml:"file,attr"`
+	Failure   *junitMark `xml:"failure"`
+	Error     *junitMark `xml:"error"`
+	Skipped   *junitMark `xml:"skipped"`
+}
+type junitMark struct{}
+type junitSuite struct {
+	Cases []junitCase `xml:"testcase"`
+}
+type junitSuites struct {
+	Suites []junitSuite `xml:"testsuite"`
+}
+
+// ParseReport normalizes a pytest JUnit XML run. It extracts the XML block from the combined output,
+// parses it under either root shape, and maps each testcase: `<failure>` → Failed, a bare testcase →
+// Passed, `<error>` → BuildFailed (a crash/collection/setup failure is not an assertion — the same
+// assertion-vs-compile axis the Go convention draws), `<skipped>` → absent. A FAILED result STICKS —
+// never masked by a same-named pass. No XML block, or one that will not parse, is an error (fail closed).
+func (pythonConvention) ParseReport(code int, stdout, stderr string) (TestReport, error) {
+	block, ok := extractJUnitXML(stdout)
+	if !ok {
+		return TestReport{}, fmt.Errorf("testconv(python): no JUnit XML report in the pytest output (exit %d)", code)
+	}
+	var suites []junitSuite
+	if strings.HasPrefix(strings.TrimSpace(block), "<testsuites") {
+		var root junitSuites
+		if err := xml.Unmarshal([]byte(block), &root); err != nil {
+			return TestReport{}, fmt.Errorf("testconv(python): parsing the JUnit XML report: %w", err)
+		}
+		suites = root.Suites
+	} else {
+		var one junitSuite
+		if err := xml.Unmarshal([]byte(block), &one); err != nil {
+			return TestReport{}, fmt.Errorf("testconv(python): parsing the JUnit XML report: %w", err)
+		}
+		suites = []junitSuite{one}
+	}
+	rep := TestReport{Tests: map[string]Outcome{}}
+	for _, s := range suites {
+		for _, c := range s.Cases {
+			id := pytestNodeID(c)
+			switch {
+			case c.Error != nil:
+				// A crash or collection/setup error: no assertion was reached. Mark the run build-failed
+				// and leave the test absent, so Classify reports ClsCompile for it (never a valid fail-before).
+				rep.BuildFailed = true
+			case c.Failure != nil:
+				rep.Tests[id] = Failed
+			case c.Skipped != nil:
+				// did not execute → absent
+			default:
+				if rep.Tests[id] != Failed { // a failure is authoritative and never masked by a pass
+					rep.Tests[id] = Passed
+				}
+			}
+		}
+	}
+	return rep, nil
+}
+
+// extractJUnitXML pulls the JUnit XML element out of pytest's combined console+XML stdout (the XML is
+// written as one block at session end). It takes from the first `<testsuites`/`<testsuite` opening to
+// the matching last close, so console text around it is ignored. No block → not found.
+func extractJUnitXML(s string) (string, bool) {
+	start := strings.Index(s, "<testsuites")
+	closeTag := "</testsuites>"
+	if start < 0 {
+		start = strings.Index(s, "<testsuite")
+		closeTag = "</testsuite>"
+	}
+	if start < 0 {
+		return "", false
+	}
+	end := strings.LastIndex(s, closeTag)
+	if end < 0 {
+		return "", false
+	}
+	return s[start : end+len(closeTag)], true
+}
+
+// pytestNodeID reconstructs the pytest nodeid (the id RunArgs selects by) from a JUnit testcase's file,
+// classname, and name. JUnit's `classname` is the dotted module path plus any test-class names; the
+// nodeid is `<file>[::<Class>...]::<name>`. The class segments are those AFTER the module — found as the
+// classname segments following the last one equal to the file's stem. A module-level function has none
+// (`file.py::name`); a method keeps its class (`file.py::TestClass::name`). Parametrized names carry
+// their `[params]` suffix in `name` on both sides, so they match. If `file` is absent (unusual), it
+// degrades to `classname::name`.
+func pytestNodeID(c junitCase) string {
+	if c.File == "" {
+		return c.Classname + "::" + c.Name
+	}
+	stem := strings.TrimSuffix(filepath.Base(c.File), filepath.Ext(c.File))
+	segs := strings.Split(c.Classname, ".")
+	last := -1
+	for i, seg := range segs {
+		if seg == stem {
+			last = i
+		}
+	}
+	parts := []string{c.File}
+	if last >= 0 && last+1 < len(segs) {
+		parts = append(parts, segs[last+1:]...)
+	}
+	parts = append(parts, c.Name)
+	return strings.Join(parts, "::")
+}
+
+// DeletesATest reports whether the diff removes test content from a Python test file — the same
+// conservative, parser-free structural signal the TS convention uses: a removed content line inside a
+// hunk of a file whose path IsTestFile (over-triggers safely; the §9.6 gate then rechecks proven pins).
+func (c pythonConvention) DeletesATest(diff string) bool {
+	inTestFile, inHunk := false, false
+	for _, line := range strings.Split(diff, "\n") {
+		switch {
+		case strings.HasPrefix(line, "diff --git "):
+			inTestFile, inHunk = diffGitTouchesTestFile(line, c), false
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
+// DirHasTests reports whether the directory holding file contains any pytest test file.
+func (c pythonConvention) DirHasTests(root, file string) bool {
+	dir := filepath.Join(root, filepath.Dir(file))
+	for _, pat := range []string{"test_*.py", "*_test.py"} {
+		if m, err := filepath.Glob(filepath.Join(dir, pat)); err == nil && len(m) > 0 {
+			return true
+		}
+	}
+	return false
+}
+
+// SemanticallyNull always reports false: there is no standard-library Python tokenizer and the seam is
+// dependency-free, so the trivial-pin pre-screen is skipped for Python (the safe direction).
+func (pythonConvention) SemanticallyNull(orig, mutated string) bool { return false }
+
+// diffGitTouchesTestFile reports whether either path on a `diff --git a/… b/…` line is a test file for
+// the given convention (so a deleted test file, whose new side is /dev/null, is caught by its old path).
+func diffGitTouchesTestFile(line string, c Convention) bool {
+	for _, f := range strings.Fields(strings.TrimPrefix(line, "diff --git ")) {
+		f = strings.TrimPrefix(strings.TrimPrefix(f, "a/"), "b/")
+		if c.IsTestFile(f) {
+			return true
+		}
+	}
+	return false
+}
diff --git a/internal/testconv/python_test.go b/internal/testconv/python_test.go
new file mode 100644
index 0000000..04a4af7
--- /dev/null
+++ b/internal/testconv/python_test.go
@@ -0,0 +1,163 @@
+package testconv
+
+import (
+	"os"
+	"path/filepath"
+	"strings"
+	"testing"
+)
+
+func TestPyBasics(t *testing.T) {
+	c := pythonConvention{}
+	for _, f := range []string{"test_calc.py", "tests/test_mod.py", "calc_test.py", "pkg/a_test.py"} {
+		if !c.IsTestFile(f) {
+			t.Fatalf("%s must be a pytest test file", f)
+		}
+	}
+	for _, f := range []string{"calc.py", "conftest.py", "test_notpy.txt", "protest.py"} {
+		if c.IsTestFile(f) {
+			t.Fatalf("%s must NOT be a test file", f)
+		}
+	}
+	run := c.RunArgs([]string{"pytest"}, "tests/test_mod.py::TestC::test_x")
+	// pytest ≥6.1's default (xunit2) drops the file attr, so we pin xunit1 to keep nodeid identity.
+	if !strings.Contains(strings.Join(run, " "), "junit_family=xunit1") {
+		t.Fatalf("RunArgs must pin xunit1 (for the file attr), got %v", run)
+	}
+	if got := run[len(run)-2:]; got[0] != "--junit-xml=/dev/stdout" || got[1] != "tests/test_mod.py::TestC::test_x" {
+		t.Fatalf("RunArgs must add junit-xml=/dev/stdout and the nodeid positional last, got %v", run)
+	}
+	base := []string{"pytest"}
+	_ = c.RunArgs(base, "x")
+	if len(base) != 1 {
+		t.Fatal("RunArgs must not mutate the base command")
+	}
+	suite := c.SuiteArgs([]string{"pytest"})
+	if !strings.Contains(strings.Join(suite, " "), "junit_family=xunit1") || suite[len(suite)-1] != "--junit-xml=/dev/stdout" {
+		t.Fatalf("SuiteArgs must pin xunit1 and end with the junit flag, got %v", suite)
+	}
+}
+
+func TestPyNodeIDReconstruction(t *testing.T) {
+	// A module-level function: no class segment.
+	fn := junitCase{Classname: "tests.test_mod", Name: "test_foo", File: "tests/test_mod.py"}
+	if got := pytestNodeID(fn); got != "tests/test_mod.py::test_foo" {
+		t.Fatalf("function nodeid: %q", got)
+	}
+	// A class method: the class segment(s) after the module stem are kept.
+	m := junitCase{Classname: "tests.test_mod.TestCalc", Name: "test_bar", File: "tests/test_mod.py"}
+	if got := pytestNodeID(m); got != "tests/test_mod.py::TestCalc::test_bar" {
+		t.Fatalf("method nodeid: %q", got)
+	}
+	// Parametrized: the [params] suffix rides along in name.
+	p := junitCase{Classname: "test_mod", Name: "test_foo[1-2]", File: "test_mod.py"}
+	if got := pytestNodeID(p); got != "test_mod.py::test_foo[1-2]" {
+		t.Fatalf("parametrized nodeid: %q", got)
+	}
+	// No file attribute → degrade to classname::name.
+	if got := pytestNodeID(junitCase{Classname: "test_mod", Name: "test_x"}); got != "test_mod::test_x" {
+		t.Fatalf("no-file nodeid: %q", got)
+	}
+}
+
+const (
+	// classic <testsuites> root
+	pyPass = `<?xml version="1.0"?><testsuites><testsuite name="pytest" tests="1"><testcase classname="tests.test_mod" name="test_foo" file="tests/test_mod.py"/></testsuite></testsuites>`
+	// bare <testsuite> root (xunit2 default)
+	pyFail = `<testsuite name="pytest" tests="1" failures="1"><testcase classname="tests.test_mod.TestCalc" name="test_bar" file="tests/test_mod.py"><failure message="assert 1 == 2">details</failure></testcase></testsuite>`
+	// a collection/crash error → BuildFailed; skipped → absent
+	pyError   = `<testsuite name="pytest" errors="1"><testcase classname="tests.test_mod" name="test_foo" file="tests/test_mod.py"><error message="ImportError">boom</error></testcase></testsuite>`
+	pySkipped = `<testsuite name="pytest"><testcase classname="tests.test_mod" name="test_foo" file="tests/test_mod.py"><skipped/></testcase></testsuite>`
+)
+
+func TestPyParseReport(t *testing.T) {
+	c := pythonConvention{}
+	must := func(out string) TestReport {
+		r, err := c.ParseReport(1, out, "")
+		if err != nil {
+			t.Fatalf("ParseReport error: %v", err)
+		}
+		return r
+	}
+	if Classify(must(pyPass), "tests/test_mod.py::test_foo") != ClsPassed {
+		t.Fatal("a passing testcase -> ClsPassed (classic <testsuites> root)")
+	}
+	if Classify(must(pyFail), "tests/test_mod.py::TestCalc::test_bar") != ClsAssert {
+		t.Fatal("a <failure> -> ClsAssert (bare <testsuite> root, method nodeid)")
+	}
+	if Classify(must(pyError), "tests/test_mod.py::test_foo") != ClsCompile {
+		t.Fatal("an <error> (collection/crash) -> ClsCompile")
+	}
+	if Classify(must(pySkipped), "tests/test_mod.py::test_foo") != ClsNoTest {
+		t.Fatal("a <skipped> testcase is absent -> ClsNoTest")
+	}
+	// XML embedded in console noise is still extracted.
+	if Classify(must("=== test session starts ===\nblah\n"+pyPass+"\n1 passed\n"), "tests/test_mod.py::test_foo") != ClsPassed {
+		t.Fatal("XML must be extracted from surrounding console output")
+	}
+	// A FAILED result must not be masked by a same-named pass in another suite.
+	mixed := `<testsuites><testsuite><testcase classname="test_mod" name="t" file="test_mod.py"><failure/></testcase></testsuite>` +
+		`<testsuite><testcase classname="test_mod" name="t" file="test_mod.py"/></testsuite></testsuites>`
+	if Classify(must(mixed), "test_mod.py::t") != ClsAssert {
+		t.Fatal("a same-named pass must not mask a failure")
+	}
+}
+
+func TestPyParseReportUnreadable(t *testing.T) {
+	c := pythonConvention{}
+	if _, err := c.ParseReport(1, "no xml here, just console output\n", ""); err == nil {
+		t.Fatal("output with no JUnit XML must be an error")
+	}
+	// An opening tag with no matching close → no extractable block → error (not a clean report).
+	if _, err := c.ParseReport(1, "<testsuite name=\"pytest\"> truncated, never closed\n", ""); err == nil {
+		t.Fatal("an unterminated JUnit block must be an error")
+	}
+	// A block that opens a testsuite but is not well-formed must be a parse error, not a clean report.
+	if _, err := c.ParseReport(1, "<testsuite><testcase name=></testsuite>", ""); err == nil {
+		t.Fatal("malformed XML must be an error")
+	}
+	// The <testsuites> root parse-error path.
+	if _, err := c.ParseReport(1, "<testsuites><bad</testsuites>", ""); err == nil {
+		t.Fatal("malformed <testsuites> must be an error")
+	}
+}
+
+func TestPyDeletesATest(t *testing.T) {
+	c := pythonConvention{}
+	del := "diff --git a/test_mod.py b/test_mod.py\n--- a/test_mod.py\n+++ b/test_mod.py\n@@ -1,3 +1,2 @@\n def test_keep(): pass\n-def test_gone(): pass\n"
+	if !c.DeletesATest(del) {
+		t.Fatal("a removed line in a test file must trigger the gate")
+	}
+	delFile := "diff --git a/a_test.py b/a_test.py\n--- a/a_test.py\n+++ /dev/null\n@@ -1,1 +0,0 @@\n-def test_x(): pass\n"
+	if !c.DeletesATest(delFile) {
+		t.Fatal("a deleted test file must trigger the gate (old path)")
+	}
+	prod := "diff --git a/calc.py b/calc.py\n--- a/calc.py\n+++ b/calc.py\n@@ -1,2 +1,1 @@\n-x = 1\n"
+	if c.DeletesATest(prod) {
+		t.Fatal("a removal in a production file must not trigger")
+	}
+	add := "diff --git a/test_mod.py b/test_mod.py\n--- a/test_mod.py\n+++ b/test_mod.py\n@@ -1,1 +1,2 @@\n def test_x(): pass\n+def test_new(): pass\n"
+	if c.DeletesATest(add) {
+		t.Fatal("an addition-only test-file change must not trigger")
+	}
+}
+
+func TestPyDirHasTestsAndNull(t *testing.T) {
+	c := pythonConvention{}
+	root := t.TempDir()
+	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
+		t.Fatal(err)
+	}
+	if c.DirHasTests(root, "pkg/calc.py") {
+		t.Fatal("a dir with no test file has no tests")
+	}
+	if err := os.WriteFile(filepath.Join(root, "pkg", "calc_test.py"), []byte(""), 0o644); err != nil {
+		t.Fatal(err)
+	}
+	if !c.DirHasTests(root, "pkg/calc.py") {
+		t.Fatal("a dir with a *_test.py has tests")
+	}
+	if c.SemanticallyNull("x", "x") {
+		t.Fatal("python SemanticallyNull must be conservatively false")
+	}
+}
diff --git a/internal/testconv/testconv_test.go b/internal/testconv/testconv_test.go
index 6154797..336aa8f 100644
--- a/internal/testconv/testconv_test.go
+++ b/internal/testconv/testconv_test.go
@@ -17,6 +17,9 @@ func TestForSelectsAndFailsClosed(t *testing.T) {
 	if c, ok := For("vitest"); !ok || c == nil || c.Name() != "vitest" {
 		t.Fatalf("For(vitest) must return the Vitest convention, got %v ok=%v", c, ok)
 	}
+	if c, ok := For("python"); !ok || c == nil || c.Name() != "python" {
+		t.Fatalf("For(python) must return the pytest convention, got %v ok=%v", c, ok)
+	}
 	// The load-bearing negative: an unknown or empty name returns (nil,false) so the caller fails
 	// closed — never a silent default to a language.
 	if c, ok := For("cobol"); ok || c != nil {



```

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

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

## Shepherding round 1 (Cursor Bugbot — 1× High, fixed)

- **Default xunit2 omits `file`, breaking nodeid identity.** Bugbot flagged (and the pytest docs
  confirm) that pytest ≥6.1 defaults to the **xunit2** family, which drops the `<testcase file=…>`
  attribute — so `pytestNodeID` would degrade to `classname::name`, never matching the nodeid `RunArgs`
  selects by, and every pytest reproduction proof would fail to match. Fixed: `RunArgs`/`SuiteArgs` now
  **pin `-o junit_family=xunit1`** (which restores `file`/`line`), overriding any repo ini setting. Not
  a dependency — just the format variant that carries the identity this seam needs. Test asserts the
  pin; mutation-verified.

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

