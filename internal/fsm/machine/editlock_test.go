package machine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/run"
)

// fakeLock is one worktree's edit lock: it records its calls and refuses a second holder.
type fakeLock struct {
	off        bool // a lock that does nothing: a run begun before the lock existed
	holder     string
	calls      []string
	dirs       []string
	releaseErr error
}

func (f *fakeLock) Acquire(runID string) error {
	if f.off {
		return nil
	}
	f.calls = append(f.calls, "acquire "+runID)
	if f.holder != "" && f.holder != runID {
		return errs.E("ERR_EDIT_LOCKED", "held", "holder", f.holder)
	}
	f.holder = runID
	return nil
}

func (f *fakeLock) Release(runID string) error {
	f.calls = append(f.calls, "release "+runID)
	if f.holder == runID {
		f.holder = ""
	}
	return f.releaseErr
}

func withLock(h *harness) *fakeLock {
	lock := &fakeLock{}
	h.deps.EditLock = func(dir string) EditLocker { lock.dirs = append(lock.dirs, dir); return lock }
	return lock
}

// toFix drives a fresh sdlc-loop run to its fix node's input.
func toFix(t *testing.T, h *harness) *Machine {
	t.Helper()
	h.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
	m := h.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars})
	h.advance(m)
	h.record(m, "discover", findings(1))
	h.advance(m)
	if r := h.advance(m); r.To != "fix" {
		t.Fatalf("→fix: %+v", r)
	}
	return m
}

// AC-5.2 (#180) at the machine: a second run needing the fix node in the same worktree fails fast with the lock
// error, having recorded nothing, and goes ahead once the first leaves its fix node.
func TestEditLockSerializesFixNodesInOneWorktree(t *testing.T) {
	h := newHarness(t)
	lock := withLock(h)
	a := toFix(t, h)
	if lock.holder != a.runID {
		t.Fatalf("entering fix takes the lock: %+v", lock)
	}
	b := h.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars})
	h.advance(b)
	h.record(b, "discover", findings(1))
	h.advance(b)
	_, err := b.Advance(context.Background())
	if !errs.Is(err, "ERR_EDIT_LOCKED") || errs.As(err).Fields["holder"] != a.runID {
		t.Fatalf("the second run must fail fast naming the holder: %v", err)
	}
	// Its adjudication is kept (a retry reuses it rather than calling the judge again), but no transition into
	// fix is recorded: it stays where it was.
	for _, ev := range h.events(b) {
		if ev.Type == run.TypeTransition && decode[run.TransitionData](t, ev).To == "fix" {
			t.Fatal("a refused run must not record a transition into fix")
		}
	}
	if b.View().Snapshot.State != "adjudicate" {
		t.Fatalf("state %s", b.View().Snapshot.State)
	}
	// The first run's fix completes and leaves the node; the lock goes with it.
	h.advance(a)
	h.record(a, "fix", `{"commit":"`+shaFix+`","summary":"fixed"}`)
	if r := h.advance(a); r.To != "verify" || lock.holder != "" {
		t.Fatalf("leaving fix releases the lock: %+v %+v", r, lock)
	}
	if r := h.advance(b); r.To != "fix" || lock.holder != b.runID {
		t.Fatalf("the second run goes ahead once the lock is free: %+v %+v", r, lock)
	}
	for _, dir := range lock.dirs {
		if dir != "/repo" {
			t.Fatalf("the lock is the run's work directory's: %v", lock.dirs)
		}
	}
}

// A run that finishes (here: fails) in its fix node drops the lock; a fork into fix takes it for the child.
func TestEditLockIsReleasedAtTheEndAndTakenByAForkIntoFix(t *testing.T) {
	h := newHarness(t)
	lock := withLock(h)
	a := toFix(t, h)
	h.git.def.Counts = nil // the fix below adds no commit, so its gate fails and the run ends
	h.advance(a)
	h.record(a, "fix", `{"commit":"`+shaHead+`","summary":"no commit"}`)
	if r := h.advance(a); r.Status != StatusGateFailed || lock.holder != "" {
		t.Fatalf("a run that ends releases the lock: %+v %+v", r, lock)
	}
	child, _, err := a.Fork(context.Background(), ForkOptions{From: "fix"})
	if err != nil {
		t.Fatal(err)
	}
	if lock.holder != child.runID {
		t.Fatalf("a child forked into fix holds the lock: %+v", lock)
	}
	// A fork into fix while another live run holds the lock is refused before the child exists.
	lock.holder = "someone-else"
	runs, _ := h.store.List()
	if _, _, err := a.Fork(context.Background(), ForkOptions{From: "fix"}); !errs.Is(err, "ERR_EDIT_LOCKED") {
		t.Fatalf("got %v", err)
	}
	if after, _ := h.store.List(); len(after) != len(runs) {
		t.Fatalf("a refused fork creates no child: %d→%d runs", len(runs), len(after))
	}
}

// A failed release is best effort: the run still leaves its fix node (the stale hold is taken over later).
func TestEditLockReleaseFailureDoesNotBlockTheRun(t *testing.T) {
	h := newHarness(t)
	lock := withLock(h)
	lock.releaseErr = errors.New("disk gone")
	a := toFix(t, h)
	h.advance(a)
	h.record(a, "fix", `{"commit":"`+shaFix+`","summary":"fixed"}`)
	if r := h.advance(a); r.To != "verify" {
		t.Fatalf("got %+v", r)
	}
}

// Without an edit lock the machine behaves as before.
func TestNoEditLock(t *testing.T) {
	h := newHarness(t)
	a := toFix(t, h)
	h.advance(a)
	h.record(a, "fix", `{"commit":"`+shaFix+`","summary":"fixed"}`)
	if r := h.advance(a); r.To != "verify" {
		t.Fatalf("got %+v", r)
	}
}

// collidingStore refuses every Create as an existing id.
type collidingStore struct {
	run.RunStore
	creates int
}

func (c *collidingStore) Create(id string, _ run.Event) (run.FoldState, error) {
	c.creates++
	return run.FoldState{}, &run.StoreError{Code: run.CodeRunExists, Detail: id}
}

// #180: runs started at once in several worktrees can generate one id (the init time on a microsecond clock).
// The second takes the next microsecond; an id the caller chose is never changed; the retry is bounded.
func TestInitTakesTheNextIDWhenOneIsTaken(t *testing.T) {
	h := newHarness(t)
	frozen := h.deps.Clock()
	h.deps.Clock = func() run.Time { return frozen }
	a := h.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars})
	b := h.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars})
	if a.runID == b.runID {
		t.Fatalf("both runs got %s", a.runID)
	}
	if got := b.View().Snapshot.CreatedAt.Sub(frozen.Time); got != time.Microsecond {
		t.Fatalf("the second run is a microsecond later: %v", got)
	}
	if _, err := h.init(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, RunID: a.runID}); err == nil {
		t.Fatal("an id the caller chose is never changed")
	}
	store := &collidingStore{RunStore: h.deps.Store}
	h.deps.Store = store
	if _, err := h.init(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars}); err == nil || store.creates != maxIDAttempts {
		t.Fatalf("the retry is bounded: %v after %d creates", err, store.creates)
	}
}

// A run already in its fix node without the lock — begun before 0.14, or started in a fix state — takes it at its
// next advance, or is refused while another run holds it (#180).
func TestARunInItsFixNodeTakesTheLockAtItsNextAdvance(t *testing.T) {
	h := newHarness(t)
	lock := withLock(h)
	lock.off = true
	a := toFix(t, h) // it entered fix holding nothing: an in-flight run from before the lock existed
	lock.off = false
	lock.holder = "another-run"
	if _, err := a.Advance(context.Background()); !errs.Is(err, "ERR_EDIT_LOCKED") {
		t.Fatalf("a run in fix must not be handed the node while another holds the lock: %v", err)
	}
	lock.holder = ""
	if r := h.advance(a); r.Status != StatusNeedsInput || lock.holder != a.runID {
		t.Fatalf("it takes the lock and gets its node: %+v %+v", r, lock)
	}
}
