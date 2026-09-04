package gitcontext

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// installFakeGit replaces the package git seam for the duration of a test.
func installFakeGit(t *testing.T, fn func(root string, args ...string) (string, error)) {
	t.Helper()
	real := git
	t.Cleanup(func() { git = real })
	git = fn
}

// installFakeAbs replaces the filepathAbs seam for the duration of a test.
func installFakeAbs(t *testing.T, fn func(string) (string, error)) {
	t.Helper()
	real := filepathAbs
	t.Cleanup(func() { filepathAbs = real })
	filepathAbs = fn
}

// argsHave reports whether the git args contain a token (exact match).
func argsHave(args []string, token string) bool {
	for _, a := range args {
		if a == token {
			return true
		}
	}
	return false
}

// --- pure helpers ---------------------------------------------------------

func TestMatchesExcludeAndAny(t *testing.T) {
	cases := []struct {
		file, exclude string
		want          bool
	}{
		{"a/b.go", ".", false},        // "." exclude never matches
		{"a/b.go", "", false},         // empty exclude never matches
		{"docs/x", "docs/**", true},   // /** directory glob
		{"docs", "docs/**", true},     // the dir itself
		{"other/x", "docs/**", false}, // outside the glob
		{"pkg/f.go", "pkg", true},     // bare dir exclude is recursive
		{"pkg", "pkg", true},          // exact
		{"pkgx/f", "pkg", false},      // sibling sharing a name prefix is NOT matched
	}
	for _, c := range cases {
		if got := matchesExclude(c.file, c.exclude); got != c.want {
			t.Fatalf("matchesExclude(%q,%q)=%v want %v", c.file, c.exclude, got, c.want)
		}
	}
	if !matchesAnyExclude("pkg/f.go", []string{"docs/**", "pkg"}) {
		t.Fatalf("matchesAnyExclude should match via the second pattern")
	}
	if matchesAnyExclude("src/f.go", []string{"docs/**", "pkg"}) {
		t.Fatalf("matchesAnyExclude should not match an unrelated path")
	}
}

func TestValidateRef(t *testing.T) {
	for _, bad := range []string{"", "  ", "-oops", "a..b", "bad space", "tilde~ok/but has space"} {
		if err := validateRef(bad); err == nil {
			t.Fatalf("validateRef(%q) should be rejected", bad)
		}
	}
	for _, ok := range []string{"main", "origin/main", "HEAD~1", "v1.2.3"} {
		if err := validateRef(ok); err != nil {
			t.Fatalf("validateRef(%q) should pass: %v", ok, err)
		}
	}
}

func TestHasPrefixFlag(t *testing.T) {
	if !hasPrefixFlag([]string{"--src-prefix=a/"}, "--src-prefix") {
		t.Fatalf("should match a flag=value form")
	}
	if !hasPrefixFlag([]string{"--no-prefix"}, "--no-prefix") {
		t.Fatalf("should match a bare flag")
	}
	if hasPrefixFlag([]string{"--other"}, "--src-prefix") {
		t.Fatalf("should not match an unrelated flag")
	}
}

func TestTruncate(t *testing.T) {
	if got, cut, _ := truncate("abc", 10); got != "abc" || cut {
		t.Fatalf("under-limit must pass through untouched")
	}
	if got, cut, _ := truncate("abcdef", 0); got != "" || !cut {
		t.Fatalf("a zero limit over a non-empty value truncates to empty, got %q cut=%v", got, cut)
	}
	// A cut landing mid-rune trims back to a valid boundary.
	s := "é" + strings.Repeat("x", 10) // 'é' is 2 bytes
	got, cut, _ := truncate(s, 1)      // slicing at 1 splits the rune
	if !cut {
		t.Fatalf("over-limit must report truncation")
	}
	if !isValidUTF8(got) {
		t.Fatalf("truncation must leave valid UTF-8, got %q", got)
	}
}

func isValidUTF8(s string) bool {
	for _, r := range s {
		if r == '�' && len(s) > 0 {
			// range yields RuneError for invalid bytes; a real replacement char would be multi-byte.
			return false
		}
	}
	return true
}

func TestSafeJoinRejectsEscapes(t *testing.T) {
	root := t.TempDir()
	rootAbs, _ := filepath.Abs(root)
	for _, rel := range []string{".", "..", ".." + string(filepath.Separator) + "x", string(filepath.Separator) + "etc"} {
		if _, err := safeJoin(rootAbs, rel); err == nil {
			t.Fatalf("safeJoin(%q) should be rejected", rel)
		}
	}
	if _, err := safeJoin(rootAbs, "sub/file.txt"); err != nil {
		t.Fatalf("a normal in-root path should join: %v", err)
	}
}

// safeJoin's filepath.Abs guards are unreachable with already-absolute inputs; the seam forces both
// the error branch and the "resolved outside root" branch.
func TestSafeJoinAbsSeamBranches(t *testing.T) {
	root := t.TempDir()
	rootAbs, _ := filepath.Abs(root)

	installFakeAbs(t, func(string) (string, error) { return "", errors.New("abs boom") })
	if _, err := safeJoin(rootAbs, "sub/file.txt"); err == nil {
		t.Fatalf("an Abs failure must surface")
	}

	installFakeAbs(t, func(string) (string, error) { return string(filepath.Separator) + "elsewhere/x", nil })
	if _, err := safeJoin(rootAbs, "sub/file.txt"); err == nil {
		t.Fatalf("a path resolving outside root must be rejected")
	}
}

// --- git()/tryGit low-level ------------------------------------------------

// The real git wrapper surfaces a start failure (non-existent dir, empty stderr) via err.Error().
func TestGitRealSurfacesStartFailure(t *testing.T) {
	if _, err := gitReal(filepath.Join(t.TempDir(), "nope"), "status"); err == nil {
		t.Fatalf("git in a non-existent dir must error")
	}
}

// tryGit swallows an error into an empty string.
func TestTryGitSwallowsError(t *testing.T) {
	if out := tryGit(filepath.Join(t.TempDir(), "nope"), "status"); out != "" {
		t.Fatalf("tryGit must return empty on error, got %q", out)
	}
}

// --- realGit (branchfiles) -------------------------------------------------

func TestRealGitReportsStderr(t *testing.T) {
	repo := initRepo(t)
	// Verifying a non-existent ref makes git exit non-zero with a stderr message.
	if _, err := realGit(repo, nil, "rev-parse", "--verify", "does-not-exist-ref"); err == nil {
		t.Fatalf("a failing git command must error")
	}
}

func TestRealGitStartFailureHasNoStderr(t *testing.T) {
	// A non-existent working directory makes cmd.Output fail before git writes stderr.
	if _, err := realGit(filepath.Join(t.TempDir(), "nope"), nil, "status"); err == nil {
		t.Fatalf("git in a non-existent dir must error")
	}
}

// --- measureBranchFiles error paths (via fake RunGit) ----------------------

func TestMeasureBranchFilesSurfacesErrors(t *testing.T) {
	boom := errors.New("run boom")
	// branchPaths fails.
	failPaths := func(root string, env []string, args ...string) ([]byte, error) { return nil, boom }
	if _, err := measureBranchFiles("r", "base", nil, failPaths); err == nil {
		t.Fatalf("a branchPaths failure must surface")
	}
	// branchPaths succeeds (one path), the per-file measure fails.
	failMeasure := func(root string, env []string, args ...string) ([]byte, error) {
		if argsHave(args, "--name-only") {
			return []byte("a.go\x00"), nil
		}
		return nil, boom
	}
	if _, err := measureBranchFiles("r", "base", nil, failMeasure); err == nil {
		t.Fatalf("a per-file measure failure must surface")
	}
}

// --- readUntrackedExcerpts branches (direct) -------------------------------

func TestReadUntrackedExcerptsBranches(t *testing.T) {
	root := t.TempDir()
	// A regular file that is large enough to be truncated mid-rune, a missing file (stat continue),
	// and a normal small file.
	// Place a 2-byte rune straddling the maxUntrackedFileBytes cut point so the truncation lands
	// mid-rune and the UTF-8 trim-back loop runs.
	big := strings.Repeat("a", maxUntrackedFileBytes-1) + "é" + strings.Repeat("b", 100)
	if err := os.WriteFile(filepath.Join(root, "big.txt"), []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "small.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	excerpts, omitted, truncated, bytes, err := readUntrackedExcerpts(root, []string{"big.txt", "missing.txt", "small.txt"})
	if err != nil {
		t.Fatalf("readUntrackedExcerpts: %v", err)
	}
	if truncated != 1 {
		t.Fatalf("the big file should be counted truncated, got %d", truncated)
	}
	if omitted != 0 {
		t.Fatalf("three files under the cap omit none, got %d", omitted)
	}
	if bytes == 0 || excerpts == "" {
		t.Fatalf("expected excerpts and a byte total: bytes=%d", bytes)
	}
}

// A safeJoin failure (an escaping path) propagates out of readUntrackedExcerpts.
func TestReadUntrackedExcerptsSurfacesSafeJoinError(t *testing.T) {
	if _, _, _, _, err := readUntrackedExcerpts(t.TempDir(), []string{".." + string(filepath.Separator) + "escape"}); err == nil {
		t.Fatalf("an escaping untracked path must surface an error")
	}
}

// The root filepath.Abs failure surfaces (seam).
func TestReadUntrackedExcerptsSurfacesAbsError(t *testing.T) {
	installFakeAbs(t, func(string) (string, error) { return "", errors.New("abs boom") })
	if _, _, _, _, err := readUntrackedExcerpts(t.TempDir(), []string{"a.txt"}); err == nil {
		t.Fatalf("a root Abs failure must surface")
	}
}

// An unreadable regular file surfaces a read error.
func TestReadUntrackedExcerptsSurfacesReadError(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("unreadable-file case needs a non-root POSIX host")
	}
	root := t.TempDir()
	secret := filepath.Join(root, "secret.txt")
	if err := os.WriteFile(secret, []byte("x"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(secret, 0o644) })
	if _, _, _, _, err := readUntrackedExcerpts(root, []string{"secret.txt"}); err == nil {
		t.Fatalf("an unreadable untracked file must surface a read error")
	}
}

// initRepo makes a real one-commit git repository and returns its root.
func initRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGitCmd(t, root, "init", "-q")
	runGitCmd(t, root, "config", "user.email", "t@example.com")
	runGitCmd(t, root, "config", "user.name", "T")
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitCmd(t, root, "add", ".")
	runGitCmd(t, root, "commit", "-qm", "init")
	return root
}

func runGitCmd(t *testing.T, root string, args ...string) {
	t.Helper()
	if _, err := gitReal(root, args...); err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
}

var _ = fmt.Sprintf
