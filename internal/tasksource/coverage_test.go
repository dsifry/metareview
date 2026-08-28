package tasksource

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every remaining branch of this file, including the defensive ones the seams
// exist for. A task target decides what gets reviewed, so a wrong answer here
// silently reviews the wrong thing.

func TestResolveRejectsAnEmptyTarget(t *testing.T) {
	for _, target := range []string{"", "   ", "\t\n"} {
		if _, err := Resolve(t.TempDir(), target); err == nil ||
			!strings.Contains(err.Error(), "task target is required") {
			t.Fatalf("%q: %v", target, err)
		}
	}
}

func TestResolveFallsBackToAdvisoryWhenBeadsHasNoMatch(t *testing.T) {
	root := t.TempDir()
	src, err := Resolve(root, "mrv-123")
	if err != nil || src.Kind != "advisory" || src.ID != "mrv-123" ||
		!strings.Contains(src.Body, "Advisory task target: mrv-123") {
		t.Fatalf("%+v %v", src, err)
	}
}

func TestResolveBeads(t *testing.T) {
	write := func(t *testing.T, content string) string {
		t.Helper()
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, ".beads"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, ".beads", "issues.jsonl"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return root
	}

	t.Run("blank lines are skipped and a match wins", func(t *testing.T) {
		root := write(t, "\n   \n"+
			`{"id":"other","title":"no"}`+"\n"+
			`{"id":"mrv-1","summary":"fallback title","status":"open","description":"d"}`+"\n")
		src, err := Resolve(root, "mrv-1")
		if err != nil || src.Kind != "beads" || src.Title != "fallback title" ||
			src.Path != ".beads/issues.jsonl" || !strings.Contains(src.Body, "Status: open") {
			t.Fatalf("%+v %v", src, err)
		}
	})

	t.Run("a malformed line is an error, not a miss", func(t *testing.T) {
		root := write(t, "{not json}\n")
		if _, err := Resolve(root, "mrv-1"); err == nil {
			t.Fatal("malformed JSONL must surface, not read as no-match")
		}
	})

	t.Run("a scan failure is an error, not a miss", func(t *testing.T) {
		// One line past bufio.Scanner's token limit: Scan stops and reports
		// ErrTooLong. Reading that as "no such issue" would silently downgrade
		// the target to advisory.
		root := write(t, `{"id":"`+strings.Repeat("x", 128<<10)+`"}`)
		err := func() error { _, e := Resolve(root, "mrv-1"); return e }()
		if err == nil {
			t.Fatal("an oversized line must surface as an error")
		}
	})

	t.Run("an unopenable ledger is an error, not a miss", func(t *testing.T) {
		// .beads is a regular file, so opening .beads/issues.jsonl fails with
		// ENOTDIR — an error that is not os.IsNotExist.
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, ".beads"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Resolve(root, "mrv-1"); err == nil {
			t.Fatal("ENOTDIR on the ledger must surface")
		}
	})
}

func TestResolveMarkdown(t *testing.T) {
	root := t.TempDir()
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("a missing file is reported by target, not by resolved path", func(t *testing.T) {
		if _, err := Resolve(real, "docs/missing.md"); err == nil ||
			!strings.Contains(err.Error(), "task path not found: docs/missing.md") {
			t.Fatalf("%v", err)
		}
	})

	t.Run("a directory is refused", func(t *testing.T) {
		if err := os.MkdirAll(filepath.Join(real, "docs"), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := Resolve(real, "docs/"); err == nil ||
			!strings.Contains(err.Error(), "not a regular file") {
			t.Fatalf("%v", err)
		}
	})

	t.Run("an unreadable file surfaces the read error", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(real, "task.md"), []byte("# T\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		defer swap(&readFile, func(string) ([]byte, error) { return nil, errors.New("read blew up") })()
		if _, err := Resolve(real, "task.md"); err == nil || !strings.Contains(err.Error(), "read blew up") {
			t.Fatalf("%v", err)
		}
	})

	t.Run("the title falls back to the relative path", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(real, "untitled.md"), []byte("no heading here\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		src, err := Resolve(real, "untitled.md")
		if err != nil || src.Title != "untitled.md" || src.Kind != "markdown" {
			t.Fatalf("%+v %v", src, err)
		}
	})
}

func TestContainedPathDefensiveBranches(t *testing.T) {
	root := t.TempDir()
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("an unresolvable root", func(t *testing.T) {
		if _, _, err := containedPath(filepath.Join(real, "gone"), "x.md"); err == nil {
			t.Fatal("a root that does not exist must be an error")
		}
	})

	t.Run("a candidate under a regular file", func(t *testing.T) {
		// EvalSymlinks fails with ENOTDIR rather than ENOENT, so this takes the
		// branch that does not treat the target as merely not-yet-created.
		if err := os.WriteFile(filepath.Join(real, "file.md"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, _, err := containedPath(real, "file.md/child.md"); err == nil {
			t.Fatal("ENOTDIR under a regular file must be an error")
		}
	})

	t.Run("the working directory cannot be resolved", func(t *testing.T) {
		defer swap(&absPath, func(string) (string, error) { return "", errors.New("no cwd") })()
		if _, _, err := containedPath("relative", "x.md"); err == nil || !strings.Contains(err.Error(), "no cwd") {
			t.Fatalf("%v", err)
		}
	})

	t.Run("the candidate cannot be made absolute", func(t *testing.T) {
		calls := 0
		defer swap(&absPath, func(p string) (string, error) {
			calls++
			if calls == 1 {
				return filepath.Abs(p)
			}
			return "", errors.New("candidate abs failed")
		})()
		if _, _, err := containedPath(real, "x.md"); err == nil ||
			!strings.Contains(err.Error(), "candidate abs failed") {
			t.Fatalf("%v", err)
		}
	})

	t.Run("relative path fails for an existing target", func(t *testing.T) {
		defer swap(&relPath, func(string, string) (string, error) { return "", errors.New("rel failed") })()
		if _, _, err := containedPath(real, "file.md"); err == nil || !strings.Contains(err.Error(), "rel failed") {
			t.Fatalf("%v", err)
		}
	})

	t.Run("relative path fails for a not-yet-created target", func(t *testing.T) {
		defer swap(&relPath, func(string, string) (string, error) { return "", errors.New("rel failed") })()
		if _, _, err := containedPath(real, "not-created-yet.md"); err == nil ||
			!strings.Contains(err.Error(), "rel failed") {
			t.Fatalf("%v", err)
		}
	})
}

func TestFirstNonEmptyAndStringField(t *testing.T) {
	if got := firstNonEmpty("", "  ", ""); got != "" {
		t.Fatalf("all-blank must yield empty, got %q", got)
	}
	if got := stringField(map[string]any{"n": 42}, "n"); got != "" {
		t.Fatalf("a non-string field must yield empty, got %q", got)
	}
	if got := stringField(map[string]any{}, "absent"); got != "" {
		t.Fatalf("an absent field must yield empty, got %q", got)
	}
}

// swap installs a replacement seam and returns the restore func.
func swap[T any](target *T, replacement T) func() {
	original := *target
	*target = replacement
	return func() { *target = original }
}
