package run

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

// MigrationReport is what MigrateLegacyRuns did: the run ids it moved, and the ids it left in place because the
// store already held a run of that id.
type MigrationReport struct {
	Moved      []string
	Collisions []string
}

// migrateFlock is the lock seam (tests inject a failing one).
var migrateFlock = syscall.Flock

// MigrateLegacyRuns moves a 0.13.x run store — <checkout>/.metareview/runs/<id>/ in the main checkout — into git's
// common directory, <common>/metareview/runs/<id>/ (#173). Each run is renamed whole, so its files arrive
// byte-identical. It is idempotent (a moved run is gone from the legacy store) and never overwrites: an id the new
// store already holds is a collision, reported and left in place on both sides. An exclusive lock on
// <common>/metareview/migrate.lock serializes concurrent invocations, so none loses or duplicates a run.
func MigrateLegacyRuns(checkout, common string) (MigrationReport, error) {
	rep := MigrationReport{Moved: []string{}, Collisions: []string{}}
	legacy := filepath.Join(checkout, ".metareview", "runs") // root: store (the 0.13.x location being migrated)
	if _, err := os.Lstat(legacy); errors.Is(err, fs.ErrNotExist) {
		return rep, nil
	}
	store := filepath.Join(common, "metareview", "runs") // root: store (git's common directory)
	if err := os.MkdirAll(store, 0o700); err != nil {
		return rep, pathErr(0, err)
	}
	// root: store (git's common directory) — the migration lock beside the store.
	lock, err := os.OpenFile(filepath.Join(common, "metareview", "migrate.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return rep, pathErr(0, err)
	}
	defer func() { _ = lock.Close() }()
	if err := migrateFlock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return rep, pathErr(0, err)
	}
	defer func() { _ = migrateFlock(int(lock.Fd()), syscall.LOCK_UN) }()
	// Read under the lock: another invocation may have moved runs while this one waited.
	entries, err := os.ReadDir(legacy)
	if err != nil {
		return rep, pathErr(0, err)
	}
	for _, e := range entries {
		id := e.Name()
		if !isLegacyRun(e) {
			continue
		}
		dest := filepath.Join(store, id)
		if _, err := os.Lstat(dest); err == nil {
			rep.Collisions = append(rep.Collisions, id)
			continue
		}
		if err := os.Rename(filepath.Join(legacy, id), dest); err != nil {
			return rep, pathErr(0, err)
		}
		rep.Moved = append(rep.Moved, id)
	}
	sort.Strings(rep.Moved)
	sort.Strings(rep.Collisions)
	return rep, nil
}

// isLegacyRun is what the migration moves: a run directory, not the legacy store's own bookkeeping (.gitignore,
// .torn) or anything that is not a valid run id.
func isLegacyRun(e fs.DirEntry) bool {
	return e.IsDir() && !strings.HasPrefix(e.Name(), ".") && ValidateRunID(e.Name()) == nil
}

// PendingLegacyRuns lists, read-only, the 0.13.x runs the next MigrateLegacyRuns(checkout, common) would move: the
// ones it would not skip as bookkeeping or leave behind as collisions. For callers that must not migrate (status).
func PendingLegacyRuns(checkout, common string) []string {
	entries, err := os.ReadDir(filepath.Join(checkout, ".metareview", "runs")) // root: store (the 0.13.x location)
	if err != nil {
		return nil
	}
	var ids []string
	for _, e := range entries {
		if !isLegacyRun(e) {
			continue
		}
		if _, err := os.Lstat(filepath.Join(common, "metareview", "runs", e.Name())); err == nil { // root: store (git's common directory)
			continue // a collision: the migration leaves it, so it is not pending
		}
		ids = append(ids, e.Name())
	}
	return ids
}
