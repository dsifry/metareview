package reviewstate

import (
	"os"
	"path/filepath"
	"strings"
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
// The deterministic gate (pr-ready/task-done/epic-ready) requires one matching the current head before it can PASS — it
// is the bridge from the FSM's review-lenses/adjudicate engine to the gate. Written to .metareview/runs.jsonl
// (the log the gate already reads). See docs/specs/2026-09-03-require-adjudicated-review.md.
type ReviewEvidence struct {
	SchemaVersion       int      `json:"schemaVersion"`
	Kind                string   `json:"kind"`          // always ReviewEvidenceKind
	Scope               string   `json:"scope"`         // always ReviewEvidenceScope, so run readers skip it
	ReviewedScope       string   `json:"reviewedScope"` // the gate this satisfies: "pr-ready" | "task-done" | "epic-ready"
	HeadSHA             string   `json:"headSha"`       // the diff head the review covered
	BaseSHA             string   `json:"baseSha,omitempty"`
	RequestedBase       string   `json:"requestedBase,omitempty"` // --base as typed, beside the SHA (#175); never matched on
	LensSet             []string `json:"lensSet"`                 // the lenses that ran
	AdjudicatedVerdict  string   `json:"adjudicatedVerdict"`      // the reviewer set's verdict
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

// LatestReviewEvidence returns the marker covering reviewedScope over the exact baseSHA..headSHA diff, if
// any. Currency is BOTH endpoints: a marker for a different head does not satisfy the gate (a new commit
// requires a fresh review), and neither does one adjudicated over a different base — a review of a narrow
// base..head must not be credited for a wider one. Of several matching markers the LAST-RECORDED wins
// (append order in runs.jsonl is record order): re-reviewing an unchanged head lets the newer verdict
// supersede the older, and it avoids the string-compare tie-break trap where an RFC3339Nano stamp on an
// exact-zero-nanosecond second sorts after a later fractional one.
//
// It is CurrentReviewEvidence with no carry-over: one selection loop, two rules.
func LatestReviewEvidence(root, reviewedScope, baseSHA, headSHA string) (ReviewEvidence, bool, error) {
	return CurrentReviewEvidence(root, reviewedScope, baseSHA, headSHA, exactOnly)
}

// exactOnly reports every earlier head as unrelated, so only an exact-head marker counts.
func exactOnly(string, string) ([]string, bool, error) { return nil, false, nil }

// gateArtifactDirs are the folders under docs/metareview/ the gates write for committing (review logs, context packs,
// shard results, FSM export bundles, post-merge learning), and gateArtifactExts the only kinds of file they write there
// — plus the workflow.yaml every `fsm export` bundle carries (fsmBundleDir).
var (
	gateArtifactDirs = []string{"docs/metareview/reviews/", "docs/metareview/context/", "docs/metareview/shards/",
		fsmBundleDir, "docs/metareview/learning/"}
	gateArtifactExts = []string{".md", ".json", ".jsonl"}
)

const fsmBundleDir = "docs/metareview/fsm/"

// IsGateArtifact reports whether path (repository-relative, slash-separated) is a file the review gates write and ask
// to have committed after they pass: a Markdown or JSON(L) file in one of their folders, or the rendered
// docs/metareview/FINDINGS.md. Nothing else is — not a .go file dropped into those folders (it would be compiled), nor
// another document beside them.
func IsGateArtifact(path string) bool {
	if path == "docs/metareview/FINDINGS.md" {
		return true
	}
	if strings.Contains(path, "..") {
		return false
	}
	// Exactly a bundle's own workflow.yaml (docs/metareview/fsm/<run>/workflow.yaml), nowhere deeper.
	if run, ok := strings.CutSuffix(strings.TrimPrefix(path, fsmBundleDir), "/workflow.yaml"); ok && strings.HasPrefix(path, fsmBundleDir) &&
		run != "" && !strings.Contains(run, "/") {
		return true
	}
	for _, dir := range gateArtifactDirs {
		if strings.HasPrefix(path, dir) {
			for _, ext := range gateArtifactExts {
				if strings.HasSuffix(path, ext) {
					return true
				}
			}
		}
	}
	return false
}

// CurrentReviewEvidence is LatestReviewEvidence that also counts a marker recorded at an earlier head when every
// commit since it only added gate artifacts (IsGateArtifact) — committing a passing gate's review log, shard results,
// FSM bundles or FINDINGS.md must not strand the review of the code, which is unchanged (#161). changed reports whether
// a marker's head is an ancestor of head and which paths changed since; a failure there never counts the marker (fail
// closed). Code, tests or any other document committed after the marker still invalidate it. As in the exact match,
// the last-recorded eligible marker wins, so a later NEEDS_REVISION withdraws an earlier PASS.
func CurrentReviewEvidence(root, reviewedScope, baseSHA, headSHA string, changed func(from, to string) ([]string, bool, error)) (ReviewEvidence, bool, error) {
	markers, err := DiscoverReviewEvidence(root)
	if err != nil {
		return ReviewEvidence{}, false, err
	}
	// Newest first: the first eligible marker is the last-recorded one, and older heads are never asked about.
	carried := map[string]bool{}
	for i := len(markers) - 1; i >= 0; i-- {
		m := markers[i]
		if m.ReviewedScope != reviewedScope || m.BaseSHA != baseSHA || m.HeadSHA == "" {
			continue
		}
		if m.HeadSHA != headSHA {
			ok, seen := carried[m.HeadSHA]
			if !seen {
				ok = onlyGateArtifactsSince(m.HeadSHA, headSHA, changed)
				carried[m.HeadSHA] = ok
			}
			if !ok {
				continue
			}
		}
		return m, true, nil
	}
	return ReviewEvidence{}, false, nil
}

func onlyGateArtifactsSince(from, to string, changed func(from, to string) ([]string, bool, error)) bool {
	paths, ancestor, err := changed(from, to)
	if err != nil || !ancestor {
		return false
	}
	for _, p := range paths {
		if !IsGateArtifact(p) {
			return false
		}
	}
	return true
}

// RequireAdjudicatedReview reports whether the gate must require a real adjudicated lens review (build B).
// Default true; set METAREVIEW_ALLOW_MECHANICAL_PASS=1 to restore the legacy deterministic pass — a one-release
// migration escape so in-flight branches are not suddenly wedged.
func RequireAdjudicatedReview() bool {
	return os.Getenv("METAREVIEW_ALLOW_MECHANICAL_PASS") != "1"
}
