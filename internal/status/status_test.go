package status

import (
	"encoding/json"
	"github.com/dsifry/metareview/internal/reviewlog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeLog(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, "docs", "metareview", "reviews")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeRuns(t *testing.T, root string, lines ...string) {
	t.Helper()
	dir := filepath.Join(root, ".metareview")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := ""
	for _, l := range lines {
		body += l + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "runs.jsonl"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// must_clear is initialised to an empty slice rather than left nil so the JSON emits [] and not
// null - a wire-contract choice for the hook consumers the package doc describes, and one nothing
// pinned: removing the initialiser was green, because the only test touching the field asked
// len(...) == 0, which a nil slice satisfies. A hook doing `for (const b of r.must_clear)` breaks
// on null and not on [].
func TestMustClearIsAnEmptyArrayNotNull(t *testing.T) {
	r, err := Build(t.TempDir())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"must_clear":[]`) {
		t.Errorf("a clean repository must emit must_clear as [], not null:\n%s", b)
	}
}

// The contract a host hook sits on: one machine-readable answer to "may I proceed, and if
// not, what must be cleared". Prose output cannot be a gate - a hook has to branch on it.
func TestReportIsMachineReadableAndNamesWhatMustBeCleared(t *testing.T) {
	root := t.TempDir()
	writeLog(t, root, "mrv-1-task-done-a.md", "# metareview: task-done review\n\nRun ID: `mrv-1`\nTarget: `task-a`\n\n## Verdict\n\nNEEDS_REVISION\n")
	r, err := Build(root)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("the report must marshal: %v", err)
	}
	// Asserting the field NAMES appear proves nothing: neither tag carries omitempty, so both
	// literals are in every marshalled Report - deleting the fixture above entirely left this
	// test passing. What a hook actually branches on is the CONTENT, so that is what is checked:
	// blocked true, and a must_clear entry carrying the operator-facing payload.
	var got struct {
		Blocked   bool `json:"blocked"`
		MustClear []struct {
			Target        string `json:"target"`
			RunID         string `json:"run_id"`
			Verdict       string `json:"verdict"`
			Kind          string `json:"kind"`
			Path          string `json:"path"`
			BlockingCount int    `json:"blocking_count"`
			AttemptNumber int    `json:"attempt_number"`
			MaxAttempts   int    `json:"max_attempts"`
		} `json:"must_clear"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("the report must unmarshal into the documented contract: %v\n%s", err, b)
	}
	if !got.Blocked {
		t.Errorf("blocked must be true when a review has unresolved blockers:\n%s", b)
	}
	if len(got.MustClear) != 1 {
		t.Fatalf("must_clear must name the blocking review, got %d entries:\n%s", len(got.MustClear), b)
	}
	e := got.MustClear[0]
	if e.Target != "task-a" || e.RunID != "mrv-1" || e.Verdict != "NEEDS_REVISION" {
		t.Errorf("must_clear entry does not identify the review: %+v", e)
	}
	if e.Kind != "task-done" {
		t.Errorf("kind = %q, want task-done: a hook uses it to pick which gate to re-run", e.Kind)
	}
	if e.Path == "" {
		t.Error("path is empty: a hook has no way to point an operator at the log")
	}
}

// A NEEDS_REVISION run superseded by a later PASS in its previousRunId lineage must NOT keep blocking.
// The repair loop is: a review finds blockers (NEEDS_REVISION) -> fix -> re-review with --previous-run
// passes (PASS). Nothing retires the parent today, so must_clear keeps it forever and the gate can never
// reach clean after any first review that found something — which is every real first review. This is the
// second half of docs/tasks/2026-09-01-status-covered-paths-gap.md.
func TestNeedsRevisionSupersededByLineagePass(t *testing.T) {
	root := t.TempDir()
	writeLog(t, root, "mrv-parent-task-done-a.md", "# metareview: task-done review\n\nRun ID: `mrv-parent`\nTarget: `task-a`\n\n## Verdict\n\nNEEDS_REVISION\n")
	writeLog(t, root, "mrv-child-task-done-a.md", "# metareview: task-done review\n\nRun ID: `mrv-child`\nTarget: `task-a`\n\nPrevious run: `mrv-parent`\n\n## Verdict\n\nPASS\n")
	r, err := Build(root)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if r.Blocked || len(r.MustClear) != 0 {
		t.Fatalf("a NEEDS_REVISION superseded by a lineage PASS must not block; blocked=%v must_clear=%+v", r.Blocked, r.MustClear)
	}
}

// A NEEDS_REVISION with NO passing descendant still blocks: only a lineage PASS supersedes it, and an
// unrelated PASS (different lineage) must not clear it — that would be the stale-review failure one level up.
func TestNeedsRevisionWithoutLineagePassStillBlocks(t *testing.T) {
	root := t.TempDir()
	writeLog(t, root, "mrv-open-task-done-a.md", "# metareview: task-done review\n\nRun ID: `mrv-open`\nTarget: `task-a`\n\n## Verdict\n\nNEEDS_REVISION\n")
	// A PASS of a DIFFERENT target/lineage must not retire the open one.
	writeLog(t, root, "mrv-other-task-done-b.md", "# metareview: task-done review\n\nRun ID: `mrv-other`\nTarget: `task-b`\n\n## Verdict\n\nPASS\n")
	r, err := Build(root)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(r.MustClear) != 1 || r.MustClear[0].RunID != "mrv-open" {
		t.Fatalf("an unsuperseded NEEDS_REVISION must still block; must_clear=%+v", r.MustClear)
	}
}

// blocking_count, attempt_number and max_attempts are the operator-facing half of the contract -
// how many blockers remain, and which attempt of how many, which is the escalation signal the
// Completion Rule depends on. Deleting the line that populates all three left ./internal/status
// and ./cmd/... green, and no assertion on any of the three names existed anywhere.
func TestBlockerCarriesTheAttemptAndBlockerCounts(t *testing.T) {
	root := t.TempDir()
	writeLog(t, root, "mrv-2-task-done-b.md", "# metareview: task-done review\n\nRun ID: `mrv-2`\nTarget: `task-b`\n\n## Verdict\n\nNEEDS_REVISION\n")
	writeRuns(t, root, `{"id":"mrv-2","attemptNumber":2,"maxAttempts":3,"blockingFindingCount":4,"verdict":"NEEDS_REVISION"}`)
	r, err := Build(root)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(r.MustClear) != 1 {
		t.Fatalf("must_clear = %d entries, want 1", len(r.MustClear))
	}
	e := r.MustClear[0]
	if e.BlockingCount != 4 {
		t.Errorf("blocking_count = %d, want 4", e.BlockingCount)
	}
	if e.AttemptNumber != 2 || e.MaxAttempts != 3 {
		t.Errorf("attempt = %d of %d, want 2 of 3", e.AttemptNumber, e.MaxAttempts)
	}
}

// A clean repository must not block: a hook that always blocks is a livelock, not a gate.
func TestCleanRepositoryIsNotBlocked(t *testing.T) {
	r, err := Build(t.TempDir())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if r.Blocked {
		t.Error("no reviews means nothing to clear")
	}
	if len(r.MustClear) != 0 {
		t.Errorf("MustClear = %v, want empty", r.MustClear)
	}
}

// Blocking is decided by the review log's own predicate, not a second definition invented
// here: two definitions of "blocker" would drift, which is the defect this branch keeps finding.
func TestBlockedFollowsTheReviewLogsOwnVerdict(t *testing.T) {
	root := t.TempDir()
	writeLog(t, root, "mrv-2-task-done-b.md", "# metareview: task-done review\n\nRun ID: `mrv-2`\nTarget: `task-b`\n\n## Verdict\n\nPASS\n")
	r, err := Build(root)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, s := range r.Reviews {
		if s.HasUnresolvedBlockers != containsTarget(r.MustClear, s.Target) {
			t.Errorf("MustClear disagrees with the review log for %q", s.Target)
		}
	}
}

func containsTarget(list []Blocker, target string) bool {
	for _, b := range list {
		if b.Target == target {
			return true
		}
	}
	return false
}

// A review log that cannot be read must surface as an error, not as "nothing to clear" -
// a gate that fails open is not a gate.
func TestUnreadableReviewsAreAnErrorNotAnAllClear(t *testing.T) {
	root := t.TempDir()
	// the reviews directory is a regular file, so reading it fails
	if err := os.MkdirAll(filepath.Join(root, "docs", "metareview"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "metareview", "reviews"), []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Build(root)
	if err == nil {
		t.Fatal("want an error when the review logs cannot be read")
	}
	if r.Blocked {
		t.Error("an errored Build must not also claim blocked; the caller decides what to do")
	}
}

// Emit keeps main.go thin: the marshalling and the exit decision are part of the contract and
// belong where they can be tested, not in an untested main.
func TestEmitWritesJSONAndReportsTheExitCode(t *testing.T) {
	var buf strings.Builder
	code, err := Emit(t.TempDir(), &buf)
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if code != 0 {
		t.Errorf("exit = %d, want 0 for a clean repository", code)
	}
	var back Report
	if err := json.Unmarshal([]byte(buf.String()), &back); err != nil {
		t.Fatalf("Emit must write valid JSON: %v\n%s", err, buf.String())
	}
	if back.Blocked {
		t.Error("clean repository must not report blocked")
	}
}

func TestEmitReportsExitOneWhenSomethingMustBeCleared(t *testing.T) {
	root := t.TempDir()
	writeLog(t, root, "mrv-3-task-done-c.md", "# metareview: task-done review\n\nRun ID: `mrv-3`\nTarget: `task-c`\n\n## Verdict\n\nNEEDS_REVISION\n")
	var buf strings.Builder
	code, err := Emit(root, &buf)
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if code != 1 {
		t.Errorf("exit = %d, want 1 so a hook needs no parsing for the common decision", code)
	}
	if !strings.Contains(buf.String(), "task-c") {
		t.Errorf("the blocker must be named in the output:\n%s", buf.String())
	}
}

func TestEmitSurfacesBuildErrors(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs", "metareview"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "metareview", "reviews"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	if _, err := Emit(root, &buf); err == nil {
		t.Error("want the read failure surfaced rather than an all-clear")
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, os.ErrClosed }

// A hook reads this on stdout; if the write fails the caller must hear about it rather than
// receive a silent exit 0 that reads as "nothing to clear".
func TestEmitSurfacesWriteFailures(t *testing.T) {
	if _, err := Emit(t.TempDir(), failWriter{}); err == nil {
		t.Error("want the write failure surfaced")
	}
}

// --target narrows the report to the work in hand. Without it `blocked` spans the whole review
// history — 66 entries on this repository when this was written — so a Stop hook wired to it
// would block an agent on work it never touched. That is a livelock, not a gate, and it is the
// reason the hooks could not ship.
func TestTargetScopingNarrowsWhatMustBeCleared(t *testing.T) {
	root := t.TempDir()
	writeLog(t, root, "mrv-a-task-done-alpha.md",
		"# metareview: task-done review\n\nRun ID: `mrv-a`\nTarget: `internal/alpha/thing.go`\n\n## Verdict\n\nNEEDS_REVISION\n")
	writeLog(t, root, "mrv-b-task-done-beta.md",
		"# metareview: task-done review\n\nRun ID: `mrv-b`\nTarget: `internal/beta/other.go`\n\n## Verdict\n\nNEEDS_REVISION\n")
	// A target that was reviewed and passed, so the gate can be shown to let work through.
	const cleanTarget = "internal/delta/done.go"
	writeLog(t, root, "mrv-c-task-done-delta.md",
		"# metareview: task-done review\n\nRun ID: `mrv-c`\nTarget: `"+cleanTarget+"`\n\n## Verdict\n\nPASS\n")

	all, err := Build(root)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(all.MustClear) != 2 || !all.Blocked {
		t.Fatalf("unscoped must see both: %+v", all.MustClear)
	}

	scoped, err := BuildFor(root, "internal/alpha/thing.go")
	if err != nil {
		t.Fatalf("BuildFor: %v", err)
	}
	if len(scoped.MustClear) != 1 || scoped.MustClear[0].Target != "internal/alpha/thing.go" {
		t.Fatalf("scoped must see only its own target: %+v", scoped.MustClear)
	}
	if !scoped.Blocked {
		t.Error("a blocker on the target in hand must still block")
	}

	// The point of the flag: an agent working on beta is not held up by alpha's blocker.
	other, err := BuildFor(root, "internal/beta/other.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(other.MustClear) != 1 || other.MustClear[0].Target != "internal/beta/other.go" {
		t.Fatalf("scoping must not leak across targets: %+v", other.MustClear)
	}

	// A target NO REVIEW COVERS is blocked, and blocked as UNREVIEWED rather than as a finding.
	// This assertion used to be the opposite, and that was the bug: the narrower the scope an
	// agent claimed, the more certainly the gate let it through, because a target nothing had
	// reviewed matched no log, produced an empty must_clear and reported blocked:false. Asking
	// about a file that had never been reviewed was the reliable way to be told all was well.
	unreviewed, err := BuildFor(root, "internal/gamma/new.go")
	if err != nil {
		t.Fatal(err)
	}
	if !unreviewed.Blocked || len(unreviewed.MustClear) != 1 {
		t.Fatalf("a target no review covers must not read as cleared: %+v", unreviewed)
	}
	if got := unreviewed.MustClear[0].Verdict; got != VerdictUnreviewed {
		t.Errorf("verdict = %q, want %q: a hook must tell \"fix your findings\" from \"run a review\"", got, VerdictUnreviewed)
	}
	// A target that WAS reviewed and came back clean still passes, or the gate is a livelock.
	passed, err := BuildFor(root, cleanTarget)
	if err != nil {
		t.Fatal(err)
	}
	if passed.Blocked {
		t.Errorf("a reviewed, passing target must let work through: %+v", passed)
	}
	// And the scope is reported, so a reader knows whether they are seeing everything.
	if unreviewed.Target != "internal/gamma/new.go" || all.Target != "" {
		t.Errorf("the report must say what it was scoped to: %q / %q", unreviewed.Target, all.Target)
	}
}

// Scoping narrows the whole report, not only must_clear: a document headed `"target": "t-1"` that
// still lists every other target's reviews invites exactly the misreading the field exists to
// prevent.
func TestTargetScopingNarrowsTheReviewListToo(t *testing.T) {
	root := t.TempDir()
	writeLog(t, root, "mrv-a-task-done-alpha.md",
		"# metareview: task-done review\n\nRun ID: `mrv-a`\nTarget: `alpha.go`\n\n## Verdict\n\nPASS\n")
	writeLog(t, root, "mrv-b-task-done-beta.md",
		"# metareview: task-done review\n\nRun ID: `mrv-b`\nTarget: `beta.go`\n\n## Verdict\n\nPASS\n")

	all, err := Build(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Reviews) != 2 {
		t.Fatalf("unscoped lists everything: %d", len(all.Reviews))
	}
	scoped, err := BuildFor(root, "alpha.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(scoped.Reviews) != 1 || scoped.Reviews[0].Target != "alpha.go" {
		t.Errorf("a scoped report must list only its own target's reviews: %+v", scoped.Reviews)
	}
}

// A review answers for a path when it EXAMINED that path, not only when the path happens to be
// its target. Target strings are task ids, document paths, or the literal `current branch`, so a
// source file matched no review at all — and an unmatched file used to report as clear.
func TestCoversUsesTheFilesAReviewActuallyRead(t *testing.T) {
	// The review examined a commit this branch has: it may answer for what it read.
	now := map[string]bool{"c1": true}
	branch := reviewlog.Summary{Target: "current branch", HeadSHA: "c1", CoveredPaths: []string{"internal/a.go", "internal/b.go"}}
	if !covers(branch, "internal/a.go", now) {
		t.Error("a review that read the file must be able to answer for it")
	}
	if covers(branch, "internal/c.go", now) {
		t.Error("a review must not answer for a file it never read")
	}
	if !covers(branch, "current branch", now) {
		t.Error("the named target still matches")
	}

	// The same review, recorded on a commit this branch does not have. It read internal/a.go —
	// but some other version of it. Crediting that returned blocked:false and exit 0 for work
	// nothing current had reviewed, and the Stop hook reaches this path whenever
	// METAREVIEW_TARGET is set, so it was reachable in enforcement.
	stale := branch
	stale.HeadSHA = "elsewhere"
	if covers(stale, "internal/a.go", now) {
		t.Error("a review of another commit must not answer for this branch's version of a file")
	}
	if !covers(stale, "current branch", now) {
		t.Error("it still answers for its own named target, whatever commit it ran on")
	}
	// An unknown commit set is not permission to credit: unknown fails toward blocking.
	if covers(branch, "internal/a.go", nil) {
		t.Error("with no commit set established, a path claim cannot be credited")
	}
	// A review recorded before CoveredPaths existed carries none. It cannot answer for a path,
	// and must not: an old log silently clearing a file it never read is the failure this whole
	// change is about.
	legacy := reviewlog.Summary{Target: "task-1"}
	if covers(legacy, "internal/a.go", now) {
		t.Error("a review with no recorded paths cannot answer for a path")
	}
	if !covers(legacy, "task-1", now) {
		t.Error("a legacy review still answers for its own target")
	}
}
