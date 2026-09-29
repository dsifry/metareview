package findings

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dsifry/metareview/internal/gitcontext"
	"github.com/dsifry/metareview/internal/markdown"
	"github.com/dsifry/metareview/internal/state"
)

// committedReviewsDir is where the gates read committed review logs from.
const committedReviewsDir = "docs/metareview/reviews"

// ImportedFingerprintPrefix marks a row imported from a committed review log (#188). It never matches a live finding's
// fingerprint, so a later run neither refreshes nor deduplicates against it.
const ImportedFingerprintPrefix = "imported-review-log:"

// Seams over the filesystem and git, so the import's failure paths are testable.
var (
	readReviewsDir = os.ReadDir
	readReviewLog  = os.ReadFile
	headOf         = gitcontext.Head
)

// committedFinding finds a blocking finding that exists only in a committed review log — one written on another clone,
// or whose ledger row is gone — and returns it as an open ledger row, so the override commands can reach the blockers
// the gates read from those logs (#188). Only a finding under the "## Blocking Findings" heading of the log whose Run
// ID raised it is taken: a later log that quotes the ID, or an advisory, is not a blocker to override. The row belongs to
// the branch in hand (the branch whose gate the log blocks), at HEAD, and records the log it came from.
func committedFinding(root, findingID string, now string) (Record, bool, error) {
	entries, err := readReviewsDir(filepath.Join(root, filepath.FromSlash(committedReviewsDir)))
	if os.IsNotExist(err) {
		return Record{}, false, nil
	}
	if err != nil {
		return Record{}, false, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		rel := committedReviewsDir + "/" + name
		raw, err := readReviewLog(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return Record{}, false, err
		}
		record, ok := parseCommittedFinding(string(raw), findingID)
		if !ok {
			continue
		}
		head, err := headOf(root)
		if err != nil {
			return Record{}, false, err
		}
		record.SchemaVersion = 1
		record.Status = "open"
		record.Fingerprint = ImportedFingerprintPrefix + findingID
		record.Evidence = []Evidence{{Type: "review-log", Path: rel}}
		record.CreatedAt, record.UpdatedAt = now, now
		record.RepoRoot = root
		record.GitHead = head
		record.Branch = loadScope(root).Current
		return record, true, nil
	}
	return Record{}, false, nil
}

// parseCommittedFinding reads one blocking finding out of a review log's markdown.
func parseCommittedFinding(text, findingID string) (Record, bool) {
	lines := strings.Split(text, "\n")
	record := Record{ID: findingID}
	heading := "### " + findingID + ": "
	section, found := "", false
	for i, line := range lines {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(line, "# metareview: ") && strings.HasSuffix(line, " review") && record.Scope == "":
			record.Scope = strings.TrimSuffix(strings.TrimPrefix(line, "# metareview: "), " review")
		case strings.HasPrefix(line, "Run ID:") && record.RunID == "":
			record.RunID = markdown.FirstInlineCode(line)
		case strings.HasPrefix(line, "Target:") && record.Target == nil:
			record.Target = map[string]string{"type": "review-log", "id": markdown.FirstInlineCode(line)}
		case strings.HasPrefix(line, "## "):
			section = strings.TrimSpace(strings.TrimPrefix(line, "## "))
		case strings.HasPrefix(line, heading) && section == "Blocking Findings" && !found:
			found = true
			record.Title = strings.TrimSpace(strings.TrimPrefix(line, heading))
			readFindingFields(&record, lines[i+1:])
		}
	}
	if !found || !raisedBy(findingID, record.RunID) || !IsBlockingClass(record) {
		return Record{}, false
	}
	return record, true
}

// raisedBy reports whether findingID is one run runID raised: state.FindingID's form, with a numeric index.
func raisedBy(findingID, runID string) bool {
	if runID == "" {
		return false
	}
	prefix := strings.TrimSuffix(state.FindingID(runID, 0), "000")
	index, ok := strings.CutPrefix(findingID, prefix)
	return ok && index != "" && strings.Trim(index, "0123456789") == ""
}

// readFindingFields reads a finding's "- Field: value" bullets, up to the next heading.
func readFindingFields(record *Record, lines []string) {
	fields := map[string]*string{
		"Reviewer": &record.Reviewer, "Severity": &record.Severity, "Classification": &record.Classification,
		"Finding": &record.Finding, "Expected": &record.Expected, "Found": &record.Found,
		"Recommendation": &record.Recommendation,
	}
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "#") {
			return
		}
		name, value, ok := strings.Cut(strings.TrimPrefix(line, "- "), ": ")
		if field, known := fields[name]; ok && known && strings.HasPrefix(line, "- ") && *field == "" {
			*field = strings.TrimSpace(value)
		}
	}
	record.Classification = canonicalClass(record.Classification)
}
