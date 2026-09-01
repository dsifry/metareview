package mutation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---- fake seams for the decision table ----

type runResp struct {
	code int
	out  string
	err  error
}

// fakeGit answers the exact git verbs the engine issues. Every response is a field so a test dials
// in one edge without a real repository.
type fakeGit struct {
	partition       string // stdout for `diff --name-status`
	partitionCode   int
	partitionErr    error
	addCode         int
	addErr          error
	showBody        map[string]string // path -> body for `show <sha>:<path>`
	showCode        int
	showErr         error
	showErrOnce     map[string]bool // paths whose first show returns showErr
	removedWorktree bool
	removeCode      int   // exit code returned by `worktree remove`
	removeErr       error // error returned by `worktree remove`
	pruned          bool  // set when `worktree prune` is issued
}

// gitVerb returns the subcommand and its following args, skipping leading `-c <value>` config pairs
// (the engine prefixes some calls with `-c core.quotePath=false`).
func gitVerb(args []string) (string, []string) {
	i := 0
	for i+1 < len(args) && args[i] == "-c" {
		i += 2
	}
	if i >= len(args) {
		return "", nil
	}
	return args[i], args[i+1:]
}

func (f *fakeGit) run(_ context.Context, args ...string) (string, int, error) {
	verb, rest := gitVerb(args)
	switch {
	case verb == "diff":
		return f.partition, f.partitionCode, f.partitionErr
	case verb == "worktree" && rest[0] == "add":
		return "", f.addCode, f.addErr
	case verb == "worktree" && rest[0] == "remove":
		f.removedWorktree = true
		return "", f.removeCode, f.removeErr
	case verb == "worktree" && rest[0] == "prune":
		f.pruned = true
		return "", 0, nil
	case verb == "show":
		spec := args[len(args)-1]
		path := spec[strings.Index(spec, ":")+1:]
		if len(f.showErrOnce) > 0 { // targeted mode: only the named paths fail
			if f.showErrOnce[path] {
				return "", 0, f.showErr
			}
			return f.showBody[path], f.showCode, nil
		}
		if f.showErr != nil {
			return "", 0, f.showErr
		}
		return f.showBody[path], f.showCode, nil
	}
	return "", 0, nil
}

// seqRunner returns queued responses in order and records the dirs/argv it saw.
type seqRunner struct {
	resp    []runResp
	i       int
	gotDirs []string
	gotArgv [][]string
}

func (s *seqRunner) run(_ context.Context, dir string, argv []string) (int, string, error) {
	s.gotDirs = append(s.gotDirs, dir)
	s.gotArgv = append(s.gotArgv, argv)
	r := s.resp[s.i]
	s.i++
	return r.code, r.out, r.err
}

func newReproducer(t *testing.T, g *fakeGit, run func(context.Context, string, []string) (int, string, error)) Reproducer {
	t.Helper()
	return Reproducer{
		Dir: t.TempDir(), PreFixSHA: "pre", PostFixSHA: "post", TestCmd: []string{"go", "test", "./..."},
		Git: g.run, Run: run, MkWork: func() (string, error) { return t.TempDir(), nil },
	}
}

func reproduceOne(t *testing.T, r Reproducer, test string) ReproResult {
	t.Helper()
	res, err := r.Reproduce(context.Background(), []Proof{{Test: test}})
	if err != nil {
		t.Fatalf("Reproduce: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("got %d results, want 1", len(res))
	}
	return res[0]
}

// hasFlagValue reports whether argv contains flag immediately followed by value.
func hasFlagValue(argv []string, flag, value string) bool {
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == flag && argv[i+1] == value {
			return true
		}
	}
	return false
}

func mustOutcome(t *testing.T, got ReproResult, want Outcome) {
	t.Helper()
	if got.Outcome != want {
		t.Fatalf("outcome %q, want %q (detail: %s)", got.Outcome, want, got.Detail)
	}
	if got.Proven != (want == PinProven) {
		t.Fatalf("Proven=%v for outcome %q", got.Proven, want)
	}
}

// ---- the core property: a genuine fail-before(assertion)/pass-after is proven ----

func TestReproduceProvenPath(t *testing.T) {
	g := &fakeGit{
		partition: "A\tpkg/new_test.go\nM\tpkg/prod.go\nD\tpkg/old.go\nT\tpkg/typed.go\ngarbage-no-tab\n\n",
		showBody:  map[string]string{"pkg/new_test.go": "test body", "pkg/prod.go": "prod body", "pkg/typed.go": "typed body"},
	}
	// The engine removes the worktree on return, so the fix must be observed AT the pass-after run,
	// not after Reproduce completes.
	var prodAtPassAfter, argvAtFailBefore []string
	var prodPresent bool
	calls := 0
	run := func(_ context.Context, dir string, argv []string) (int, string, error) {
		calls++
		if calls == 1 {
			argvAtFailBefore = argv
			return 1, "=== RUN   TestX\n--- FAIL: TestX (0.00s)\nFAIL\tpkg\t0.1s\n", nil // fail-before: assertion
		}
		_, err := os.Stat(filepath.Join(dir, "pkg", "prod.go"))
		prodPresent = err == nil
		prodAtPassAfter = argv
		return 0, "=== RUN   TestX\n--- PASS: TestX (0.00s)\nok  \tpkg\t0.1s\n", nil // pass-after
	}
	r := newReproducer(t, g, run)
	res := reproduceOne(t, r, "TestX")
	mustOutcome(t, res, PinProven)
	// A Proven result must carry the pre-fix assertion output for the §9.2 symptom reviewer.
	if !strings.Contains(res.FailBefore, "--- FAIL: TestX") {
		t.Fatalf("Proven result must carry the fail-before output: %q", res.FailBefore)
	}

	// The pre-fix run must be narrowed to the target test with an anchored -run, verbosely (-v so the
	// target's own markers appear).
	if !hasFlagValue(argvAtFailBefore, "-run", "^TestX$") || argvAtFailBefore[len(argvAtFailBefore)-1] != "-v" {
		t.Fatalf("target test must be selected with an anchored -run and -v: %v", argvAtFailBefore)
	}
	if !prodPresent {
		t.Fatalf("the fix's production file must be applied before the pass-after run (argv %v)", prodAtPassAfter)
	}
	if !g.removedWorktree {
		t.Fatalf("the throwaway worktree must be removed")
	}
	if g.pruned {
		t.Fatalf("a successful worktree remove must not fall back to prune")
	}
}

// The deleted file the fix removes must be gone from the worktree after step (d).
func TestReproduceAppliesDeletion(t *testing.T) {
	g := &fakeGit{
		partition: "A\tpkg/new_test.go\nD\tpkg/gone.go\n",
		showBody:  map[string]string{"pkg/new_test.go": "t"},
	}
	// Observe the deletion AT the pass-after run: the engine removes the worktree on return.
	var goneAbsent bool
	calls := 0
	run := func(_ context.Context, dir string, _ []string) (int, string, error) {
		calls++
		if calls == 1 {
			return 1, "=== RUN   TestX\n--- FAIL: TestX\n", nil
		}
		_, err := os.Stat(filepath.Join(dir, "pkg", "gone.go"))
		goneAbsent = os.IsNotExist(err)
		return 0, "=== RUN   TestX\n--- PASS: TestX\nok\n", nil
	}
	r := newReproducer(t, g, run)
	// Pre-create the file the fix deletes so step (d) has something to remove.
	wtParent := t.TempDir()
	r.MkWork = func() (string, error) { return wtParent, nil }
	if err := os.MkdirAll(filepath.Join(wtParent, "wt", "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtParent, "wt", "pkg", "gone.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustOutcome(t, reproduceOne(t, r, "TestX"), PinProven)
	if !goneAbsent {
		t.Fatal("the deleted file must be removed in step (d) before the pass-after run")
	}
}

// A deletion whose target is a non-empty directory cannot be removed: unverifiable, not a crash.
func TestReproduceDeletionRemoveError(t *testing.T) {
	g := &fakeGit{partition: "A\tpkg/new_test.go\nD\tpkg/busy\n", showBody: map[string]string{"pkg/new_test.go": "t"}}
	run := &seqRunner{resp: []runResp{{code: 1, out: "=== RUN   TestX\n--- FAIL: TestX\n"}}}
	r := newReproducer(t, g, run.run)
	wtParent := t.TempDir()
	r.MkWork = func() (string, error) { return wtParent, nil }
	// A non-empty directory at the delete path forces os.Remove to fail with something other than
	// IsNotExist.
	if err := os.MkdirAll(filepath.Join(wtParent, "wt", "pkg", "busy", "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := reproduceOne(t, r, "TestX")
	mustOutcome(t, got, PinUnverifiable)
	if !strings.Contains(got.Detail, "could not remove deleted file") {
		t.Fatalf("detail must name the removal failure: %s", got.Detail)
	}
}

// When `worktree remove` cannot complete (e.g. a cancelled run), the engine deletes the worktree
// directory itself and THEN prunes — because `git worktree prune` only drops a registration whose
// directory is already gone. Pruning before the dir is removed would be a no-op and orphan the
// .git/worktrees entry, and the fixed leaf name "wt" would then break a later `worktree add`.
func TestReproducePrunesWhenRemoveFails(t *testing.T) {
	parent := t.TempDir()
	wt := filepath.Join(parent, "wt")
	var wtGoneAtPrune, pruned bool
	git := func(_ context.Context, args ...string) (string, int, error) {
		verb, rest := gitVerb(args)
		switch {
		case verb == "diff":
			return "A\tpkg/new_test.go\n", 0, nil
		case verb == "show":
			return "t", 0, nil
		case verb == "worktree" && rest[0] == "add":
			return "", 0, nil
		case verb == "worktree" && rest[0] == "remove":
			return "", 1, nil // remove could not complete
		case verb == "worktree" && rest[0] == "prune":
			pruned = true
			_, err := os.Stat(wt)
			wtGoneAtPrune = os.IsNotExist(err)
			return "", 0, nil
		}
		return "", 0, nil
	}
	run := &seqRunner{resp: []runResp{
		{code: 1, out: "=== RUN   TestX\n--- FAIL: TestX\n"},
		{code: 0, out: "=== RUN   TestX\n--- PASS: TestX\nok\n"},
	}}
	r := Reproducer{Dir: t.TempDir(), PreFixSHA: "pre", PostFixSHA: "post", TestCmd: []string{"go", "test", "./..."},
		Git: git, Run: run.run, MkWork: func() (string, error) { return parent, nil }}
	mustOutcome(t, reproduceOne(t, r, "TestX"), PinProven)
	if !pruned {
		t.Fatal("a failed worktree remove must fall back to prune")
	}
	if !wtGoneAtPrune {
		t.Fatal("the worktree dir must be deleted BEFORE prune, or prune is a no-op and orphans metadata")
	}
}

// ---- fail-before classification (the load-bearing assertion-vs-compile axis) ----

func TestReproduceFailBeforeClassification(t *testing.T) {
	for _, tc := range []struct {
		name string
		resp runResp
		want Outcome
		hint string
	}{
		{"pass-before is a test gap", runResp{code: 0, out: "=== RUN   TestX\n--- PASS: TestX\nok\tpkg\t0.1s\n"}, PinSurvived, "does not exercise the fault"},
		{"filter matched nothing", runResp{code: 0, out: "testing: warning: no tests to run\nPASS\n"}, PinMalformed, "was not found"},
		{"compile error is not an assertion", runResp{code: 1, out: "# pkg\n./x_test.go:1:1: undefined: Z\nFAIL\tpkg [build failed]\n"}, PinMalformed, "compile/import error"},
		{"non-assertion failure fails closed", runResp{code: 1, out: "some unexpected failure\n"}, PinMalformed, "not a recognizable test assertion"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := &fakeGit{partition: "A\tpkg/new_test.go\n", showBody: map[string]string{"pkg/new_test.go": "t"}}
			run := &seqRunner{resp: []runResp{tc.resp}}
			got := reproduceOne(t, newReproducer(t, g, run.run), "TestX")
			mustOutcome(t, got, tc.want)
			if !strings.Contains(got.Detail, tc.hint) {
				t.Fatalf("detail %q must contain %q", got.Detail, tc.hint)
			}
		})
	}
}

// ---- pass-after classification ----

func TestReproducePassAfterClassification(t *testing.T) {
	for _, tc := range []struct {
		name  string
		after runResp
		want  Outcome
		hint  string
	}{
		{"still failing means the fix did not hold", runResp{code: 1, out: "=== RUN   TestX\n--- FAIL: TestX\nFAIL\n"}, PinSurvived, "did not make the test pass"},
		{"vanished test is unverifiable", runResp{code: 0, out: "no tests to run\n"}, PinUnverifiable, "vanished"},
		{"post-fix build failure is unverifiable", runResp{code: 1, out: "FAIL\tpkg [build failed]\n"}, PinUnverifiable, "did not build"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := &fakeGit{partition: "A\tpkg/new_test.go\n", showBody: map[string]string{"pkg/new_test.go": "t"}}
			run := &seqRunner{resp: []runResp{{code: 1, out: "=== RUN   TestX\n--- FAIL: TestX\n"}, tc.after}}
			got := reproduceOne(t, newReproducer(t, g, run.run), "TestX")
			mustOutcome(t, got, tc.want)
			if !strings.Contains(got.Detail, tc.hint) {
				t.Fatalf("detail %q must contain %q", got.Detail, tc.hint)
			}
		})
	}
}

// ---- setup / precondition failures all fail closed to unverifiable ----

func TestReproduceEmptyProofs(t *testing.T) {
	res, err := (Reproducer{}).Reproduce(context.Background(), nil)
	if err != nil || res != nil {
		t.Fatalf("empty proofs: %v %v", res, err)
	}
}

func TestReproduceNoTestCmdAborts(t *testing.T) {
	r := Reproducer{Dir: t.TempDir(), PreFixSHA: "pre", PostFixSHA: "post"}
	if _, err := r.Reproduce(context.Background(), []Proof{{Test: "T"}}); err == nil {
		t.Fatal("a missing test command must abort the run, not mark every proof survived")
	}
}

func TestReproduceMissingAnchor(t *testing.T) {
	for _, sha := range []struct{ pre, post string }{{"", "post"}, {"pre", ""}} {
		r := Reproducer{Dir: t.TempDir(), PreFixSHA: sha.pre, PostFixSHA: sha.post, TestCmd: []string{"go", "test"}}
		res, err := r.Reproduce(context.Background(), []Proof{{Test: "T"}})
		if err != nil {
			t.Fatal(err)
		}
		mustOutcome(t, res[0], PinUnverifiable)
		if !strings.Contains(res[0].Detail, "no pre/post-fix anchor") {
			t.Fatalf("detail must name the missing anchor: %s", res[0].Detail)
		}
	}
}

func TestReproducePartitionFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name string
		g    *fakeGit
	}{
		{"git error", &fakeGit{partitionErr: errors.New("boom")}},
		{"nonzero exit", &fakeGit{partitionCode: 128}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newReproducer(t, tc.g, (&seqRunner{}).run)
			got := reproduceOne(t, r, "T")
			mustOutcome(t, got, PinUnverifiable)
			if !strings.Contains(got.Detail, "changed files") {
				t.Fatalf("detail must name the partition failure: %s", got.Detail)
			}
		})
	}
}

func TestReproduceEmptyTestIsMalformed(t *testing.T) {
	g := &fakeGit{partition: "A\tpkg/new_test.go\n"}
	got := reproduceOne(t, newReproducer(t, g, (&seqRunner{}).run), "")
	mustOutcome(t, got, PinMalformed)
	if !strings.Contains(got.Detail, "names no test") {
		t.Fatalf("detail must say the proof names no test: %s", got.Detail)
	}
}

func TestReproduceMkWorkError(t *testing.T) {
	g := &fakeGit{partition: "A\tpkg/new_test.go\n"}
	r := newReproducer(t, g, (&seqRunner{}).run)
	r.MkWork = func() (string, error) { return "", errors.New("no temp") }
	got := reproduceOne(t, r, "T")
	mustOutcome(t, got, PinUnverifiable)
	if !strings.Contains(got.Detail, "work dir") {
		t.Fatalf("detail must name the work dir failure: %s", got.Detail)
	}
}

func TestReproduceWorktreeAddFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name string
		g    *fakeGit
		hint string
	}{
		{"exec error", &fakeGit{partition: "A\tpkg/new_test.go\n", addErr: errors.New("boom")}, "could not create the pre-fix worktree"},
		{"nonzero exit", &fakeGit{partition: "A\tpkg/new_test.go\n", addCode: 128}, "git worktree add exited"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := reproduceOne(t, newReproducer(t, tc.g, (&seqRunner{}).run), "T")
			mustOutcome(t, got, PinUnverifiable)
			if !strings.Contains(got.Detail, tc.hint) {
				t.Fatalf("detail %q must contain %q", got.Detail, tc.hint)
			}
		})
	}
}

func TestReproduceTestOverlayFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name string
		g    *fakeGit
	}{
		{"show error", &fakeGit{partition: "A\tpkg/new_test.go\n", showErr: errors.New("boom")}},
		{"show nonzero exit", &fakeGit{partition: "A\tpkg/new_test.go\n", showBody: map[string]string{}, showCode: 128}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := reproduceOne(t, newReproducer(t, tc.g, (&seqRunner{}).run), "T")
			mustOutcome(t, got, PinUnverifiable)
			if !strings.Contains(got.Detail, "overlay test file") {
				t.Fatalf("detail must name the overlay failure: %s", got.Detail)
			}
		})
	}
}

func TestReproduceFailBeforeRunError(t *testing.T) {
	g := &fakeGit{partition: "A\tpkg/new_test.go\n", showBody: map[string]string{"pkg/new_test.go": "t"}}
	run := &seqRunner{resp: []runResp{{err: errors.New("exec failed")}}}
	got := reproduceOne(t, newReproducer(t, g, run.run), "T")
	mustOutcome(t, got, PinUnverifiable)
	if !strings.Contains(got.Detail, "pre-fix test run failed to execute") {
		t.Fatalf("detail must name the pre-fix run failure: %s", got.Detail)
	}
}

func TestReproducePassAfterRunError(t *testing.T) {
	g := &fakeGit{partition: "A\tpkg/new_test.go\n", showBody: map[string]string{"pkg/new_test.go": "t"}}
	run := &seqRunner{resp: []runResp{{code: 1, out: "=== RUN   T\n--- FAIL: T\n"}, {err: errors.New("exec failed")}}}
	got := reproduceOne(t, newReproducer(t, g, run.run), "T")
	mustOutcome(t, got, PinUnverifiable)
	if !strings.Contains(got.Detail, "post-fix test run failed to execute") {
		t.Fatalf("detail must name the post-fix run failure: %s", got.Detail)
	}
}

// The production-file overlay happens only after a valid assertion fail-before (step d), so its
// failure surfaces there.
func TestReproduceProdOverlayFailsClosed(t *testing.T) {
	g := &fakeGit{
		partition:   "A\tpkg/new_test.go\nM\tpkg/prod.go\n",
		showBody:    map[string]string{"pkg/new_test.go": "t"},
		showErrOnce: map[string]bool{"pkg/prod.go": true},
		showErr:     errors.New("boom"),
	}
	run := &seqRunner{resp: []runResp{{code: 1, out: "=== RUN   T\n--- FAIL: T\n"}}}
	got := reproduceOne(t, newReproducer(t, g, run.run), "T")
	mustOutcome(t, got, PinUnverifiable)
	if !strings.Contains(got.Detail, "could not apply the fix") {
		t.Fatalf("detail must name the fix-application failure: %s", got.Detail)
	}
}

func TestClassifyAndPredicates(t *testing.T) {
	if !isTestFile("a/b_test.go") || isTestFile("a/b.go") {
		t.Fatal("isTestFile must key on the _test.go suffix")
	}
	const run = "=== RUN   TestX\n"
	// Keyed on the target: exit 0 WITH the target's RUN marker is passed; without it the target never
	// ran, so it is absent (clsNoTest) — even amid sibling-package "no test files"/"no tests to run".
	if classify("TestX", 0, run+"--- PASS: TestX\nok\n") != clsPassed {
		t.Fatal("zero exit with the target's RUN marker is passed")
	}
	if classify("TestX", 0, "?   sibling [no test files]\nok  other\ttesting: warning: no tests to run\n") != clsNoTest {
		t.Fatal("zero exit without the target's RUN marker is clsNoTest, ignoring sibling-package noise")
	}
	// A build/setup/vet failure is compile — before the assertion check.
	if classify("TestX", 1, "FAIL [build failed]") != clsCompile || classify("TestX", 1, "[setup failed]") != clsCompile || classify("TestX", 1, "[vet failed]") != clsCompile {
		t.Fatal("a build/setup/vet failure is clsCompile")
	}
	// Compile beats assertion: a build-failed run that also prints the target's --- FAIL is still compile.
	if classify("TestX", 1, run+"--- FAIL: TestX\nFAIL\tpkg [build failed]\n") != clsCompile {
		t.Fatal("a compile failure must win over an assertion marker")
	}
	if classify("TestX", 1, run+"--- FAIL: TestX\n") != clsAssert {
		t.Fatal("the target's assertion failure is clsAssert")
	}
	// A non-zero exit whose target neither ran nor failed (e.g. a sibling package failed) is clsOther.
	if classify("TestX", 1, "mystery\n") != clsOther {
		t.Fatal("a nonzero exit with no target marker is clsOther")
	}
	// Ran but no target FAIL line (an odd non-assertion failure) is also clsOther, not a valid assertion.
	if classify("TestX", 1, run+"panic: boom\n") != clsOther {
		t.Fatal("ran but no target --- FAIL line is clsOther")
	}
	// A SIBLING test's failure is not the target's assertion: the FAIL check is keyed on the target.
	if classify("TestX", 1, run+"--- FAIL: TestOther (0.00s)\n") != clsOther {
		t.Fatal("a non-target --- FAIL line must not count as the target's assertion")
	}
}
