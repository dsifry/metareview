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
	if strings.Contains(text, "NEEDS_REVISION") {
		summary.HasUnresolvedBlockers = true
	}
	return summary
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
