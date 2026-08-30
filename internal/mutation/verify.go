package mutation

import (
	"context"
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

// PinResult is what verification found. Proven is the only value a gate should accept, and it is
// true only when BOTH halves held: the mutation failed the tests and the restored code passed
// them. Either half alone proves nothing — a suite that always fails "detects" everything, and
// one that always passes detects nothing.
type PinResult struct {
	Pin    Pin
	Proven bool
	Detail string
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
	fail := func(format string, a ...any) PinResult {
		return PinResult{Pin: p, Detail: fmt.Sprintf(format, a...)}
	}
	work, err := os.MkdirTemp("", "mrv-verify-")
	if err != nil {
		return fail("could not create a working copy: %v", err)
	}
	defer func() { _ = os.RemoveAll(work) }()
	if err := copyTree(v.Dir, work); err != nil {
		return fail("could not copy the tree: %v", err)
	}

	target := filepath.Join(work, filepath.FromSlash(p.File))
	original, err := os.ReadFile(target) // #nosec G304 -- path validated by the caller, inside a temp copy
	if err != nil {
		return fail("could not read %s in the copy: %v", p.File, err)
	}
	body := string(original)
	if n := strings.Count(body, p.From); n != 1 {
		// Zero occurrences means the pin does not describe this tree; more than one means the
		// mutation is ambiguous and we would be proving something other than what was claimed.
		return fail("the anchor text appears %d times in %s; it must appear exactly once", n, p.File)
	}

	// 1. Baseline: the restored tree must pass, or the mutation result means nothing.
	if code, out, err := v.run(ctx, work); err != nil {
		return fail("baseline test run failed to execute: %v", err)
	} else if code != 0 {
		return fail("the tests do not pass before mutating, so nothing can be concluded: %s", tail(out))
	}

	// 2. Mutate.
	if err := os.WriteFile(target, []byte(strings.Replace(body, p.From, p.To, 1)), 0o644); err != nil {
		return fail("could not write the mutation: %v", err)
	}

	// 3. It has to compile. A mutation that breaks the build fails every test for a reason that
	//    has nothing to do with the behaviour under test.
	if code, out, err := v.build(ctx, work); err != nil {
		return fail("build failed to execute: %v", err)
	} else if code != 0 {
		return fail("the mutation does not compile, so it proves nothing: %s", tail(out))
	}

	// 4. The mutation must be caught.
	code, out, err := v.run(ctx, work)
	if err != nil {
		return fail("mutated test run failed to execute: %v", err)
	}
	if code == 0 {
		return fail("the mutation survived: %s -> %s in %s left the tests passing, so %q does not hold this line",
			p.From, p.To, p.File, p.Test)
	}
	return PinResult{Pin: p, Proven: true,
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
	if cmd.ProcessState != nil {
		return cmd.ProcessState.ExitCode(), string(out), nil
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

// FindingsForPins turns unproven claims into blocking findings. A proven pin produces nothing:
// it is the expected state, and a gate that reported it would drown the one that matters.
func FindingsForPins(results []PinResult) []findings.Input {
	out := make([]findings.Input, 0, len(results))
	for _, r := range results {
		if r.Proven {
			continue
		}
		out = append(out, findings.Input{
			Reviewer:       "mutation-verify",
			Severity:       "high",
			Classification: "blocking",
			Title:          "Unproven fix: " + r.Pin.File,
			Finding: fmt.Sprintf("The fix claims %q holds %s, but breaking that line did not make the tests fail.",
				r.Pin.Test, r.Pin.File),
			Expected:       "Breaking the fixed line makes the named test fail, and restoring it makes the test pass.",
			Found:          r.Detail,
			Recommendation: "Add a test that fails under this mutation, or withdraw the claim. A fix nothing holds is a fix nothing keeps.",
			Evidence:       []findings.Evidence{{Type: "pin", Path: r.Pin.File}},
			Fingerprint:    "mutation-verify:unproven:" + r.Pin.File + ":" + r.Pin.From,
		})
	}
	return out
}
