package findings

import (
	"encoding/json"
	"fmt"
	"github.com/dsifry/metareview/internal/jsonl"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dsifry/metareview/internal/state"
)

// maxJSONLLineBytes is this package's name for the shared JSONL line cap. The buffer sizing that
// used to be explained here now lives in jsonl.NewScanner, which every reader in this package uses.
const maxJSONLLineBytes = jsonl.MaxLineBytes

type Run struct {
	ID       string `json:"id"`
	Scope    string `json:"scope"`
	Target   any    `json:"target"`
	RepoRoot string `json:"repoRoot"`
	GitHead  string `json:"gitHead"`
}

type Options struct {
	PreviousRunID  string
	PreviousRunIDs []string
	ResetRunIDs    []string
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
	Scope              string     `json:"scope,omitempty"`
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

	// Process-exception provenance (see override.go). An override is never a fix:
	// FixedInRunID stays empty.
	OverrideRequestedBy   string `json:"overrideRequestedBy,omitempty"`
	OverrideRequestedAt   string `json:"overrideRequestedAt,omitempty"`
	OverrideRequestReason string `json:"overrideRequestReason,omitempty"`
	OverrideEscalation    string `json:"overrideEscalation,omitempty"`
	OverrideGrantedBy     string `json:"overrideGrantedBy,omitempty"`
	OverrideGrantedAt     string `json:"overrideGrantedAt,omitempty"`
	OverrideGrantReason   string `json:"overrideGrantReason,omitempty"`
	// What the exception is an exception TO. See bind.go: a grant is bound to the files the
	// finding names (or to the head when it names none), and lapses when they change, so an
	// unattended run cannot accumulate permanent blind spots.
	OverrideBoundKind  string   `json:"overrideBoundKind,omitempty"`
	OverrideBoundPaths []string `json:"overrideBoundPaths,omitempty"`
	OverrideBoundHash  string   `json:"overrideBoundHash,omitempty"`
	OverrideLapsedAt   string   `json:"overrideLapsedAt,omitempty"`
	CreatedAt          string   `json:"createdAt"`
	UpdatedAt          string   `json:"updatedAt"`
	RepoRoot           string   `json:"repoRoot"`
	GitHead            string   `json:"gitHead"`
}

type Result struct {
	Findings          []Record `json:"findings"`
	NewFindings       []Record `json:"newFindings"`
	OpenFindings      []Record `json:"openFindings"`
	OpenBlockingCount int      `json:"openBlockingCount"`
}

func Reconcile(root string, run Run, current []Input, options Options) (Result, error) {
	path := findingsPath(root)
	existing, err := readJSONL(path)
	if err != nil {
		return Result{}, err
	}
	existing, err = supersedeLegacyContextRisk(path, existing, run, nowISO())
	if err != nil {
		return Result{}, err
	}
	existing = lapseStaleOverrides(root, existing, run, nowISO())
	previousRuns := previousRunSet(options)
	resetRuns := resetRunSet(options)
	currentFingerprints := map[string]bool{}
	for _, finding := range current {
		if finding.Fingerprint != "" {
			currentFingerprints[finding.Fingerprint] = true
		}
	}
	now := nowISO()
	updated := make([]Record, 0, len(existing))
	for _, record := range existing {
		if record.Status == "open" &&
			record.Fingerprint != "" &&
			currentFingerprints[record.Fingerprint] &&
			sameRunTarget(record, run) {
			record.Scope = firstNonEmpty(record.Scope, run.Scope)
			record.GitHead = firstNonEmpty(run.GitHead, record.GitHead)
			record.UpdatedAt = now
		}
		// override-pending closes here too, not just open. A requested override
		// that is then genuinely fixed had no way out: the fix transition matched
		// only "open", so the record stayed pending, Blocks kept returning true,
		// and `override list --pending` exited 1 forever with no command able to
		// clear it — the CLI offers request|grant|list and no withdraw. A finding
		// that is no longer found is fixed, and the pending request is moot.
		//
		// StatusOverridden is deliberately not included: a granted override is an
		// acknowledged exception, never a fix, and its fixedInRunId stays empty so
		// post-merge learning can tell the two apart.
		if (previousRuns[record.RunID] || resetFinding(record, run, resetRuns)) &&
			sameRunTarget(record, run) &&
			(record.Status == "open" || record.Status == StatusOverridePending) &&
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
		if record.Status != "fixed" && record.Fingerprint != "" && sameRunTarget(record, run) {
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
	openFindings := openForRun(all, run)
	for _, record := range all {
		if record.Status == "open" &&
			record.Fingerprint != "" &&
			currentFingerprints[record.Fingerprint] &&
			sameRunTarget(record, run) {
			activeCurrent = append(activeCurrent, record)
		}
	}
	return Result{
		Findings:          activeCurrent,
		NewFindings:       newRecords,
		OpenFindings:      openFindings,
		OpenBlockingCount: CountByClass(openFindings).Blocking,
	}, nil
}

// StatusSuperseded marks a row whose fingerprint an upgrade replaced. It is
// neither open (so it never blocks) nor fixed (so learning never reads it as a
// correction), and its fixedInRunId stays empty for the same reason.
const StatusSuperseded = "superseded"

// legacyContextRiskPrefixes are the reason-bearing context-risk fingerprints
// 0.8.3 replaced with reason-independent ones.
var legacyContextRiskPrefixes = []string{
	"architecture:context-risk:",
	"pr:architecture:context-risk:",
	"epic:context-risk:",
}

// supersedeLegacyContextRisk aliases the pre-0.8.3 context-risk rows for this
// target onto the new fingerprint. It runs on every reconcile — including one
// without --previous-run and one on an escalated chain — and is idempotent,
// since a superseded row is no longer open.
func supersedeLegacyContextRisk(path string, records []Record, run Run, now string) ([]Record, error) {
	touched := false
	for _, record := range records {
		if legacyContextRiskRow(record, run) {
			touched = true
			break
		}
	}
	if !touched {
		return records, nil
	}
	if err := backupOnce(path); err != nil {
		return nil, err
	}
	for i := range records {
		if legacyContextRiskRow(records[i], run) {
			records[i].Status = StatusSuperseded
			records[i].UpdatedAt = now
		}
	}
	return records, nil
}

func legacyContextRiskRow(record Record, run Run) bool {
	if record.Status != "open" || !sameRunTarget(record, run) {
		return false
	}
	for _, prefix := range legacyContextRiskPrefixes {
		if strings.HasPrefix(record.Fingerprint, prefix) {
			return true
		}
	}
	return false
}

// backupOnce copies the findings ledger aside before the first alias pass.
func backupOnce(path string) error {
	backup := path + ".pre-0.8.3.bak"
	if _, err := os.Stat(backup); err == nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.WriteFile(backup, data, 0o644)
}

func resetFinding(record Record, run Run, resetRuns map[string]bool) bool {
	return resetRuns[record.RunID] && resetScopeMatches(record, run) && staleForCurrentHead(record, run)
}

func staleForCurrentHead(record Record, run Run) bool {
	recordHead := strings.TrimSpace(record.GitHead)
	runHead := strings.TrimSpace(run.GitHead)
	return recordHead != "" && runHead != "" && recordHead != runHead
}

func sameRunTarget(record Record, run Run) bool {
	return sameCompatibleScope(record, run) && sameTarget(firstTarget(record.Target, run.Target), run.Target)
}

func sameCompatibleScope(record Record, run Run) bool {
	recordScope := strings.TrimSpace(record.Scope)
	runScope := strings.TrimSpace(run.Scope)
	return recordScope == "" || runScope == "" || recordScope == runScope
}

func resetScopeMatches(record Record, run Run) bool {
	recordScope := strings.TrimSpace(record.Scope)
	runScope := strings.TrimSpace(run.Scope)
	return recordScope == "" || (runScope != "" && recordScope == runScope)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
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
	document := "# metareview Findings\n\n" + body + "\n"
	if overrides := overrideLines(records); len(overrides) > 0 {
		document += "\n## Process Overrides\n\n" +
			"Deliberate exceptions to the review workflow. Pending entries still block CI.\n\n" +
			strings.Join(overrides, "\n") + "\n"
	}
	return os.WriteFile(path, []byte(document), 0o644)
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
		Scope:              run.Scope,
		Reviewer:           finding.Reviewer,
		Severity:           finding.Severity,
		Classification:     canonicalClass(finding.Classification),
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
		if !Blocks(record.Status) {
			continue
		}
		if classForCount(record.Classification, record.Severity) == "blocking" {
			blockers = append(blockers, record)
		}
	}
	return blockers
}

type ClassCounts struct {
	Blocking int
	Advisory int
	FollowUp int
	Warnings int
}

func CountByClass(records []Record) ClassCounts {
	var counts ClassCounts
	for _, record := range records {
		switch classForCount(record.Classification, record.Severity) {
		case "blocking":
			counts.Blocking++
		case "advisory":
			counts.Advisory++
		case "follow-up":
			counts.FollowUp++
		default:
			counts.Warnings++
		}
	}
	return counts
}

func canonicalClass(classification string) string {
	classification = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(classification, "_", "-")))
	switch classification {
	case "blocker", "spec-contract":
		return "spec-contract"
	case "blocking":
		return "blocking"
	case "advisory":
		return "advisory"
	case "follow-up", "followup":
		return "follow-up"
	default:
		return "warning"
	}
}

func classForCount(classification, severity string) string {
	switch canonicalClass(classification) {
	case "spec-contract":
		return "blocking"
	case "blocking":
		switch strings.ToLower(strings.TrimSpace(severity)) {
		case "critical", "high":
			return "blocking"
		default:
			return "warning"
		}
	case "advisory":
		return "advisory"
	case "follow-up":
		return "follow-up"
	default:
		return "warning"
	}
}

func openForRun(records []Record, run Run) []Record {
	open := make([]Record, 0, len(records))
	for _, record := range records {
		if Blocks(record.Status) && sameRunTarget(record, run) {
			open = append(open, record)
		}
	}
	return open
}

func previousRunSet(options Options) map[string]bool {
	ids := map[string]bool{}
	if options.PreviousRunID != "" {
		ids[options.PreviousRunID] = true
	}
	for _, id := range options.PreviousRunIDs {
		if id != "" {
			ids[id] = true
		}
	}
	return ids
}

func resetRunSet(options Options) map[string]bool {
	ids := map[string]bool{}
	for _, id := range options.ResetRunIDs {
		if id != "" {
			ids[id] = true
		}
	}
	return ids
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
	// Read-only path: there is nothing a Close error could tell the caller.
	defer func() { _ = file.Close() }()
	records := []Record{}
	scanner := jsonl.NewScanner(file)
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
