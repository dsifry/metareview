package findings

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dsifry/metareview/internal/gitcontext"
	"github.com/dsifry/metareview/internal/markdown"
)

// committedReviewsDir is where the gates read committed review logs from.
const committedReviewsDir = "docs/metareview/reviews"

// ImportedFingerprintPrefix marks a row imported from a committed review log (#188). It never matches a live finding's
// fingerprint, so a later run neither refreshes nor deduplicates against it.
const ImportedFingerprintPrefix = "imported-review-log:"

// reviewKinds are the scopes a run ID names after its timestamp (mrv-<date>-<time>-<kind>-<slug>).
var reviewKinds = []string{"task-done", "epic-ready", "pr-ready", "artifact"}

// Seams over the filesystem and git, so the import's failure paths are testable.
var (
	readReviewsDir = os.ReadDir
	readReviewLog  = os.ReadFile
	headOf         = gitcontext.Head
)

// committedFindings reaches the blockers the gates read from committed review logs (#188) — written on another clone,
// or whose ledger rows are gone. It returns, as open ledger rows, every blocking finding the ledger lacks (known holds its
// IDs) of each log that lists findingID under "## Blocking Findings", the finding itself first: pr-ready clears a log
// once every finding the ledger KNOWS is resolved, so an override that left a log's other blockers unknown would let its
// grant retire them. It is asked on every override, not only for an unknown ID — a finding imported as another's sibling
// may also be listed by a log whose other blockers are still unknown. A finding is taken as its raising run's log states
// it where that log is committed, else as a later log carries it forward; its run is the one its ID names. pr-ready's
// own "Unresolved review blockers" summary is never a sibling: every pr-ready run re-derives it from the logs it
// summarises, which gate on their own. The rows belong to the branch in hand (the branch whose gate the logs block), at
// HEAD, and record the log they came from.
func committedFindings(root, findingID, now string, known map[string]bool) ([]Record, error) {
	if _, ok := runOfFinding(findingID); !ok {
		return nil, nil
	}
	entries, err := readReviewsDir(filepath.Join(root, filepath.FromSlash(committedReviewsDir)))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	chosen := map[string]committedEntry{} // by ID: the raising run's own log wins over a log that carries the finding forward
	var order []string
	for _, name := range names {
		rel := committedReviewsDir + "/" + name
		raw, err := readReviewLog(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		blockers := parseCommittedBlockers(string(raw))
		if !listsFinding(blockers, findingID) {
			continue
		}
		for _, entry := range blockers {
			if known[entry.record.ID] || entry.record.ID != findingID && derivedSummary(entry.record) {
				continue
			}
			entry.record.Evidence = []Evidence{{Type: "review-log", Path: rel}}
			previous, seen := chosen[entry.record.ID]
			if !seen {
				order = append(order, entry.record.ID)
			}
			if !seen || !previous.raisedHere && entry.raisedHere {
				chosen[entry.record.ID] = entry
			}
		}
	}
	if len(order) == 0 {
		return nil, nil
	}
	head, err := headOf(root)
	if err != nil {
		return nil, err
	}
	branch := loadScope(root).Current
	// The requested finding first, then its siblings in the order the logs list them.
	var records []Record
	if entry, ok := chosen[findingID]; ok {
		records = append(records, entry.record)
	}
	for _, id := range order {
		if id != findingID {
			records = append(records, chosen[id].record)
		}
	}
	for i := range records {
		records[i].SchemaVersion = 1
		records[i].Status = "open"
		records[i].Fingerprint = ImportedFingerprintPrefix + records[i].ID
		records[i].CreatedAt, records[i].UpdatedAt = now, now
		records[i].RepoRoot = root
		records[i].GitHead = head
		records[i].Branch = branch
	}
	return records, nil
}

// derivedSummary reports whether a finding is pr-ready's "Unresolved review blockers" summary of other logs' blockers.
func derivedSummary(record Record) bool {
	return record.Scope == "pr-ready" && record.Reviewer == "pr-readiness-reviewer" && record.Title == "Unresolved review blockers"
}

// committedEntry is one blocking finding a committed log lists; raisedHere is true in the log of the run that raised it.
type committedEntry struct {
	record     Record
	raisedHere bool
}

func listsFinding(entries []committedEntry, id string) bool {
	for _, entry := range entries {
		if entry.record.ID == id {
			return true
		}
	}
	return false
}

// runOfFinding returns the run a finding ID names: state.FindingID's form, mrvf-<run suffix>-<numeric index>.
func runOfFinding(findingID string) (string, bool) {
	rest, ok := strings.CutPrefix(findingID, "mrvf-")
	cut := strings.LastIndex(rest, "-")
	if !ok || cut <= 0 {
		return "", false
	}
	index := rest[cut+1:]
	if index == "" || strings.Trim(index, "0123456789") != "" {
		return "", false
	}
	return "mrv-" + rest[:cut], true
}

// kindOfRun returns the review kind a run ID names, or "".
func kindOfRun(runID string) string {
	parts := strings.SplitN(runID, "-", 4)
	if len(parts) < 4 {
		return ""
	}
	for _, kind := range reviewKinds {
		if parts[3] == kind || strings.HasPrefix(parts[3], kind+"-") {
			return kind
		}
	}
	return ""
}

// parseCommittedBlockers reads a review log's header Run ID and every blocking finding under its "## Blocking Findings"
// heading. Like the gate's own parser, the header (Run ID, Target, kind) is read only above the first "## " heading, first
// match wins, so body text never supplies it.
func parseCommittedBlockers(text string) []committedEntry {
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], "\r")
	}
	var runID, kind string
	var target any
	inHeader, section := true, ""
	var entries []committedEntry
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "## "):
			inHeader = false
			section = strings.TrimSpace(strings.TrimPrefix(line, "## "))
		case inHeader && strings.HasPrefix(line, "# metareview: ") && strings.HasSuffix(line, " review") && kind == "":
			kind = strings.TrimSuffix(strings.TrimPrefix(line, "# metareview: "), " review")
		case inHeader && strings.HasPrefix(line, "Run ID:") && runID == "":
			runID = markdown.FirstInlineCode(line)
		case inHeader && strings.HasPrefix(line, "Target:") && target == nil:
			target = map[string]string{"type": "review-log", "id": markdown.FirstInlineCode(line)}
		case section == "Blocking Findings" && strings.HasPrefix(line, "### "):
			id, title, ok := strings.Cut(strings.TrimPrefix(line, "### "), ": ")
			run, valid := runOfFinding(id)
			if !ok || !valid || listsFinding(entries, id) {
				continue
			}
			record := Record{ID: id, RunID: run, Scope: kindOfRun(run), Title: strings.TrimSpace(title), Target: target}
			if run == runID {
				record.Scope = kind
			}
			readFindingFields(&record, lines[i+1:])
			if IsBlockingClass(record) {
				entries = append(entries, committedEntry{record: record, raisedHere: run == runID})
			}
		}
	}
	return entries
}

// readFindingFields reads a finding's "- Field: value" bullets, up to the next heading.
func readFindingFields(record *Record, lines []string) {
	fields := map[string]*string{
		"Reviewer": &record.Reviewer, "Severity": &record.Severity, "Classification": &record.Classification,
		"Finding": &record.Finding, "Expected": &record.Expected, "Found": &record.Found,
		"Recommendation": &record.Recommendation,
	}
	for _, line := range lines {
		if strings.HasPrefix(line, "#") {
			break
		}
		name, value, ok := strings.Cut(strings.TrimPrefix(line, "- "), ": ")
		if field, known := fields[name]; ok && known && strings.HasPrefix(line, "- ") && *field == "" {
			*field = strings.TrimSpace(value)
		}
	}
	record.Classification = canonicalClass(record.Classification)
}
