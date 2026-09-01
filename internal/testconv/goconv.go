package testconv

import (
	"encoding/json"
	"fmt"
	"go/scanner"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
)

func init() { register(goConvention{}) }

// goConvention is the reference Convention: Go's `*_test.go` files, `go test -json` for structured
// results, the `testing` package's test-name rule for deletion, and a `go/scanner` token walk for the
// trivial-pin pre-screen. It carries no state.
type goConvention struct{}

func (goConvention) Name() string { return "go" }

// IsTestFile is Go's `*_test.go` convention (spec §9.1) — the same one DirHasTests globs on.
func (goConvention) IsTestFile(path string) bool { return strings.HasSuffix(path, "_test.go") }

// RunArgs narrows the base command to exactly one test and asks for the JSON event stream. The name is
// regexp-quoted and anchored so -run selects that one test and cannot inject a flag (argv, never a
// shell string). -json subsumes -v: test2json emits per-test run/pass/fail events without it.
func (goConvention) RunArgs(base []string, test string) []string {
	return append(append([]string(nil), base...), "-run", "^"+regexp.QuoteMeta(test)+"$", "-json")
}

// SuiteArgs runs the whole suite (no -run filter) with the JSON event stream.
func (goConvention) SuiteArgs(base []string) []string {
	return append(append([]string(nil), base...), "-json")
}

// goEvent is the subset of a `go test -json` (test2json) event this reads: the per-test pass/fail
// actions carry Test; package-level output (Test == "") carries the build/setup/vet failure text.
type goEvent struct {
	Action  string `json:"Action"`
	Test    string `json:"Test"`
	Output  string `json:"Output"`
	Package string `json:"Package"`
}

// ParseReport normalizes a `go test -json` run. Each line is one event; a per-test pass/fail action
// records the test's Outcome (keyed by the target's own name, so a sibling package's noise is a
// different Test/Package and simply never recorded — the multi-package hole the console-scrape had is
// gone structurally). Package-level output plus any non-JSON line (an early compiler error test2json
// did not wrap) is scanned for a build/setup/vet-failure marker, which sets BuildFailed. A failed run
// that emitted no JSON events at all and no recognizable build marker is unreadable — an error, so the
// caller fails closed rather than treat a garbage run as a clean (empty) report.
func (goConvention) ParseReport(code int, stdout, stderr string) (TestReport, error) {
	rep := TestReport{Tests: map[string]Outcome{}}
	var buildText strings.Builder
	sawEvent := false
	for _, raw := range strings.Split(stdout, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		var ev goEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			// Not a test2json line — an early compiler error printed before the stream. Keep it for the
			// build-marker scan; it is not a test outcome.
			buildText.WriteString(line + "\n")
			continue
		}
		sawEvent = true
		switch ev.Action {
		case "pass":
			if ev.Test != "" {
				rep.Tests[ev.Test] = Passed
			}
		case "fail":
			if ev.Test != "" {
				rep.Tests[ev.Test] = Failed
			}
		case "output":
			if ev.Test == "" { // package-level output — where "FAIL pkg [build failed]" lands
				buildText.WriteString(ev.Output)
			}
		}
	}
	if isBuildFailure(buildText.String()) {
		rep.BuildFailed = true
	}
	// A nonzero exit with neither a JSON stream nor a build marker is unreadable: something ran that was
	// not `go test -json`, or it died before emitting anything. Refuse rather than report a clean run.
	if !sawEvent && !rep.BuildFailed && code != 0 {
		return TestReport{}, fmt.Errorf("testconv(go): the test run produced no go-test-json events and no build-failure marker (exit %d)", code)
	}
	return rep, nil
}

// isBuildFailure reports a `go test` build/setup/vet failure — the modes that make a non-zero exit mean
// "the code did not build", never "an assertion failed".
func isBuildFailure(out string) bool {
	return strings.Contains(out, "[build failed]") ||
		strings.Contains(out, "[setup failed]") ||
		strings.Contains(out, "[vet failed]")
}

// testFuncRemovedRe matches a removed Go test declaration, following the testing package's naming rule:
// TestXxx/BenchmarkXxx/FuzzXxx/ExampleXxx where Xxx is EMPTY or starts with a non-lowercase rune (so
// `Test`/`TestFoo`/`Test_x`/`TestÜ` match but `Testhelper` does not), with optional whitespace before
// the parameter list. Suffix runes are Unicode identifier characters, not ASCII-only.
var testFuncRemovedRe = regexp.MustCompile(`^-func (Test|Benchmark|Fuzz|Example)([^\p{Ll}\s(][\p{L}\p{N}_]*)?\s*\(`)

// DeletesATest reports whether the diff removes a Go test function — a removed `func Test.../Benchmark
// .../Fuzz.../Example...` line, covering both a deleted *_test.go file (its func lines appear removed)
// and a test removed from a surviving file. This is the `testing` package's own name rule, matched
// exactly on the diff's removed lines — a language spec, not a hand-rolled parser. Header lines are
// skipped.
func (goConvention) DeletesATest(diff string) bool {
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "---") {
			continue
		}
		if testFuncRemovedRe.MatchString(line) {
			return true
		}
	}
	return false
}

// DirHasTests reports whether the directory holding file (relative to root) contains Go test files.
func (goConvention) DirHasTests(root, file string) bool {
	matches, err := filepath.Glob(filepath.Join(root, filepath.Dir(file), "*_test.go"))
	return err == nil && len(matches) > 0
}

// SemanticallyNull reports whether orig and mutated are the SAME Go program ignoring comments and
// whitespace — a no-op mutation the compiler cannot tell apart (spec §9.8 R7). It compares the two
// token streams (comments dropped by the scanner, whitespace irrelevant between tokens): position-
// independent and pure. If EITHER body fails to scan, it returns false (a mutation that breaks scanning
// is the compile step's job, not this pre-screen's). No dead-code/reachability detection is attempted.
func (goConvention) SemanticallyNull(orig, mutated string) bool {
	a, ok := goTokens(orig)
	if !ok {
		return false
	}
	b, ok := goTokens(mutated)
	if !ok {
		return false
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// goTokens scans src into its Go token stream, one "tok lit" string per token (the literal distinguishes
// identifiers/literals of the same class). Ordinary comments are dropped (semantically null), but a
// comment DIRECTIVE (//go:build, //go:embed, //line, //export, //extern, // +build) is KEPT: it affects
// build selection, embedded data, or linkage, so a change to one is NOT null. ok is false if the source
// did not scan cleanly.
func goTokens(src string) (toks []string, ok bool) {
	var fset token.FileSet
	file := fset.AddFile("", fset.Base(), len(src))
	var s scanner.Scanner
	ok = true
	s.Init(file, []byte(src), func(token.Position, string) { ok = false }, scanner.ScanComments)
	for {
		_, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		if tok == token.COMMENT {
			if d, isDir := directive(lit); isDir {
				toks = append(toks, "//dir "+d)
			}
			continue
		}
		if lit != "" {
			toks = append(toks, tok.String()+" "+lit)
		} else {
			toks = append(toks, tok.String())
		}
	}
	return toks, ok
}

// directive reports whether a line comment is a meaningful Go comment directive (returning its text
// without the leading //). It covers go/ast's isDirective form — a "//line " directive, or "//name:arg"
// with a lowercase/digit name and no space before the colon (e.g. //go:build) — PLUS the comments that
// change linkage or build selection: cgo "//export Name", gccgo "//extern name", and the legacy
// "// +build" constraint. An ordinary comment (even one with a colon, or a block comment) is not one.
func directive(comment string) (string, bool) {
	if !strings.HasPrefix(comment, "//") {
		return "", false // block comments are never directives
	}
	c := comment[2:]
	if strings.HasPrefix(c, "line ") || strings.HasPrefix(c, "export ") || strings.HasPrefix(c, "extern ") ||
		strings.HasPrefix(strings.TrimLeft(c, " \t"), "+build") {
		return c, true
	}
	colon := strings.Index(c, ":")
	if colon <= 0 || colon+1 >= len(c) {
		return "", false
	}
	for i := 0; i < colon; i++ {
		b := c[i]
		lowerOrDigit := b >= 'a' && b <= 'z' || b >= '0' && b <= '9'
		if !lowerOrDigit {
			return "", false
		}
	}
	return c, true
}
