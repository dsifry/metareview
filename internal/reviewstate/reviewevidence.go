package reviewstate

import (
	"os"
	"path/filepath"
	"time"

	"github.com/dsifry/metareview/internal/state"
)

// ReviewEvidenceScope is the runs.jsonl `scope` value of a review-evidence marker. It is deliberately NOT one
// of the lifecycle scopes (pr-ready/task-done/epic-ready/artifact), so the scope-filtered run readers
// (runchain, the projector) skip markers while the adversarial-review gate reads them by Kind.
const ReviewEvidenceScope = "review-evidence"

// ReviewEvidenceKind identifies a review-evidence record within runs.jsonl.
const ReviewEvidenceKind = "review-evidence"

// Execution modes for the recorded adversarial review.
const (
	// ReviewModeSubagentAdjudicated is the real thing: the FSM review-lenses ran as subagents and were
	// adjudicated by the judge.
	ReviewModeSubagentAdjudicated = "subagent-adjudicated"
	// ReviewModeInSessionEmulated is the labeled escape hatch: the agent ran the lenses inline (no subagents),
	// which is weaker, non-independent evidence.
	ReviewModeInSessionEmulated = "in-session-emulated"
)

// ReviewEvidence is the durable marker that an adjudicated lens review ran over a diff AT a specific head.
// The deterministic gate (pr-ready/task-done) requires one matching the current head before it can PASS — it
// is the bridge from the FSM's review-lenses/adjudicate engine to the gate. Written to .metareview/runs.jsonl
// (the log the gate already reads). See docs/specs/2026-09-03-require-adjudicated-review.md.
type ReviewEvidence struct {
	SchemaVersion       int      `json:"schemaVersion"`
	Kind                string   `json:"kind"`          // always ReviewEvidenceKind
	Scope               string   `json:"scope"`         // always ReviewEvidenceScope, so run readers skip it
	ReviewedScope       string   `json:"reviewedScope"` // the gate this satisfies: "pr-ready" | "task-done"
	HeadSHA             string   `json:"headSha"`       // the diff head the review covered
	BaseSHA             string   `json:"baseSha,omitempty"`
	LensSet             []string `json:"lensSet"`            // the lenses that ran
	AdjudicatedVerdict  string   `json:"adjudicatedVerdict"` // the reviewer set's verdict
	ConfirmedFindingIDs []string `json:"confirmedFindingIds,omitempty"`
	ExecutionMode       string   `json:"executionMode"` // ReviewModeSubagentAdjudicated | ReviewModeInSessionEmulated
	FromFSMRunID        string   `json:"fromFsmRunId,omitempty"`
	CreatedAt           string   `json:"createdAt"`
}

// IsEmulated reports whether the marker is the weaker in-session escape hatch.
func (e ReviewEvidence) IsEmulated() bool { return e.ExecutionMode == ReviewModeInSessionEmulated }

// RecordReviewEvidence appends a review-evidence marker to runs.jsonl, filling the invariant fields.
func RecordReviewEvidence(root string, ev ReviewEvidence) error {
	ev.SchemaVersion = 1
	ev.Kind = ReviewEvidenceKind
	ev.Scope = ReviewEvidenceScope
	if ev.CreatedAt == "" {
		ev.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return state.AppendJSONL(filepath.Join(root, ".metareview", "runs.jsonl"), ev)
}

// DiscoverReviewEvidence returns every review-evidence marker in runs.jsonl, skipping ordinary run records
// (which share the file but carry a different, empty Kind).
func DiscoverReviewEvidence(root string) ([]ReviewEvidence, error) {
	all, err := state.ReadJSONL[ReviewEvidence](filepath.Join(root, ".metareview", "runs.jsonl"))
	if err != nil {
		return nil, err
	}
	markers := make([]ReviewEvidence, 0, len(all))
	for _, e := range all {
		if e.Kind == ReviewEvidenceKind {
			markers = append(markers, e)
		}
	}
	return markers, nil
}

// LatestReviewEvidence returns the most recent marker (by CreatedAt) covering reviewedScope at headSHA, if
// any. A marker for a DIFFERENT head does not satisfy the gate: a new commit requires a fresh review, which
// is how the push gate already reasons about currency.
func LatestReviewEvidence(root, reviewedScope, headSHA string) (ReviewEvidence, bool, error) {
	markers, err := DiscoverReviewEvidence(root)
	if err != nil {
		return ReviewEvidence{}, false, err
	}
	var best ReviewEvidence
	found := false
	for _, m := range markers {
		if m.ReviewedScope == reviewedScope && m.HeadSHA == headSHA {
			if !found || m.CreatedAt > best.CreatedAt {
				best, found = m, true
			}
		}
	}
	return best, found, nil
}

// RequireAdjudicatedReview reports whether the gate must require a real adjudicated lens review (build B).
// Default true; set METAREVIEW_ALLOW_MECHANICAL_PASS=1 to restore the legacy deterministic pass — a one-release
// migration escape so in-flight branches are not suddenly wedged.
func RequireAdjudicatedReview() bool {
	return os.Getenv("METAREVIEW_ALLOW_MECHANICAL_PASS") != "1"
}
