package prready

import (
	"fmt"
	"strings"

	"github.com/dsifry/metareview/internal/findings"
	"github.com/dsifry/metareview/internal/githubcontext"
	"github.com/dsifry/metareview/internal/markdown"
	"github.com/dsifry/metareview/internal/reviewlog"
)

type EvidenceInput struct {
	Summary       string
	Validation    []string
	TaskReviews   []ReviewEvidence
	EpicReviews   []ReviewEvidence
	Blockers      []Blocker
	CurrentReview *ReviewEvidence
	GitHub        githubcontext.Context
	// Findings is the full finding ledger (every status, not just unresolved
	// blockers). The reconciliation layer (#40) cross-references a historical
	// review's finding IDs against it, so an entry whose blockers were cleared by
	// an override or a fix is rendered historical/resolved rather than
	// contradicting a clear blocker-status section.
	Findings []findings.Record
}

type ReviewEvidence struct {
	Target                string
	Verdict               string
	Path                  string
	FindingIDs            []string
	HasUnresolvedBlockers bool
	AttemptNumber         int
	MaxAttempts           int
	BlockingFindingCount  int
	AdvisoryFindingCount  int
	FollowUpFindingCount  int
	// Reconciliation annotations (#40), filled by reconcileReviews. Resolved is
	// true when every blocking finding this review raised has since been cleared
	// (overridden or fixed), so the entry is rendered historical rather than
	// "with unresolved blockers". Resolution names how (the override grant or the
	// fixing run). AttemptNote explains an over-limit attempt counter so a bare
	// "N/M" with N>M never renders.
	Resolved    bool
	Resolution  string
	AttemptNote string
}

type Blocker struct {
	ID     string
	Title  string
	Status string
}

func FromReviewLog(log reviewlog.Summary) ReviewEvidence {
	return ReviewEvidence{
		Target:                log.Target,
		Verdict:               log.Verdict,
		Path:                  log.Path,
		FindingIDs:            log.FindingIDs,
		HasUnresolvedBlockers: log.HasUnresolvedBlockers,
		AttemptNumber:         log.AttemptNumber,
		MaxAttempts:           log.MaxAttempts,
		BlockingFindingCount:  log.BlockingFindingCount,
		AdvisoryFindingCount:  log.AdvisoryFindingCount,
		FollowUpFindingCount:  log.FollowUpFindingCount,
	}
}

func RenderEvidence(input EvidenceInput) string {
	byID := indexFindings(input.Findings)
	taskReviews := reconcileReviews(input.TaskReviews, byID)
	epicReviews := reconcileReviews(input.EpicReviews, byID)
	var builder strings.Builder
	builder.WriteString("## metareview PR Evidence\n\n")
	builder.WriteString("### Summary\n\n")
	builder.WriteString(firstNonEmpty(githubcontext.Redact(input.Summary), "No PR summary supplied."))
	builder.WriteString("\n\n")
	builder.WriteString("### Validation\n\n")
	builder.WriteString(list(input.Validation, "No validation evidence supplied."))
	builder.WriteString("\n\n")
	builder.WriteString("### Task Review Evidence\n\n")
	builder.WriteString(reviewList(taskReviews, "No task review evidence discovered."))
	builder.WriteString("\n\n")
	builder.WriteString("### Epic Review Evidence\n\n")
	builder.WriteString(reviewList(epicReviews, "No epic review evidence discovered."))
	builder.WriteString("\n\n")
	builder.WriteString("### Recorded-Evidence Blocker Status\n\n")
	builder.WriteString("_Blockers carried forward from recorded review evidence; findings this gate run raised are under `## Blocking Findings` above._\n\n")
	builder.WriteString(blockerList(input.Blockers))
	builder.WriteString("\n\n")
	if input.CurrentReview != nil {
		builder.WriteString("### Current PR Review\n\n")
		builder.WriteString(reviewList([]ReviewEvidence{*input.CurrentReview}, "No current review evidence."))
		builder.WriteString("\n\n")
	}
	builder.WriteString("### External GitHub Review Context\n\n")
	builder.WriteString(githubcontext.RenderMarkdown(input.GitHub))
	return builder.String()
}

func reviewList(reviews []ReviewEvidence, empty string) string {
	if len(reviews) == 0 {
		return empty
	}
	lines := make([]string, 0, len(reviews))
	for _, review := range reviews {
		status := review.Verdict
		switch {
		case review.Resolved:
			// The review's blockers were cleared out of band; render it historical
			// with its resolver so it agrees with a clear blocker-status section.
			status += " (historical; resolved: " + firstNonEmpty(review.Resolution, "no longer blocking") + ")"
		case review.HasUnresolvedBlockers:
			status += " with unresolved blockers"
		}
		status += attemptSummary(review)
		if review.BlockingFindingCount > 0 || review.AdvisoryFindingCount > 0 || review.FollowUpFindingCount > 0 {
			status += fmt.Sprintf(" findings: blocking %d, advisory %d, follow-up %d", review.BlockingFindingCount, review.AdvisoryFindingCount, review.FollowUpFindingCount)
		}
		line := "- " + firstNonEmpty(review.Target, "unknown") + ": " + firstNonEmpty(status, "unknown")
		if review.Path != "" {
			line += " (" + review.Path + ")"
		}
		if len(review.FindingIDs) > 0 {
			line += " findings: " + strings.Join(review.FindingIDs, ", ")
		}
		lines = append(lines, githubcontext.Redact(line))
	}
	return strings.Join(lines, "\n")
}

// attemptSummary renders the attempt counter, never emitting a bare "N/M" with
// N>M (#40): an over-limit counter is only valid with an explicit override or a
// corrected maximum, so above the limit it is always annotated.
func attemptSummary(review ReviewEvidence) string {
	if review.AttemptNumber <= 0 || review.MaxAttempts <= 0 {
		return ""
	}
	if review.AttemptNumber <= review.MaxAttempts {
		return fmt.Sprintf(" attempt %d/%d", review.AttemptNumber, review.MaxAttempts)
	}
	note := review.AttemptNote
	if note == "" {
		note = "exceeds recorded limit; no override recorded"
	}
	return fmt.Sprintf(" attempt %d (%s)", review.AttemptNumber, note)
}

// indexFindings keys the ledger by finding ID for reconciliation lookups.
func indexFindings(records []findings.Record) map[string]findings.Record {
	byID := make(map[string]findings.Record, len(records))
	for _, record := range records {
		if record.ID != "" {
			byID[record.ID] = record
		}
	}
	return byID
}

// reconcileReviews annotates each review against the current finding ledger so
// the report's historical task/epic summary agrees with its blocker-status
// section (#40). Point-in-time review logs stay immutable; only the emitted
// evidence is reconciled.
func reconcileReviews(reviews []ReviewEvidence, byID map[string]findings.Record) []ReviewEvidence {
	if len(reviews) == 0 {
		return reviews
	}
	out := make([]ReviewEvidence, len(reviews))
	for i, review := range reviews {
		out[i] = reconcileReview(review, byID)
	}
	return out
}

func reconcileReview(review ReviewEvidence, byID map[string]findings.Record) ReviewEvidence {
	resolvers, anyBlocking := classifyReviewFindings(review.FindingIDs, byID)
	// A review flagged with unresolved blockers is historical when it raised at
	// least one blocker-class finding that has since been cleared and NONE of its
	// blocker-class findings still block. Only blocker-class findings count, and
	// with the SAME predicate the blocker-status section uses (findings.
	// IsBlockingClass): a still-open ADVISORY on the review must not keep it
	// "with unresolved blockers" while the blocker-status section reads clear —
	// that mismatch is the very contradiction #40 removes.
	if review.HasUnresolvedBlockers && !anyBlocking && len(resolvers) > 0 {
		review.Resolved = true
		review.HasUnresolvedBlockers = false
		review.Resolution = strings.Join(dedupeStrings(resolvers), "; ")
	}
	// An over-limit attempt counter is only valid with an explicit resolver
	// (override/fix) among the review's blocker-class findings; annotate it with
	// that reference so the counter never contradicts the recorded maximum. Absent
	// a resolver, attemptSummary emits the generic "exceeds recorded limit" note.
	if review.AttemptNumber > review.MaxAttempts && review.MaxAttempts > 0 && len(resolvers) > 0 {
		review.AttemptNote = fmt.Sprintf("over the recorded limit of %d; %s", review.MaxAttempts, strings.Join(dedupeStrings(resolvers), "; "))
	}
	return review
}

// classifyReviewFindings inspects the blocker-class ledger rows a review
// references. anyBlocking reports whether any of them still holds the gate
// closed; resolvers describes how the cleared ones were resolved (override grant
// or fix). Non-blocker-class findings (advisory, follow-up, warning) are ignored:
// they never appear in the blocker-status section, so they must not sway whether
// the review reads as still-blocking.
func classifyReviewFindings(findingIDs []string, byID map[string]findings.Record) (resolvers []string, anyBlocking bool) {
	for _, id := range findingIDs {
		record, ok := byID[id]
		if !ok || !findings.IsBlockingClass(record) {
			continue
		}
		if findings.Blocks(record.Status) {
			anyBlocking = true
			continue
		}
		resolvers = append(resolvers, resolverPhrase(record))
	}
	return resolvers, anyBlocking
}

// resolverPhrase describes how a no-longer-blocking finding was cleared. Free
// text (grantor, reason, run id) is run through markdown.PlainText so a value
// carrying newlines or control characters cannot break out of its bullet and
// inject headings or list items into a report that may be posted to a PR.
func resolverPhrase(record findings.Record) string {
	switch record.Status {
	case findings.StatusOverridden:
		phrase := "override granted"
		if by := markdown.PlainText(record.OverrideGrantedBy); by != "" {
			phrase += " by " + by
		}
		if reason := markdown.PlainText(record.OverrideGrantReason); reason != "" {
			phrase += " (" + reason + ")"
		}
		return phrase
	case findings.StatusSuperseded:
		return "superseded"
	default:
		// Fixed, or any other non-blocking terminal status.
		if runID := markdown.PlainText(record.FixedInRunID); runID != "" {
			return "fixed in run " + runID
		}
		return "resolved"
	}
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func blockerList(blockers []Blocker) string {
	if len(blockers) == 0 {
		return "No unresolved blockers in the recorded review evidence."
	}
	lines := make([]string, 0, len(blockers))
	for _, blocker := range blockers {
		line := "- " + firstNonEmpty(blocker.ID, "unknown") + ": " + firstNonEmpty(blocker.Title, "untitled")
		if blocker.Status != "" {
			line += " [" + blocker.Status + "]"
		}
		lines = append(lines, githubcontext.Redact(line))
	}
	return strings.Join(lines, "\n")
}

func list(values []string, empty string) string {
	if len(values) == 0 {
		return empty
	}
	lines := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		lines = append(lines, "- "+githubcontext.Redact(strings.TrimSpace(value)))
	}
	if len(lines) == 0 {
		return empty
	}
	return strings.Join(lines, "\n")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
