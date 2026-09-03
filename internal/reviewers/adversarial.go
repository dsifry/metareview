package reviewers

import "strings"

// AdversarialReviewStatus is the resolved review-evidence for the current head, computed by the caller (which
// reads internal/reviewstate) so this package stays free of that dependency. Present=false means no adjudicated
// lens review has been recorded for this exact head.
type AdversarialReviewStatus struct {
	Present  bool
	Verdict  string // the adjudicated verdict of the recorded review, when Present
	Emulated bool   // true = in-session-emulated (weaker, non-independent evidence)
	HeadSHA  string
	// WorkingTreeUnattested is set when the reviewed surface includes uncommitted working-tree content the
	// marker cannot vouch for (pr-ready folds it only behind --include-working-tree; epic-ready folds it
	// unconditionally). The marker attests only the committed base..HEAD, so it must not satisfy the gate.
	WorkingTreeUnattested bool
	// WorkflowHint names the FSM workflow the remediation message should recommend for this scope, so a
	// blocked scope is steered to the workflow whose lenses apply ITS rubric — epic-ready must be sent to
	// epic-review-loop, not the task-done-rubric review-loop (which validateFromRunDiff would then credit for
	// epic-ready, silently bypassing the epic lenses). Empty defaults to "review-loop" (pr-ready/task-done).
	WorkflowHint string
}

// adversarialReviewFindings enforces that a real adjudicated lens review ran over THIS head. When required and
// absent it BLOCKS (the mechanistic pass is gone); when present-but-not-a-pass it blocks on the review's own
// unresolved findings; when present-and-passing-but-emulated it records a non-blocking weaker-evidence note.
// `require` is the feature flag (default on; off restores the legacy deterministic pass for one migration
// release). Shared by the pr-ready, task-done, and epic-ready reviewer sets.
func adversarialReviewFindings(require bool, s AdversarialReviewStatus) []Finding {
	if !require {
		return nil
	}
	head := s.HeadSHA
	if head == "" {
		head = "the current head"
	}
	workflow := s.WorkflowHint
	if workflow == "" {
		workflow = "review-loop"
	}
	if !s.Present {
		return []Finding{finding(Finding{
			Reviewer:       "adversarial-review-reviewer",
			Severity:       "high",
			Title:          "No adjudicated lens review recorded",
			Finding:        "This gate requires an adversarial lens review adjudicated over this exact diff; none is recorded for HEAD " + head + ".",
			Expected:       "A recorded, adjudicated review-lenses run over base..HEAD, or an explicitly-labeled in-session-emulated review.",
			Found:          "No review-evidence marker matches this head.",
			Recommendation: "Run `metareview fsm --workflow " + workflow + " --base <ref>` to review the diff and record the result, then re-run this gate; or record an in-session review with `metareview review record-lenses` when subagents are unavailable.",
			Fingerprint:    "review:no-adjudicated-review",
		})}
	}
	if !isPassVerdict(s.Verdict) {
		return []Finding{finding(Finding{
			Reviewer:       "adversarial-review-reviewer",
			Severity:       "high",
			Title:          "Adjudicated lens review has unresolved findings",
			Finding:        "The recorded adjudicated review of this head returned " + s.Verdict + ", not a pass.",
			Expected:       "The adversarial review's blocking findings are fixed and re-reviewed (a fresh marker), or explicitly human-accepted.",
			Found:          "The latest review-evidence marker for HEAD " + head + " has verdict " + s.Verdict + ".",
			Recommendation: "Clear the review's findings and re-run the review loop, then re-run this gate.",
			Fingerprint:    "review:adjudicated-review-not-clean",
		})}
	}
	if s.WorkingTreeUnattested {
		// The review covers uncommitted content the marker (base..HEAD only) cannot vouch for.
		return []Finding{finding(Finding{
			Reviewer:       "adversarial-review-reviewer",
			Severity:       "high",
			Title:          "Adjudicated review does not cover the working tree",
			Finding:        "This run's reviewed surface includes uncommitted working-tree changes, but the recorded review-evidence marker attests only the committed base..HEAD diff — it cannot vouch for uncommitted content.",
			Expected:       "The reviewed content is committed (so the marker's base..HEAD covers it), or a fresh review is recorded over the working tree.",
			Found:          "A marker for HEAD " + head + " exists, but the working tree has uncommitted changes it does not attest.",
			Recommendation: "Commit (or stash/discard) the working-tree changes so the marker's committed base..HEAD covers the reviewed surface, then re-run the gate.",
			Fingerprint:    "review:working-tree-unattested",
		})}
	}
	if s.Emulated {
		// Satisfied, but weaker: not independently adversarial. Recorded as ADVISORY, never blocking.
		return []Finding{{
			Reviewer:       "adversarial-review-reviewer",
			Severity:       "low",
			Classification: "advisory",
			Title:          "Adversarial review was in-session-emulated",
			Finding:        "The adjudicated review for this head was recorded as in-session-emulated (no independent subagents), which is weaker, non-independent evidence.",
			Expected:       "An independent subagent-adjudicated review where delegation is available.",
			Found:          "executionMode = in-session-emulated.",
			Recommendation: "When subagents are available, prefer `metareview fsm --workflow " + workflow + "` for an independent review.",
			Fingerprint:    "review:adversarial-review-emulated",
		}}
	}
	return nil // present, passing, independent → satisfied
}

func isPassVerdict(v string) bool {
	return strings.EqualFold(v, "PASS") || strings.EqualFold(v, "PASS_ADVISORY")
}
