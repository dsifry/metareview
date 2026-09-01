package mutation

import (
	"context"
	"encoding/json"
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

// go test -json event-stream builders. reproduceOne runs a mock's output through
// GoConvention.ParseReport, so mocks emit test2json events, not console text.
func jRun(test string) string { return `{"Action":"run","Package":"pkg","Test":"` + test + `"}` + "\n" }
func jTerm(action, test string) string {
	return `{"Action":"` + action + `","Package":"pkg","Test":"` + test + `"}` + "\n"
}
func jOut(test, s string) string {
	b, _ := json.Marshal(s)
	return `{"Action":"output","Package":"pkg","Test":"` + test + `","Output":` + string(b) + `}` + "\n"
}

// jFailBefore is a target that ran and failed an assertion (a valid fail-before); its output carries a
// recognizable "assertion failed" line for the FailBefore checks. jPassAfter is a target that passed.
func jFailBefore(test string) string {
	return jRun(test) + jOut(test, "    x_test.go:1: assertion failed\n") + jTerm("fail", test)
}
func jPassAfter(test string) string { return jRun(test) + jTerm("pass", test) }

// jBuildFail is a package that failed to build (no per-test events). jNoTests is a run in which only a
// sibling package appeared and the target never ran.
func jBuildFail() string {
	return `{"Action":"output","Package":"pkg","Output":"# pkg [pkg.test]\n"}` + "\n" +
		`{"Action":"output","Package":"pkg","Output":"FAIL\tpkg [build failed]\n"}` + "\n" +
		`{"Action":"fail","Package":"pkg"}` + "\n"
}
func jNoTests() string {
	return `{"Action":"output","Package":"sib","Output":"testing: warning: no tests to run\n"}` + "\n"
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
			return 1, jFailBefore("TestX"), nil // fail-before: assertion
		}
		_, err := os.Stat(filepath.Join(dir, "pkg", "prod.go"))
		prodPresent = err == nil
		prodAtPassAfter = argv
		return 0, jPassAfter("TestX"), nil // pass-after
	}
	r := newReproducer(t, g, run)
	res := reproduceOne(t, r, "TestX")
	mustOutcome(t, res, PinProven)
	// A Proven result must carry the pre-fix assertion output for the §9.2 symptom reviewer.
	if !strings.Contains(res.FailBefore, "assertion failed") {
		t.Fatalf("Proven result must carry the fail-before output: %q", res.FailBefore)
	}

	// The pre-fix run must be narrowed to the target test with an anchored -run, and request the
	// structured report (-json) that classification reads.
	if !hasFlagValue(argvAtFailBefore, "-run", "^TestX$") || argvAtFailBefore[len(argvAtFailBefore)-1] != "-json" {
		t.Fatalf("target test must be selected with an anchored -run and -json: %v", argvAtFailBefore)
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

// A Proven result stores only a bounded, copied tail of the pre-fix output — a test that logs a lot
// before its assertion must not keep the whole buffer alive for the §9.2 reviewer.
func TestReproduceCapsFailBefore(t *testing.T) {
	// A huge output event, then a late "assertion failed" line and the terminal fail — so the assertion
	// lives in the tail the cap keeps.
	huge := jRun("T") + jOut("T", strings.Repeat("x", maxFailBeforeBytes+5000)) + jOut("T", "assertion failed\n") + jTerm("fail", "T")
	g := &fakeGit{partition: "A\tpkg/new_test.go\n", showBody: map[string]string{"pkg/new_test.go": "t"}}
	run := &seqRunner{resp: []runResp{{code: 1, out: huge}, {code: 0, out: jPassAfter("T")}}}
	res := reproduceOne(t, newReproducer(t, g, run.run), "T")
	mustOutcome(t, res, PinProven)
	if len(res.FailBefore) > maxFailBeforeBytes {
		t.Fatalf("FailBefore must be capped to its tail: %d > %d", len(res.FailBefore), maxFailBeforeBytes)
	}
	if !strings.Contains(res.FailBefore, "assertion failed") {
		t.Fatal("the cap must keep the tail (where the assertion is)")
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
			return 1, jFailBefore("TestX"), nil
		}
		_, err := os.Stat(filepath.Join(dir, "pkg", "gone.go"))
		goneAbsent = os.IsNotExist(err)
		return 0, jPassAfter("TestX"), nil
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
	run := &seqRunner{resp: []runResp{{code: 1, out: jFailBefore("TestX")}}}
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
		{code: 1, out: jFailBefore("TestX")},
		{code: 0, out: jPassAfter("TestX")},
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
		{"pass-before is a test gap", runResp{code: 0, out: jPassAfter("TestX")}, PinSurvived, "does not exercise the fault"},
		{"filter matched nothing", runResp{code: 0, out: jNoTests()}, PinMalformed, "was not found"},
		{"compile error is not an assertion", runResp{code: 1, out: jBuildFail()}, PinMalformed, "compile/import error"},
		{"unreadable failing output is unverifiable", runResp{code: 1, out: "some unexpected failure\n"}, PinUnverifiable, "failed to execute"},
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
		{"still failing means the fix did not hold", runResp{code: 1, out: jFailBefore("TestX")}, PinSurvived, "did not make the test pass"},
		{"vanished test is unverifiable", runResp{code: 0, out: jNoTests()}, PinUnverifiable, "vanished"},
		{"post-fix build failure is unverifiable", runResp{code: 1, out: jBuildFail()}, PinUnverifiable, "did not build"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := &fakeGit{partition: "A\tpkg/new_test.go\n", showBody: map[string]string{"pkg/new_test.go": "t"}}
			run := &seqRunner{resp: []runResp{{code: 1, out: jFailBefore("TestX")}, tc.after}}
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
	run := &seqRunner{resp: []runResp{{code: 1, out: jFailBefore("T")}, {err: errors.New("exec failed")}}}
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
	run := &seqRunner{resp: []runResp{{code: 1, out: jFailBefore("T")}}}
	got := reproduceOne(t, newReproducer(t, g, run.run), "T")
	mustOutcome(t, got, PinUnverifiable)
	if !strings.Contains(got.Detail, "could not apply the fix") {
		t.Fatalf("detail must name the fix-application failure: %s", got.Detail)
	}
}

// ScopeSuite runs the full test command (no -run) on both trees: green→green passes, green→red blocks,
// a red or unrunnable baseline is unverifiable.
func TestScopeSuite(t *testing.T) {
	newR := func(g *fakeGit, run func(context.Context, string, []string) (int, string, error)) Reproducer {
		return Reproducer{Dir: t.TempDir(), PreFixSHA: "pre", PostFixSHA: "post", TestCmd: []string{"go", "test", "./..."},
			Git: g.run, Run: run, MkWork: func() (string, error) { return t.TempDir(), nil }}
	}
	okGit := func() *fakeGit { return &fakeGit{} }

	// green baseline, green post → proven.
	seq := &seqRunner{resp: []runResp{{code: 0, out: "ok\n"}, {code: 0, out: "ok\n"}}}
	if r := newR(okGit(), seq.run).ScopeSuite(context.Background()); r.Outcome != PinProven {
		t.Fatalf("green→green must be proven: %+v", r)
	}
	// the scope run must NOT carry a -run filter (it is the WHOLE suite).
	for _, argv := range seq.gotArgv {
		if hasFlagValue(argv, "-run", argv[len(argv)-1]) || (len(argv) > 0 && argv[len(argv)-1] == "-v") {
			t.Fatalf("the scope suite must run the full command, no -run/-v: %v", argv)
		}
	}
	// green baseline, red post → survived (regressed an existing test).
	seq = &seqRunner{resp: []runResp{{code: 0, out: "ok\n"}, {code: 1, out: "--- FAIL: TestOther\n"}}}
	if r := newR(okGit(), seq.run).ScopeSuite(context.Background()); r.Outcome != PinSurvived || !strings.Contains(r.Detail, "green→red") {
		t.Fatalf("green→red must be survived: %+v", r)
	}
	// red baseline → unverifiable (cannot attribute).
	seq = &seqRunner{resp: []runResp{{code: 1, out: "--- FAIL: TestPre\n"}}}
	if r := newR(okGit(), seq.run).ScopeSuite(context.Background()); r.Outcome != PinUnverifiable || !strings.Contains(r.Detail, "not green") {
		t.Fatalf("a red baseline must be unverifiable: %+v", r)
	}
	// baseline run error → unverifiable.
	seq = &seqRunner{resp: []runResp{{err: errors.New("boom")}}}
	if r := newR(okGit(), seq.run).ScopeSuite(context.Background()); r.Outcome != PinUnverifiable || !strings.Contains(r.Detail, "baseline") {
		t.Fatalf("a baseline exec error must be unverifiable: %+v", r)
	}
	// post run error → unverifiable.
	seq = &seqRunner{resp: []runResp{{code: 0, out: "ok\n"}, {err: errors.New("boom")}}}
	if r := newR(okGit(), seq.run).ScopeSuite(context.Background()); r.Outcome != PinUnverifiable || !strings.Contains(r.Detail, "post-deletion") {
		t.Fatalf("a post exec error must be unverifiable: %+v", r)
	}
	// worktree add failure → unverifiable (surfaced through the baseline run).
	if r := newR(&fakeGit{addCode: 128}, (&seqRunner{}).run).ScopeSuite(context.Background()); r.Outcome != PinUnverifiable {
		t.Fatalf("a worktree add failure must be unverifiable: %+v", r)
	}
	// misconfiguration / missing anchor.
	if r := (Reproducer{PreFixSHA: "pre", PostFixSHA: "post"}).ScopeSuite(context.Background()); r.Outcome != PinUnverifiable {
		t.Fatalf("no test command → unverifiable: %+v", r)
	}
	if r := (Reproducer{TestCmd: []string{"go", "test"}}).ScopeSuite(context.Background()); r.Outcome != PinUnverifiable {
		t.Fatalf("missing anchor → unverifiable: %+v", r)
	}
}
