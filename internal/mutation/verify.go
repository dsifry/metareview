package mutation

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/dsifry/metareview/internal/findings"
)

// Pin is a claim that one test holds one line of production code: replacing From with To at File
// must make the tests fail. It mirrors run.Pin, kept here so this package does not depend on the
// FSM's types.
type Pin struct {
	File string
	From string
	To   string
	Test string
}

// Outcome separates "this claim is false" from "I could not evaluate this claim". They call for
// opposite responses — write a test, versus rewrite the pin — and collapsing them makes an agent
// with clumsy syntax indistinguishable from one shipping untested code, while burning the
// iteration budget on the wrong problem.
type Outcome string

const (
	// Proven: breaking the line failed the tests and restoring it passed them.
	PinProven Outcome = "proven"
	// Survived: the mutation was applied, compiled, and the tests still passed. A real defect in
	// the test, and the finding this whole node exists to raise.
	PinSurvived Outcome = "survived"
	// Malformed: the pin could not be evaluated — the anchor is absent or ambiguous, the file is
	// unreadable, or the mutation does not compile. Widened per §2.2/§9.8 R7 to also cover a
	// compiles-but-semantically-null mutation (a comment/whitespace/dead-code pin) once the §9.8
	// AST pre-screen (T1.6) classifies it here rather than letting it reach `survived`. Says
	// nothing about the fix: the CLAIM is bad, rewrite the pin.
	PinMalformed Outcome = "malformed"
	// Unverifiable: the tree itself could not answer — the baseline is red, or a command would not
	// run. Neither the pin nor the fix is implicated, and nothing may be concluded from the run.
	PinUnverifiable Outcome = "unverifiable"
)

// PinResult is what verification found. Proven is the only value a gate should accept, and it is
// true only when BOTH halves held: the mutation failed the tests and the restored code passed
// them. Either half alone proves nothing — a suite that always fails "detects" everything, and
// one that always passes detects nothing.
type PinResult struct {
	Pin Pin
	// Proven is kept as the single thing a gate needs, and is true exactly when Outcome is Proven.
	Proven  bool
	Outcome Outcome
	Detail  string
}

// Verifier proves pins against a copy of a tree.
//
// It never modifies Dir. Every mutation is applied inside a fresh temporary copy, because on
// 2026-08-29 a fix agent left its mutation in the live tree, reported "No production behavior
// changed", and claimed a kill its test did not achieve. One `git diff` would have caught it, and
// nothing was running one.
//
// TestCmd is supplied by the caller from the workflow's consent-hashed command block, never by
// the agent whose work is being checked: a pin says what to break, never what to run.
type Verifier struct {
	Dir     string
	TestCmd []string
	// BuildCmd checks that a mutation still compiles. Empty defaults to the Go form. It is
	// separate from TestCmd because "does this build" and "do the tests pass" are different
	// questions, and a language whose build check is not a test invocation needs to say so.
	BuildCmd []string
	Timeout  time.Duration
	// Now and Run are seams for tests; nil means the real thing.
	Run func(ctx context.Context, dir string, argv []string) (int, string, error)
}

const defaultVerifyTimeout = 10 * time.Minute

// Verify checks every pin and returns one result each, in order. A pin that cannot be checked is
// reported unproven with the reason; it is never an error for the whole run, because one bad pin
// must not discard the evidence for the others.
func (v Verifier) Verify(ctx context.Context, pins []Pin) ([]PinResult, error) {
	// A misconfiguration aborts the whole run rather than marking every pin unproven. Reporting
	// "the mutation survived" for each of them when no test command was ever configured would
	// blame the fixes for the harness's own fault, and a wall of false blockers is worse than one
	// honest error.
	if len(pins) > 0 && len(v.TestCmd) == 0 {
		return nil, fmt.Errorf("mutation: no test command configured, so no pin can be checked")
	}
	out := make([]PinResult, 0, len(pins))
	for _, p := range pins {
		out = append(out, v.verifyOne(ctx, p))
	}
	return out, nil
}

func (v Verifier) verifyOne(ctx context.Context, p Pin) PinResult {
	fail := func(o Outcome, format string, a ...any) PinResult {
		return PinResult{Pin: p, Outcome: o, Detail: fmt.Sprintf(format, a...)}
	}
	work, err := os.MkdirTemp("", "mrv-verify-")
	if err != nil {
		return fail(PinUnverifiable, "could not create a working copy: %v", err)
	}
	defer func() { _ = os.RemoveAll(work) }()
	if err := copyTree(v.Dir, work); err != nil {
		return fail(PinUnverifiable, "could not copy the tree: %v", err)
	}

	// Pin.File is agent-supplied and untrusted: an absolute path or one climbing out with ".."
	// would make filepath.Join escape the temp copy and mutate the real tree (the one this verifier
	// promises never to touch). Reject anything that does not stay strictly inside the working copy.
	target := filepath.Join(work, filepath.FromSlash(p.File))
	if filepath.IsAbs(p.File) {
		return fail(PinMalformed, "the pin's file %q must be repo-relative", p.File)
	}
	if rel, rerr := filepath.Rel(work, target); rerr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fail(PinMalformed, "the pin's file %q escapes the working copy", p.File)
	}
	original, err := os.ReadFile(target) // #nosec G304 -- path validated just above, inside a temp copy
	if err != nil {
		return fail(PinMalformed, "could not read %s in the copy: %v", p.File, err)
	}
	body := string(original)
	if n := strings.Count(body, p.From); n != 1 {
		// Zero occurrences means the pin does not describe this tree; more than one means the
		// mutation is ambiguous and we would be proving something other than what was claimed.
		return fail(PinMalformed, "the anchor text appears %d times in %s; it must appear exactly once", n, p.File)
	}

	// 1. Baseline: the restored tree must pass, or the mutation result means nothing.
	if code, out, err := v.run(ctx, work); err != nil {
		return fail(PinUnverifiable, "baseline test run failed to execute: %v", err)
	} else if code != 0 {
		return fail(PinUnverifiable, "the tests do not pass before mutating, so nothing can be concluded: %s", tail(out))
	}

	mutated := strings.Replace(body, p.From, p.To, 1)

	// 1b. Trivial-pin pre-screen (spec §9.8 R7): a mutation that changes only a comment or whitespace is
	//     semantically null — it compiles and breaks no test, so the break step would score it `survived`
	//     and send the actor to write a test for a comment. Reject it as malformed here, before the break
	//     step, so the fix agent rewrites the pin rather than chasing a phantom test gap. (Dead-code
	//     triviality is out of scope: no pure syntactic method detects reachability, and the spec states
	//     no reachability contract; the compile-then-break steps handle everything the token check does
	//     not.)
	if semanticallyNull(body, mutated) {
		return fail(PinMalformed, "the mutation %q -> %q changes only comments or whitespace; it is semantically null, so no test could catch it — rewrite the pin to mutate real behaviour", p.From, p.To)
	}

	// 2. Mutate.
	if err := os.WriteFile(target, []byte(mutated), 0o644); err != nil {
		return fail(PinUnverifiable, "could not write the mutation: %v", err)
	}

	// 3. It has to compile. A mutation that breaks the build fails every test for a reason that
	//    has nothing to do with the behaviour under test.
	if code, out, err := v.build(ctx, work); err != nil {
		return fail(PinUnverifiable, "build failed to execute: %v", err)
	} else if code != 0 {
		return fail(PinMalformed, "the mutation does not compile, so it proves nothing: %s", tail(out))
	}

	// 4. The mutation must be caught.
	code, out, err := v.run(ctx, work)
	if err != nil {
		return fail(PinUnverifiable, "mutated test run failed to execute: %v", err)
	}
	if code == 0 {
		return fail(PinSurvived, "the mutation survived: %s -> %s in %s left the tests passing, so %q does not hold this line",
			p.From, p.To, p.File, p.Test)
	}
	return PinResult{Pin: p, Proven: true, Outcome: PinProven,
		Detail: fmt.Sprintf("%s -> %s in %s fails the tests and the restored code passes them (%s)", p.From, p.To, p.File, tail(out))}
}

func (v Verifier) run(ctx context.Context, dir string) (int, string, error) {
	return v.exec(ctx, dir, v.TestCmd)
}

// build checks that the mutation still compiles — INCLUDING test files. `go build ./...` does not
// compile tests, so a mutation that renames a production symbol builds cleanly and then fails
// every test with "undefined: X": a build error scored as a kill. `go test -run ^$` compiles
// everything and runs nothing, which is the question actually being asked.
func (v Verifier) build(ctx context.Context, dir string) (int, string, error) {
	if len(v.BuildCmd) > 0 {
		return v.exec(ctx, dir, v.BuildCmd)
	}
	return v.exec(ctx, dir, []string{"go", "test", "-run", "^$", "./..."})
}

func (v Verifier) exec(ctx context.Context, dir string, argv []string) (int, string, error) {
	if v.Run != nil {
		return v.Run(ctx, dir, argv)
	}
	timeout := v.Timeout
	if timeout <= 0 {
		timeout = defaultVerifyTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) // #nosec G204 -- argv comes from the consent-hashed workflow, never the agent
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	// A signal-killed process HAS a ProcessState, and its ExitCode() is -1. Returning that as an
	// ordinary exit code told the caller "non-zero", which step 4 reads as "the mutation was
	// caught" — so a run killed by the timeout was recorded as PROOF, with a detail sentence
	// asserting tests that never finished had failed. That is the same class of defect as the
	// engine bug this package exists to correct, pointing the other way: theirs loses timeouts
	// from the score, ours counted them as kills.
	if ctx.Err() != nil {
		return -1, string(out), fmt.Errorf("the test command did not finish within %s: %w", timeout, ctx.Err())
	}
	if cmd.ProcessState != nil {
		if code := cmd.ProcessState.ExitCode(); code >= 0 {
			return code, string(out), nil
		}
		// Killed by a signal with no deadline of ours: still not an answer about the mutation.
		return -1, string(out), fmt.Errorf("the test command was killed by a signal: %v", cmd.ProcessState)
	}
	return -1, string(out), err
}

// tail keeps the last of a command's output: the failure is at the end, and the whole thing does
// not belong in a finding.
func tail(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 400 {
		s = "..." + s[len(s)-400:]
	}
	return strings.ReplaceAll(s, "\n", " / ")
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if info.IsDir() {
			if base := filepath.Base(p); base == ".git" || base == "node_modules" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		body, err := os.ReadFile(p) // #nosec G304 -- walking a caller-supplied tree by design
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), body, info.Mode().Perm())
	})
}

// FindingsForPins turns what verification found into findings. A proven pin produces nothing: it
// is the expected state, and reporting it would bury the one that matters.
//
// Severity follows the outcome, because the outcomes call for different work. An unheld fix is a
// defect in the code's tests and blocks. A malformed pin is a defect in the CLAIM — the fix may
// be perfect — so it is advisory: worth fixing, never evidence that the code is wrong. A tree
// that could not answer blocks, because nothing was learned and a gate must not read silence as
// success.
func FindingsForPins(results []PinResult) []findings.Input {
	out := make([]findings.Input, 0, len(results))
	for _, r := range results {
		if r.Outcome == PinProven {
			continue
		}
		f := findings.Input{
			Reviewer: "mutation-verify",
			Found:    r.Detail,
			Evidence: []findings.Evidence{{Type: "pin", Path: r.Pin.File}},
			// A short digest of the anchor, not the raw source: From can be multi-line and unbounded,
			// which makes a raw fingerprint fragile as a dedupe key.
			Fingerprint: fmt.Sprintf("mutation-verify:%s:%s:%x", r.Outcome, r.Pin.File, sha256.Sum256([]byte(r.Pin.From))),
		}
		switch r.Outcome {
		case PinSurvived:
			f.Severity, f.Classification = "high", "blocking"
			f.Title = "Unproven fix: " + r.Pin.File
			f.Finding = fmt.Sprintf("The fix claims %q holds %s, but breaking that line did not make the tests fail.", r.Pin.Test, r.Pin.File)
			f.Expected = "Breaking the fixed line makes the named test fail, and restoring it makes the test pass."
			f.Recommendation = "Add a test that fails under this mutation, or withdraw the claim. A fix nothing holds is a fix nothing keeps."
		case PinMalformed:
			f.Severity, f.Classification = "medium", "advisory"
			f.Title = "Unusable pin: " + r.Pin.File
			f.Finding = fmt.Sprintf("The pin for %s could not be evaluated, so the fix was neither proved nor disproved.", r.Pin.File)
			f.Expected = "A pin names text that appears exactly once and mutates it into something that still compiles."
			f.Recommendation = "Rewrite the pin. This says nothing about the fix itself: the claim could not be checked, not that it was false."
		default:
			f.Severity, f.Classification = "high", "blocking"
			f.Title = "Verification could not run: " + r.Pin.File
			f.Finding = "The tree could not answer whether this fix is held: " + r.Detail
			f.Expected = "The test suite passes before mutation, so a failure after it means something."
			f.Recommendation = "Fix the tree or the harness. Nothing was learned here, and an unanswered check is not a passed one."
		}
		out = append(out, f)
	}
	return out
}
