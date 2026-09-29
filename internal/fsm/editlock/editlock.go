// Package editlock is the per-worktree fix-loop lock (#180). A fix node (agent-edit) edits the very tree its run
// then re-reviews, so two runs fixing in one worktree would corrupt each other's diff. The lock lets at most one
// run hold a worktree's agent-edit node at a time.
//
// A node spans several processes — the host edits between `fsm advance` and `fsm record` — so no process can hold
// an OS lock for it. The lock is a file naming its holder run, taken when a run enters an agent-edit state and
// dropped when it leaves one. Every read-decide-write of that file happens under an flock on a guard file beside
// it, so two processes never both take the lock. A holder is live while its run is still in an agent-edit state;
// a lock whose holder is gone (finished, stopped, abandoned, deleted) is stale and taken over, and a live lock is
// never stolen.
package editlock

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/dsifry/metareview/internal/fsm/errs"
)

// CodeEditLocked is the error a second run gets when another live run holds the worktree's edit lock.
const CodeEditLocked = "ERR_EDIT_LOCKED"

// Lock is one worktree's edit lock.
type Lock struct {
	// Path is the lock file: <this worktree's git dir>/metareview/edit.lock.
	Path string
	// Live reports whether holder is still in an agent-edit node. An error counts as live: a lock is never
	// stolen on a guess.
	Live func(holder string) (bool, error)
	// Now stamps a new hold.
	Now func() time.Time
}

// hold is the lock file's content.
type hold struct {
	RunID string `json:"run_id"`
	Since string `json:"since"`
}

// Seams over the guard's flock and the hold's write, so their failure branches are testable.
var (
	flock     = syscall.Flock
	writeHold = writeAtomic
)

// Acquire takes the lock for runID, or fails fast with CodeEditLocked naming the live holder. It is re-entrant:
// a run that already holds the lock keeps it.
func (l Lock) Acquire(runID string) error {
	return l.guarded(func() error {
		current, err := l.read()
		if err != nil {
			return err
		}
		if current.RunID != "" && current.RunID != runID {
			live, err := l.Live(current.RunID)
			if err != nil || live {
				return errs.E(CodeEditLocked, "another run is fixing in this worktree; run one fix loop per worktree (or use another worktree) and retry once it leaves its fix node",
					"holder", current.RunID, "since", current.Since, "lock", l.Path)
			}
		}
		if current.RunID == runID {
			return nil
		}
		data, _ := json.Marshal(hold{RunID: runID, Since: l.Now().UTC().Format(time.RFC3339)})
		return writeHold(l.Path, append(data, '\n'))
	})
}

// Release drops runID's hold. Releasing a lock another run holds, or none, is a no-op.
func (l Lock) Release(runID string) error {
	return l.guarded(func() error {
		current, err := l.read()
		if err != nil || current.RunID != runID {
			return err
		}
		if err := os.Remove(l.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	})
}

// Holder is the run that holds the lock, or "".
func (l Lock) Holder() (string, error) {
	current, err := l.read()
	return current.RunID, err
}

// read returns the current hold. A missing file is no hold; an unreadable or malformed one is an error, so the
// lock is never taken over on a guess.
func (l Lock) read() (hold, error) {
	raw, err := os.ReadFile(l.Path)
	if errors.Is(err, os.ErrNotExist) {
		return hold{}, nil
	}
	if err != nil {
		return hold{}, err
	}
	var h hold
	if err := json.Unmarshal(raw, &h); err != nil || h.RunID == "" {
		return hold{}, errs.E(CodeEditLocked, "the edit lock file is malformed; remove it once no fix loop is running in this worktree", "lock", l.Path)
	}
	return h, nil
}

// guarded runs fn under an exclusive flock on the guard file beside the lock.
func (l Lock) guarded(fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(l.Path), 0o755); err != nil {
		return err
	}
	guard, err := os.OpenFile(l.Path+".guard", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = guard.Close() }()
	if err := flock(int(guard.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer func() { _ = flock(int(guard.Fd()), syscall.LOCK_UN) }()
	return fn()
}

// writeAtomic replaces path with data by a temp file and a rename, so a reader sees the old hold or the new.
func writeAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
