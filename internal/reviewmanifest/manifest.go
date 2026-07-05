package reviewmanifest

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/dsifry/metareview/internal/contextprofile"
)

const (
	SchemaVersion = 1

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

	CrossShardID = "cross-shard"
)

type Input struct {
	Scope            string
	Target           map[string]string
	Profile          contextprofile.Profile
	ShardPlan        contextprofile.ShardPlan
	PathDispositions []PathDisposition
	ShardResults     []ReviewResult
	CrossShardResult *ReviewResult
}

type Manifest struct {
	SchemaVersion          int
	Scope                  string
	Target                 map[string]string
	SourcePaths            []string
	GeneratedExcludedPaths []string
	PathDispositions       []PathDisposition
	ShardPlan              contextprofile.ShardPlan
	ShardResults           []ReviewResult
	CrossShardResult       *ReviewResult
	SourceManifestHash     string
	RuntimeAssessment      string
}

type PathDisposition struct {
	Path        string
	Disposition string
	Rationale   string
}

type ReviewResult struct {
	SchemaVersion      int
	ID                 string
	Path               string
	ShardID            string
	Verdict            string
	SourceManifestHash string
	Reviewer           string
	CoveredPaths       []string
	CoveredShardIDs    []string
	Evidence           []EvidenceRef
	Findings           []ResultFinding
	BlockingCount      int
}

type EvidenceRef struct {
	Path string
	Line int
	Note string
}

type ResultFinding struct {
	Severity    string
	Disposition string
	Evidence    []EvidenceRef
}

type AggregateResult struct {
	Verdict  string
	Blockers []string
}

func GeneratedPathDispositions(paths []string) []PathDisposition {
	cleaned := cleanSortedUnique(paths)
	result := make([]PathDisposition, 0, len(cleaned))
	for _, path := range cleaned {
		result = append(result, PathDisposition{
			Path:        path,
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
		SourcePaths:            sourcePaths(input.Profile),
		GeneratedExcludedPaths: cleanSortedUnique(input.Profile.GeneratedExcludedFiles),
		PathDispositions:       canonicalPathDispositions(input.PathDispositions),
		ShardPlan:              canonicalShardPlan(input.ShardPlan),
		ShardResults:           canonicalReviewResults(input.ShardResults),
		RuntimeAssessment:      "static-only; runtime not assessed",
	}
	if input.CrossShardResult != nil {
		cross := *input.CrossShardResult
		manifest.CrossShardResult = &cross
	}
	manifest.SourceManifestHash = sourceManifestHash(manifest)
	return manifest
}

func Aggregate(manifest Manifest) AggregateResult {
	var blockers []string
	blockers = append(blockers, pathDispositionBlockers(manifest)...)
	blockers = append(blockers, sourceAssignmentBlockers(manifest)...)
	blockers = append(blockers, shardResultBlockers(manifest)...)
	blockers = append(blockers, crossShardBlockers(manifest)...)
	verdict := VerdictPass
	if len(blockers) > 0 {
		verdict = VerdictNeedsRevision
	}
	sort.Strings(blockers)
	return AggregateResult{Verdict: verdict, Blockers: blockers}
}

func Markdown(manifest Manifest, aggregate AggregateResult) string {
	lines := []string{
		"## Review Manifest",
		"",
		"- Manifest verdict: `" + firstNonEmpty(aggregate.Verdict, VerdictPass) + "`",
		"- Source manifest hash: `" + manifest.SourceManifestHash + "`",
		"- Runtime assessment: " + firstNonEmpty(manifest.RuntimeAssessment, "static-only; runtime not assessed"),
		"",
		"### Source Paths",
	}
	lines = append(lines, markdownList(manifest.SourcePaths, "No source paths recorded.")...)
	if len(manifest.PathDispositions) > 0 {
		lines = append(lines, "", "### Path Dispositions")
		for _, disposition := range manifest.PathDispositions {
			lines = append(lines, "- "+disposition.Path+": "+disposition.Disposition+" ("+disposition.Rationale+")")
		}
	}
	if len(manifest.ShardPlan.Shards) > 0 {
		lines = append(lines, "", "### Shards")
		for _, shard := range manifest.ShardPlan.Shards {
			lines = append(lines, "- "+shard.ID+": "+strings.Join(shard.Paths, ", "))
		}
	}
	lines = append(lines, "", "### Manifest Blockers")
	lines = append(lines, markdownList(aggregate.Blockers, "No manifest blockers.")...)
	return strings.Join(lines, "\n")
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
	for _, path := range manifest.GeneratedExcludedPaths {
		disposition, ok := dispositions[path]
		if !ok {
			blockers = append(blockers, "missing disposition for "+path)
			continue
		}
		if !validRationale(disposition.Rationale) {
			blockers = append(blockers, "invalid disposition rationale for "+path)
		}
	}
	return blockers
}

func sourceAssignmentBlockers(manifest Manifest) []string {
	sourceSet := stringSet(manifest.SourcePaths)
	counts := map[string]int{}
	var blockers []string
	for _, shard := range manifest.ShardPlan.Shards {
		for _, path := range cleanSortedUnique(shard.Paths) {
			if !sourceSet[path] {
				blockers = append(blockers, "shard "+shard.ID+" includes non-source path "+path)
				continue
			}
			counts[path]++
		}
	}
	for _, path := range manifest.SourcePaths {
		switch counts[path] {
		case 0:
			blockers = append(blockers, path+" is not assigned to a primary shard")
		case 1:
		default:
			blockers = append(blockers, path+" assigned to multiple primary shards")
		}
	}
	return blockers
}

func shardResultBlockers(manifest Manifest) []string {
	planned := map[string]bool{}
	for _, shard := range manifest.ShardPlan.Shards {
		planned[shard.ID] = true
	}
	byShard := map[string]ReviewResult{}
	var blockers []string
	for _, result := range manifest.ShardResults {
		shardID := strings.TrimSpace(result.ShardID)
		if shardID == "" {
			blockers = append(blockers, "shard result missing shard ID")
			continue
		}
		if !planned[shardID] {
			blockers = append(blockers, "unexpected shard result for "+shardID)
			continue
		}
		if _, ok := byShard[shardID]; ok {
			blockers = append(blockers, "duplicate shard result for "+shardID)
			continue
		}
		byShard[shardID] = result
	}
	for _, shard := range manifest.ShardPlan.Shards {
		result, ok := byShard[shard.ID]
		if !ok {
			blockers = append(blockers, "missing shard result for "+shard.ID)
			continue
		}
		blockers = append(blockers, reviewResultBlockers("shard result "+shard.ID, result, manifest.SourceManifestHash, shard.Paths, nil)...)
	}
	return blockers
}

func crossShardBlockers(manifest Manifest) []string {
	if len(manifest.ShardPlan.Shards) <= 1 && manifest.CrossShardResult == nil {
		return nil
	}
	if manifest.CrossShardResult == nil {
		return []string{"missing cross-shard result"}
	}
	required := make([]string, 0, len(manifest.ShardPlan.Shards))
	for _, shard := range manifest.ShardPlan.Shards {
		required = append(required, shard.ID)
	}
	return reviewResultBlockers("cross-shard result", *manifest.CrossShardResult, manifest.SourceManifestHash, nil, required)
}

func reviewResultBlockers(label string, result ReviewResult, sourceManifestHash string, requiredPaths, requiredShardIDs []string) []string {
	var blockers []string
	if result.SchemaVersion != SchemaVersion {
		blockers = append(blockers, label+" has unsupported schema version")
	}
	if strings.TrimSpace(result.ID) == "" && strings.TrimSpace(result.Path) == "" {
		blockers = append(blockers, label+" missing result ID or path")
	}
	if !validVerdict(result.Verdict) {
		blockers = append(blockers, label+" unknown verdict "+result.Verdict)
	}
	if result.SourceManifestHash != sourceManifestHash {
		if strings.HasPrefix(label, "shard result ") {
			blockers = append(blockers, "stale "+label)
		} else {
			blockers = append(blockers, label+" is stale")
		}
	}
	if strings.TrimSpace(result.Reviewer) == "" {
		blockers = append(blockers, label+" missing reviewer")
	}
	if !hasValidEvidence(result.Evidence) {
		blockers = append(blockers, label+" missing provenance or coverage evidence")
	}
	if len(requiredPaths) > 0 {
		covered := stringSet(result.CoveredPaths)
		required := stringSet(requiredPaths)
		for _, path := range requiredPaths {
			if !covered[path] {
				blockers = append(blockers, label+" does not cover "+path)
			}
		}
		for _, path := range cleanSortedUnique(result.CoveredPaths) {
			if !required[path] {
				blockers = append(blockers, label+" covers unknown path "+path)
			}
		}
	}
	if len(requiredShardIDs) > 0 {
		covered := stringSet(result.CoveredShardIDs)
		required := stringSet(requiredShardIDs)
		for _, shardID := range requiredShardIDs {
			if !covered[shardID] {
				blockers = append(blockers, label+" does not cover "+shardID)
			}
		}
		for _, shardID := range cleanSortedUnique(result.CoveredShardIDs) {
			if !required[shardID] {
				blockers = append(blockers, label+" covers unknown shard "+shardID)
			}
		}
	}
	if result.BlockingCount > 0 || verdictBlocks(result.Verdict) {
		blockers = append(blockers, label+" has blockers")
	}
	for _, finding := range result.Findings {
		if !validSeverity(finding.Severity) {
			blockers = append(blockers, label+" unknown severity "+finding.Severity)
		}
		if !validFindingDisposition(finding.Disposition) {
			blockers = append(blockers, label+" unknown disposition "+finding.Disposition)
		}
		if severityBlocks(finding.Severity) && finding.Disposition == DispositionOpen {
			blockers = append(blockers, label+" has unresolved medium finding")
		}
		if !hasValidEvidence(finding.Evidence) {
			blockers = append(blockers, label+" finding missing evidence")
		}
	}
	return blockers
}

func sourceManifestHash(manifest Manifest) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("schema=%d\n", manifest.SchemaVersion))
	for _, path := range cleanSortedUnique(manifest.SourcePaths) {
		builder.WriteString("source=" + path + "\n")
	}
	for _, path := range cleanSortedUnique(manifest.GeneratedExcludedPaths) {
		builder.WriteString("generated=" + path + "\n")
	}
	for _, disposition := range canonicalPathDispositions(manifest.PathDispositions) {
		builder.WriteString("disposition=" + disposition.Path + "|" + disposition.Disposition + "|" + disposition.Rationale + "\n")
	}
	builder.WriteString("diff=" + manifest.ShardPlan.SourceDiffHash + "\n")
	for _, shard := range canonicalShardPlan(manifest.ShardPlan).Shards {
		builder.WriteString(fmt.Sprintf("shard=%s|%d|%s\n", shard.ID, shard.ByteCount, strings.Join(cleanSortedUnique(shard.Paths), ",")))
	}
	sum := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(sum[:])[:16]
}

func sourcePaths(profile contextprofile.Profile) []string {
	paths := make([]string, 0, len(profile.Files))
	for _, file := range profile.Files {
		paths = append(paths, file.Path)
	}
	return cleanSortedUnique(paths)
}

func canonicalShardPlan(plan contextprofile.ShardPlan) contextprofile.ShardPlan {
	out := plan
	out.Shards = append([]contextprofile.Shard{}, plan.Shards...)
	for i := range out.Shards {
		out.Shards[i].Paths = cleanSortedUnique(out.Shards[i].Paths)
	}
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
	return len(strings.TrimSpace(value.Note)) >= 12
}

func markdownList(values []string, empty string) []string {
	if len(values) == 0 {
		return []string{empty}
	}
	lines := make([]string, 0, len(values))
	for _, value := range values {
		lines = append(lines, "- "+value)
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
