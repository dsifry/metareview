package cli

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dsifry/metareview/internal/findings"
	"github.com/dsifry/metareview/internal/fsm/editlock"
	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/run"
)

const holderID = "mrv-20260929-000000000000000-fsm-sdlc-loop-sdlc-loop-aaaaaaaa"

// holderRun writes a run into store whose last transition went to a state of kind toKind, with outcome.
func holderRun(t *testing.T, store run.RunStore, toKind run.Kind, outcome run.Outcome) {
	t.Helper()
	b := run.NewBuilder(holderID)
	b.Init(run.InitData{Workflow: "sdlc-loop", WorkflowHash: "wh", RepoMode: "advisory", RepoRoot: "/r", WorkDir: "/r",
		BaseSHA: "base0000", Head: "head0000", InitialState: "discover"})
	b.Event(run.TypeTransition, run.TransitionData{From: "discover", To: "fix", ToKind: toKind, Outcome: outcome, Head: "head0000"})
	events := b.Events()
	st, err := store.Create(holderID, events[0])
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := store.Lock(holderID)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	for _, ev := range events[1:] {
		if st, err = store.Append(holderID, st, ev); err != nil {
			t.Fatal(err)
		}
	}
}

func TestHolderLive(t *testing.T) {
	saved := loadLedger
	t.Cleanup(func() { loadLedger = saved })
	loadLedger = func(string) ([]findings.Record, error) { return nil, nil }

	store := run.NewMemStore(run.Options{})
	if live, err := holderLive(store, "/r", holderID); live || err != nil {
		t.Fatalf("a run that does not exist is gone: %v %v", live, err)
	}
	holderRun(t, store, run.KindAgentEdit, "")
	if live, err := holderLive(store, "/r", holderID); !live || err != nil {
		t.Fatalf("a run in its fix node is live: %v %v", live, err)
	}
	// A granted closure (#179) of the run as it now stands ends the hold; one for an earlier state does not.
	log, _ := store.Events(holderID)
	moved := lastMove(log.Events)
	loadLedger = func(string) ([]findings.Record, error) {
		return []findings.Record{findings.AbandonedRunRecord(holderID, "(x)", "main", "head0000", "an earlier state", "now")}, nil
	}
	if live, _ := holderLive(store, "/r", holderID); !live {
		t.Fatal("a closure row that is not granted, or not for this state, closes nothing")
	}
	granted := findings.AbandonedRunRecord(holderID, "(x)", "main", "head0000", moved, "now")
	granted.Status = findings.StatusOverridden
	loadLedger = func(string) ([]findings.Record, error) { return []findings.Record{granted}, nil }
	if live, _ := holderLive(store, "/r", holderID); live {
		t.Fatal("a granted closure of the run as it stands releases its hold")
	}
	loadLedger = func(string) ([]findings.Record, error) { return nil, errors.New("unreadable") }
	if live, _ := holderLive(store, "/r", holderID); !live {
		t.Fatal("an unreadable ledger closes nothing")
	}

	// A run that left its fix node, or finished, is gone.
	for name, c := range map[string]struct {
		kind    run.Kind
		outcome run.Outcome
	}{"left fix": {"review-lenses", ""}, "finished": {run.KindAgentEdit, run.OutcomeFailed}} {
		s := run.NewMemStore(run.Options{})
		holderRun(t, s, c.kind, c.outcome)
		if live, err := holderLive(s, "/r", holderID); live || err != nil {
			t.Errorf("%s: %v %v", name, live, err)
		}
	}
}

// Unreadable store state is live: the lock is never taken over on a guess.
func TestHolderLiveOnReadErrors(t *testing.T) {
	if live, err := holderLive(brokenStore{err: errors.New("disk")}, "/r", holderID); !live || err == nil {
		t.Fatalf("a store error: %v %v", live, err)
	}
	if live, err := holderLive(brokenStore{events: []run.Event{{Type: run.TypeTree}}}, "/r", holderID); !live || err == nil {
		t.Fatalf("an audit that does not fold: %v %v", live, err)
	}
}

type brokenStore struct {
	run.RunStore
	err    error
	events []run.Event
}

func (b brokenStore) Events(string) (run.Log, error) { return run.Log{Events: b.events}, b.err }

func TestEditLocks(t *testing.T) {
	saved := worktreeStoreDir
	t.Cleanup(func() { worktreeStoreDir = saved })
	dir := t.TempDir()
	worktreeStoreDir = func(string) (string, error) { return dir, nil }
	store := run.NewMemStore(run.Options{})
	holderRun(t, store, run.KindAgentEdit, "")
	lock := editLocks(store, func() time.Time { return time.Unix(0, 0) })("/r")
	l, ok := lock.(editlock.Lock)
	if !ok || l.Path != filepath.Join(dir, "edit.lock") {
		t.Fatalf("lock %+v", lock)
	}
	if err := lock.Acquire(holderID); err != nil {
		t.Fatal(err)
	}
	// Another run cannot take a live holder's lock.
	if err := lock.Acquire("mrv-other"); !errs.Is(err, editlock.CodeEditLocked) || !strings.Contains(err.Error(), holderID) {
		t.Fatalf("got %v", err)
	}

	// A work directory outside any repository cannot be locked, so a fix node cannot be entered there.
	worktreeStoreDir = func(string) (string, error) { return "", errors.New("not a repository") }
	broken := editLocks(store, time.Now)("/nowhere")
	if broken.Acquire("x") == nil || broken.Release("x") == nil {
		t.Fatal("a broken lock fails both ways")
	}
}
