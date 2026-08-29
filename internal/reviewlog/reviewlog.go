package reviewlog

import (
	"encoding/json"
	"fmt"
	"github.com/dsifry/metareview/internal/jsonl"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dsifry/metareview/internal/findings"
	"github.com/dsifry/metareview/internal/runchain"
)

type Summary struct {
	Path                  string            `json:"path"`
	RunID                 string            `json:"runId"`
	Target                string            `json:"target"`
	Verdict               string            `json:"verdict"`
	Kind                  string            `json:"kind"`
	PreviousRunID         string            `json:"previousRunId,omitempty"`
	ContextRel            string            `json:"contextRel,omitempty"`
	FindingIDs            []string          `json:"findingIds"`
	HasUnresolvedBlockers bool              `json:"hasUnresolvedBlockers"`
	AttemptNumber         int               `json:"attemptNumber,omitempty"`
	MaxAttempts           int               `json:"maxAttempts,omitempty"`
	BlockingFindingCount  int               `json:"blockingFindingCount,omitempty"`
	AdvisoryFindingCount  int               `json:"advisoryFindingCount,omitempty"`
	FollowUpFindingCount  int               `json:"followUpFindingCount,omitempty"`
	WarningFindingCount   int               `json:"warningFindingCount,omitempty"`
	RunChain              []runchain.Record `json:"runChain,omitempty"`
	Warnings              []string          `json:"warnings,omitempty"`
}

type findingRecord struct {
	ID             string         `json:"id"`
	RunID          string         `json:"runId"`
	Status         string         `json:"status"`
	Classification string         `json:"classification"`
	Severity       string         `json:"severity"`
	Target         map[string]any `json:"target"`
}

var inlineCodePattern = regexp.MustCompile("`([^`]+)`")
var findingIDPattern = regexp.MustCompile(`mrvf-[A-Za-z0-9._@/-]+`)

func Discover(root string) ([]Summary, error) {
	records, err := readFindings(root)
	if err != nil {
		return nil, err
	}
	runs, err := runchain.ReadRuns(root)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, "docs", "metareview", "reviews")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return []Summary{}, nil
	}
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	logs := make([]Summary, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		rel := filepath.ToSlash(filepath.Join("docs", "metareview", "reviews", entry.Name()))
		bytes, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		summary := parseMarkdown(rel, string(bytes))
		mergeFindings(&summary, records)
		mergeRunMetadata(&summary, runs)
		logs = append(logs, summary)
	}
	return logs, nil
}

func ForTarget(root, target string) ([]Summary, error) {
	logs, err := Discover(root)
	if err != nil {
		return nil, err
	}
	var matches []Summary
	for _, log := range logs {
		if log.Target == target {
			matches = append(matches, log)
		}
	}
	return matches, nil
}

func parseMarkdown(rel, text string) Summary {
	summary := Summary{Path: rel}
	var declaredLenses []string
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "# metareview:"):
			summary.Kind = reviewKind(line)
		case strings.HasPrefix(line, "Run ID:"):
			summary.RunID = firstInlineCode(line)
		case strings.HasPrefix(line, "Target:"):
			summary.Target = firstInlineCode(line)
		case strings.HasPrefix(line, "Previous run:"):
			summary.PreviousRunID = previousRunID(firstInlineCode(line))
		case strings.HasPrefix(line, "Context pack:"):
			summary.ContextRel = firstInlineCode(line)
		case strings.HasPrefix(line, "Required lenses:"):
			declaredLenses = splitLenses(firstInlineCode(line))
		case strings.TrimSpace(line) == "## Verdict":
			summary.Verdict = nextNonEmpty(lines, i+1)
		}
		for _, id := range findingIDPattern.FindAllString(line, -1) {
			summary.FindingIDs = appendUnique(summary.FindingIDs, id)
		}
	}
	if verdictIsUnresolved(summary.Verdict) {
		summary.HasUnresolvedBlockers = true
	}
	if summary.Kind == "artifact" && !artifactReviewComplete(lines, declaredLenses, summary.RunID) {
		summary.HasUnresolvedBlockers = true
	}
	return summary
}

func previousRunID(value string) string {
	value = strings.TrimSpace(value)
	if strings.EqualFold(value, "none") {
		return ""
	}
	return value
}

func reviewKind(line string) string {
	lower := strings.ToLower(line)
	switch {
	case strings.Contains(lower, "artifact review"):
		return "artifact"
	case strings.Contains(lower, "task-done review"):
		return "task-done"
	case strings.Contains(lower, "epic-ready review"):
		return "epic-ready"
	case strings.Contains(lower, "pr-ready review"):
		return "pr-ready"
	default:
		return ""
	}
}

func verdictIsUnresolved(verdict string) bool {
	switch strings.ToUpper(strings.TrimSpace(verdict)) {
	case "", "NOT_REVIEWED", "ESCALATE", "ESCALATED", "NEEDS_REVISION":
		return true
	default:
		return false
	}
}

// currentLenses is the set an artifact review must cover today. legacyLenses is the set that was
// required before security (0.7.0) and testing-quality / data-migration (0.8.0) were added.
var currentLenses = []string{"feasibility", "completeness", "scopeandalignment", "architecture", "intentpreservation", "security", "testingquality", "datamigration"}
var legacyLenses = []string{"feasibility", "completeness", "scopeandalignment", "architecture", "intentpreservation"}

// lensEra records a rubric and the date it took effect. Adding a lens means appending an era, not
// editing currentLenses: a log has to stay judged against the rubric of its own date, or every
// completed review becomes incomplete the day the next lens ships - which is the failure the
// Required lenses marker exists to prevent, returning exactly when it is needed.
//
// Eras are ordered oldest first and compared as YYYYMMDD strings. Security (0.7.0) and
// testing-quality / data-migration (0.8.0) both shipped on 2026-08-24.
type lensEra struct {
	from   string
	lenses []string
}

var lensEras = []lensEra{
	{from: "", lenses: legacyLenses},
	{from: "20260824", lenses: currentLenses},
}

// eraLenses is the rubric in force when this run happened. A run ID with no parseable date is
// judged against the newest rubric, not the oldest: the grandfather is an allowance for provenance
// we can verify, and treating unknown provenance as old would let any log claim it.
func eraLenses(runID string) []string {
	date := runDate(runID)
	if date == "" {
		return lensEras[len(lensEras)-1].lenses
	}
	lenses := lensEras[0].lenses
	for _, era := range lensEras {
		if era.from <= date {
			lenses = era.lenses
		}
	}
	return lenses
}

// requiredLenses answers which lenses this particular log had to cover.
//
// A completed review is evidence about the artifact as the rubric stood when it ran. Judging an
// old log against lenses invented afterwards marks work incomplete that was complete, and since
// an artifact log only reaches a gate once someone edits the file it reviewed, the result is a
// blocker that can never be resolved by fixing anything - only by a standing override. So the log
// declares its own set, and one written before the marker existed falls back to the set of its era.
// requiredLenses answers which lenses this particular log had to cover: the rubric in force on its
// date, plus anything its own declaration adds.
//
// The date is the authority and the declaration may only strengthen. A declaration that could
// reduce the requirement would be an opt-out written by the thing being judged - a current review
// could declare the legacy five and drop security, testing-quality and data-migration, which is
// precisely the escape the unmarked path already refuses. An unrecognised declaration adds
// nothing; the era floor still applies.
func requiredLenses(declared []string, runID string) []string {
	required := map[string]bool{}
	for _, name := range eraLenses(runID) {
		required[name] = true
	}
	if known := knownRubric(declared); known != nil {
		for _, name := range known {
			required[name] = true
		}
	}
	out := make([]string, 0, len(required))
	for name := range required {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// runDate extracts the YYYYMMDD segment of a run ID (mrv-20260705-...), or "" when there is none.
// A log whose age cannot be established is judged against the current set, not the legacy one:
// the grandfather is an allowance for provenance we can verify, and treating unknown provenance as
// old would let any log claim it by carrying a malformed ID.
func runDate(runID string) string {
	parts := strings.Split(runID, "-")
	if len(parts) < 2 || len(parts[1]) < 8 {
		return ""
	}
	date := parts[1][:8]
	// Parse it as a real calendar date, not just eight digits: "20260230" is all digits and sorts
	// before the cutoff, so a digit check alone would hand the legacy rubric to a log whose id is
	// merely plausible. time.Parse rejects an out-of-range month or day.
	if _, err := time.Parse("20060102", date); err != nil {
		return ""
	}
	return date
}

// knownRubric returns the shipped lens set the declaration names, or nil when it names none.
func knownRubric(declared []string) []string {
	for _, rubric := range [][]string{currentLenses, legacyLenses} {
		if sameLensSet(declared, rubric) {
			return rubric
		}
	}
	return nil
}

func sameLensSet(a, b []string) bool {
	// Length first: comparing only the count of UNIQUE names let a declaration that repeats a lens
	// match a shipped rubric, so "the five legacy lenses, one of them twice" was honoured as the
	// legacy set. A declaration is provenance; a malformed one is not evidence of anything.
	if len(a) != len(b) {
		return false
	}
	seen := map[string]bool{}
	for _, name := range a {
		seen[name] = true
	}
	if len(seen) != len(b) {
		return false
	}
	for _, name := range b {
		if !seen[name] {
			return false
		}
	}
	return true
}

func splitLenses(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		if name := normalizedReviewer(part); name != "" {
			out = append(out, name)
		}
	}
	return out
}

func artifactReviewComplete(lines []string, declared []string, runID string) bool {
	required := map[string]bool{}
	for _, name := range requiredLenses(declared, runID) {
		required[name] = false
	}
	for _, line := range lines {
		columns := markdownTableColumns(line)
		if len(columns) < 2 {
			continue
		}
		name := normalizedReviewer(columns[0])
		if _, ok := required[name]; !ok {
			continue
		}
		if reviewerVerdictComplete(columns[1]) {
			required[name] = true
		}
	}
	for _, complete := range required {
		if !complete {
			return false
		}
	}
	return true
}

func markdownTableColumns(line string) []string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") {
		return nil
	}
	raw := strings.Split(strings.Trim(line, "|"), "|")
	columns := make([]string, 0, len(raw))
	for _, column := range raw {
		columns = append(columns, strings.TrimSpace(column))
	}
	return columns
}

func normalizedReviewer(value string) string {
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, "&", "and")
	replacer := strings.NewReplacer("-", "", "_", "", "/", "", " ", "")
	return canonicalLens(replacer.Replace(value))
}

// canonicalLens folds spellings of the same lens onto one key. The scope lens is written both ways
// in this repository - the artifact scaffold declares "scope-alignment" while the reviewer row it
// asks for is "Scope and alignment" - and both already appear in committed review logs, so folding
// here is what lets a declared set and a reviewer table refer to the same lens. Dropping it makes
// every newly completed artifact review permanently incomplete.
func canonicalLens(name string) string {
	if name == "scopealignment" {
		return "scopeandalignment"
	}
	return name
}

func reviewerVerdictComplete(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "PASS", "PASS_ADVISORY", "NEEDS_REVISION", "ESCALATE", "NOT_APPLICABLE":
		return true
	default:
		return false
	}
}

func mergeFindings(summary *Summary, records []findingRecord) {
	for _, record := range records {
		if record.RunID != summary.RunID {
			continue
		}
		if record.ID != "" {
			summary.FindingIDs = appendUnique(summary.FindingIDs, record.ID)
		}
		if isOpenBlocker(record) {
			summary.HasUnresolvedBlockers = true
		}
	}
}

func mergeRunMetadata(summary *Summary, runs []runchain.Record) {
	if summary.RunID == "" {
		return
	}
	byID := map[string]runchain.Record{}
	for _, run := range runs {
		byID[run.ID] = run
	}
	current, ok := byID[summary.RunID]
	if !ok {
		return
	}
	summary.AttemptNumber = current.AttemptNumber
	summary.MaxAttempts = current.MaxAttempts
	summary.BlockingFindingCount = current.BlockingFindingCount
	summary.AdvisoryFindingCount = current.AdvisoryFindingCount
	summary.FollowUpFindingCount = current.FollowUpFindingCount
	summary.WarningFindingCount = current.WarningFindingCount
	if current.WarningFindingCount > 0 {
		summary.Warnings = append(summary.Warnings, "unknown finding classification present")
	}
	if current.Verdict == "ESCALATED" {
		summary.HasUnresolvedBlockers = true
	}
	chain, err := runchain.ChainTo(runs, summary.RunID)
	if err != nil {
		summary.Warnings = append(summary.Warnings, fmt.Sprintf("run chain unavailable: %v", err))
		return
	}
	summary.RunChain = chain
}

func readFindings(root string) ([]findingRecord, error) {
	path := filepath.Join(root, ".metareview", "findings.jsonl")
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return []findingRecord{}, nil
	}
	if err != nil {
		return nil, err
	}
	// Read-only path: there is nothing a Close error could tell the caller.
	defer func() { _ = file.Close() }()
	var records []findingRecord
	scanner := jsonl.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record findingRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, scanner.Err()
}

func isOpenBlocker(record findingRecord) bool {
	// A pending override still blocks; a granted one does not.
	if !findings.Blocks(record.Status) {
		return false
	}
	if record.Classification == "spec-contract" {
		return true
	}
	return record.Classification == "blocking" && (record.Severity == "critical" || record.Severity == "high")
}

func firstInlineCode(line string) string {
	match := inlineCodePattern.FindStringSubmatch(line)
	if len(match) == 2 {
		return match[1]
	}
	return ""
}

func nextNonEmpty(lines []string, start int) string {
	for i := start; i < len(lines); i++ {
		value := strings.TrimSpace(lines[i])
		if value != "" {
			return value
		}
	}
	return ""
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
