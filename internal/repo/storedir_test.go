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

// AC-2.8 (#173, run-store half): git is unaffected by a populated <common>/metareview/. Worktree remove/prune,
// gc --prune=now --aggressive, repack -ad, reflog expire --all and clean -fdx leave every store file byte-identical;
// fsck --full --strict passes; status is clean; clone and clone --bare copy none of it.
func TestGitIsUnaffectedByTheStore(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	main := filepath.Join(root, "main")
	gitT(t, root, "init", "-q", "-b", "main", main)
	if err := os.WriteFile(filepath.Join(main, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, main, "add", "a.txt")
	gitT(t, main, "commit", "-q", "-m", "a")
	wt := filepath.Join(root, "wt")
	gitT(t, main, "worktree", "add", "-q", "-b", "feat", wt)
	gone := filepath.Join(root, "gone")
	gitT(t, main, "worktree", "add", "-q", "-b", "gone", gone)

	store, err := StoreDir(wt)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"runs/mrv-a-000000001/audit.jsonl":   "{\"type\":\"init\"}\n",
		"runs/mrv-a-000000001/workflow.yaml": "workflow: x\n",
		"runs.jsonl":                         "{\"id\":\"mrv-a-000000001\"}\n",
		"sessions/s.json":                    "{}\n",
		"migrate.lock":                       "",
	}
	for rel, body := range files {
		p := filepath.Join(store, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	gitT(t, main, "worktree", "remove", "--force", gone)
	gitT(t, main, "worktree", "prune")
	gitT(t, main, "gc", "-q", "--prune=now", "--aggressive")
	gitT(t, main, "repack", "-q", "-ad")
	gitT(t, main, "reflog", "expire", "--all", "--expire=now")
	gitT(t, main, "clean", "-fdx")
	for rel, body := range files {
		got, err := os.ReadFile(filepath.Join(store, rel))
		if err != nil || string(got) != body {
			t.Errorf("%s changed under git maintenance: %q %v", rel, got, err)
		}
	}
	gitT(t, main, "fsck", "--full", "--strict")
	for _, dir := range []string{main, wt} {
		if out := gitT(t, dir, "status", "--porcelain"); out != "" {
			t.Errorf("git status in %s is not clean: %q", dir, out)
		}
	}
	clone, bareClone := filepath.Join(root, "clone"), filepath.Join(root, "clone.git")
	gitT(t, root, "clone", "-q", main, clone)
	gitT(t, root, "clone", "-q", "--bare", main, bareClone)
	for _, p := range []string{filepath.Join(clone, ".git", "metareview"), filepath.Join(bareClone, "metareview")} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("a clone copied the store: %s", p)
		}
	}
}

func TestToplevel(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	gitT(t, root, "init", "-q", "-b", "main", root)
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := Toplevel(sub); err != nil || got != root {
		t.Fatalf("Toplevel(%s) = %q, %v", sub, got, err)
	}
	if _, err := Toplevel(t.TempDir()); err == nil {
		t.Fatal("outside a work tree there is no toplevel")
	}
}
