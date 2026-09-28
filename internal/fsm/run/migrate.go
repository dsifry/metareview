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
		if !e.IsDir() || strings.HasPrefix(id, ".") || ValidateRunID(id) != nil {
			continue // the legacy store's own bookkeeping (.gitignore, .torn), or not a run
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
