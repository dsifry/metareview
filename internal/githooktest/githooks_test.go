// Package githooktest exercises the git-native review-gate hooks (hooks/git/pre-push and
// hooks/git/post-commit) as a black box, so their behaviour is a repeatable, mutation-checkable test. Git
// invokes these on the actual commit/push, so there is no command-string classification to test — only that
// pre-push BLOCKS (fail closed) on a nonzero gate and post-commit NAMES the committed files without blocking.
package githooktest

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func hookPath(t *testing.T, name string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	p := filepath.Join(filepath.Dir(thisFile), "..", "..", "hooks", "git", name)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("hook not found: %v", err)
	}
	return p
}

// fakeBin writes a stand-in `metareview` whose `review gate` exits with the given code, so the test controls
// the gate verdict the pre-push hook reacts to.
func fakeBin(t *testing.T, gateExit int) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "metareview")
	body := "#!/bin/sh\nif [ \"$1\" = review ] && [ \"$2\" = gate ]; then echo 'FAKE GATE OUTPUT' >&2; exit " +
		itoa(gateExit) + "; fi\nexit 0\n"
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// runPrePush runs pre-push with a metareview binary and returns its exit code and combined output. git feeds
// the hook the refs on stdin, so we do too — a normal (non-deletion) push of the current branch.
func runPrePush(t *testing.T, bin string) (int, string) {
	return runPrePushStdin(t, bin, "refs/heads/main abc refs/heads/main def\n")
}

// runPrePushStdin is runPrePush with caller-supplied stdin, so a test can feed the ref lines git would — in
// particular a DELETION line (all-zero local sha), which the hook must treat as "nothing to review".
func runPrePushStdin(t *testing.T, bin, stdin string) (int, string) {
	t.Helper()
	c := exec.Command("bash", hookPath(t, "pre-push"), "origin", "https://example/repo.git")
	c.Env = append(os.Environ(), "METAREVIEW_BIN="+bin, "METAREVIEW_BASE=main")
	c.Stdin = strings.NewReader(stdin)
	out, _ := c.CombinedOutput()
	return c.ProcessState.ExitCode(), string(out)
}

// A ref DELETION sends no content, so the pre-push gate must NOT block it — even when the gate binary would
// fail (fakeBin(1)). git signals a deletion with an all-zero local sha. This is the case that blocked a
// `git push --delete` of an already-merged branch.
func TestPrePushSkipsGateOnRefDeletion(t *testing.T) {
	zero := "0000000000000000000000000000000000000000"
	// Only a deletion is pushed: the hook exits 0 without ever consulting the (failing) gate.
	if code, out := runPrePushStdin(t, fakeBin(t, 1), "(delete) "+zero+" refs/heads/gone "+zero+"\n"); code != 0 {
		t.Fatalf("a pure ref deletion must not be gated (exit 0), got code=%d out=%q", code, out)
	}
	// A deletion mixed with a real (non-deletion) push still gates — there IS content to review, so a failing
	// gate blocks. This keeps the deletion shortcut from becoming a bypass for a piggybacked real push.
	mixed := "(delete) " + zero + " refs/heads/gone " + zero + "\nrefs/heads/main abc refs/heads/main def\n"
	if code, _ := runPrePushStdin(t, fakeBin(t, 1), mixed); code != 1 {
		t.Fatalf("a non-deletion ref alongside a deletion must still gate (exit 1), got code=%d", code)
	}
}

// The hooks MUST be executable: git silently SKIPS a non-executable hook, which would make the gate fail
// OPEN on a fresh clone. This guards the exec bit against a regression (e.g. an edit that drops it).
func TestGitHooksAreExecutable(t *testing.T) {
	for _, name := range []string{"pre-push", "post-commit"} {
		info, err := os.Stat(hookPath(t, name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode()&0o100 == 0 {
			t.Fatalf("hooks/git/%s must be executable — git silently skips a non-executable hook and the gate then fails OPEN; mode=%v", name, info.Mode())
		}
	}
	// session-start-check.sh lives in hooks/ (not hooks/git/) and .claude/settings.json invokes it directly
	// as a command, so it needs the exec bit too — a dropped bit silently disables the install reminder.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	sessionHook := filepath.Join(filepath.Dir(thisFile), "..", "..", "hooks", "session-start-check.sh")
	info, err := os.Stat(sessionHook)
	if err != nil {
		t.Fatalf("session-start-check.sh not found: %v", err)
	}
	if info.Mode()&0o100 == 0 {
		t.Fatalf("hooks/session-start-check.sh must be executable — .claude/settings.json runs it directly; mode=%v", info.Mode())
	}
}

// pre-push is the HARD gate: a nonzero gate must BLOCK the push (exit 1) and surface the reason; a clean gate
// (exit 0) must allow it.
func TestPrePushBlocksOnGateFailureAllowsWhenClean(t *testing.T) {
	if code, out := runPrePush(t, fakeBin(t, 1)); code != 1 || !strings.Contains(out, "PUSH BLOCKED") {
		t.Fatalf("a nonzero gate must block the push (exit 1) with a reason; got code=%d out=%q", code, out)
	}
	if code, _ := runPrePush(t, fakeBin(t, 0)); code != 0 {
		t.Fatalf("a clean gate must allow the push (exit 0); got %d", code)
	}
	// A broken gate (exit 2) is not a clean branch — it must still block.
	if code, out := runPrePush(t, fakeBin(t, 2)); code != 1 {
		t.Fatalf("a broken gate must block the push, not read as clean; got code=%d out=%q", code, out)
	}
}

// pre-push fails CLOSED: a missing binary must block the push, never wave it through.
func TestPrePushFailsClosedOnMissingBinary(t *testing.T) {
	if code, out := runPrePush(t, filepath.Join(t.TempDir(), "does-not-exist")); code != 1 {
		t.Fatalf("a missing binary must block the push (fail closed); got code=%d out=%q", code, out)
	}
}

// gitRepo makes a throwaway repo whose HEAD commit wrote exactly `files`, and returns its root.
func gitRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	run := func(args ...string) {
		c := exec.Command("git", args...)
		c.Dir = root
		c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e")
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	for name, body := range files {
		full := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("add", "-A")
	run("commit", "-qm", "commit")
	return root
}

// post-commit NEVER blocks (exit 0) and NAMES the files the commit wrote, so the agent sees a review-owed
// nudge in the git-commit output.
func TestPostCommitNamesCommittedFiles(t *testing.T) {
	root := gitRepo(t, map[string]string{"foo.go": "package p\n", "bar/baz.go": "package bar\n"})
	c := exec.Command("bash", hookPath(t, "post-commit"))
	c.Dir = root
	out, _ := c.CombinedOutput()
	if code := c.ProcessState.ExitCode(); code != 0 {
		t.Fatalf("post-commit must never block (exit 0); got %d\n%s", code, out)
	}
	s := string(out)
	if !strings.Contains(s, "review is OWED") || !strings.Contains(s, "foo.go") || !strings.Contains(s, "bar/baz.go") {
		t.Fatalf("post-commit must name every committed file in a review-owed nudge:\n%s", s)
	}
}

// A commit that wrote ONLY metareview's own generated artifacts has nothing a review can cover, so
// post-commit must stay silent (no nudge loop).
func TestPostCommitIgnoresGeneratedArtifacts(t *testing.T) {
	root := gitRepo(t, map[string]string{"docs/metareview/reviews/x.md": "# review\n", ".metareview/runs.jsonl": "{}\n"})
	c := exec.Command("bash", hookPath(t, "post-commit"))
	c.Dir = root
	out, _ := c.CombinedOutput()
	if strings.Contains(string(out), "review is OWED") {
		t.Fatalf("post-commit must not nudge about generated artifacts:\n%s", out)
	}
}

// repoHook resolves a hook that lives in hooks/ (not hooks/git/), e.g. session-start-check.sh.
func repoHook(t *testing.T, name string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "hooks", name)
}

// session-start-check.sh reminds when the gate is NOT installed, and must read an equivalent-but-non-
// canonical core.hooksPath (e.g. $ROOT/./hooks/git) as INSTALLED — otherwise it nags on a correctly
// installed repo (CodeRabbit: session-start-check.sh — normalize the relative path before comparing).
func TestSessionStartCheckNormalizesEquivalentHooksPath(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		c := exec.Command("git", append([]string{"-C", root}, args...)...)
		c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e")
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q", "-b", "main")
	if err := os.MkdirAll(filepath.Join(root, "hooks", "git"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func() string {
		c := exec.Command("bash", repoHook(t, "session-start-check.sh"))
		c.Env = append(os.Environ(), "CLAUDE_PROJECT_DIR="+root)
		out, _ := c.CombinedOutput()
		return strings.TrimSpace(string(out))
	}
	// Unset core.hooksPath → the not-installed reminder fires (proves the check is live, not vacuous).
	if out := run(); !strings.Contains(out, "NOT installed") {
		t.Fatalf("an unset core.hooksPath must produce the not-installed reminder; got %q", out)
	}
	// A non-canonical but EQUIVALENT spelling must read as installed → no reminder (the normalization fix).
	git("config", "core.hooksPath", root+"/./hooks/git")
	if out := run(); out != "" {
		t.Fatalf("$ROOT/./hooks/git is equivalent to $ROOT/hooks/git and must read as installed; got %q", out)
	}
}
