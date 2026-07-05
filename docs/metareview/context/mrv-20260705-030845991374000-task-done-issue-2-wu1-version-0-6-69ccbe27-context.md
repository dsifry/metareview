# metareview task-done context

Run ID: `mrv-20260705-030845991374000-task-done-issue-2-wu1-version-0-6-69ccbe27`

## Task

Advisory task target: issue-2-wu1-version-0.6.0

## Git

- Base: `afdda501dd2196197d5417dc0418aee55acdef3b`
- Head: `765043d0d24adfc26ce6696c92bc227cee84c5fe`
- Branch: `codex/issue-2-wu1`
- Gate effect: `gate`

## Changed Files

- internal/prready/review.go
- internal/reviewlog/reviewlog.go
- internal/reviewlog/reviewlog_test.go
- internal/reviewstate/projector.go
- internal/reviewstate/projector_test.go
- tests/go/test-pr-ready-review.sh
- .claude-plugin/marketplace.json
- .claude-plugin/plugin.json
- .codex-plugin/plugin.json
- internal/version/version.go
- package.json

## Diff

```diff
diff --git a/internal/prready/review.go b/internal/prready/review.go
index b14b098..b91417a 100644
--- a/internal/prready/review.go
+++ b/internal/prready/review.go
@@ -15,6 +15,7 @@ import (
     "github.com/dsifry/metareview/internal/repo"
     "github.com/dsifry/metareview/internal/reviewers"
     "github.com/dsifry/metareview/internal/reviewlog"
+    "github.com/dsifry/metareview/internal/reviewstate"
     "github.com/dsifry/metareview/internal/runchain"
     "github.com/dsifry/metareview/internal/state"
 )
@@ -88,10 +89,15 @@ func Create(root string, options Options) (Result, error) {
     if err != nil {
         return Result{}, err
     }
+    targetRecord := map[string]string{"type": "branch", "id": firstNonEmpty(git.Branch, git.HeadSHA)}
     logs, err := reviewlog.Discover(root)
     if err != nil {
         return Result{}, err
     }
+    chain, previousRunIDs, err := resolveRunChain(root, targetRecord, options, logs, git)
+    if err != nil {
+        return Result{}, err
+    }
     blockers, err := findings.UnresolvedBlocking(root)
     if err != nil {
         return Result{}, err
@@ -104,13 +110,21 @@ func Create(root string, options Options) (Result, error) {
     if err != nil {
         return Result{}, err
     }
-    reviewLogs := append(latestLogsByTarget(logs), blockerLogs(blockers)...)
+    projection := reviewstate.ProjectRecords(logs, blockers, reviewstate.Options{
+        Scope:            "pr-ready",
+        Target:           targetRecord,
+        PreviousRunIDs:   previousRunIDs,
+        HistoricalRunIDs: historicalPRReadyRunIDsForCurrentTarget(root, logs, targetRecord, git),
+        ChangedPaths:     reviewedPaths(reviewGit),
+        CurrentTarget:    targetRecord,
+    })
+    reviewLogs := append(latestLogsByTarget(projection.CurrentReviewLogs()), blockerLogs(projection.CurrentBlockers())...)
     prEvidence := RenderEvidence(EvidenceInput{
         Summary:     branchSummary(reviewGit),
         Validation:  validationLines(evidenceText),
         TaskReviews: taskReviewEvidence(reviewLogs),
         EpicReviews: epicReviewEvidence(reviewLogs),
-        Blockers:    blockerEvidence(blockers),
+        Blockers:    blockerEvidence(projection.CurrentBlockers()),
         GitHub:      ghCtx,
     })

@@ -133,7 +147,6 @@ func Create(root string, options Options) (Result, error) {
         gateEffect = "gate"
     }
     rawFindings := reviewers.RunPRReady(reviewerContext(reviewGit, knowledgeContext, reviewLogs, evidenceText, prEvidence, ghCtx))
-    targetRecord := map[string]string{"type": "branch", "id": firstNonEmpty(git.Branch, git.HeadSHA)}
     run := findings.Run{ID: runID, Scope: "pr-ready", Target: targetRecord, RepoRoot: root, GitHead: git.HeadSHA}

     result := Result{RunID: runID, ReviewRel: reviewRel, ContextRel: contextRel}
@@ -147,19 +160,6 @@ func Create(root string, options Options) (Result, error) {
         if err := os.WriteFile(contextPath, []byte(contextMarkdown(runID, reviewGit, knowledgeContext, reviewLogs, evidenceText, ghCtx, prEvidence, gateEffect)), 0o644); err != nil {
             return err
         }
-        chain, err := runchain.Resolve(root, runchain.Options{
-            Scope:         "pr-ready",
-            Target:        targetRecord,
-            PreviousRunID: options.PreviousRunID,
-            MaxAttempts:   options.MaxAttempts,
-        })
-        if err != nil {
-            return err
-        }
-        previousRunIDs := make([]string, 0, len(chain.Chain))
-        for _, link := range chain.Chain {
-            previousRunIDs = append(previousRunIDs, link.ID)
-        }
         reconciled, err := findings.Reconcile(root, run, rawFindings, findings.Options{PreviousRunID: options.PreviousRunID, PreviousRunIDs: previousRunIDs})
         if err != nil {
             return err
@@ -219,6 +219,175 @@ func Create(root string, options Options) (Result, error) {
     return result, nil
 }

+func resolveRunChain(root string, targetRecord map[string]string, options Options, logs []reviewlog.Summary, git gitcontext.Context) (runchain.Decision, []string, error) {
+    chain, err := runchain.Resolve(root, runchain.Options{
+        Scope:         "pr-ready",
+        Target:        targetRecord,
+        PreviousRunID: options.PreviousRunID,
+        MaxAttempts:   options.MaxAttempts,
+    })
+    if err == nil {
+        if options.PreviousRunID == "" && len(chain.Chain) == 0 {
+            if escalated, ok := legacyEscalatedPRReadyForTarget(root, logs, targetRecord, git); ok {
+                return runchain.Decision{}, nil, fmt.Errorf("same target already escalated in run %s", escalated)
+            }
+        }
+        return chain, runIDsFromChain(chain.Chain), nil
+    }
+    if options.PreviousRunID == "" || !legacyRecoverableRunchainError(err) {
+        return runchain.Decision{}, nil, err
+    }
+    previousRunIDs, legacyErr := legacyPreviousRunIDsForPRReady(root, logs, options.PreviousRunID, targetRecord, git)
+    if legacyErr != nil {
+        return runchain.Decision{}, nil, legacyErr
+    }
+    if len(previousRunIDs) == 0 {
+        return runchain.Decision{}, nil, err
+    }
+    fallback, fallbackErr := runchain.Resolve(root, runchain.Options{
+        Scope:       "pr-ready",
+        Target:      targetRecord,
+        MaxAttempts: options.MaxAttempts,
+    })
+    if fallbackErr != nil {
+        return runchain.Decision{}, nil, fallbackErr
+    }
+    return fallback, previousRunIDs, nil
+}
+
+func legacyPreviousRunIDsForPRReady(root string, logs []reviewlog.Summary, previousRunID string, targetRecord map[string]string, git gitcontext.Context) ([]string, error) {
+    ids := reviewstate.LegacyPreviousRunIDs(logs, previousRunID)
+    if len(ids) == 0 {
+        return nil, nil
+    }
+    byID := map[string]reviewlog.Summary{}
+    for _, log := range logs {
+        if log.RunID != "" {
+            byID[log.RunID] = log
+        }
+    }
+    for _, id := range ids {
+        log, ok := byID[id]
+        if !ok || log.Kind != "pr-ready" || !legacyPRReadyTargetMatches(root, log, targetRecord, git) {
+            return nil, nil
+        }
+        if strings.EqualFold(log.Verdict, "ESCALATED") {
+            return nil, fmt.Errorf("previous run %s already escalated", id)
+        }
+    }
+    if escalated, ok := legacyEscalatedPRReadyForTarget(root, logs, targetRecord, git); ok {
+        return nil, fmt.Errorf("same target already escalated in run %s", escalated)
+    }
+    return ids, nil
+}
+
+func legacyPRReadyTargetMatches(root string, log reviewlog.Summary, targetRecord map[string]string, git gitcontext.Context) bool {
+    matches, known := legacyPRReadyTargetMatch(root, log, targetRecord, git)
+    return known && matches
+}
+
+func legacyPRReadyTargetMatch(root string, log reviewlog.Summary, targetRecord map[string]string, git gitcontext.Context) (bool, bool) {
+    target := strings.TrimSpace(log.Target)
+    if target != "current branch" && target != targetRecord["id"] {
+        return false, true
+    }
+    if target != "current branch" {
+        return true, true
+    }
+    identity, err := readLegacyPRReadyContextIdentity(root, log.ContextRel)
+    if err != nil {
+        return false, false
+    }
+    if git.Branch == "" || identity.Branch == "" {
+        return false, false
+    }
+    return identity.Branch == git.Branch, true
+}
+
+func historicalPRReadyRunIDsForCurrentTarget(root string, logs []reviewlog.Summary, targetRecord map[string]string, git gitcontext.Context) []string {
+    var ids []string
+    for _, log := range logs {
+        if log.RunID == "" || log.Kind != "pr-ready" {
+            continue
+        }
+        matches, known := legacyPRReadyTargetMatch(root, log, targetRecord, git)
+        if known && !matches {
+            ids = append(ids, log.RunID)
+        }
+    }
+    return ids
+}
+
+func legacyEscalatedPRReadyForTarget(root string, logs []reviewlog.Summary, targetRecord map[string]string, git gitcontext.Context) (string, bool) {
+    for _, log := range logs {
+        if log.RunID == "" || log.Kind != "pr-ready" || !strings.EqualFold(log.Verdict, "ESCALATED") {
+            continue
+        }
+        if legacyPRReadyTargetMatches(root, log, targetRecord, git) {
+            return log.RunID, true
+        }
+    }
+    return "", false
+}
+
+type legacyPRReadyContextIdentity struct {
+    Branch string
+    Head   string
+}
+
+func readLegacyPRReadyContextIdentity(root, rel string) (legacyPRReadyContextIdentity, error) {
+    rel = strings.TrimSpace(rel)
+    if rel == "" {
+        return legacyPRReadyContextIdentity{}, fmt.Errorf("legacy context path is required")
+    }
+    clean := filepath.Clean(rel)
+    if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
+        return legacyPRReadyContextIdentity{}, fmt.Errorf("legacy context path escapes repository: %s", rel)
+    }
+    bytes, err := os.ReadFile(filepath.Join(root, clean))
+    if err != nil {
+        return legacyPRReadyContextIdentity{}, err
+    }
+    var identity legacyPRReadyContextIdentity
+    for _, line := range strings.Split(string(bytes), "\n") {
+        switch {
+        case strings.HasPrefix(line, "- Branch:"):
+            identity.Branch = firstInlineCodeValue(line)
+        case strings.HasPrefix(line, "- Head:"):
+            identity.Head = firstInlineCodeValue(line)
+        }
+    }
+    if identity.Branch == "" && identity.Head == "" {
+        return legacyPRReadyContextIdentity{}, fmt.Errorf("legacy context lacks branch and head identity")
+    }
+    return identity, nil
+}
+
+func firstInlineCodeValue(line string) string {
+    start := strings.Index(line, "`")
+    if start < 0 {
+        return ""
+    }
+    end := strings.Index(line[start+1:], "`")
+    if end < 0 {
+        return ""
+    }
+    return line[start+1 : start+1+end]
+}
+
+func runIDsFromChain(chain []runchain.Record) []string {
+    ids := make([]string, 0, len(chain))
+    for _, link := range chain {
+        ids = append(ids, link.ID)
+    }
+    return ids
+}
+
+func legacyRecoverableRunchainError(err error) bool {
+    message := err.Error()
+    return strings.Contains(message, "previous run ") && (strings.Contains(message, " not found") || strings.Contains(message, " chain missing "))
+}
+
 func reviewerContext(git gitcontext.Context, knowledgeContext knowledge.Context, logs []reviewlog.Summary, evidenceText, prEvidence string, ghCtx githubcontext.Context) reviewers.PRReadyContext {
     return reviewers.PRReadyContext{
         Git: reviewers.GitContext{
@@ -356,6 +525,30 @@ func branchSummary(git gitcontext.Context) string {
     return branch + " changes " + strings.Join(git.ChangedFiles, ", ")
 }

+func reviewedPaths(git gitcontext.Context) []string {
+    var paths []string
+    paths = append(paths, git.ChangedFiles...)
+    paths = append(paths, git.StagedFiles...)
+    paths = append(paths, git.UnstagedFiles...)
+    paths = append(paths, git.WorkingTreeFiles...)
+    paths = append(paths, git.UntrackedFiles...)
+    return uniqueStrings(paths)
+}
+
+func uniqueStrings(values []string) []string {
+    seen := map[string]bool{}
+    result := make([]string, 0, len(values))
+    for _, value := range values {
+        value = strings.TrimSpace(value)
+        if value == "" || seen[value] {
+            continue
+        }
+        seen[value] = true
+        result = append(result, value)
+    }
+    return result
+}
+
 func validationLines(text string) []string {
     var lines []string
     for _, line := range strings.Split(text, "\n") {
diff --git a/internal/reviewlog/reviewlog.go b/internal/reviewlog/reviewlog.go
index 2ae44b8..2e98b87 100644
--- a/internal/reviewlog/reviewlog.go
+++ b/internal/reviewlog/reviewlog.go
@@ -19,6 +19,8 @@ type Summary struct {
     Target                string            `json:"target"`
     Verdict               string            `json:"verdict"`
     Kind                  string            `json:"kind"`
+    PreviousRunID         string            `json:"previousRunId,omitempty"`
+    ContextRel            string            `json:"contextRel,omitempty"`
     FindingIDs            []string          `json:"findingIds"`
     HasUnresolvedBlockers bool              `json:"hasUnresolvedBlockers"`
     AttemptNumber         int               `json:"attemptNumber,omitempty"`
@@ -104,6 +106,10 @@ func parseMarkdown(rel, text string) Summary {
             summary.RunID = firstInlineCode(line)
         case strings.HasPrefix(line, "Target:"):
             summary.Target = firstInlineCode(line)
+        case strings.HasPrefix(line, "Previous run:"):
+            summary.PreviousRunID = previousRunID(firstInlineCode(line))
+        case strings.HasPrefix(line, "Context pack:"):
+            summary.ContextRel = firstInlineCode(line)
         case strings.TrimSpace(line) == "## Verdict":
             summary.Verdict = nextNonEmpty(lines, i+1)
         }
@@ -111,7 +117,7 @@ func parseMarkdown(rel, text string) Summary {
             summary.FindingIDs = appendUnique(summary.FindingIDs, id)
         }
     }
-    if verdictIsUnresolved(summary.Verdict) || strings.Contains(text, "NEEDS_REVISION") {
+    if verdictIsUnresolved(summary.Verdict) {
         summary.HasUnresolvedBlockers = true
     }
     if summary.Kind == "artifact" && !artifactReviewComplete(lines) {
@@ -120,6 +126,14 @@ func parseMarkdown(rel, text string) Summary {
     return summary
 }

+func previousRunID(value string) string {
+    value = strings.TrimSpace(value)
+    if strings.EqualFold(value, "none") {
+        return ""
+    }
+    return value
+}
+
 func reviewKind(line string) string {
     lower := strings.ToLower(line)
     switch {
diff --git a/internal/reviewlog/reviewlog_test.go b/internal/reviewlog/reviewlog_test.go
index 7c39617..3e42006 100644
--- a/internal/reviewlog/reviewlog_test.go
+++ b/internal/reviewlog/reviewlog_test.go
@@ -95,6 +95,53 @@ func TestCompletedArtifactReviewIsNotUnresolved(t *testing.T) {
     }
 }

+func TestPassReviewMentioningHistoricalNeedsRevisionIsNotUnresolved(t *testing.T) {
+    root := t.TempDir()
+    mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "task.md"),
+        "# metareview: task-done review\n\n"+
+            "Run ID: `mrv-task`\n\n"+
+            "Target: `task-1`\n\n"+
+            "## Verdict\n\nPASS\n\n"+
+            "## Notes\n\nPrevious run mrv-old was NEEDS_REVISION; this run fixed it.\n\n"+
+            "## Findings\n\nNo blocking findings.\n")
+
+    logs, err := ForTarget(root, "task-1")
+    if err != nil {
+        t.Fatalf("target logs: %v", err)
+    }
+    if len(logs) != 1 {
+        t.Fatalf("expected one log, got %+v", logs)
+    }
+    if logs[0].HasUnresolvedBlockers {
+        t.Fatalf("historical NEEDS_REVISION prose must not poison PASS: %+v", logs[0])
+    }
+}
+
+func TestDiscoverParsesLegacyPreviousRunFromMarkdown(t *testing.T) {
+    root := t.TempDir()
+    mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "task.md"),
+        "# metareview: pr-ready review\n\n"+
+            "Run ID: `mrv-task`\n\n"+
+            "Target: `current branch`\n\n"+
+            "Context pack: `docs/metareview/context/mrv-task-context.md`\n\n"+
+            "Previous run: `mrv-root`\n\n"+
+            "## Verdict\n\nNEEDS_REVISION\n")
+
+    logs, err := Discover(root)
+    if err != nil {
+        t.Fatalf("discover logs: %v", err)
+    }
+    if len(logs) != 1 {
+        t.Fatalf("expected one log, got %+v", logs)
+    }
+    if logs[0].PreviousRunID != "mrv-root" {
+        t.Fatalf("expected previous run from Markdown, got %+v", logs[0])
+    }
+    if logs[0].ContextRel != "docs/metareview/context/mrv-task-context.md" {
+        t.Fatalf("expected context pack from Markdown, got %+v", logs[0])
+    }
+}
+
 func TestEscalatedVerdictIsUnresolved(t *testing.T) {
     root := t.TempDir()
     mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "task.md"), reviewMarkdown("mrv-task", "task-1", "ESCALATED", ""))
diff --git a/internal/reviewstate/projector.go b/internal/reviewstate/projector.go
new file mode 100644
index 0000000..1f6bd8f
--- /dev/null
+++ b/internal/reviewstate/projector.go
@@ -0,0 +1,292 @@
+package reviewstate
+
+import (
+    "path/filepath"
+    "strings"
+
+    "github.com/dsifry/metareview/internal/findings"
+    "github.com/dsifry/metareview/internal/reviewlog"
+    "github.com/dsifry/metareview/internal/runchain"
+)
+
+type Options struct {
+    Scope            string
+    Target           map[string]string
+    PreviousRunID    string
+    PreviousRunIDs   []string
+    HistoricalRunIDs []string
+    ChangedPaths     []string
+    CurrentTarget    map[string]string
+    CurrentRunID     string
+}
+
+type Projection struct {
+    targetKeyValue       string
+    currentReviewLogs    []reviewlog.Summary
+    currentBlockers      []findings.Record
+    historicalLogs       []reviewlog.Summary
+    historicalBlockers   []findings.Record
+    supersededRunIDs     map[string]bool
+    supersededFindingIDs map[string]bool
+}
+
+func Project(root string, options Options) (Projection, error) {
+    logs, err := reviewlog.Discover(root)
+    if err != nil {
+        return Projection{}, err
+    }
+    blockers, err := findings.UnresolvedBlocking(root)
+    if err != nil {
+        return Projection{}, err
+    }
+    if options.PreviousRunID != "" {
+        chain, err := runchain.Resolve(root, runchain.Options{
+            Scope:         options.Scope,
+            Target:        targetForRunchain(options),
+            PreviousRunID: options.PreviousRunID,
+        })
+        if err != nil {
+            return Projection{}, err
+        }
+        options.PreviousRunIDs = append(options.PreviousRunIDs, runIDsFromChain(chain.Chain)...)
+    }
+    return ProjectRecords(logs, blockers, options), nil
+}
+
+func targetForRunchain(options Options) map[string]string {
+    if len(options.CurrentTarget) > 0 {
+        return options.CurrentTarget
+    }
+    return options.Target
+}
+
+func runIDsFromChain(chain []runchain.Record) []string {
+    ids := make([]string, 0, len(chain))
+    for _, link := range chain {
+        ids = append(ids, link.ID)
+    }
+    return ids
+}
+
+func ProjectRecords(logs []reviewlog.Summary, blockers []findings.Record, options Options) Projection {
+    previous := stringSet(options.PreviousRunIDs)
+    historical := stringSet(options.HistoricalRunIDs)
+    changed := normalizedPathSet(options.ChangedPaths)
+    historicalRunIDs := map[string]bool{}
+    currentTarget := options.CurrentTarget
+    if len(currentTarget) == 0 {
+        currentTarget = options.Target
+    }
+    projection := Projection{
+        targetKeyValue:       TargetKey(options.Scope, currentTarget),
+        currentReviewLogs:    make([]reviewlog.Summary, 0, len(logs)),
+        currentBlockers:      make([]findings.Record, 0, len(blockers)),
+        historicalLogs:       []reviewlog.Summary{},
+        historicalBlockers:   []findings.Record{},
+        supersededRunIDs:     map[string]bool{},
+        supersededFindingIDs: map[string]bool{},
+    }
+    for _, log := range logs {
+        if previous[log.RunID] {
+            projection.supersededRunIDs[log.RunID] = true
+            projection.historicalLogs = append(projection.historicalLogs, log)
+            continue
+        }
+        if historical[log.RunID] {
+            projection.historicalLogs = append(projection.historicalLogs, log)
+            continue
+        }
+        if unrelatedArtifact(log, changed) {
+            if log.RunID != "" {
+                historicalRunIDs[log.RunID] = true
+            }
+            projection.historicalLogs = append(projection.historicalLogs, log)
+            continue
+        }
+        projection.currentReviewLogs = append(projection.currentReviewLogs, log)
+    }
+    for _, blocker := range blockers {
+        if previous[blocker.RunID] {
+            projection.supersededFindingIDs[blocker.ID] = true
+            continue
+        }
+        if historicalRunIDs[blocker.RunID] {
+            projection.historicalBlockers = append(projection.historicalBlockers, blocker)
+            continue
+        }
+        if historical[blocker.RunID] || unrelatedBranchBlocker(blocker, currentTarget) || unrelatedPathBlocker(blocker, changed) {
+            projection.historicalBlockers = append(projection.historicalBlockers, blocker)
+            continue
+        }
+        projection.currentBlockers = append(projection.currentBlockers, blocker)
+    }
+    return projection
+}
+
+func TargetKey(scope string, target map[string]string) string {
+    scope = strings.TrimSpace(scope)
+    if len(target) == 0 {
+        return scope
+    }
+    targetType := strings.TrimSpace(target["type"])
+    targetID := strings.TrimSpace(firstNonEmpty(target["id"], target["path"]))
+    if scope == "" {
+        return targetType + ":" + targetID
+    }
+    return scope + ":" + targetType + ":" + targetID
+}
+
+func (projection Projection) TargetKey() string {
+    return projection.targetKeyValue
+}
+
+func (projection Projection) CurrentReviewLogs() []reviewlog.Summary {
+    return projection.currentReviewLogs
+}
+
+func (projection Projection) CurrentBlockers() []findings.Record {
+    return projection.currentBlockers
+}
+
+func (projection Projection) HistoricalUnrelated() []reviewlog.Summary {
+    return projection.historicalLogs
+}
+
+func (projection Projection) HistoricalBlockers() []findings.Record {
+    return projection.historicalBlockers
+}
+
+func (projection Projection) SupersededRunIDs() map[string]bool {
+    return projection.supersededRunIDs
+}
+
+func (projection Projection) SupersededFindingIDs() map[string]bool {
+    return projection.supersededFindingIDs
+}
+
+func LegacyPreviousRunIDs(logs []reviewlog.Summary, previousRunID string) []string {
+    previousRunID = strings.TrimSpace(previousRunID)
+    if previousRunID == "" {
+        return nil
+    }
+    byID := map[string]reviewlog.Summary{}
+    for _, log := range logs {
+        if log.RunID != "" {
+            byID[log.RunID] = log
+        }
+    }
+    var reversed []string
+    seen := map[string]bool{}
+    for id := previousRunID; id != ""; {
+        if seen[id] {
+            return nil
+        }
+        seen[id] = true
+        log, ok := byID[id]
+        if !ok {
+            return nil
+        }
+        reversed = append(reversed, id)
+        id = strings.TrimSpace(log.PreviousRunID)
+    }
+    ids := make([]string, 0, len(reversed))
+    for i := len(reversed) - 1; i >= 0; i-- {
+        ids = append(ids, reversed[i])
+    }
+    return ids
+}
+
+func stringSet(values []string) map[string]bool {
+    result := map[string]bool{}
+    for _, value := range values {
+        value = strings.TrimSpace(value)
+        if value != "" {
+            result[value] = true
+        }
+    }
+    return result
+}
+
+func normalizedPathSet(paths []string) map[string]bool {
+    result := map[string]bool{}
+    for _, path := range paths {
+        path = normalizePath(path)
+        if path != "" {
+            result[path] = true
+        }
+    }
+    return result
+}
+
+func unrelatedArtifact(log reviewlog.Summary, changed map[string]bool) bool {
+    if log.Kind != "artifact" {
+        return false
+    }
+    target := normalizePath(log.Target)
+    if target == "" {
+        return false
+    }
+    return !reviewedPathOverlaps(changed, target)
+}
+
+func unrelatedBranchBlocker(blocker findings.Record, current map[string]string) bool {
+    if current["type"] != "branch" || current["id"] == "" {
+        return false
+    }
+    targetType, targetID := findingTarget(blocker.Target)
+    return targetType == "branch" && targetID != "" && targetID != current["id"]
+}
+
+func unrelatedPathBlocker(blocker findings.Record, changed map[string]bool) bool {
+    targetType, targetID := findingTarget(blocker.Target)
+    if targetType != "path" {
+        return false
+    }
+    target := normalizePath(targetID)
+    if target == "" {
+        return false
+    }
+    return !reviewedPathOverlaps(changed, target)
+}
+
+func reviewedPathOverlaps(changed map[string]bool, target string) bool {
+    for path := range changed {
+        if path == target || strings.HasPrefix(path, strings.TrimSuffix(target, "/")+"/") {
+            return true
+        }
+    }
+    return false
+}
+
+func findingTarget(target any) (string, string) {
+    switch typed := target.(type) {
+    case map[string]any:
+        return stringValue(typed["type"]), firstNonEmpty(stringValue(typed["id"]), stringValue(typed["path"]))
+    case map[string]string:
+        return typed["type"], firstNonEmpty(typed["id"], typed["path"])
+    default:
+        return "", ""
+    }
+}
+
+func stringValue(value any) string {
+    typed, _ := value.(string)
+    return typed
+}
+
+func firstNonEmpty(values ...string) string {
+    for _, value := range values {
+        if value != "" {
+            return value
+        }
+    }
+    return ""
+}
+
+func normalizePath(path string) string {
+    path = strings.TrimSpace(path)
+    if path == "" {
+        return ""
+    }
+    return filepath.ToSlash(filepath.Clean(path))
+}
diff --git a/internal/reviewstate/projector_test.go b/internal/reviewstate/projector_test.go
new file mode 100644
index 0000000..14dde07
--- /dev/null
+++ b/internal/reviewstate/projector_test.go
@@ -0,0 +1,300 @@
+package reviewstate
+
+import (
+    "os"
+    "path/filepath"
+    "strings"
+    "testing"
+
+    "github.com/dsifry/metareview/internal/findings"
+    "github.com/dsifry/metareview/internal/reviewlog"
+)
+
+func TestTargetKey(t *testing.T) {
+    if got := TargetKey("pr-ready", map[string]string{"type": "branch", "id": "feature"}); got != "pr-ready:branch:feature" {
+        t.Fatalf("unexpected target key: %s", got)
+    }
+    if got := TargetKey("artifact", map[string]string{"type": "path", "path": "docs/spec.md"}); got != "artifact:path:docs/spec.md" {
+        t.Fatalf("unexpected path target key: %s", got)
+    }
+}
+
+func TestProjectReadsRepositoryReviewState(t *testing.T) {
+    root := t.TempDir()
+    writeFile(t, filepath.Join(root, "docs", "metareview", "reviews", "artifact.md"),
+        "# metareview: artifact review\n\n"+
+            "Run ID: `mrv-artifact`\n\n"+
+            "Target: `docs/spec.md`\n\n"+
+            "## Verdict\n\nNOT_REVIEWED\n")
+    writeFile(t, filepath.Join(root, ".metareview", "findings.jsonl"),
+        `{"schemaVersion":1,"id":"mrvf-path-001","runId":"mrv-artifact","status":"open","classification":"blocking","severity":"high","target":{"type":"path","path":"docs/spec.md"}}`+"\n")
+
+    projection, err := Project(root, Options{
+        Scope:        "pr-ready",
+        Target:       map[string]string{"type": "branch", "id": "feature"},
+        ChangedPaths: []string{"lib/parser.js"},
+    })
+    if err != nil {
+        t.Fatalf("project repository state: %v", err)
+    }
+
+    if projection.TargetKey() != "pr-ready:branch:feature" {
+        t.Fatalf("unexpected target key: %s", projection.TargetKey())
+    }
+    if len(projection.CurrentBlockers()) != 0 {
+        t.Fatalf("expected unrelated path blocker to be historical: %+v", projection.CurrentBlockers())
+    }
+    if len(projection.HistoricalUnrelated()) != 1 || projection.HistoricalUnrelated()[0].RunID != "mrv-artifact" {
+        t.Fatalf("expected artifact log to be historical: %+v", projection.HistoricalUnrelated())
+    }
+    if len(projection.HistoricalBlockers()) != 1 || projection.HistoricalBlockers()[0].ID != "mrvf-path-001" {
+        t.Fatalf("expected path blocker to be historical: %+v", projection.HistoricalBlockers())
+    }
+}
+
+func TestProjectResolvesPreviousRunChainFromRepositoryState(t *testing.T) {
+    root := t.TempDir()
+    writeFile(t, filepath.Join(root, ".metareview", "runs.jsonl"),
+        `{"id":"mrv-root","scope":"pr-ready","target":{"type":"branch","id":"feature"},"verdict":"NEEDS_REVISION","attemptNumber":1,"maxAttempts":3}`+"\n"+
+            `{"id":"mrv-leaf","scope":"pr-ready","target":{"type":"branch","id":"feature"},"verdict":"NEEDS_REVISION","previousRunId":"mrv-root","attemptNumber":2,"maxAttempts":3}`+"\n")
+    writeFile(t, filepath.Join(root, "docs", "metareview", "reviews", "root.md"),
+        "# metareview: pr-ready review\n\n"+
+            "Run ID: `mrv-root`\n\n"+
+            "Target: `feature`\n\n"+
+            "## Verdict\n\nNEEDS_REVISION\n")
+    writeFile(t, filepath.Join(root, "docs", "metareview", "reviews", "leaf.md"),
+        "# metareview: pr-ready review\n\n"+
+            "Run ID: `mrv-leaf`\n\n"+
+            "Target: `feature`\n\n"+
+            "## Verdict\n\nNEEDS_REVISION\n")
+    writeFile(t, filepath.Join(root, ".metareview", "findings.jsonl"),
+        `{"schemaVersion":1,"id":"mrvf-root-001","runId":"mrv-root","status":"open","classification":"blocking","severity":"high","target":{"type":"branch","id":"feature"}}`+"\n"+
+            `{"schemaVersion":1,"id":"mrvf-leaf-001","runId":"mrv-leaf","status":"open","classification":"blocking","severity":"high","target":{"type":"branch","id":"feature"}}`+"\n")
+
+    projection, err := Project(root, Options{
+        Scope:         "pr-ready",
+        Target:        map[string]string{"type": "branch", "id": "feature"},
+        PreviousRunID: "mrv-leaf",
+    })
+    if err != nil {
+        t.Fatalf("project previous chain: %v", err)
+    }
+
+    if len(projection.CurrentBlockers()) != 0 {
+        t.Fatalf("expected previous-chain blockers to be superseded: %+v", projection.CurrentBlockers())
+    }
+    if !projection.SupersededRunIDs()["mrv-root"] || !projection.SupersededRunIDs()["mrv-leaf"] {
+        t.Fatalf("expected previous-chain run IDs to be superseded: %+v", projection.SupersededRunIDs())
+    }
+    if !projection.SupersededFindingIDs()["mrvf-root-001"] || !projection.SupersededFindingIDs()["mrvf-leaf-001"] {
+        t.Fatalf("expected previous-chain finding IDs to be superseded: %+v", projection.SupersededFindingIDs())
+    }
+}
+
+func TestProjectRejectsMismatchedPreviousRunTarget(t *testing.T) {
+    root := t.TempDir()
+    writeFile(t, filepath.Join(root, ".metareview", "runs.jsonl"),
+        `{"id":"mrv-root","scope":"pr-ready","target":{"type":"branch","id":"other"},"verdict":"NEEDS_REVISION","attemptNumber":1,"maxAttempts":3}`+"\n")
+
+    _, err := Project(root, Options{
+        Scope:         "pr-ready",
+        Target:        map[string]string{"type": "branch", "id": "feature"},
+        PreviousRunID: "mrv-root",
+    })
+    if err == nil || !strings.Contains(err.Error(), "does not match pr-ready feature") {
+        t.Fatalf("expected target mismatch error, got %v", err)
+    }
+}
+
+func TestProjectFiltersPreviousRunChainState(t *testing.T) {
+    logs := []reviewlog.Summary{
+        {RunID: "mrv-old", Target: "codex/issue-2", Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
+        {RunID: "mrv-other", Target: "task-2", Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
+    }
+    blockers := []findings.Record{
+        {ID: "mrvf-old-001", RunID: "mrv-old", Status: "open", Classification: "blocking", Severity: "high"},
+        {ID: "mrvf-other-001", RunID: "mrv-other", Status: "open", Classification: "blocking", Severity: "high"},
+    }
+
+    projection := ProjectRecords(logs, blockers, Options{PreviousRunIDs: []string{"mrv-old"}})
+
+    if len(projection.CurrentReviewLogs()) != 1 || projection.CurrentReviewLogs()[0].RunID != "mrv-other" {
+        t.Fatalf("expected only non-previous review log to remain current: %+v", projection.CurrentReviewLogs())
+    }
+    if len(projection.CurrentBlockers()) != 1 || projection.CurrentBlockers()[0].RunID != "mrv-other" {
+        t.Fatalf("expected only non-previous blocker to remain current: %+v", projection.CurrentBlockers())
+    }
+    if !projection.SupersededRunIDs()["mrv-old"] || !projection.SupersededFindingIDs()["mrvf-old-001"] {
+        t.Fatalf("expected previous run and finding to be marked superseded: %+v", projection)
+    }
+}
+
+func writeFile(t *testing.T, path, text string) {
+    t.Helper()
+    if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
+        t.Fatal(err)
+    }
+    if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
+        t.Fatal(err)
+    }
+}
+
+func TestLegacyPreviousRunIDsRecoversChainFromLogs(t *testing.T) {
+    logs := []reviewlog.Summary{
+        {RunID: "mrv-root", Kind: "pr-ready", Target: "current branch", Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
+        {RunID: "mrv-leaf", Kind: "pr-ready", Target: "current branch", Verdict: "NEEDS_REVISION", PreviousRunID: "mrv-root", HasUnresolvedBlockers: true},
+    }
+
+    ids := LegacyPreviousRunIDs(logs, "mrv-leaf")
+
+    if len(ids) != 2 || ids[0] != "mrv-root" || ids[1] != "mrv-leaf" {
+        t.Fatalf("expected root-to-leaf legacy chain IDs, got %+v", ids)
+    }
+}
+
+func TestProjectDoesNotApplyUnvalidatedLegacyPreviousRunID(t *testing.T) {
+    logs := []reviewlog.Summary{
+        {RunID: "mrv-task", Kind: "task-done", Target: "task-1", Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
+    }
+    blockers := []findings.Record{
+        {ID: "mrvf-task-001", RunID: "mrv-task", Status: "open", Classification: "blocking", Severity: "high"},
+    }
+
+    projection := ProjectRecords(logs, blockers, Options{})
+
+    if len(projection.CurrentReviewLogs()) != 1 || projection.CurrentReviewLogs()[0].RunID != "mrv-task" {
+        t.Fatalf("projector should not filter legacy logs without validated previous IDs: %+v", projection.CurrentReviewLogs())
+    }
+    if len(projection.CurrentBlockers()) != 1 || projection.CurrentBlockers()[0].RunID != "mrv-task" {
+        t.Fatalf("projector should not filter legacy blockers without validated previous IDs: %+v", projection.CurrentBlockers())
+    }
+}
+
+func TestProjectTreatsUnrelatedArtifactLogAsHistorical(t *testing.T) {
+    logs := []reviewlog.Summary{
+        {RunID: "mrv-artifact", Kind: "artifact", Target: "docs/spec.md", Verdict: "NOT_REVIEWED", HasUnresolvedBlockers: true},
+    }
+
+    projection := ProjectRecords(logs, nil, Options{ChangedPaths: []string{"lib/parser.js"}})
+
+    if len(projection.CurrentReviewLogs()) != 0 {
+        t.Fatalf("unrelated artifact log should not remain current: %+v", projection.CurrentReviewLogs())
+    }
+    if len(projection.HistoricalUnrelated()) != 1 || projection.HistoricalUnrelated()[0].RunID != "mrv-artifact" {
+        t.Fatalf("expected unrelated artifact to be historical: %+v", projection.HistoricalUnrelated())
+    }
+}
+
+func TestProjectTreatsArtifactLogAsHistoricalWhenNoPathsReviewed(t *testing.T) {
+    logs := []reviewlog.Summary{
+        {RunID: "mrv-artifact", Kind: "artifact", Target: "docs/spec.md", Verdict: "NOT_REVIEWED", HasUnresolvedBlockers: true},
+    }
+
+    projection := ProjectRecords(logs, nil, Options{})
+
+    if len(projection.CurrentReviewLogs()) != 0 {
+        t.Fatalf("artifact log should not block when no reviewed path overlaps it: %+v", projection.CurrentReviewLogs())
+    }
+    if len(projection.HistoricalUnrelated()) != 1 || projection.HistoricalUnrelated()[0].RunID != "mrv-artifact" {
+        t.Fatalf("expected artifact log to be historical: %+v", projection.HistoricalUnrelated())
+    }
+}
+
+func TestProjectTreatsBlockersFromUnrelatedArtifactRunAsHistorical(t *testing.T) {
+    logs := []reviewlog.Summary{
+        {RunID: "mrv-artifact", Kind: "artifact", Target: "docs/spec.md", Verdict: "NOT_REVIEWED", HasUnresolvedBlockers: true},
+    }
+    blockers := []findings.Record{
+        {ID: "mrvf-artifact-001", RunID: "mrv-artifact", Status: "open", Classification: "blocking", Severity: "high"},
+        {ID: "mrvf-ambiguous-001", RunID: "mrv-ambiguous", Status: "open", Classification: "blocking", Severity: "high"},
+    }
+
+    projection := ProjectRecords(logs, blockers, Options{ChangedPaths: []string{"lib/parser.js"}})
+
+    if len(projection.CurrentBlockers()) != 1 || projection.CurrentBlockers()[0].RunID != "mrv-ambiguous" {
+        t.Fatalf("expected only ambiguous blocker to remain current: %+v", projection.CurrentBlockers())
+    }
+    if projection.SupersededFindingIDs()["mrvf-artifact-001"] {
+        t.Fatalf("unrelated historical blocker should not be marked fixed/superseded: %+v", projection.SupersededFindingIDs())
+    }
+}
+
+func TestProjectTreatsUnrelatedPathBlockerWithoutLogAsHistorical(t *testing.T) {
+    blockers := []findings.Record{
+        {ID: "mrvf-path-001", RunID: "mrv-path", Status: "open", Classification: "blocking", Severity: "high", Target: map[string]any{"type": "path", "path": "docs/spec.md"}},
+        {ID: "mrvf-ambiguous-001", RunID: "mrv-ambiguous", Status: "open", Classification: "blocking", Severity: "high"},
+    }
+
+    projection := ProjectRecords(nil, blockers, Options{ChangedPaths: []string{"lib/parser.js"}})
+
+    if len(projection.CurrentBlockers()) != 1 || projection.CurrentBlockers()[0].RunID != "mrv-ambiguous" {
+        t.Fatalf("expected only ambiguous blocker to remain current: %+v", projection.CurrentBlockers())
+    }
+    if len(projection.HistoricalBlockers()) != 1 || projection.HistoricalBlockers()[0].RunID != "mrv-path" {
+        t.Fatalf("expected path blocker to be historical: %+v", projection.HistoricalBlockers())
+    }
+}
+
+func TestProjectKeepsRelevantPathBlockerCurrent(t *testing.T) {
+    blockers := []findings.Record{
+        {ID: "mrvf-path-001", RunID: "mrv-path", Status: "open", Classification: "blocking", Severity: "high", Target: map[string]any{"type": "path", "path": "lib/parser.js"}},
+    }
+
+    projection := ProjectRecords(nil, blockers, Options{ChangedPaths: []string{"lib/parser.js"}})
+
+    if len(projection.CurrentBlockers()) != 1 || projection.CurrentBlockers()[0].RunID != "mrv-path" {
+        t.Fatalf("expected relevant path blocker to remain current: %+v", projection.CurrentBlockers())
+    }
+}
+
+func TestProjectTreatsMismatchedBranchRunAsHistorical(t *testing.T) {
+    logs := []reviewlog.Summary{
+        {RunID: "mrv-branch-a", Kind: "pr-ready", Target: "current branch", Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true},
+    }
+    blockers := []findings.Record{
+        {ID: "mrvf-branch-a-001", RunID: "mrv-branch-a", Status: "open", Classification: "blocking", Severity: "high", Target: map[string]any{"type": "branch", "id": "branch-a"}},
+        {ID: "mrvf-task-001", RunID: "mrv-task", Status: "open", Classification: "blocking", Severity: "high", Target: map[string]any{"type": "beads-task", "id": "task-1"}},
+    }
+
+    projection := ProjectRecords(logs, blockers, Options{
+        HistoricalRunIDs: []string{"mrv-branch-a"},
+        CurrentTarget:    map[string]string{"type": "branch", "id": "branch-b"},
+    })
+
+    if len(projection.CurrentReviewLogs()) != 0 {
+        t.Fatalf("mismatched branch review log should not remain current: %+v", projection.CurrentReviewLogs())
+    }
+    if len(projection.CurrentBlockers()) != 1 || projection.CurrentBlockers()[0].RunID != "mrv-task" {
+        t.Fatalf("expected task blocker to remain current and branch blocker historical: %+v", projection.CurrentBlockers())
+    }
+    if len(projection.HistoricalBlockers()) != 1 || projection.HistoricalBlockers()[0].RunID != "mrv-branch-a" {
+        t.Fatalf("expected branch blocker to be historical: %+v", projection.HistoricalBlockers())
+    }
+}
+
+func TestProjectTreatsMismatchedBranchBlockerAsHistoricalWithoutLog(t *testing.T) {
+    blockers := []findings.Record{
+        {ID: "mrvf-branch-a-001", RunID: "mrv-branch-a", Status: "open", Classification: "blocking", Severity: "high", Target: map[string]any{"type": "branch", "id": "branch-a"}},
+    }
+
+    projection := ProjectRecords(nil, blockers, Options{CurrentTarget: map[string]string{"type": "branch", "id": "branch-b"}})
+
+    if len(projection.CurrentBlockers()) != 0 {
+        t.Fatalf("mismatched branch blocker should not remain current: %+v", projection.CurrentBlockers())
+    }
+    if len(projection.HistoricalBlockers()) != 1 {
+        t.Fatalf("expected branch blocker to be historical: %+v", projection.HistoricalBlockers())
+    }
+}
+
+func TestProjectKeepsRelevantArtifactLogCurrent(t *testing.T) {
+    logs := []reviewlog.Summary{
+        {RunID: "mrv-artifact", Kind: "artifact", Target: "lib/parser.js", Verdict: "NOT_REVIEWED", HasUnresolvedBlockers: true},
+    }
+
+    projection := ProjectRecords(logs, nil, Options{ChangedPaths: []string{"lib/parser.js"}})
+
+    if len(projection.CurrentReviewLogs()) != 1 || projection.CurrentReviewLogs()[0].RunID != "mrv-artifact" {
+        t.Fatalf("expected changed-path artifact to remain current: %+v", projection.CurrentReviewLogs())
+    }
+}
diff --git a/tests/go/test-pr-ready-review.sh b/tests/go/test-pr-ready-review.sh
index e151728..ccb477b 100644
--- a/tests/go/test-pr-ready-review.sh
+++ b/tests/go/test-pr-ready-review.sh
@@ -80,7 +80,7 @@ cat > docs/metareview/reviews/spec-not-reviewed.md <<'REVIEW'

 Run ID: `mrv-spec-not-reviewed`

-Target: `docs/spec.md`
+Target: `lib/parser.js`

 ## Verdict

@@ -106,7 +106,128 @@ set -e
 test "$code" -eq 1
 incomplete_artifact_review="$(cat "$TMP/incomplete-artifact.out")"
 grep -q "Unresolved review blockers" "$repo/$incomplete_artifact_review"
-grep -q "docs/spec.md" "$repo/$incomplete_artifact_review"
+grep -q "lib/parser.js" "$repo/$incomplete_artifact_review"
+
+repo="$TMP/unrelated-artifact"
+init_repo "$repo"
+base="$(git rev-parse HEAD)"
+mkdir -p docs/metareview/reviews
+cat > docs/metareview/reviews/spec-not-reviewed.md <<'REVIEW'
+# metareview: artifact review
+
+Run ID: `mrv-spec-not-reviewed`
+
+Target: `docs/spec.md`
+
+## Verdict
+
+NOT_REVIEWED
+
+## Reviewer Results
+
+| Reviewer | Verdict | Blocking | Warnings | Notes |
+| --- | --- | ---: | ---: | --- |
+
+## Findings
+
+No reviewer findings recorded yet.
+REVIEW
+mkdir -p .metareview
+cat > .metareview/findings.jsonl <<'JSONL'
+{"schemaVersion":1,"id":"mrvf-spec-not-reviewed-001","runId":"mrv-spec-not-reviewed","status":"open","classification":"blocking","severity":"high","title":"Unrelated artifact blocker","target":{"type":"path","path":"docs/spec.md"}}
+JSONL
+printf "'use strict';\nmodule.exports = input => JSON.parse(input); // unrelated artifact\n" > lib/parser.js
+git add .
+git commit -qm "branch change"
+printf "bash tests/run-all.sh exited 0\n" > "$TMP/unrelated-artifact-evidence.md"
+"$TMP/metareview" review pr-ready --base "$base" --evidence "$TMP/unrelated-artifact-evidence.md" > "$TMP/unrelated-artifact.out"
+unrelated_artifact_review="$(cat "$TMP/unrelated-artifact.out")"
+grep -q "PASS" "$repo/$unrelated_artifact_review"
+! grep -q "Unresolved review blockers" "$repo/$unrelated_artifact_review"
+! grep -q "docs/spec.md" "$repo/$unrelated_artifact_review"
+
+repo="$TMP/unrelated-artifact-working-tree"
+init_repo "$repo"
+base="$(git rev-parse HEAD)"
+mkdir -p docs/metareview/reviews .metareview
+cat > docs/metareview/reviews/spec-not-reviewed.md <<'REVIEW'
+# metareview: artifact review
+
+Run ID: `mrv-spec-not-reviewed`
+
+Target: `docs/spec.md`
+
+## Verdict
+
+NOT_REVIEWED
+
+## Reviewer Results
+
+| Reviewer | Verdict | Blocking | Warnings | Notes |
+| --- | --- | ---: | ---: | --- |
+
+## Findings
+
+No reviewer findings recorded yet.
+REVIEW
+cat > .metareview/findings.jsonl <<'JSONL'
+{"schemaVersion":1,"id":"mrvf-spec-not-reviewed-001","runId":"mrv-spec-not-reviewed","status":"open","classification":"blocking","severity":"high","title":"Unrelated artifact blocker","target":{"type":"path","path":"docs/spec.md"}}
+JSONL
+printf "'use strict';\nmodule.exports = input => JSON.parse(input); // working tree artifact relevance\n" > lib/parser.js
+printf "bash tests/run-all.sh exited 0\n" > "$TMP/unrelated-artifact-working-tree-evidence.md"
+"$TMP/metareview" review pr-ready --base "$base" --evidence "$TMP/unrelated-artifact-working-tree-evidence.md" > "$TMP/unrelated-artifact-working-tree.out"
+unrelated_artifact_working_tree_review="$(cat "$TMP/unrelated-artifact-working-tree.out")"
+grep -q "PASS" "$repo/$unrelated_artifact_working_tree_review"
+! grep -q "Unresolved review blockers" "$repo/$unrelated_artifact_working_tree_review"
+! grep -q "docs/spec.md" "$repo/$unrelated_artifact_working_tree_review"
+
+repo="$TMP/unrelated-path-finding-no-log"
+init_repo "$repo"
+base="$(git rev-parse HEAD)"
+mkdir -p .metareview
+cat > .metareview/findings.jsonl <<'JSONL'
+{"schemaVersion":1,"id":"mrvf-path-001","runId":"mrv-path","status":"open","classification":"blocking","severity":"high","title":"Unrelated path blocker","target":{"type":"path","path":"docs/spec.md"}}
+JSONL
+printf "'use strict';\nmodule.exports = input => JSON.parse(input); // unrelated path finding without log\n" > lib/parser.js
+git add .
+git commit -qm "branch change"
+printf "bash tests/run-all.sh exited 0\n" > "$TMP/unrelated-path-finding-no-log-evidence.md"
+"$TMP/metareview" review pr-ready --base "$base" --evidence "$TMP/unrelated-path-finding-no-log-evidence.md" > "$TMP/unrelated-path-finding-no-log.out"
+unrelated_path_finding_no_log_review="$(cat "$TMP/unrelated-path-finding-no-log.out")"
+grep -q "PASS" "$repo/$unrelated_path_finding_no_log_review"
+! grep -q "Unresolved review blockers" "$repo/$unrelated_path_finding_no_log_review"
+! grep -q "docs/spec.md" "$repo/$unrelated_path_finding_no_log_review"
+
+repo="$TMP/wrong-scope-previous"
+init_repo "$repo"
+base="$(git rev-parse HEAD)"
+mkdir -p docs/metareview/reviews
+cat > docs/metareview/reviews/task-blocked.md <<'REVIEW'
+# metareview: task-done review
+
+Run ID: `mrv-task-blocked`
+
+Target: `task-1`
+
+## Verdict
+
+NEEDS_REVISION
+
+## Findings
+
+### mrvf-task-001: Existing task blocker
+REVIEW
+printf "'use strict';\nmodule.exports = input => JSON.parse(input); // wrong previous scope\n" > lib/parser.js
+git add .
+git commit -qm "branch change"
+printf "bash tests/run-all.sh exited 0\n" > "$TMP/wrong-scope-evidence.md"
+set +e
+"$TMP/metareview" review pr-ready --base "$base" --previous-run mrv-task-blocked --evidence "$TMP/wrong-scope-evidence.md" > "$TMP/wrong-scope.out" 2>"$TMP/wrong-scope.err"
+wrong_scope_code=$?
+set -e
+test "$wrong_scope_code" -ne 0
+test ! -s "$TMP/wrong-scope.out"
+grep -q "previous run mrv-task-blocked not found" "$TMP/wrong-scope.err"

 repo="$TMP/missing-validation"
 init_repo "$repo"
@@ -121,6 +242,119 @@ set -e
 test "$code" -eq 1
 missing_review="$(cat "$TMP/missing-validation.out")"
 grep -q "Missing validation evidence" "$repo/$missing_review"
+previous_missing_run="$(basename "$missing_review" .md)"
+printf "bash tests/run-all.sh exited 0\n" > "$TMP/fixed-validation.md"
+rm .metareview/runs.jsonl
+"$TMP/metareview" review pr-ready --base "$base" --previous-run "$previous_missing_run" --evidence "$TMP/fixed-validation.md" > "$TMP/fixed-validation.out"
+fixed_review="$(cat "$TMP/fixed-validation.out")"
+grep -q "PASS" "$repo/$fixed_review"
+! grep -q "Unresolved review blockers" "$repo/$fixed_review"
+! grep -q "Missing validation evidence" "$repo/$fixed_review"
+
+repo="$TMP/cross-branch-current-log"
+init_repo "$repo"
+base="$(git rev-parse HEAD)"
+git checkout -qb branch-a
+printf "'use strict';\nmodule.exports = input => JSON.parse(input); // branch a blocker\n" > lib/parser.js
+git add .
+git commit -qm "branch a change"
+set +e
+"$TMP/metareview" review pr-ready --base "$base" > "$TMP/current-log-branch-a.out" 2>"$TMP/current-log-branch-a.err"
+current_log_branch_a_code=$?
+set -e
+test "$current_log_branch_a_code" -eq 1
+git checkout -qb branch-b "$base"
+printf "'use strict';\nmodule.exports = input => JSON.parse(input); // branch b valid\n" > lib/parser.js
+git add .
+git commit -qm "branch b change"
+printf "bash tests/run-all.sh exited 0\n" > "$TMP/current-log-branch-b-evidence.md"
+"$TMP/metareview" review pr-ready --base "$base" --evidence "$TMP/current-log-branch-b-evidence.md" > "$TMP/current-log-branch-b.out"
+current_log_branch_b_review="$(cat "$TMP/current-log-branch-b.out")"
+grep -q "PASS" "$repo/$current_log_branch_b_review"
+! grep -q "Unresolved review blockers" "$repo/$current_log_branch_b_review"
+
+repo="$TMP/cross-branch-previous"
+init_repo "$repo"
+base="$(git rev-parse HEAD)"
+git checkout -qb branch-a
+printf "'use strict';\nmodule.exports = input => JSON.parse(input); // branch a missing validation\n" > lib/parser.js
+git add .
+git commit -qm "branch a change"
+set +e
+"$TMP/metareview" review pr-ready --base "$base" > "$TMP/branch-a.out" 2>"$TMP/branch-a.err"
+branch_a_code=$?
+set -e
+test "$branch_a_code" -eq 1
+branch_a_review="$(cat "$TMP/branch-a.out")"
+branch_a_previous_run="$(basename "$branch_a_review" .md)"
+git checkout -qb branch-b "$base"
+printf "'use strict';\nmodule.exports = input => JSON.parse(input); // branch b fixed validation\n" > lib/parser.js
+git add .
+git commit -qm "branch b change"
+rm .metareview/runs.jsonl
+printf "bash tests/run-all.sh exited 0\n" > "$TMP/branch-b-evidence.md"
+set +e
+"$TMP/metareview" review pr-ready --base "$base" --previous-run "$branch_a_previous_run" --evidence "$TMP/branch-b-evidence.md" > "$TMP/branch-b.out" 2>"$TMP/branch-b.err"
+branch_b_code=$?
+set -e
+test "$branch_b_code" -ne 0
+test ! -s "$TMP/branch-b.out"
+grep -q "previous run $branch_a_previous_run not found" "$TMP/branch-b.err"
+
+repo="$TMP/detached-legacy-previous"
+init_repo "$repo"
+base="$(git rev-parse HEAD)"
+printf "'use strict';\nmodule.exports = input => JSON.parse(input); // branch missing validation before detach\n" > lib/parser.js
+git add .
+git commit -qm "branch missing validation"
+set +e
+"$TMP/metareview" review pr-ready --base "$base" > "$TMP/detached-branch.out" 2>"$TMP/detached-branch.err"
+detached_branch_code=$?
+set -e
+test "$detached_branch_code" -eq 1
+detached_previous_run="$(basename "$(cat "$TMP/detached-branch.out")" .md)"
+head_sha="$(git rev-parse HEAD)"
+git checkout -q --detach "$head_sha"
+rm .metareview/runs.jsonl
+printf "bash tests/run-all.sh exited 0\n" > "$TMP/detached-evidence.md"
+set +e
+"$TMP/metareview" review pr-ready --base "$base" --previous-run "$detached_previous_run" --evidence "$TMP/detached-evidence.md" > "$TMP/detached.out" 2>"$TMP/detached.err"
+detached_code=$?
+set -e
+test "$detached_code" -ne 0
+test ! -s "$TMP/detached.out"
+grep -q "previous run $detached_previous_run not found" "$TMP/detached.err"
+
+repo="$TMP/escalated-legacy-previous"
+init_repo "$repo"
+base="$(git rev-parse HEAD)"
+printf "'use strict';\nmodule.exports = input => JSON.parse(input); // escalated previous\n" > lib/parser.js
+git add .
+git commit -qm "escalated branch change"
+set +e
+"$TMP/metareview" review pr-ready --base "$base" --max-attempts 1 > "$TMP/escalated.out" 2>"$TMP/escalated.err"
+escalated_code=$?
+set -e
+test "$escalated_code" -eq 1
+escalated_review="$(cat "$TMP/escalated.out")"
+grep -q "ESCALATED" "$repo/$escalated_review"
+escalated_previous_run="$(basename "$escalated_review" .md)"
+rm .metareview/runs.jsonl
+printf "bash tests/run-all.sh exited 0\n" > "$TMP/escalated-fixed-evidence.md"
+set +e
+"$TMP/metareview" review pr-ready --base "$base" --previous-run "$escalated_previous_run" --evidence "$TMP/escalated-fixed-evidence.md" > "$TMP/escalated-fixed.out" 2>"$TMP/escalated-fixed.err"
+escalated_fixed_code=$?
+set -e
+test "$escalated_fixed_code" -ne 0
+test ! -s "$TMP/escalated-fixed.out"
+grep -q "previous run $escalated_previous_run already escalated" "$TMP/escalated-fixed.err"
+set +e
+"$TMP/metareview" review pr-ready --base "$base" --evidence "$TMP/escalated-fixed-evidence.md" > "$TMP/escalated-fresh.out" 2>"$TMP/escalated-fresh.err"
+escalated_fresh_code=$?
+set -e
+test "$escalated_fresh_code" -ne 0
+test ! -s "$TMP/escalated-fresh.out"
+grep -q "same target already escalated in run $escalated_previous_run" "$TMP/escalated-fresh.err"

 repo="$TMP/clean"
 init_repo "$repo"

diff --git a/.claude-plugin/marketplace.json b/.claude-plugin/marketplace.json
index 1dd7f72..6ac640b 100644
--- a/.claude-plugin/marketplace.json
+++ b/.claude-plugin/marketplace.json
@@ -8,7 +8,7 @@
     {
       "name": "metareview",
       "description": "Internal review harness, adversarial gates, and post-merge learning for coding agents",
-      "version": "0.3.1",
+      "version": "0.6.0",
       "source": "./",
       "author": {
         "name": "David Sifry"
diff --git a/.claude-plugin/plugin.json b/.claude-plugin/plugin.json
index 18815c4..c78add1 100644
--- a/.claude-plugin/plugin.json
+++ b/.claude-plugin/plugin.json
@@ -1,6 +1,6 @@
 {
   "name": "metareview",
-  "version": "0.3.1",
+  "version": "0.6.0",
   "description": "Go-based metaswarm-compatible internal review harness for plans, specs, decompositions, task-done code review, acceptance evidence, PR readiness, and post-merge learning. Packaged releases use bin/metareview; source checkout mode requires Go 1.22+.",
   "author": {
     "name": "David Sifry"
diff --git a/.codex-plugin/plugin.json b/.codex-plugin/plugin.json
index 49c9e1e..05bbc95 100644
--- a/.codex-plugin/plugin.json
+++ b/.codex-plugin/plugin.json
@@ -1,6 +1,6 @@
 {
   "name": "metareview",
-  "version": "0.3.1",
+  "version": "0.6.0",
   "description": "Go-based metaswarm-compatible internal review harness for plans, specs, decompositions, task-done code review, acceptance evidence, PR readiness, and post-merge learning",
   "author": {
     "name": "David Sifry"
diff --git a/internal/version/version.go b/internal/version/version.go
index c1635d2..6cc8872 100644
--- a/internal/version/version.go
+++ b/internal/version/version.go
@@ -1,3 +1,3 @@
 package version

-const Version = "0.4.0"
+const Version = "0.6.0"
diff --git a/package.json b/package.json
index 6ae54cf..a8bc2d7 100644
--- a/package.json
+++ b/package.json
@@ -1,6 +1,6 @@
 {
   "name": "metareview",
-  "version": "0.4.0",
+  "version": "0.6.0",
   "description": "Go-based metaswarm-compatible internal review harness for plans, specs, decompositions, code, acceptance evidence, PR readiness, and post-merge learning",
   "bin": {
     "metareview": "cli/metareview.js"

```

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

Verification evidence:
- bash tests/manifest/test-manifests.sh exited 1 before fix with Error: version mismatch
- bash tests/manifest/test-manifests.sh exited 0 after aligning package/plugin/version sources to 0.6.0
- go test ./... exited 0
- git diff --check exited 0
- bash tests/run-all.sh exited 0
- go run ./cmd/metareview --version exited 0 and printed 0.6.0
