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
	historicalRunIDs := map[string]bool{}
	currentTarget := options.CurrentTarget
	if len(currentTarget) == 0 {
		currentTarget = options.Target
	}
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
		if previous[log.RunID] {
			projection.supersededRunIDs[log.RunID] = true
			projection.historicalLogs = append(projection.historicalLogs, log)
			continue
		}
		if historical[log.RunID] {
			projection.historicalLogs = append(projection.historicalLogs, log)
			continue
		}
		if unrelatedArtifact(log, changed) {
			if log.RunID != "" {
				historicalRunIDs[log.RunID] = true
			}
			projection.historicalLogs = append(projection.historicalLogs, log)
			continue
		}
		projection.currentReviewLogs = append(projection.currentReviewLogs, log)
	}
	for _, blocker := range blockers {
		if previous[blocker.RunID] {
			projection.supersededFindingIDs[blocker.ID] = true
			continue
		}
		if historicalRunIDs[blocker.RunID] {
			projection.historicalBlockers = append(projection.historicalBlockers, blocker)
			continue
		}
		if historical[blocker.RunID] || unrelatedBranchBlocker(blocker, currentTarget) || unrelatedPathBlocker(blocker, changed) {
			projection.historicalBlockers = append(projection.historicalBlockers, blocker)
			continue
		}
		projection.currentBlockers = append(projection.currentBlockers, blocker)
	}
	return projection
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

func unrelatedBranchBlocker(blocker findings.Record, current map[string]string) bool {
	if current["type"] != "branch" || current["id"] == "" {
		return false
	}
	targetType, targetID := findingTarget(blocker.Target)
	return targetType == "branch" && targetID != "" && targetID != current["id"]
}

func unrelatedPathBlocker(blocker findings.Record, changed map[string]bool) bool {
	targetType, targetID := findingTarget(blocker.Target)
	if targetType != "path" {
		return false
	}
	target := normalizePath(targetID)
	if target == "" {
		return false
	}
	return !reviewedPathOverlaps(changed, target)
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
