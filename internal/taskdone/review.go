package taskdone

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dsifry/metareview/internal/findings"
	"github.com/dsifry/metareview/internal/gitcontext"
	"github.com/dsifry/metareview/internal/knowledge"
	"github.com/dsifry/metareview/internal/markdown"
	"github.com/dsifry/metareview/internal/repo"
	"github.com/dsifry/metareview/internal/reviewers"
	"github.com/dsifry/metareview/internal/runchain"
	"github.com/dsifry/metareview/internal/state"
	"github.com/dsifry/metareview/internal/tasksource"
)

type Options struct {
	Base          string
	PreviousRunID string
	EvidencePath  string
	MaxAttempts   int
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
	SchemaVersion        int                 `json:"schemaVersion"`
	ID                   string              `json:"id"`
	Scope                string              `json:"scope"`
	Target               map[string]string   `json:"target"`
	Status               string              `json:"status"`
	Verdict              string              `json:"verdict"`
	ExecutionMode        string              `json:"executionMode"`
	PreviousRunID        string              `json:"previousRunId,omitempty"`
	AttemptNumber        int                 `json:"attemptNumber"`
	MaxAttempts          int                 `json:"maxAttempts"`
	BaseSHA              string              `json:"baseSha"`
	HeadSHA              string              `json:"headSha"`
	ContextPath          string              `json:"contextPackPath"`
	ReviewPath           string              `json:"reviewLogPath"`
	Reviewers            []string            `json:"reviewers"`
	FindingIDs           []string            `json:"findingIds"`
	SourceRefs           []map[string]string `json:"sourceRefs"`
	GateEffect           string              `json:"gateEffect"`
	BlockingFindingCount int                 `json:"blockingFindingCount"`
	AdvisoryFindingCount int                 `json:"advisoryFindingCount"`
	FollowUpFindingCount int                 `json:"followUpFindingCount"`
	WarningFindingCount  int                 `json:"warningFindingCount"`
	EscalationReason     string              `json:"escalationReason"`
	CreatedAt            string              `json:"createdAt"`
	UpdatedAt            string              `json:"updatedAt"`
	RepoRoot             string              `json:"repoRoot"`
	GitHead              string              `json:"gitHead"`
}

type fileSnapshot struct {
	existed bool
	content []byte
}

var reviewerNames = []string{"code-quality-reviewer", "security-reviewer", "test-reviewer", "architecture-reviewer"}

func Create(root, target string, options Options) (Result, error) {
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	report := repo.Detect(root)
	task, err := tasksource.Resolve(root, target)
	if err != nil {
		return Result{}, err
	}
	git, err := gitcontext.CollectWithExcludes(root, options.Base, generatedMetareviewPathExcludes())
	if err != nil {
		return Result{}, err
	}
	reviewGit := filterGeneratedGitContext(git)
	knowledgeContext, err := knowledge.Collect(root)
	if err != nil {
		return Result{}, err
	}
	evidenceText, err := readEvidence(options.EvidencePath)
	if err != nil {
		return Result{}, err
	}

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
	rawFindings := reviewers.RunTaskDone(reviewerContext(task, reviewGit, knowledgeContext, evidenceText))
	targetRecord := map[string]string{"type": taskTargetType(task), "id": task.ID}
	run := findings.Run{ID: runID, Scope: "task-done", Target: targetRecord, RepoRoot: root, GitHead: git.HeadSHA}

	result := Result{RunID: runID, ReviewRel: reviewRel, ContextRel: contextRel}
	err = func() error {
		if err := os.MkdirAll(filepath.Dir(contextPath), 0o755); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(reviewPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(contextPath, []byte(contextMarkdown(runID, task, reviewGit, knowledgeContext, evidenceText, gateEffect)), 0o644); err != nil {
			return err
		}
		chain, err := runchain.Resolve(root, runchain.Options{
			Scope:         "task-done",
			Target:        targetRecord,
			PreviousRunID: options.PreviousRunID,
			MaxAttempts:   options.MaxAttempts,
		})
		if err != nil {
			return err
		}
		previousRunIDs := make([]string, 0, len(chain.Chain))
		for _, link := range chain.Chain {
			previousRunIDs = append(previousRunIDs, link.ID)
		}
		reconciled, err := findings.Reconcile(root, run, rawFindings, findings.Options{PreviousRunID: options.PreviousRunID, PreviousRunIDs: previousRunIDs})
		if err != nil {
			return err
		}
		counts := findings.CountByClass(reconciled.OpenFindings)
		verdict, status, blocking, escalationReason := verdictForCounts(counts, gateEffect, chain.AttemptNumber, chain.MaxAttempts)
		result.Verdict = verdict
		result.Blocking = blocking
		record := runRecord{
			SchemaVersion:        1,
			ID:                   runID,
			Scope:                "task-done",
			Target:               targetRecord,
			Status:               status,
			Verdict:              verdict,
			ExecutionMode:        "deterministic-local",
			PreviousRunID:        options.PreviousRunID,
			AttemptNumber:        chain.AttemptNumber,
			MaxAttempts:          chain.MaxAttempts,
			BaseSHA:              git.BaseSHA,
			HeadSHA:              git.HeadSHA,
			ContextPath:          contextRel,
			ReviewPath:           reviewRel,
			Reviewers:            reviewerNames,
			FindingIDs:           findingIDs(reconciled.OpenFindings),
			SourceRefs:           sourceRefs(task),
			GateEffect:           gateEffect,
			BlockingFindingCount: counts.Blocking,
			AdvisoryFindingCount: counts.Advisory,
			FollowUpFindingCount: counts.FollowUp,
			WarningFindingCount:  counts.Warnings,
			EscalationReason:     escalationReason,
			CreatedAt:            now.UTC().Format(time.RFC3339Nano),
			UpdatedAt:            now.UTC().Format(time.RFC3339Nano),
			RepoRoot:             root,
			GitHead:              git.HeadSHA,
		}
		if err := state.AppendJSONL(runsPath, record); err != nil {
			return err
		}
		meta := reviewMetadata{
			AttemptNumber:        chain.AttemptNumber,
			MaxAttempts:          chain.MaxAttempts,
			RunChain:             chain.Chain,
			BlockingFindingCount: counts.Blocking,
			AdvisoryFindingCount: counts.Advisory,
			FollowUpFindingCount: counts.FollowUp,
			WarningFindingCount:  counts.Warnings,
		}
		return os.WriteFile(reviewPath, []byte(reviewMarkdown(runID, target, contextRel, options.PreviousRunID, gateEffect, verdict, reconciled.OpenFindings, meta)), 0o644)
	}()
	if err != nil {
		restoreSnapshots(snapshots)
		removeEmptyDirs(root)
		return Result{}, err
	}
	return result, nil
}

func reviewerContext(task tasksource.Source, git gitcontext.Context, knowledgeContext knowledge.Context, evidenceText string) reviewers.Context {
	filteredGit := filterGeneratedGitContext(git)
	return reviewers.Context{
		Task: reviewers.TaskContext{Type: taskTargetType(task), ID: task.ID, Text: task.Body},
		Git: reviewers.GitContext{
			ChangedFiles:             filteredGit.ChangedFiles,
			StagedFiles:              filteredGit.StagedFiles,
			UnstagedFiles:            filteredGit.UnstagedFiles,
			WorkingTreeFiles:         filteredGit.WorkingTreeFiles,
			UntrackedFiles:           filteredGit.UntrackedFiles,
			Diff:                     filteredGit.Diff,
			StagedDiff:               filteredGit.StagedDiff,
			WorkingTreeDiff:          filteredGit.WorkingTreeDiff,
			UntrackedExcerpts:        filteredGit.UntrackedExcerpts,
			DiffTruncated:            filteredGit.DiffTruncated,
			StagedDiffTruncated:      filteredGit.StagedDiffTruncated,
			WorkingTreeDiffTruncated: filteredGit.WorkingTreeDiffTruncated,
		},
		Knowledge:    reviewerKnowledge(knowledgeContext),
		EvidenceText: evidenceText,
	}
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

func generatedMetareviewPathExcludes() []string {
	return []string{".metareview", ".metareview/**", "docs/metareview", "docs/metareview/**"}
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
		runID := state.RunID("task-done", target, runAt)
		contextRel := filepath.ToSlash(filepath.Join("docs", "metareview", "context", runID+"-context.md"))
		reviewRel := filepath.ToSlash(filepath.Join("docs", "metareview", "reviews", runID+".md"))
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(reviewRel))); os.IsNotExist(err) {
			return runID, contextRel, reviewRel, nil
		}
		runAt = runAt.Add(time.Nanosecond)
	}
}

func contextMarkdown(runID string, task tasksource.Source, git gitcontext.Context, knowledgeContext knowledge.Context, evidenceText, gateEffect string) string {
	changed := append([]string{}, git.ChangedFiles...)
	changed = append(changed, git.StagedFiles...)
	changed = append(changed, git.WorkingTreeFiles...)
	changed = append(changed, git.UntrackedFiles...)
	return "# metareview task-done context\n\n" +
		"Run ID: " + markdown.InlineCode(runID) + "\n\n" +
		"## Task\n\n" + task.Body + "\n\n" +
		"## Git\n\n" +
		"- Base: " + markdown.InlineCode(git.BaseSHA) + "\n" +
		"- Head: " + markdown.InlineCode(git.HeadSHA) + "\n" +
		"- Branch: " + markdown.InlineCode(git.Branch) + "\n" +
		"- Gate effect: " + markdown.InlineCode(gateEffect) + "\n\n" +
		"## Changed Files\n\n" + markdownList(changed, "No changed files.") + "\n\n" +
		"## Diff\n\n" + markdown.FencedCodeBlock("diff", strings.Join([]string{git.Diff, git.StagedDiff, git.WorkingTreeDiff, git.UntrackedExcerpts}, "\n")) + "\n\n" +
		"## Knowledge And Registries\n\n" + knowledgeMarkdown(knowledgeContext) + "\n\n" +
		"## Evidence\n\n" + firstNonEmpty(evidenceText, "No external validation evidence supplied.") + "\n"
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

type reviewMetadata struct {
	AttemptNumber        int
	MaxAttempts          int
	RunChain             []runchain.Record
	BlockingFindingCount int
	AdvisoryFindingCount int
	FollowUpFindingCount int
	WarningFindingCount  int
}

func verdictForCounts(counts findings.ClassCounts, gateEffect string, attemptNumber, maxAttempts int) (string, string, bool, string) {
	blocking := counts.Blocking > 0
	nonBlocking := counts.Advisory > 0 || counts.FollowUp > 0 || counts.Warnings > 0
	if blocking && attemptNumber >= maxAttempts {
		reason := fmt.Sprintf("blocking findings remain after attempt %d of %d", attemptNumber, maxAttempts)
		return "ESCALATED", "escalated", true, reason
	}
	if blocking {
		return "NEEDS_REVISION", "needs-revision", true, ""
	}
	if gateEffect == "advisory" || nonBlocking {
		return "PASS_ADVISORY", "passed", false, ""
	}
	return "PASS", "passed", false, ""
}

func reviewMarkdown(runID, target, contextRel, previousRun, gateEffect, verdict string, records []findings.Record, meta reviewMetadata) string {
	return "# metareview: task-done review\n\n" +
		"Run ID: " + markdown.InlineCode(runID) + "\n\n" +
		"Target: " + markdown.InlineCode(target) + "\n\n" +
		"Context pack: " + markdown.InlineCode(contextRel) + "\n\n" +
		"Execution mode: " + markdown.InlineCode("deterministic-local") + "\n\n" +
		"Gate effect: " + markdown.InlineCode(gateEffect) + "\n\n" +
		"Previous run: " + markdown.InlineCode(firstNonEmpty(previousRun, "none")) + "\n\n" +
		"## Verdict\n\n" + verdict + "\n\n" +
		"## Reviewer Results\n\n| Reviewer | Verdict | Blocking | Notes |\n| --- | --- | ---: | --- |\n" +
		reviewerTable(records) + "\n\n" +
		findingsMarkdown(records) + "\n" +
		runChainMarkdown(runID, verdict, meta)
}

func reviewerTable(records []findings.Record) string {
	lines := make([]string, 0, len(reviewerNames))
	for _, reviewer := range reviewerNames {
		var blockers, nonBlockers []string
		for _, record := range records {
			if record.Reviewer != reviewer {
				continue
			}
			counts := findings.CountByClass([]findings.Record{record})
			if counts.Blocking > 0 {
				blockers = append(blockers, record.Title)
			} else {
				nonBlockers = append(nonBlockers, record.Title)
			}
		}
		verdict := "PASS"
		note := "No blocking findings."
		if len(blockers) > 0 {
			verdict = "NEEDS_REVISION"
			note = strings.Join(blockers, "; ")
		} else if len(nonBlockers) > 0 {
			verdict = "PASS_ADVISORY"
			note = strings.Join(nonBlockers, "; ")
		}
		lines = append(lines, fmt.Sprintf("| %s | %s | %d | %s |", reviewer, verdict, len(blockers), note))
	}
	return strings.Join(lines, "\n")
}

func findingsMarkdown(records []findings.Record) string {
	return classifiedFindingsMarkdown(records)
}

func classifiedFindingsMarkdown(records []findings.Record) string {
	sections := []struct {
		title string
		label string
	}{
		{title: "## Blocking Findings", label: "blocking"},
		{title: "## Advisory Findings", label: "advisory"},
		{title: "## Follow-up Findings", label: "follow-up"},
		{title: "## Warnings", label: "warning"},
	}
	var output []string
	for _, section := range sections {
		var items []string
		for _, record := range records {
			if classForDisplay(record) != section.label {
				continue
			}
			items = append(items, "### "+record.ID+": "+record.Title+"\n\n"+
				"- Reviewer: "+record.Reviewer+"\n"+
				"- Severity: "+record.Severity+"\n"+
				"- Classification: "+record.Classification+"\n"+
				"- Finding: "+record.Finding+"\n"+
				"- Expected: "+record.Expected+"\n"+
				"- Found: "+record.Found+"\n"+
				"- Recommendation: "+record.Recommendation+"\n")
		}
		body := "No findings in this class.\n"
		if len(items) > 0 {
			body = strings.Join(items, "\n")
		}
		output = append(output, section.title+"\n\n"+body)
	}
	return strings.Join(output, "\n\n")
}

func classForDisplay(record findings.Record) string {
	counts := findings.CountByClass([]findings.Record{record})
	switch {
	case counts.Blocking > 0:
		return "blocking"
	case counts.Advisory > 0:
		return "advisory"
	case counts.FollowUp > 0:
		return "follow-up"
	default:
		return "warning"
	}
}

func runChainMarkdown(runID, verdict string, meta reviewMetadata) string {
	if verdict != "ESCALATED" {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("\n## Run Chain\n\n")
	for _, link := range meta.RunChain {
		builder.WriteString(fmt.Sprintf("- %s: %s attempt %d/%d\n", link.ID, link.Verdict, link.AttemptNumber, link.MaxAttempts))
	}
	builder.WriteString(fmt.Sprintf("- %s: %s attempt %d/%d\n", runID, verdict, meta.AttemptNumber, meta.MaxAttempts))
	builder.WriteString("\n## Unresolved Blocker Summary\n\n")
	builder.WriteString(fmt.Sprintf("- Blocking: %d\n- Advisory: %d\n- Follow-up: %d\n- Warnings: %d\n", meta.BlockingFindingCount, meta.AdvisoryFindingCount, meta.FollowUpFindingCount, meta.WarningFindingCount))
	return builder.String()
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

func sourceRefs(task tasksource.Source) []map[string]string {
	if task.Path != "" {
		return []map[string]string{{"type": task.Kind, "path": task.Path}}
	}
	return []map[string]string{{"type": task.Kind, "id": task.ID}}
}

func taskTargetType(task tasksource.Source) string {
	switch task.Kind {
	case "beads":
		return "beads-task"
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
