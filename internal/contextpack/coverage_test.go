package contextpack

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dsifry/metareview/internal/state"
)

// writeFile is a tiny fixture helper: it creates parent dirs and writes content.
func writeFileAt(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// initFixtureRepo makes root a git repo with one commit, its config isolated from the machine's
// global/system git config so the fixture is deterministic on any runner (no inherited gpgsign,
// hooksPath, or sha256 object format).
func initFixtureRepo(t *testing.T, root string) {
	t.Helper()
	env := append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"commit", "--allow-empty", "-m", "seed"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

// gitValue returns "unavailable" outside a repo (covered by other tests) and the trimmed command
// output inside one — the success branch.
func TestGitValueReturnsTrimmedOutputInRepo(t *testing.T) {
	root := t.TempDir()
	initFixtureRepo(t, root)
	branch := gitValue(root, "rev-parse", "--abbrev-ref", "HEAD")
	if branch == "unavailable" || branch == "" {
		t.Fatalf("expected a real branch name, got %q", branch)
	}
	if strings.ContainsAny(branch, "\n\r") {
		t.Fatalf("gitValue did not trim output: %q", branch)
	}
}

func TestGitValueUnavailableOutsideRepo(t *testing.T) {
	if got := gitValue(t.TempDir(), "rev-parse", "HEAD"); got != "unavailable" {
		t.Fatalf("expected \"unavailable\" outside a repo, got %q", got)
	}
}

// assertInsideFile rejects a target that resolves outside the repository root.
func TestAssertInsideFileRejectsOutsideRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "repo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFileAt(t, filepath.Join(parent, "outside.md"), "x")
	_, err := assertInsideFile(root, filepath.Join("..", "outside.md"))
	if err == nil || !strings.Contains(err.Error(), "outside repository root") {
		t.Fatalf("expected an outside-root error, got %v", err)
	}
}

// assertInsideFile rejects a target that resolves to a directory rather than a regular file.
func TestAssertInsideFileRejectsNonRegularFile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "adir"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := assertInsideFile(root, "adir")
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("expected a not-a-regular-file error, got %v", err)
	}
}

// assertInsideFile surfaces EvalSymlinks failure for a root that does not exist.
func TestAssertInsideFileMissingRoot(t *testing.T) {
	if _, err := assertInsideFile(filepath.Join(t.TempDir(), "nope"), "A.md"); err == nil {
		t.Fatal("expected an error for a nonexistent root")
	}
}

// assertInsideFile surfaces EvalSymlinks failure for a target that does not exist.
func TestAssertInsideFileMissingTarget(t *testing.T) {
	_, err := assertInsideFile(t.TempDir(), "missing.md")
	if err == nil {
		t.Fatal("expected an error for a nonexistent target")
	}
}

// assertInsideFile surfaces filepath.Abs failure on the root path, via the seam.
func TestAssertInsideFileAbsRootError(t *testing.T) {
	orig := filepathAbs
	t.Cleanup(func() { filepathAbs = orig })
	sentinel := errors.New("abs root failed")
	calls := 0
	filepathAbs = func(p string) (string, error) {
		calls++
		if calls == 1 {
			return "", sentinel
		}
		return orig(p)
	}
	_, err := assertInsideFile(t.TempDir(), "A.md")
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected the abs-root error, got %v", err)
	}
}

// assertInsideFile surfaces filepath.Abs failure on the target path (the second Abs call), via the
// seam. Keying on call order proves it is the target Abs, not the root Abs, whose failure surfaces.
func TestAssertInsideFileAbsTargetError(t *testing.T) {
	orig := filepathAbs
	t.Cleanup(func() { filepathAbs = orig })
	sentinel := errors.New("abs target failed")
	calls := 0
	filepathAbs = func(p string) (string, error) {
		calls++
		if calls == 2 {
			return "", sentinel
		}
		return orig(p)
	}
	_, err := assertInsideFile(t.TempDir(), "A.md")
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected the abs-target error, got %v", err)
	}
}

// assertInsideFile surfaces the os.Stat failure that EvalSymlinks cannot itself produce — the
// defensive not-found branch after symlink resolution — via the seam.
func TestAssertInsideFileStatError(t *testing.T) {
	root := t.TempDir()
	writeFileAt(t, filepath.Join(root, "A.md"), "# a\n")
	orig := osStat
	t.Cleanup(func() { osStat = orig })
	sentinel := errors.New("stat failed")
	osStat = func(string) (os.FileInfo, error) { return nil, sentinel }
	_, err := assertInsideFile(root, "A.md")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected a not-found error from the stat failure, got %v", err)
	}
}

// readLimited returns "" when the file cannot be read.
func TestReadLimitedMissingFile(t *testing.T) {
	if got := readLimited(filepath.Join(t.TempDir(), "nope"), 100); got != "" {
		t.Fatalf("expected empty string for a missing file, got %q", got)
	}
}

// readLimited truncates content longer than the limit and returns the whole file otherwise.
func TestReadLimitedTruncates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.txt")
	writeFileAt(t, path, "0123456789")
	if got := readLimited(path, 4); got != "0123" {
		t.Fatalf("expected truncation to 4 bytes, got %q", got)
	}
	if got := readLimited(path, 100); got != "0123456789" {
		t.Fatalf("expected whole file under the limit, got %q", got)
	}
}

// knowledgeFacts reads up to five non-blank lines per .jsonl file under .beads/knowledge, sorted by
// name, skipping directories and non-.jsonl entries and blank lines, and capping at five lines.
func TestKnowledgeFactsReadsBeadsKnowledge(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".beads", "knowledge")
	// a.jsonl: a blank first line (skipped) then content lines. Only the first FIVE lines are
	// examined (the i>=5 break), so of "", l1..l6 the examined window is "",l1,l2,l3,l4 → l1..l4
	// survive; l5 (index 5) is past the cap and l6 further still.
	writeFileAt(t, filepath.Join(dir, "a.jsonl"), "\nl1\nl2\nl3\nl4\nl5\nl6\n")
	// b.jsonl sorts after a.jsonl and contributes one line.
	writeFileAt(t, filepath.Join(dir, "b.jsonl"), "only\n")
	// A non-.jsonl file and a subdirectory are both skipped.
	writeFileAt(t, filepath.Join(dir, "notes.txt"), "ignore\n")
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	facts := knowledgeFacts(root)
	joined := strings.Join(facts, "\n")
	var aCount, bCount int
	for _, f := range facts {
		if strings.Contains(f, "a.jsonl:") {
			aCount++
		}
		if strings.Contains(f, "b.jsonl:") {
			bCount++
		}
		if strings.Contains(f, "notes.txt") {
			t.Errorf("non-.jsonl file must be skipped: %s", f)
		}
	}
	if aCount != 4 {
		t.Errorf("expected 4 facts from a.jsonl (blank skipped, only first 5 lines examined), got %d: %v", aCount, facts)
	}
	// The blank line was skipped (l1 present) and the cap held (l5/l6 past the 5-line window).
	if !strings.Contains(joined, "a.jsonl: l1") || !strings.Contains(joined, "a.jsonl: l4") {
		t.Errorf("expected l1..l4 from a.jsonl: %v", facts)
	}
	if strings.Contains(joined, "a.jsonl: l5") || strings.Contains(joined, "a.jsonl: l6") {
		t.Errorf("l5/l6 are past the 5-line cap and must not appear: %v", facts)
	}
	if bCount != 1 {
		t.Errorf("expected 1 fact from b.jsonl, got %d: %v", bCount, facts)
	}
	// Sorted by name: the first a.jsonl fact precedes the b.jsonl fact.
	firstA, firstB := -1, -1
	for i, f := range facts {
		if firstA < 0 && strings.Contains(f, "a.jsonl:") {
			firstA = i
		}
		if firstB < 0 && strings.Contains(f, "b.jsonl:") {
			firstB = i
		}
	}
	if firstA > firstB {
		t.Errorf("facts not sorted by filename: %v", facts)
	}
}

// knowledgeFacts returns nil when there is no .beads/knowledge directory.
func TestKnowledgeFactsAbsentDir(t *testing.T) {
	if facts := knowledgeFacts(t.TempDir()); facts != nil {
		t.Fatalf("expected nil with no .beads/knowledge, got %v", facts)
	}
}

// Build folds a service inventory and Beads knowledge facts into the context pack when both are
// present, and reports the git branch/head from the fixture repo.
func TestBuildIncludesInventoryAndFacts(t *testing.T) {
	root := t.TempDir()
	initFixtureRepo(t, root)
	writeFileAt(t, filepath.Join(root, "A.md"), "# artifact\n\nbody\n")
	writeFileAt(t, filepath.Join(root, "docs", "SERVICE_INVENTORY.md"), "svc-one\nsvc-two\n")
	writeFileAt(t, filepath.Join(root, ".beads", "knowledge", "k.jsonl"), "fact-one\n")

	res, err := Build(root, "A.md", time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(res.ContextRel)))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "svc-one") {
		t.Errorf("context pack missing service inventory content:\n%s", text)
	}
	if !strings.Contains(text, "fact-one") {
		t.Errorf("context pack missing knowledge fact:\n%s", text)
	}
	// gitValue's success path: the head short SHA is not "unavailable".
	if strings.Contains(text, "Git head: `unavailable`") {
		t.Errorf("expected a real git head in a fixture repo:\n%s", text)
	}
}

// Build surfaces the assertInsideFile error for a missing target.
func TestBuildMissingTarget(t *testing.T) {
	if _, err := Build(t.TempDir(), "missing.md", time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("expected an error for a missing target")
	}
}

// Build surfaces the MkdirAll error when the output directory cannot be created (a plain file sits
// at docs, so docs/metareview/context cannot be made).
func TestBuildMkdirAllError(t *testing.T) {
	root := t.TempDir()
	writeFileAt(t, filepath.Join(root, "A.md"), "# a\n")
	writeFileAt(t, filepath.Join(root, "docs"), "x") // docs is a file
	if _, err := Build(root, "A.md", time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("expected an error when docs is a file")
	}
}

// Build surfaces the WriteFile error when the output path is a directory.
func TestBuildWriteFileError(t *testing.T) {
	root := t.TempDir()
	writeFileAt(t, filepath.Join(root, "A.md"), "# a\n")
	at := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	// Pre-create the exact output path as a directory so os.WriteFile fails.
	runID := state.RunID("artifact", "A.md", at)
	outputPath := filepath.Join(root, "docs", "metareview", "context", runID+"-context.md")
	if err := os.MkdirAll(outputPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(root, "A.md", at); err == nil {
		t.Fatal("expected an error when the output path is a directory")
	}
}
