package learning

import (
	"regexp"
	"sort"
	"strings"

	"github.com/dsifry/metareview/internal/findings"
	"github.com/dsifry/metareview/internal/githubcontext"
	"github.com/dsifry/metareview/internal/sessionhistory"
)

type Input struct {
	Findings []findings.Record
	GitHub   githubcontext.Context
	Session  sessionhistory.Context
}

type Result struct {
	Knowledge   []Candidate `json:"knowledge"`
	Calibration []Candidate `json:"calibration"`
}

type Candidate struct {
	Kind           string      `json:"kind"`
	Text           string      `json:"text"`
	Scope          string      `json:"scope"`
	Provenance     string      `json:"provenance"`
	SourceRefs     []SourceRef `json:"sourceRefs"`
	Confidence     string      `json:"confidence"`
	ProposedTarget string      `json:"proposedTarget"`
	Disposition    string      `json:"disposition,omitempty"`
}

type SourceRef struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`
	Path string `json:"path,omitempty"`
	URL  string `json:"url,omitempty"`
}

var pathPattern = regexp.MustCompile(`\b[\w./-]+\.(go|js|ts|tsx|jsx|py|rb|json|md)\b`)

func ExtractCandidates(input Input) Result {
	var result Result
	result.Knowledge = append(result.Knowledge, knowledgeFromFindings(input.Findings)...)
	result.Knowledge = append(result.Knowledge, repeatedBlockerThemes(input.Findings)...)
	result.Knowledge = append(result.Knowledge, githubReviewBlockers(input.GitHub)...)
	result.Knowledge = append(result.Knowledge, sessionCorrections(input.Session)...)
	result.Calibration = append(result.Calibration, calibrationFromFindings(input.Findings)...)
	return result
}

func knowledgeFromFindings(records []findings.Record) []Candidate {
	var candidates []Candidate
	for _, record := range records {
		if record.KnowledgeCandidate {
			candidates = append(candidates, Candidate{
				Kind:           "knowledge-candidate",
				Text:           generalized(firstNonEmpty(record.Recommendation, record.Expected, record.Finding, record.Title)),
				Scope:          "repository-review",
				Provenance:     "review finding marked as knowledge candidate",
				SourceRefs:     findingRefs(record),
				Confidence:     confidenceForFinding(record),
				ProposedTarget: "repository-knowledge",
			})
		}
		if record.Status == "fixed" && record.FixedInRunID != "" {
			candidates = append(candidates, Candidate{
				Kind:           "fixed-review-defect",
				Text:           generalized("Capture review-driven fix: " + firstNonEmpty(record.Recommendation, record.Expected, record.Finding, record.Title)),
				Scope:          "repository-review",
				Provenance:     "review finding fixed in a later run",
				SourceRefs:     append(findingRefs(record), SourceRef{Type: "fixed-run", ID: record.FixedInRunID}),
				Confidence:     "high",
				ProposedTarget: "repository-knowledge",
			})
		}
	}
	return compactCandidates(candidates)
}

func repeatedBlockerThemes(records []findings.Record) []Candidate {
	groups := map[string][]findings.Record{}
	for _, record := range records {
		if !isBlocking(record) {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(record.Title))
		if key == "" {
			key = strings.ToLower(strings.TrimSpace(record.Fingerprint))
		}
		if key == "" {
			continue
		}
		groups[key] = append(groups[key], record)
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var candidates []Candidate
	for _, key := range keys {
		group := groups[key]
		if len(group) < 2 {
			continue
		}
		sourceRefs := make([]SourceRef, 0, len(group))
		for _, record := range group {
			sourceRefs = append(sourceRefs, findingRefs(record)...)
		}
		candidates = append(candidates, Candidate{
			Kind:           "repeated-blocker-theme",
			Text:           generalized("Repeated blocker theme: " + firstNonEmpty(group[0].Title, group[0].Finding)),
			Scope:          "repository-review",
			Provenance:     "same blocker theme appeared in multiple review findings",
			SourceRefs:     sourceRefs,
			Confidence:     "medium",
			ProposedTarget: "repository-knowledge",
		})
	}
	return compactCandidates(candidates)
}

func githubReviewBlockers(ctx githubcontext.Context) []Candidate {
	if !ctx.Available {
		return nil
	}
	var candidates []Candidate
	for _, review := range ctx.Reviews {
		body := strings.TrimSpace(review.Body)
		state := strings.ToUpper(strings.TrimSpace(review.State))
		if body == "" || (state != "CHANGES_REQUESTED" && !containsBlockerLanguage(body)) {
			continue
		}
		candidates = append(candidates, Candidate{
			Kind:           "github-review-blocker",
			Text:           generalized("GitHub review blocker: " + body),
			Scope:          "repository-review",
			Provenance:     "external GitHub review context",
			SourceRefs:     []SourceRef{{Type: "github-review", URL: review.URL, ID: ctx.PRNumber}},
			Confidence:     "medium",
			ProposedTarget: "repository-knowledge",
		})
	}
	return compactCandidates(candidates)
}

func sessionCorrections(ctx sessionhistory.Context) []Candidate {
	if !ctx.Available {
		return nil
	}
	var candidates []Candidate
	for _, signal := range ctx.Signals {
		text := strings.TrimSpace(signal.Excerpt)
		if text == "" || !containsCorrectionLanguage(text) {
			continue
		}
		candidates = append(candidates, Candidate{
			Kind:           "session-derived-correction",
			Text:           generalized(text),
			Scope:          "repository-review",
			Provenance:     "bounded session-history signal",
			SourceRefs:     []SourceRef{{Type: signal.SourceType, Path: signal.Path}},
			Confidence:     signal.Confidence,
			ProposedTarget: "repository-knowledge",
		})
	}
	return compactCandidates(candidates)
}

func calibrationFromFindings(records []findings.Record) []Candidate {
	var candidates []Candidate
	for _, record := range records {
		disposition := calibrationDisposition(record)
		if disposition == "" {
			continue
		}
		candidates = append(candidates, Candidate{
			Kind:           "reviewer-calibration",
			Text:           generalized(firstNonEmpty(record.Finding, record.Title)),
			Scope:          "reviewer-calibration",
			Provenance:     "finding disposition should calibrate future reviewer behavior",
			SourceRefs:     findingRefs(record),
			Confidence:     "medium",
			ProposedTarget: "reviewer-calibration",
			Disposition:    disposition,
		})
	}
	return compactCandidates(candidates)
}

func calibrationDisposition(record findings.Record) string {
	status := strings.ToLower(strings.TrimSpace(record.Status))
	classification := strings.ToLower(strings.TrimSpace(record.Classification))
	title := strings.ToLower(record.Title + " " + record.Finding)
	for _, value := range []string{status, classification} {
		switch value {
		case "false-positive", "rebutted", "accepted-risk":
			return value
		}
	}
	switch {
	case strings.Contains(title, "false positive"):
		return "false-positive"
	case strings.Contains(title, "rebutted"):
		return "rebutted"
	case strings.Contains(title, "accepted risk"):
		return "accepted-risk"
	default:
		return ""
	}
}

func isBlocking(record findings.Record) bool {
	if record.Status != "open" {
		return false
	}
	return record.Classification == "spec-contract" ||
		(record.Classification == "blocking" && (record.Severity == "critical" || record.Severity == "high"))
}

func confidenceForFinding(record findings.Record) string {
	if record.Status == "fixed" || record.FixedInRunID != "" {
		return "high"
	}
	return "medium"
}

func findingRefs(record findings.Record) []SourceRef {
	ref := SourceRef{Type: "finding", ID: record.ID}
	return []SourceRef{ref}
}

func containsBlockerLanguage(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "blocker") || strings.Contains(lower, "must") || strings.Contains(lower, "do not")
}

func containsCorrectionLanguage(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "correction") || strings.Contains(lower, "fixed") || strings.Contains(lower, "instead of")
}

func generalized(text string) string {
	text = githubcontext.Redact(text)
	text = pathPattern.ReplaceAllString(text, "[path]")
	text = strings.Join(strings.Fields(text), " ")
	return strings.TrimSpace(text)
}

func compactCandidates(candidates []Candidate) []Candidate {
	out := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.Text) == "" || len(candidate.SourceRefs) == 0 {
			continue
		}
		out = append(out, candidate)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
