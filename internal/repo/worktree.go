package repo

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/dsifry/metareview/internal/fsm/gate"
)

// MainWorktreeFromPorcelain reads the main worktree out of `git worktree list --porcelain`: the
// first block, whose first line is `worktree <path>`. bare reports a bare main worktree, which has
// no checkout to hold a run store.
//
// The main worktree is a run's anchor (RepoRoot) and the 0.13.x run-store location. Since #173 the runs themselves
// live in StoreDir (git's common directory), shared by every worktree; the FSM migrates a 0.13.x store out of the
// main checkout's .metareview/runs/ on first use. Every place outside the FSM that builds a runs path must declare
// which store it means: a call to StoreDir or RunStoreRoot, or a `run-store:` comment (`shared` for the common or
// legacy shared location, `current-worktree` with the reason when it deliberately reads a worktree's own), on that
// line or within the three lines above. TestRunStoreReadersAreDeclared checks each such site. It is a tripwire, not
// proof: it matches literal path elements only, so a path spelled another way is not seen.
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

// runStoreGit is a seam over the git call so the fallback branches are reachable from a test. It
// runs git exactly as the FSM does when it writes a run (gate.RealExec: GIT_* variables scrubbed,
// system config off), so an exported GIT_DIR or GIT_WORK_TREE — a git hook, a wrapper — cannot
// point the reader at a different store than the writer used.
var runStoreGit = func(dir string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, _, code, err := gate.RealExec(ctx, dir, nil, "worktree", "list", "--porcelain")
	if err != nil || code != 0 {
		return "", errNotARepo
	}
	return strings.TrimSpace(string(out)), nil
}

var errNotARepo = errors.New("git worktree list failed")

// RunStoreRoot is the main worktree, as the FSM resolves it: a run's anchor (RepoRoot) and the 0.13.x run-store
// location (.metareview/runs/), which the FSM migrates into StoreDir on first use (#173) and which readers consult
// for one release as a fallback. Current runs live in StoreDir. Diff identity (base..head) must still come from
// start's own worktree.
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

// commonDirGit is the seam over `git rev-parse --git-common-dir`, run exactly as the FSM runs git (gate.RealExec:
// GIT_* scrubbed), so an exported GIT_DIR cannot point a reader at a different store than the writer used.
var commonDirGit = func(dir string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, _, code, err := gate.RealExec(ctx, dir, nil, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil || code != 0 {
		return "", errNotARepo
	}
	return strings.TrimSpace(string(out)), nil
}

// StoreDir is metareview's shared store for the repository containing start (#173): <git-common-dir>/metareview.
// The main checkout and every linked worktree resolve the same directory, it exists in a bare repository, and it
// does not depend on any one checkout: moving or deleting the main checkout, or `git clean -fdX` in it, leaves it
// alone. FSM runs live in its runs/ (alongside the session bindings in sessions/, #166).
func StoreDir(start string) (string, error) {
	common, err := commonDirGit(start)
	if err != nil || common == "" {
		return "", errNotARepo
	}
	return StoreDirIn(common), nil
}

// StoreDirIn is the store inside a known git common directory — the one definition of its layout, for a caller that
// already has the common dir from its own git call (session bindings).
func StoreDirIn(common string) string { return filepath.Join(common, "metareview") }

// Toplevel is the root of the work tree containing dir, via the same scrubbed git as StoreDir.
func Toplevel(dir string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, _, code, err := gate.RealExec(ctx, dir, nil, "rev-parse", "--show-toplevel")
	if err != nil || code != 0 {
		return "", errNotARepo
	}
	return strings.TrimSpace(string(out)), nil
}
