package mutation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// --- Parse: routing to a parser that then fails --------------------------------

// A report whose `files` value is an object routes to the Stryker parser; if that object is
// malformed for the schema, the error propagates out of Parse (it is not retried as gremlins).
func TestParseSurfacesStrykerParseError(t *testing.T) {
	// files is an object (-> Stryker) but mutants is the wrong type, so strykerReport unmarshal fails.
	if _, err := Parse([]byte(`{"files":{"a.go":{"mutants":"not-an-array"}}}`), "r.json"); err == nil {
		t.Fatalf("a malformed Stryker report must surface an error")
	}
}

// A report whose `files` value is an array routes to the gremlins parser; a malformed array
// surfaces the gremlins parse error.
func TestParseSurfacesGremlinsParseError(t *testing.T) {
	if _, err := Parse([]byte(`{"files":[{"mutations":"not-an-array"}]}`), "r.json"); err == nil {
		t.Fatalf("a malformed gremlins report must surface an error")
	}
}

// --- runProc: timeout, self-kill signal, and start failure ---------------------

func TestRunProcTimesOut(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	code, _, err := runProc(context.Background(), t.TempDir(), []string{"sh", "-c", "sleep 5"}, time.Millisecond)
	if err == nil || code != -1 {
		t.Fatalf("a run exceeding the timeout must error with code -1, got code=%d err=%v", code, err)
	}
}

func TestRunProcReportsSignalKill(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh/kill")
	}
	// The process kills itself with SIGKILL while the context is still live, so ProcessState exists
	// with a negative exit code and this is reported as a signal death, not an ordinary exit.
	code, _, err := runProc(context.Background(), t.TempDir(), []string{"sh", "-c", "kill -KILL $$"}, time.Minute)
	if err == nil || code != -1 {
		t.Fatalf("a signal-killed run must error with code -1, got code=%d err=%v", code, err)
	}
}

func TestRunProcReportsStartFailure(t *testing.T) {
	code, _, err := runProc(context.Background(), t.TempDir(), []string{"mrv-no-such-binary-xyzzy"}, time.Minute)
	if err == nil || code != -1 {
		t.Fatalf("a command that cannot start must error with code -1, got code=%d err=%v", code, err)
	}
}

// --- Reproducer.git (real shell-out): non-ExitError start failure --------------

// The real git seam (r.Git == nil) surfaces a start failure (e.g. a non-existent working dir) as an
// error, distinct from a git process that ran and exited non-zero.
func TestReproducerRealGitSurfacesStartFailure(t *testing.T) {
	r := Reproducer{Dir: filepath.Join(t.TempDir(), "does-not-exist")}
	if _, _, err := r.git(context.Background(), "status"); err == nil {
		t.Fatalf("git in a non-existent dir must surface a start error")
	}
}

// --- reproduceOne: deleted TEST file that cannot be removed ---------------------

// A deleted test file whose worktree path is a non-empty directory cannot be removed during the
// test-overlay step: unverifiable, naming the removal failure.
func TestReproduceDeletedTestRemoveError(t *testing.T) {
	g := &fakeGit{
		partition: "A\tpkg/new_test.go\nD\tpkg/busy_test.go\n",
		showBody:  map[string]string{"pkg/new_test.go": "t"},
	}
	r := newReproducer(t, g, (&seqRunner{}).run)
	parent := t.TempDir()
	r.MkWork = func() (string, error) { return parent, nil }
	if err := os.MkdirAll(filepath.Join(parent, "wt", "pkg", "busy_test.go", "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := reproduceOne(t, r, "TestX")
	mustOutcome(t, got, PinUnverifiable)
	if !strings.Contains(got.Detail, "could not remove deleted test file") {
		t.Fatalf("detail must name the deleted-test removal failure: %s", got.Detail)
	}
}

// --- reproduceOne: overlay whose destination directory cannot be created --------

// When a parent path component of an overlaid file is an existing regular file, MkdirAll fails and
// the overlay error is surfaced.
func TestReproduceOverlayMkdirError(t *testing.T) {
	g := &fakeGit{partition: "A\tsub/x_test.go\n", showBody: map[string]string{"sub/x_test.go": "t"}}
	r := newReproducer(t, g, (&seqRunner{}).run)
	parent := t.TempDir()
	r.MkWork = func() (string, error) { return parent, nil }
	// wt/sub is a FILE, so MkdirAll(wt/sub) for the overlay destination fails with ENOTDIR.
	if err := os.MkdirAll(filepath.Join(parent, "wt"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "wt", "sub"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := reproduceOne(t, r, "TestX")
	mustOutcome(t, got, PinUnverifiable)
	if !strings.Contains(got.Detail, "could not overlay test file") {
		t.Fatalf("detail must name the overlay failure: %s", got.Detail)
	}
}

// --- ScopeSuite: the suiteRun failure branches ---------------------------------

func TestScopeSuiteSurfacesWorkDirFailure(t *testing.T) {
	r := Reproducer{
		Dir: t.TempDir(), PreFixSHA: "pre", PostFixSHA: "post", TestCmd: []string{"go", "test"},
		Git:    (&fakeGit{}).run,
		MkWork: func() (string, error) { return "", errors.New("mkwork boom") },
	}
	res := r.ScopeSuite(context.Background())
	if res.Outcome != PinUnverifiable {
		t.Fatalf("a work-dir failure must be unverifiable: %+v", res)
	}
}

func TestScopeSuiteSurfacesWorktreeAddFailure(t *testing.T) {
	r := Reproducer{
		Dir: t.TempDir(), PreFixSHA: "pre", PostFixSHA: "post", TestCmd: []string{"go", "test"},
		Git:    (&fakeGit{addErr: errors.New("add boom")}).run,
		MkWork: func() (string, error) { return t.TempDir(), nil },
	}
	res := r.ScopeSuite(context.Background())
	if res.Outcome != PinUnverifiable {
		t.Fatalf("a worktree-add failure must be unverifiable: %+v", res)
	}
}

// When `worktree remove` cannot complete, ScopeSuite's cleanup deletes the worktree dir itself and
// prunes — exercised here with a green pre/post suite so both suiteRun cleanups run.
func TestScopeSuitePrunesWhenRemoveFails(t *testing.T) {
	g := &fakeGit{removeCode: 1}
	r := Reproducer{
		Dir: t.TempDir(), PreFixSHA: "pre", PostFixSHA: "post", TestCmd: []string{"go", "test"},
		Git:    g.run,
		Run:    func(context.Context, string, []string) (int, string, error) { return 0, "", nil },
		MkWork: func() (string, error) { return t.TempDir(), nil },
	}
	res := r.ScopeSuite(context.Background())
	if res.Outcome != PinProven {
		t.Fatalf("a green pre/post suite must be proven: %+v", res)
	}
	if !g.pruned {
		t.Fatalf("a failed worktree remove must fall back to prune")
	}
}

// --- Verifier.exec: start failure ----------------------------------------------

func TestVerifierExecReportsStartFailure(t *testing.T) {
	code, _, err := Verifier{}.exec(context.Background(), t.TempDir(), []string{"mrv-no-such-binary-xyzzy"})
	if err == nil || code != -1 {
		t.Fatalf("a command that cannot start must error with code -1, got code=%d err=%v", code, err)
	}
}

// --- copyTree: Walk error, unreadable file, and the Rel seam --------------------

func TestCopyTreeSurfacesWalkError(t *testing.T) {
	if err := copyTree(filepath.Join(t.TempDir(), "does-not-exist"), t.TempDir()); err == nil {
		t.Fatalf("copying a non-existent tree must error")
	}
}

func TestCopyTreeSurfacesUnreadableFile(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("unreadable-file case needs a non-root POSIX host")
	}
	src := t.TempDir()
	secret := filepath.Join(src, "secret.txt")
	if err := os.WriteFile(secret, []byte("x"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(secret, 0o644) })
	if err := copyTree(src, t.TempDir()); err == nil {
		t.Fatalf("an unreadable file must surface a copy error")
	}
}

// copyTree's filepath.Rel guard is unreachable in normal walks (Walk yields paths under src); the
// seam forces it.
func TestCopyTreeSurfacesRelError(t *testing.T) {
	real := copyTreeRel
	t.Cleanup(func() { copyTreeRel = real })
	copyTreeRel = func(string, string) (string, error) { return "", errors.New("rel boom") }

	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyTree(src, t.TempDir()); err == nil {
		t.Fatalf("a Rel failure must surface a copy error")
	}
}

// --- verifyOne: MkdirTemp, copyTree, and WriteFile failures ---------------------

// A working-copy temp dir that cannot be created (TMPDIR points nowhere) is unverifiable.
func TestVerifyOneSurfacesWorkDirFailure(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "no", "such", "dir"))
	v := Verifier{Dir: t.TempDir(), TestCmd: []string{"go", "test"}}
	res, err := v.Verify(context.Background(), []Pin{{File: "a.go", From: "x", To: "y", Test: "T"}})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res[0].Outcome != PinUnverifiable {
		t.Fatalf("a work-dir failure must be unverifiable: %+v", res[0])
	}
}

// A source tree that cannot be copied (it does not exist) is unverifiable.
func TestVerifyOneSurfacesCopyFailure(t *testing.T) {
	v := Verifier{Dir: filepath.Join(t.TempDir(), "does-not-exist"), TestCmd: []string{"go", "test"}}
	res, err := v.Verify(context.Background(), []Pin{{File: "a.go", From: "x", To: "y", Test: "T"}})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res[0].Outcome != PinUnverifiable {
		t.Fatalf("a copy failure must be unverifiable: %+v", res[0])
	}
}

// A read-only target file (0444, preserved by copyTree) passes the baseline but cannot be
// mutated: writing the mutation fails, and the pin is unverifiable.
func TestVerifyOneSurfacesWriteFailure(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("read-only-write case needs a non-root POSIX host")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "calc.go")
	if err := os.WriteFile(target, []byte("package p\nconst A = 1\n"), 0o444); err != nil {
		t.Fatal(err)
	}
	v := Verifier{
		Dir:     dir,
		TestCmd: []string{"go", "test"},
		// Baseline passes so we reach the write step; the change is real (not semantically null).
		Run: func(context.Context, string, []string) (int, string, error) { return 0, "", nil },
	}
	res, err := v.Verify(context.Background(), []Pin{{File: "calc.go", From: "A = 1", To: "A = 2", Test: "T"}})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res[0].Outcome != PinUnverifiable {
		t.Fatalf("a write failure must be unverifiable: %+v", res[0])
	}
}
