package reviewstate

import (
	"path/filepath"
	"strings"

	"github.com/dsifry/metareview/internal/findings"
	"github.com/dsifry/metareview/internal/reviewlog"
	"github.com/dsifry/metareview/internal/runchain"
)

type Options struct {
	Scope            string
	Target           map[string]string
	PreviousRunID    string
	PreviousRunIDs   []string
	HistoricalRunIDs []string
	ChangedPaths     []string
	CurrentTarget    map[string]string
	LinkedTargets    []map[string]string
	CurrentRunID     string
}

type Projection struct {
	targetKeyValue       string
	currentReviewLogs    []reviewlog.Summary
	currentBlockers      []findings.Record
	historicalLogs       []reviewlog.Summary
	historicalBlockers   []findings.Record
	supersededRunIDs     map[string]bool
	supersededFindingIDs map[string]bool
}

func Project(root string, options Options) (Projection, error) {
	logs, err := reviewlog.Discover(root)
	if err != nil {
		return Projection{}, err
	}
	blockers, err := findings.UnresolvedBlocking(root)
	if err != nil {
		return Projection{}, err
	}
	if options.PreviousRunID != "" {
		chain, err := runchain.Resolve(root, runchain.Options{
			Scope:         options.Scope,
			Target:        targetForRunchain(options),
			PreviousRunID: options.PreviousRunID,
		})
		if err != nil {
			return Projection{}, err
		}
		options.PreviousRunIDs = append(options.PreviousRunIDs, runIDsFromChain(chain.Chain)...)
	}
	return ProjectRecords(logs, blockers, options), nil
}

func targetForRunchain(options Options) map[string]string {
	if len(options.CurrentTarget) > 0 {
		return options.CurrentTarget
	}
	return options.Target
}

func runIDsFromChain(chain []runchain.Record) []string {
	ids := make([]string, 0, len(chain))
	for _, link := range chain {
		ids = append(ids, link.ID)
	}
	return ids
}

func ProjectRecords(logs []reviewlog.Summary, blockers []findings.Record, options Options) Projection {
	previous := stringSet(options.PreviousRunIDs)
	historical := stringSet(options.HistoricalRunIDs)
	changed := normalizedPathSet(options.ChangedPaths)
	linked := targetSet(options.LinkedTargets)
	historicalRunIDs := map[string]bool{}
	currentRunIDs := map[string]bool{}
	currentTarget := targetForRunchain(options)
	if key := findingTargetKey(currentTarget); key != "" {
		linked[key] = true
	}
	// Same-head dedup (issue #97): re-running the SAME review over the SAME (kind, target, headSha) must
	// REPLACE the prior run, not stack another blocker. Without this, `review pr-ready` run three times over
	// one commit lands three review logs, and the gate renders the branch as three blockers that never clear.
	// Keyed on head, so a fix-loop that reviews a DIFFERENT commit each attempt is untouched; skipped for logs
	// with no headSha (legacy), where we cannot tell whether two logs are the same review.
	staleSameHead := StaleSameHeadRunIDs(logs)
	staleSameHeadFindings := staleSameHeadFindingIDs(logs, blockers)
	projection := Projection{
		targetKeyValue:       TargetKey(options.Scope, currentTarget),
		currentReviewLogs:    make([]reviewlog.Summary, 0, len(logs)),
		currentBlockers:      make([]findings.Record, 0, len(blockers)),
		historicalLogs:       []reviewlog.Summary{},
		historicalBlockers:   []findings.Record{},
		supersededRunIDs:     map[string]bool{},
		supersededFindingIDs: map[string]bool{},
	}
	for _, log := range logs {
		if previous[log.RunID] || staleSameHead[log.RunID] {
			projection.supersededRunIDs[log.RunID] = true
			projection.historicalLogs = append(projection.historicalLogs, log)
			continue
		}
		if historical[log.RunID] {
			projection.historicalLogs = append(projection.historicalLogs, log)
			continue
		}
		unrelated := unrelatedTargetLog(log, changed, linked)
		if options.Scope == "pr-ready" && log.Kind == "pr-ready" {
			// Prior PR-ready logs are outcomes, not reviewer prerequisites, so keep
			// them out of the current reviewer input. Only an unrelated target makes
			// that run's findings historical; current-target findings remain part of
			// the live frontier until an explicit fix chain supersedes them.
			if unrelated && log.RunID != "" {
				historicalRunIDs[log.RunID] = true
			}
			projection.historicalLogs = append(projection.historicalLogs, log)
			continue
		}
		if unrelated {
			if log.RunID != "" {
				historicalRunIDs[log.RunID] = true
			}
			projection.historicalLogs = append(projection.historicalLogs, log)
			continue
		}
		projection.currentReviewLogs = append(projection.currentReviewLogs, log)
		if log.RunID != "" {
			currentRunIDs[log.RunID] = true
		}
	}
	for _, blocker := range blockers {
		if previous[blocker.RunID] || staleSameHeadFindings[blocker.ID] {
			projection.supersededFindingIDs[blocker.ID] = true
			continue
		}
		if historicalRunIDs[blocker.RunID] {
			projection.historicalBlockers = append(projection.historicalBlockers, blocker)
			continue
		}
		if historical[blocker.RunID] || unrelatedFindingTarget(blocker, currentRunIDs, changed, linked) {
			projection.historicalBlockers = append(projection.historicalBlockers, blocker)
			continue
		}
		projection.currentBlockers = append(projection.currentBlockers, blocker)
	}
	return projection
}

// StaleSameHeadRunIDs returns the run IDs that an idempotent re-run of the SAME review over the SAME commit
// supersedes (issue #97) — so `review pr-ready` run three times over one head renders as ONE blocker, not
// three. It is deliberately VERDICT-AWARE to avoid a false-CLEAR: because the head is byte-identical, a later
// CLEAN re-look can never represent a FIX (only a non-deterministic reviewer miss), so a BLOCKING run is
// retired ONLY by a later BLOCKING run of the same (kind, target, headSha); a clean run may be retired by any
// later run of the group. Logs with no headSha or run ID are never grouped (we cannot prove two are the same
// review). "Latest" is the lexicographically greatest run ID, which orders by the timestamp its `mrv-<ts>-…`
// id encodes.
func StaleSameHeadRunIDs(logs []reviewlog.Summary) map[string]bool {
	latestAll := map[string]string{}      // latest run of the group, regardless of verdict
	latestBlocking := map[string]string{} // latest BLOCKING run of the group
	for _, log := range logs {
		if log.HeadSHA == "" || log.RunID == "" {
			continue
		}
		key := sameHeadKey(log)
		if log.RunID > latestAll[key] {
			latestAll[key] = log.RunID
		}
		if LogBlocks(log) && log.RunID > latestBlocking[key] {
			latestBlocking[key] = log.RunID
		}
	}
	stale := map[string]bool{}
	for _, log := range logs {
		if log.HeadSHA == "" || log.RunID == "" {
			continue
		}
		key := sameHeadKey(log)
		if LogBlocks(log) {
			// A blocker is retired ONLY by a LATER blocking run — never by a clean re-look at the same code.
			if log.RunID != latestBlocking[key] {
				stale[log.RunID] = true
			}
			continue
		}
		// A clean run is just an old look; the newest run of the group (any verdict) supersedes it.
		if log.RunID != latestAll[key] {
			stale[log.RunID] = true
		}
	}
	return stale
}

// staleSameHeadFindingIDs deduplicates only findings whose stable fingerprint
// is repeated by a later blocking run over the same target and head. A later run
// can inherit an earlier finding without writing a duplicate record, so run-level
// staleness alone is not enough to retire the sole durable finding.
func staleSameHeadFindingIDs(logs []reviewlog.Summary, blockers []findings.Record) map[string]bool {
	runGroups := map[string]string{}
	for _, log := range logs {
		if log.HeadSHA == "" || log.RunID == "" || !LogBlocks(log) {
			continue
		}
		runGroups[log.RunID] = sameHeadKey(log)
	}
	latestByFingerprint := map[string]string{}
	for _, blocker := range blockers {
		group := runGroups[blocker.RunID]
		fingerprint := strings.TrimSpace(blocker.Fingerprint)
		if group == "" || fingerprint == "" {
			continue
		}
		key := group + "\x00" + fingerprint
		if blocker.RunID > latestByFingerprint[key] {
			latestByFingerprint[key] = blocker.RunID
		}
	}
	stale := map[string]bool{}
	for _, blocker := range blockers {
		group := runGroups[blocker.RunID]
		fingerprint := strings.TrimSpace(blocker.Fingerprint)
		if group == "" || fingerprint == "" {
			continue
		}
		if blocker.RunID != latestByFingerprint[group+"\x00"+fingerprint] {
			stale[blocker.ID] = true
		}
	}
	return stale
}

// LogBlocks reports whether a review log holds work that must clear before the branch is done: open
// unresolved blockers, OR an ESCALATED verdict (a hard stop that a later clean re-run must not erase, even if
// the log records no open finding count). It is the ONE verdict-aware blocking predicate — shared by the
// same-head dedup here and by the status gate's must_clear builders — so the "what supersedes what" decision
// and the "what blocks" decision can never drift apart (reviewlog already forces HasUnresolvedBlockers for an
// ESCALATED verdict, so this is also defense-in-depth against that guarantee changing).
func LogBlocks(log reviewlog.Summary) bool {
	return log.HasUnresolvedBlockers || strings.EqualFold(strings.TrimSpace(log.Verdict), "ESCALATED")
}

func sameHeadKey(log reviewlog.Summary) string {
	target := strings.TrimSpace(log.Target)
	if log.RunRecordAuthenticated {
		if key := findingTargetKey(log.TargetRecord); key != "" {
			target = key
		}
	}
	return log.Kind + "\x00" + target + "\x00" + log.HeadSHA
}

func TargetKey(scope string, target map[string]string) string {
	scope = strings.TrimSpace(scope)
	if len(target) == 0 {
		return scope
	}
	targetType := strings.TrimSpace(target["type"])
	targetID := strings.TrimSpace(firstNonEmpty(target["id"], target["path"]))
	if scope == "" {
		return targetType + ":" + targetID
	}
	return scope + ":" + targetType + ":" + targetID
}

func (projection Projection) TargetKey() string {
	return projection.targetKeyValue
}

func (projection Projection) CurrentReviewLogs() []reviewlog.Summary {
	return projection.currentReviewLogs
}

func (projection Projection) CurrentBlockers() []findings.Record {
	return projection.currentBlockers
}

func (projection Projection) HistoricalUnrelated() []reviewlog.Summary {
	return projection.historicalLogs
}

func (projection Projection) HistoricalBlockers() []findings.Record {
	return projection.historicalBlockers
}

func (projection Projection) SupersededRunIDs() map[string]bool {
	return projection.supersededRunIDs
}

func (projection Projection) SupersededFindingIDs() map[string]bool {
	return projection.supersededFindingIDs
}

func LegacyPreviousRunIDs(logs []reviewlog.Summary, previousRunID string) []string {
	previousRunID = strings.TrimSpace(previousRunID)
	if previousRunID == "" {
		return nil
	}
	byID := map[string]reviewlog.Summary{}
	for _, log := range logs {
		if log.RunID != "" {
			byID[log.RunID] = log
		}
	}
	var reversed []string
	seen := map[string]bool{}
	for id := previousRunID; id != ""; {
		if seen[id] {
			return nil
		}
		seen[id] = true
		log, ok := byID[id]
		if !ok {
			return nil
		}
		reversed = append(reversed, id)
		id = strings.TrimSpace(log.PreviousRunID)
	}
	ids := make([]string, 0, len(reversed))
	for i := len(reversed) - 1; i >= 0; i-- {
		ids = append(ids, reversed[i])
	}
	return ids
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

func normalizedPathSet(paths []string) map[string]bool {
	result := map[string]bool{}
	for _, path := range paths {
		path = normalizePath(path)
		if path != "" {
			result[path] = true
		}
	}
	return result
}

func unrelatedArtifact(log reviewlog.Summary, changed map[string]bool) bool {
	if log.Kind != "artifact" {
		return false
	}
	target := normalizePath(log.Target)
	if target == "" {
		return false
	}
	return !reviewedPathOverlaps(changed, target)
}

func unrelatedTargetLog(log reviewlog.Summary, changed, linked map[string]bool) bool {
	if unrelatedArtifact(log, changed) {
		return true
	}
	if log.Kind == "pr-ready" {
		key := findingTargetKey(log.TargetRecord)
		return key != "" && !linked[key]
	}
	if log.Kind != "task-done" {
		return false
	}
	if linked[canonicalTargetKey("task", log.Target)] {
		return false
	}
	// A legacy log with no coverage identity remains current (fail closed). Once a
	// task review records what it covered, only an overlap with this target's diff
	// makes it part of the PR-ready decision.
	if !log.CoveredPathsKnown {
		return false
	}
	for _, target := range log.CoveredPaths {
		if reviewedPathOverlaps(changed, normalizePath(target)) {
			return false
		}
	}
	return true
}

func unrelatedFindingTarget(blocker findings.Record, currentRunIDs, changed, linked map[string]bool) bool {
	if currentRunIDs[blocker.RunID] {
		return false
	}
	targetType, targetID := findingTarget(blocker.Target)
	switch canonicalTargetType(targetType) {
	case "branch", "task", "pull-request":
		return targetID != "" && !linked[canonicalTargetKey(targetType, targetID)]
	case "path":
		target := normalizePath(targetID)
		return target != "" && !reviewedPathOverlaps(changed, target)
	default:
		// Unknown and unscoped records stay blocking. Target-aware selection must
		// not turn missing provenance into an accidental pass.
		return false
	}
}

func targetSet(targets []map[string]string) map[string]bool {
	result := map[string]bool{}
	for _, target := range targets {
		if key := findingTargetKey(target); key != "" {
			result[key] = true
		}
	}
	return result
}

func findingTargetKey(target map[string]string) string {
	return canonicalTargetKey(target["type"], firstNonEmpty(target["id"], target["path"]))
}

func canonicalTargetKey(targetType, targetID string) string {
	targetType = canonicalTargetType(targetType)
	targetID = strings.TrimSpace(targetID)
	if targetType == "" || targetID == "" {
		return ""
	}
	return targetType + ":" + targetID
}

func canonicalTargetType(targetType string) string {
	switch strings.ToLower(strings.TrimSpace(targetType)) {
	case "beads", "beads-task", "task":
		return "task"
	case "pr", "github-pr", "pull-request":
		return "pull-request"
	case "branch":
		return "branch"
	case "path", "markdown":
		return "path"
	default:
		return strings.ToLower(strings.TrimSpace(targetType))
	}
}

func reviewedPathOverlaps(changed map[string]bool, target string) bool {
	for path := range changed {
		if path == target || strings.HasPrefix(path, strings.TrimSuffix(target, "/")+"/") {
			return true
		}
	}
	return false
}

func findingTarget(target any) (string, string) {
	switch typed := target.(type) {
	case map[string]any:
		return stringValue(typed["type"]), firstNonEmpty(stringValue(typed["id"]), stringValue(typed["path"]))
	case map[string]string:
		return typed["type"], firstNonEmpty(typed["id"], typed["path"])
	default:
		return "", ""
	}
}

func stringValue(value any) string {
	typed, _ := value.(string)
	return typed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(path))
}
