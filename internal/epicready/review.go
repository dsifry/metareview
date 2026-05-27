package epicready

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dsifry/metareview/internal/epicsource"
	"github.com/dsifry/metareview/internal/findings"
	"github.com/dsifry/metareview/internal/gitcontext"
	"github.com/dsifry/metareview/internal/knowledge"
	"github.com/dsifry/metareview/internal/markdown"
	"github.com/dsifry/metareview/internal/repo"
	"github.com/dsifry/metareview/internal/reviewers"
	"github.com/dsifry/metareview/internal/reviewlog"
	"github.com/dsifry/metareview/internal/state"
	"github.com/dsifry/metareview/internal/tasksource"
)

type Options struct {
	Base          string
	PreviousRunID string
	EvidencePath  string
	Now           time.Time
}

type Result struct {
	RunID      string
	ReviewRel  string
	ContextRel string
	Verdict    string
	Blocking   bool
}

type runRecord struct {
	SchemaVersion int                 `json:"schemaVersion"`
	ID            string              `json:"id"`
	Scope         string              `json:"scope"`
	Target        map[string]string   `json:"target"`
	Status        string              `json:"status"`
	Verdict       string              `json:"verdict"`
	ExecutionMode string              `json:"executionMode"`
	PreviousRunID *string             `json:"previousRunId"`
	BaseSHA       string              `json:"baseSha"`
	HeadSHA       string              `json:"headSha"`
	ContextPath   string              `json:"contextPackPath"`
	ReviewPath    string              `json:"reviewLogPath"`
	Reviewers     []string            `json:"reviewers"`
	FindingIDs    []string            `json:"findingIds"`
	SourceRefs    []map[string]string `json:"sourceRefs"`
	GateEffect    string              `json:"gateEffect"`
	CreatedAt     string              `json:"createdAt"`
	UpdatedAt     string              `json:"updatedAt"`
	RepoRoot      string              `json:"repoRoot"`
	GitHead       string              `json:"gitHead"`
}

type fileSnapshot struct {
	existed bool
	content []byte
}

var reviewerNames = []string{"epic-integration-reviewer", "acceptance-reviewer", "intent-preservation-reviewer", "architecture-reviewer"}

func Create(root, target string, options Options) (Result, error) {
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	report := repo.Detect(root)
	epic, err := epicsource.Resolve(root, target)
	if err != nil {
		return Result{}, err
	}
	git, err := gitcontext.Collect(root, options.Base)
	if err != nil {
		return Result{}, err
	}
	reviewGit := filterGeneratedGitContext(git)
	knowledgeContext, err := knowledge.Collect(root)
	if err != nil {
		return Result{}, err
	}
	logs, err := reviewlog.Discover(root)
	if err != nil {
		return Result{}, err
	}
	openChildBlockers, err := childOpenBlockerLogs(root, epic.ChildIDs)
	if err != nil {
		return Result{}, err
	}
	evidenceText, err := readEvidence(options.EvidencePath)
	if err != nil {
		return Result{}, err
	}
	children := resolveChildren(root, epic.ChildIDs)
	childLogs := append(latestChildLogs(logs, epic.ChildIDs), openChildBlockers...)

	runID, contextRel, reviewRel, err := uniquePaths(root, target, now)
	if err != nil {
		return Result{}, err
	}
	contextPath := filepath.Join(root, filepath.FromSlash(contextRel))
	reviewPath := filepath.Join(root, filepath.FromSlash(reviewRel))
	runsPath := filepath.Join(root, ".metareview", "runs.jsonl")
	findingsPath := filepath.Join(root, ".metareview", "findings.jsonl")
	findingsIndexPath := filepath.Join(root, "docs", "metareview", "FINDINGS.md")
	snapshots := map[string]fileSnapshot{}
	for _, path := range []string{contextPath, reviewPath, runsPath, findingsPath, findingsIndexPath} {
		snapshots[path] = snapshot(path)
	}

	gateEffect := "advisory"
	if report.Capabilities.Beads || report.Capabilities.Metaswarm {
		gateEffect = "gate"
	}
	rawFindings := reviewers.RunEpicReady(reviewerContext(epic, children, reviewGit, knowledgeContext, childLogs, evidenceText))
	targetRecord := map[string]string{"type": epicTargetType(epic), "id": epic.ID}
	run := findings.Run{ID: runID, Scope: "epic-ready", Target: targetRecord, RepoRoot: root, GitHead: git.HeadSHA}

	result := Result{RunID: runID, ReviewRel: reviewRel, ContextRel: contextRel}
	err = func() error {
		if err := os.MkdirAll(filepath.Dir(contextPath), 0o755); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(reviewPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(contextPath, []byte(contextMarkdown(runID, epic, children, reviewGit, knowledgeContext, childLogs, evidenceText, gateEffect)), 0o644); err != nil {
			return err
		}
		reconciled, err := findings.Reconcile(root, run, rawFindings, findings.Options{PreviousRunID: options.PreviousRunID})
		if err != nil {
			return err
		}
		blocking := len(reconciled.Findings) > 0
		verdict := "PASS"
		status := "passed"
		if blocking {
			verdict = "NEEDS_REVISION"
			status = "needs-revision"
		} else if gateEffect == "advisory" {
			verdict = "PASS_ADVISORY"
		}
		result.Verdict = verdict
		result.Blocking = blocking
		record := runRecord{
			SchemaVersion: 1,
			ID:            runID,
			Scope:         "epic-ready",
			Target:        targetRecord,
			Status:        status,
			Verdict:       verdict,
			ExecutionMode: "deterministic-local",
			PreviousRunID: optionalString(options.PreviousRunID),
			BaseSHA:       git.BaseSHA,
			HeadSHA:       git.HeadSHA,
			ContextPath:   contextRel,
			ReviewPath:    reviewRel,
			Reviewers:     reviewerNames,
			FindingIDs:    findingIDs(reconciled.Findings),
			SourceRefs:    sourceRefs(epic),
			GateEffect:    gateEffect,
			CreatedAt:     now.UTC().Format(time.RFC3339Nano),
			UpdatedAt:     now.UTC().Format(time.RFC3339Nano),
			RepoRoot:      root,
			GitHead:       git.HeadSHA,
		}
		if err := state.AppendJSONL(runsPath, record); err != nil {
			return err
		}
		return os.WriteFile(reviewPath, []byte(reviewMarkdown(runID, target, contextRel, options.PreviousRunID, gateEffect, verdict, reconciled.Findings)), 0o644)
	}()
	if err != nil {
		restoreSnapshots(snapshots)
		removeEmptyDirs(root)
		return Result{}, err
	}
	return result, nil
}

func reviewerContext(epic epicsource.Source, children []tasksource.Source, git gitcontext.Context, knowledgeContext knowledge.Context, logs []reviewlog.Summary, evidenceText string) reviewers.EpicReadyContext {
	return reviewers.EpicReadyContext{
		Epic:     reviewers.EpicContext{ID: epic.ID, Title: epic.Title, Body: epic.Body},
		Children: reviewerChildren(children),
		Git: reviewers.EpicGitContext{
			ChangedFiles: git.ChangedFiles,
			Diff:         strings.Join([]string{git.Diff, git.StagedDiff, git.WorkingTreeDiff, git.UntrackedExcerpts}, "\n"),
		},
		ReviewLogs:   reviewerLogs(logs),
		Knowledge:    reviewers.EpicKnowledgeContext{ServiceInventory: knowledgeContext.ServiceInventory},
		EvidenceText: evidenceText,
	}
}

func resolveChildren(root string, childIDs []string) []tasksource.Source {
	children := make([]tasksource.Source, 0, len(childIDs))
	for _, id := range childIDs {
		child, err := tasksource.Resolve(root, id)
		if err != nil {
			children = append(children, tasksource.Source{Kind: "advisory", ID: id, Title: id, Body: "Unresolved child target: " + id})
			continue
		}
		children = append(children, child)
	}
	return children
}

func latestChildLogs(logs []reviewlog.Summary, childIDs []string) []reviewlog.Summary {
	childSet := map[string]bool{}
	for _, id := range childIDs {
		childSet[id] = true
	}
	if len(childSet) == 0 {
		return nil
	}
	latest := map[string]reviewlog.Summary{}
	for _, log := range logs {
		if !childSet[log.Target] {
			continue
		}
		current, ok := latest[log.Target]
		if !ok || logSortKey(log) > logSortKey(current) {
			latest[log.Target] = log
		}
	}
	filtered := make([]reviewlog.Summary, 0, len(latest))
	for _, id := range childIDs {
		if log, ok := latest[id]; ok {
			filtered = append(filtered, log)
		}
	}
	return filtered
}

func childOpenBlockerLogs(root string, childIDs []string) ([]reviewlog.Summary, error) {
	childSet := map[string]bool{}
	for _, id := range childIDs {
		childSet[id] = true
	}
	if len(childSet) == 0 {
		return nil, nil
	}
	blockers, err := findings.UnresolvedBlocking(root)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var logs []reviewlog.Summary
	for _, blocker := range blockers {
		id := findingTargetID(blocker.Target)
		if !childSet[id] || seen[id] {
			continue
		}
		seen[id] = true
		logs = append(logs, reviewlog.Summary{
			Target:                id,
			Verdict:               "NEEDS_REVISION",
			FindingIDs:            []string{blocker.ID},
			HasUnresolvedBlockers: true,
		})
	}
	return logs, nil
}

func findingTargetID(target any) string {
	switch typed := target.(type) {
	case map[string]any:
		if id, ok := typed["id"].(string); ok && id != "" {
			return id
		}
		if path, ok := typed["path"].(string); ok {
			return path
		}
	case map[string]string:
		if typed["id"] != "" {
			return typed["id"]
		}
		return typed["path"]
	}
	return ""
}

func logSortKey(log reviewlog.Summary) string {
	if log.RunID != "" {
		return log.RunID
	}
	return log.Path
}

func reviewerChildren(children []tasksource.Source) []reviewers.EpicChild {
	result := make([]reviewers.EpicChild, 0, len(children))
	for _, child := range children {
		result = append(result, reviewers.EpicChild{ID: child.ID, Title: child.Title, Body: child.Body})
	}
	return result
}

func reviewerLogs(logs []reviewlog.Summary) []reviewers.EpicReviewLog {
	result := make([]reviewers.EpicReviewLog, 0, len(logs))
	for _, log := range logs {
		result = append(result, reviewers.EpicReviewLog{
			Target:                log.Target,
			Verdict:               log.Verdict,
			FindingIDs:            log.FindingIDs,
			HasUnresolvedBlockers: log.HasUnresolvedBlockers,
		})
	}
	return result
}

func filterGeneratedGitContext(git gitcontext.Context) gitcontext.Context {
	git.ChangedFiles = filterGeneratedFiles(git.ChangedFiles)
	git.StagedFiles = filterGeneratedFiles(git.StagedFiles)
	git.UnstagedFiles = filterGeneratedFiles(git.UnstagedFiles)
	git.WorkingTreeFiles = filterGeneratedFiles(git.WorkingTreeFiles)
	git.UntrackedFiles = filterGeneratedFiles(git.UntrackedFiles)
	git.Diff = filterGeneratedDiff(git.Diff)
	git.StagedDiff = filterGeneratedDiff(git.StagedDiff)
	git.WorkingTreeDiff = filterGeneratedDiff(git.WorkingTreeDiff)
	git.UntrackedExcerpts = filterGeneratedUntrackedExcerpts(git.UntrackedExcerpts)
	return git
}

func filterGeneratedFiles(files []string) []string {
	filtered := make([]string, 0, len(files))
	for _, file := range files {
		if isGeneratedMetareviewPath(file) {
			continue
		}
		filtered = append(filtered, file)
	}
	return filtered
}

func filterGeneratedUntrackedExcerpts(text string) string {
	var sections []string
	var current []string
	flush := func() {
		if len(current) == 0 {
			return
		}
		header := strings.TrimPrefix(current[0], "--- ")
		if !isGeneratedMetareviewPath(header) {
			sections = append(sections, strings.Join(current, "\n"))
		}
		current = nil
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "--- ") {
			flush()
		}
		if line != "" || len(current) > 0 {
			current = append(current, line)
		}
	}
	flush()
	return strings.Join(sections, "\n")
}

func filterGeneratedDiff(text string) string {
	var sections []string
	var current []string
	flush := func() {
		if len(current) == 0 {
			return
		}
		if !isGeneratedDiffSection(current[0]) {
			sections = append(sections, strings.Join(current, "\n"))
		}
		current = nil
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			flush()
		}
		if line != "" || len(current) > 0 {
			current = append(current, line)
		}
	}
	flush()
	return strings.Join(sections, "\n")
}

func isGeneratedDiffSection(header string) bool {
	fields := strings.Fields(header)
	if len(fields) < 4 {
		return false
	}
	path := strings.TrimPrefix(fields[2], "a/")
	return isGeneratedMetareviewPath(path)
}

func isGeneratedMetareviewPath(path string) bool {
	return strings.HasPrefix(path, ".metareview/") ||
		path == ".metareview" ||
		strings.HasPrefix(path, "docs/metareview/")
}

func readEvidence(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	text := string(bytes)
	if len(text) > 12000 {
		return text[:12000], nil
	}
	return text, nil
}

func uniquePaths(root, target string, at time.Time) (string, string, string, error) {
	runAt := at
	for {
		runID := state.RunID("epic-ready", target, runAt)
		contextRel := filepath.ToSlash(filepath.Join("docs", "metareview", "context", runID+"-context.md"))
		reviewRel := filepath.ToSlash(filepath.Join("docs", "metareview", "reviews", runID+".md"))
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(reviewRel))); os.IsNotExist(err) {
			return runID, contextRel, reviewRel, nil
		}
		runAt = runAt.Add(time.Nanosecond)
	}
}

func contextMarkdown(runID string, epic epicsource.Source, children []tasksource.Source, git gitcontext.Context, knowledgeContext knowledge.Context, logs []reviewlog.Summary, evidenceText, gateEffect string) string {
	changed := append([]string{}, git.ChangedFiles...)
	changed = append(changed, git.StagedFiles...)
	changed = append(changed, git.WorkingTreeFiles...)
	changed = append(changed, git.UntrackedFiles...)
	return "# metareview epic-ready context\n\n" +
		"Run ID: " + markdown.InlineCode(runID) + "\n\n" +
		"## Epic\n\n" + epic.Body + "\n\n" +
		"## Children\n\n" + childrenMarkdown(children) + "\n\n" +
		"## Git\n\n" +
		"- Base: " + markdown.InlineCode(git.BaseSHA) + "\n" +
		"- Head: " + markdown.InlineCode(git.HeadSHA) + "\n" +
		"- Branch: " + markdown.InlineCode(git.Branch) + "\n" +
		"- Gate effect: " + markdown.InlineCode(gateEffect) + "\n\n" +
		"## Changed Files\n\n" + markdownList(changed, "No changed files.") + "\n\n" +
		"## Diff\n\n" + markdown.FencedCodeBlock("diff", strings.Join([]string{git.Diff, git.StagedDiff, git.WorkingTreeDiff, git.UntrackedExcerpts}, "\n")) + "\n\n" +
		"## Child Review Logs\n\n" + reviewLogsMarkdown(logs) + "\n\n" +
		"## Knowledge And Registries\n\n" + knowledgeMarkdown(knowledgeContext) + "\n\n" +
		"## Evidence\n\n" + firstNonEmpty(evidenceText, "No external validation evidence supplied.") + "\n"
}

func childrenMarkdown(children []tasksource.Source) string {
	if len(children) == 0 {
		return "No child tasks discovered."
	}
	sections := make([]string, 0, len(children))
	for _, child := range children {
		sections = append(sections, "### "+firstNonEmpty(child.ID, child.Title)+"\n\n"+child.Body)
	}
	return strings.Join(sections, "\n\n")
}

func reviewLogsMarkdown(logs []reviewlog.Summary) string {
	if len(logs) == 0 {
		return "No child review logs discovered."
	}
	lines := make([]string, 0, len(logs))
	for _, log := range logs {
		lines = append(lines, fmt.Sprintf("- %s: %s (%s)", log.Target, log.Verdict, log.Path))
	}
	return strings.Join(lines, "\n")
}

func knowledgeMarkdown(context knowledge.Context) string {
	service := "Service inventory: none\n\nNo service inventory found."
	if context.ServiceInventoryPath != "" {
		service = "Service inventory: " + markdown.InlineCode(context.ServiceInventoryPath) + "\n\n" + context.ServiceInventory
	}
	facts := "No Beads knowledge facts found."
	if len(context.Facts) > 0 {
		lines := make([]string, 0, len(context.Facts))
		for _, fact := range context.Facts {
			lines = append(lines, "- "+fact.Source+": "+fact.Text)
		}
		facts = strings.Join(lines, "\n")
	}
	return service + "\n\nKnowledge facts:\n\n" + facts
}

func reviewMarkdown(runID, target, contextRel, previousRun, gateEffect, verdict string, records []findings.Record) string {
	return "# metareview: epic-ready review\n\n" +
		"Run ID: " + markdown.InlineCode(runID) + "\n\n" +
		"Target: " + markdown.InlineCode(target) + "\n\n" +
		"Context pack: " + markdown.InlineCode(contextRel) + "\n\n" +
		"Execution mode: " + markdown.InlineCode("deterministic-local") + "\n\n" +
		"Gate effect: " + markdown.InlineCode(gateEffect) + "\n\n" +
		"Previous run: " + markdown.InlineCode(firstNonEmpty(previousRun, "none")) + "\n\n" +
		"## Verdict\n\n" + verdict + "\n\n" +
		"## Reviewer Results\n\n| Reviewer | Verdict | Blocking | Notes |\n| --- | --- | ---: | --- |\n" +
		reviewerTable(records) + "\n\n" +
		"## Findings\n\n" + findingsMarkdown(records) + "\n"
}

func reviewerTable(records []findings.Record) string {
	lines := make([]string, 0, len(reviewerNames))
	for _, reviewer := range reviewerNames {
		var titles []string
		for _, record := range records {
			if record.Reviewer == reviewer {
				titles = append(titles, record.Title)
			}
		}
		verdict := "PASS"
		if len(titles) > 0 {
			verdict = "NEEDS_REVISION"
		}
		note := "No blocking findings."
		if len(titles) > 0 {
			note = strings.Join(titles, "; ")
		}
		lines = append(lines, fmt.Sprintf("| %s | %s | %d | %s |", reviewer, verdict, len(titles), note))
	}
	return strings.Join(lines, "\n")
}

func findingsMarkdown(records []findings.Record) string {
	if len(records) == 0 {
		return "No blocking findings.\n"
	}
	sections := make([]string, 0, len(records))
	for _, record := range records {
		sections = append(sections, "### "+record.ID+": "+record.Title+"\n\n"+
			"- Reviewer: "+record.Reviewer+"\n"+
			"- Severity: "+record.Severity+"\n"+
			"- Classification: "+record.Classification+"\n"+
			"- Finding: "+record.Finding+"\n"+
			"- Expected: "+record.Expected+"\n"+
			"- Found: "+record.Found+"\n"+
			"- Recommendation: "+record.Recommendation+"\n")
	}
	return strings.Join(sections, "\n")
}

func snapshot(path string) fileSnapshot {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return fileSnapshot{existed: false}
	}
	return fileSnapshot{existed: true, content: bytes}
}

func restoreSnapshots(snapshots map[string]fileSnapshot) {
	for path, snapshot := range snapshots {
		if snapshot.existed {
			_ = os.MkdirAll(filepath.Dir(path), 0o755)
			_ = os.WriteFile(path, snapshot.content, 0o644)
			continue
		}
		_ = os.Remove(path)
	}
}

func removeEmptyDirs(root string) {
	for _, rel := range []string{
		filepath.Join("docs", "metareview", "context"),
		filepath.Join("docs", "metareview", "reviews"),
		filepath.Join("docs", "metareview"),
		filepath.Join("docs"),
	} {
		_ = os.Remove(filepath.Join(root, rel))
	}
}

func markdownList(values []string, empty string) string {
	if len(values) == 0 {
		return empty
	}
	lines := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		lines = append(lines, "- "+value)
	}
	if len(lines) == 0 {
		return empty
	}
	return strings.Join(lines, "\n")
}

func sourceRefs(epic epicsource.Source) []map[string]string {
	refs := []map[string]string{{"type": epic.Kind, "id": epic.ID}}
	if epic.Path != "" {
		refs[0]["path"] = epic.Path
	}
	for _, child := range epic.ChildIDs {
		refs = append(refs, map[string]string{"type": "child", "id": child})
	}
	return refs
}

func epicTargetType(epic epicsource.Source) string {
	switch epic.Kind {
	case "beads":
		return "beads-epic"
	case "markdown":
		return "path"
	default:
		return "advisory"
	}
}

func findingIDs(records []findings.Record) []string {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.ID)
	}
	return ids
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
