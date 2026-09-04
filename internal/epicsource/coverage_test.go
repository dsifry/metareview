package epicsource

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Resolve rejects a blank target before touching the filesystem.
func TestResolveRejectsEmptyTarget(t *testing.T) {
	if _, err := Resolve(t.TempDir(), "   "); err == nil {
		t.Fatalf("a blank epic target must be rejected")
	}
}

// A rich beads fixture drives the resolveBeads/readIssues/stringField/stringList/body/firstNonEmpty
// branches that the happy-path test doesn't: a non-string title falling back to summary, a
// single-string children field, an empty and a non-array dependency field (both yielding nothing),
// a status line in the body, and children discovered via every relationship key.
func TestResolveBeadsExercisesFieldCoercions(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, ".beads", "issues.jsonl"), strings.Join([]string{
		`{"id":"epic-9","title":5,"summary":"Coercions epic","children":"task-a","depends_on":"","dependencies":7,"status":"open","description":"Main body"}`,
		"", // a blank line in the middle of the ledger must be skipped
		`{"id":"task-b","parent":"epic-9"}`,
		`{"id":"task-c","epic":"epic-9"}`,
		`{"id":"task-d","dependencies":["epic-9"]}`,
		`{"id":"task-e","depends_on":["epic-9"]}`,
	}, "\n"))

	epic, err := Resolve(root, "epic-9")
	if err != nil {
		t.Fatalf("resolve beads: %v", err)
	}
	if epic.Kind != "beads" {
		t.Fatalf("expected beads kind: %+v", epic)
	}
	// title was non-string → firstNonEmpty falls through to summary.
	if epic.Title != "Coercions epic" {
		t.Fatalf("title should fall back to summary, got %q", epic.Title)
	}
	// status is rendered with its label; description is included plain.
	if !strings.Contains(epic.Body, "Status: open") || !strings.Contains(epic.Body, "Main body") {
		t.Fatalf("body missing status/description: %q", epic.Body)
	}
	// children: single-string "task-a" plus the four issues linked by parent/epic/dependencies/depends_on.
	// The empty depends_on and non-array dependencies on the epic contribute nothing.
	got := strings.Join(epic.ChildIDs, ",")
	want := "task-a,task-b,task-c,task-d,task-e"
	if got != want {
		t.Fatalf("children mismatch: got %q want %q", got, want)
	}
}

// A non-existent .beads ledger is not an error — Resolve falls through to an advisory target.
// (readIssues returns nil,nil on os.IsNotExist.)
func TestResolveBeadsMissingLedgerFallsThroughToAdvisory(t *testing.T) {
	epic, err := Resolve(t.TempDir(), "epic-none")
	if err != nil {
		t.Fatalf("missing ledger should not error: %v", err)
	}
	if epic.Kind != "advisory" {
		t.Fatalf("expected advisory fallback: %+v", epic)
	}
}

// A malformed ledger line is a hard error surfaced through Resolve.
func TestResolveBeadsRejectsMalformedLedger(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, ".beads", "issues.jsonl"), "{not valid json\n")
	if _, err := Resolve(root, "epic-x"); err == nil {
		t.Fatalf("a malformed ledger line must surface an error")
	}
}

// A ledger path whose parent component is a regular file (not a directory) makes os.Open fail with
// a non-IsNotExist error, which readIssues surfaces rather than swallowing.
func TestResolveBeadsSurfacesNonNotExistOpenError(t *testing.T) {
	root := t.TempDir()
	// Write .beads as a FILE, so opening .beads/issues.jsonl fails with ENOTDIR.
	mustWrite(t, filepath.Join(root, ".beads"), "not a directory\n")
	if _, err := Resolve(root, "epic-x"); err == nil {
		t.Fatalf("a non-IsNotExist open error must be surfaced")
	}
}

// A path-like target that resolves to a directory is rejected as not a regular file.
func TestResolveMarkdownRejectsNonRegularFile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub", "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(root, "sub/dir"); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("a directory target must be rejected as non-regular, got %v", err)
	}
}

// A markdown file with no level-1 heading falls back to its relative path as the title.
func TestResolveMarkdownTitleFallsBackToPath(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "plain.md"), "no heading here\njust text\n")
	epic, err := Resolve(root, "docs/plain.md")
	if err != nil {
		t.Fatalf("resolve markdown: %v", err)
	}
	if epic.Title != "docs/plain.md" {
		t.Fatalf("title should fall back to the rel path, got %q", epic.Title)
	}
}

// A path-like target that does not exist fails containment (EvalSymlinks can't resolve a missing
// path) rather than being silently treated as advisory.
func TestResolveMarkdownMissingPathIsRejected(t *testing.T) {
	if _, err := Resolve(t.TempDir(), "docs/missing.md"); err == nil {
		t.Fatalf("a non-existent markdown target must be rejected")
	}
}

// A symlink that stays inside the root by string but resolves outside it is rejected: the
// EvalSymlinks-based containment check is the real defense against symlink escape.
func TestResolveMarkdownRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink escape test is POSIX-oriented")
	}
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "evil.md"), []byte("# Evil\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// root/sub -> outside (a symlink whose string path is inside root).
	if err := os.Symlink(outside, filepath.Join(root, "sub")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	if _, err := Resolve(root, "sub/evil.md"); err == nil || !strings.Contains(err.Error(), "outside repository root") {
		t.Fatalf("a symlink escaping the root must be rejected, got %v", err)
	}
}

// A markdown file readable by stat but not by ReadFile (mode 0000) surfaces the read error.
func TestResolveMarkdownSurfacesReadError(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permission-denied read is not reproducible as root or on Windows")
	}
	root := t.TempDir()
	path := filepath.Join(root, "docs", "secret.md")
	mustWrite(t, path, "# Secret\n")
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
	if _, err := Resolve(root, "docs/secret.md"); err == nil {
		t.Fatalf("an unreadable file must surface a read error")
	}
}

// --- seam-driven tests for the otherwise-unreachable defensive branches -----

// osStat's error in resolveMarkdown sits behind a successful EvalSymlinks, so a real missing path
// never reaches it. The seam forces a stat failure on an existing, in-root file.
func TestResolveMarkdownSurfacesStatError(t *testing.T) {
	real := osStat
	t.Cleanup(func() { osStat = real })
	osStat = func(string) (os.FileInfo, error) { return nil, errors.New("stat boom") }

	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "epic.md"), "# Epic\n")
	if _, err := Resolve(root, "docs/epic.md"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("a stat failure must surface as not-found, got %v", err)
	}
}

// filepath.Abs only fails when os.Getwd fails on a relative path; the seam forces both call sites
// (root, then candidate) to fail.
func TestContainedPathSurfacesAbsErrors(t *testing.T) {
	realAbs := filepathAbs
	t.Cleanup(func() { filepathAbs = realAbs })
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "epic.md"), "# Epic\n")

	// First Abs call (root) fails.
	filepathAbs = func(string) (string, error) { return "", errors.New("abs root boom") }
	if _, err := Resolve(root, "docs/epic.md"); err == nil {
		t.Fatalf("a failure resolving the root abs path must surface")
	}

	// Second Abs call (candidate) fails: succeed once, then fail.
	calls := 0
	filepathAbs = func(p string) (string, error) {
		calls++
		if calls == 1 {
			return realAbs(p)
		}
		return "", errors.New("abs candidate boom")
	}
	if _, err := Resolve(root, "docs/epic.md"); err == nil {
		t.Fatalf("a failure resolving the candidate abs path must surface")
	}
}

// filepath.Rel only fails on operands the callers never produce; the seam forces it.
func TestContainedPathSurfacesRelError(t *testing.T) {
	real := filepathRel
	t.Cleanup(func() { filepathRel = real })
	filepathRel = func(string, string) (string, error) { return "", errors.New("rel boom") }

	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "epic.md"), "# Epic\n")
	if _, err := Resolve(root, "docs/epic.md"); err == nil {
		t.Fatalf("a Rel failure must surface")
	}
}

// evalSymlinks(rootAbs) can't fail for an existing root, so the seam forces that first-call branch.
func TestContainedPathSurfacesRootEvalError(t *testing.T) {
	real := evalSymlinks
	t.Cleanup(func() { evalSymlinks = real })
	evalSymlinks = func(string) (string, error) { return "", errors.New("eval root boom") }

	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "epic.md"), "# Epic\n")
	if _, err := Resolve(root, "docs/epic.md"); err == nil {
		t.Fatalf("a root symlink-eval failure must surface")
	}
}

// firstNonEmpty returns "" when every candidate is blank (unreachable via resolveBeads, where the
// id is always non-empty, so exercised directly).
func TestFirstNonEmptyAllBlank(t *testing.T) {
	if got := firstNonEmpty("", "   ", "\t"); got != "" {
		t.Fatalf("all-blank inputs should yield empty, got %q", got)
	}
}
