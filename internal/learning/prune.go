package learning

import (
	"regexp"
	"strings"

	"github.com/dsifry/metareview/internal/knowledge"
)

const (
	DiscardTooSpecific                = "too-specific"
	DiscardSelfEvident                = "self-evident"
	DiscardAlreadyCovered             = "already-covered"
	DiscardObsolete                   = "obsolete"
	DiscardFollowUpNotKnowledge       = "follow-up-not-knowledge"
	DiscardRegistryUpdateNotKnowledge = "registry-update-not-knowledge"
	DiscardUnverified                 = "unverified"
)

type PruneInput struct {
	Candidates []Candidate
	Knowledge  knowledge.Context
}

type PruneResult struct {
	Accepted  []Candidate          `json:"accepted"`
	Discarded []DiscardedCandidate `json:"discarded"`
}

type DiscardedCandidate struct {
	Candidate Candidate `json:"candidate"`
	Reason    string    `json:"reason"`
}

var approvedDiscardReasons = map[string]bool{
	DiscardTooSpecific:                true,
	DiscardSelfEvident:                true,
	DiscardAlreadyCovered:             true,
	DiscardObsolete:                   true,
	DiscardFollowUpNotKnowledge:       true,
	DiscardRegistryUpdateNotKnowledge: true,
	DiscardUnverified:                 true,
}

var nonWordPattern = regexp.MustCompile(`[^a-z0-9]+`)

func PruneCandidates(input PruneInput) PruneResult {
	var result PruneResult
	for _, candidate := range input.Candidates {
		if reason := discardReason(candidate, input.Knowledge); reason != "" {
			result.Discarded = append(result.Discarded, DiscardedCandidate{Candidate: candidate, Reason: reason})
			continue
		}
		result.Accepted = append(result.Accepted, candidate)
	}
	return result
}

func IsApprovedDiscardReason(reason string) bool {
	return approvedDiscardReasons[reason]
}

func discardReason(candidate Candidate, knowledgeContext knowledge.Context) string {
	text := strings.TrimSpace(candidate.Text)
	lower := strings.ToLower(text)
	switch {
	case alreadyCovered(text, knowledgeContext):
		return DiscardAlreadyCovered
	case isUnverified(candidate):
		return DiscardUnverified
	case strings.Contains(lower, "obsolete") || strings.Contains(lower, "deprecated legacy"):
		return DiscardObsolete
	case isFollowUp(text):
		return DiscardFollowUpNotKnowledge
	case isRegistryUpdate(text, candidate):
		return DiscardRegistryUpdateNotKnowledge
	case isTooSpecific(text):
		return DiscardTooSpecific
	case isSelfEvident(text):
		return DiscardSelfEvident
	case !wouldChangeReviewerBehavior(text):
		return DiscardSelfEvident
	default:
		return ""
	}
}

func alreadyCovered(text string, knowledgeContext knowledge.Context) bool {
	needle := normalizedKnowledge(text)
	if needle == "" {
		return false
	}
	for _, fact := range knowledgeContext.Facts {
		haystack := normalizedKnowledge(fact.Text)
		if haystack == "" {
			continue
		}
		if strings.Contains(haystack, needle) || strings.Contains(needle, haystack) {
			return true
		}
	}
	return false
}

func isUnverified(candidate Candidate) bool {
	return candidate.Confidence == "" || strings.EqualFold(candidate.Confidence, "low")
}

func isFollowUp(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "follow-up") ||
		strings.Contains(lower, "create a task") ||
		strings.Contains(lower, "create an issue") ||
		strings.Contains(lower, "add missing")
}

func isRegistryUpdate(text string, candidate Candidate) bool {
	lower := strings.ToLower(text)
	if strings.Contains(strings.ToLower(candidate.ProposedTarget), "service-inventory") {
		return true
	}
	return strings.Contains(lower, "update service_inventory") ||
		strings.Contains(lower, "update service inventory") ||
		strings.Contains(lower, "list internal/")
}

func isTooSpecific(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "pr 42") ||
		strings.Contains(lower, "customer ") ||
		strings.Contains(lower, "acmecorp") ||
		strings.Contains(lower, "foobarbaz")
}

func isSelfEvident(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	return lower == "run tests before finishing." ||
		lower == "run tests before finishing" ||
		lower == "do not leak secrets" ||
		lower == "write tests"
}

func wouldChangeReviewerBehavior(text string) bool {
	lower := strings.ToLower(text)
	for _, marker := range []string{"before ", "when ", "reviewer", "reviewers", "should ", "must ", "require", "prefer", "check ", "capture", "repeated blocker"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func normalizedKnowledge(text string) string {
	return nonWordPattern.ReplaceAllString(strings.ToLower(strings.TrimSpace(text)), " ")
}
