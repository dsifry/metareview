package repo

import (
	"errors"
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
