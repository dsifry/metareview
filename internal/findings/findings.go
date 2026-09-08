package findings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/dsifry/metareview/internal/jsonl"
	"github.com/dsifry/metareview/internal/markdown"
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
	CreatedAt             string `json:"createdAt"`
	UpdatedAt             string `json:"updatedAt"`
	RepoRoot              string `json:"repoRoot"`
	GitHead               string `json:"gitHead"`
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

// WriteIndexSeed writes the first-render content of the findings index through
// writeIndexAtomic, so every writer of the durable audit file — the reconciler and the
// artifact scaffold's first-creation path — carries the same fsync/mode/unique-temp
// guarantees instead of a truncating in-place write.
func WriteIndexSeed(path string) error {
	return writeIndexAtomic(path, "# metareview Findings\n\nNo unresolved findings recorded yet.\n")
}

func RenderIndex(root string) error {
	records, err := readJSONL(findingsPath(root))
	if err != nil {
		return err
	}
	return RenderIndexWithRecords(root, records)
}

// carryOverLine matches both shapes the index renders — unresolved-blocker bullets
// ("- mrvf-… [high] Title (reviewer)") and Process Overrides entries ("- mrvf-… [granted] …") —
// keyed by the leading finding ID, which is unique and stable across worktrees and sessions.
var carryOverLine = regexp.MustCompile(`^- (mrvf-[A-Za-z0-9-]+) \[`)

// carryOverLines returns the committed FINDINGS.md's blocker and override lines whose finding
// IDs the rendering records do not know (issue #151).
//
// The local ledger (.metareview/findings.jsonl) is per-worktree transient state; the
// committed docs/metareview/FINDINGS.md is the durable, shared audit trail. Rendering purely
// from the local ledger let a fresh worktree — whose ledger is empty — rewrite the committed
// file to "No unresolved findings recorded yet.", destroying the granted-override provenance
// and open blockers recorded by other worktrees and sessions (observed twice on 2026-09-08;
// once swept into a PR and caught only by CodeRabbit). The render now carries every committed
// line whose finding ID the local records do not contain, verbatim: a record the ledger knows
// (open, fixed, overridden — any status) renders from the ledger and suppresses its committed
// line, so fresh local knowledge always wins — for a non-blocking status the record renders
// as ABSENCE — and a record the ledger has never seen is preserved
// rather than destroyed. An empty ledger is thereby NO INFORMATION, not "no findings" — the
// same stance CoveredPaths takes for none-vs-absent.
//
// Scope boundary, stated so it is not read as more than it is: carry-over is
// display-preserving ONLY. It does not feed carried records back into the local ledger, so
// cross-worktree enforcement (override list, blocking counts) still reports local state, and
// because finding IDs are run-scoped, the same underlying finding re-recorded in a second
// worktree can render twice (its old committed line carried beside the new local one) — a
// duplication that is strictly better than the destruction it replaced, and the price of
// keying carry-over on the only stable identifier the lossy render carries. It also has no
// retirement path: a carried line is suppressed only by a ledger that knows its finding ID,
// and the ledger is transient, so after a clone or ledger cleanup a carried line renders
// indefinitely — clearing it means editing the committed file by hand (or the durable
// ledger reconcile #93-style work would give it).
func carryOverLines(path string, known map[string]bool) (blockers, overrides []string, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// No committed index yet (first render) — nothing to carry. ONLY this case is
			// "no information": a file that exists but cannot be read (permissions, transient
			// I/O) must FAIL the render, because proceeding would overwrite the committed
			// audit trail with the local-only view — recreating, on the error path, the exact
			// silent destruction this carry-over exists to prevent.
			return nil, nil, nil
		}
		return nil, nil, err
	}
	// Carry-over is bounded to the two sections the renderer itself emits — the top
	// unresolved-blockers section and Process Overrides. A bullet under ANY other section
	// (a hand-maintained history, or the "Stale" partition the shelved #93 design adds) is
	// left alone: verbatim preservation of a line is not preservation of its meaning, and
	// promoting a stale section's entries to current blockers would resurrect dead findings.
	const (
		sectionTop = iota
		sectionOverrides
		sectionOther
	)
	section := sectionTop
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "## ") {
			if strings.HasPrefix(line, "## Process Overrides") {
				section = sectionOverrides
			} else {
				section = sectionOther
			}
			continue
		}
		m := carryOverLine.FindStringSubmatch(line)
		if m == nil || known[m[1]] {
			continue
		}
		switch section {
		case sectionTop:
			blockers = append(blockers, line)
		case sectionOverrides:
			overrides = append(overrides, line)
		}
	}
	return blockers, overrides, nil
}

func RenderIndexWithRecords(root string, records []Record) error {
	blockers := unresolvedBlockingFrom(records)
	path := filepath.Join(root, "docs", "metareview", "FINDINGS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	known := make(map[string]bool, len(records))
	for _, record := range records {
		if record.ID != "" {
			known[record.ID] = true
		}
	}
	coBlockers, coOverrides, err := carryOverLines(path, known)
	if err != nil {
		return err // fail closed: an unreadable committed index must not be overwritten
	}
	lines := make([]string, 0, len(blockers)+len(coBlockers))
	for _, finding := range blockers {
		// Emission is canonicalized to one physical line per entry: the committed index is
		// re-read line-by-line by carryOverLines, so a free-text field carrying an embedded
		// newline would make one entry span lines and only its first line carry back. Titles
		// and reviewer names are flattened (whitespace runs to a single space, control
		// characters dropped) so the writer and the reader agree on one form.
		lines = append(lines, fmt.Sprintf("- %s [%s] %s (%s)", finding.ID, finding.Severity,
			singleLine(finding.Title), singleLine(finding.Reviewer)))
	}
	lines = append(lines, coBlockers...)
	body := "No unresolved findings recorded yet."
	if len(lines) > 0 {
		body = strings.Join(lines, "\n")
	}
	document := "# metareview Findings\n\n" + body + "\n"
	overrides := append(overrideLines(records), coOverrides...)
	if len(overrides) > 0 {
		document += "\n## Process Overrides\n\n" +
			"Deliberate exceptions to the review workflow. Pending entries still block CI.\n\n" +
			strings.Join(overrides, "\n") + "\n"
	}
	for _, section := range preservedSections(path) {
		document += "\n" + section + "\n"
	}
	return writeIndexAtomic(path, document)
}

// singleLine flattens a free-text field to the canonical single physical line every emitted
// index entry uses (see the blocker-bullet comment).
func singleLine(s string) string {
	return markdown.PlainText(strings.Join(strings.Fields(s), " "))
}

// preservedSections extracts every committed ## section the renderer does not itself emit
// (anything other than Process Overrides), verbatim, so hand-maintained content — a history
// note, the shelved #93 "Stale" partition — survives the rewrite instead of being silently
// deleted: the render regenerates only its own two sections and must not destroy the rest of
// the committed file. Same trade-off as carried lines: a preserved section has no retirement
// path, and removing one means editing the committed file by hand.
func preservedSections(path string) []string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []string
	var cur []string
	flush := func() {
		if len(cur) > 0 && !strings.HasPrefix(cur[0], "## Process Overrides") {
			out = append(out, strings.TrimRight(strings.Join(cur, "\n"), "\n"))
		}
		cur = nil
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "## ") {
			flush()
			cur = append(cur, line)
			continue
		}
		if len(cur) > 0 {
			cur = append(cur, line)
		}
	}
	flush()
	return out
}

// writeIndexAtomic replaces the committed index write-temp-then-rename, never a truncating
// in-place write (note: rename replaces a SYMLINK at the target with a regular file, where the
// old in-place write wrote through it): the committed index is the durable audit trail this package exists to
// preserve, and os.WriteFile truncates before it writes — a crash or I/O failure mid-write
// (disk full, process kill) would leave it truncated or half-written, the same data loss the
// carry-over prevents on the read path. The rename replaces the file atomically; the temp
// file sits beside it so the rename stays on one filesystem, and is removed on every path
// that does not rename it.
func writeIndexAtomic(path, document string) error {
	// The destination's mode is preserved across replacement (rename does not carry it), and
	// a write-PROTECTED index (owner-write bit clear — an operator's lock on the audit trail)
	// is refused instead of being silently replaced by a fresh writable file. The check is
	// the owner-write bit only: it catches the deliberate lock, not every EACCES the old
	// in-place write could raise (a file another user owns and group/other cannot write still
	// renames — rename needs directory permission, not file permission).
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
		if mode&0o200 == 0 {
			return fmt.Errorf("committed findings index %s is write-protected (mode %v) — refusing to replace it", path, mode)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	// A UNIQUE temp name, not path+".tmp": concurrent renders in one checkout (a gate run
	// overlapping a Stop hook) share a fixed name, where one render's error cleanup can
	// delete another's in-flight temp and turn its rename into a spurious failure. Unique
	// names make the writers independent; the pattern is gitignored for the hard-crash
	// window between write and rename.
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	// Seams over the fallible calls so their error branches — unreachable on a healthy
	// filesystem in tests — stay covered by fault injection instead of being deleted for
	// coverage (the repo's absPath precedent). Every failure path must remove the temp:
	// a leftover would sit untracked in the committed docs/metareview tree until the next
	// render, exactly the kind of file a careless `git add docs/metareview` sweeps in.
	if err := seamWriteString(tmp, document); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return err
	}
	// Sync before rename: on delayed-allocation filesystems the rename can otherwise be
	// journaled ahead of the data blocks, and a power loss leaves a zero-length index —
	// which the next render would then read as nothing to carry.
	if err := seamSync(tmp); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if err := seamClose(tmp); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := seamChmod(name, mode); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return err
	}
	// Best-effort directory sync: the rename is durable only once the directory entry is.
	// Failure is ignored deliberately — the rename already landed, and a dir-fsync error
	// (unsupported on some platforms, e.g. Windows) must not fail a completed write.
	if d, err := os.Open(filepath.Dir(path)); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

var (
	seamWriteString = func(f *os.File, s string) error { _, err := f.WriteString(s); return err }
	seamSync        = func(f *os.File) error { return f.Sync() }
	seamClose       = func(f *os.File) error { return f.Close() }
	seamChmod       = func(name string, mode os.FileMode) error { return os.Chmod(name, mode) }
)

func UnresolvedBlocking(root string) ([]Record, error) {
	records, err := readJSONL(findingsPath(root))
	if err != nil {
		return nil, err
	}
	return unresolvedBlockingFrom(records), nil
}

// All returns every recorded finding regardless of status. The report
// reconciliation layer (#40) needs the resolved, overridden and superseded rows
// too, not just the unresolved blockers, so a historical review can be rendered
// against how its findings were actually cleared.
func All(root string) ([]Record, error) {
	return readJSONL(findingsPath(root))
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
		if IsBlockingClass(record) {
			blockers = append(blockers, record)
		}
	}
	return blockers
}

// IsBlockingClass reports whether a finding's classification and severity put it
// in the gate-closing blocker class (a spec-contract, or a blocking finding at
// critical/high severity). It is the SAME predicate UnresolvedBlocking uses, so
// a consumer that reconciles a review against the blocker set — the report
// renderer in #40 — classifies a finding exactly as the blocker set does, rather
// than on Blocks(status) alone (which is true for any open finding, advisory
// included, and so disagreed with the blocker-status section).
func IsBlockingClass(record Record) bool {
	return classForCount(record.Classification, record.Severity) == "blocking"
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

// Load returns the findings ledger's records — the read-only view callers like the push
// gate reconcile against (issue #147). A repo with no ledger yet is empty, not an error,
// matching Reconcile's own behavior.
func Load(root string) ([]Record, error) {
	return readJSONL(filepath.Join(root, ".metareview", "findings.jsonl"))
}

// IsResolvedTerminal reports whether a finding status is a RECOGNIZED terminal value:
// fixed, override-granted, or superseded. It is an allowlist, not a denylist — a ledger
// row with an unrecognized status (typo, empty, a future value an older reader receives)
// is unvouched: reconciliation consumers must treat it as still blocking, never as
// resolved (issue #147 review: a malformed row must not clear a gate).
func IsResolvedTerminal(status string) bool {
	switch status {
	case "fixed", StatusOverridden, StatusSuperseded:
		return true
	}
	return false
}
