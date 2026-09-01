package mutation

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Proof is a reproduction-form differential claim (spec §9.1): one committed test that must FAIL on
// the pre-fix tree — with the fix's test-only files overlaid — and PASS once the full fix is
// applied. Test is the test function the differential is attributed to; the engine selects it with
// `-run ^<Test>$`, so a bare fail→pass elsewhere in the suite cannot stand in for it.
type Proof struct {
	Test string
}

// ReproResult is what proving one reproduction proof found. It shares the four Outcome values and
// the Proven-only-when-PinProven rule with the pin PinResult, so the adapter maps it onto a
// run.ProofResult by value; the engine keeps its own type so this package does not depend on the FSM.
type ReproResult struct {
	Proof   Proof
	Proven  bool
	Outcome Outcome
	Detail  string
}

// Reproducer proves reproduction-form proofs against a REAL git worktree (maintainer-approved
// strategy). For each proof it:
//
//	(a) checks out the PRE-FIX tree (PreFixSHA) into a throwaway detached worktree;
//	(b) overlays ONLY the test-only files the fix adds or changes (helpers included, never
//	    production code) from the POST-FIX tree (PostFixSHA);
//	(c) runs the target test — it must FAIL with an ASSERTION, not a compile/import, error;
//	(d) applies the full fix (materializes every production/deletion change to reach PostFixSHA);
//	(e) re-runs the target test — it must PASS.
//
// The compile-vs-assertion distinction on the pre-fix run is the load-bearing axis (spec §9.2/§9.3):
// a fix's test that only fails to compile against the pre-fix tree proves nothing about the fault,
// so it is malformed, not a valid fail-before.
//
// It never mutates the repository: every tree it touches is a throwaway worktree removed afterward.
// The design is reused by T1.5's deletion proof — a deletion is fail-before/pass-after with the
// removed span present pre-fix and gone post-fix, which materializing PostFixSHA already effects.
type Reproducer struct {
	Dir        string        // repository root; git runs here
	PreFixSHA  string        // the pre-fix tree (run.Snapshot.FixEntryHead); "" fails closed
	PostFixSHA string        // the post-fix tree (run.Snapshot.Head)
	TestCmd    []string      // the consent-hashed base test command, e.g. [go test ./...]; never the agent's
	Timeout    time.Duration // per test run; <=0 uses defaultVerifyTimeout
	// Git runs a hardened git in Dir and returns stdout, the exit code, and a non-nil error only
	// when the process could not be run at all. nil uses a real shell-out. A seam for tests.
	Git func(ctx context.Context, args ...string) (stdout string, code int, err error)
	// Run runs argv in dir (a worktree) and returns exit code, combined output, and a non-nil error
	// only when the process could not be run or was killed. nil uses a real exec. A seam for tests.
	Run func(ctx context.Context, dir string, argv []string) (int, string, error)
	// MkWork creates the throwaway worktree's parent dir; nil uses os.MkdirTemp. A seam for tests.
	MkWork func() (string, error)
}

// Reproduce checks every proof and returns one result each, in order. A proof that cannot be checked
// is reported unproven (never Proven) with the reason; a per-proof tree failure is unverifiable, not
// a whole-run error, so one bad proof does not discard the evidence for the others. A missing test
// command is a misconfiguration that aborts the run — reporting every proof "survived" would blame
// the fixes for the harness's own fault (mirrors Verifier.Verify).
func (r Reproducer) Reproduce(ctx context.Context, proofs []Proof) ([]ReproResult, error) {
	if len(proofs) == 0 {
		return nil, nil
	}
	if len(r.TestCmd) == 0 {
		return nil, fmt.Errorf("mutation: no test command configured, so no reproduction proof can be checked")
	}
	out := make([]ReproResult, 0, len(proofs))
	// A same-iteration fork leaves FixEntryHead empty (spec §9.1): there is no pre-fix anchor, so
	// nothing can be concluded — fail closed rather than guess a baseline.
	if r.PreFixSHA == "" || r.PostFixSHA == "" {
		for _, p := range proofs {
			out = append(out, unproven(p, PinUnverifiable, "no pre/post-fix anchor to reproduce against (a same-iteration fork leaves FixEntryHead empty)"))
		}
		return out, nil
	}
	// The fix's changed-file partition is identical for every proof (same two trees), so compute it
	// once. A git failure here means no tree can be built: fail closed for every proof.
	part, err := r.changedPartition(ctx)
	if err != nil {
		for _, p := range proofs {
			out = append(out, unproven(p, PinUnverifiable, fmt.Sprintf("could not read the fix's changed files: %v", err)))
		}
		return out, nil
	}
	for _, p := range proofs {
		out = append(out, r.reproduceOne(ctx, p, part))
	}
	return out, nil
}

// unproven builds a non-Proven result for a proof the engine could not carry to proof.
func unproven(p Proof, o Outcome, detail string) ReproResult {
	return ReproResult{Proof: p, Outcome: o, Detail: detail}
}

// partition is the fix's changed files split by role. The pre-fix run must see the fix's TEST state
// against the OLD production, so ALL test-only changes — adds, modifies, AND deletes — are applied in
// step (b); production changes and production deletions land only when the fix is applied (step d).
// Applying test deletions in step (b) matters for a renamed test (a delete plus an add with rename
// detection off): leaving the old copy present would redeclare the moved test and fail to compile.
type partition struct {
	testFiles []string // *_test.go files the fix adds or changes — overlaid in step (b)
	delTests  []string // *_test.go files the fix deletes — removed in step (b), with the overlay
	prodFiles []string // non-test files the fix adds or changes — applied in step (d)
	delFiles  []string // non-test files the fix deletes — removed in step (d)
}

// isTestFile is the test-vs-production predicate (spec §9.1): the Go `*_test.go` convention, the same
// one packageHasTests keys on. A non-Go partition would key on its own test convention here.
func isTestFile(path string) bool { return strings.HasSuffix(path, "_test.go") }

// changedPartition lists the fix's changed files (PreFixSHA→PostFixSHA) split by role. Rename
// detection is disabled so a rename is a delete plus an add, which materialize correctly.
func (r Reproducer) changedPartition(ctx context.Context) (partition, error) {
	// core.quotePath=false keeps a non-ASCII path raw, so the byte string this parser hands to
	// `git show` and the worktree write is the one git resolves — a quoted "pkg/\303\251.go" would not.
	out, code, err := r.git(ctx, "-c", "core.quotePath=false", "diff", "--no-renames", "--name-status", "--no-color", "--end-of-options", r.PreFixSHA, r.PostFixSHA)
	if err != nil {
		return partition{}, err
	}
	if code != 0 {
		return partition{}, fmt.Errorf("git diff --name-status exited %d", code)
	}
	var part partition
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 2)
		if len(fields) != 2 || fields[0] == "" {
			continue
		}
		status, path := fields[0][0], fields[1]
		test := isTestFile(path)
		switch status {
		case 'D':
			if test {
				part.delTests = append(part.delTests, path)
			} else {
				part.delFiles = append(part.delFiles, path)
			}
		case 'A', 'M', 'T':
			if test {
				part.testFiles = append(part.testFiles, path)
			} else {
				part.prodFiles = append(part.prodFiles, path)
			}
		}
	}
	return part, nil
}

func (r Reproducer) reproduceOne(ctx context.Context, p Proof, part partition) ReproResult {
	fail := func(o Outcome, format string, a ...any) ReproResult {
		return unproven(p, o, fmt.Sprintf(format, a...))
	}
	// A proof that names no test can never be attributed: it is malformed (the CLAIM is bad).
	if p.Test == "" {
		return fail(PinMalformed, "the reproduction proof names no test")
	}
	parent, err := r.mkWork()
	if err != nil {
		return fail(PinUnverifiable, "could not create a work dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(parent) }()
	// git creates the leaf; a pre-existing non-empty path makes `worktree add` refuse.
	wt := filepath.Join(parent, "wt")

	// (a) check out the pre-fix tree.
	if _, code, err := r.git(ctx, "worktree", "add", "--detach", "--end-of-options", wt, r.PreFixSHA); err != nil {
		return fail(PinUnverifiable, "could not create the pre-fix worktree: %v", err)
	} else if code != 0 {
		return fail(PinUnverifiable, "git worktree add exited %d for %s", code, r.PreFixSHA)
	}
	// Cleanup must survive a cancelled run: if the proof's ctx is already cancelled, passing it here
	// makes git exit before deleting the .git/worktrees entry, and os.RemoveAll(parent) below then
	// leaves stale metadata in the MAIN repo. Use a cancellation-independent context, and prune if the
	// remove could not complete.
	defer func() {
		clean := context.WithoutCancel(ctx)
		if _, code, err := r.git(clean, "worktree", "remove", "--force", "--end-of-options", wt); err != nil || code != 0 {
			_, _, _ = r.git(clean, "worktree", "prune")
		}
	}()

	// (b) put the fix's TEST state onto the pre-fix tree: overlay the test files it adds or changes,
	//     and remove the ones it deletes (so a renamed test does not redeclare its moved function).
	for _, f := range part.testFiles {
		if err := r.overlay(ctx, wt, f); err != nil {
			return fail(PinUnverifiable, "could not overlay test file %s: %v", f, err)
		}
	}
	for _, f := range part.delTests {
		if err := removeInTree(wt, f); err != nil {
			return fail(PinUnverifiable, "could not remove deleted test file %s: %v", f, err)
		}
	}

	// (c) fail-before: the target test must FAIL with an assertion, not a compile/import, error.
	code, before, err := r.runTest(ctx, wt, p.Test)
	if err != nil {
		return fail(PinUnverifiable, "the pre-fix test run failed to execute: %v", err)
	}
	switch classify(p.Test, code, before) {
	case clsPassed:
		return fail(PinSurvived, "the test passes on the pre-fix tree, so it does not exercise the fault: %s", tail(before))
	case clsNoTest:
		return fail(PinMalformed, "the target test %q was not found in the pre-fix tree", p.Test)
	case clsCompile:
		return fail(PinMalformed, "the pre-fix failure is a compile/import error, not an assertion, so it proves nothing: %s", tail(before))
	case clsOther:
		return fail(PinMalformed, "the pre-fix failure was not a recognizable test assertion: %s", tail(before))
	}

	// (d) apply the full fix: bring every changed production file to its post-fix content and remove
	//     what the fix deletes. Test files are already overlaid, so the tree now equals PostFixSHA.
	for _, f := range part.prodFiles {
		if err := r.overlay(ctx, wt, f); err != nil {
			return fail(PinUnverifiable, "could not apply the fix (%s): %v", f, err)
		}
	}
	for _, f := range part.delFiles {
		if err := removeInTree(wt, f); err != nil {
			return fail(PinUnverifiable, "could not remove deleted file %s: %v", f, err)
		}
	}

	// (e) pass-after: the target test must now PASS.
	code, after, err := r.runTest(ctx, wt, p.Test)
	if err != nil {
		return fail(PinUnverifiable, "the post-fix test run failed to execute: %v", err)
	}
	switch classify(p.Test, code, after) {
	case clsPassed:
		return ReproResult{Proof: p, Proven: true, Outcome: PinProven,
			Detail: fmt.Sprintf("%q fails on the pre-fix tree (assertion) and passes once the fix is applied", p.Test)}
	case clsNoTest:
		return fail(PinUnverifiable, "the target test %q vanished from the post-fix tree", p.Test)
	case clsCompile:
		return fail(PinUnverifiable, "the post-fix tree did not build, so nothing can be concluded: %s", tail(after))
	default:
		return fail(PinSurvived, "applying the fix did not make the test pass; the differential did not hold: %s", tail(after))
	}
}

// overlay writes PostFixSHA:path into the worktree, creating parent directories. Byte-for-byte: the
// stdout of `git show` is the file body, and a test file's line numbers must survive intact.
func (r Reproducer) overlay(ctx context.Context, wt, path string) error {
	out, code, err := r.git(ctx, "show", "--end-of-options", r.PostFixSHA+":"+path)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("git show %s:%s exited %d", r.PostFixSHA, path, code)
	}
	dest := filepath.Join(wt, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, []byte(out), 0o644)
}

// removeInTree deletes path from the worktree, treating an already-absent path as success. A path
// that cannot be removed (e.g. a non-empty directory) is a real error.
func removeInTree(wt, path string) error {
	if err := os.Remove(filepath.Join(wt, filepath.FromSlash(path))); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// runTest runs the consent-hashed test command narrowed to the single target test, verbosely. The
// test name is regexp-quoted and anchored so -run selects exactly that test and cannot inject a flag
// (it is one argv element, never a shell string); -v makes the target's own `=== RUN`/`--- PASS`/
// `--- FAIL` markers appear, so classify keys on the target rather than inferring from suite noise.
func (r Reproducer) runTest(ctx context.Context, dir, test string) (int, string, error) {
	argv := append(append([]string(nil), r.TestCmd...), "-run", "^"+regexp.QuoteMeta(test)+"$", "-v")
	if r.Run != nil {
		return r.Run(ctx, dir, argv)
	}
	return runProc(ctx, dir, argv, r.Timeout)
}

func (r Reproducer) mkWork() (string, error) {
	if r.MkWork != nil {
		return r.MkWork()
	}
	return os.MkdirTemp("", "mrv-repro-")
}

// git runs a git command in Dir through the injected seam, or a real hardened shell-out when none is
// set. Production always injects the gate.Exec seam; the real form here backs only this package's own
// end-to-end test. It disables prompts, external diff/config, and pagers and scrubs GIT_* from the
// environment, matching gate.RealExec (which this package may not import — a layering boundary).
func (r Reproducer) git(ctx context.Context, args ...string) (string, int, error) {
	if r.Git != nil {
		return r.Git(ctx, args...)
	}
	full := append([]string{"-c", "core.fsmonitor=false", "-c", "diff.external=", "-c", "core.excludesFile=", "-c", "core.attributesFile=", "--no-pager"}, args...)
	cmd := exec.CommandContext(ctx, "git", full...) // #nosec G204 -- fixed git flags plus caller-controlled SHAs/paths, never a shell
	cmd.Dir = r.Dir
	env := cmd.Environ()[:0:0]
	for _, kv := range cmd.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			env = append(env, kv)
		}
	}
	cmd.Env = append(env, "GIT_TERMINAL_PROMPT=0", "LC_ALL=C", "GIT_EXTERNAL_DIFF=", "GIT_CONFIG_NOSYSTEM=1")
	var out, errb strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if err != nil {
		var ee *exec.ExitError
		if ok := asExitErr(err, &ee); ok {
			return out.String(), ee.ExitCode(), nil
		}
		return "", 0, err
	}
	return out.String(), 0, nil
}

func asExitErr(err error, target **exec.ExitError) bool {
	ee, ok := err.(*exec.ExitError)
	if ok {
		*target = ee
	}
	return ok
}

// failClass is how a test run's exit code and output classify against the assertion-vs-compile axis.
type failClass int

const (
	clsAssert  failClass = iota // exited non-zero with a real "--- FAIL:" assertion — a valid fail-before
	clsPassed                   // exited zero with tests actually run
	clsNoTest                   // exited zero but the -run filter matched nothing — the test is absent
	clsCompile                  // a compile/import/setup failure — NOT an assertion, so it proves nothing
	clsOther                    // exited non-zero but no assertion marker — fail closed on the assertion axis
)

// classify reads a verbose `go test -run ^<test>$ -v` run, keyed on the TARGET test's own markers.
// Because TestCmd is the repo-wide command (e.g. `go test ./...`), sibling packages print
// "[no test files]" and "no tests to run" for reasons unrelated to the target — so inferring the
// target's fate from those strings misfires in any multi-package repo. Instead:
//   - `=== RUN   <test>` proves the target actually ran;
//   - the compile check comes BEFORE the assertion check, so a build-failed run that still prints a
//     `--- FAIL:` from another package is compile, not a valid fail-before (the hole spec §9.3(b) closes);
//   - `--- FAIL: <test>` proves the target itself failed an assertion.
func classify(test string, code int, out string) failClass {
	ran := strings.Contains(out, "=== RUN   "+test)
	if code == 0 {
		if ran {
			return clsPassed
		}
		return clsNoTest // exit 0 but the target never ran → it is absent from the tree
	}
	if isCompileError(out) {
		return clsCompile
	}
	if ran && strings.Contains(out, "--- FAIL: "+test) {
		return clsAssert
	}
	return clsOther
}

// isCompileError reports a `go test` build/setup/vet failure — the failure modes that make a
// non-zero exit mean "the code did not compile", never "an assertion failed".
func isCompileError(out string) bool {
	return strings.Contains(out, "[build failed]") ||
		strings.Contains(out, "[setup failed]") ||
		strings.Contains(out, "[vet failed]")
}

// runProc runs argv in dir with a timeout, returning the exit code and combined output. A run killed
// by the deadline or a signal is an error, never a non-zero exit that a caller could read as a
// legitimate test failure (the same trap Verifier.exec documents).
func runProc(ctx context.Context, dir string, argv []string, timeout time.Duration) (int, string, error) {
	if timeout <= 0 {
		timeout = defaultVerifyTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) // #nosec G204 -- argv is the consent-hashed workflow command plus an anchored -run, never the agent's
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return -1, string(out), fmt.Errorf("the test command did not finish within %s: %w", timeout, ctx.Err())
	}
	if cmd.ProcessState != nil {
		if c := cmd.ProcessState.ExitCode(); c >= 0 {
			return c, string(out), nil
		}
		return -1, string(out), fmt.Errorf("the test command was killed by a signal: %v", cmd.ProcessState)
	}
	return -1, string(out), err
}
