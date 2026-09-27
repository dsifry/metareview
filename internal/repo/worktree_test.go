package repo

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMainWorktreeFromPorcelain(t *testing.T) {
	out := "worktree /repo/main\nHEAD abc\nbranch refs/heads/main\n\nworktree /repo/linked\nHEAD def\ndetached"
	if path, bare := MainWorktreeFromPorcelain(out); path != "/repo/main" || bare {
		t.Fatalf("got %q bare=%v, want the first block's worktree", path, bare)
	}
	if _, bare := MainWorktreeFromPorcelain("worktree /repo/bare.git\nbare\n\nworktree /repo/wt\nHEAD abc"); !bare {
		t.Fatal("a bare main worktree must be reported as bare")
	}
}

func TestRunStoreRootResolvesMainWorktreeAndFallsBack(t *testing.T) {
	orig := runStoreGit
	t.Cleanup(func() { runStoreGit = orig })
	start := t.TempDir()

	runStoreGit = func(string) (string, error) {
		return "worktree /repo/main\nHEAD abc\n\nworktree " + start + "\nHEAD abc", nil
	}
	if got := RunStoreRoot(start); got != "/repo/main" {
		t.Fatalf("linked worktree: got %q, want the main worktree", got)
	}
	for name, stub := range map[string]func(string) (string, error){
		"git fails": func(string) (string, error) { return "", errors.New("not a git repository") },
		"empty":     func(string) (string, error) { return "", nil },
		"bare main": func(string) (string, error) { return "worktree /repo/bare.git\nbare", nil },
	} {
		runStoreGit = stub
		if got := RunStoreRoot(start); got != RootOr(start) {
			t.Errorf("%s: got %q, want the RootOr fallback %q", name, got, RootOr(start))
		}
	}
}

// TestRunStoreReadersAreDeclared keeps #169 from recurring on a new surface: outside the FSM
// (which owns the store), every line that builds a `.metareview/runs` path must have RunStoreRoot or
// a `run-store:` declaration on it or within the three lines above. Checked per site, not per file,
// so a second, undeclared reader in a file that already has a declared one still fails.
func TestRunStoreReadersAreDeclared(t *testing.T) {
	const window = 3
	repoRoot := filepath.Join("..", "..")
	sites := 0
	for _, top := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(repoRoot, top), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if strings.HasPrefix(d.Name(), ".") || path == filepath.Join(repoRoot, "internal", "fsm") {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			src, err := os.ReadFile(path) // #nosec G304 -- walking this repository's own sources
			if err != nil {
				return err
			}
			lines := strings.Split(string(src), "\n")
			for i, line := range lines {
				// Comments mention the path too; only code builds it, and only code may count toward
				// the non-vacuity check below.
				if strings.HasPrefix(strings.TrimSpace(line), "//") || !strings.Contains(line, `".metareview", "runs"`) {
					continue
				}
				sites++
				declared := false
				for j := max(0, i-window); j <= i; j++ {
					if strings.Contains(lines[j], "RunStoreRoot(") || strings.Contains(lines[j], "run-store:") {
						declared = true
					}
				}
				if !declared {
					t.Errorf("%s:%d builds a .metareview/runs path without RunStoreRoot or a `run-store:` declaration within %d lines above", path, i+1, window)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if sites == 0 {
		t.Fatal("found no .metareview/runs sites at all; the walk is not looking at this repository's sources")
	}
}

// TestRunStoreRootIgnoresExportedGitDir: the FSM writer runs git with GIT_* scrubbed, so the reader
// must too. With GIT_DIR exported to an unrelated repository (as inside a git hook or a wrapper),
// RunStoreRoot from a linked worktree must still name that worktree's own main checkout.
func TestRunStoreRootIgnoresExportedGitDir(t *testing.T) {
	gitIn := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		var env []string
		for _, kv := range os.Environ() {
			if !strings.HasPrefix(kv, "GIT_") {
				env = append(env, kv)
			}
		}
		cmd.Env = append(env, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	newRepo := func() string {
		dir, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		gitIn(dir, "init", "-q", "-b", "main")
		gitIn(dir, "-c", "user.email=a@b.c", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "seed")
		return dir
	}
	main, other := newRepo(), newRepo()
	linked := filepath.Join(t.TempDir(), "linked")
	gitIn(main, "worktree", "add", "-q", "--detach", linked)

	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_WORK_TREE", other)
	got, err := filepath.EvalSymlinks(RunStoreRoot(linked))
	if err != nil {
		t.Fatal(err)
	}
	if got != main {
		t.Fatalf("RunStoreRoot with GIT_DIR exported: got %q, want the linked worktree's main checkout %q", got, main)
	}
}

// TestRunStoreRootOutsideARepositoryFallsBack drives the real git call (no seam) from a directory
// that is not in any repository: git fails, and RunStoreRoot falls back to RootOr.
func TestRunStoreRootOutsideARepositoryFallsBack(t *testing.T) {
	start := t.TempDir()
	if got := RunStoreRoot(start); got != RootOr(start) {
		t.Fatalf("outside a repository: got %q, want the RootOr fallback %q", got, RootOr(start))
	}
}
