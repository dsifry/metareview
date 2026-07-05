package reviewers

import (
	"sort"
	"strings"

	"github.com/dsifry/metareview/internal/evidence"
	"github.com/dsifry/metareview/internal/findings"
)

type PRReadyContext struct {
	Git                   GitContext
	Knowledge             KnowledgeContext
	EvidenceText          string
	PREvidenceMarkdown    string
	ReviewLogs            []PRReviewLog
	GitHub                PRGitHubContext
	IncludeWorkingTree    bool
	WorkingTreeDirtyFiles []string
}

type PRReviewLog struct {
	Target                string
	Verdict               string
	FindingIDs            []string
	HasUnresolvedBlockers bool
}

type PRGitHubContext struct {
	Available         bool
	UnavailableReason string
	Entries           []PRGitHubEntry
}

type PRGitHubEntry struct {
	Author string
	URL    string
	State  string
	Body   string
}

func RunPRReady(context PRReadyContext) []Finding {
	var results []Finding
	if !context.IncludeWorkingTree && len(context.WorkingTreeDirtyFiles) > 0 {
		results = append(results, finding(Finding{
			Reviewer:       "pr-readiness-reviewer",
			Severity:       "high",
			Title:          "Working tree changes excluded from PR-ready review",
			Finding:        "PR-ready reviews the committed branch diff by default, but non-generated staged, unstaged, or untracked files are present.",
			Expected:       "PR-ready runs on a clean working tree, or the caller explicitly opts into reviewing local dirt with --include-working-tree.",
			Found:          "Dirty files: " + strings.Join(context.WorkingTreeDirtyFiles, ", "),
			Evidence:       []findings.Evidence{{Type: "git-status"}},
			Recommendation: "Commit, stash, or remove the local files, or rerun with --include-working-tree when the local changes are intentionally part of the review.",
			Fingerprint:    "pr:working-tree-dirty:" + strings.Join(context.WorkingTreeDirtyFiles, "|"),
		}))
	}
	if blocked := unresolvedPRReviewTargets(context.ReviewLogs); len(blocked) > 0 {
		results = append(results, finding(Finding{
			Reviewer:       "pr-readiness-reviewer",
			Severity:       "high",
			Title:          "Unresolved review blockers",
			Finding:        "Task, epic, or prior review evidence still contains unresolved blockers.",
			Expected:       "All task and epic review blockers are resolved before PR readiness.",
			Found:          "Blocked targets: " + strings.Join(blocked, ", "),
			Evidence:       []findings.Evidence{{Type: "review-log"}},
			Recommendation: "Resolve blockers and re-run the relevant task or epic review before PR-ready.",
			Fingerprint:    "pr:unresolved-review-blockers:" + strings.Join(blocked, "|"),
		}))
	}
	if !hasPRValidationEvidence(context.EvidenceText) {
		results = append(results, finding(Finding{
			Reviewer:       "validation-reviewer",
			Severity:       "high",
			Title:          "Missing validation evidence",
			Finding:        "PR readiness requires explicit validation output.",
			Expected:       "Validation evidence records the relevant test or verification command and passing result.",
			Found:          "No passing validation evidence was supplied.",
			Evidence:       []findings.Evidence{{Type: "validation-evidence"}},
			Recommendation: "Run the relevant verification command and pass its output through PR-ready evidence.",
			Fingerprint:    "pr:missing-validation-evidence",
		}))
	}
	if !hasReadablePREvidence(context.PREvidenceMarkdown) {
		results = append(results, finding(Finding{
			Reviewer:       "pr-readiness-reviewer",
			Severity:       "high",
			Title:          "Missing PR evidence section",
			Finding:        "The PR-ready package lacks a readable metareview evidence section.",
			Expected:       "PR evidence includes summary, validation, review evidence, blocker status, and review links.",
			Found:          "No readable `metareview PR Evidence` section was supplied.",
			Evidence:       []findings.Evidence{{Type: "pr-evidence"}},
			Recommendation: "Render and include the metareview PR evidence section before PR-ready passes.",
			Fingerprint:    "pr:missing-evidence-section",
		}))
	}
	results = append(results, branchDiffFindings(context)...)
	results = append(results, externalGitHubFindings(context.GitHub)...)
	return results
}

func hasPRValidationEvidence(text string) bool {
	bundle, err := evidence.Parse([]byte(text))
	if err != nil {
		return false
	}
	return bundle.HasSuccessfulValidation(evidence.KindGeneric)
}

func unresolvedPRReviewTargets(logs []PRReviewLog) []string {
	var blocked []string
	for _, log := range logs {
		if log.HasUnresolvedBlockers || log.Verdict == "NEEDS_REVISION" {
			blocked = append(blocked, firstNonEmpty(log.Target, "unknown"))
		}
	}
	sort.Strings(blocked)
	return blocked
}

func hasReadablePREvidence(markdown string) bool {
	text := strings.ToLower(markdown)
	return strings.Contains(text, "metareview pr evidence") &&
		strings.Contains(text, "validation")
}

func branchDiffFindings(context PRReadyContext) []Finding {
	raw := RunTaskDone(Context{
		Task:         TaskContext{Type: "branch", ID: "pr-ready", Text: "PR-ready branch review"},
		Git:          context.Git,
		Knowledge:    context.Knowledge,
		EvidenceText: context.EvidenceText,
	})
	filtered := make([]Finding, 0, len(raw))
	for _, item := range raw {
		if strings.HasPrefix(item.Fingerprint, "tests:missing:") {
			continue
		}
		item.Fingerprint = "pr:" + item.Fingerprint
		filtered = append(filtered, item)
	}
	return filtered
}

func externalGitHubFindings(context PRGitHubContext) []Finding {
	if !context.Available {
		return nil
	}
	var results []Finding
	for _, entry := range context.Entries {
		text := strings.ToUpper(entry.State + "\n" + entry.Body)
		if !strings.Contains(text, "CHANGES_REQUESTED") &&
			!strings.Contains(text, "REQUEST_CHANGES") &&
			!strings.Contains(text, "BLOCKER") {
			continue
		}
		results = append(results, finding(Finding{
			Reviewer:       "external-reviewer",
			Severity:       "high",
			Title:          "External GitHub review blocker",
			Finding:        "GitHub review context includes a blocking external review signal.",
			Expected:       "External review blockers are resolved or explicitly dispositioned before PR-ready passes.",
			Found:          firstNonEmpty(entry.State, entry.URL, entry.Author, "blocking external review"),
			Evidence:       []findings.Evidence{{Type: "github-review", Path: entry.URL}},
			Recommendation: "Resolve the external review comment or document why it is no longer applicable.",
			Fingerprint:    "pr:github-external-blocker:" + firstNonEmpty(entry.URL, entry.Author, "unknown"),
		}))
	}
	return results
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
