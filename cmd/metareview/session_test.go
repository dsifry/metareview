package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// sessionWorktree adds a sibling worktree to a gitRepo fixture: the shape the Stop-hook bug was
// captured in — the host launched on one checkout, the work in another.
func sessionWorktree(t *testing.T, root string) string {
	t.Helper()
	wt := filepath.Join(t.TempDir(), "wt")
	cmd := exec.Command("git", "worktree", "add", "-q", "-b", "keeper", wt)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}
	real, err := filepath.EvalSymlinks(wt)
	if err != nil {
		t.Fatal(err)
	}
	return real
}

func TestSessionBindResolveUnbind(t *testing.T) {
	root := gitRepo(t)
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	wt := sessionWorktree(t, root)

	// Unbound: resolve answers with the host's own checkout.
	code, out, errOut := runCLI(t, root, nil, "session", "resolve", "s-1")
	if code != 0 || strings.TrimSpace(out) != realRoot && strings.TrimSpace(out) != root || errOut != "" {
		t.Fatalf("unbound resolve: code=%d out=%q err=%q", code, out, errOut)
	}

	code, out, errOut = runCLI(t, root, nil, "session", "bind", "s-1", wt)
	if code != 0 || !strings.Contains(out, wt) || !strings.Contains(out, "keeper") {
		t.Fatalf("bind: code=%d out=%q err=%q", code, out, errOut)
	}

	// Bound: the checkout, then a line saying the binding chose it — so the hook need not infer
	// "bound" from a path comparison that a nested marker directory can fool.
	code, out, errOut = runCLI(t, root, nil, "session", "resolve", "s-1")
	if code != 0 || out != wt+"\nbound\n" || errOut != "" {
		t.Fatalf("bound resolve: code=%d out=%q err=%q", code, out, errOut)
	}

	code, out, _ = runCLI(t, wt, nil, "session", "unbind", "s-1")
	if code != 0 || !strings.Contains(out, "unbound") {
		t.Fatalf("unbind: code=%d out=%q", code, out)
	}
	code, out, _ = runCLI(t, root, nil, "session", "unbind", "s-1")
	if code != 0 || !strings.Contains(out, "no binding") {
		t.Fatalf("second unbind: code=%d out=%q", code, out)
	}
}

func TestSessionResolveWithoutAnIDIsTheCheckoutRoot(t *testing.T) {
	root := gitRepo(t)
	deep := filepath.Join(root, "internal")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	code, out, _ := runCLI(t, deep, nil, "session", "resolve")
	if code != 0 || strings.TrimSpace(out) != root {
		t.Fatalf("code=%d out=%q want %s", code, out, root)
	}
}

// A stale binding still resolves (exit 0 — the hook must get an answer), but says so on stderr.
func TestSessionResolveWarnsOnAStaleBinding(t *testing.T) {
	root := gitRepo(t)
	wt := sessionWorktree(t, root)
	if code, _, errOut := runCLI(t, root, nil, "session", "bind", "s-2", wt); code != 0 {
		t.Fatalf("bind: %s", errOut)
	}
	if err := os.RemoveAll(wt); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := runCLI(t, root, nil, "session", "resolve", "s-2")
	if code != 0 || out != root+"\n" || !strings.Contains(errOut, "cannot be used") {
		t.Fatalf("code=%d out=%q err=%q", code, out, errOut)
	}
}

func TestSessionBindRefusals(t *testing.T) {
	root := gitRepo(t)
	other := gitRepo(t)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"session", "bind", "s-3", other}, "not a worktree of this repository"},
		{[]string{"session", "bind", "../x", root}, "invalid session id"},
		{[]string{"session", "bind", "s-3", filepath.Join(root, "nope")}, "no such file"},
	} {
		code, _, errOut := runCLI(t, root, nil, tc.args...)
		if code != 1 || !strings.Contains(errOut, tc.want) {
			t.Errorf("%v: code=%d err=%q, want exit 1 mentioning %q", tc.args, code, errOut, tc.want)
		}
	}
}

func TestSessionUsage(t *testing.T) {
	root := gitRepo(t)
	for _, args := range [][]string{
		{"session"},
		{"session", "bogus"},
		{"session", "bind", "only-id"},
		{"session", "bind", "a", "b", "c"},
		{"session", "resolve", "a", "b"},
		{"session", "unbind"},
	} {
		code, _, errOut := runCLI(t, root, nil, args...)
		if code != 2 || !strings.Contains(errOut, "Usage: metareview session") {
			t.Errorf("%v: code=%d err=%q", args, code, errOut)
		}
	}
	code, out, _ := runCLI(t, root, nil, "session", "--help")
	if code != 0 || !strings.Contains(out, "metareview session bind") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	if code, _, errOut := runCLI(t, root, nil, "session", "unbind", "../x"); code != 1 || !strings.Contains(errOut, "invalid session id") {
		t.Errorf("unbind invalid id: code=%d err=%q", code, errOut)
	}
}
