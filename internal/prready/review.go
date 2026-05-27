package prready

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dsifry/metareview/internal/findings"
	"github.com/dsifry/metareview/internal/gitcontext"
	"github.com/dsifry/metareview/internal/githubcontext"
	"github.com/dsifry/metareview/internal/knowledge"
	"github.com/dsifry/metareview/internal/markdown"
	"github.com/dsifry/metareview/internal/repo"
	"github.com/dsifry/metareview/internal/reviewers"
	"github.com/dsifry/metareview/internal/reviewlog"
	"github.com/dsifry/metareview/internal/state"
)

type Options struct {
	Base          string
	PreviousRunID string
	EvidencePath  string
	GitHubPR      string
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

var reviewerNames = []string{"pr-readiness-reviewer", "validation-reviewer", "security-reviewer", "code-quality-reviewer", "architecture-reviewer", "external-reviewer"}

func Create(root string, options Options) (Result, error) {
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	report := repo.Detect(root)
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
	blockers, err := findings.UnresolvedBlocking(root)
	if err != nil {
		return Result{}, err
	}
	evidenceText, err := readEvidence(options.EvidencePath)
	if err != nil {
		return Result{}, err
	}
	ghCtx, err := githubcontext.Collect(root, options.GitHubPR)
	if err != nil {
		return Result{}, err
	}
	reviewLogs := append(latestLogsByTarget(logs), blockerLogs(blockers)...)
	prEvidence := RenderEvidence(EvidenceInput{
		Summary:     branchSummary(reviewGit),
		Validation:  validationLines(evidenceText),
		TaskReviews: taskReviewEvidence(reviewLogs),
		EpicReviews: epicReviewEvidence(reviewLogs),
		Blockers:    blockerEvidence(blockers),
		GitHub:      ghCtx,
	})

	runID, contextRel, reviewRel, err := uniquePaths(root, now)
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
	rawFindings := reviewers.RunPRReady(reviewerContext(reviewGit, knowledgeContext, reviewLogs, evidenceText, prEvidence, ghCtx))
	targetRecord := map[string]string{"type": "branch", "id": firstNonEmpty(git.Branch, git.HeadSHA)}
	run := findings.Run{ID: runID, Scope: "pr-ready", Target: targetRecord, RepoRoot: root, GitHead: git.HeadSHA}

	result := Result{RunID: runID, ReviewRel: reviewRel, ContextRel: contextRel}
	err = func() error {
		if err := os.MkdirAll(filepath.Dir(contextPath), 0o755); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(reviewPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(contextPath, []byte(contextMarkdown(runID, reviewGit, knowledgeContext, reviewLogs, evidenceText, ghCtx, prEvidence, gateEffect)), 0o644); err != nil {
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
			Scope:         "pr-ready",
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
			SourceRefs:    []map[string]string{{"type": "branch", "id": targetRecord["id"]}},
			GateEffect:    gateEffect,
			CreatedAt:     now.UTC().Format(time.RFC3339Nano),
			UpdatedAt:     now.UTC().Format(time.RFC3339Nano),
			RepoRoot:      root,
			GitHead:       git.HeadSHA,
		}
		if err := state.AppendJSONL(runsPath, record); err != nil {
			return err
		}
		return os.WriteFile(reviewPath, []byte(reviewMarkdown(runID, contextRel, options.PreviousRunID, gateEffect, verdict, reconciled.Findings, prEvidence)), 0o644)
	}()
	if err != nil {
		restoreSnapshots(snapshots)
		removeEmptyDirs(root)
		return Result{}, err
	}
	return result, nil
}

func reviewerContext(git gitcontext.Context, knowledgeContext knowledge.Context, logs []reviewlog.Summary, evidenceText, prEvidence string, ghCtx githubcontext.Context) reviewers.PRReadyContext {
	return reviewers.PRReadyContext{
		Git: reviewers.GitContext{
			ChangedFiles:             git.ChangedFiles,
			StagedFiles:              git.StagedFiles,
			UnstagedFiles:            git.UnstagedFiles,
			WorkingTreeFiles:         git.WorkingTreeFiles,
			UntrackedFiles:           git.UntrackedFiles,
			Diff:                     git.Diff,
			StagedDiff:               git.StagedDiff,
			WorkingTreeDiff:          git.WorkingTreeDiff,
			UntrackedExcerpts:        git.UntrackedExcerpts,
			DiffTruncated:            git.DiffTruncated,
			StagedDiffTruncated:      git.StagedDiffTruncated,
			WorkingTreeDiffTruncated: git.WorkingTreeDiffTruncated,
		},
		Knowledge:          reviewerKnowledge(knowledgeContext),
		EvidenceText:       evidenceText,
		PREvidenceMarkdown: prEvidence,
		ReviewLogs:         reviewerLogs(logs),
		GitHub:             reviewerGitHub(ghCtx),
	}
}

func reviewerKnowledge(context knowledge.Context) reviewers.KnowledgeContext {
	facts := make([]reviewers.KnowledgeFact, 0, len(context.Facts))
	for _, fact := range context.Facts {
		facts = append(facts, reviewers.KnowledgeFact{Source: fact.Source, Text: fact.Text})
	}
	return reviewers.KnowledgeContext{
		ServiceInventoryPath: context.ServiceInventoryPath,
		ServiceInventory:     context.ServiceInventory,
		Facts:                facts,
	}
}

func reviewerLogs(logs []reviewlog.Summary) []reviewers.PRReviewLog {
	result := make([]reviewers.PRReviewLog, 0, len(logs))
	for _, log := range logs {
		result = append(result, reviewers.PRReviewLog{
			Target:                log.Target,
			Verdict:               log.Verdict,
			FindingIDs:            log.FindingIDs,
			HasUnresolvedBlockers: log.HasUnresolvedBlockers,
		})
	}
	return result
}

func reviewerGitHub(context githubcontext.Context) reviewers.PRGitHubContext {
	entries := make([]reviewers.PRGitHubEntry, 0, len(context.Comments)+len(context.Reviews))
	for _, item := range context.Comments {
		entries = append(entries, reviewers.PRGitHubEntry{Author: item.Author, URL: item.URL, Body: item.Body})
	}
	for _, item := range context.Reviews {
		entries = append(entries, reviewers.PRGitHubEntry{Author: item.Author, URL: item.URL, State: item.State, Body: item.Body})
	}
	return reviewers.PRGitHubContext{Available: context.Available, UnavailableReason: context.UnavailableReason, Entries: entries}
}

func latestLogsByTarget(logs []reviewlog.Summary) []reviewlog.Summary {
	latest := map[string]reviewlog.Summary{}
	for _, log := range logs {
		if log.Target == "" {
			continue
		}
		current, ok := latest[log.Target]
		if !ok || logSortKey(log) > logSortKey(current) {
			latest[log.Target] = log
		}
	}
	result := make([]reviewlog.Summary, 0, len(latest))
	for _, log := range latest {
		result = append(result, log)
	}
	return result
}

func blockerLogs(blockers []findings.Record) []reviewlog.Summary {
	result := make([]reviewlog.Summary, 0, len(blockers))
	for _, blocker := range blockers {
		target := findingTargetID(blocker.Target)
		if target == "" {
			target = blocker.ID
		}
		result = append(result, reviewlog.Summary{
			Target:                target,
			Verdict:               "NEEDS_REVISION",
			FindingIDs:            []string{blocker.ID},
			HasUnresolvedBlockers: true,
		})
	}
	return result
}

func taskReviewEvidence(logs []reviewlog.Summary) []ReviewEvidence {
	var result []ReviewEvidence
	for _, log := range logs {
		if looksEpicTarget(log.Target, log.Path) {
			continue
		}
		result = append(result, FromReviewLog(log))
	}
	return result
}

func epicReviewEvidence(logs []reviewlog.Summary) []ReviewEvidence {
	var result []ReviewEvidence
	for _, log := range logs {
		if looksEpicTarget(log.Target, log.Path) {
			result = append(result, FromReviewLog(log))
		}
	}
	return result
}

func blockerEvidence(blockers []findings.Record) []Blocker {
	result := make([]Blocker, 0, len(blockers))
	for _, blocker := range blockers {
		result = append(result, Blocker{ID: blocker.ID, Title: blocker.Title, Status: blocker.Status})
	}
	return result
}

func looksEpicTarget(target, path string) bool {
	text := strings.ToLower(target + "\n" + path)
	return strings.Contains(text, "epic")
}

func branchSummary(git gitcontext.Context) string {
	branch := firstNonEmpty(git.Branch, "current branch")
	if len(git.ChangedFiles) == 0 {
		return branch + " has no committed file changes in the reviewed diff."
	}
	return branch + " changes " + strings.Join(git.ChangedFiles, ", ")
}

func validationLines(text string) []string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, strings.TrimSpace(line))
		}
	}
	return lines
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

func uniquePaths(root string, at time.Time) (string, string, string, error) {
	runAt := at
	for {
		runID := state.RunID("pr-ready", "branch", runAt)
		contextRel := filepath.ToSlash(filepath.Join("docs", "metareview", "context", runID+"-context.md"))
		reviewRel := filepath.ToSlash(filepath.Join("docs", "metareview", "reviews", runID+".md"))
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(reviewRel))); os.IsNotExist(err) {
			return runID, contextRel, reviewRel, nil
		}
		runAt = runAt.Add(time.Nanosecond)
	}
}

func contextMarkdown(runID string, git gitcontext.Context, knowledgeContext knowledge.Context, logs []reviewlog.Summary, evidenceText string, ghCtx githubcontext.Context, prEvidence, gateEffect string) string {
	changed := append([]string{}, git.ChangedFiles...)
	changed = append(changed, git.StagedFiles...)
	changed = append(changed, git.WorkingTreeFiles...)
	changed = append(changed, git.UntrackedFiles...)
	return "# metareview pr-ready context\n\n" +
		"Run ID: " + markdown.InlineCode(runID) + "\n\n" +
		"## Git\n\n" +
		"- Base: " + markdown.InlineCode(git.BaseSHA) + "\n" +
		"- Head: " + markdown.InlineCode(git.HeadSHA) + "\n" +
		"- Branch: " + markdown.InlineCode(git.Branch) + "\n" +
		"- Gate effect: " + markdown.InlineCode(gateEffect) + "\n\n" +
		"## Changed Files\n\n" + markdownList(changed, "No changed files.") + "\n\n" +
		"## Diff\n\n" + markdown.FencedCodeBlock("diff", strings.Join([]string{git.Diff, git.StagedDiff, git.WorkingTreeDiff, git.UntrackedExcerpts}, "\n")) + "\n\n" +
		"## Review Logs\n\n" + reviewLogsMarkdown(logs) + "\n\n" +
		"## Knowledge And Registries\n\n" + knowledgeMarkdown(knowledgeContext) + "\n\n" +
		"## Validation Evidence\n\n" + firstNonEmpty(evidenceText, "No external validation evidence supplied.") + "\n\n" +
		"## GitHub Context\n\n" + githubcontext.RenderMarkdown(ghCtx) + "\n\n" +
		"## Suggested PR Evidence\n\n" + prEvidence + "\n"
}

func reviewMarkdown(runID, contextRel, previousRun, gateEffect, verdict string, records []findings.Record, prEvidence string) string {
	return "# metareview: pr-ready review\n\n" +
		"Run ID: " + markdown.InlineCode(runID) + "\n\n" +
		"Target: `current branch`\n\n" +
		"Context pack: " + markdown.InlineCode(contextRel) + "\n\n" +
		"Execution mode: " + markdown.InlineCode("deterministic-local") + "\n\n" +
		"Gate effect: " + markdown.InlineCode(gateEffect) + "\n\n" +
		"Previous run: " + markdown.InlineCode(firstNonEmpty(previousRun, "none")) + "\n\n" +
		"## Verdict\n\n" + verdict + "\n\n" +
		"## Reviewer Results\n\n| Reviewer | Verdict | Blocking | Notes |\n| --- | --- | ---: | --- |\n" +
		reviewerTable(records) + "\n\n" +
		"## Findings\n\n" + findingsMarkdown(records) + "\n" +
		"\n## Suggested PR Evidence\n\n" + prEvidence + "\n"
}

func reviewLogsMarkdown(logs []reviewlog.Summary) string {
	if len(logs) == 0 {
		return "No review logs discovered."
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
