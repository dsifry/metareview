package editlock

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dsifry/metareview/internal/fsm/errs"
)

func lockAt(t *testing.T, live map[string]bool) Lock {
	t.Helper()
	return Lock{
		Path: filepath.Join(t.TempDir(), "metareview", "edit.lock"),
		Now:  func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) },
		Live: func(holder string) (bool, error) { return live[holder], nil },
	}
}

func TestAcquireTakesAFreeLockAndIsReentrant(t *testing.T) {
	l := lockAt(t, nil)
	if err := l.Acquire("run-a"); err != nil {
		t.Fatal(err)
	}
	if err := l.Acquire("run-a"); err != nil {
		t.Fatalf("the holder re-acquiring must keep the lock: %v", err)
	}
	if h, err := l.Holder(); err != nil || h != "run-a" {
		t.Fatalf("holder %q %v", h, err)
	}
	raw, _ := os.ReadFile(l.Path)
	if !strings.Contains(string(raw), `"since":"2026-09-29T12:00:00Z"`) {
		t.Fatalf("the hold records when it was taken: %s", raw)
	}
}

// AC-5.2 at the lock: a second run fails fast, naming the holder.
func TestAcquireRefusesALiveHolder(t *testing.T) {
	l := lockAt(t, map[string]bool{"run-a": true})
	if err := l.Acquire("run-a"); err != nil {
		t.Fatal(err)
	}
	err := l.Acquire("run-b")
	if !errs.Is(err, CodeEditLocked) || !strings.Contains(err.Error(), "run-a") {
		t.Fatalf("want %s naming run-a, got %v", CodeEditLocked, err)
	}
	if h, _ := l.Holder(); h != "run-a" {
		t.Fatalf("a live lock is never stolen: holder %q", h)
	}
}

// AC-5.3: a stale lock (holder gone) is taken over — once its grace has passed.
func TestAcquireTakesOverAStaleHold(t *testing.T) {
	l := lockAt(t, map[string]bool{"run-a": false})
	if err := l.Acquire("run-a"); err != nil {
		t.Fatal(err)
	}
	// Within the grace a hold is live whatever its run's state: its run may not have appended its transition
	// into the fix state yet (or, forked, may not exist yet).
	if err := l.Acquire("run-b"); !errs.Is(err, CodeEditLocked) {
		t.Fatalf("a fresh hold must not be taken over: %v", err)
	}
	taken := l.Now()
	l.Now = func() time.Time { return taken.Add(Grace) }
	if err := l.Acquire("run-b"); err != nil {
		t.Fatalf("a stale hold must be taken over: %v", err)
	}
	if h, _ := l.Holder(); h != "run-b" {
		t.Fatalf("holder %q", h)
	}
}

// A hold dated beyond the grace into the future (a clock stepped back, a hand-edited file) has no grace either.
func TestAFutureDatedHoldHasNoGrace(t *testing.T) {
	l := lockAt(t, map[string]bool{"run-a": false})
	if err := os.MkdirAll(filepath.Dir(l.Path), 0o755); err != nil {
		t.Fatal(err)
	}
	far := l.Now().Add(Grace + time.Second).UTC().Format(time.RFC3339)
	if err := os.WriteFile(l.Path, []byte(`{"run_id":"run-a","since":"`+far+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := l.Acquire("run-b"); err != nil {
		t.Fatalf("a future-dated dead hold must be taken over: %v", err)
	}
	// A little skew within the grace still counts as fresh.
	near := l.Now().Add(Grace / 2).UTC().Format(time.RFC3339)
	if err := os.WriteFile(l.Path, []byte(`{"run_id":"run-a","since":"`+near+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := l.Acquire("run-b"); !errs.Is(err, CodeEditLocked) {
		t.Fatalf("got %v", err)
	}
}

// A hold whose time does not parse has no grace: its run alone decides.
func TestAHoldWithoutATimeHasNoGrace(t *testing.T) {
	l := lockAt(t, map[string]bool{"run-a": false})
	if err := os.MkdirAll(filepath.Dir(l.Path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(l.Path, []byte(`{"run_id":"run-a","since":"yesterday"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := l.Acquire("run-b"); err != nil {
		t.Fatalf("got %v", err)
	}
}

// A holder whose liveness cannot be read is treated as live: the lock is never stolen on a guess.
func TestAcquireTreatsAnUnreadableHolderAsLive(t *testing.T) {
	l := lockAt(t, nil)
	if err := l.Acquire("run-a"); err != nil {
		t.Fatal(err)
	}
	l.Live = func(string) (bool, error) { return false, errors.New("store unreadable") }
	if err := l.Acquire("run-b"); !errs.Is(err, CodeEditLocked) {
		t.Fatalf("got %v", err)
	}
}

func TestReleaseDropsOnlyTheHoldersLock(t *testing.T) {
	l := lockAt(t, map[string]bool{"run-a": true})
	if err := l.Release("run-a"); err != nil {
		t.Fatalf("releasing no lock is a no-op: %v", err)
	}
	if err := l.Acquire("run-a"); err != nil {
		t.Fatal(err)
	}
	if err := l.Release("run-b"); err != nil {
		t.Fatal(err)
	}
	if h, _ := l.Holder(); h != "run-a" {
		t.Fatalf("another run's release must not drop the hold: %q", h)
	}
	if err := l.Release("run-a"); err != nil {
		t.Fatal(err)
	}
	if h, _ := l.Holder(); h != "" {
		t.Fatalf("holder after release: %q", h)
	}
	if _, err := os.Stat(l.Path); !os.IsNotExist(err) {
		t.Fatalf("the lock file is removed: %v", err)
	}
}

// A malformed lock file is never taken over or dropped.
func TestAMalformedLockIsRefused(t *testing.T) {
	l := lockAt(t, nil)
	if err := os.MkdirAll(filepath.Dir(l.Path), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"not json", `{"run_id":""}`} {
		if err := os.WriteFile(l.Path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := l.Acquire("run-a"); !errs.Is(err, CodeEditLocked) || !strings.Contains(err.Error(), "malformed") {
			t.Errorf("%q: got %v", body, err)
		}
		if err := l.Release("run-a"); err == nil {
			t.Errorf("%q: release must fail too", body)
		}
	}
}

func TestFailures(t *testing.T) {
	boom := errors.New("boom")
	t.Run("lock dir", func(t *testing.T) {
		l := lockAt(t, nil)
		blocker := filepath.Dir(l.Path)
		if err := os.WriteFile(blocker, nil, 0o644); err != nil { // a file where the directory belongs
			t.Fatal(err)
		}
		if err := l.Acquire("run-a"); err == nil {
			t.Fatal("want an error")
		}
	})
	t.Run("guard open", func(t *testing.T) {
		l := lockAt(t, nil)
		if err := os.MkdirAll(l.Path+".guard", 0o755); err != nil { // a directory where the guard file belongs
			t.Fatal(err)
		}
		if err := l.Acquire("run-a"); err == nil {
			t.Fatal("want an error")
		}
	})
	t.Run("flock", func(t *testing.T) {
		saved := flock
		t.Cleanup(func() { flock = saved })
		flock = func(int, int) error { return boom }
		if err := lockAt(t, nil).Acquire("run-a"); !errors.Is(err, boom) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("read", func(t *testing.T) {
		l := lockAt(t, nil)
		if err := os.MkdirAll(l.Path, 0o755); err != nil { // a directory where the lock file belongs
			t.Fatal(err)
		}
		if err := l.Acquire("run-a"); err == nil {
			t.Fatal("want an error")
		}
		if _, err := l.Holder(); err == nil {
			t.Fatal("want an error")
		}
	})
	t.Run("write", func(t *testing.T) {
		saved := writeHold
		t.Cleanup(func() { writeHold = saved })
		writeHold = func(string, []byte) error { return boom }
		if err := lockAt(t, nil).Acquire("run-a"); !errors.Is(err, boom) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("remove", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root removes from a read-only directory")
		}
		l := lockAt(t, nil)
		if err := l.Acquire("run-a"); err != nil {
			t.Fatal(err)
		}
		dir := filepath.Dir(l.Path)
		// The guard exists already, so the release can open it; the directory then refuses the removal.
		if err := os.Chmod(dir, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
		if err := l.Release("run-a"); err == nil {
			t.Fatal("want an error")
		}
	})
}

func TestWriteAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.lock")
	if err := writeAtomic(path, []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(filepath.Join(dir, "missing", "edit.lock"), []byte("x")); err == nil {
		t.Fatal("a missing directory must fail the write")
	}
	// A directory at the target makes the rename fail; the temp file is removed.
	target := filepath.Join(dir, "occupied")
	if err := os.MkdirAll(filepath.Join(target, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(target, []byte("x")); err == nil {
		t.Fatal("want a rename error")
	}
	if _, err := os.Stat(target + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temp left behind: %v", err)
	}
}
