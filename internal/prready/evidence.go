package prready

import (
	"strings"

	"github.com/dsifry/metareview/internal/githubcontext"
	"github.com/dsifry/metareview/internal/reviewlog"
)

type EvidenceInput struct {
	Summary     string
	Validation  []string
	TaskReviews []ReviewEvidence
	EpicReviews []ReviewEvidence
	Blockers    []Blocker
	GitHub      githubcontext.Context
}

type ReviewEvidence struct {
	Target                string
	Verdict               string
	Path                  string
	FindingIDs            []string
	HasUnresolvedBlockers bool
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
	}
}

func RenderEvidence(input EvidenceInput) string {
	var builder strings.Builder
	builder.WriteString("## metareview PR Evidence\n\n")
	builder.WriteString("### Summary\n\n")
	builder.WriteString(firstNonEmpty(githubcontext.Redact(input.Summary), "No PR summary supplied."))
	builder.WriteString("\n\n")
	builder.WriteString("### Validation\n\n")
	builder.WriteString(list(input.Validation, "No validation evidence supplied."))
	builder.WriteString("\n\n")
	builder.WriteString("### Task Review Evidence\n\n")
	builder.WriteString(reviewList(input.TaskReviews, "No task review evidence discovered."))
	builder.WriteString("\n\n")
	builder.WriteString("### Epic Review Evidence\n\n")
	builder.WriteString(reviewList(input.EpicReviews, "No epic review evidence discovered."))
	builder.WriteString("\n\n")
	builder.WriteString("### Blocker Status\n\n")
	builder.WriteString(blockerList(input.Blockers))
	builder.WriteString("\n\n")
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
		if review.HasUnresolvedBlockers {
			status += " with unresolved blockers"
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

func blockerList(blockers []Blocker) string {
	if len(blockers) == 0 {
		return "No unresolved blockers discovered."
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
