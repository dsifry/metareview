package status

import (
	"encoding/json"
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
