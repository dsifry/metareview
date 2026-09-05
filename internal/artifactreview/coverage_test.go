package artifactreview

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dsifry/metareview/internal/contextpack"
)

// newArtifact writes a minimal artifact and its .metareview dir under a fresh temp root, returning
// the root. It is the deterministic fixture the Create tests drive — no ambient git or HOME state.
func newArtifact(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".metareview"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "A.md"), []byte("# artifact\n\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// gitHead's success return (`string(out[:len(out)-1])`) is only reached when `git rev-parse HEAD`
// succeeds, so it needs a real repository. A fixture repo initialised in a temp dir is deterministic
// and CI-reproducible — unlike the machine's ambient git state.
func TestGitHeadReturnsHeadInRealRepo(t *testing.T) {
	root := t.TempDir()
	// Neutralise the machine's global/system git config so the fixture is deterministic on any
	// runner: no inherited commit.gpgsign, hooksPath, or sha256 object format to break or skew it.
	gitEnv := append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_OPTIONAL_LOCKS=0", "GIT_CONFIG_COUNT=2", "GIT_CONFIG_KEY_0=gc.auto", "GIT_CONFIG_VALUE_0=0", "GIT_CONFIG_KEY_1=maintenance.auto", "GIT_CONFIG_VALUE_1=false")
	runGit := func(args ...string) []byte {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = gitEnv
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return out
	}
	runGit("init")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "Test")
	runGit("commit", "--allow-empty", "-m", "seed")

	// Assert against an independent rev-parse rather than a hardcoded hash length: this proves the
	// success path is taken and the trailing newline is stripped, without assuming the object format.
	want := strings.TrimSpace(string(runGit("rev-parse", "HEAD")))
	if got := gitHead(root); got != want {
		t.Fatalf("gitHead(root) = %q, want the repo HEAD %q", got, want)
	}
}

// A non-repository directory drives the error branch: `git rev-parse` fails and gitHead reports
// "unavailable" rather than a partial string.
func TestGitHeadUnavailableOutsideRepo(t *testing.T) {
	if got := gitHead(t.TempDir()); got != "unavailable" {
		t.Fatalf("expected \"unavailable\" outside a repo, got %q", got)
	}
}

// ensureEmpty leaves an existing file untouched (its non-write return-nil branch): the sentinel
// content must survive the call rather than be truncated to empty.
func TestEnsureEmptyLeavesExistingFileUntouched(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "findings.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("PRE-EXISTING\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureEmpty(path); err != nil {
		t.Fatalf("ensureEmpty: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "PRE-EXISTING\n" {
		t.Fatalf("ensureEmpty overwrote an existing file: %q", got)
	}
}

// ensureEmpty creates the file (and parents) when absent, writing an empty file.
func TestEnsureEmptyCreatesAbsentFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "findings.jsonl")
	if err := ensureEmpty(path); err != nil {
		t.Fatalf("ensureEmpty: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected file created: %v", err)
	}
	if info.Size() != 0 {
		t.Fatalf("expected empty file, got %d bytes", info.Size())
	}
}

// ensureEmpty surfaces the MkdirAll error when the parent path cannot be a directory (a plain file
// sits where a directory segment is needed). Real error, no seam.
func TestEnsureEmptyMkdirAllError(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureEmpty(filepath.Join(blocker, "sub", "findings.jsonl")); err == nil {
		t.Fatal("expected an error when a parent segment is a file")
	}
}

// ensureFindingsIndex returns early, leaving an existing index untouched.
func TestEnsureFindingsIndexLeavesExistingUntouched(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "docs", "metareview", "FINDINGS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("EXISTING INDEX\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureFindingsIndex(root); err != nil {
		t.Fatalf("ensureFindingsIndex: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "EXISTING INDEX\n" {
		t.Fatalf("ensureFindingsIndex overwrote an existing index: %q", got)
	}
}

// ensureFindingsIndex creates the seed index when it is absent.
func TestEnsureFindingsIndexCreatesSeed(t *testing.T) {
	root := t.TempDir()
	if err := ensureFindingsIndex(root); err != nil {
		t.Fatalf("ensureFindingsIndex: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(root, "docs", "metareview", "FINDINGS.md"))
	if err != nil {
		t.Fatalf("expected seed index created: %v", err)
	}
	if !strings.Contains(string(got), "No unresolved findings recorded yet.") {
		t.Fatalf("unexpected seed content: %q", got)
	}
}

// ensureFindingsIndex surfaces the MkdirAll error when a parent segment is a plain file.
func TestEnsureFindingsIndexMkdirAllError(t *testing.T) {
	root := t.TempDir()
	// Put a file at docs so MkdirAll(docs/metareview) cannot create the directory.
	if err := os.WriteFile(filepath.Join(root, "docs"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureFindingsIndex(root); err == nil {
		t.Fatal("expected an error when docs is a file")
	}
}

// Create bumps the run timestamp by a nanosecond when a review log already exists at the computed
// path, so two artifact reviews requested at the same instant never collide.
func TestCreateBumpsTimestampOnCollision(t *testing.T) {
	root := newArtifact(t)
	at := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	first, err := Create(root, "A.md", "", at)
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}
	// Same instant: the loop must detect the existing log and advance to a distinct run ID.
	second, err := Create(root, "A.md", "", at)
	if err != nil {
		t.Fatalf("second Create: %v", err)
	}
	if first.RunID == second.RunID {
		t.Fatalf("collision not resolved: both runs share ID %s", first.RunID)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(second.ReviewRel))); err != nil {
		t.Fatalf("second review log not written: %v", err)
	}
}

// Create records a non-empty previous run in both the run record's PreviousRun result field and the
// scaffold's "Previous run:" marker line.
func TestCreateRecordsPreviousRun(t *testing.T) {
	root := newArtifact(t)
	res, err := Create(root, "A.md", "mrv-earlier-run", time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if res.PreviousRun != "mrv-earlier-run" {
		t.Fatalf("PreviousRun = %q, want mrv-earlier-run", res.PreviousRun)
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(res.ReviewRel)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "Previous run: `mrv-earlier-run`") {
		t.Fatalf("scaffold missing previous-run marker:\n%s", body)
	}
}

// Create surfaces the context-pack build error when the target is not a real file inside the root.
func TestCreateBuildError(t *testing.T) {
	root := newArtifact(t)
	if _, err := Create(root, "does-not-exist.md", "", time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("expected an error for a missing target artifact")
	}
}

// Create fails closed when the context pack reports a run ID that disagrees with the one Create
// derived — a guard against the two RunID derivations drifting apart. Only a seam can produce the
// disagreement, since both halves derive the ID from the same inputs in production.
func TestCreateRunIDMismatch(t *testing.T) {
	root := newArtifact(t)
	orig := buildContext
	t.Cleanup(func() { buildContext = orig })
	buildContext = func(r, target string, at time.Time) (contextpack.Result, error) {
		return contextpack.Result{RunID: "mismatched-run-id", ContextRel: "docs/x.md"}, nil
	}
	_, err := Create(root, "A.md", "", time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC))
	if err == nil || !strings.Contains(err.Error(), "run ID mismatch") {
		t.Fatalf("expected a run ID mismatch error, got %v", err)
	}
}

// Create surfaces the error from creating the reviews directory. The loop's os.Stat interacts with
// any on-disk blocker at that path, so the failure is injected through the mkdirAll seam scoped to
// the reviews directory alone.
func TestCreateReviewsDirError(t *testing.T) {
	root := newArtifact(t)
	orig := mkdirAll
	t.Cleanup(func() { mkdirAll = orig })
	sentinel := errors.New("reviews mkdir denied")
	mkdirAll = func(path string, perm os.FileMode) error {
		if strings.HasSuffix(path, "reviews") {
			return sentinel
		}
		return orig(path, perm)
	}
	_, err := Create(root, "A.md", "", time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC))
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected the reviews-dir error, got %v", err)
	}
}

// Create surfaces the error from seeding the empty findings ledger. A plain file at .metareview
// blocks the MkdirAll inside ensureEmpty — a real filesystem failure, no seam.
func TestCreateEnsureEmptyError(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "A.md"), []byte("# artifact\n\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// .metareview is a file, so ensureEmpty's MkdirAll(.metareview) cannot make a directory.
	if err := os.WriteFile(filepath.Join(root, ".metareview"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(root, "A.md", "", time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("expected an error when .metareview is a file")
	}
}

// Create surfaces the error from seeding the findings index. ensureEmpty (findings.jsonl) must
// succeed first, so the mkdirAll seam fails only on the FINDINGS.md directory (docs/metareview),
// leaving the reviews and context subdirectories creatable.
func TestCreateEnsureFindingsIndexError(t *testing.T) {
	root := newArtifact(t)
	orig := mkdirAll
	t.Cleanup(func() { mkdirAll = orig })
	sentinel := errors.New("findings index mkdir denied")
	findingsDir := filepath.Join("docs", "metareview")
	mkdirAll = func(path string, perm os.FileMode) error {
		if strings.HasSuffix(path, findingsDir) {
			return sentinel
		}
		return orig(path, perm)
	}
	_, err := Create(root, "A.md", "", time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC))
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected the findings-index error, got %v", err)
	}
}

// Create surfaces the error from writing the review scaffold. The loop's os.Stat rules out an
// on-disk blocker at the scaffold path, so the failure is injected through the writeFile seam scoped
// to the reviews path.
func TestCreateWriteReviewError(t *testing.T) {
	root := newArtifact(t)
	orig := writeFile
	t.Cleanup(func() { writeFile = orig })
	sentinel := errors.New("review write denied")
	writeFile = func(path string, data []byte, perm os.FileMode) error {
		if strings.Contains(path, filepath.Join("metareview", "reviews")) {
			return sentinel
		}
		return orig(path, data, perm)
	}
	_, err := Create(root, "A.md", "", time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC))
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected the review-write error, got %v", err)
	}
}

// Create surfaces the error from appending the run record. A directory at the runs.jsonl path makes
// the append's OpenFile fail — a real filesystem failure, no seam.
func TestCreateAppendRunError(t *testing.T) {
	root := newArtifact(t)
	// runs.jsonl as a directory: state.AppendJSONL's OpenFile(runs.jsonl) then fails.
	if err := os.MkdirAll(filepath.Join(root, ".metareview", "runs.jsonl"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := Create(root, "A.md", "", time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("expected an error when runs.jsonl is a directory")
	}
}
