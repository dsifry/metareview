package epicready

import (
	"testing"

	"github.com/dsifry/metareview/internal/gitcontext"
	"github.com/dsifry/metareview/internal/reviewstate"
)

// The epic-ready gate requires an adjudicated review over the integration diff at this head (build B
// fast-follow). adversarialStatus resolves the review-evidence marker for scope "epic-ready" over the exact
// base..head, and flags an unattested working tree — since epic-ready always folds uncommitted content into
// the reviewed surface but the marker attests only the committed span.

func TestAdversarialStatusAbsentWhenNoMarker(t *testing.T) {
	root := t.TempDir()
	git := gitcontext.Context{BaseSHA: "base1", HeadSHA: "head1"}
	status := adversarialStatus(root, git, gitcontext.Context{})
	if status.Present {
		t.Fatalf("no marker recorded, want Present=false: %+v", status)
	}
	if status.HeadSHA != "head1" {
		t.Fatalf("HeadSHA must be the current head, got %q", status.HeadSHA)
	}
	// A blocked epic-ready must be steered to epic-review-loop, not the task-done-rubric review-loop.
	if status.WorkflowHint != "epic-review-loop" {
		t.Fatalf("WorkflowHint must be epic-review-loop, got %q", status.WorkflowHint)
	}
}

func TestAdversarialStatusResolvesMatchingMarker(t *testing.T) {
	root := t.TempDir()
	git := gitcontext.Context{BaseSHA: "base1", HeadSHA: "head1"}
	if err := reviewstate.RecordReviewEvidence(root, reviewstate.ReviewEvidence{
		ReviewedScope: "epic-ready", BaseSHA: "base1", HeadSHA: "head1",
		LensSet: []string{"epic-integration"}, AdjudicatedVerdict: "PASS",
		ExecutionMode: reviewstate.ReviewModeInSessionEmulated,
	}); err != nil {
		t.Fatal(err)
	}
	status := adversarialStatus(root, git, gitcontext.Context{})
	if !status.Present || status.Verdict != "PASS" || !status.Emulated {
		t.Fatalf("a matching emulated PASS marker must resolve present+PASS+emulated: %+v", status)
	}
}

// A matching subagent-adjudicated marker resolves present+PASS with Emulated=FALSE (so the gate credits it as
// independent, no advisory note) — the mirror of the emulated case above.
func TestAdversarialStatusResolvesSubagentMarkerNotEmulated(t *testing.T) {
	root := t.TempDir()
	git := gitcontext.Context{BaseSHA: "base1", HeadSHA: "head1"}
	if err := reviewstate.RecordReviewEvidence(root, reviewstate.ReviewEvidence{
		ReviewedScope: "epic-ready", BaseSHA: "base1", HeadSHA: "head1",
		LensSet: []string{"epic-integration"}, AdjudicatedVerdict: "PASS",
		ExecutionMode: reviewstate.ReviewModeSubagentAdjudicated, FromFSMRunID: "fsm-1",
	}); err != nil {
		t.Fatal(err)
	}
	status := adversarialStatus(root, git, gitcontext.Context{})
	if !status.Present || status.Verdict != "PASS" || status.Emulated {
		t.Fatalf("a subagent-adjudicated marker must resolve present+PASS+NOT emulated: %+v", status)
	}
}

func TestAdversarialStatusIgnoresDifferentBaseOrHead(t *testing.T) {
	root := t.TempDir()
	if err := reviewstate.RecordReviewEvidence(root, reviewstate.ReviewEvidence{
		ReviewedScope: "epic-ready", BaseSHA: "base1", HeadSHA: "head1",
		LensSet: []string{"epic-integration"}, AdjudicatedVerdict: "PASS",
		ExecutionMode: reviewstate.ReviewModeSubagentAdjudicated,
	}); err != nil {
		t.Fatal(err)
	}
	// A marker over a different base (main advanced) or a stale head must not be credited.
	for _, git := range []gitcontext.Context{
		{BaseSHA: "base2", HeadSHA: "head1"},
		{BaseSHA: "base1", HeadSHA: "head2"},
	} {
		if adversarialStatus(root, git, gitcontext.Context{}).Present {
			t.Fatalf("marker over base1..head1 must not satisfy %s..%s", git.BaseSHA, git.HeadSHA)
		}
	}
}

// A marker for a DIFFERENT scope (pr-ready) must not satisfy epic-ready even over the same base..head.
func TestAdversarialStatusIsScopeSpecific(t *testing.T) {
	root := t.TempDir()
	if err := reviewstate.RecordReviewEvidence(root, reviewstate.ReviewEvidence{
		ReviewedScope: "pr-ready", BaseSHA: "base1", HeadSHA: "head1",
		LensSet: []string{"security"}, AdjudicatedVerdict: "PASS",
		ExecutionMode: reviewstate.ReviewModeInSessionEmulated,
	}); err != nil {
		t.Fatal(err)
	}
	if adversarialStatus(root, gitcontext.Context{BaseSHA: "base1", HeadSHA: "head1"}, gitcontext.Context{}).Present {
		t.Fatal("a pr-ready marker must not satisfy the epic-ready gate")
	}
}

func TestAdversarialStatusFlagsUncommittedReviewedContent(t *testing.T) {
	root := t.TempDir()
	git := gitcontext.Context{BaseSHA: "base1", HeadSHA: "head1"}
	// epic-ready folds staged/working/untracked into the reviewed surface; any of them present → unattested.
	dirty := []gitcontext.Context{
		{StagedDiff: "diff --git a/x b/x\n+staged\n"},
		{WorkingTreeDiff: "diff --git a/y b/y\n+worktree\n"},
		{UntrackedExcerpts: "--- new.go\n+content\n"},
	}
	// Also the file-LIST signal, for uncommitted content that produces no diff excerpt: an untracked symlink or
	// directory, a staged empty/mode-only change. Keying on diff strings alone would miss these and wrongly
	// credit the committed-only marker.
	dirty = append(dirty,
		gitcontext.Context{StagedFiles: []string{"empty.go"}},
		gitcontext.Context{UnstagedFiles: []string{"mode-only.sh"}},
		gitcontext.Context{WorkingTreeFiles: []string{"touched.go"}},
		gitcontext.Context{UntrackedFiles: []string{"link"}}, // e.g. an untracked symlink → no excerpt
	)
	for _, reviewGit := range dirty {
		if !adversarialStatus(root, git, reviewGit).WorkingTreeUnattested {
			t.Fatalf("uncommitted reviewed content must set WorkingTreeUnattested: %+v", reviewGit)
		}
	}
	// A clean reviewed surface (committed diff only, no uncommitted files, plus whitespace-only diff noise) is
	// attested — no file lists set, diff strings trim to empty.
	clean := gitcontext.Context{Diff: "diff --git a/z b/z\n+committed\n", ChangedFiles: []string{"z"}, StagedDiff: "  \n", WorkingTreeDiff: "", UntrackedExcerpts: "\n\n"}
	if adversarialStatus(root, git, clean).WorkingTreeUnattested {
		t.Fatal("a committed-only reviewed surface must be attested")
	}
}
