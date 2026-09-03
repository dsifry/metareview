package reviewers

import (
	"fmt"
	"strings"

	"github.com/dsifry/metareview/internal/findings"
)

// HygieneContext carries the generated-artifact hygiene inputs (R5). StaleHeadBlockers are the ORPHANED
// stale-head open blockers the caller resolved from the ledger (findings.StaleHeadBlockersInLedger) — blockers
// recorded against a prior HEAD, not part of this run's reconcile set, that the render moves into the
// FINDINGS.md "Stale" section.
type HygieneContext struct {
	StaleHeadBlockers []findings.Record
}

// generatedArtifactHygieneFindings (R5) surfaces stale-head ledger cruft as an ADVISORY note — it never blocks
// the gate. A BLOCKING lens was rejected: same-target stale blockers already block via the verdict (openForRun
// is head-agnostic), so blocking there is redundant; and blocking on cross-target stale findings would
// false-positive on a legitimate other-branch open blocker sharing the ledger. The advisory gives the "clean
// the cross-head ledger" signal without that friction, and the render already partitions these into the
// FINDINGS.md "Stale" section so the committed index is no longer self-contradictory (the #90 harm).
func generatedArtifactHygieneFindings(ctx HygieneContext) []Finding {
	if len(ctx.StaleHeadBlockers) == 0 {
		return nil
	}
	ids := make([]string, 0, len(ctx.StaleHeadBlockers))
	for _, blocker := range ctx.StaleHeadBlockers {
		ids = append(ids, blocker.ID)
	}
	joined := strings.Join(ids, ", ")
	noun := "open blocking finding"
	if len(ids) != 1 {
		noun += "s"
	}
	return []Finding{{
		Reviewer:       "generated-artifact-hygiene-reviewer",
		Severity:       "low",
		Classification: "advisory",
		Title:          "Stale-head findings in the ledger",
		Finding:        fmt.Sprintf("The findings ledger carries %d %s recorded against a prior HEAD (rendered under the FINDINGS.md \"Stale\" section, not the current unresolved list). They do not block, but they are cruft the ledger accumulated across heads.", len(ids), noun),
		Expected:       "The committed FINDINGS.md reflects the current HEAD; stale cross-head findings are cleared.",
		Found:          "Stale-head findings: " + joined,
		Evidence:       []findings.Evidence{{Type: "findings-ledger"}},
		Recommendation: "Re-review at the current head (a fresh review supersedes them via `--previous-run`), or clear the cross-head ledger (`: > .metareview/findings.jsonl`) when they are known cruft.",
		Fingerprint:    "hygiene:stale-head-findings:" + joined,
	}}
}
