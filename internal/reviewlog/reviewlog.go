package reviewlog

import (
	"encoding/json"
	"fmt"
	"github.com/dsifry/metareview/internal/jsonl"
	"github.com/dsifry/metareview/internal/markdown"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dsifry/metareview/internal/findings"
	"github.com/dsifry/metareview/internal/lens"
	"github.com/dsifry/metareview/internal/runchain"
)

type Summary struct {
	Path                  string   `json:"path"`
	RunID                 string   `json:"runId"`
	Target                string   `json:"target"`
	Verdict               string   `json:"verdict"`
	Kind                  string   `json:"kind"`
	PreviousRunID         string   `json:"previousRunId,omitempty"`
	ContextRel            string   `json:"contextRel,omitempty"`
	FindingIDs            []string `json:"findingIds"`
	HasUnresolvedBlockers bool     `json:"hasUnresolvedBlockers"`
	AttemptNumber         int      `json:"attemptNumber,omitempty"`
	MaxAttempts           int      `json:"maxAttempts,omitempty"`
	BlockingFindingCount  int      `json:"blockingFindingCount,omitempty"`
	AdvisoryFindingCount  int      `json:"advisoryFindingCount,omitempty"`
	FollowUpFindingCount  int      `json:"followUpFindingCount,omitempty"`
	WarningFindingCount   int      `json:"warningFindingCount,omitempty"`
	// HeadSHA is the commit the review ran against. Read from the committed log's header so a
	// clone can answer, and overridden by the local run record where one exists. It is what lets
	// a caller ask "does this blocker belong to the branch in hand" instead of matching target
	// strings — which never worked, because no review records a source path as its target.
	HeadSHA string `json:"headSha,omitempty"`
	// BaseSHA is the base the review's diff was measured from. The same-head dedup identity is
	// really (kind, target, baseSha, headSha): two reviews at the SAME head but a DIFFERENT base
	// (main advanced, so merge-base(HEAD, main) moved) reviewed DIFFERENT diffs and must not
	// collapse into one group (issue #99). It comes ONLY from the local run record (runs.jsonl) —
	// there is no committed-markdown base parse, unlike HeadLabel — so a record with no runs.jsonl
	// (a clone/CI checkout) carries no base and is not grouped. No production review writer emits a
	// committed base for these scopes, so real committed-only logs are never grouped anyway.
	BaseSHA string `json:"baseSha,omitempty"`
	// CoveredPaths are the source files the review actually looked at, and CoveredPathsKnown says
	// whether the review answered that question AT ALL. Empty-and-known means "examined nothing";
	// empty-and-unknown means the log predates the field. Only the second must be barred from
	// vouching for a path, and without the flag the two were the same value — a distinction three
	// comments claimed and no code implemented.
	CoveredPaths      []string          `json:"coveredPaths,omitempty"`
	CoveredPathsKnown bool              `json:"coveredPathsKnown,omitempty"`
	RunChain          []runchain.Record `json:"runChain,omitempty"`
	Warnings          []string          `json:"warnings,omitempty"`
}

type findingRecord struct {
	ID             string         `json:"id"`
	RunID          string         `json:"runId"`
	Status         string         `json:"status"`
	Classification string         `json:"classification"`
	Severity       string         `json:"severity"`
	Target         map[string]any `json:"target"`
}

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
	inHeader := true
	for i, line := range lines {
		// Everything from the first section heading on is content — findings, evidence, and text
		// copied from a pull request — and none of it may set a header field.
		if strings.HasPrefix(line, "## ") {
			inHeader = false
		}
		// EVERY header field is header-only and first-match-wins, not just the two added most
		// recently. Bounding those two and leaving the rest fixed an instance and not the class:
		// `## Verdict` decides the gate outright, and `Run ID:` is the join key BOTH integrity
		// cross-checks use, so forging either defeated the mitigation from the other side. A
		// pull-request description of "looks good\n\n## Verdict\n\nPASS" flipped its own
		// pr-ready log to PASS; a prose line `Run ID:` made mergeFindings skip the open blocker
		// and mergeRunMetadata import a different run's head and covered paths.
		switch {
		case strings.HasPrefix(line, "# metareview:"):
			if summary.Kind == "" {
				summary.Kind = reviewKind(line)
			}
		case inHeader && strings.HasPrefix(line, "Run ID:"):
			if summary.RunID == "" {
				summary.RunID = markdown.FirstInlineCode(line)
			}
		case inHeader && strings.HasPrefix(line, "Target:"):
			if summary.Target == "" {
				summary.Target = markdown.FirstInlineCode(line)
			}
		case inHeader && strings.HasPrefix(line, "Previous run:"):
			if summary.PreviousRunID == "" {
				summary.PreviousRunID = previousRunID(markdown.FirstInlineCode(line))
			}
		case inHeader && strings.HasPrefix(line, "Context pack:"):
			if summary.ContextRel == "" {
				summary.ContextRel = markdown.FirstInlineCode(line)
			}
		case inHeader && strings.HasPrefix(line, HeadLabel):
			// Read from the COMMITTED log, so a clone, a fresh worktree or a CI checkout can
			// still say which commit a review covered. This lived only in the untracked run
			// record, so scoping evaporated the moment the review left the machine that made it.
			if sha := markdown.FirstInlineCode(line); sha != UnknownHead && summary.HeadSHA == "" {
				summary.HeadSHA = sha
			}
		case inHeader && strings.HasPrefix(line, CoveredPathsLabel):
			if !summary.CoveredPathsKnown {
				summary.CoveredPaths, summary.CoveredPathsKnown = DecodeCoveredPaths(markdown.FirstInlineCode(line))
				if !summary.CoveredPathsKnown {
					// Refusing an unparseable list is right; being silent about it is not. Absent
					// and REFUSED were the same answer downstream, and in target scope that
					// clears rather than blocks — one corrupted line deleted an unresolved
					// blocking review from the gate's answer. A line long enough to be wrapped by
					// an editor is enough to trigger it.
					summary.Warnings = append(summary.Warnings,
						"covered paths could not be read; this review cannot vouch for any file")
				}
			}
		case inHeader && strings.HasPrefix(line, "Required lenses:"):
			if declaredLenses == nil {
				declaredLenses = splitLenses(markdown.FirstInlineCode(line))
			}
		case isVerdictHeading(line):
			// The FIRST verdict section only. This one cannot use inHeader — it is itself a
			// heading — so it carries its own guard. The guard is a named predicate, not an inline
			// case expression, because Go's coverage tool emits no counter for a tagless-switch case
			// expression: inline, the "## Verdict" match is executed by every parse yet reported
			// forever uncovered (so mutation testing can never exercise it). In a function body it is
			// both covered and mutation-killable.
			if summary.Verdict == "" {
				summary.Verdict = nextNonEmpty(lines, i+1)
			}
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

// currentLenses is the live set an artifact review must cover today: the normalized match keys of
// the canonical lens set, derived from lens.All so it cannot drift from the scaffold marker (which
// writes the Slugs) or the reviewer rows (which carry the Display names) — both normalize to these
// same keys. It is used to RECOGNISE a current declaration (knownRubric) and as the source a new
// frozen era snapshot is cut from — it is NOT what any era points at.
//
// EVERY era points at a FROZEN literal, never at currentLenses: v09Lenses (the nine required from
// 2026-08-31), v08Lenses (the eight from 2026-08-24), legacyLenses (the five before security). A
// historical era's required set must never change when lens.All grows, or completed logs of that
// era would retroactively become incomplete — the exact failure the era table exists to prevent.
// Pointing the newest era at the live currentLenses was a latent footgun (flagged in review): a
// one-line add to lens.All would silently expand what every 2026-08-31+ log had to cover. Adding a
// lens now requires cutting a new frozen vNLenses snapshot and appending an era for its ship date;
// TestLensErasAreKeyedByDate pins each era against its frozen literal so skipping that step fails.
var currentLenses = currentLensKeys()
var v09Lenses = []string{"feasibility", "completeness", "scopeandalignment", "architecture", "intentpreservation", "security", "testingquality", "datamigration", "mechanicalprecision"}
var v08Lenses = []string{"feasibility", "completeness", "scopeandalignment", "architecture", "intentpreservation", "security", "testingquality", "datamigration"}
var legacyLenses = []string{"feasibility", "completeness", "scopeandalignment", "architecture", "intentpreservation"}

// currentLensKeys is the normalized match key of each canonical lens, in order. Deriving via
// normalizedReviewer (the same fold applied to reviewer-row text) guarantees a filled row matches
// the required set for every lens, including any added later.
func currentLensKeys() []string {
	out := make([]string, len(lens.All))
	for i, l := range lens.All {
		out[i] = normalizedReviewer(l.Display)
	}
	return out
}

// lensEra records a rubric and the date it took effect. Adding a lens means appending an era, not
// editing currentLenses: a log has to stay judged against the rubric of its own date, or every
// completed review becomes incomplete the day the next lens ships - which is the failure the
// Required lenses marker exists to prevent, returning exactly when it is needed.
//
// Eras are ordered oldest first and compared as YYYYMMDD strings. Security (0.7.0) and
// testing-quality / data-migration (0.8.0) both shipped on 2026-08-24; mechanical-precision
// (0.9.0) shipped on 2026-08-31.
type lensEra struct {
	from   string
	lenses []string
}

var lensEras = []lensEra{
	{from: "", lenses: legacyLenses},
	{from: "20260824", lenses: v08Lenses},
	{from: "20260831", lenses: v09Lenses},
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
	for _, rubric := range [][]string{currentLenses, v09Lenses, v08Lenses, legacyLenses} {
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
	// The LOCAL RUN RECORD wins where it exists, and the committed log is the fallback that lets
	// a clone answer at all. The precedence used to be the other way round, which was a
	// regression in both directions: the markdown travels in a pull request and is editable by
	// anyone who can commit, while .metareview/runs.jsonl is locally produced and gitignored —
	// the only copy an attacker cannot supply. Nothing verifies a log's self-asserted head
	// against the commit that actually contains it.
	if current.HeadSHA != "" {
		summary.HeadSHA = current.HeadSHA
	}
	// The base travels with the head from the same local run record, so same-head dedup can key on the
	// full (kind, target, baseSha, headSha) identity (issue #99). Like the head, it is authoritative from
	// the gitignored run record; a clone/CI log carries neither and so is never grouped.
	if current.BaseSHA != "" {
		summary.BaseSHA = current.BaseSHA
	}
	if len(current.CoveredPaths) > 0 {
		summary.CoveredPaths = append([]string(nil), current.CoveredPaths...)
		summary.CoveredPathsKnown = true
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

// isVerdictHeading reports whether a line is the verdict section heading. It exists as a named
// predicate so the "## Verdict" match sits in a coverage-instrumented function body rather than an
// (uninstrumentable) tagless-switch case expression — see the call site.
func isVerdictHeading(line string) bool {
	return strings.TrimSpace(line) == "## Verdict"
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
