package repo

import (
	"errors"
	"io/fs"
	"os"
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
// (which owns the store), every Go file that builds a `.metareview/runs` path must either resolve
// it through RunStoreRoot or carry a `run-store: current-worktree` comment explaining why not.
func TestRunStoreReadersAreDeclared(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
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
			text := string(src)
			if !strings.Contains(text, `".metareview", "runs"`) {
				return nil
			}
			if !strings.Contains(text, "RunStoreRoot(") && !strings.Contains(text, "run-store: current-worktree") {
				t.Errorf("%s builds a .metareview/runs path without repo.RunStoreRoot or a `run-store: current-worktree` justification", path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
