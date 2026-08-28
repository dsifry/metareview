package tasksource

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// containedPath is the guard that keeps a task target inside the repository. It
// is the one function here where a wrong answer is a security problem rather
// than a wrong review, so each way out of it is pinned.
func TestContainedPathKeepsTargetsInsideTheRoot(t *testing.T) {
	root := t.TempDir()
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realRoot, "task.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("relative target inside the root", func(t *testing.T) {
		abs, rel, err := containedPath(realRoot, "task.md")
		if err != nil || rel != "task.md" || abs != filepath.Join(realRoot, "task.md") {
			t.Fatalf("abs=%q rel=%q err=%v", abs, rel, err)
		}
	})

	t.Run("absolute target inside the root", func(t *testing.T) {
		_, rel, err := containedPath(realRoot, filepath.Join(realRoot, "task.md"))
		if err != nil || rel != "task.md" {
			t.Fatalf("rel=%q err=%v", rel, err)
		}
	})

	t.Run("target that does not exist yet", func(t *testing.T) {
		// Resolvable lexically but absent on disk: allowed, and reported relative.
		_, rel, err := containedPath(realRoot, "docs/new-task.md")
		if err != nil || rel != "docs/new-task.md" {
			t.Fatalf("rel=%q err=%v", rel, err)
		}
	})

	t.Run("traversal above the root is refused", func(t *testing.T) {
		if _, _, err := containedPath(realRoot, "../escape.md"); err == nil ||
			!strings.Contains(err.Error(), "outside repository root") {
			t.Fatalf("traversal was not refused: %v", err)
		}
	})

	t.Run("absolute target elsewhere is refused", func(t *testing.T) {
		if _, _, err := containedPath(realRoot, t.TempDir()); err == nil ||
			!strings.Contains(err.Error(), "outside repository root") {
			t.Fatalf("outside absolute path was not refused: %v", err)
		}
	})

	t.Run("a symlink pointing out of the root is refused", func(t *testing.T) {
		// The lexical check passes here — the link itself is inside the root —
		// so only the post-EvalSymlinks check can catch this one.
		outside := t.TempDir()
		secret := filepath.Join(outside, "secret.md")
		if err := os.WriteFile(secret, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(realRoot, "link.md")
		if err := os.Symlink(secret, link); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		if _, _, err := containedPath(realRoot, "link.md"); err == nil ||
			!strings.Contains(err.Error(), "outside repository root") {
			t.Fatalf("a symlink escaping the root was accepted: %v", err)
		}
	})

	t.Run("the root itself is contained", func(t *testing.T) {
		if _, _, err := containedPath(realRoot, "."); err != nil {
			t.Fatalf("the root must be contained in itself: %v", err)
		}
	})
}
