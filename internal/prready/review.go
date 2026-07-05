package prready

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dsifry/metareview/internal/contextprofile"
	"github.com/dsifry/metareview/internal/evidence"
	"github.com/dsifry/metareview/internal/findings"
	"github.com/dsifry/metareview/internal/gitcontext"
	"github.com/dsifry/metareview/internal/githubcontext"
	"github.com/dsifry/metareview/internal/knowledge"
	"github.com/dsifry/metareview/internal/markdown"
	"github.com/dsifry/metareview/internal/repo"
	"github.com/dsifry/metareview/internal/reviewers"
	"github.com/dsifry/metareview/internal/reviewlog"
	"github.com/dsifry/metareview/internal/reviewmanifest"
	"github.com/dsifry/metareview/internal/reviewstate"
	"github.com/dsifry/metareview/internal/runchain"
	"github.com/dsifry/metareview/internal/state"
)

type Options struct {
	Base               string
	PreviousRunID      string
	EvidencePath       string
	GitHubPR           string
	MaxAttempts        int
	IncludeWorkingTree bool
	Now                time.Time
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

var reviewerNames = []string{"pr-readiness-reviewer", "validation-reviewer", "security-reviewer", "code-quality-reviewer", "architecture-reviewer", "external-reviewer"}

func Create(root string, options Options) (Result, error) {
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	report := repo.Detect(root)
	git, err := gitcontext.CollectWithExcludes(root, options.Base, generatedMetareviewPathExcludes())
	if err != nil {
		return Result{}, err
	}
	reviewGit := filterGeneratedGitContext(git)
	dirtyFiles := workingTreeDirtyFiles(reviewGit)
	analysisGit := reviewGit
	if !options.IncludeWorkingTree {
		analysisGit = branchOnlyGitContext(reviewGit)
	}
	profile := contextprofile.FromGit(analysisGit, contextprofile.Options{})
	knowledgeContext, err := knowledge.Collect(root)
	if err != nil {
		return Result{}, err
	}
	targetRecord := map[string]string{"type": "branch", "id": firstNonEmpty(git.Branch, git.HeadSHA)}
	logs, err := reviewlog.Discover(root)
	if err != nil {
		return Result{}, err
	}
	chain, previousRunIDs, err := resolveRunChain(root, targetRecord, options, logs, git)
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
	projection := reviewstate.ProjectRecords(logs, blockers, reviewstate.Options{
		Scope:            "pr-ready",
		Target:           targetRecord,
		PreviousRunIDs:   previousRunIDs,
		HistoricalRunIDs: historicalPRReadyRunIDsForCurrentTarget(root, logs, targetRecord, git),
		ChangedPaths:     reviewedPaths(analysisGit),
		CurrentTarget:    targetRecord,
	})
	reviewLogs := append(latestLogsByTarget(projection.CurrentReviewLogs()), blockerLogs(projection.CurrentBlockers())...)
	prEvidence := RenderEvidence(EvidenceInput{
		Summary:     branchSummary(analysisGit),
		Validation:  validationLines(evidenceText),
		TaskReviews: taskReviewEvidence(reviewLogs),
		EpicReviews: epicReviewEvidence(reviewLogs),
		Blockers:    blockerEvidence(projection.CurrentBlockers()),
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
	rawFindings := reviewers.RunPRReady(reviewerContext(analysisGit, profile, knowledgeContext, reviewLogs, evidenceText, prEvidence, ghCtx, options.IncludeWorkingTree, dirtyFiles))
	run := findings.Run{ID: runID, Scope: "pr-ready", Target: targetRecord, RepoRoot: root, GitHead: git.HeadSHA}

	result := Result{RunID: runID, ReviewRel: reviewRel, ContextRel: contextRel}
	err = func() error {
		if err := os.MkdirAll(filepath.Dir(contextPath), 0o755); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(reviewPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(contextPath, []byte(contextMarkdown(runID, analysisGit, profile, knowledgeContext, reviewLogs, evidenceText, ghCtx, prEvidence, gateEffect)), 0o644); err != nil {
			return err
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
			Scope:                "pr-ready",
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
			SourceRefs:           []map[string]string{{"type": "branch", "id": targetRecord["id"]}},
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
		return os.WriteFile(reviewPath, []byte(reviewMarkdown(runID, contextRel, options.PreviousRunID, gateEffect, verdict, reconciled.OpenFindings, prEvidence, meta)), 0o644)
	}()
	if err != nil {
		restoreSnapshots(snapshots)
		removeEmptyDirs(root)
		return Result{}, err
	}
	return result, nil
}

func resolveRunChain(root string, targetRecord map[string]string, options Options, logs []reviewlog.Summary, git gitcontext.Context) (runchain.Decision, []string, error) {
	chain, err := runchain.Resolve(root, runchain.Options{
		Scope:         "pr-ready",
		Target:        targetRecord,
		PreviousRunID: options.PreviousRunID,
		MaxAttempts:   options.MaxAttempts,
	})
	if err == nil {
		if options.PreviousRunID == "" && len(chain.Chain) == 0 {
			if escalated, ok := legacyEscalatedPRReadyForTarget(root, logs, targetRecord, git); ok {
				return runchain.Decision{}, nil, fmt.Errorf("same target already escalated in run %s", escalated)
			}
		}
		return chain, runIDsFromChain(chain.Chain), nil
	}
	if options.PreviousRunID == "" || !legacyRecoverableRunchainError(err) {
		return runchain.Decision{}, nil, err
	}
	previousRunIDs, legacyErr := legacyPreviousRunIDsForPRReady(root, logs, options.PreviousRunID, targetRecord, git)
	if legacyErr != nil {
		return runchain.Decision{}, nil, legacyErr
	}
	if len(previousRunIDs) == 0 {
		return runchain.Decision{}, nil, err
	}
	fallback, fallbackErr := runchain.Resolve(root, runchain.Options{
		Scope:       "pr-ready",
		Target:      targetRecord,
		MaxAttempts: options.MaxAttempts,
	})
	if fallbackErr != nil {
		return runchain.Decision{}, nil, fallbackErr
	}
	return fallback, previousRunIDs, nil
}

func legacyPreviousRunIDsForPRReady(root string, logs []reviewlog.Summary, previousRunID string, targetRecord map[string]string, git gitcontext.Context) ([]string, error) {
	ids := reviewstate.LegacyPreviousRunIDs(logs, previousRunID)
	if len(ids) == 0 {
		return nil, nil
	}
	byID := map[string]reviewlog.Summary{}
	for _, log := range logs {
		if log.RunID != "" {
			byID[log.RunID] = log
		}
	}
	for _, id := range ids {
		log, ok := byID[id]
		if !ok || log.Kind != "pr-ready" || !legacyPRReadyTargetMatches(root, log, targetRecord, git) {
			return nil, nil
		}
		if strings.EqualFold(log.Verdict, "ESCALATED") {
			return nil, fmt.Errorf("previous run %s already escalated", id)
		}
	}
	if escalated, ok := legacyEscalatedPRReadyForTarget(root, logs, targetRecord, git); ok {
		return nil, fmt.Errorf("same target already escalated in run %s", escalated)
	}
	return ids, nil
}

func legacyPRReadyTargetMatches(root string, log reviewlog.Summary, targetRecord map[string]string, git gitcontext.Context) bool {
	matches, known := legacyPRReadyTargetMatch(root, log, targetRecord, git)
	return known && matches
}

func legacyPRReadyTargetMatch(root string, log reviewlog.Summary, targetRecord map[string]string, git gitcontext.Context) (bool, bool) {
	target := strings.TrimSpace(log.Target)
	if target != "current branch" && target != targetRecord["id"] {
		return false, true
	}
	if target != "current branch" {
		return true, true
	}
	identity, err := readLegacyPRReadyContextIdentity(root, log.ContextRel)
	if err != nil {
		return false, false
	}
	if git.Branch == "" || identity.Branch == "" {
		return false, false
	}
	return identity.Branch == git.Branch, true
}

func historicalPRReadyRunIDsForCurrentTarget(root string, logs []reviewlog.Summary, targetRecord map[string]string, git gitcontext.Context) []string {
	var ids []string
	for _, log := range logs {
		if log.RunID == "" || log.Kind != "pr-ready" {
			continue
		}
		matches, known := legacyPRReadyTargetMatch(root, log, targetRecord, git)
		if known && !matches {
			ids = append(ids, log.RunID)
		}
	}
	return ids
}

func legacyEscalatedPRReadyForTarget(root string, logs []reviewlog.Summary, targetRecord map[string]string, git gitcontext.Context) (string, bool) {
	for _, log := range logs {
		if log.RunID == "" || log.Kind != "pr-ready" || !strings.EqualFold(log.Verdict, "ESCALATED") {
			continue
		}
		if legacyPRReadyTargetMatches(root, log, targetRecord, git) {
			return log.RunID, true
		}
	}
	return "", false
}

type legacyPRReadyContextIdentity struct {
	Branch string
	Head   string
}

func readLegacyPRReadyContextIdentity(root, rel string) (legacyPRReadyContextIdentity, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return legacyPRReadyContextIdentity{}, fmt.Errorf("legacy context path is required")
	}
	clean := filepath.Clean(rel)
	if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return legacyPRReadyContextIdentity{}, fmt.Errorf("legacy context path escapes repository: %s", rel)
	}
	bytes, err := os.ReadFile(filepath.Join(root, clean))
	if err != nil {
		return legacyPRReadyContextIdentity{}, err
	}
	var identity legacyPRReadyContextIdentity
	for _, line := range strings.Split(string(bytes), "\n") {
		switch {
		case strings.HasPrefix(line, "- Branch:"):
			identity.Branch = firstInlineCodeValue(line)
		case strings.HasPrefix(line, "- Head:"):
			identity.Head = firstInlineCodeValue(line)
		}
	}
	if identity.Branch == "" && identity.Head == "" {
		return legacyPRReadyContextIdentity{}, fmt.Errorf("legacy context lacks branch and head identity")
	}
	return identity, nil
}

func firstInlineCodeValue(line string) string {
	start := strings.Index(line, "`")
	if start < 0 {
		return ""
	}
	end := strings.Index(line[start+1:], "`")
	if end < 0 {
		return ""
	}
	return line[start+1 : start+1+end]
}

func runIDsFromChain(chain []runchain.Record) []string {
	ids := make([]string, 0, len(chain))
	for _, link := range chain {
		ids = append(ids, link.ID)
	}
	return ids
}

func legacyRecoverableRunchainError(err error) bool {
	message := err.Error()
	return strings.Contains(message, "previous run ") && (strings.Contains(message, " not found") || strings.Contains(message, " chain missing "))
}

func reviewerContext(git gitcontext.Context, profile contextprofile.Profile, knowledgeContext knowledge.Context, logs []reviewlog.Summary, evidenceText, prEvidence string, ghCtx githubcontext.Context, includeWorkingTree bool, dirtyFiles []string) reviewers.PRReadyContext {
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
			RawDiffBytes:             profile.RawDiffBytes,
			FilteredDiffBytes:        profile.FilteredDiffBytes,
			GeneratedExcludedFiles:   profile.GeneratedExcludedFiles,
			UntrackedOmittedCount:    profile.UntrackedOmittedCount,
			RiskLevel:                profile.RiskLevel,
			RiskReasons:              profile.RiskReasons,
		},
		Knowledge:             reviewerKnowledge(knowledgeContext),
		EvidenceText:          evidenceText,
		PREvidenceMarkdown:    prEvidence,
		ReviewLogs:            reviewerLogs(logs),
		GitHub:                reviewerGitHub(ghCtx),
		IncludeWorkingTree:    includeWorkingTree,
		WorkingTreeDirtyFiles: dirtyFiles,
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

func reviewedPaths(git gitcontext.Context) []string {
	var paths []string
	paths = append(paths, git.ChangedFiles...)
	paths = append(paths, git.StagedFiles...)
	paths = append(paths, git.UnstagedFiles...)
	paths = append(paths, git.WorkingTreeFiles...)
	paths = append(paths, git.UntrackedFiles...)
	return uniqueStrings(paths)
}

func workingTreeDirtyFiles(git gitcontext.Context) []string {
	var paths []string
	paths = append(paths, git.StagedFiles...)
	paths = append(paths, git.UnstagedFiles...)
	paths = append(paths, git.WorkingTreeFiles...)
	paths = append(paths, git.UntrackedFiles...)
	return uniqueStrings(paths)
}

func branchOnlyGitContext(git gitcontext.Context) gitcontext.Context {
	git.StagedFiles = nil
	git.UnstagedFiles = nil
	git.WorkingTreeFiles = nil
	git.UntrackedFiles = nil
	git.StagedStat = ""
	git.WorkingTreeStat = ""
	git.StagedDiff = ""
	git.WorkingTreeDiff = ""
	git.UntrackedExcerpts = ""
	git.StagedDiffTruncated = false
	git.WorkingTreeDiffTruncated = false
	git.UntrackedOmittedCount = 0
	git.RawDiffBytes = len(git.Diff)
	git.FilteredDiffBytes = len(git.Diff)
	return git
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func validationLines(text string) []string {
	bundle, err := evidence.Parse([]byte(text))
	if err == nil {
		if summaries := bundle.ValidationSummaries(); len(summaries) > 0 {
			return summaries
		}
	}
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

func generatedMetareviewPathExcludes() []string {
	return []string{".metareview", ".metareview/**", "docs/metareview", "docs/metareview/**"}
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

func contextMarkdown(runID string, git gitcontext.Context, profile contextprofile.Profile, knowledgeContext knowledge.Context, logs []reviewlog.Summary, evidenceText string, ghCtx githubcontext.Context, prEvidence, gateEffect string) string {
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
		contextprofile.Markdown(profile) + "\n\n" +
		contextprofile.ShardPlanMarkdown(profile, contextprofile.ShardOptions{MaxBytesPerShard: contextprofile.DefaultMaxBytesPerShard, GroupBy: "path"}) + "\n\n" +
		reviewManifestMarkdown("pr-ready", map[string]string{"type": "branch", "id": firstNonEmpty(git.Branch, git.HeadSHA)}, profile) + "\n\n" +
		"## Changed Files\n\n" + markdownList(changed, "No changed files.") + "\n\n" +
		"## Diff\n\n" + markdown.FencedCodeBlock("diff", strings.Join([]string{git.Diff, git.StagedDiff, git.WorkingTreeDiff, git.UntrackedExcerpts}, "\n")) + "\n\n" +
		"## Review Logs\n\n" + reviewLogsMarkdown(logs) + "\n\n" +
		"## Knowledge And Registries\n\n" + knowledgeMarkdown(knowledgeContext) + "\n\n" +
		"## Validation Evidence\n\n" + firstNonEmpty(evidenceText, "No external validation evidence supplied.") + "\n\n" +
		"## GitHub Context\n\n" + githubcontext.RenderMarkdown(ghCtx) + "\n\n" +
		"## Suggested PR Evidence\n\n" + prEvidence + "\n"
}

func reviewManifestMarkdown(scope string, target map[string]string, profile contextprofile.Profile) string {
	plan, err := contextprofile.PlanShards(profile, contextprofile.ShardOptions{MaxBytesPerShard: contextprofile.DefaultMaxBytesPerShard, GroupBy: "path"})
	if err != nil {
		return "## Review Manifest\n\nUnable to generate review manifest: " + err.Error()
	}
	manifest := reviewmanifest.Build(reviewmanifest.Input{
		Scope:            scope,
		Target:           target,
		Profile:          profile,
		ShardPlan:        plan,
		PathDispositions: reviewmanifest.GeneratedPathDispositions(profile.GeneratedExcludedFiles),
	})
	return reviewmanifest.Markdown(manifest, reviewmanifest.Aggregate(manifest))
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

func reviewMarkdown(runID, contextRel, previousRun, gateEffect, verdict string, records []findings.Record, prEvidence string, meta reviewMetadata) string {
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
		findingsMarkdown(records) + "\n" +
		"\n## Suggested PR Evidence\n\n" + prEvidence + "\n" +
		runChainMarkdown(runID, verdict, meta)
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
