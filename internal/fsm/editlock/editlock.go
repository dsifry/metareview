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
	// Now stamps a new hold, and dates it.
	Now func() time.Time
}

// Grace is how long a new hold counts as live whatever its run's state says. A run takes the lock before it
// appends its transition into the fix state (and a fork before its child exists), so for that moment its run does
// not yet look like a holder; without the grace a second run arriving then would judge the hold stale and take it
// over, and both would edit the tree (#180).
const Grace = 2 * time.Minute

// hold is the lock file's content.
type hold struct {
	RunID string `json:"run_id"`
	Since string `json:"since"`
	// PID is the process that took the hold. While it is alive the hold is live whatever its run's state: the
	// process is between taking the lock and publishing its fix state (#180).
	PID int `json:"pid,omitempty"`
}

// Seams over the guard's flock, the hold's write and the process checks, so their branches are testable.
var (
	flock     = syscall.Flock
	writeHold = writeAtomic
	getpid    = os.Getpid
	alive     = func(pid int) bool {
		err := syscall.Kill(pid, 0)
		return err == nil || errors.Is(err, syscall.EPERM)
	}
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
			if err != nil || live || l.fresh(current) || acquiring(current) {
				return errs.E(CodeEditLocked, "another run is fixing in this worktree; run one fix loop per worktree (or use another worktree) and retry once it leaves its fix node",
					"holder", current.RunID, "since", current.Since, "lock", l.Path)
			}
		}
		if current.RunID == runID {
			return nil
		}
		data, _ := json.Marshal(hold{RunID: runID, Since: l.Now().UTC().Format(time.RFC3339), PID: getpid()})
		return writeHold(l.Path, append(data, '\n'))
	})
}

// fresh reports whether h was taken within Grace. A hold whose time does not parse, or lies further in the future
// than Grace (a clock stepped back, a hand-edited file), is not fresh: its run decides, so it cannot pin the lock.
func (l Lock) fresh(h hold) bool {
	since, err := time.Parse(time.RFC3339, h.Since)
	age := l.Now().Sub(since)
	return err == nil && age > -Grace && age < Grace
}

// acquiring reports whether the process that took h is another one still running: it may be paused between
// taking the lock and appending its transition into the fix state, and no grace bounds how long that takes. A
// reused PID only keeps a dead hold a little longer — never steals one.
func acquiring(h hold) bool {
	return h.PID > 0 && h.PID != getpid() && alive(h.PID)
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
