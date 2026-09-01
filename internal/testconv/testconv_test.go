package testconv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestForSelectsAndFailsClosed(t *testing.T) {
	c, ok := For("go")
	if !ok || c == nil || c.Name() != "go" {
		t.Fatalf("For(go) must return the Go convention, got %v ok=%v", c, ok)
	}
	// The load-bearing negative: an unknown or empty name returns (nil,false) so the caller fails
	// closed — never a silent default to Go.
	if c, ok := For("typescript"); ok || c != nil {
		t.Fatal("an unregistered convention must return (nil,false)")
	}
	if c, ok := For(""); ok || c != nil {
		t.Fatal("the empty name must return (nil,false)")
	}
}

func TestClassify(t *testing.T) {
	pass := TestReport{Tests: map[string]Outcome{"TestX": Passed}}
	if Classify(pass, "TestX") != ClsPassed {
		t.Fatal("a passed target is ClsPassed")
	}
	fail := TestReport{Tests: map[string]Outcome{"TestX": Failed}}
	if Classify(fail, "TestX") != ClsAssert {
		t.Fatal("a failed target is ClsAssert")
	}
	// Precision the console-scrape lacked: the target genuinely failed, so a co-occurring build failure
	// (a sibling package) is irrelevant — it is still a valid assertion fail-before.
	if Classify(TestReport{Tests: map[string]Outcome{"TestX": Failed}, BuildFailed: true}, "TestX") != ClsAssert {
		t.Fatal("a target that itself failed is ClsAssert even when some package build-failed")
	}
	// Absent + build failed = the target's package did not build.
	if Classify(TestReport{Tests: map[string]Outcome{}, BuildFailed: true}, "TestX") != ClsCompile {
		t.Fatal("an absent target with a build failure is ClsCompile")
	}
	// Absent + clean build = the test is simply not in the tree.
	if Classify(TestReport{Tests: map[string]Outcome{"TestOther": Passed}}, "TestX") != ClsNoTest {
		t.Fatal("an absent target with a clean build is ClsNoTest")
	}
}

func TestGoConventionBasics(t *testing.T) {
	c := goConvention{}
	if !c.IsTestFile("a/b_test.go") || c.IsTestFile("a/b.go") {
		t.Fatal("IsTestFile keys on the _test.go suffix")
	}
	run := c.RunArgs([]string{"go", "test", "./..."}, "TestX")
	if got := run[len(run)-3:]; got[0] != "-run" || got[1] != "^TestX$" || got[2] != "-json" {
		t.Fatalf("RunArgs must anchor -run and request -json, got %v", run)
	}
	// The base is copied, not aliased: appending must not scribble on the caller's slice.
	base := []string{"go", "test", "./..."}
	_ = c.RunArgs(base, "TestX")
	if len(base) != 3 {
		t.Fatal("RunArgs must not mutate the base command")
	}
	suite := c.SuiteArgs([]string{"go", "test", "./..."})
	if suite[len(suite)-1] != "-json" || len(suite) != 4 {
		t.Fatalf("SuiteArgs must append only -json, got %v", suite)
	}
	// A name with regexp metacharacters must be quoted so -run matches it literally.
	q := c.RunArgs([]string{"go", "test"}, "Test.$")
	if q[len(q)-2] != `^Test\.\$$` {
		t.Fatalf("RunArgs must regexp-quote the test name, got %q", q[len(q)-2])
	}
}

// go test -json fixtures: one JSON object per line.
const (
	evPass    = `{"Action":"run","Package":"p","Test":"TestX"}` + "\n" + `{"Action":"pass","Package":"p","Test":"TestX"}` + "\n"
	evFail    = `{"Action":"run","Package":"p","Test":"TestX"}` + "\n" + `{"Action":"fail","Package":"p","Test":"TestX"}` + "\n"
	evSibling = `{"Action":"output","Package":"sib","Output":"?   sib [no test files]\n"}` + "\n"
	evBuild   = `{"Action":"output","Package":"p","Output":"# p [p.test]\n"}` + "\n" +
		`{"Action":"output","Package":"p","Output":"./a_test.go:5:2: undefined: foo\n"}` + "\n" +
		`{"Action":"output","Package":"p","Output":"FAIL\tp [build failed]\n"}` + "\n" +
		`{"Action":"fail","Package":"p"}` + "\n"
)

func TestGoParseReport(t *testing.T) {
	c := goConvention{}
	must := func(code int, out string) TestReport {
		r, err := c.ParseReport(code, out, "")
		if err != nil {
			t.Fatalf("ParseReport unexpected error: %v", err)
		}
		return r
	}
	// A passing target, amid sibling "[no test files]" noise, classifies as passed.
	if Classify(must(0, evSibling+evPass), "TestX") != ClsPassed {
		t.Fatal("pass event -> ClsPassed, sibling noise ignored")
	}
	// A failing target is an assertion fail-before.
	if Classify(must(1, evFail), "TestX") != ClsAssert {
		t.Fatal("fail event -> ClsAssert")
	}
	// Exit 0, target never ran (only a sibling) -> absent, no build failure.
	if Classify(must(0, evSibling), "TestX") != ClsNoTest {
		t.Fatal("no target event, clean build -> ClsNoTest")
	}
	// The target's package failed to build -> ClsCompile.
	if Classify(must(1, evBuild), "TestX") != ClsCompile {
		t.Fatal("build-failed package -> ClsCompile")
	}
	// Precision: the TARGET failed an assertion while a SIBLING build-failed -> still ClsAssert.
	mixed := evFail + `{"Action":"output","Package":"sib","Output":"FAIL\tsib [build failed]\n"}` + "\n" + `{"Action":"fail","Package":"sib"}` + "\n"
	r := must(1, mixed)
	if !r.BuildFailed {
		t.Fatal("a sibling build failure must be recorded")
	}
	if Classify(r, "TestX") != ClsAssert {
		t.Fatal("the target's own assertion failure wins over a sibling build failure")
	}
	// An early, non-JSON compiler error line (test2json did not wrap it) still sets BuildFailed.
	if Classify(must(2, "# p\n./a_test.go:1:1: syntax error\nFAIL\tp [build failed]\n"), "TestX") != ClsCompile {
		t.Fatal("a non-JSON build-failure line must set BuildFailed")
	}
}

func TestGoParseReportUnreadable(t *testing.T) {
	c := goConvention{}
	// A nonzero exit with neither JSON events nor a build marker is unreadable — an error, so the
	// caller fails closed rather than treat garbage as a clean (empty) report.
	if _, err := c.ParseReport(1, "totally not json and no marker\n", ""); err == nil {
		t.Fatal("unreadable failing output must be an error")
	}
	// Exit 0 with empty output is a legitimately empty run (nothing matched -run), not an error.
	if r, err := c.ParseReport(0, "", ""); err != nil || len(r.Tests) != 0 || r.BuildFailed {
		t.Fatalf("empty clean output must be an empty report, got %+v err=%v", r, err)
	}
}

func TestGoDeletesATest(t *testing.T) {
	c := goConvention{}
	if !c.DeletesATest("@@\n-func TestFoo(t *testing.T) {") {
		t.Fatal("a removed TestFoo must be detected")
	}
	for _, kind := range []string{"Test", "Benchmark", "Fuzz", "Example"} {
		if !c.DeletesATest("@@\n-func " + kind + "Bar(") {
			t.Fatalf("a removed %sBar must be detected", kind)
		}
	}
	// A bare "Test(" and "Test_x(" match the naming rule; a lowercase suffix (Testhelper) does not.
	if !c.DeletesATest("-func Test(") || !c.DeletesATest("-func Test_x(") {
		t.Fatal("Test( and Test_x( follow the naming rule")
	}
	if c.DeletesATest("-func Testhelper(") {
		t.Fatal("Testhelper is not a test function")
	}
	// A "---" diff header line that happens to start with a removed marker must be skipped.
	if c.DeletesATest("--- a/foo_test.go\n") {
		t.Fatal("a diff header must not be read as a removed test")
	}
	if c.DeletesATest("@@\n context line\n+func TestAdded(") {
		t.Fatal("an added test is not a deletion")
	}
}

func TestGoDirHasTests(t *testing.T) {
	c := goConvention{}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if c.DirHasTests(root, "pkg/a.go") {
		t.Fatal("a package with no _test.go has no tests")
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "a_test.go"), []byte("package pkg\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !c.DirHasTests(root, "pkg/a.go") {
		t.Fatal("a package with a _test.go has tests")
	}
}

func TestGoSemanticallyNull(t *testing.T) {
	c := goConvention{}
	null := [][2]string{
		{"package p\n// one\nvar x = 1\n", "package p\n// two\nvar x = 1\n"},             // comment change
		{"package p\nvar x = 1\n", "package p\nvar   x =  1\n"},                          // whitespace
		{"package p\n// note: one\nvar x = 1\n", "package p\n// note: two\nvar x = 1\n"}, // colon in a plain comment
		{"package p\n/* one */\nvar x = 1\n", "package p\n/* two */\nvar x = 1\n"},       // block comment
	}
	for _, p := range null {
		if !c.SemanticallyNull(p[0], p[1]) {
			t.Fatalf("expected null: %q vs %q", p[0], p[1])
		}
	}
	notNull := [][2]string{
		{"package p\nvar x = 1\n", "package p\nvar x = 2\n"},                 // real change
		{"package p\nvar x = 1\n", "package p\nvar x = 1 + y\n"},             // added token
		{"package p\nvar x = 1\n", "package p\nvar x = \"unterminated\n"},    // mutated does not scan
		{"package p\nvar x = `unterminated", "package p\nvar x = 1\n"},       // orig does not scan
		{"//go:build linux\npackage p\n", "//go:build windows\npackage p\n"}, // directive
		{"package p\n//go:noinline\nfunc f() {}\n", "package p\n//go:noescape\nfunc f() {}\n"},
		{"package p\nfunc f() {}\n", "package p\n//go:noinline\nfunc f() {}\n"}, // directive added
		{"package p\n//export One\nfunc f() {}\n", "package p\n//export Two\nfunc f() {}\n"},
		{"package p\n//extern one\nfunc f()\n", "package p\n//extern two\nfunc f()\n"},
		{"// +build linux\npackage p\n", "// +build windows\npackage p\n"},
		{"//   +build linux\npackage p\n", "//   +build windows\npackage p\n"},
		{"//\t+build linux\npackage p\n", "//\t+build windows\npackage p\n"},
	}
	for _, p := range notNull {
		if c.SemanticallyNull(p[0], p[1]) {
			t.Fatalf("expected NOT null: %q vs %q", p[0], p[1])
		}
	}
}
