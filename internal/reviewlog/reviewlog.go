package reviewlog

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Summary struct {
	Path                  string   `json:"path"`
	RunID                 string   `json:"runId"`
	Target                string   `json:"target"`
	Verdict               string   `json:"verdict"`
	Kind                  string   `json:"kind"`
	FindingIDs            []string `json:"findingIds"`
	HasUnresolvedBlockers bool     `json:"hasUnresolvedBlockers"`
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
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "# metareview:"):
			summary.Kind = reviewKind(line)
		case strings.HasPrefix(line, "Run ID:"):
			summary.RunID = firstInlineCode(line)
		case strings.HasPrefix(line, "Target:"):
			summary.Target = firstInlineCode(line)
		case strings.TrimSpace(line) == "## Verdict":
			summary.Verdict = nextNonEmpty(lines, i+1)
		}
		for _, id := range findingIDPattern.FindAllString(line, -1) {
			summary.FindingIDs = appendUnique(summary.FindingIDs, id)
		}
	}
	if verdictIsUnresolved(summary.Verdict) || strings.Contains(text, "NEEDS_REVISION") {
		summary.HasUnresolvedBlockers = true
	}
	if summary.Kind == "artifact" && !artifactReviewComplete(lines) {
		summary.HasUnresolvedBlockers = true
	}
	return summary
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
	case "", "NOT_REVIEWED", "ESCALATE", "NEEDS_REVISION":
		return true
	default:
		return false
	}
}

func artifactReviewComplete(lines []string) bool {
	required := map[string]bool{
		"feasibility":        false,
		"completeness":       false,
		"scopeandalignment":  false,
		"architecture":       false,
		"intentpreservation": false,
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
	return replacer.Replace(value)
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

func readFindings(root string) ([]findingRecord, error) {
	path := filepath.Join(root, ".metareview", "findings.jsonl")
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return []findingRecord{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var records []findingRecord
	scanner := bufio.NewScanner(file)
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
	if record.Status != "open" {
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
