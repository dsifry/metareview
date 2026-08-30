package gate

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/run"
)

const (
	shaA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	shaB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	shaF = "ffffffffffffffffffffffffffffffffffffffff"
)

func bugs(n int) []run.Bug {
	out := make([]run.Bug, n)
	for i := range out {
		out[i] = run.Bug{ID: strings.Repeat("a", 12), Desc: "d"}
	}
	return out
}

func TestG1Gates(t *testing.T) {
	ctx := context.Background()
	fnd := run.Snapshot{Findings: []run.Finding{{IssueText: "x"}}}
	cnf := run.Snapshot{Confirmed: bugs(2)}
	fixed := run.Snapshot{AllFound: bugs(3), Unfixed: 0}
	remain := run.Snapshot{AllFound: bugs(3), Unfixed: 2}
	cases := []struct {
		gate string
		s    run.Snapshot
		code string // "" → pass
	}{
		{"findings_nonempty", fnd, ""}, {"findings_nonempty", run.Snapshot{}, CodeNoFindings},
		{"findings_empty", run.Snapshot{}, ""}, {"findings_empty", fnd, CodeFindingsPresent},
		{"confirmed_nonempty", cnf, ""}, {"confirmed_nonempty", run.Snapshot{}, CodeNoConfirmed},
		{"confirmed_empty", run.Snapshot{}, ""}, {"confirmed_empty", cnf, CodeConfirmedPresent},
		{"all_fixed", fixed, ""}, {"all_fixed", remain, CodeBugsRemain}, {"all_fixed", run.Snapshot{}, CodeBugsRemain},
		{"bugs_remain", remain, ""}, {"bugs_remain", run.Snapshot{}, ""}, {"bugs_remain", fixed, CodeAllFixed},
		{"nothing_found", run.Snapshot{}, ""}, {"nothing_found", fnd, CodeFindingsPresent}, {"nothing_found", run.Snapshot{AllFound: bugs(1)}, CodeBugsKnown},
		{"nothing_confirmed", run.Snapshot{}, ""}, {"nothing_confirmed", cnf, CodeConfirmedPresent}, {"nothing_confirmed", run.Snapshot{AllFound: bugs(1), Unfixed: 1}, CodeBugsKnown},
	}
	for _, c := range cases {
		g, ok := Builtin(c.gate)
		if !ok {
			t.Fatalf("missing gate %s", c.gate)
		}
		err := g(ctx, c.s, &Fake{})
		if c.code == "" {
			if err != nil {
				t.Errorf("%s: unexpected %+v", c.gate, err)
			}
			continue
		}
		if err == nil || err.Code != c.code || err.Gate != c.gate || err.Detail == "" {
			t.Errorf("%s: got %+v want %s", c.gate, err, c.code)
		}
	}
	if _, ok := Builtin("nope"); ok {
		t.Fatal("unknown gate")
	}
	want := []string{"all_fixed", "bugs_remain", "commit_exists", "confirmed_empty", "confirmed_nonempty", "findings_empty", "findings_nonempty", "nothing_confirmed", "nothing_found", "pins_proven"}
	if strings.Join(Names(), ",") != strings.Join(want, ",") {
		t.Fatalf("Names: %v", Names())
	}
}

func TestG1CommitExists(t *testing.T) {
	ctx := context.Background()
	g, _ := Builtin("commit_exists")
	// inapplicable before the first fix entry
	if err := g(ctx, run.Snapshot{BaseSHA: shaB}, &Fake{HeadSHA: shaA}); err == nil || err.Code != CodeGateInapplicable {
		t.Fatalf("inapplicable: %+v", err)
	}
	snap := run.Snapshot{BaseSHA: shaB, FixEntryHead: shaF}
	// counts are keyed by (from, to): the gate must count from FixEntryHead, not BaseSHA
	f := &Fake{HeadSHA: shaA, Counts: map[string]int{shaF + ".." + shaA: 1, shaB + ".." + shaA: 0}, Clean: true}
	if err := g(ctx, snap, f); err != nil {
		t.Fatalf("pass: %+v", err)
	}
	if strings.Join(f.Calls, ";") != "[Head];[CommitCount "+shaF+" "+shaA+"];[Status]" {
		t.Fatalf("calls: %v", f.Calls)
	}
	// commits but dirty → ERR_NO_COMMIT with porcelain + working diff, capped
	big := strings.Repeat("x", run.MaxDetail)
	f = &Fake{HeadSHA: shaA, Counts: map[string]int{shaF + ".." + shaA: 2}, Porcelain: "1 .M N... 100644 100644 100644 0 0 f.go\n", Diffs: map[string]string{"HEAD": big}}
	err := g(ctx, snap, f)
	if err == nil || err.Code != CodeNoCommit || !strings.Contains(err.Detail, "f.go") || !strings.Contains(err.Detail, "2 commits") || !err.DetailTruncated || len(err.Detail) > run.MaxDetail {
		t.Fatalf("dirty: %+v", err)
	}
	// zero commits, clean
	f = &Fake{HeadSHA: shaA, Clean: true}
	if err := g(ctx, snap, f); err == nil || err.Code != CodeNoCommit || !strings.Contains(err.Detail, "0 commits since "+shaF) {
		t.Fatalf("no commits: %+v", err)
	}
	// each git call site failing individually → ERR_GIT
	boom := errors.New("boom")
	for _, failAt := range []string{"Head", "CommitCount", "Status", "WorkingDiff"} {
		f := &failingFake{Fake: Fake{HeadSHA: shaA, Counts: map[string]int{shaF + ".." + shaA: 0}}, failAt: failAt, err: boom}
		err := g(ctx, snap, f)
		if err == nil || err.Code != CodeGit || !strings.Contains(err.Detail, "boom") {
			t.Fatalf("fail at %s: %+v", failAt, err)
		}
	}
}

// failingFake fails exactly one method.
type failingFake struct {
	Fake
	failAt string
	err    error
}

func (f *failingFake) Head(ctx context.Context) (string, error) {
	if f.failAt == "Head" {
		return "", f.err
	}
	return f.Fake.Head(ctx)
}
func (f *failingFake) CommitCount(ctx context.Context, a, b string) (int, error) {
	if f.failAt == "CommitCount" {
		return 0, f.err
	}
	return f.Fake.CommitCount(ctx, a, b)
}
func (f *failingFake) Status(ctx context.Context) (bool, string, error) {
	if f.failAt == "Status" {
		return false, "", f.err
	}
	return f.Fake.Status(ctx)
}
func (f *failingFake) WorkingDiff(ctx context.Context, max int) (string, bool, error) {
	if f.failAt == "WorkingDiff" {
		return "", false, f.err
	}
	return f.Fake.WorkingDiff(ctx, max)
}

// ---- real git ----

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t", "GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func repo(t *testing.T) (string, string, string) {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	write(t, dir, "a.txt", "one\n")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-qm", "c1")
	c1 := git(t, dir, "rev-parse", "HEAD")
	write(t, dir, "a.txt", "one\ntwo é\n")
	git(t, dir, "commit", "-qam", "c2")
	c2 := git(t, dir, "rev-parse", "HEAD")
	return dir, c1, c2
}

func TestG2RealGit(t *testing.T) {
	ctx := context.Background()
	dir, c1, c2 := repo(t)
	g := NewExec(dir, RealExec)
	head, err := g.Head(ctx)
	if err != nil || head != c2 {
		t.Fatalf("head %s %v", head, err)
	}
	git(t, dir, "tag", "-a", "v1", "-m", "tag")
	for ref, want := range map[string]string{"main": c2, "HEAD~1": c1, c1[:7]: c1, "v1": c2} {
		if got, err := g.RevParse(ctx, ref); err != nil || got != want {
			t.Errorf("RevParse %s: %s %v", ref, got, err)
		}
	}
	if _, err := g.RevParse(ctx, "nope"); !errs.Is(err, CodeGit) || errs.As(err).Field("ref") != "nope" {
		t.Fatalf("unknown ref: %v", err)
	}
	for _, bad := range []string{"", "-bad", "a b", "a\x00b", "a\nb", "x\x7f"} {
		if _, err := g.RevParse(ctx, bad); !errs.Is(err, CodeGitRef) {
			t.Errorf("ref %q: %v", bad, err)
		}
	}
	// ancestor: true, false (exit 1)
	if ok, err := g.IsAncestor(ctx, c1, c2); err != nil || !ok {
		t.Fatal("c1 ancestor of c2")
	}
	if ok, err := g.IsAncestor(ctx, c2, c1); err != nil || ok {
		t.Fatal("c2 not ancestor of c1")
	}
	if n, err := g.CommitCount(ctx, c1, c2); err != nil || n != 1 {
		t.Fatalf("count %d %v", n, err)
	}
	if n, err := g.CommitCount(ctx, c2, c2); err != nil || n != 0 {
		t.Fatalf("count0 %d %v", n, err)
	}
	// sha validation on every sha-taking method (HEAD~1 is refused; RevParse accepts it)
	for _, bad := range []string{"HEAD~1", "main", "abcdef", strings.Repeat("a", 41), "ABCDEFA", ""} {
		if _, err := g.IsAncestor(ctx, bad, c2); !errs.Is(err, CodeGitRef) {
			t.Errorf("IsAncestor %q: %v", bad, err)
		}
		if _, err := g.CommitCount(ctx, c1, bad); !errs.Is(err, CodeGitRef) {
			t.Errorf("CommitCount %q: %v", bad, err)
		}
		if _, _, err := g.Diff(ctx, bad, c2, 10); !errs.Is(err, CodeGitRef) {
			t.Errorf("Diff %q: %v", bad, err)
		}
	}
	for _, ok := range []string{"abcdefa", strings.Repeat("a", 40), "HEAD"} {
		if !ValidSHA(ok) {
			t.Errorf("ValidSHA %q", ok)
		}
	}
	// status clean, then dirty incl. a file in a new untracked directory
	clean, porcelain, err := g.Status(ctx)
	if err != nil || !clean || porcelain != "" {
		t.Fatalf("clean: %v %q %v", clean, porcelain, err)
	}
	tree1, err := g.WorkTree(ctx)
	if err != nil || len(tree1) != 40 {
		t.Fatalf("worktree %s %v", tree1, err)
	}
	write(t, dir, "new/dir/u.txt", "u")
	write(t, dir, "a.txt", "changed\n")
	clean, porcelain, err = g.Status(ctx)
	if err != nil || clean || !strings.Contains(porcelain, "? new/dir/u.txt") || !strings.Contains(porcelain, "a.txt") {
		t.Fatalf("dirty: %v %q %v", clean, porcelain, err)
	}
	tree2, _ := g.WorkTree(ctx)
	if tree2 == tree1 {
		t.Fatal("worktree hash must move with content")
	}
	// content change to the untracked file: porcelain identical, tree moves
	write(t, dir, "new/dir/u.txt", "v")
	_, porcelain2, _ := g.Status(ctx)
	tree3, _ := g.WorkTree(ctx)
	if porcelain2 != porcelain || tree3 == tree2 {
		t.Fatalf("content-blind: %v %v", porcelain2 == porcelain, tree3 == tree2)
	}
	// the scratch index must not touch the real index
	if out := git(t, dir, "diff", "--cached", "--name-only"); out != "" {
		t.Fatalf("real index touched: %s", out)
	}
	common, err := g.CommonDir(ctx)
	if err != nil || !filepath.IsAbs(common) || filepath.Base(common) != ".git" {
		t.Fatalf("common dir %q %v", common, err)
	}
	wd, tr, err := g.WorkingDiff(ctx, 1<<20)
	if err != nil || tr || !strings.Contains(wd, "+changed") {
		t.Fatalf("working diff: %v %v", tr, err)
	}
	// diff + truncation at a rune boundary (é is 2 bytes)
	full, tr, err := g.Diff(ctx, c1, c2, 1<<20)
	if err != nil || tr || !strings.Contains(full, "+two é") {
		t.Fatalf("diff: %q %v %v", full, tr, err)
	}
	idx := strings.Index(full, "é")
	cut, tr, err := g.Diff(ctx, c1, c2, idx+1)
	if err != nil || !tr || cut != full[:idx] {
		t.Fatalf("rune cut: %q %v", cut, tr)
	}
}

func TestG3TreeHashPin(t *testing.T) {
	// sha256("abc\ndef") — hand-computed, the authority for the preimage layout.
	if got := TreeHash("abc", "def"); got != "d53d6b91af7caf8fe3d8021f116270137c0079d579a1e16965da80c2ed138ffb" {
		t.Fatalf("TreeHash preimage changed: %s", got)
	}
	if TreeHash("abc", "def") == TreeHash("abcd", "ef") {
		t.Fatal("separator must keep head and tree apart")
	}
}

func TestG2ExecErrorBranches(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("spawn failed")
	// process cannot run
	g := NewExec("/", func(context.Context, string, []string, ...string) ([]byte, []byte, int, error) {
		return nil, nil, 0, boom
	})
	if _, err := g.Head(ctx); !errs.Is(err, CodeGit) || !errors.Is(err, boom) {
		t.Fatalf("spawn: %v", err)
	}
	// exit code ≥ 2 anywhere → ERR_GIT with stderr in Detail
	g = NewExec("/", func(context.Context, string, []string, ...string) ([]byte, []byte, int, error) {
		return nil, []byte("fatal: broken\n"), 128, nil
	})
	if _, err := g.RevParse(ctx, "main"); !errs.Is(err, CodeGit) || !strings.Contains(err.Error(), "fatal: broken") || errs.As(err).Field("exit") != "128" {
		t.Fatalf("exit 128: %v", err)
	}
	if _, err := g.IsAncestor(ctx, shaA, shaB); !errs.Is(err, CodeGit) {
		t.Fatal("ancestor exit 128")
	}
	if _, err := g.CommitCount(ctx, shaA, shaB); !errs.Is(err, CodeGit) {
		t.Fatal("count exit 128")
	}
	if _, _, err := g.Status(ctx); !errs.Is(err, CodeGit) {
		t.Fatal("status exit 128")
	}
	if _, _, err := g.Diff(ctx, shaA, shaB, 1); !errs.Is(err, CodeGit) {
		t.Fatal("diff exit 128")
	}
	if _, _, err := g.WorkingDiff(ctx, 1); !errs.Is(err, CodeGit) {
		t.Fatal("wdiff exit 128")
	}
	if _, err := g.WorkTree(ctx); !errs.Is(err, CodeGit) {
		t.Fatal("worktree exit 128")
	}
	if _, err := g.CommonDir(ctx); !errs.Is(err, CodeGit) {
		t.Fatal("common exit 128")
	}
	// exit 1 where it is not a legal answer, and malformed stdout
	exit1 := NewExec("/", func(_ context.Context, _ string, _ []string, args ...string) ([]byte, []byte, int, error) {
		return []byte("garbage"), nil, 1, nil
	})
	if _, err := exit1.RevParse(ctx, "main"); !errs.Is(err, CodeGit) {
		t.Fatal("rev-parse exit 1")
	}
	if _, err := exit1.CommitCount(ctx, shaA, shaB); !errs.Is(err, CodeGit) {
		t.Fatal("count exit 1")
	}
	if _, _, err := exit1.Status(ctx); !errs.Is(err, CodeGit) {
		t.Fatal("status exit 1")
	}
	if _, _, err := exit1.Diff(ctx, shaA, shaB, 1); !errs.Is(err, CodeGit) {
		t.Fatal("diff exit 1")
	}
	if _, _, err := exit1.WorkingDiff(ctx, 1); !errs.Is(err, CodeGit) {
		t.Fatal("wdiff exit 1")
	}
	if _, err := exit1.WorkTree(ctx); !errs.Is(err, CodeGit) {
		t.Fatal("worktree add exit 1")
	}
	if _, err := exit1.CommonDir(ctx); !errs.Is(err, CodeGit) {
		t.Fatal("common exit 1")
	}
	// write-tree exit 1 / short output after a successful add
	calls := 0
	wt := NewExec("/", func(_ context.Context, _ string, env []string, args ...string) ([]byte, []byte, int, error) {
		calls++
		if args[0] == "add" {
			if len(env) != 1 || !strings.HasPrefix(env[0], "GIT_INDEX_FILE=") {
				t.Fatalf("scratch index env: %v", env)
			}
			return nil, nil, 0, nil
		}
		return []byte("short"), nil, 0, nil
	})
	if _, err := wt.WorkTree(ctx); !errs.Is(err, CodeGit) {
		t.Fatal("write-tree short")
	}
	// write-tree spawn failure after a successful add, and scratch-index creation failure
	n := 0
	wt2 := NewExec("/", func(_ context.Context, _ string, _ []string, args ...string) ([]byte, []byte, int, error) {
		n++
		if n == 1 {
			return nil, nil, 0, nil
		}
		return nil, nil, 0, boom
	})
	if _, err := wt2.WorkTree(ctx); !errors.Is(err, boom) {
		t.Fatal("write-tree spawn failure")
	}
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
	if _, err := wt2.WorkTree(ctx); !errs.Is(err, CodeGit) || !strings.Contains(err.Error(), "scratch index") {
		t.Fatalf("scratch index failure: %v", err)
	}
	t.Setenv("TMPDIR", os.TempDir())
	// rev-parse returning a non-sha with exit 0, and count with non-integer stdout
	odd := NewExec("/", func(_ context.Context, _ string, _ []string, args ...string) ([]byte, []byte, int, error) {
		return []byte("not-a-sha\n"), nil, 0, nil
	})
	if _, err := odd.RevParse(ctx, "main"); !errs.Is(err, CodeGit) {
		t.Fatal("rev-parse odd")
	}
	if _, err := odd.CommitCount(ctx, shaA, shaB); !errs.Is(err, CodeGit) {
		t.Fatal("count odd")
	}
	// argument shape observed through the seam: --end-of-options before user data
	var seen [][]string
	spy := NewExec("/", func(_ context.Context, _ string, _ []string, args ...string) ([]byte, []byte, int, error) {
		seen = append(seen, args)
		return []byte(shaA + "\n"), nil, 0, nil
	})
	_, _ = spy.RevParse(ctx, "main")
	_, _ = spy.IsAncestor(ctx, shaA, shaB)
	_, _, _ = spy.Diff(ctx, shaA, shaB, 10)
	for _, a := range seen {
		if !strings.Contains(strings.Join(a, " "), "--end-of-options") {
			t.Fatalf("missing --end-of-options: %v", a)
		}
	}
	if !strings.HasSuffix(seen[0][len(seen[0])-1], "^{commit}") {
		t.Fatal("rev-parse must peel to a commit")
	}
	// RealExec: spawn failure branch via a bogus dir
	if _, _, _, err := RealExec(ctx, "/nonexistent-dir-xyz", nil, "status"); err == nil {
		t.Fatal("RealExec must report an unrunnable process")
	}
	// scrubEnv drops GIT_* overrides
	got := scrubEnv([]string{"GIT_DIR=x", "GIT_CONFIG_COUNT=1", "GIT_WORK_TREE=y", "GIT_INDEX_FILE=z", "GIT_EXTERNAL_DIFF=e", "GIT_OBJECT_DIRECTORY=o", "GIT_ALTERNATE_OBJECT_DIRECTORIES=a", "GIT_COMMON_DIR=c", "GIT_TRACE=1", "PATH=p", "HOME=h"})
	if strings.Join(got, ",") != "PATH=p,HOME=h" {
		t.Fatalf("scrub: %v", got)
	}
}

func TestG4FakeContract(t *testing.T) {
	ctx := context.Background()
	f := &Fake{HeadSHA: shaA, Refs: map[string]string{"main": shaA}, Ancestors: map[string]bool{shaB + " " + shaA: true}, Counts: map[string]int{shaB + ".." + shaA: 3}, Clean: false, Porcelain: "p", Diffs: map[string]string{shaB + ".." + shaA: "diff é", "HEAD": "wd"}, Tree: "t"}
	if h, _ := f.Head(ctx); h != shaA {
		t.Fatal("head")
	}
	if r, _ := f.RevParse(ctx, "HEAD"); r != shaA {
		t.Fatal("revparse HEAD")
	}
	if r, _ := f.RevParse(ctx, "main"); r != shaA {
		t.Fatal("revparse main")
	}
	if _, err := f.RevParse(ctx, "nope"); !errs.Is(err, CodeGit) {
		t.Fatal("unknown ref")
	}
	if ok, _ := f.IsAncestor(ctx, shaB, shaA); !ok {
		t.Fatal("ancestor")
	}
	if n, _ := f.CommitCount(ctx, shaB, shaA); n != 3 {
		t.Fatal("count")
	}
	if c, p, _ := f.Status(ctx); c || p != "p" {
		t.Fatal("status")
	}
	if d, tr, _ := f.Diff(ctx, shaB, shaA, 6); d != "diff " || !tr {
		t.Fatalf("diff cut %q", d)
	}
	if d, _, _ := f.WorkingDiff(ctx, 100); d != "wd" {
		t.Fatal("wd")
	}
	if tr, _ := f.WorkTree(ctx); tr != "t" {
		t.Fatal("tree")
	}
	f.Common = "/r/.git"
	if c, _ := f.CommonDir(ctx); c != "/r/.git" {
		t.Fatal("common")
	}
	if len(f.Calls) != 11 {
		t.Fatalf("calls %v", f.Calls)
	}
	boom := errors.New("boom")
	f.Err = boom
	if _, err := f.Head(ctx); err != boom {
		t.Fatal("err head")
	}
	if _, err := f.RevParse(ctx, "HEAD"); err != boom {
		t.Fatal("err revparse")
	}
	if _, _, err := f.Diff(ctx, shaB, shaA, 1); err != boom {
		t.Fatal("err diff")
	}
	if _, _, err := f.WorkingDiff(ctx, 1); err != boom {
		t.Fatal("err wd")
	}
	// Cut edge cases
	if s, tr := Cut("abc", -1); s != "abc" || tr {
		t.Fatal("negative max keeps all")
	}
	if s, tr := Cut("é", 1); s != "" || !tr {
		t.Fatal("cut before a partial rune")
	}
}

// pins_proven is what stops a fix round advancing on a claim nothing holds. It is vacuously true
// when the fix declared no pins: "made no claim" and "made a claim that failed" are different
// states, and only the second is a defect in the fix.
func TestG1PinsProven(t *testing.T) {
	g, ok := Builtin("pins_proven")
	if !ok {
		t.Fatal("pins_proven is not registered")
	}
	ctx := context.Background()
	if err := g(ctx, run.Snapshot{}, &Fake{}); err != nil {
		t.Errorf("no findings at all must pass: %+v", err)
	}
	clean := run.Snapshot{Findings: []run.Finding{
		{IssueText: "some ordinary finding"},
		// The prose alone must not trip the gate. An earlier version matched a prefix of
		// IssueText, so a review finding that merely quoted the phrase failed the run, and
		// rewording the real message disabled the gate. Selection is on Source+Category now.
		{IssueText: "Unproven fix in calc.go: breaking \"n < 10\" did not make the tests fail."},
		// Right producer, but an outcome that does not block.
		{IssueText: "bad.go: could not be evaluated", File: "bad.go",
			Source: run.SourceMutationVerify, Category: run.CategoryMalformedPin},
		// Right category, but not from the prover: not this gate's business.
		{IssueText: "calc.go: unproven", File: "calc.go", Category: run.CategoryUnprovenFix},
	}}
	if err := g(ctx, clean, &Fake{}); err != nil {
		t.Errorf("only a blocking mutation-verify finding may fail this gate: %+v", err)
	}

	for _, cat := range []string{run.CategoryUnprovenFix, run.CategoryUnverifiable} {
		bad := run.Snapshot{Findings: []run.Finding{
			{IssueText: "some ordinary finding"},
			{IssueText: "calc.go: breaking \"n < 10\" did not make the tests fail.", File: "calc.go",
				Source: run.SourceMutationVerify, Category: cat},
		}}
		err := g(ctx, bad, &Fake{})
		if err == nil {
			t.Fatalf("category %q must fail the gate", cat)
		}
		if !strings.Contains(err.Detail, "calc.go") {
			t.Errorf("the failure must name the file: %+v", err)
		}
	}
}

// The schema owns the pin vocabulary, so an outcome it does not recognise must not read as a
// success anywhere. Valid() is what lets a caller say "I do not know this" instead of guessing.
func TestPinOutcomeVocabulary(t *testing.T) {
	for _, o := range []run.PinOutcome{run.PinProven, run.PinSurvived, run.PinMalformed, run.PinUnverifiable} {
		if !o.Valid() {
			t.Errorf("%q is part of the schema but Valid() rejects it", o)
		}
	}
	for _, o := range []run.PinOutcome{"", "ok", "PROVEN", "unusable"} {
		if o.Valid() {
			t.Errorf("%q is not in the schema but Valid() accepts it", o)
		}
	}
}
