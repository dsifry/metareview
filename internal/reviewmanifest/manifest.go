// Package reviewmanifest builds the source manifest for a sharded review and
// aggregates the result files a reviewing agent writes about its own work.
//
// Freshness is by content hash: a shard result is fresh when its shardHash is a
// current shard's hash, a cross-shard result when its planHash is the current
// plan's. Anything else is ignored with a reason — there is no stale category.
package reviewmanifest

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dsifry/metareview/internal/contextprofile"
	"github.com/dsifry/metareview/internal/markdown"
)

const (
	SchemaVersion = 1

	// ResultSchemaVersion is the result-file schema, versioned independently of
	// the manifest's own SchemaVersion.
	ResultSchemaVersion = 1

	// MaxResultBytes is the accident guard on a result file's size.
	MaxResultBytes = 256 * 1024

	// EvidenceNoteMinLength is the note length that stands in for path+line.
	EvidenceNoteMinLength = 12

	VerdictPass          = "PASS"
	VerdictPassAdvisory  = "PASS_ADVISORY"
	VerdictNeedsRevision = "NEEDS_REVISION"
	VerdictEscalated     = "ESCALATED"

	SeverityLow      = "low"
	SeverityMedium   = "medium"
	SeverityHigh     = "high"
	SeverityCritical = "critical"

	DispositionGenerated     = "generated"
	DispositionOutOfScope    = "out-of-scope"
	DispositionFixed         = "fixed"
	DispositionWaived        = "waived"
	DispositionAcceptedRisk  = "accepted-risk"
	DispositionFalsePositive = "false-positive"
	DispositionDeferred      = "deferred"
	DispositionOpen          = "open"

	KindShard      = "shard"
	KindCrossShard = "cross-shard"

	CrossShardID = "cross-shard"
)

var shardIDPattern = regexp.MustCompile(`^shard-[0-9a-f]{1,3}(-[0-9]+)?$`)

var resultNamePattern = regexp.MustCompile(`^(shard-[0-9a-f]{1,3}(?:-[0-9]+)?|cross-shard)\.[0-9a-f]{16}\.result\.json$`)

type Input struct {
	Scope             string
	Target            map[string]string
	Profile           contextprofile.Profile
	ShardPlan         contextprofile.ShardPlan
	PathDispositions  []PathDisposition
	ShardResults      []ReviewResult
	CrossShardResult  *ReviewResult
	IgnoredResults    []IgnoredResult
	UnreadableResults []string
}

type Manifest struct {
	SchemaVersion          int
	Scope                  string
	Target                 map[string]string
	SourcePaths            []string
	LocalPaths             []string
	GeneratedExcludedPaths []string
	PathDispositions       []PathDisposition
	ShardPlan              contextprofile.ShardPlan
	ShardResults           []ReviewResult
	CrossShardResult       *ReviewResult
	IgnoredResults         []IgnoredResult
	UnreadableResults      []string
	SourceManifestHash     string
	RuntimeAssessment      string
}

type PathDisposition struct {
	Path        string
	Disposition string
	Rationale   string
}

// IgnoredResult is a result file that is not about the current content.
type IgnoredResult struct {
	Path   string
	Reason string
}

// ReviewResult is one result file: evidence a reviewing agent writes about the
// shard (or the seams) it reviewed.
type ReviewResult struct {
	SchemaVersion int             `json:"schemaVersion"`
	ID            string          `json:"id"`
	Kind          string          `json:"kind"`
	ShardID       string          `json:"shardId,omitempty"`
	ShardHash     string          `json:"shardHash,omitempty"`
	PlanHash      string          `json:"planHash"`
	Verdict       string          `json:"verdict"`
	Reviewer      string          `json:"reviewer"`
	ReviewedAt    string          `json:"reviewedAt"`
	Evidence      []EvidenceRef   `json:"evidence"`
	Findings      []ResultFinding `json:"findings,omitempty"`
	BlockingCount int             `json:"blockingCount"`

	// Path is the file the result was read from; it never round-trips through JSON.
	Path string `json:"-"`
}

type EvidenceRef struct {
	Path string `json:"path,omitempty"`
	Line int    `json:"line,omitempty"`
	Note string `json:"note,omitempty"`
}

type ResultFinding struct {
	Severity    string        `json:"severity"`
	Disposition string        `json:"disposition"`
	Note        string        `json:"note,omitempty"`
	Evidence    []EvidenceRef `json:"evidence,omitempty"`
}

type AggregateResult struct {
	Verdict       string
	Blockers      []string
	Ignored       []IgnoredResult
	ShardCount    int
	ShardsCovered int
	CrossShard    bool
	PlanHash      string
}

// ParseResult decodes one result file. A non-empty reason means the file is
// ignored; an error means it could not be read as a result at all.
func ParseResult(data []byte) (ReviewResult, string, error) {
	if len(data) > MaxResultBytes {
		return ReviewResult{}, fmt.Sprintf("result file exceeds %d bytes", MaxResultBytes), nil
	}
	var result ReviewResult
	if err := json.Unmarshal(data, &result); err != nil {
		return ReviewResult{}, "", err
	}
	return result, "", nil
}

// ShardIDFromResultPath returns the shard id a result file name encodes, or "".
func ShardIDFromResultPath(file string) string {
	match := resultNamePattern.FindStringSubmatch(path.Base(filepathToSlash(file)))
	if match == nil {
		return ""
	}
	return match[1]
}

// IsResultFileName reports whether name is a discoverable result file.
func IsResultFileName(name string) bool { return resultNamePattern.MatchString(name) }

// MatchShard returns the current shard a result's shardHash names.
func MatchShard(plan contextprofile.ShardPlan, result ReviewResult) (contextprofile.Shard, bool) {
	hash := strings.TrimSpace(result.ShardHash)
	if hash == "" {
		return contextprofile.Shard{}, false
	}
	for _, shard := range plan.Shards {
		if shard.Hash == hash {
			return shard, true
		}
	}
	return contextprofile.Shard{}, false
}

// MatchesPlan reports whether a cross-shard result is about the current plan.
func MatchesPlan(plan contextprofile.ShardPlan, result ReviewResult) bool {
	return strings.TrimSpace(result.PlanHash) != "" && result.PlanHash == plan.PlanHash
}

func GeneratedPathDispositions(paths []string) []PathDisposition {
	cleaned := cleanSortedUnique(paths)
	result := make([]PathDisposition, 0, len(cleaned))
	for _, item := range cleaned {
		result = append(result, PathDisposition{
			Path:        item,
			Disposition: DispositionGenerated,
			Rationale:   "metareview generated review artifact excluded from source manifest",
		})
	}
	return result
}

func Build(input Input) Manifest {
	manifest := Manifest{
		SchemaVersion:          SchemaVersion,
		Scope:                  strings.TrimSpace(input.Scope),
		Target:                 copyStringMap(input.Target),
		SourcePaths:            profilePaths(input.Profile, true),
		LocalPaths:             profilePaths(input.Profile, false),
		GeneratedExcludedPaths: cleanSortedUnique(input.Profile.GeneratedExcludedFiles),
		PathDispositions:       canonicalPathDispositions(input.PathDispositions),
		ShardPlan:              canonicalShardPlan(input.ShardPlan),
		ShardResults:           canonicalReviewResults(input.ShardResults),
		IgnoredResults:         append([]IgnoredResult{}, input.IgnoredResults...),
		UnreadableResults:      cleanSortedUnique(input.UnreadableResults),
		RuntimeAssessment:      "static-only; runtime not assessed",
		// The manifest hash is the plan hash: content-derived, and free of the
		// generated paths and dispositions that made it churn on every run.
		SourceManifestHash: input.ShardPlan.PlanHash,
	}
	if input.CrossShardResult != nil {
		cross := *input.CrossShardResult
		manifest.CrossShardResult = &cross
	}
	return manifest
}

func Aggregate(manifest Manifest) AggregateResult {
	var blockers []string
	blockers = append(blockers, pathDispositionBlockers(manifest)...)
	blockers = append(blockers, sourceAssignmentBlockers(manifest)...)

	shardBlockers, ignored, covered := shardResultBlockers(manifest)
	blockers = append(blockers, shardBlockers...)

	crossBlockers, crossIgnored, crossPresent := crossShardBlockers(manifest)
	blockers = append(blockers, crossBlockers...)
	ignored = append(ignored, crossIgnored...)

	for _, file := range manifest.UnreadableResults {
		blockers = append(blockers, "unreadable result file "+file)
	}

	verdict := VerdictPass
	if len(blockers) > 0 {
		verdict = VerdictNeedsRevision
	}
	sort.Strings(blockers)
	return AggregateResult{
		Verdict:       verdict,
		Blockers:      blockers,
		Ignored:       append(append([]IgnoredResult{}, manifest.IgnoredResults...), ignored...),
		ShardCount:    len(manifest.ShardPlan.Shards),
		ShardsCovered: covered,
		CrossShard:    crossPresent,
		PlanHash:      manifest.ShardPlan.PlanHash,
	}
}

func Markdown(manifest Manifest, aggregate AggregateResult) string {
	lines := []string{
		"## Review Manifest",
		"",
		"- Manifest verdict: " + markdown.InlineCode(firstNonEmpty(aggregate.Verdict, VerdictPass)),
		"- Source manifest hash: " + manifestHashText(manifest.SourceManifestHash),
		"- Runtime assessment: " + firstNonEmpty(manifest.RuntimeAssessment, "static-only; runtime not assessed"),
		"",
		"### Source Paths",
	}
	lines = append(lines, markdownList(manifest.SourcePaths, "No source paths recorded.")...)
	if len(manifest.LocalPaths) > 0 {
		lines = append(lines, "", "### Local changes (not sharded)")
		lines = append(lines, markdownList(manifest.LocalPaths, "None.")...)
	}
	if len(manifest.PathDispositions) > 0 {
		lines = append(lines, "", "### Path Dispositions")
		for _, disposition := range manifest.PathDispositions {
			lines = append(lines, "- "+disposition.Path+": "+disposition.Disposition+" ("+disposition.Rationale+")")
		}
	}
	if len(manifest.ShardPlan.Shards) > 0 {
		lines = append(lines, "", "### Shards")
		for _, shard := range manifest.ShardPlan.Shards {
			lines = append(lines, "- shard-"+shard.ID+": "+strings.Join(contextprofile.ShardPaths(shard), ", "))
		}
	}
	if len(manifest.ShardResults) > 0 || manifest.CrossShardResult != nil {
		lines = append(lines, "", "### Shard Results")
		for _, result := range manifest.ShardResults {
			lines = append(lines, "- "+resultLine(result))
		}
		if manifest.CrossShardResult != nil {
			lines = append(lines, "- "+resultLine(*manifest.CrossShardResult))
		}
	}
	if len(aggregate.Ignored) > 0 {
		lines = append(lines, "", "### Ignored Result Files")
		for _, ignored := range aggregate.Ignored {
			lines = append(lines, "- "+ingestedCode(ignored.Path)+": "+ingested(ignored.Reason))
		}
	}
	lines = append(lines, "", "### Manifest Blockers")
	lines = append(lines, markdownList(aggregate.Blockers, "No manifest blockers.")...)
	return strings.Join(lines, "\n")
}

// manifestHashText states the unsharded case rather than rendering an empty
// value: the manifest hash is the plan hash, and an unsharded review has no plan.
func manifestHashText(hash string) string {
	if strings.TrimSpace(hash) == "" {
		return "not sharded"
	}
	return markdown.InlineCode(hash)
}

// resultLine renders one result as a single sanitised bullet body.
func resultLine(result ReviewResult) string {
	id := firstNonEmpty(result.ShardID, CrossShardID)
	return markdown.InlineCode(id) + " " + markdown.InlineCode(result.Verdict) +
		" by " + ingested(result.Reviewer) +
		fmt.Sprintf(" (%d blocking)", result.BlockingCount)
}

func pathDispositionBlockers(manifest Manifest) []string {
	sourceSet := stringSet(manifest.SourcePaths)
	dispositions := map[string]PathDisposition{}
	var blockers []string
	for _, disposition := range manifest.PathDispositions {
		if strings.TrimSpace(disposition.Path) == "" {
			continue
		}
		if _, ok := dispositions[disposition.Path]; ok {
			blockers = append(blockers, "duplicate disposition for "+disposition.Path)
		}
		dispositions[disposition.Path] = disposition
		if sourceSet[disposition.Path] {
			blockers = append(blockers, disposition.Path+" has both source coverage and disposition")
		}
		if !validPathDisposition(disposition.Disposition) {
			blockers = append(blockers, "unknown path disposition for "+disposition.Path)
		}
		if !validRationale(disposition.Rationale) {
			blockers = append(blockers, "invalid disposition rationale for "+disposition.Path)
		}
	}
	for _, item := range manifest.GeneratedExcludedPaths {
		disposition, ok := dispositions[item]
		if !ok {
			blockers = append(blockers, "missing disposition for "+item)
			continue
		}
		if !validRationale(disposition.Rationale) {
			blockers = append(blockers, "invalid disposition rationale for "+item)
		}
	}
	return blockers
}

// sourceAssignmentBlockers checks the chunk set: every branch path is chunked and
// every chunk lands in exactly one shard. Local files are never assigned.
func sourceAssignmentBlockers(manifest Manifest) []string {
	if len(manifest.ShardPlan.Shards) == 0 {
		return nil
	}
	sourceSet := stringSet(manifest.SourcePaths)
	chunkCounts := map[string]int{}
	pathSeen := map[string]bool{}
	var blockers []string
	for _, shard := range manifest.ShardPlan.Shards {
		for _, chunk := range shard.Chunks {
			if !sourceSet[chunk.Path] {
				blockers = append(blockers, "shard "+shard.ID+" includes non-source path "+chunk.Path)
				continue
			}
			pathSeen[chunk.Path] = true
			chunkCounts[fmt.Sprintf("%s part %d", chunk.Path, chunk.Part)]++
		}
	}
	keys := make([]string, 0, len(chunkCounts))
	for key := range chunkCounts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if chunkCounts[key] > 1 {
			blockers = append(blockers, key+" assigned to multiple shards")
		}
	}
	for _, item := range manifest.SourcePaths {
		if !pathSeen[item] {
			blockers = append(blockers, item+" is not assigned to a shard")
		}
	}
	return blockers
}

func shardResultBlockers(manifest Manifest) ([]string, []IgnoredResult, int) {
	var blockers []string
	var ignored []IgnoredResult
	byShard := map[string]ReviewResult{}
	for _, result := range manifest.ShardResults {
		shard, ok := MatchShard(manifest.ShardPlan, result)
		if !ok {
			ignored = append(ignored, IgnoredResult{
				Path:   result.Path,
				Reason: "no current shard has hash " + firstNonEmpty(result.ShardHash, "(none)"),
			})
			continue
		}
		id := "shard-" + shard.ID
		if _, seen := byShard[id]; seen {
			blockers = append(blockers, "duplicate shard result "+id)
			continue
		}
		byShard[id] = result
		blockers = append(blockers, resultBlockers("shard result "+id, result, KindShard)...)
		if strings.TrimSpace(result.ShardID) != id {
			blockers = append(blockers, "shard result "+id+" declares shard "+
				firstNonEmpty(result.ShardID, "(none)"))
		}
	}
	for _, shard := range manifest.ShardPlan.Shards {
		if _, ok := byShard["shard-"+shard.ID]; !ok {
			blockers = append(blockers, "missing shard result for shard-"+shard.ID)
		}
	}
	return blockers, ignored, len(byShard)
}

func crossShardBlockers(manifest Manifest) ([]string, []IgnoredResult, bool) {
	multi := len(manifest.ShardPlan.Shards) > 1
	if manifest.CrossShardResult == nil {
		if multi {
			return []string{"missing cross-shard result"}, nil, false
		}
		return nil, nil, false
	}
	result := *manifest.CrossShardResult
	if !MatchesPlan(manifest.ShardPlan, result) {
		ignored := []IgnoredResult{{
			Path:   result.Path,
			Reason: "not the current plan hash " + firstNonEmpty(manifest.ShardPlan.PlanHash, "(none)"),
		}}
		if multi {
			return []string{"missing cross-shard result"}, ignored, false
		}
		return nil, ignored, false
	}
	blockers := resultBlockers("cross-shard result", result, KindCrossShard)
	if strings.TrimSpace(result.ShardID) != "" {
		blockers = append(blockers, "cross-shard result must not name a shard")
	}
	return blockers, nil, true
}

// resultBlockers is the shape and disposition validation every result gets.
func resultBlockers(label string, result ReviewResult, kind string) []string {
	var blockers []string
	if result.SchemaVersion != ResultSchemaVersion {
		blockers = append(blockers, label+" has unsupported schema version")
	}
	if strings.TrimSpace(result.ID) == "" {
		blockers = append(blockers, label+" missing result ID")
	}
	if result.Kind != kind {
		blockers = append(blockers, label+" unknown kind "+markdown.PlainText(result.Kind))
	}
	if !validVerdict(result.Verdict) {
		blockers = append(blockers, label+" unknown verdict "+markdown.PlainText(result.Verdict))
	}
	if strings.TrimSpace(result.Reviewer) == "" {
		blockers = append(blockers, label+" missing reviewer")
	}
	if _, err := time.Parse(time.RFC3339, strings.TrimSpace(result.ReviewedAt)); err != nil {
		blockers = append(blockers, label+" unparsable reviewedAt")
	}
	if kind == KindShard && !shardIDPattern.MatchString(result.ShardID) {
		blockers = append(blockers, label+" invalid shard ID "+markdown.PlainText(result.ShardID))
	}
	if result.Path != "" {
		want := KindCrossShard
		if kind == KindShard {
			want = result.ShardID
		}
		if ShardIDFromResultPath(result.Path) != want {
			blockers = append(blockers, label+" does not match its file name "+markdown.PlainText(result.Path))
		}
	}
	if !hasValidEvidence(result.Evidence) {
		blockers = append(blockers, label+" missing evidence")
	}
	if result.BlockingCount > 0 || verdictBlocks(result.Verdict) {
		blockers = append(blockers, label+" has blockers")
	}
	for _, finding := range result.Findings {
		if !validSeverity(finding.Severity) {
			blockers = append(blockers, label+" unknown severity "+markdown.PlainText(finding.Severity))
		}
		if !validFindingDisposition(finding.Disposition) {
			blockers = append(blockers, label+" unknown disposition "+markdown.PlainText(finding.Disposition))
		} else if severityBlocks(finding.Severity) && !dispositionCloses(finding.Disposition) {
			blockers = append(blockers, label+" has unresolved "+finding.Disposition+" finding")
		}
		if !hasValidEvidence(finding.Evidence) {
			blockers = append(blockers, label+" finding missing evidence")
		}
	}
	return blockers
}

func profilePaths(profile contextprofile.Profile, branch bool) []string {
	paths := make([]string, 0, len(profile.Files))
	for _, file := range profile.Files {
		isBranch := file.Source == "" || file.Source == contextprofile.SourceBranch
		if isBranch == branch {
			paths = append(paths, file.Path)
		}
	}
	return cleanSortedUnique(paths)
}

func canonicalShardPlan(plan contextprofile.ShardPlan) contextprofile.ShardPlan {
	out := plan
	out.Shards = append([]contextprofile.Shard{}, plan.Shards...)
	sort.Slice(out.Shards, func(i, j int) bool { return out.Shards[i].ID < out.Shards[j].ID })
	return out
}

func canonicalPathDispositions(values []PathDisposition) []PathDisposition {
	result := append([]PathDisposition{}, values...)
	for i := range result {
		result[i].Path = strings.TrimSpace(result[i].Path)
		result[i].Disposition = strings.TrimSpace(result[i].Disposition)
		result[i].Rationale = strings.TrimSpace(result[i].Rationale)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Path == result[j].Path {
			return result[i].Disposition < result[j].Disposition
		}
		return result[i].Path < result[j].Path
	})
	return result
}

func canonicalReviewResults(values []ReviewResult) []ReviewResult {
	result := append([]ReviewResult{}, values...)
	sort.Slice(result, func(i, j int) bool {
		if result[i].ShardID == result[j].ShardID {
			return result[i].ID < result[j].ID
		}
		return result[i].ShardID < result[j].ShardID
	})
	return result
}

func cleanSortedUnique(values []string) []string {
	seen := map[string]bool{}
	var result []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func copyStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func stringSet(values []string) map[string]bool {
	result := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result[value] = true
		}
	}
	return result
}

func filepathToSlash(value string) string { return strings.ReplaceAll(value, "\\", "/") }

func validPathDisposition(value string) bool {
	switch value {
	case DispositionGenerated, DispositionOutOfScope:
		return true
	default:
		return false
	}
}

func validRationale(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) < 12 {
		return false
	}
	switch value {
	case "n/a", "none", "to" + "do", "tbd":
		return false
	default:
		return true
	}
}

func validVerdict(value string) bool {
	switch value {
	case VerdictPass, VerdictPassAdvisory, VerdictNeedsRevision, VerdictEscalated:
		return true
	default:
		return false
	}
}

func verdictBlocks(value string) bool {
	return value == VerdictNeedsRevision || value == VerdictEscalated
}

func validSeverity(value string) bool {
	switch value {
	case SeverityLow, SeverityMedium, SeverityHigh, SeverityCritical:
		return true
	default:
		return false
	}
}

func severityBlocks(value string) bool {
	return value == SeverityMedium || value == SeverityHigh || value == SeverityCritical
}

func validFindingDisposition(value string) bool {
	switch value {
	case DispositionFixed, DispositionWaived, DispositionAcceptedRisk, DispositionFalsePositive, DispositionDeferred, DispositionOpen:
		return true
	default:
		return false
	}
}

// dispositionCloses reports the two dispositions that close a finding.
func dispositionCloses(value string) bool {
	return value == DispositionFixed || value == DispositionFalsePositive
}

func hasValidEvidence(values []EvidenceRef) bool {
	for _, value := range values {
		if evidenceRefValid(value) {
			return true
		}
	}
	return false
}

func evidenceRefValid(value EvidenceRef) bool {
	if strings.TrimSpace(value.Path) != "" && value.Line > 0 {
		return true
	}
	return len(strings.TrimSpace(value.Note)) >= EvidenceNoteMinLength
}

func markdownList(values []string, empty string) []string {
	if len(values) == 0 {
		return []string{empty}
	}
	lines := make([]string, 0, len(values))
	for _, value := range values {
		lines = append(lines, "- "+markdown.PlainText(value))
	}
	return lines
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

// ingested sanitises a string that came out of a result file. Newlines and
// control characters would break a table; an mrvf- token would be harvested by
// reviewlog as one of the run's finding IDs, so its prefix is neutralised.
func ingested(value string) string {
	return strings.ReplaceAll(markdown.PlainText(value), "mrvf-", "mrvf_")
}

// ingestedCode is ingested for a value rendered as inline code. Result file
// paths are named by the host, so they carry the same injection risk as the
// strings inside a result.
func ingestedCode(value string) string {
	return strings.ReplaceAll(markdown.InlineCode(value), "mrvf-", "mrvf_")
}

// ShardedReviewMarkdown renders the review log's `## Sharded Review` section: the
// results ingested, the files ignored with their reason, anything unreadable, and
// the files that were reviewed as chunks. It returns "" when nothing was read.
func ShardedReviewMarkdown(manifest Manifest, aggregate AggregateResult) string {
	if len(manifest.ShardResults) == 0 && manifest.CrossShardResult == nil &&
		len(aggregate.Ignored) == 0 && len(manifest.UnreadableResults) == 0 {
		return ""
	}
	lines := []string{
		"## Sharded Review",
		"",
		"- Plan hash: " + markdown.InlineCode(manifest.SourceManifestHash),
		"- Shards covered: " + fmt.Sprintf("%d of %d", aggregate.ShardsCovered, aggregate.ShardCount),
		"",
		"| Shard | Shard hash | Verdict | Reviewer | Blocking | File |",
		"| --- | --- | --- | --- | ---: | --- |",
	}
	rows := append([]ReviewResult{}, manifest.ShardResults...)
	if manifest.CrossShardResult != nil {
		rows = append(rows, *manifest.CrossShardResult)
	}
	for _, result := range rows {
		lines = append(lines, fmt.Sprintf("| %s | %s | %s | %s | %d | %s |",
			markdown.InlineCode(firstNonEmpty(result.ShardID, CrossShardID)),
			markdown.InlineCode(firstNonEmpty(result.ShardHash, result.PlanHash)),
			markdown.InlineCode(result.Verdict),
			ingested(result.Reviewer),
			result.BlockingCount,
			ingestedCode(result.Path)))
	}
	if len(aggregate.Ignored) > 0 {
		lines = append(lines, "", "### Ignored result files", "")
		for _, ignored := range aggregate.Ignored {
			lines = append(lines, "- "+ingestedCode(ignored.Path)+": "+ingested(ignored.Reason))
		}
	}
	if len(manifest.UnreadableResults) > 0 {
		lines = append(lines, "", "### Unreadable result files", "")
		for _, file := range manifest.UnreadableResults {
			lines = append(lines, "- "+ingestedCode(file))
		}
	}
	if chunked := chunkedFileLines(manifest.ShardPlan); len(chunked) > 0 {
		lines = append(lines, "", "### Files reviewed as chunks", "")
		lines = append(lines, chunked...)
	}
	return strings.Join(lines, "\n")
}

func chunkedFileLines(plan contextprofile.ShardPlan) []string {
	parts := map[string]int{}
	shards := map[string][]string{}
	for _, shard := range plan.Shards {
		for _, chunk := range shard.Chunks {
			if chunk.Parts <= 1 {
				continue
			}
			parts[chunk.Path] = chunk.Parts
			shards[chunk.Path] = appendUnique(shards[chunk.Path], "shard-"+shard.ID)
		}
	}
	paths := make([]string, 0, len(parts))
	for path := range parts {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	lines := make([]string, 0, len(paths))
	for _, path := range paths {
		ids := shards[path]
		sort.Strings(ids)
		lines = append(lines, fmt.Sprintf("- %s: %d parts across %s",
			markdown.InlineCode(path), parts[path], strings.Join(ids, ", ")))
	}
	return lines
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
