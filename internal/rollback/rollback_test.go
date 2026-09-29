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

	set := Take(existing, private, created, createdShared, existingDir).Shared(createdShared)

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
	if got := read(t, createdShared); got != "a concurrent render" {
		t.Errorf("a shared path the run created is left in place: %q", got)
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
