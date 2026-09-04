package testconv

import (
	"encoding/xml"
	"fmt"
	"path/filepath"
	"strings"
)

func init() { register(pythonConvention{}) }

// pythonConvention supports Python repositories driven by PYTEST. pytest has no native stdout JSON, so
// this reads its native, dependency-free machine format — **JUnit XML** (`--junit-xml`, built into
// pytest core; also the cross-language test-result standard) — written to `/dev/stdout` so the engine's
// stdout-only contract holds, and extracted from the combined output. It parses that XML with
// encoding/xml; it never parses Python source. It carries no state.
//
// Portability: `--junit-xml=/dev/stdout` is a Unix mechanism. On Windows the report would need a
// file-based seam extension (a documented follow-up, the same shape as the Jest/Vitest outputFile
// edge). If the XML does not reach stdout, ParseReport fails CLOSED (unverifiable), never silently wrong.
type pythonConvention struct{}

func (pythonConvention) Name() string { return "python" }

// IsTestFile is pytest's default discovery convention: a basename `test_*.py` or `*_test.py`.
func (pythonConvention) IsTestFile(path string) bool {
	base := filepath.Base(path)
	return strings.HasSuffix(base, "_test.py") || (strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py"))
}

// pinXunit1 forces the xunit1 JUnit family. pytest ≥6.1 defaults to xunit2, which OMITS the `file`
// attribute on `<testcase>` — and without `file` the nodeid cannot be reconstructed, so `Classify`
// would never match the id `RunArgs` selects by. xunit1 carries `file` (and `line`), so pinning it on
// the command line (overriding any repo ini/pyproject setting) makes test identity reliable. It is not
// a dependency — just the format variant that reports the identity this seam needs.
var pinXunit1 = []string{"-o", "junit_family=xunit1"}

// RunArgs narrows the base pytest command to exactly one test by its nodeid (the unambiguous selector
// pytest accepts as a positional — never `-k`, which is a substring match), pins xunit1, and writes the
// JUnit XML to stdout. The nodeid is passed verbatim as one argv element, so it cannot inject a flag.
func (pythonConvention) RunArgs(base []string, test string) []string {
	out := append(append([]string(nil), base...), pinXunit1...)
	return append(out, "--junit-xml=/dev/stdout", test)
}

// SuiteArgs runs the whole suite (xunit1, JUnit XML on stdout).
func (pythonConvention) SuiteArgs(base []string) []string {
	out := append(append([]string(nil), base...), pinXunit1...)
	return append(out, "--junit-xml=/dev/stdout")
}

// junit* mirror the subset of pytest's JUnit XML this reads. The root is `<testsuites>` (classic) or a
// bare `<testsuite>` (xunit2, pytest ≥6.1's default) — both are handled. A testcase's child element is
// the outcome: `<failure>` = an assertion failed (the test ran); `<error>` = it crashed or failed to
// collect/setup (never reached an assertion); `<skipped>` = it did not run.
type junitCase struct {
	Classname string     `xml:"classname,attr"`
	Name      string     `xml:"name,attr"`
	File      string     `xml:"file,attr"`
	Failure   *junitMark `xml:"failure"`
	Error     *junitMark `xml:"error"`
	Skipped   *junitMark `xml:"skipped"`
}
type junitMark struct{}
type junitSuite struct {
	Cases []junitCase `xml:"testcase"`
}
type junitSuites struct {
	Suites []junitSuite `xml:"testsuite"`
}

// ParseReport normalizes a pytest JUnit XML run. It extracts the XML block from the combined output,
// parses it under either root shape, and maps each testcase: `<failure>` → Failed, a bare testcase →
// Passed, `<error>` → BuildFailed (a crash/collection/setup failure is not an assertion — the same
// assertion-vs-compile axis the Go convention draws), `<skipped>` → absent. A FAILED result STICKS —
// never masked by a same-named pass. No XML block, or one that will not parse, is an error (fail closed).
func (pythonConvention) ParseReport(code int, stdout, stderr string) (TestReport, error) {
	block, ok := extractJUnitXML(stdout)
	if !ok {
		return TestReport{}, fmt.Errorf("testconv(python): no JUnit XML report in the pytest output (exit %d)", code)
	}
	var suites []junitSuite
	if strings.HasPrefix(strings.TrimSpace(block), "<testsuites") {
		var root junitSuites
		if err := xml.Unmarshal([]byte(block), &root); err != nil {
			return TestReport{}, fmt.Errorf("testconv(python): parsing the JUnit XML report: %w", err)
		}
		suites = root.Suites
	} else {
		var one junitSuite
		if err := xml.Unmarshal([]byte(block), &one); err != nil {
			return TestReport{}, fmt.Errorf("testconv(python): parsing the JUnit XML report: %w", err)
		}
		suites = []junitSuite{one}
	}
	rep := TestReport{Tests: map[string]Outcome{}}
	for _, s := range suites {
		for _, c := range s.Cases {
			id := pytestNodeID(c)
			// An if/else-if chain rather than a tagless `switch {}`: Go's coverage tool emits no counter
			// for a tagless-switch case expression, so these guards read as permanently uncovered and
			// mutation testing can never exercise them. As `if` conditions they are covered and killable;
			// behaviour is identical (first true branch wins).
			if c.Error != nil {
				// A crash or collection/setup error: no assertion was reached. Mark the run build-failed
				// and leave the test absent, so Classify reports ClsCompile for it (never a valid fail-before).
				rep.BuildFailed = true
			} else if c.Failure != nil {
				rep.Tests[id] = Failed
			} else if c.Skipped != nil {
				// did not execute → absent
			} else if rep.Tests[id] != Failed { // a failure is authoritative and never masked by a pass
				rep.Tests[id] = Passed
			}
		}
	}
	return rep, nil
}

// extractJUnitXML pulls the JUnit XML element out of pytest's combined console+XML stdout (the XML is
// written as one block at session end). It takes from the first `<testsuites`/`<testsuite` opening to
// the matching last close, so console text around it is ignored. No block → not found.
func extractJUnitXML(s string) (string, bool) {
	start := strings.Index(s, "<testsuites")
	closeTag := "</testsuites>"
	if start < 0 {
		start = strings.Index(s, "<testsuite")
		closeTag = "</testsuite>"
	}
	if start < 0 {
		return "", false
	}
	end := strings.LastIndex(s, closeTag)
	if end < 0 {
		return "", false
	}
	return s[start : end+len(closeTag)], true
}

// pytestNodeID reconstructs the pytest nodeid (the id RunArgs selects by) from a JUnit testcase's file,
// classname, and name. JUnit's `classname` is the dotted module path plus any test-class names; the
// nodeid is `<file>[::<Class>...]::<name>`. The class segments are those AFTER the module — found as the
// classname segments following the last one equal to the file's stem. A module-level function has none
// (`file.py::name`); a method keeps its class (`file.py::TestClass::name`). Parametrized names carry
// their `[params]` suffix in `name` on both sides, so they match. If `file` is absent (unusual), it
// degrades to `classname::name`.
func pytestNodeID(c junitCase) string {
	if c.File == "" {
		return c.Classname + "::" + c.Name
	}
	stem := strings.TrimSuffix(filepath.Base(c.File), filepath.Ext(c.File))
	segs := strings.Split(c.Classname, ".")
	last := -1
	for i, seg := range segs {
		if seg == stem {
			last = i
		}
	}
	parts := []string{c.File}
	if last >= 0 && last+1 < len(segs) {
		parts = append(parts, segs[last+1:]...)
	}
	parts = append(parts, c.Name)
	return strings.Join(parts, "::")
}

// DeletesATest reports whether the diff removes test content from a Python test file — the same
// conservative, parser-free structural signal the TS convention uses: a removed content line inside a
// hunk of a file whose path IsTestFile (over-triggers safely; the §9.6 gate then rechecks proven pins).
func (c pythonConvention) DeletesATest(diff string) bool {
	inTestFile, inHunk := false, false
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			inTestFile, inHunk = diffGitTouchesTestFile(line, c), false
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

// DirHasTests reports whether the directory holding file contains any pytest test file.
func (c pythonConvention) DirHasTests(root, file string) bool {
	dir := filepath.Join(root, filepath.Dir(file))
	for _, pat := range []string{"test_*.py", "*_test.py"} {
		if m, err := filepath.Glob(filepath.Join(dir, pat)); err == nil && len(m) > 0 {
			return true
		}
	}
	return false
}

// SemanticallyNull always reports false: there is no standard-library Python tokenizer and the seam is
// dependency-free, so the trivial-pin pre-screen is skipped for Python (the safe direction).
func (pythonConvention) SemanticallyNull(orig, mutated string) bool { return false }

// diffGitTouchesTestFile reports whether either path on a `diff --git a/… b/…` line is a test file for
// the given convention (so a deleted test file, whose new side is /dev/null, is caught by its old path).
func diffGitTouchesTestFile(line string, c Convention) bool {
	for _, f := range strings.Fields(strings.TrimPrefix(line, "diff --git ")) {
		f = strings.TrimPrefix(strings.TrimPrefix(f, "a/"), "b/")
		if c.IsTestFile(f) {
			return true
		}
	}
	return false
}
