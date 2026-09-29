package rollback

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil { // WriteFile's mode is masked by umask
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func temps(t *testing.T, dir string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, ".*.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	return matches
}

func TestRestorePutsEveryPathBack(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "docs", "FINDINGS.md")
	private := filepath.Join(dir, "ledger.jsonl")
	created := filepath.Join(dir, "reviews", "run.md")
	createdShared := filepath.Join(dir, "shared.md")
	existingDir := filepath.Join(dir, "context")
	write(t, existing, "before\n", 0o644)
	write(t, private, "rows\n", 0o600)
	if err := os.Mkdir(existingDir, 0o755); err != nil {
		t.Fatal(err)
	}

	set := Take(existing, private, created, createdShared, existingDir)

	// The run writes over, creates, and removes.
	write(t, existing, "after, half-written", 0o644)
	write(t, private, "more rows\n", 0o644)
	write(t, created, "run log", 0o644)
	write(t, createdShared, "a concurrent render", 0o644)
	if err := os.Remove(existingDir); err != nil {
		t.Fatal(err)
	}
	// A hard link to the file the run left: an in-place (truncating) restore would change it too.
	link := filepath.Join(dir, "link")
	if err := os.Link(existing, link); err != nil {
		t.Fatal(err)
	}

	set.Restore()

	if got := read(t, existing); got != "before\n" {
		t.Errorf("existing file: %q", got)
	}
	if got := read(t, link); got != "after, half-written" {
		t.Errorf("the restore must replace the file by rename, not write through it: the link reads %q", got)
	}
	if got := read(t, private); got != "rows\n" {
		t.Errorf("private file: %q", got)
	}
	if info, err := os.Stat(private); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the file's own mode must be kept, got %v (%v)", info.Mode().Perm(), err)
	}
	if _, err := os.Stat(created); !os.IsNotExist(err) {
		t.Errorf("a path the run created must be removed: %v", err)
	}
	if _, err := os.Stat(createdShared); !os.IsNotExist(err) {
		t.Errorf("every path the run created is removed, a render included: %v", err)
	}
	if info, err := os.Stat(existingDir); err != nil || !info.IsDir() {
		t.Errorf("a directory that existed must exist again: %v", err)
	}
	if left := append(temps(t, dir), temps(t, filepath.Join(dir, "docs"))...); len(left) != 0 {
		t.Errorf("temp files left behind: %v", left)
	}
}

func TestTakeRecordsAnUnreadableFileAsAbsent(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a mode-000 file")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "locked")
	write(t, path, "secret", 0o000)
	set := Take(path)
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	set.Restore()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("a path Take could not read is restored as absent: %v", err)
	}
}

// Every failure inside the replace leaves the run's file as it was and no temp file behind.
func TestReplaceFailuresLeaveNoTemp(t *testing.T) {
	boom := errors.New("boom")
	cases := map[string]func(){
		"create": func() { createTemp = func(string, string) (*os.File, error) { return nil, boom } },
		"write":  func() { writeTemp = func(*os.File, []byte) (int, error) { return 0, boom } },
		"sync":   func() { syncTemp = func(*os.File) error { return boom } },
		"close":  func() { closeTemp = func(*os.File) error { return boom } },
		"chmod":  func() { chmod = func(string, os.FileMode) error { return boom } },
		"rename": func() { rename = func(string, string) error { return boom } },
	}
	for name, inject := range cases {
		t.Run(name, func(t *testing.T) {
			saved := [...]any{createTemp, writeTemp, syncTemp, closeTemp, chmod, rename}
			t.Cleanup(func() {
				createTemp = saved[0].(func(string, string) (*os.File, error))
				writeTemp = saved[1].(func(*os.File, []byte) (int, error))
				syncTemp = saved[2].(func(*os.File) error)
				closeTemp = saved[3].(func(*os.File) error)
				chmod = saved[4].(func(string, os.FileMode) error)
				rename = saved[5].(func(string, string) error)
			})
			dir := t.TempDir()
			path := filepath.Join(dir, "FINDINGS.md")
			write(t, path, "run output", 0o644)
			inject()
			if err := replace(path, []byte("before"), 0o644); !errors.Is(err, boom) {
				t.Fatalf("replace: %v", err)
			}
			if got := read(t, path); got != "run output" {
				t.Errorf("a failed replace must not touch the file: %q", got)
			}
			if left := temps(t, dir); len(left) != 0 {
				t.Errorf("temp left behind: %v", left)
			}
		})
	}
}

func TestReplaceNamesItsTempAfterTheFile(t *testing.T) {
	saved := rename
	t.Cleanup(func() { rename = saved })
	var from string
	rename = func(oldpath, newpath string) error { from = oldpath; return saved(oldpath, newpath) }
	dir := t.TempDir()
	if err := replace(filepath.Join(dir, "FINDINGS.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// docs/metareview/.FINDINGS.md.tmp-* is gitignored for the hard-crash window.
	if base := filepath.Base(from); !strings.HasPrefix(base, ".FINDINGS.md.tmp-") || filepath.Dir(from) != dir {
		t.Fatalf("temp %q", from)
	}
}

// A file whose parent directory the run removed is restored with the directory.
func TestRestoreRecreatesAFilesParentDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deeper", "log.md")
	write(t, path, "before", 0o644)
	set := Take(path)
	if err := os.RemoveAll(filepath.Join(dir, "nested")); err != nil {
		t.Fatal(err)
	}
	set.Restore()
	if got := read(t, path); got != "before" {
		t.Fatalf("got %q", got)
	}
}

// A symlinked output path the run wrote through gets its target's content back; one the run replaced with a
// regular file (the gates' write-temp-then-rename writers do) gets the link back, and its target — which the
// run never wrote — is left as it is.
func TestRestorePutsASymlinkedPathBack(t *testing.T) {
	cases := map[string]struct {
		run        func(t *testing.T, link, target string)
		wantTarget string
	}{
		"through the link": {func(t *testing.T, link, target string) { write(t, target, "run output", 0o644) }, "before"},
		"over the link": {func(t *testing.T, link, target string) {
			write(t, target, "another writer", 0o644)
			tmp := link + ".new"
			write(t, tmp, "run output", 0o644)
			if err := os.Rename(tmp, link); err != nil {
				t.Fatal(err)
			}
		}, "another writer"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "elsewhere", "FINDINGS.md")
			link := filepath.Join(dir, "FINDINGS.md")
			write(t, target, "before", 0o644)
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
			set := Take(link)
			c.run(t, link, target)
			set.Restore()
			if got, err := os.Readlink(link); err != nil || got != target {
				t.Fatalf("the path must be the link again: %q %v", got, err)
			}
			if got := read(t, target); got != c.wantTarget {
				t.Fatalf("target: %q, want %q", got, c.wantTarget)
			}
		})
	}
}

// Relinking fails safe: a failed symlink or rename leaves no temp link behind.
func TestRelinkFailures(t *testing.T) {
	boom := errors.New("boom")
	savedSymlink, savedRename := symlink, rename
	t.Cleanup(func() { symlink, rename = savedSymlink, savedRename })
	dir := t.TempDir()
	path := filepath.Join(dir, "FINDINGS.md")
	write(t, path, "regular", 0o644)
	symlink = func(string, string) error { return boom }
	if err := relink(path, "target"); !errors.Is(err, boom) {
		t.Fatalf("symlink: %v", err)
	}
	symlink, rename = savedSymlink, func(string, string) error { return boom }
	if err := relink(path, "target"); !errors.Is(err, boom) {
		t.Fatalf("rename: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, ".FINDINGS.md.link-tmp")); !os.IsNotExist(err) {
		t.Fatalf("temp link left behind: %v", err)
	}
	if got := read(t, path); got != "regular" {
		t.Fatalf("a failed relink must leave the path alone: %q", got)
	}
}

// Where the atomic replace cannot run (a read-only or full directory), the in-place write still restores.
func TestRestoreFallsBackToAnInPlaceWrite(t *testing.T) {
	saved := createTemp
	t.Cleanup(func() { createTemp = saved })
	dir := t.TempDir()
	path := filepath.Join(dir, "runs.jsonl")
	write(t, path, "before", 0o644)
	set := Take(path)
	write(t, path, "run output", 0o644)
	createTemp = func(string, string) (*os.File, error) { return nil, errors.New("read-only directory") }
	set.Restore()
	if got := read(t, path); got != "before" {
		t.Fatalf("got %q", got)
	}
}

// An unresolvable link target (a race after the read) falls back to the path itself.
func TestTakeFallsBackToThePathWhenTheLinkCannotBeResolved(t *testing.T) {
	saved := evalSymlinks
	t.Cleanup(func() { evalSymlinks = saved })
	evalSymlinks = func(string) (string, error) { return "", errors.New("gone") }
	dir := t.TempDir()
	path := filepath.Join(dir, "a.md")
	write(t, path, "before", 0o644)
	set := Take(path)
	write(t, path, "run output", 0o644)
	set.Restore()
	if got := read(t, path); got != "before" {
		t.Fatalf("got %q", got)
	}
}

// A gate's rollback set: every output restored, or removed if the run created it — FINDINGS.md included, so no
// render outlives the ledger it was rendered from.
func TestGateOutputs(t *testing.T) {
	dir := t.TempDir()
	p := func(name string) string { return filepath.Join(dir, name) }
	write(t, p("runs.jsonl"), "runs\n", 0o644)
	write(t, p("findings.jsonl"), "rows\n", 0o644)
	set := GateOutputs(p("context.md"), p("review.md"), p("runs.jsonl"), p("findings.jsonl"), p("FINDINGS.md"))
	for _, name := range []string{"context.md", "review.md", "runs.jsonl", "findings.jsonl", "FINDINGS.md"} {
		write(t, p(name), "run output", 0o644)
	}
	set.Restore()
	for name, want := range map[string]string{"runs.jsonl": "runs\n", "findings.jsonl": "rows\n"} {
		if got := read(t, p(name)); got != want {
			t.Errorf("%s: %q, want %q", name, got, want)
		}
	}
	for _, name := range []string{"context.md", "review.md", "FINDINGS.md"} {
		if _, err := os.Stat(p(name)); !os.IsNotExist(err) {
			t.Errorf("%s must be removed: %v", name, err)
		}
	}
}
