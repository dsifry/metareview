package run

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
)

func legacyRun(t *testing.T, checkout, id string, files map[string]string) {
	t.Helper()
	dir := filepath.Join(checkout, ".metareview", "runs", id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func sumOf(t *testing.T, path string) [32]byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(b)
}

// AC-2.5 (#173): a 0.13.x store migrates into git's common directory with byte-identical files, and re-running is
// a no-op.
func TestMigrateLegacyRunsMovesByteIdentical(t *testing.T) {
	checkout, common := t.TempDir(), t.TempDir()
	want := map[string][32]byte{}
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("mrv-legacy-%07d", i)
		legacyRun(t, checkout, id, map[string]string{"audit.jsonl": fmt.Sprintf("{\"seq\":%d}\n", i), "workflow.yaml": "w"})
		want[id] = sumOf(t, filepath.Join(checkout, ".metareview", "runs", id, "audit.jsonl"))
	}
	rep, err := MigrateLegacyRuns(checkout, common)
	if err != nil || len(rep.Moved) != 5 || len(rep.Collisions) != 0 {
		t.Fatalf("first migration: %+v %v", rep, err)
	}
	for id, sum := range want {
		if got := sumOf(t, filepath.Join(common, "metareview", "runs", id, "audit.jsonl")); got != sum {
			t.Errorf("%s: audit bytes changed in migration", id)
		}
		if _, err := os.Stat(filepath.Join(checkout, ".metareview", "runs", id)); !os.IsNotExist(err) {
			t.Errorf("%s: the legacy copy must be gone after a move", id)
		}
	}
	if rep, err := MigrateLegacyRuns(checkout, common); err != nil || len(rep.Moved) != 0 || len(rep.Collisions) != 0 {
		t.Fatalf("re-running must be a no-op: %+v %v", rep, err)
	}
}

// A run id already in the new store is a collision: reported, never merged, and neither copy is lost.
func TestMigrateLegacyRunsReportsCollisions(t *testing.T) {
	checkout, common := t.TempDir(), t.TempDir()
	const id = "mrv-collide-000001"
	legacyRun(t, checkout, id, map[string]string{"audit.jsonl": "legacy\n"})
	dest := filepath.Join(common, "metareview", "runs", id)
	if err := os.MkdirAll(dest, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "audit.jsonl"), []byte("new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rep, err := MigrateLegacyRuns(checkout, common)
	if err != nil || len(rep.Collisions) != 1 || rep.Collisions[0] != id || len(rep.Moved) != 0 {
		t.Fatalf("collision: %+v %v", rep, err)
	}
	if b, _ := os.ReadFile(filepath.Join(checkout, ".metareview", "runs", id, "audit.jsonl")); string(b) != "legacy\n" {
		t.Fatal("the legacy copy must survive a collision")
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "audit.jsonl")); string(b) != "new\n" {
		t.Fatal("the store's copy must survive a collision")
	}
}

// Two invocations migrating at once neither lose nor duplicate a run.
func TestMigrateLegacyRunsConcurrently(t *testing.T) {
	checkout, common := t.TempDir(), t.TempDir()
	for i := 0; i < 20; i++ {
		legacyRun(t, checkout, fmt.Sprintf("mrv-concur-%07d", i), map[string]string{"audit.jsonl": "x\n"})
	}
	var wg sync.WaitGroup
	moved := make([]int, 4)
	for w := range moved {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			rep, err := MigrateLegacyRuns(checkout, common)
			if err != nil {
				t.Error(err)
			}
			moved[w] = len(rep.Moved)
		}(w)
	}
	wg.Wait()
	total := 0
	for _, n := range moved {
		total += n
	}
	entries, _ := os.ReadDir(filepath.Join(common, "metareview", "runs"))
	runs := 0
	for _, e := range entries {
		if e.IsDir() {
			runs++
		}
	}
	if total != 20 || runs != 20 {
		t.Fatalf("moved %d, store holds %d; want 20 each", total, runs)
	}
}

// Nothing to migrate: no legacy store, or only its own bookkeeping.
func TestMigrateLegacyRunsNothingToDo(t *testing.T) {
	checkout, common := t.TempDir(), t.TempDir()
	if rep, err := MigrateLegacyRuns(checkout, common); err != nil || len(rep.Moved)+len(rep.Collisions) != 0 {
		t.Fatalf("no legacy store: %+v %v", rep, err)
	}
	legacy := filepath.Join(checkout, ".metareview", "runs")
	if err := os.MkdirAll(filepath.Join(legacy, ".torn"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, ".gitignore"), []byte("*\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if rep, err := MigrateLegacyRuns(checkout, common); err != nil || len(rep.Moved)+len(rep.Collisions) != 0 {
		t.Fatalf("bookkeeping only: %+v %v", rep, err)
	}
}

// Every failure surfaces as ERR_STORE_PATH rather than a silent partial migration.
func TestMigrateLegacyRunsFailures(t *testing.T) {
	setup := func(t *testing.T) (string, string) {
		checkout, common := t.TempDir(), t.TempDir()
		legacyRun(t, checkout, "mrv-fail-0000001", map[string]string{"audit.jsonl": "x\n"})
		return checkout, common
	}
	t.Run("store is a file", func(t *testing.T) {
		checkout, common := setup(t)
		_ = os.WriteFile(filepath.Join(common, "metareview"), []byte("x"), 0o600)
		_, err := MigrateLegacyRuns(checkout, common)
		storeErr(t, err, CodeStorePath)
	})
	t.Run("lock is a directory", func(t *testing.T) {
		checkout, common := setup(t)
		_ = os.MkdirAll(filepath.Join(common, "metareview", "migrate.lock"), 0o700)
		_, err := MigrateLegacyRuns(checkout, common)
		storeErr(t, err, CodeStorePath)
	})
	t.Run("lock fails", func(t *testing.T) {
		checkout, common := setup(t)
		orig := migrateFlock
		t.Cleanup(func() { migrateFlock = orig })
		migrateFlock = func(int, int) error { return syscall.EBADF }
		_, err := MigrateLegacyRuns(checkout, common)
		storeErr(t, err, CodeStorePath)
	})
	t.Run("legacy store is a file", func(t *testing.T) {
		checkout, common := t.TempDir(), t.TempDir()
		_ = os.MkdirAll(filepath.Join(checkout, ".metareview"), 0o700)
		_ = os.WriteFile(filepath.Join(checkout, ".metareview", "runs"), []byte("x"), 0o600)
		_, err := MigrateLegacyRuns(checkout, common)
		storeErr(t, err, CodeStorePath)
	})
	t.Run("rename fails", func(t *testing.T) {
		if os.Getuid() == 0 {
			t.Skip("root ignores directory permissions")
		}
		checkout, common := setup(t)
		store := filepath.Join(common, "metareview", "runs")
		_ = os.MkdirAll(store, 0o700)
		_ = os.Chmod(store, 0o500)
		t.Cleanup(func() { _ = os.Chmod(store, 0o700) })
		_, err := MigrateLegacyRuns(checkout, common)
		storeErr(t, err, CodeStorePath)
	})
}

// PendingLegacyRuns is exactly what the next migration would move: not bookkeeping, not a non-run directory, and not
// a collision (which the migration leaves in place, so reporting it as pending would never clear).
func TestPendingLegacyRuns(t *testing.T) {
	checkout, common := t.TempDir(), t.TempDir()
	if got := PendingLegacyRuns(checkout, common); got != nil {
		t.Fatalf("no legacy store: got %v", got)
	}
	legacyRun(t, checkout, "mrv-pending-000001", map[string]string{"audit.jsonl": "x\n"})
	legacyRun(t, checkout, "mrv-collide-000002", map[string]string{"audit.jsonl": "x\n"})
	for _, d := range []string{".torn", "not a run id"} {
		_ = os.MkdirAll(filepath.Join(checkout, ".metareview", "runs", d), 0o700)
	}
	_ = os.MkdirAll(filepath.Join(common, "metareview", "runs", "mrv-collide-000002"), 0o700)
	if got := PendingLegacyRuns(checkout, common); len(got) != 1 || got[0] != "mrv-pending-000001" {
		t.Fatalf("got %v, want only the movable run", got)
	}
	if _, err := MigrateLegacyRuns(checkout, common); err != nil {
		t.Fatal(err)
	}
	if got := PendingLegacyRuns(checkout, common); len(got) != 0 {
		t.Fatalf("after migration nothing is pending, got %v", got)
	}
}
