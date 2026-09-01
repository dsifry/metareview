// Package testconv is metareview's pluggable test-convention seam (spec §4.2). The differential-proof
// engine (reproduction, deletion, trivial-pin) has a language-agnostic spine but a handful of leaf
// decisions that are language-specific: which files are tests, how to run one test and the whole
// suite, how to read the result, whether a directory has tests, and whether a mutation is a no-op.
// A Convention answers exactly those, so a repository in any language participates by selecting one.
//
// It mirrors the mutation-report idiom (internal/mutation/{parse,stryker}.go): metareview reads a
// robust EXTERNAL tool's own structured output — here each test runner's machine-readable report
// (`go test -json`, Jest `--json`, Vitest `--reporter=json`) — and normalizes it. It never parses
// source and never scrapes console text. As there, an outcome the normaliser does not recognise is
// never scored as success, and output it cannot read is refused rather than treated as an empty
// (clean-looking) result.
package testconv

// Outcome is what became of one test in a run. Only the two decided states exist: a test that did not
// run leaves no entry in TestReport.Tests at all (its absence is the signal), so there is no third
// "unknown" value to be mistaken for a pass.
type Outcome int

const (
	Passed Outcome = iota // the test ran and passed
	Failed                // the test ran and failed an assertion
)

// TestReport is the normalized result of running the suite (or a single narrowed test) — the analogue
// of mutation.Report, built ONLY from a runner's structured output. Tests maps a test's identity (the
// runner's own test id: a Go test function name, a Jest `describe > it` string) to its Outcome; a test
// that did not run is simply absent. BuildFailed records that the target's own package failed to
// compile/transpile or its setup failed, so no assertion was ever reached — the load-bearing
// distinction (spec §9.2/§9.3): a "failure" that is really a build error proves nothing about a fault.
type TestReport struct {
	Tests       map[string]Outcome
	BuildFailed bool
}

// Class is the assertion-vs-compile axis for ONE target test, the five-way result reproduceOne
// switches on (formerly mutation.failClass). It is DERIVED generically from a TestReport by Classify —
// no per-language logic — so every runner shares one classification once its report is normalized.
type Class int

const (
	ClsAssert  Class = iota // the target ran and failed an assertion — a valid fail-before
	ClsPassed               // the target ran and passed
	ClsNoTest               // the target did not run and the build was fine — it is absent from the tree
	ClsCompile              // the target's package did not build — NOT an assertion, so it proves nothing
)

// Classify maps a normalized report onto the target test's Class. It is the whole point of normalizing
// to a report first: the decision is one language-independent function.
//
//   - target present & Passed  -> ClsPassed
//   - target present & Failed  -> ClsAssert  (it genuinely ran and failed; a sibling package's build
//     failure is irrelevant because the report attributes the failure to THIS test — the precision the
//     old console-scrape lacked, so it had to conservatively call any build marker a compile error)
//   - target absent & BuildFailed -> ClsCompile (the target's package did not build, so it could not run)
//   - target absent & built       -> ClsNoTest  (the run was clean but this test is not in the tree)
func Classify(r TestReport, test string) Class {
	if o, ok := r.Tests[test]; ok {
		if o == Passed {
			return ClsPassed
		}
		return ClsAssert
	}
	if r.BuildFailed {
		return ClsCompile
	}
	return ClsNoTest
}

// Convention is the per-language seam. Implementations are pure and cheap except ParseReport, which
// only reads bytes a runner already produced. Everything the engine needs that is language-specific
// lives here; the engine itself is language-agnostic.
type Convention interface {
	// Name is the stable identifier a workflow selects it by ("go", "typescript").
	Name() string
	// IsTestFile reports whether a repository-relative path is a test file (the reproduction partition
	// and DirHasTests both key on this).
	IsTestFile(path string) bool
	// RunArgs builds the argv that runs EXACTLY the one named test, emitting the runner's structured
	// report, from the consent-hashed base command. The test id is quoted/anchored by the convention so
	// it selects one test and cannot inject a flag.
	RunArgs(base []string, test string) []string
	// SuiteArgs builds the argv that runs the WHOLE suite (no test filter), emitting the structured
	// report — used by the over-deletion scope check.
	SuiteArgs(base []string) []string
	// ParseReport normalizes one run's (exit code, stdout, stderr) into a TestReport. Output it cannot
	// read is an error (the caller fails closed), never an empty report — an empty report would score a
	// broken run as a clean one (the refuse-unreadable rule from the mutation package).
	ParseReport(code int, stdout, stderr string) (TestReport, error)
	// DeletesATest reports whether a unified diff removes a test (the §9.6 gate trigger). Each language
	// uses its most robust available mechanism; it is a cheap diff check, never a runner invocation.
	DeletesATest(diff string) bool
	// DirHasTests reports whether the directory holding a repository-relative file contains test files
	// (the §3.1 owed-pin gate). root is the repository/worktree root.
	DirHasTests(root, file string) bool
	// SemanticallyNull reports whether a from->to mutation is a no-op the compiler cannot tell apart —
	// the §9.8 R7 trivial-pin pre-screen. A convention with no robust tokenizer returns false (the safe
	// direction: the full compile-then-break steps still run).
	SemanticallyNull(orig, mutated string) bool
}

// For returns the convention selected by name. An empty or unknown name returns (nil, false): the
// caller MUST fail closed (a proof it cannot classify is unverifiable), never fall through to a
// default language — a non-Go repository silently scored by Go rules would mint false proofs and miss
// test deletions, which is worse than declining to prove.
func For(name string) (Convention, bool) {
	c, ok := registry[name]
	return c, ok
}

// registry is the fixed set of built-in conventions. It is populated by each convention's file init;
// there is no dynamic registration, so the set is auditable at compile time.
var registry = map[string]Convention{}

func register(c Convention) { registry[c.Name()] = c }
