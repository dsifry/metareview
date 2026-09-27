package repo

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// MainWorktreeFromPorcelain reads the main worktree out of `git worktree list --porcelain`: the
// first block, whose first line is `worktree <path>`. bare reports a bare main worktree, which has
// no checkout to hold a run store.
//
// It is the definition of "where FSM runs live" that the FSM (the writer) and record-lenses (a
// reader) share. The FSM stores every run under the main worktree, so a run started from a linked
// worktree lands in the main checkout's .metareview/runs/. record-lenses used to resolve the
// CURRENT worktree instead, and from a linked worktree it reported "no such FSM run" for a run the
// FSM had just created (#169). Any other reader of .metareview/runs/ must use RunStoreRoot or say,
// with a `run-store: current-worktree` comment, why it does not; TestRunStoreReadersAreDeclared
// enforces that.
func MainWorktreeFromPorcelain(out string) (path string, bare bool) {
	block, _, _ := strings.Cut(out, "\n\n")
	lines := strings.Split(block, "\n")
	for _, line := range lines {
		if line == "bare" {
			return "", true
		}
	}
	return strings.TrimPrefix(lines[0], "worktree "), false
}

// runStoreGit is a seam over the git call so the fallback branches are reachable from a test.
var runStoreGit = func(dir string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "worktree", "list", "--porcelain")
	cmd.Dir = dir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// RunStoreRoot is the directory whose .metareview/runs/ holds the FSM runs visible from start: the
// main worktree, exactly as the FSM resolves it when it writes them. Only the run STORE is shared
// — diff identity (base..head) must still come from start's own worktree.
//
// Outside a git repository, or with a bare main worktree (where the FSM refuses to create runs),
// it falls back to RootOr(start); a lookup there finds no run and says so.
func RunStoreRoot(start string) string {
	out, err := runStoreGit(start)
	if err != nil || out == "" {
		return RootOr(start)
	}
	path, bare := MainWorktreeFromPorcelain(out)
	if bare || path == "" {
		return RootOr(start)
	}
	return path
}
