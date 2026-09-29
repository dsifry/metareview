package cli

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/dsifry/metareview/internal/findings"
	"github.com/dsifry/metareview/internal/fsm/editlock"
	"github.com/dsifry/metareview/internal/fsm/machine"
	"github.com/dsifry/metareview/internal/fsm/run"
	"github.com/dsifry/metareview/internal/repo"
)

// Seams over the worktree's git dir and its findings ledger, so the edit lock's failure paths are testable.
var (
	worktreeStoreDir = repo.WorktreeStoreDir
	loadLedger       = findings.Load
)

// editLocks is the per-worktree fix-loop lock (#180) of each work directory, judged live against store: a hold
// is live while its run is in an agent-edit state, unfinished and not closed.
func editLocks(store run.RunStore, now func() time.Time) func(workDir string) machine.EditLocker {
	return func(workDir string) machine.EditLocker {
		dir, err := worktreeStoreDir(workDir)
		if err != nil {
			return brokenLock{err}
		}
		return editlock.Lock{
			Path: filepath.Join(dir, "edit.lock"), // root: work (this worktree's own git dir, never shared)
			Now:  now,
			Live: func(holder string) (bool, error) { return holderLive(store, workDir, holder) },
		}
	}
}

// brokenLock is the lock of a work directory whose git dir cannot be found: nothing can be locked there, so
// entering a fix node fails rather than going ahead unguarded.
type brokenLock struct{ err error }

func (b brokenLock) Acquire(string) error { return b.err }
func (b brokenLock) Release(string) error { return b.err }

// holderLive reports whether holder still holds its worktree's fix node: its run exists, is unfinished, is in an
// agent-edit state, and has no granted closure (#179) for the state it is in. A run that no longer exists is gone;
// any other read error is returned, and the lock treats it as live.
func holderLive(store run.RunStore, workDir, holder string) (bool, error) {
	log, err := store.Events(holder)
	var se *run.StoreError
	if errors.As(err, &se) && se.Code == run.CodeRunNotFound {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	snap, err := run.Fold(log.Events)
	if err != nil {
		return true, err
	}
	if snap.Outcome != "" || snap.StateKind != run.KindAgentEdit {
		return false, nil
	}
	return !closed(workDir, holder, lastMove(log.Events)), nil
}

// lastMove is the time of the run's last event that is not a note, as a closure records it (RunUpdated).
func lastMove(events []run.Event) string {
	moved := ""
	for _, ev := range events {
		if ev.Type != run.TypeRecord {
			raw, _ := json.Marshal(ev.At)
			moved = strings.Trim(string(raw), `"`)
		}
	}
	return moved
}

// closed reports whether workDir's ledger grants a closure of runID as it now stands. An unreadable ledger closes
// nothing.
func closed(workDir, runID, moved string) bool {
	ledger, err := loadLedger(workDir)
	if err != nil {
		return false
	}
	for _, record := range ledger {
		if findings.IsRunClosure(record) && record.ID == runID && record.Status == findings.StatusOverridden && record.RunUpdated == moved {
			return true
		}
	}
	return false
}
