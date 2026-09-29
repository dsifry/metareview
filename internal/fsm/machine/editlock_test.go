package machine

import (
	"context"
	"errors"
	"testing"

	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/run"
)

// fakeLock is one worktree's edit lock: it records its calls and refuses a second holder.
type fakeLock struct {
	holder     string
	calls      []string
	dirs       []string
	releaseErr error
}

func (f *fakeLock) Acquire(runID string) error {
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
