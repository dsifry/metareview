package prready

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dsifry/metareview/internal/contextprofile"
	"github.com/dsifry/metareview/internal/evidence"
	"github.com/dsifry/metareview/internal/findings"
	"github.com/dsifry/metareview/internal/gitcontext"
	"github.com/dsifry/metareview/internal/githubcontext"
	"github.com/dsifry/metareview/internal/knowledge"
	"github.com/dsifry/metareview/internal/markdown"
	"github.com/dsifry/metareview/internal/mutation"
	"github.com/dsifry/metareview/internal/repo"
	"github.com/dsifry/metareview/internal/reviewers"
	"github.com/dsifry/metareview/internal/reviewlog"
	"github.com/dsifry/metareview/internal/reviewmanifest"
	"github.com/dsifry/metareview/internal/reviewstate"
	"github.com/dsifry/metareview/internal/runchain"
	"github.com/dsifry/metareview/internal/shardpack"
	"github.com/dsifry/metareview/internal/state"
	"github.com/dsifry/metareview/internal/version"
)

type Options struct {
	Base               string
	PreviousRunID      string
	EvidencePath       string
	GitHubPR           string
	MaxAttempts        int
	IncludeWorkingTree bool
	Now                time.Time
	// ShardResultPaths and CrossShardResultPaths are explicit --shard-result and
	// --cross-shard-result files, added to whatever the results directory holds.
	ShardResultPaths      []string
	CrossShardResultPaths []string
	// MutationReportPaths are --mutation-report files: a mutation-testing engine's output,
	// either mutation-testing-report-schema or gremlins JSON. Empty is the ordinary case.
	MutationReportPaths []string
	// ShardWriter is the pack-writing seam; nil uses the real filesystem.
	ShardWriter shardpack.Writer
}

type Result struct {
	RunID           string
	ReviewRel       string
	ContextRel      string
	Verdict         string
	Blocking        bool
	Reused          bool
	ReusedFromRunID string
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
	ReviewInputDigest    string              `json:"reviewInputDigest"`
	ReusedFromRunID      string              `json:"reusedFromRunId,omitempty"`
}

type fileSnapshot struct {
	existed bool
	content []byte
}

var reviewerNames = []string{"pr-readiness-reviewer", "validation-reviewer", "security-reviewer", "code-quality-reviewer", "architecture-reviewer", "external-reviewer"}
var runPRReadyReviewers = reviewers.RunPRReady

type reviewerInput struct {
	SchemaVersion          int                      `json:"schemaVersion"`
	Target                 map[string]string        `json:"target"`
	BaseSHA                string                   `json:"baseSha"`
	HeadSHA                string                   `json:"headSha"`
	GitHub                 githubcontext.Context    `json:"github"`
	EvidenceDigest         string                   `json:"evidenceDigest"`
	ReviewerImplementation string                   `json:"reviewerImplementation"`
	ReviewerNames          []string                 `json:"reviewers"`
	Context                reviewers.PRReadyContext `json:"context"`
	RelevantFindings       []findings.Record        `json:"relevantFindingFrontier"`
	GateEffect             string                   `json:"gateEffect"`
}

func canonicalReviewerInputDigest(input reviewerInput) (string, error) {
	input.SchemaVersion = 1
	// Metareview's own generated logs are deliberately excluded from the
	// reviewed diff. Their raw byte count and diagnostic path list change after
	// the first run, but they are not reviewer inputs and must not defeat reuse.
	input.Context.Git.RawDiffBytes = input.Context.Git.FilteredDiffBytes
	input.Context.Git.GeneratedExcludedFiles = nil
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("encode canonical reviewer input: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return fmt.Sprintf("sha256:%x", digest), nil
}

func contentDigest(content string) string {
	digest := sha256.Sum256([]byte(content))
	return fmt.Sprintf("sha256:%x", digest)
}

func canonicalFindingFrontier(records []findings.Record) []findings.Record {
	result := append([]findings.Record(nil), records...)
	sort.Slice(result, func(i, j int) bool {
		left := result[i].ID + "\x00" + result[i].RunID + "\x00" + result[i].Fingerprint
		right := result[j].ID + "\x00" + result[j].RunID + "\x00" + result[j].Fingerprint
		return left < right
	})
	return result
}

func validateExplicitPreviousRunInput(root string, logs []reviewlog.Summary, previousRunID string, targetRecord map[string]string, git gitcontext.Context) error {
	previousRunID = strings.TrimSpace(previousRunID)
	if previousRunID == "" {
		return nil
	}
	for _, log := range logs {
		if log.RunID != previousRunID {
			continue
		}
		if !log.RunRecordAuthenticated && !committedPRReadyInputAuthenticated(root, log, targetRecord, git) {
			return fmt.Errorf("previous run %s has a stale or unauthenticated reviewer input digest", previousRunID)
		}
		return nil
	}
	return fmt.Errorf("previous run %s not found", previousRunID)
}

func reusableVerdict(logs []reviewlog.Summary, target map[string]string, baseSHA, headSHA, digest string) (reviewlog.Summary, bool) {
	for i := len(logs) - 1; i >= 0; i-- {
		log := logs[i]
		if !log.RunRecordAuthenticated || log.Kind != "pr-ready" || log.Status != "passed" {
			continue
		}
		if log.Verdict != "PASS" && log.Verdict != "PASS_ADVISORY" {
			continue
		}
		if log.BaseSHA != baseSHA || log.HeadSHA != headSHA || log.ReviewInputDigest != digest || !sameTarget(log.TargetRecord, target) {
			continue
		}
		if !sameStrings(log.Reviewers, reviewerNames) {
			continue
		}
		if log.ExecutionMode != "deterministic-local" && log.ExecutionMode != "deterministic-local-reused" {
			continue
		}
		return log, true
	}
	return reviewlog.Summary{}, false
}

func sameTarget(left, right map[string]string) bool {
	return strings.TrimSpace(left["type"]) == strings.TrimSpace(right["type"]) &&
		strings.TrimSpace(firstNonEmpty(left["id"], left["path"])) == strings.TrimSpace(firstNonEmpty(right["id"], right["path"]))
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

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
	shardPlan, err := contextprofile.PlanShards(profile, analysisGit.BranchFiles, contextprofile.ShardOptions{
		MaxBytesPerShard: contextprofile.DefaultMaxBytesPerShard,
		Scope:            "pr-ready",
		TargetID:         shardTargetID(analysisGit),
	})
	if err != nil {
		return Result{}, err
	}
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
	// The full ledger (every status) lets the evidence renderer reconcile a
	// historical review against how its findings were actually cleared (#40). Read
	// before this run reconciles: the overrides/fixes that clear a historical
	// review were recorded by earlier runs and are already on disk.
	allFindings, err := findings.All(root)
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
	linkedTargets := []map[string]string{}
	if ghCtx.PRNumber != "" {
		linkedTargets = append(linkedTargets, map[string]string{"type": "pull-request", "id": ghCtx.PRNumber})
	}
	projection := reviewstate.ProjectRecords(logs, blockers, reviewstate.Options{
		Scope:            "pr-ready",
		Target:           targetRecord,
		PreviousRunIDs:   previousRunIDs,
		HistoricalRunIDs: historicalPRReadyRunIDsForCurrentTarget(root, logs, targetRecord, git),
		ChangedPaths:     reviewedPaths(analysisGit),
		CurrentTarget:    targetRecord,
		LinkedTargets:    linkedTargets,
	})
	reviewLogs := append(latestLogsByTarget(projection.CurrentReviewLogs()), blockerLogs(projection.CurrentBlockers())...)
	blockingReviewLogs := gateReviewLogs(reviewLogs, allFindings)
	prEvidence := RenderEvidence(EvidenceInput{
		Summary:     branchSummary(analysisGit),
		Validation:  validationLines(evidenceText),
		TaskReviews: taskReviewEvidence(reviewLogs),
		EpicReviews: epicReviewEvidence(reviewLogs),
		Blockers:    blockerEvidence(projection.CurrentBlockers()),
		GitHub:      ghCtx,
		Findings:    allFindings,
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
	// Packs are written before the review body so the context pack can name them,
	// and are undone by packRollback if anything later in the run fails.
	packWriter := options.ShardWriter
	if packWriter == nil {
		packWriter = shardpack.New(shardpack.OSDeps())
	}
	manifest, aggregate, err := ingestShardResults(root, packWriter, shardPlan, profile, analysisGit, options)
	if err != nil {
		return Result{}, err
	}
	mutationContext, err := mutationContextFor(options.MutationReportPaths)
	if err != nil {
		return Result{}, err
	}
	reviewerCtx := reviewerContext(analysisGit, profile, knowledgeContext, blockingReviewLogs, evidenceText, prEvidence, ghCtx, options.IncludeWorkingTree, dirtyFiles, manifestContext(manifest, aggregate))
	// Build B: require a real adjudicated lens review over THIS head (the deterministic checks above are not a
	// review). Resolve the review-evidence marker the FSM review-loop records; RunPRReady blocks when it is
	// missing/not-clean unless the mechanical-pass escape is set.
	reviewerCtx.RequireLenses = reviewstate.RequireAdjudicatedReview()
	reviewerCtx.Adversarial = reviewers.AdversarialReviewStatus{
		HeadSHA: git.HeadSHA,
		// A marker attests the committed base..HEAD only; when this run folds in a dirty working tree, that
		// content is unattested and the marker must not satisfy the gate.
		WorkingTreeUnattested: options.IncludeWorkingTree && len(dirtyFiles) > 0,
	}
	// A corrupt runs.jsonl never reaches here silently: the run projection above reads the same file and
	// fails the whole review loudly with the parse error first (fail-closed). So a read error here can only
	// mean "no marker" — treat it as absent.
	if ev, ok, evErr := reviewstate.LatestReviewEvidence(root, "pr-ready", git.BaseSHA, git.HeadSHA); evErr == nil && ok {
		reviewerCtx.Adversarial.Present = true
		reviewerCtx.Adversarial.Verdict = ev.AdjudicatedVerdict
		reviewerCtx.Adversarial.Emulated = ev.IsEmulated()
	}
	reviewerCtx.Mutation = mutationContext
	reviewInputDigest, err := canonicalReviewerInputDigest(reviewerInput{
		Target:                 targetRecord,
		BaseSHA:                git.BaseSHA,
		HeadSHA:                git.HeadSHA,
		GitHub:                 ghCtx,
		EvidenceDigest:         contentDigest(evidenceText),
		ReviewerImplementation: version.Version,
		ReviewerNames:          reviewerNames,
		Context:                reviewerCtx,
		RelevantFindings:       canonicalFindingFrontier(projection.CurrentBlockers()),
		GateEffect:             gateEffect,
	})
	if err != nil {
		return Result{}, err
	}
	if err := validateExplicitPreviousRunInput(root, logs, options.PreviousRunID, targetRecord, git); err != nil {
		return Result{}, err
	}
	reused, hasReusableVerdict := reusableVerdict(logs, targetRecord, git.BaseSHA, git.HeadSHA, reviewInputDigest)
	// An explicit predecessor denotes a fix-chain transition. Reviewers must run
	// so Reconcile can close (or preserve) findings from that predecessor; an
	// older matching PASS cannot establish that the newer blocker was fixed.
	if strings.TrimSpace(options.PreviousRunID) != "" {
		hasReusableVerdict = false
	}
	var rawFindings []reviewers.Finding
	if !hasReusableVerdict {
		rawFindings = runPRReadyReviewers(reviewerCtx)
	}
	run := findings.Run{ID: runID, Scope: "pr-ready", Target: targetRecord, RepoRoot: root, GitHead: git.HeadSHA}

	packDir := ""
	packRollback := func() error { return nil }
	if len(shardPlan.Shards) > 0 {
		packDir = shardpack.Dir(root, "pr-ready", shardTargetID(analysisGit), shardPlan.PlanHash)
		packRollback, err = packWriter.Write(root, shardPlan, shardpack.Header{
			Scope:    "pr-ready",
			TargetID: shardTargetID(analysisGit),
			Base:     analysisGit.BaseSHA,
			Head:     analysisGit.HeadSHA,
			Budget:   contextprofile.DefaultMaxBytesPerShard,
		}, analysisGit.BranchFiles)
		if err != nil {
			// Write can fail after the new pack set is already in place; the
			// rollback it returns restores the previous set, so run it rather
			// than leaving the failed run's packs behind.
			if packRollback != nil {
				_ = packRollback()
			}
			return Result{}, err
		}
	}

	result := Result{RunID: runID, ReviewRel: reviewRel, ContextRel: contextRel, Reused: hasReusableVerdict}
	if hasReusableVerdict {
		result.ReusedFromRunID = reused.RunID
	}
	err = func() error {
		if err := os.MkdirAll(filepath.Dir(contextPath), 0o755); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(reviewPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(contextPath, []byte(contextMarkdown(runID, analysisGit, profile, shardPlan, packDir, manifest, aggregate, knowledgeContext, reviewLogs, evidenceText, ghCtx, prEvidence, gateEffect, reviewInputDigest)), 0o644); err != nil {
			return err
		}
		if hasReusableVerdict {
			result.Verdict = reused.Verdict
			result.Blocking = false
			record := runRecord{
				SchemaVersion:        1,
				ID:                   runID,
				Scope:                "pr-ready",
				Target:               targetRecord,
				Status:               "passed",
				Verdict:              reused.Verdict,
				ExecutionMode:        "deterministic-local-reused",
				PreviousRunID:        options.PreviousRunID,
				AttemptNumber:        chain.AttemptNumber,
				MaxAttempts:          chain.MaxAttempts,
				BaseSHA:              git.BaseSHA,
				HeadSHA:              git.HeadSHA,
				ContextPath:          contextRel,
				ReviewPath:           reviewRel,
				Reviewers:            append([]string(nil), reviewerNames...),
				FindingIDs:           append([]string(nil), reused.FindingIDs...),
				SourceRefs:           []map[string]string{{"type": "branch", "id": targetRecord["id"]}},
				GateEffect:           gateEffect,
				BlockingFindingCount: reused.BlockingFindingCount,
				AdvisoryFindingCount: reused.AdvisoryFindingCount,
				FollowUpFindingCount: reused.FollowUpFindingCount,
				WarningFindingCount:  reused.WarningFindingCount,
				CreatedAt:            now.UTC().Format(time.RFC3339Nano),
				UpdatedAt:            now.UTC().Format(time.RFC3339Nano),
				RepoRoot:             root,
				GitHead:              git.HeadSHA,
				ReviewInputDigest:    reviewInputDigest,
				ReusedFromRunID:      reused.RunID,
			}
			if err := state.AppendJSONL(runsPath, record); err != nil {
				return err
			}
			meta := reviewMetadata{
				AttemptNumber:        chain.AttemptNumber,
				MaxAttempts:          chain.MaxAttempts,
				RunChain:             chain.Chain,
				BlockingFindingCount: reused.BlockingFindingCount,
				AdvisoryFindingCount: reused.AdvisoryFindingCount,
				FollowUpFindingCount: reused.FollowUpFindingCount,
				WarningFindingCount:  reused.WarningFindingCount,
				ReviewInputDigest:    reviewInputDigest,
				ReusedFromRunID:      reused.RunID,
				ReusedFromReviewPath: reused.Path,
				HistoricalBlockers:   projection.HistoricalBlockers(),
			}
			return os.WriteFile(reviewPath, []byte(reviewMarkdown(runID, contextRel, options.PreviousRunID, gateEffect, reused.Verdict, reviewGit.ChangedFiles, nil, prEvidence, reviewmanifest.ShardedReviewMarkdown(manifest, aggregate), meta)), 0o644)
		}
		reconciled, err := findings.Reconcile(root, run, rawFindings, findings.Options{
			PreviousRunID:  options.PreviousRunID,
			PreviousRunIDs: previousRunIDs,
			ResetRunIDs:    chain.ResetRunIDs,
		})
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
			ReviewInputDigest:    reviewInputDigest,
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
			ReviewInputDigest:    reviewInputDigest,
			HistoricalBlockers:   projection.HistoricalBlockers(),
		}
		return os.WriteFile(reviewPath, []byte(reviewMarkdown(runID, contextRel, options.PreviousRunID, gateEffect, verdict, reviewGit.ChangedFiles, reconciled.OpenFindings, prEvidence, reviewmanifest.ShardedReviewMarkdown(manifest, aggregate), meta)), 0o644)
	}()
	if err != nil {
		restoreSnapshots(snapshots)
		removeEmptyDirs(root)
		// The rollback error must not replace the error that caused the rollback.
		if rollbackErr := packRollback(); rollbackErr != nil {
			return Result{}, errors.Join(err, rollbackErr)
		}
		return Result{}, err
	}
	if len(shardPlan.Shards) > 0 {
		// The run is already recorded on disk. Pruning obsolete packs and collecting
		// superseded results are housekeeping: their failure must not discard the run
		// identifiers the caller needs.
		_ = packWriter.Prune(root, "pr-ready", shardTargetID(analysisGit), shardPlan.PlanHash)
		// Superseded result files are collected only once the gate has passed, so a
		// failing run never destroys the record it was judged on.
		if !result.Blocking {
			_ = packWriter.GC(root, "pr-ready", shardTargetID(analysisGit), shardPlan)
		}
	}
	return result, nil
}

// ingestShardResults discovers the result files for this target and aggregates
// them into the review manifest the gate and the context pack both read.
func ingestShardResults(root string, writer shardpack.Writer, plan contextprofile.ShardPlan, profile contextprofile.Profile, git gitcontext.Context, options Options) (reviewmanifest.Manifest, reviewmanifest.AggregateResult, error) {
	explicit := append(append([]string{}, options.ShardResultPaths...), options.CrossShardResultPaths...)
	found := shardpack.Found{}
	if len(plan.Shards) > 0 || len(explicit) > 0 {
		discovered, err := writer.Discover(root, "pr-ready", shardTargetID(git), plan, explicit)
		if err != nil {
			return reviewmanifest.Manifest{}, reviewmanifest.AggregateResult{}, err
		}
		found = discovered
	}
	manifest := reviewmanifest.Build(reviewmanifest.Input{
		Scope:             "pr-ready",
		Target:            map[string]string{"type": "branch", "id": firstNonEmpty(git.Branch, git.HeadSHA)},
		Profile:           profile,
		ShardPlan:         plan,
		PathDispositions:  reviewmanifest.GeneratedPathDispositions(profile.GeneratedExcludedFiles),
		ShardResults:      found.Shards,
		CrossShardResult:  found.CrossShard,
		IgnoredResults:    found.Ignored,
		UnreadableResults: found.Unreadable,
	})
	return manifest, reviewmanifest.Aggregate(manifest), nil
}

func manifestContext(manifest reviewmanifest.Manifest, aggregate reviewmanifest.AggregateResult) reviewers.ManifestContext {
	return reviewers.ManifestContext{
		Present:       len(manifest.ShardResults) > 0 || manifest.CrossShardResult != nil,
		Verdict:       aggregate.Verdict,
		Blockers:      aggregate.Blockers,
		ShardCount:    aggregate.ShardCount,
		ShardsCovered: aggregate.ShardsCovered,
		CrossShard:    aggregate.CrossShard,
		PlanHash:      aggregate.PlanHash,
	}
}

func resolveRunChain(root string, targetRecord map[string]string, options Options, logs []reviewlog.Summary, git gitcontext.Context) (runchain.Decision, []string, error) {
	chain, err := runchain.Resolve(root, runchain.Options{
		Scope:         "pr-ready",
		Target:        targetRecord,
		PreviousRunID: options.PreviousRunID,
		MaxAttempts:   options.MaxAttempts,
		HeadSHA:       git.HeadSHA,
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
		HeadSHA:     git.HeadSHA,
	})
	if fallbackErr != nil {
		return runchain.Decision{}, nil, fallbackErr
	}
	fallback.AttemptNumber = len(previousRunIDs) + 1
	if rootMaxAttempts := authenticatedLegacyRootMaxAttempts(root, logs, previousRunIDs, targetRecord, git); rootMaxAttempts > 0 {
		fallback.MaxAttempts = rootMaxAttempts
	}
	return fallback, previousRunIDs, nil
}

func authenticatedLegacyRootMaxAttempts(root string, logs []reviewlog.Summary, previousRunIDs []string, targetRecord map[string]string, git gitcontext.Context) int {
	if len(previousRunIDs) == 0 {
		return 0
	}
	for _, log := range logs {
		if log.RunID == previousRunIDs[0] && (log.RunRecordAuthenticated || committedPRReadyInputAuthenticated(root, log, targetRecord, git)) {
			return log.MaxAttempts
		}
	}
	return 0
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
	if identity.Branch != git.Branch {
		return false, true
	}
	if git.HeadSHA != "" && identity.Head != "" && identity.Head != git.HeadSHA {
		if !validGitObjectID(identity.Head) || !validGitObjectID(git.HeadSHA) || !gitCommitIsAncestor(root, identity.Head, git.HeadSHA) {
			return false, true
		}
	}
	return true, true
}

func historicalPRReadyRunIDsForCurrentTarget(root string, logs []reviewlog.Summary, targetRecord map[string]string, git gitcontext.Context) []string {
	var ids []string
	for _, log := range logs {
		if log.RunID == "" || log.Kind != "pr-ready" {
			continue
		}
		if log.RunRecordAuthenticated {
			if !sameTarget(log.TargetRecord, targetRecord) {
				ids = append(ids, log.RunID)
			}
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
		if legacyPRReadyTargetMatches(root, log, targetRecord, git) && legacyEscalationLocksCurrentHead(root, log, git) {
			return log.RunID, true
		}
	}
	return "", false
}

func legacyEscalationLocksCurrentHead(root string, log reviewlog.Summary, git gitcontext.Context) bool {
	currentHead := strings.TrimSpace(git.HeadSHA)
	if currentHead == "" {
		return true
	}
	if reviewedHead := strings.TrimSpace(log.HeadSHA); reviewedHead != "" {
		return reviewedHead == currentHead
	}
	identity, err := readLegacyPRReadyContextIdentity(root, log.ContextRel)
	if err != nil || strings.TrimSpace(identity.Head) == "" {
		// Without a reviewed head, there is no evidence that the escalation is
		// stale. Preserve the hard stop rather than guessing.
		return true
	}
	return identity.Head == currentHead
}

type legacyPRReadyContextIdentity struct {
	RunID             string
	Base              string
	Branch            string
	Head              string
	ReviewInputDigest string
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
	content, err := os.ReadFile(filepath.Join(root, clean))
	if err != nil {
		return legacyPRReadyContextIdentity{}, err
	}
	identity := parsePRReadyContextIdentity(string(content))
	if identity.Branch == "" && identity.Head == "" {
		return legacyPRReadyContextIdentity{}, fmt.Errorf("legacy context lacks branch and head identity")
	}
	return identity, nil
}

func parsePRReadyContextIdentity(text string) legacyPRReadyContextIdentity {
	var identity legacyPRReadyContextIdentity
	section := ""
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			section = strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
			continue
		}
		// An if/else-if chain rather than a tagless `switch {}`: Go's coverage tool emits no counter for a
		// tagless-switch case expression, so these guards read as permanently uncovered and mutation testing
		// can never exercise them. As `if` conditions they are covered and mutation-killable. The prefixes are
		// mutually exclusive per line, so the chain matches exactly what the switch did (first match wins).
		if section == "" && strings.HasPrefix(trimmed, "Run ID:") && identity.RunID == "" {
			identity.RunID = firstInlineCodeValue(trimmed)
		} else if section == "Git" && strings.HasPrefix(trimmed, "- Base:") && identity.Base == "" {
			identity.Base = firstInlineCodeValue(trimmed)
		} else if section == "Git" && strings.HasPrefix(trimmed, "- Head:") && identity.Head == "" {
			identity.Head = firstInlineCodeValue(trimmed)
		} else if section == "Git" && strings.HasPrefix(trimmed, "- Branch:") && identity.Branch == "" {
			identity.Branch = firstInlineCodeValue(trimmed)
		} else if section == "Git" && strings.HasPrefix(trimmed, "- "+reviewlog.ReviewerInputDigestLabel) && identity.ReviewInputDigest == "" {
			identity.ReviewInputDigest = firstInlineCodeValue(trimmed)
		}
	}
	return identity
}

func committedPRReadyInputAuthenticated(root string, log reviewlog.Summary, targetRecord map[string]string, git gitcontext.Context) bool {
	if log.Kind != "pr-ready" || strings.TrimSpace(log.Target) != "current branch" ||
		strings.TrimSpace(log.RunID) == "" || !validSHA256Digest(log.DeclaredInputDigest) ||
		git.Branch == "" || targetRecord["type"] != "branch" || targetRecord["id"] != git.Branch {
		return false
	}
	committedReview, ok := committedFile(root, log.Path)
	if !ok {
		return false
	}
	workingReview, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(log.Path)))
	if err != nil || !bytes.Equal(committedReview, workingReview) {
		return false
	}
	committedContext, ok := committedFile(root, log.ContextRel)
	if !ok {
		return false
	}
	identity := parsePRReadyContextIdentity(string(committedContext))
	if identity.RunID != log.RunID || identity.Base == "" || identity.Base != git.BaseSHA ||
		identity.Branch != git.Branch || identity.Head == "" || !validGitObjectID(identity.Head) ||
		identity.ReviewInputDigest != log.DeclaredInputDigest {
		return false
	}
	return identity.Head == git.HeadSHA || (validGitObjectID(git.HeadSHA) && gitCommitIsAncestor(root, identity.Head, git.HeadSHA))
}

func committedFile(root, rel string) ([]byte, bool) {
	clean := filepath.Clean(strings.TrimSpace(rel))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return nil, false
	}
	cmd := exec.Command("git", "show", "HEAD:"+filepath.ToSlash(clean))
	cmd.Dir = root
	content, err := cmd.Output()
	return content, err == nil
}

func gitCommitIsAncestor(root, ancestor, descendant string) bool {
	cmd := exec.Command("git", "merge-base", "--is-ancestor", ancestor, descendant)
	cmd.Dir = root
	return cmd.Run() == nil
}

func validSHA256Digest(value string) bool {
	const prefix = "sha256:"
	return strings.HasPrefix(value, prefix) && len(value) == len(prefix)+64 && validHex(value[len(prefix):])
}

func validGitObjectID(value string) bool {
	return (len(value) == 40 || len(value) == 64) && validHex(value)
}

func validHex(value string) bool {
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return value != ""
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

func reviewerContext(git gitcontext.Context, profile contextprofile.Profile, knowledgeContext knowledge.Context, logs []reviewlog.Summary, evidenceText, prEvidence string, ghCtx githubcontext.Context, includeWorkingTree bool, dirtyFiles []string, manifest reviewers.ManifestContext) reviewers.PRReadyContext {
	return reviewers.PRReadyContext{
		Git: reviewers.GitContext{
			ChangedFiles:             git.ChangedFiles,
			StagedFiles:              git.StagedFiles,
			UnstagedFiles:            git.UnstagedFiles,
			WorkingTreeFiles:         git.WorkingTreeFiles,
			UntrackedFiles:           git.UntrackedFiles,
			Diff:                     git.Diff,
			BranchDiffFull:           git.BranchDiffFull,
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
		Manifest:              manifest,
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

// gateReviewLogs excludes review logs whose own blocker-class findings have
// already been resolved. The report still receives the unfiltered logs and
// renders those historical records with their resolver, but a path overlap
// cannot make a cleared finding block a new PR-ready target.
func gateReviewLogs(logs []reviewlog.Summary, ledger []findings.Record) []reviewlog.Summary {
	byID := indexFindings(ledger)
	result := make([]reviewlog.Summary, 0, len(logs))
	for _, log := range logs {
		if reconcileReview(FromReviewLog(log), byID).Resolved {
			continue
		}
		result = append(result, log)
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
	return reviewers.PRGitHubContext{
		Available:         context.Available,
		UnavailableReason: context.UnavailableReason,
		ReviewDecision:    context.ReviewDecision,
		Entries:           entries,
	}
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
	// `latest` is keyed by Target, so every entry in result has a distinct, non-empty Target — sorting by
	// Target alone is already a total order. (A logSortKey tie-break would be dead code: no two entries can
	// share a Target, so it could never run, and mutation testing could never exercise it.)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Target < result[j].Target
	})
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
	git.UntrackedTruncatedCount = 0
	// Prefer the measured branch bytes; never recompute from the truncated text.
	if git.BranchFilteredDiffBytes > 0 {
		git.RawDiffBytes = git.BranchRawDiffBytes
		git.FilteredDiffBytes = git.BranchFilteredDiffBytes
	} else {
		git.RawDiffBytes = len(git.Diff)
		git.FilteredDiffBytes = len(git.Diff)
	}
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
		// Cut back to a rune boundary so truncation never splits a multi-byte rune into the pack.
		cut := 12000
		for cut > 0 && !utf8.RuneStart(text[cut]) {
			cut--
		}
		return text[:cut], nil
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

func contextMarkdown(runID string, git gitcontext.Context, profile contextprofile.Profile, plan contextprofile.ShardPlan, packDir string, manifest reviewmanifest.Manifest, aggregate reviewmanifest.AggregateResult, knowledgeContext knowledge.Context, logs []reviewlog.Summary, evidenceText string, ghCtx githubcontext.Context, prEvidence, gateEffect, reviewInputDigest string) string {
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
		"- " + reviewlog.ReviewerInputDigestLabel + " " + markdown.InlineCode(reviewInputDigest) + "\n" +
		"- Gate effect: " + markdown.InlineCode(gateEffect) + "\n\n" +
		contextprofile.Markdown(profile) + "\n\n" +
		contextprofile.ShardPlanMarkdown(plan, shardpack.Rel(packDir)) + "\n\n" +
		reviewmanifest.Markdown(manifest, aggregate) + "\n\n" +
		"## Changed Files\n\n" + markdownList(changed, "No changed files.") + "\n\n" +
		"## Diff\n\n" + markdown.FencedCodeBlock("diff", strings.Join([]string{git.Diff, git.StagedDiff, git.WorkingTreeDiff, git.UntrackedExcerpts}, "\n")) + "\n\n" +
		"## Review Logs\n\n" + reviewLogsMarkdown(logs) + "\n\n" +
		"## Knowledge And Registries\n\n" + knowledgeMarkdown(knowledgeContext) + "\n\n" +
		"## Validation Evidence\n\n" + firstNonEmpty(evidenceText, "No external validation evidence supplied.") + "\n\n" +
		"## GitHub Context\n\n" + githubcontext.RenderMarkdown(ghCtx) + "\n\n" +
		"## Suggested PR Evidence\n\n" + prEvidence + "\n"
}

type reviewMetadata struct {
	AttemptNumber        int
	MaxAttempts          int
	RunChain             []runchain.Record
	BlockingFindingCount int
	AdvisoryFindingCount int
	FollowUpFindingCount int
	WarningFindingCount  int
	ReviewInputDigest    string
	ReusedFromRunID      string
	ReusedFromReviewPath string
	HistoricalBlockers   []findings.Record
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

func reviewMarkdown(runID, contextRel, previousRun, gateEffect, verdict string, coveredPaths []string, records []findings.Record, prEvidence, shardedReview string, meta reviewMetadata) string {
	// The sharded section sits after the verdict value line, so reviewlog still
	// reads the verdict token as the first non-empty line after the heading.
	if shardedReview != "" {
		shardedReview += "\n\n"
	}
	executionMode := "deterministic-local"
	reuseHeader := ""
	reviewerResults := "## Reviewer Results\n\n| Reviewer | Verdict | Blocking | Notes |\n| --- | --- | ---: | --- |\n" +
		reviewerTable(records) + "\n\n" + findingsMarkdown(records) + "\n"
	if meta.ReusedFromRunID != "" {
		executionMode = "deterministic-local-reused"
		reuseHeader = "Reused verdict from: " + markdown.InlineCode(meta.ReusedFromRunID) + "\n\n"
		reviewerResults = "## Reviewer Results\n\nReviewer execution was skipped because the authenticated reviewer-input digest is unchanged. Prior review: " +
			markdown.InlineCode(meta.ReusedFromReviewPath) + ".\n\n" +
			"## Findings\n\nFinding counts and verdict were reused from " + markdown.InlineCode(meta.ReusedFromRunID) + ".\n"
	}
	digestHeader := ""
	if meta.ReviewInputDigest != "" {
		digestHeader = reviewlog.ReviewerInputDigestLabel + " " + markdown.InlineCode(meta.ReviewInputDigest) + "\n\n"
	}
	// Covered paths: the exclude-filtered source files this review examined, so `status` can credit a
	// clean review for the files it read (see reviewlog.DecodeCoveredPaths / status coverage accounting).
	return "# metareview: pr-ready review\n\n" +
		"Run ID: " + markdown.InlineCode(runID) + "\n\n" +
		"Target: `current branch`\n\n" +
		"Context pack: " + markdown.InlineCode(contextRel) + "\n\n" +
		"Execution mode: " + markdown.InlineCode(executionMode) + "\n\n" +
		"Gate effect: " + markdown.InlineCode(gateEffect) + "\n\n" +
		"Previous run: " + markdown.InlineCode(firstNonEmpty(previousRun, "none")) + "\n\n" +
		reviewlog.AttemptLabel + " " + markdown.InlineCode(fmt.Sprintf("%d/%d", meta.AttemptNumber, meta.MaxAttempts)) + "\n\n" +
		reuseHeader + digestHeader +
		reviewlog.CoveredPathsLabel + " " + markdown.InlineCode(reviewlog.EncodeCoveredPaths(coveredPaths)) + "\n\n" +
		"## Verdict\n\n" + verdict + "\n\n" + shardedReview +
		reviewerResults +
		"\n## Suggested PR Evidence\n\n" + prEvidence + "\n" +
		repositoryHealthMarkdown(meta.HistoricalBlockers) +
		runChainMarkdown(runID, verdict, meta)
}

func repositoryHealthMarkdown(records []findings.Record) string {
	if len(records) == 0 {
		return ""
	}
	lines := make([]string, 0, len(records))
	for _, record := range records {
		title := strings.TrimSpace(strings.NewReplacer("\n", " ", "\r", " ").Replace(record.Title))
		if title == "" {
			title = "Unresolved historical finding"
		}
		target := findingTargetID(record.Target)
		if target != "" {
			title += " (" + target + ")"
		}
		lines = append(lines, "- "+title+": remains visible in `docs/metareview/FINDINGS.md` but does not block this target.")
	}
	sort.Strings(lines)
	return "\n## Repository Health Advisory\n\n" + strings.Join(lines, "\n") + "\n"
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
	// An if-chain rather than a tagless `switch {}`: Go's coverage tool emits no counter for a
	// tagless-switch case expression, so its guards read as permanently uncovered and mutation testing
	// can never exercise them. As plain `if` conditions they are both covered and mutation-killable.
	if counts.Blocking > 0 {
		return "blocking"
	}
	if counts.Advisory > 0 {
		return "advisory"
	}
	if counts.FollowUp > 0 {
		return "follow-up"
	}
	return "warning"
}

func runChainMarkdown(runID, verdict string, meta reviewMetadata) string {
	if verdict != "ESCALATED" {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("\n## Run Chain\n\n")
	for _, link := range meta.RunChain {
		fmt.Fprintf(&builder, "- %s: %s attempt %d/%d\n", link.ID, link.Verdict, link.AttemptNumber, link.MaxAttempts)
	}
	fmt.Fprintf(&builder, "- %s: %s attempt %d/%d\n", runID, verdict, meta.AttemptNumber, meta.MaxAttempts)
	builder.WriteString("\n## Unresolved Blocker Summary\n\n")
	fmt.Fprintf(&builder, "- Blocking: %d\n- Advisory: %d\n- Follow-up: %d\n- Warnings: %d\n", meta.BlockingFindingCount, meta.AdvisoryFindingCount, meta.FollowUpFindingCount, meta.WarningFindingCount)
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

func shardTargetID(git gitcontext.Context) string {
	return firstNonEmpty(git.Branch, git.HeadSHA)
}

// mutationContextFor loads the declared mutation reports. An unreadable or unrecognised report is
// an error that stops the review, never a skipped file: a mutation gate that quietly drops a
// report is a gate that passes because it looked at less.
func mutationContextFor(paths []string) (reviewers.MutationContext, error) {
	if len(paths) == 0 {
		return reviewers.MutationContext{}, nil
	}
	reports, err := mutation.LoadAll(paths)
	if err != nil {
		return reviewers.MutationContext{}, err
	}
	return reviewers.MutationContext{Reports: reports}, nil
}
