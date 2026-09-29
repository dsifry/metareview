// Package rollback puts back the files a gate run touched when the run fails (#152). The four gates
// (task-done, pr-ready, epic-ready, post-merge learning) each kept their own copy of this, restoring with
// a truncating os.WriteFile at mode 0644 and removing every path the run had created.
package rollback

import (
	"os"
	"path/filepath"
)

// snapshot is one path's state before the run wrote it. For a symlinked path, link is the link's own text
// and target the file it resolves to: the restore puts the target's content back and the link itself.
type snapshot struct {
	existed bool
	isDir   bool
	content []byte
	mode    os.FileMode
	target  string
	link    string
}

// Set is the pre-run state of a gate run's output paths.
type Set struct {
	files  map[string]snapshot
	shared map[string]bool
}

// Take records each path's state before the run writes it. A path that cannot be read is recorded as
// absent, as the gates always did: restoring it removes what the run left there.
func Take(paths ...string) *Set {
	s := &Set{files: map[string]snapshot{}, shared: map[string]bool{}}
	for _, path := range paths {
		s.files[path] = take(path)
	}
	return s
}

func take(path string) snapshot {
	info, err := os.Stat(path)
	if err != nil {
		return snapshot{}
	}
	if info.IsDir() {
		return snapshot{existed: true, isDir: true}
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return snapshot{}
	}
	snap := snapshot{existed: true, content: content, mode: info.Mode().Perm(), target: path}
	if link, err := os.Readlink(path); err == nil {
		if target, err := evalSymlinks(path); err == nil {
			snap.target, snap.link = target, link
		}
	}
	return snap
}

// GateOutputs is the rollback set of a review gate (task-done, pr-ready, epic-ready): its context pack and
// review log, the run and findings ledgers, and the shared FINDINGS.md render.
func GateOutputs(contextPack, reviewLog, runs, ledger, findingsIndex string) *Set {
	return Take(contextPack, reviewLog, runs, ledger, findingsIndex).Shared(findingsIndex)
}

// Shared marks paths other processes also write — the rendered docs/metareview/FINDINGS.md, which a
// concurrent render replaces by rename. If the run created one, Restore leaves it where it is instead of
// removing it: in that window the file may be the other render's, and the rendered index is derived from
// the ledger (which the run does restore), so a leftover render heals at the next one.
func (s *Set) Shared(paths ...string) *Set {
	for _, path := range paths {
		s.shared[path] = true
	}
	return s
}

// Restore puts every path back as Take found it. A file that existed is replaced write-temp-then-rename
// with its own mode, never truncated in place, so a crash mid-restore leaves the old or the new content,
// never half of either. A symlinked path gets its target's content back and the link itself — the gates'
// writers replace a link with a regular file (write-temp-then-rename). Where the directory refuses the
// temp file (read-only, full), the old in-place write is the fallback, so a restore the truncating writer
// could make still happens. A path the run created is removed, unless it is Shared. Restore is best
// effort: it runs on a path that is already failing, and each path is restored independently.
func (s *Set) Restore() {
	for path, snap := range s.files {
		switch {
		case snap.isDir:
			_ = os.MkdirAll(path, 0o755)
		case snap.existed:
			_ = os.MkdirAll(filepath.Dir(snap.target), 0o755)
			if replace(snap.target, snap.content, snap.mode) != nil {
				_ = writeInPlace(snap.target, snap.content, snap.mode)
			}
			if snap.link != "" {
				_ = relink(path, snap.link)
			}
		case !s.shared[path]:
			_ = os.Remove(path)
		}
	}
}

// Seams over the fallible calls in replace, so each error branch — unreachable on a healthy filesystem —
// is exercised by fault injection.
var (
	createTemp   = os.CreateTemp
	writeTemp    = func(f *os.File, b []byte) (int, error) { return f.Write(b) }
	syncTemp     = func(f *os.File) error { return f.Sync() }
	closeTemp    = func(f *os.File) error { return f.Close() }
	chmod        = os.Chmod
	rename       = os.Rename
	writeInPlace = os.WriteFile
	evalSymlinks = filepath.EvalSymlinks
	symlink      = os.Symlink
)

// relink makes path the symlink to link again, unless it already is: a temp link beside it, renamed over it.
func relink(path, link string) error {
	if current, err := os.Readlink(path); err == nil && current == link {
		return nil
	}
	tmp := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".link-tmp")
	_ = os.Remove(tmp)
	if err := symlink(link, tmp); err != nil {
		return err
	}
	if err := rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// replace writes content to a uniquely named temp file beside path (one filesystem, so the rename is
// atomic), syncs it, gives it mode, and renames it over path. The temp file is removed on every path that
// does not rename it.
func replace(path string, content []byte, mode os.FileMode) error {
	tmp, err := createTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	fail := func(err error) error {
		_ = tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if _, err := writeTemp(tmp, content); err != nil {
		return fail(err)
	}
	if err := syncTemp(tmp); err != nil {
		return fail(err)
	}
	if err := closeTemp(tmp); err != nil {
		return fail(err)
	}
	if err := chmod(name, mode); err != nil {
		return fail(err)
	}
	if err := rename(name, path); err != nil {
		return fail(err)
	}
	return nil
}
