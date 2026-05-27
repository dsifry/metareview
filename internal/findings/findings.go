package findings

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dsifry/metareview/internal/state"
)

type Run struct {
	ID       string `json:"id"`
	Scope    string `json:"scope"`
	Target   any    `json:"target"`
	RepoRoot string `json:"repoRoot"`
	GitHead  string `json:"gitHead"`
}

type Options struct {
	PreviousRunID string
}

type Evidence struct {
	Type string `json:"type"`
	Path string `json:"path,omitempty"`
	Line int    `json:"line,omitempty"`
}

type Input struct {
	Reviewer           string     `json:"reviewer"`
	Severity           string     `json:"severity"`
	Classification     string     `json:"classification"`
	Title              string     `json:"title"`
	Finding            string     `json:"finding"`
	Expected           string     `json:"expected"`
	Found              string     `json:"found"`
	Evidence           []Evidence `json:"evidence"`
	Recommendation     string     `json:"recommendation"`
	Owner              string     `json:"owner,omitempty"`
	KnowledgeCandidate bool       `json:"knowledgeCandidate,omitempty"`
	Fingerprint        string     `json:"fingerprint"`
}

type Record struct {
	SchemaVersion      int        `json:"schemaVersion"`
	ID                 string     `json:"id"`
	RunID              string     `json:"runId"`
	Reviewer           string     `json:"reviewer"`
	Severity           string     `json:"severity"`
	Classification     string     `json:"classification"`
	Status             string     `json:"status"`
	Title              string     `json:"title"`
	Finding            string     `json:"finding"`
	Expected           string     `json:"expected"`
	Found              string     `json:"found"`
	Evidence           []Evidence `json:"evidence"`
	Recommendation     string     `json:"recommendation"`
	Owner              string     `json:"owner"`
	KnowledgeCandidate bool       `json:"knowledgeCandidate"`
	BeadsFollowupID    *string    `json:"beadsFollowupId"`
	Fingerprint        string     `json:"fingerprint"`
	Target             any        `json:"target"`
	FixedInRunID       string     `json:"fixedInRunId,omitempty"`
	CreatedAt          string     `json:"createdAt"`
	UpdatedAt          string     `json:"updatedAt"`
	RepoRoot           string     `json:"repoRoot"`
	GitHead            string     `json:"gitHead"`
}

type Result struct {
	Findings          []Record `json:"findings"`
	NewFindings       []Record `json:"newFindings"`
	OpenBlockingCount int      `json:"openBlockingCount"`
}

func Reconcile(root string, run Run, current []Input, options Options) (Result, error) {
	path := findingsPath(root)
	existing, err := readJSONL(path)
	if err != nil {
		return Result{}, err
	}
	currentFingerprints := map[string]bool{}
	for _, finding := range current {
		if finding.Fingerprint != "" {
			currentFingerprints[finding.Fingerprint] = true
		}
	}
	now := nowISO()
	updated := make([]Record, 0, len(existing))
	for _, record := range existing {
		if options.PreviousRunID != "" &&
			record.RunID == options.PreviousRunID &&
			sameTarget(firstTarget(record.Target, run.Target), run.Target) &&
			record.Status == "open" &&
			record.Fingerprint != "" &&
			!currentFingerprints[record.Fingerprint] {
			record.Status = "fixed"
			record.FixedInRunID = run.ID
			record.UpdatedAt = now
			record.GitHead = run.GitHead
		}
		updated = append(updated, record)
	}

	activeExisting := map[string]bool{}
	for _, record := range updated {
		if record.Status == "open" && record.Fingerprint != "" && sameTarget(firstTarget(record.Target, run.Target), run.Target) {
			activeExisting[record.Fingerprint] = true
		}
	}
	newRecords := make([]Record, 0, len(current))
	for _, finding := range current {
		if finding.Fingerprint != "" && activeExisting[finding.Fingerprint] {
			continue
		}
		newRecords = append(newRecords, normalize(run, finding, len(newRecords)+1, now))
	}

	all := append(updated, newRecords...)
	if err := writeJSONL(path, all); err != nil {
		return Result{}, err
	}
	if err := RenderIndexWithRecords(root, all); err != nil {
		return Result{}, err
	}
	activeCurrent := make([]Record, 0, len(current))
	for _, record := range all {
		if record.Status == "open" &&
			record.Fingerprint != "" &&
			currentFingerprints[record.Fingerprint] &&
			sameTarget(firstTarget(record.Target, run.Target), run.Target) {
			activeCurrent = append(activeCurrent, record)
		}
	}
	return Result{
		Findings:          activeCurrent,
		NewFindings:       newRecords,
		OpenBlockingCount: len(unresolvedBlockingFrom(all)),
	}, nil
}

func RenderIndex(root string) error {
	records, err := readJSONL(findingsPath(root))
	if err != nil {
		return err
	}
	return RenderIndexWithRecords(root, records)
}

func RenderIndexWithRecords(root string, records []Record) error {
	blockers := unresolvedBlockingFrom(records)
	path := filepath.Join(root, "docs", "metareview", "FINDINGS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body := "No unresolved findings recorded yet."
	if len(blockers) > 0 {
		lines := make([]string, 0, len(blockers))
		for _, finding := range blockers {
			lines = append(lines, fmt.Sprintf("- %s [%s] %s (%s)", finding.ID, finding.Severity, finding.Title, finding.Reviewer))
		}
		body = strings.Join(lines, "\n")
	}
	return os.WriteFile(path, []byte("# metareview Findings\n\n"+body+"\n"), 0o644)
}

func UnresolvedBlocking(root string) ([]Record, error) {
	records, err := readJSONL(findingsPath(root))
	if err != nil {
		return nil, err
	}
	return unresolvedBlockingFrom(records), nil
}

func normalize(run Run, finding Input, index int, createdAt string) Record {
	owner := finding.Owner
	if owner == "" {
		owner = "implementer"
	}
	return Record{
		SchemaVersion:      1,
		ID:                 state.FindingID(run.ID, index),
		RunID:              run.ID,
		Reviewer:           finding.Reviewer,
		Severity:           finding.Severity,
		Classification:     finding.Classification,
		Status:             "open",
		Title:              finding.Title,
		Finding:            finding.Finding,
		Expected:           finding.Expected,
		Found:              finding.Found,
		Evidence:           finding.Evidence,
		Recommendation:     finding.Recommendation,
		Owner:              owner,
		KnowledgeCandidate: finding.KnowledgeCandidate,
		BeadsFollowupID:    nil,
		Fingerprint:        finding.Fingerprint,
		Target:             run.Target,
		CreatedAt:          createdAt,
		UpdatedAt:          createdAt,
		RepoRoot:           run.RepoRoot,
		GitHead:            run.GitHead,
	}
}

func unresolvedBlockingFrom(records []Record) []Record {
	blockers := make([]Record, 0)
	for _, record := range records {
		if record.Status != "open" {
			continue
		}
		if record.Classification == "spec-contract" ||
			(record.Classification == "blocking" && (record.Severity == "critical" || record.Severity == "high")) {
			blockers = append(blockers, record)
		}
	}
	return blockers
}

func findingsPath(root string) string {
	return filepath.Join(root, ".metareview", "findings.jsonl")
}

func readJSONL(path string) ([]Record, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return []Record{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	records := []Record{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record Record
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, scanner.Err()
}

func writeJSONL(path string, records []Record) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	lines := make([]string, 0, len(records))
	for _, record := range records {
		bytes, err := json.Marshal(record)
		if err != nil {
			return err
		}
		lines = append(lines, string(bytes))
	}
	content := strings.Join(lines, "\n")
	if content != "" {
		content += "\n"
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func sameTarget(a, b any) bool {
	aBytes, err := json.Marshal(a)
	if err != nil {
		return false
	}
	bBytes, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return string(aBytes) == string(bBytes)
}

func firstTarget(recordTarget, fallback any) any {
	if recordTarget == nil {
		return fallback
	}
	return recordTarget
}

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
