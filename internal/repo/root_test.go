package repo

import (
	"os"
	"path/filepath"
	"testing"
)

// Every command resolved paths against the process's working directory. That is right when a
// human runs metareview from the top of a checkout and wrong in the one case enforcement depends
// on: a Stop hook inherits whatever directory the session happens to be in, `status` then found
// no review logs, reported blocked:false, and the hook exited 0. The gate was bypassed by the
// ordinary act of working in a subdirectory, and bypassed silently.
func TestRootWalksUpFromAnySubdirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(root, "internal", "mutation", "testdata")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, start := range []string{root, filepath.Join(root, "internal"), deep} {
		got, err := Root(start)
		if err != nil {
			t.Fatalf("Root(%q): %v", start, err)
		}
		if got != resolve(t, root) {
			t.Errorf("Root(%q) = %q, want %q", start, got, root)
		}
	}
}

// A repository metareview has already run in names itself, and that marker is honoured even
// where there is no .git — a plain directory of review logs, or a checkout whose .git is absent.
func TestRootRecognisesAMetareviewDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".metareview"), 0o755); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := Root(sub)
	if err != nil {
		t.Fatalf("Root: %v", err)
	}
	if got != resolve(t, root) {
		t.Errorf("Root = %q, want %q", got, root)
	}
}

// A checkout carrying committed review logs is a repository even with no .git and no transient
// state: docs/metareview is the directory reviewlog.Discover reads, so if it is there, the gate
// has something to answer with and must find it.
func TestRootRecognisesCommittedReviewLogs(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs", "metareview", "reviews"), 0o755); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "internal", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := Root(sub); err != nil || got != resolve(t, root) {
		t.Errorf("Root = %q, %v; want %q", got, err, root)
	}
}

// A git worktree and a submodule both have .git as a FILE, not a directory. Matching only on a
// directory would leave the gate bypassed in exactly the checkouts used to test it in isolation.
func TestRootAcceptsAGitFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: /elsewhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "pkg")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := Root(sub); err != nil || got != resolve(t, root) {
		t.Errorf("Root = %q, %v; want %q", got, err, root)
	}
}

// Outside a repository the caller must be told, not handed a plausible-looking directory. RootOr
// is the variant for callers that report "not a repository" themselves.
func TestRootReportsWhenThereIsNoRepository(t *testing.T) {
	bare := t.TempDir()
	if _, err := Root(bare); err == nil {
		t.Fatal("a directory with no repository above it must be an error")
	}
	if got := RootOr(bare); got != bare {
		t.Errorf("RootOr must fall back to its argument, got %q", got)
	}
	// And RootOr still prefers a real root when there is one.
	if err := os.MkdirAll(filepath.Join(bare, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(bare, "x")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := RootOr(sub); got != resolve(t, bare) {
		t.Errorf("RootOr = %q, want %q", got, bare)
	}
}

// resolve mirrors what Root does to its argument, so a temp dir behind a symlink (/var vs
// /private/var on macOS) does not make these tests compare two spellings of the same path.
func resolve(t *testing.T, p string) string {
	t.Helper()
	abs, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}
