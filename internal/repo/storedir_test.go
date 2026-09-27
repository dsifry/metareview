package repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// #173: the shared store lives in git's common directory, so the main checkout and every linked worktree resolve
// the SAME store — and it does not depend on any one checkout existing.
func TestStoreDirIsSharedAcrossWorktrees(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	main := filepath.Join(root, "main")
	gitT(t, root, "init", "-q", "-b", "main", main)
	gitT(t, main, "commit", "-q", "--allow-empty", "-m", "base")
	wt := filepath.Join(root, "wt")
	gitT(t, main, "worktree", "add", "-q", "-b", "feat", wt)
	want := filepath.Join(main, ".git", "metareview")
	for _, dir := range []string{main, wt, filepath.Join(main, ".git")} {
		got, err := StoreDir(dir)
		if err != nil || got != want {
			t.Errorf("StoreDir(%s) = %q, %v; want %q", dir, got, err, want)
		}
	}
	// A bare repository has a store too (it has no checkout to hold one otherwise).
	bare := filepath.Join(root, "bare.git")
	gitT(t, root, "init", "-q", "--bare", bare)
	if got, err := StoreDir(bare); err != nil || got != filepath.Join(bare, "metareview") {
		t.Errorf("bare: StoreDir = %q, %v", got, err)
	}
	if _, err := StoreDir(t.TempDir()); err == nil {
		t.Error("outside a repository there is no store")
	}
}

// Like the FSM, the store lookup ignores an exported GIT_DIR: a hook or wrapper must not point it elsewhere.
func TestStoreDirIgnoresExportedGitDir(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
	gitT(t, root, "init", "-q", a)
	gitT(t, root, "init", "-q", b)
	t.Setenv("GIT_DIR", filepath.Join(b, ".git"))
	if got, err := StoreDir(a); err != nil || got != filepath.Join(a, ".git", "metareview") {
		t.Fatalf("StoreDir(a) with GIT_DIR=b = %q, %v", got, err)
	}
}
