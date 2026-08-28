package reviewers

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/dsifry/metareview/internal/evidence"
	"github.com/dsifry/metareview/internal/findings"
)

type Finding = findings.Input

type Context struct {
	Task         TaskContext
	Git          GitContext
	Knowledge    KnowledgeContext
	Manifest     ManifestContext
	EvidenceText string
}

// ManifestContext is what the review manifest says about the shard results
// ingested for this run.
type ManifestContext struct {
	Present       bool
	Verdict       string
	Blockers      []string
	ShardCount    int
	ShardsCovered int
	CrossShard    bool
	PlanHash      string
}

// shardSatisfiableReasons are the only context-risk reasons a set of shard
// reviews can answer: the bytes a shard pack does not carry are never covered.
var shardSatisfiableReasons = map[string]bool{"DIFF_TRUNCATED": true, "LARGE_DIFF": true}

// Satisfies reports whether the shard results cover the whole context risk.
func (m ManifestContext) Satisfies(reasons []string) bool {
	for _, reason := range reasons {
		if !shardSatisfiableReasons[reason] {
			return false
		}
	}
	return m.Present &&
		m.ShardCount > 0 &&
		m.ShardsCovered == m.ShardCount &&
		(m.ShardCount == 1 || m.CrossShard) &&
		m.Verdict == "PASS"
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
	BranchDiffFull           string
	StagedDiff               string
	WorkingTreeDiff          string
	UntrackedExcerpts        string
	DiffTruncated            bool
	StagedDiffTruncated      bool
	WorkingTreeDiffTruncated bool
	RawDiffBytes             int
	FilteredDiffBytes        int
	GeneratedExcludedFiles   []string
	UntrackedOmittedCount    int
	RiskLevel                string
	RiskReasons              []string
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
var inventoryPathPattern = regexp.MustCompile(`[A-Za-z0-9_./-]+\.(go|js|ts|tsx|jsx|py|rb)`)

func RunTaskDone(context Context) []Finding {
	var results []Finding
	sharded := false
	if context.Git.RiskLevel == "context-risk" {
		if !context.Manifest.Satisfies(context.Git.RiskReasons) {
			return append(results, finding(Finding{
				Reviewer:       "architecture-reviewer",
				Severity:       "high",
				Title:          "Review context risk",
				Finding:        "The reviewer did not receive complete or bounded source context, so task closure cannot be trusted.",
				Expected:       "Large or incomplete review contexts are split, sharded, or rerun with complete source context before task closure.",
				Found:          contextRiskFound(context.Git) + manifestFound(context.Manifest),
				Evidence:       []findings.Evidence{{Type: "context", Path: "contextProfile"}},
				Recommendation: "Split the task, use the generated shard plan, or rerun the review with complete context.",
				Fingerprint:    "architecture:context-risk",
			}))
		}
		sharded = true
		results = append(results, advisory(Finding{
			Reviewer:       "architecture-reviewer",
			Severity:       "medium",
			Title:          "Context risk covered by shard reviews",
			Finding:        "The diff exceeded the review context limit, and every shard of the current plan has a fresh passing review result.",
			Expected:       "An oversized diff is reviewed shard by shard, with a result for every shard of the current plan.",
			Found:          contextRiskCoveredFound(context.Manifest),
			Evidence:       []findings.Evidence{{Type: "context", Path: "reviewManifest"}},
			Recommendation: "No action: the shard reviews stand in for the context metareview could not hold.",
			Fingerprint:    "architecture:context-risk-covered",
		}))
	}
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

	if len(changedSource) > 0 && !hasTestChange(context.Git) && !hasSuccessfulValidationEvidence(context.EvidenceText) {
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
		truncated := finding(Finding{
			Reviewer:       "architecture-reviewer",
			Severity:       "high",
			Title:          "Diff context was truncated",
			Finding:        "The reviewer did not receive the full diff, so task closure cannot be trusted.",
			Expected:       "Large diffs are decomposed or reviewed with complete context.",
			Found:          "Diff exceeded metareview context limit.",
			Evidence:       []findings.Evidence{{Type: "context", Path: "diffTruncated"}},
			Recommendation: "Split the task or raise the review context limit deliberately.",
			Fingerprint:    "architecture:truncated-diff",
		})
		if sharded {
			truncated.Classification = "advisory"
		}
		results = append(results, truncated)
	}

	results = append(results, duplicatePathFindings(context.Knowledge, changedSource)...)
	return results
}

// manifestFound appends what the manifest said, capped at ten blockers.
func manifestFound(manifest ManifestContext) string {
	if manifest.ShardCount == 0 {
		return ""
	}
	parts := []string{"Manifest verdict: " + manifest.Verdict,
		"shards covered: " + intString(manifest.ShardsCovered) + " of " + intString(manifest.ShardCount)}
	if !manifest.Present {
		parts = append(parts, "no shard review results were ingested")
	}
	blockers := manifest.Blockers
	if len(blockers) > 10 {
		blockers = blockers[:10]
	}
	if len(blockers) > 0 {
		parts = append(parts, "manifest blockers: "+strings.Join(blockers, "; "))
	}
	return "; " + strings.Join(parts, "; ")
}

func contextRiskCoveredFound(manifest ManifestContext) string {
	return "Plan hash: " + manifest.PlanHash + "; shards covered: " +
		intString(manifest.ShardsCovered) + " of " + intString(manifest.ShardCount) +
		"; cross-shard review: " + boolString(manifest.CrossShard)
}

func boolString(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func contextRiskFound(git GitContext) string {
	var parts []string
	if len(git.RiskReasons) > 0 {
		parts = append(parts, "Reasons: "+strings.Join(git.RiskReasons, ", "))
	}
	if git.RawDiffBytes > 0 || git.FilteredDiffBytes > 0 {
		parts = append(parts, "Raw diff bytes: "+intString(git.RawDiffBytes)+", filtered diff bytes: "+intString(git.FilteredDiffBytes))
	}
	if git.UntrackedOmittedCount > 0 {
		parts = append(parts, "Untracked files omitted: "+intString(git.UntrackedOmittedCount))
	}
	if len(parts) == 0 {
		return "Context profile reported risk."
	}
	return strings.Join(parts, "; ")
}

func intString(value int) string {
	return strconv.Itoa(value)
}

func hasSuccessfulValidationEvidence(text string) bool {
	bundle, err := evidence.Parse([]byte(text))
	if err != nil {
		return false
	}
	return bundle.HasSuccessfulValidation(evidence.KindGeneric)
}

// advisory records a finding that reports rather than gates.
func advisory(input Finding) Finding {
	out := finding(input)
	out.Classification = "advisory"
	return out
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

// addedLines returns the added lines the deterministic lints scan.
//
// Documentation is deliberately excluded. The lints look for code defects — an
// unsafe dynamic-evaluation call, a leftover work marker — and a design document
// that *discusses* those things is not one. (This comment names them obliquely
// for that very reason: spelled out, it would trip the lints it describes.)
// Before the branch diff was measured untruncated, truncation
// hid most prose by accident; once the whole diff became visible, metareview's own
// specs about its lints started failing its own gate.
func addedLines(git GitContext) []string {
	// The branch part comes from the untruncated measured diff when there is one,
	// so the lints see the bytes the packs carry rather than the truncated view.
	branch := git.BranchDiffFull
	if branch == "" {
		branch = git.Diff
	}
	var lines []string
	for _, text := range []string{branch, git.StagedDiff, git.WorkingTreeDiff} {
		lines = append(lines, addedSourceLines(text)...)
	}
	// Untracked excerpts are "--- <path>" headed rather than a diff.
	lines = append(lines, addedUntrackedLines(git.UntrackedExcerpts)...)
	return lines
}

// addedSourceLines walks a unified diff, tracking the file each hunk belongs to and
// dropping the ones the lints should not judge.
func addedSourceLines(text string) []string {
	var lines []string
	scan := true
	// A "+++ " line only names a file while we are in a file's header, before
	// its first hunk. Honouring it inside a hunk body would let content decide
	// what gets scanned: an added source line reading "++ b/docs/x.md; <call>"
	// reaches us as "+++ b/docs/x.md; <call>", and would otherwise point the
	// scope at the docs tree and hide everything after it in that file.
	inHeader := true
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			scan = lintable(diffHeaderPath(line))
			inHeader = true
			continue
		case strings.HasPrefix(line, "@@"):
			inHeader = false
			continue
		case inHeader && strings.HasPrefix(line, "+++ "):
			if path := postImagePath(strings.TrimPrefix(line, "+++ ")); path != "" && path != "/dev/null" {
				scan = lintable(path)
			}
			continue
		case inHeader && strings.HasPrefix(line, "--- "):
			// The pre-image half of the same header pair, never content.
			continue
		}
		// A bare "+++" prefix is not metadata: an added line of source that
		// itself begins with "++" arrives here as "+++<line>", and skipping it
		// would walk an unsafe call straight past the lint.
		if scan && strings.HasPrefix(line, "+") {
			lines = append(lines, line)
		}
	}
	return lines
}

func addedUntrackedLines(text string) []string {
	var lines []string
	scan := true
	// gitcontext.untrackedExcerpt writes each record as a bare "--- <path>"
	// header followed by the file's lines, every one of them "+"-prefixed. That
	// prefix is the record boundary: content can never start a line with "--- ",
	// so a file cannot name a path of its own choosing and move the scope.
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "--- ") {
			// Only the CR of a CRLF line ending is stripped. A leading or
			// trailing space can be part of a real path, and trimming it turned
			// " docs/x.js" — a file in a directory literally named " docs" —
			// into something that looked like it lived under docs/, excluding
			// it from the lints.
			scan = lintable(strings.TrimSuffix(strings.TrimPrefix(line, "--- "), "\r"))
			continue
		}
		if scan && strings.HasPrefix(line, "+") {
			lines = append(lines, line)
		}
	}
	return lines
}

// lintable reports whether a path carries code the deterministic lints judge.
func lintable(path string) bool {
	if path == "" {
		return true
	}
	// Markdown and the docs tree only: a .txt under src/ is content, not prose, and
	// excluding it would silently widen the blind spot. Matched case-insensitively
	// so README.MD is treated as the prose it is.
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown") {
		return false
	}
	return !strings.HasPrefix(lower, "docs/")
}

// diffHeaderPath reads the post-image path out of a "diff --git a/x b/x" line.
func diffHeaderPath(line string) string {
	rest := strings.TrimPrefix(line, "diff --git ")
	// Splitting on whitespace loses a path containing spaces, and git quotes
	// exactly those, so find where the post-image half begins instead.
	if i := strings.LastIndex(rest, ` "b/`); i >= 0 {
		return postImagePath(rest[i+1:])
	}
	if i := strings.LastIndex(rest, " b/"); i >= 0 {
		return postImagePath(rest[i+1:])
	}
	return ""
}

// postImagePath turns the right-hand half of a diff header into a repository
// path. git renders a path with spaces or non-ASCII bytes as a C-quoted string
// ("b/docs/design notes.md"), and leaving the quotes on would hide it from both
// the docs/ prefix and the .md suffix below.
func postImagePath(value string) string {
	value = strings.TrimSuffix(value, "\r")
	if strings.HasPrefix(value, `"`) {
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		}
	}
	return strings.TrimPrefix(value, "b/")
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
