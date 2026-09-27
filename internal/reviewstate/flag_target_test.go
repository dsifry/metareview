package reviewstate

import (
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/reviewlog"
)

// TestFlagTargetRunIDs pins the selection: task-done or epic-ready, a run id, and a target that is exactly --help or
// -h — the pre-#164 artifacts that were never reviews of work. Any other dash target, or a padded one, may have
// been a real review of current work, so it keeps blocking.
func TestFlagTargetRunIDs(t *testing.T) {
	logs := []reviewlog.Summary{
		{RunID: "mrv-help", Kind: "task-done", Target: "--help"},
		{RunID: "mrv-h", Kind: "task-done", Target: "-h"},
		{RunID: "mrv-padded", Kind: "task-done", Target: " --help"}, // a real review recorded under a padded flag keeps blocking
		{RunID: "mrv-task", Kind: "task-done", Target: "task-1"},
		{RunID: "mrv-dash-inside", Kind: "task-done", Target: "fix--help"},
		{RunID: "mrv-verbose", Kind: "task-done", Target: "--verbose"},
		{RunID: "mrv-help-arg", Kind: "task-done", Target: "--help=x"},
		{RunID: "", Kind: "task-done", Target: "--help"},
		{RunID: "mrv-epic", Kind: "epic-ready", Target: "--help", Verdict: "ESCALATED"}, // the verdict is not consulted
		{RunID: "mrv-pr", Kind: "pr-ready", Target: "--help"},
		{RunID: "mrv-empty", Kind: "task-done", Target: ""},
	}
	if got := strings.Join(FlagTargetRunIDs(logs), ","); got != "mrv-help,mrv-h,mrv-epic" {
		t.Fatalf("flag-target ids = %q", got)
	}
}

// TestFlagTargetRunIDsFailsClosedOnDuplicateIDs: callers retire by run id alone, so an id shared with another log
// (duplicated or hand-authored) would retire that log too. An ambiguous id is never selected.
func TestFlagTargetRunIDsFailsClosedOnDuplicateIDs(t *testing.T) {
	logs := []reviewlog.Summary{
		{RunID: "mrv-dup", Kind: "task-done", Target: "--help"},
		{RunID: "mrv-dup", Kind: "task-done", Target: "task-1"},
		{RunID: "mrv-h", Kind: "task-done", Target: "-h"},
	}
	if got := strings.Join(FlagTargetRunIDs(logs), ","); got != "mrv-h" {
		t.Fatalf("flag-target ids = %q, want only the unambiguous mrv-h", got)
	}
}
