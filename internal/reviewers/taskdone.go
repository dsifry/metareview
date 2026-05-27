package reviewers

import (
	"regexp"
	"sort"
	"strings"

	"github.com/dsifry/metareview/internal/findings"
)

type Finding = findings.Input

type Context struct {
	Task         TaskContext
	Git          GitContext
	Knowledge    KnowledgeContext
	EvidenceText string
}

type TaskContext struct {
	Type string
	ID   string
	Text string
}

type GitContext struct {
	ChangedFiles             []string
	StagedFiles              []string
	UnstagedFiles            []string
	WorkingTreeFiles         []string
	UntrackedFiles           []string
	Diff                     string
	StagedDiff               string
	WorkingTreeDiff          string
	UntrackedExcerpts        string
	DiffTruncated            bool
	StagedDiffTruncated      bool
	WorkingTreeDiffTruncated bool
}

type KnowledgeContext struct {
	ServiceInventoryPath string
	ServiceInventory     string
	Facts                []KnowledgeFact
}

type KnowledgeFact struct {
	Source string
	Text   string
}

var evalPattern = regexp.MustCompile(`\beval\s*\(`)
var todoPattern = regexp.MustCompile(`(?i)\b(TODO|FIXME)\b`)
var validationPattern = regexp.MustCompile(`(?i)tests?.*(pass|passed|exited 0|ok)`)
var inventoryPathPattern = regexp.MustCompile(`[A-Za-z0-9_./-]+\.(go|js|ts|tsx|jsx|py|rb)`)

func RunTaskDone(context Context) []Finding {
	var results []Finding
	lines := addedLines(context.Git)
	changedSource := sourceFiles(context.Git)

	if line := firstMatching(lines, evalPattern); line != "" {
		results = append(results, finding(Finding{
			Reviewer:       "security-reviewer",
			Severity:       "critical",
			Title:          "Unsafe eval introduced",
			Finding:        "The diff introduces eval on runtime input.",
			Expected:       "Code must parse or dispatch data without executing user-controlled strings.",
			Found:          strings.TrimPrefix(line, "+"),
			Evidence:       []findings.Evidence{{Type: "diff-pattern", Path: "eval("}},
			Recommendation: "Replace eval with a parser, lookup table, or explicit command dispatch.",
			Fingerprint:    "security:eval",
		}))
	}

	if len(changedSource) > 0 && !hasTestChange(context.Git) && !validationPattern.MatchString(context.EvidenceText) {
		sortedSource := append([]string{}, changedSource...)
		sort.Strings(sortedSource)
		results = append(results, finding(Finding{
			Reviewer:       "test-reviewer",
			Severity:       "high",
			Title:          "Missing test changes or validation evidence",
			Finding:        "Source code changed without corresponding test files or validation evidence.",
			Expected:       "Claimed task-done work includes relevant tests or explicit validation output.",
			Found:          "Changed source files: " + strings.Join(sortedSource, ", "),
			Evidence:       []findings.Evidence{{Type: "changed-files"}},
			Recommendation: "Add focused tests or attach validation evidence with --evidence.",
			Fingerprint:    "tests:missing:" + strings.Join(sortedSource, "|"),
		}))
	}

	if line := firstMatching(lines, todoPattern); line != "" {
		results = append(results, finding(Finding{
			Reviewer:       "code-quality-reviewer",
			Severity:       "high",
			Title:          "TODO left in task-done diff",
			Finding:        "The task claims done while the diff adds TODO/FIXME work markers.",
			Expected:       "Task-done diffs do not introduce unresolved implementation markers.",
			Found:          strings.TrimPrefix(line, "+"),
			Evidence:       []findings.Evidence{{Type: "diff-pattern", Path: "TODO|FIXME"}},
			Recommendation: "Complete the work or convert the remaining work into an explicit follow-up.",
			Fingerprint:    "quality:todo",
		}))
	}

	if context.Git.DiffTruncated || context.Git.StagedDiffTruncated || context.Git.WorkingTreeDiffTruncated {
		results = append(results, finding(Finding{
			Reviewer:       "architecture-reviewer",
			Severity:       "high",
			Title:          "Diff context was truncated",
			Finding:        "The reviewer did not receive the full diff, so task closure cannot be trusted.",
			Expected:       "Large diffs are decomposed or reviewed with complete context.",
			Found:          "Diff exceeded metareview context limit.",
			Evidence:       []findings.Evidence{{Type: "context", Path: "diffTruncated"}},
			Recommendation: "Split the task or raise the review context limit deliberately.",
			Fingerprint:    "architecture:truncated-diff",
		}))
	}

	results = append(results, duplicatePathFindings(context.Knowledge, changedSource)...)
	return results
}

func finding(input Finding) Finding {
	input.Classification = "blocking"
	if input.Owner == "" {
		input.Owner = "implementer"
	}
	return input
}

func duplicatePathFindings(knowledge KnowledgeContext, changedSource []string) []Finding {
	if knowledge.ServiceInventory == "" {
		return nil
	}
	inventoryPaths := inventoryPathPattern.FindAllString(knowledge.ServiceInventory, -1)
	var results []Finding
	for _, changed := range changedSource {
		changedToken := normalizedPathToken(changed)
		for _, existing := range inventoryPaths {
			if existing == changed {
				continue
			}
			if normalizedPathToken(existing) != changedToken {
				continue
			}
			results = append(results, finding(Finding{
				Reviewer:       "architecture-reviewer",
				Severity:       "high",
				Title:          "Possible duplicate code path",
				Finding:        "The diff adds or changes a path that appears to duplicate an inventoried service path.",
				Expected:       "Task-done changes reuse or deliberately modify the existing service path.",
				Found:          changed + " resembles " + existing,
				Evidence:       []findings.Evidence{{Type: "service-inventory", Path: knowledge.ServiceInventoryPath}},
				Recommendation: "Use the existing service path or document the intentional split in the service inventory.",
				Fingerprint:    "architecture:duplicate-path:" + existing + ":" + changed,
			}))
			break
		}
	}
	return results
}

func allFiles(git GitContext) []string {
	files := []string{}
	files = append(files, git.ChangedFiles...)
	files = append(files, git.StagedFiles...)
	files = append(files, git.UnstagedFiles...)
	files = append(files, git.WorkingTreeFiles...)
	files = append(files, git.UntrackedFiles...)
	return uniqueStrings(files)
}

func sourceFiles(git GitContext) []string {
	var files []string
	for _, file := range allFiles(git) {
		if strings.HasPrefix(file, "lib/") ||
			strings.HasPrefix(file, "cli/") ||
			strings.HasPrefix(file, "src/") ||
			strings.HasPrefix(file, "cmd/") ||
			strings.HasPrefix(file, "internal/") {
			files = append(files, file)
		}
	}
	return files
}

func hasTestChange(git GitContext) bool {
	for _, file := range allFiles(git) {
		lower := strings.ToLower(file)
		if strings.HasPrefix(lower, "tests/") ||
			strings.Contains(lower, "_test.") ||
			strings.Contains(lower, ".test.") ||
			strings.Contains(lower, ".spec.") {
			return true
		}
	}
	return false
}

func addedLines(git GitContext) []string {
	text := strings.Join([]string{git.Diff, git.StagedDiff, git.WorkingTreeDiff, git.UntrackedExcerpts}, "\n")
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			lines = append(lines, line)
		}
	}
	return lines
}

func firstMatching(lines []string, pattern *regexp.Regexp) string {
	for _, line := range lines {
		if pattern.MatchString(line) {
			return line
		}
	}
	return ""
}

func normalizedPathToken(file string) string {
	lower := strings.ToLower(file)
	lower = regexp.MustCompile(`\.[a-z0-9]+$`).ReplaceAllString(lower, "")
	lower = regexp.MustCompile(`v\d+|new|copy|duplicate`).ReplaceAllString(lower, "")
	lower = regexp.MustCompile(`[^a-z0-9]`).ReplaceAllString(lower, "")
	return lower
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	var result []string
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}
