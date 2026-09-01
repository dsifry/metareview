package mutation

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// These tests drive the engine end to end against a REAL git repository and a REAL `go test`, with no
// seams injected — so the default git shell-out, the worktree checkout/overlay, and the runProc exec
// are all exercised, not just the decision table. This is the proof the whole node rests on: a fix's
// committed test genuinely fails on the pre-fix tree and passes once the fix is applied.

func realGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t", "GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func writeFile(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// reproRepo builds a repo committed twice: pre holds files1, post applies files2 on top (add or
// overwrite). Returns the repo dir and the two commit SHAs.
func reproRepo(t *testing.T, files1, files2 map[string]string) (string, string, string) {
	t.Helper()
	dir := t.TempDir()
	realGit(t, dir, "init", "-q", "-b", "main")
	for rel, body := range files1 {
		writeFile(t, dir, rel, body)
	}
	realGit(t, dir, "add", "-A")
	realGit(t, dir, "commit", "-qm", "pre")
	pre := realGit(t, dir, "rev-parse", "HEAD")
	for rel, body := range files2 {
		writeFile(t, dir, rel, body)
	}
	realGit(t, dir, "add", "-A")
	realGit(t, dir, "commit", "-qm", "post")
	post := realGit(t, dir, "rev-parse", "HEAD")
	return dir, trim(pre), trim(post)
}

func trim(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

const goMod = "module fixture\n\ngo 1.22\n"

func realReproducer(dir, pre, post string) Reproducer {
	return Reproducer{Dir: dir, PreFixSHA: pre, PostFixSHA: post, TestCmd: []string{"go", "test", "./..."}}
}

// The whole point, end to end: the fix changes `n <= 10` to `n < 10` and adds a test that pins the
// boundary. The test fails (assertion) on the pre-fix tree and passes on the post-fix tree → proven.
func TestReproduceRealGitProven(t *testing.T) {
	pre := map[string]string{
		"go.mod":  goMod,
		"calc.go": "package fixture\n\nfunc Allow(n int) bool { return n <= 10 }\n",
		// A second package with NO test files makes `go test ./...` print "[no test files]" — the
		// shape every real multi-package repo has, and what makes classify key on the target's own
		// RUN marker rather than the suite-wide "no test files" / "no tests to run" noise.
		"util/util.go": "package util\n\nfunc Noop() {}\n",
	}
	post := map[string]string{
		"calc.go":      "package fixture\n\nfunc Allow(n int) bool { return n < 10 }\n",
		"calc_test.go": "package fixture\n\nimport \"testing\"\n\nfunc TestBoundary(t *testing.T) {\n\tif Allow(10) {\n\t\tt.Fatal(\"10 must not be allowed\")\n\t}\n}\n",
	}
	dir, preSHA, postSHA := reproRepo(t, pre, post)
	res, err := realReproducer(dir, preSHA, postSHA).Reproduce(context.Background(), []Proof{{Test: "TestBoundary"}})
	if err != nil {
		t.Fatalf("Reproduce: %v", err)
	}
	if !res[0].Proven || res[0].Outcome != PinProven {
		t.Fatalf("a genuine fail-before/pass-after must be proven even in a multi-package repo: %+v", res[0])
	}
}

// The multi-package regression, made explicit: a renamed test file (delete + add, rename detection
// off) must not leave both copies present during the fail-before run — the moved test would redeclare
// and fail to compile, scoring a valid reproduction as malformed. Applying the test deletion in step
// (b) keeps it proven.
func TestReproduceRealGitRenamedTestFile(t *testing.T) {
	pre := map[string]string{
		"go.mod":      goMod,
		"calc.go":     "package fixture\n\nfunc Allow(n int) bool { return n <= 10 }\n",
		"old_test.go": "package fixture\n\nimport \"testing\"\n\nfunc TestBoundary(t *testing.T) {\n\tif Allow(10) {\n\t\tt.Fatal(\"10 must not be allowed\")\n\t}\n}\n",
	}
	post := map[string]string{
		"calc.go":     "package fixture\n\nfunc Allow(n int) bool { return n < 10 }\n",
		"old_test.go": "", // deleted below
		"new_test.go": "package fixture\n\nimport \"testing\"\n\nfunc TestBoundary(t *testing.T) {\n\tif Allow(10) {\n\t\tt.Fatal(\"10 must not be allowed\")\n\t}\n}\n",
	}
	dir := t.TempDir()
	realGit(t, dir, "init", "-q", "-b", "main")
	for rel, body := range pre {
		writeFile(t, dir, rel, body)
	}
	realGit(t, dir, "add", "-A")
	realGit(t, dir, "commit", "-qm", "pre")
	preSHA := trim(realGit(t, dir, "rev-parse", "HEAD"))
	// Move the test to a new file: a delete plus an add of the SAME test function.
	if err := os.Remove(filepath.Join(dir, "old_test.go")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "new_test.go", post["new_test.go"])
	writeFile(t, dir, "calc.go", post["calc.go"])
	realGit(t, dir, "add", "-A")
	realGit(t, dir, "commit", "-qm", "post")
	postSHA := trim(realGit(t, dir, "rev-parse", "HEAD"))

	res, err := realReproducer(dir, preSHA, postSHA).Reproduce(context.Background(), []Proof{{Test: "TestBoundary"}})
	if err != nil {
		t.Fatalf("Reproduce: %v", err)
	}
	if !res[0].Proven || res[0].Outcome != PinProven {
		t.Fatalf("a renamed test file must not redeclare during fail-before; expected proven: %+v", res[0])
	}
}

// A committed test that exercises the code but does not pin the fault passes even on the pre-fix
// tree: the differential never fails-before, so it is a test gap (survived), not a proof.
func TestReproduceRealGitSurvived(t *testing.T) {
	pre := map[string]string{
		"go.mod":  goMod,
		"calc.go": "package fixture\n\nfunc Allow(n int) bool { return n <= 10 }\n",
	}
	post := map[string]string{
		"calc.go":      "package fixture\n\nfunc Allow(n int) bool { return n < 10 }\n",
		"calc_test.go": "package fixture\n\nimport \"testing\"\n\nfunc TestItRuns(t *testing.T) {\n\t_ = Allow(1)\n}\n",
	}
	dir, preSHA, postSHA := reproRepo(t, pre, post)
	res, err := realReproducer(dir, preSHA, postSHA).Reproduce(context.Background(), []Proof{{Test: "TestItRuns"}})
	if err != nil {
		t.Fatalf("Reproduce: %v", err)
	}
	if res[0].Outcome != PinSurvived {
		t.Fatalf("a test that passes pre-fix must be survived: %+v", res[0])
	}
}

// A pre-fix SHA that does not resolve makes the real git exit non-zero (not fail to spawn): the
// engine reads that exit code and fails closed to unverifiable. Exercises the default git shell-out's
// ExitError path.
func TestReproduceRealGitBadShaIsUnverifiable(t *testing.T) {
	dir, _, postSHA := reproRepo(t,
		map[string]string{"go.mod": goMod, "calc.go": "package fixture\n"},
		map[string]string{"calc_test.go": "package fixture\n"})
	bogus := "0000000000000000000000000000000000000000"
	res, err := realReproducer(dir, bogus, postSHA).Reproduce(context.Background(), []Proof{{Test: "TestX"}})
	if err != nil {
		t.Fatalf("Reproduce: %v", err)
	}
	if res[0].Outcome != PinUnverifiable {
		t.Fatalf("an unresolvable pre-fix SHA must be unverifiable: %+v", res[0])
	}
}

// A committed test that references a symbol only the fix introduces fails to COMPILE against the
// pre-fix tree. That is not the fault's assertion — it is malformed, the load-bearing distinction.
func TestReproduceRealGitCompileErrorIsMalformed(t *testing.T) {
	pre := map[string]string{
		"go.mod":  goMod,
		"calc.go": "package fixture\n\nfunc Allow(n int) bool { return n <= 10 }\n",
	}
	post := map[string]string{
		"calc.go":      "package fixture\n\nfunc Allow(n int) bool { return n < 10 }\n\nfunc Limit() int { return 10 }\n",
		"calc_test.go": "package fixture\n\nimport \"testing\"\n\nfunc TestNeedsLimit(t *testing.T) {\n\tif Limit() != 10 {\n\t\tt.Fatal(\"nope\")\n\t}\n}\n",
	}
	dir, preSHA, postSHA := reproRepo(t, pre, post)
	res, err := realReproducer(dir, preSHA, postSHA).Reproduce(context.Background(), []Proof{{Test: "TestNeedsLimit"}})
	if err != nil {
		t.Fatalf("Reproduce: %v", err)
	}
	if res[0].Outcome != PinMalformed {
		t.Fatalf("a pre-fix compile error must be malformed, never a valid fail-before: %+v", res[0])
	}
}
