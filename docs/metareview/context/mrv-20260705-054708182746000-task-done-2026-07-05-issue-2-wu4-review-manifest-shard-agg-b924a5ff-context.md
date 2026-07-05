# metareview task-done context

Run ID: `mrv-20260705-054708182746000-task-done-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff`

## Task

# Issue #2 WU4: Review Manifest Shard Aggregation

## Objective

Add the first review manifest slice for Issue #2: a deterministic schema and aggregator that can represent shard review coverage, shard results, and the required cross-shard review for multi-shard changes.

## Scope

- Add an internal manifest package for review coverage and shard aggregation.
- Reuse the existing `internal/contextprofile` shard plan shape from WU3.
- Render a manifest summary into task-done and pr-ready context packs so agents can see the shard coverage contract during review.
- Define the result-ingestion boundary as deterministic, versioned data passed to the manifest builder. In this slice, task-done and pr-ready will render an empty pending-result manifest; later CLI or skill work can populate shard results from stable files or evidence records using the same schema.
- Keep agent execution out of the Go CLI. The CLI should model and aggregate externally produced shard results, not spawn Codex, Claude, or other runtimes.

## Non-Goals

- Do not implement a full shard runner.
- Do not ingest GitHub review threads.
- Do not add runtime/deployment evidence.
- Do not replace existing task-done or pr-ready verdict logic in this slice. Aggregation produces a manifest verdict/status that is rendered as evidence; existing gates do not consume it as an additional blocker until a later integration slice.

## Acceptance Criteria

- A manifest records source paths, generated or out-of-scope path dispositions, the shard plan, shard review results, and an optional cross-shard result.
- The canonical coverage universe accounts for every changed path exactly once: generated-filtered `contextprofile.Profile.Files` paths are primary source paths, and every generated-excluded or caller-marked out-of-scope changed path must appear as a disposition with a valid rationale.
- Generated or out-of-scope path dispositions must include a rationale. A rationale is invalid when it is blank after trimming, shorter than 12 characters, or uses placeholder text such as `n/a`, `none`, `todo`, or `tbd`.
- Aggregation blocks when a canonical source path is assigned to zero or multiple primary shards.
- Aggregation blocks when any planned shard lacks a review result.
- The manifest exposes a deterministic `sourceManifestHash` computed from schema version, source paths, path dispositions, shard IDs, shard paths, shard byte counts, and the underlying WU3 source diff hash. This hash is the freshness key for shard and cross-shard results.
- Aggregation blocks when a shard result is stale for the current `sourceManifestHash`.
- Aggregation blocks when a cross-shard result is stale for the current `sourceManifestHash` or does not cover the current shard set.
- Shard and cross-shard results must use a versioned schema with canonical ordering and closed enums for verdict (`PASS`, `PASS_ADVISORY`, `NEEDS_REVISION`, `ESCALATED`), severity (`low`, `medium`, `high`, `critical`), and disposition (`fixed`, `waived`, `accepted-risk`, `false-positive`, `deferred`, `open`).
- Shard and cross-shard results must carry evidence-backed provenance: result ID or path, reviewer/source, source manifest hash, covered paths, file:line evidence or acceptance/path coverage notes, severity, and disposition.
- Aggregation blocks when any shard result or cross-shard result has blockers, a blocking verdict, missing provenance, missing coverage evidence, or unresolved medium-or-higher findings without an explicit disposition.
- Aggregation requires a cross-shard review for multi-shard manifests.
- Aggregation passes when every primary shard has a fresh passing result and the required cross-shard review passes.
- Task-done and pr-ready context packs include a readable Review Manifest section whose manifest verdict is separate from the existing gate verdict.
- The manifest summary explicitly says `Runtime assessment: static-only; runtime not assessed` so a passing manifest cannot imply live/deployed verification.

## Validation

- `go test ./...`
- `bash tests/run-all.sh`
- `git diff --check -- ':!docs/metareview/context/**' ':!docs/metareview/reviews/**'`
- `go run ./cmd/metareview review task-done issue-2-wu4 --base origin/main --evidence <evidence-file>`
- `go run ./cmd/metareview review pr-ready --base origin/main --evidence <evidence-file>`

## Deterministic Test Cases

- Source path coverage: every changed path appears exactly once as a primary source path or disposition; zero source assignment blocks; duplicate primary assignment blocks; generated/out-of-scope paths require valid rationale; invalid rationale placeholders block.
- Source manifest hash: hash changes when source paths, dispositions, shard IDs, shard paths, shard byte counts, or WU3 source diff hash change; hash is stable under input ordering differences.
- Result schema: unknown verdict, severity, or disposition enum values block deterministically; output ordering is canonical.
- Shard results: missing result blocks; stale source manifest hash blocks; blocking verdict or blocker count blocks; missing provenance or coverage evidence blocks; unresolved medium-or-higher findings without explicit disposition block.
- Cross-shard results: missing cross-shard review blocks for multi-shard manifests; stale source manifest hash blocks; mismatched shard set blocks; blocking verdict blocks; missing provenance or coverage evidence blocks; unresolved medium-or-higher findings without explicit disposition block.
- Passing cases: single-shard manifest can pass without cross-shard review; multi-shard manifest passes only with fresh passing shard results and fresh passing cross-shard result.
- Rendering: task-done and pr-ready context packs include Review Manifest, manifest verdict, coverage summary, pending shard result state, and `Runtime assessment: static-only; runtime not assessed`.


## Git

- Base: `761450000b48d1b14dfca36c0ca4d26e5dc93d41`
- Head: `761450000b48d1b14dfca36c0ca4d26e5dc93d41`
- Branch: `codex/issue-2-wu4`
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `60828`
- Filtered diff bytes: `36725`
- Risk level: `context-risk`
- Risk reasons: `UNTRACKED_TRUNCATED`
- Generated files excluded: docs/metareview/context/mrv-20260705-052913413096000-artifact-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff-context.md, docs/metareview/context/mrv-20260705-053151045920000-artifact-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff-context.md, docs/metareview/context/mrv-20260705-053422816168000-artifact-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff-context.md, docs/metareview/context/mrv-20260705-053741420316000-artifact-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff-context.md, docs/metareview/reviews/mrv-20260705-052913413096000-artifact-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff.md, docs/metareview/reviews/mrv-20260705-053151045920000-artifact-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff.md, docs/metareview/reviews/mrv-20260705-053422816168000-artifact-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff.md, docs/metareview/reviews/mrv-20260705-053741420316000-artifact-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff.md
- Untracked files truncated: `3`

## Context Shard Plan

- Source diff hash: `ccd589a1a896064f`
- shard-01: docs/superpowers/plans/2026-07-05-issue-2-wu4-review-manifest-shard-aggregation.md, internal/prready/review.go, internal/prready/review_markdown_test.go, internal/reviewmanifest/manifest.go, internal/reviewmanifest/manifest_test.go, internal/taskdone/review.go, internal/taskdone/review_markdown_test.go (19887 bytes, prompt pack `docs/metareview/shards/ccd589a1a896064f-shard-01.md`)

## Review Manifest

- Manifest verdict: `NEEDS_REVISION`
- Source manifest hash: `c907f7233905b1a8`
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- docs/superpowers/plans/2026-07-05-issue-2-wu4-review-manifest-shard-aggregation.md
- internal/prready/review.go
- internal/prready/review_markdown_test.go
- internal/reviewmanifest/manifest.go
- internal/reviewmanifest/manifest_test.go
- internal/taskdone/review.go
- internal/taskdone/review_markdown_test.go

### Shards
- shard-01: docs/superpowers/plans/2026-07-05-issue-2-wu4-review-manifest-shard-aggregation.md, internal/prready/review.go, internal/prready/review_markdown_test.go, internal/reviewmanifest/manifest.go, internal/reviewmanifest/manifest_test.go, internal/taskdone/review.go, internal/taskdone/review_markdown_test.go

### Manifest Blockers
- missing disposition for docs/metareview/context/mrv-20260705-052913413096000-artifact-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff-context.md
- missing disposition for docs/metareview/context/mrv-20260705-053151045920000-artifact-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff-context.md
- missing disposition for docs/metareview/context/mrv-20260705-053422816168000-artifact-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff-context.md
- missing disposition for docs/metareview/context/mrv-20260705-053741420316000-artifact-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff-context.md
- missing disposition for docs/metareview/reviews/mrv-20260705-052913413096000-artifact-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff.md
- missing disposition for docs/metareview/reviews/mrv-20260705-053151045920000-artifact-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff.md
- missing disposition for docs/metareview/reviews/mrv-20260705-053422816168000-artifact-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff.md
- missing disposition for docs/metareview/reviews/mrv-20260705-053741420316000-artifact-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff.md
- missing shard result for shard-01

## Changed Files

- internal/prready/review.go
- internal/prready/review_markdown_test.go
- internal/taskdone/review.go
- internal/taskdone/review_markdown_test.go
- docs/superpowers/plans/2026-07-05-issue-2-wu4-review-manifest-shard-aggregation.md
- internal/reviewmanifest/manifest.go
- internal/reviewmanifest/manifest_test.go

## Diff

```diff


diff --git a/internal/prready/review.go b/internal/prready/review.go
index c87235f..70efddc 100644
--- a/internal/prready/review.go
+++ b/internal/prready/review.go
@@ -17,6 +17,7 @@ import (
 	"github.com/dsifry/metareview/internal/repo"
 	"github.com/dsifry/metareview/internal/reviewers"
 	"github.com/dsifry/metareview/internal/reviewlog"
+	"github.com/dsifry/metareview/internal/reviewmanifest"
 	"github.com/dsifry/metareview/internal/reviewstate"
 	"github.com/dsifry/metareview/internal/runchain"
 	"github.com/dsifry/metareview/internal/state"
@@ -743,6 +744,7 @@ func contextMarkdown(runID string, git gitcontext.Context, profile contextprofil
 		"- Gate effect: " + markdown.InlineCode(gateEffect) + "\n\n" +
 		contextprofile.Markdown(profile) + "\n\n" +
 		contextprofile.ShardPlanMarkdown(profile, contextprofile.ShardOptions{MaxBytesPerShard: contextprofile.DefaultMaxBytesPerShard, GroupBy: "path"}) + "\n\n" +
+		reviewManifestMarkdown("pr-ready", map[string]string{"type": "branch", "id": firstNonEmpty(git.Branch, git.HeadSHA)}, profile) + "\n\n" +
 		"## Changed Files\n\n" + markdownList(changed, "No changed files.") + "\n\n" +
 		"## Diff\n\n" + markdown.FencedCodeBlock("diff", strings.Join([]string{git.Diff, git.StagedDiff, git.WorkingTreeDiff, git.UntrackedExcerpts}, "\n")) + "\n\n" +
 		"## Review Logs\n\n" + reviewLogsMarkdown(logs) + "\n\n" +
@@ -752,6 +754,15 @@ func contextMarkdown(runID string, git gitcontext.Context, profile contextprofil
 		"## Suggested PR Evidence\n\n" + prEvidence + "\n"
 }
 
+func reviewManifestMarkdown(scope string, target map[string]string, profile contextprofile.Profile) string {
+	plan, err := contextprofile.PlanShards(profile, contextprofile.ShardOptions{MaxBytesPerShard: contextprofile.DefaultMaxBytesPerShard, GroupBy: "path"})
+	if err != nil {
+		return "## Review Manifest\n\nUnable to generate review manifest: " + err.Error()
+	}
+	manifest := reviewmanifest.Build(reviewmanifest.Input{Scope: scope, Target: target, Profile: profile, ShardPlan: plan})
+	return reviewmanifest.Markdown(manifest, reviewmanifest.Aggregate(manifest))
+}
+
 type reviewMetadata struct {
 	AttemptNumber        int
 	MaxAttempts          int
diff --git a/internal/prready/review_markdown_test.go b/internal/prready/review_markdown_test.go
index 3ff4aa2..8923290 100644
--- a/internal/prready/review_markdown_test.go
+++ b/internal/prready/review_markdown_test.go
@@ -4,7 +4,11 @@ import (
 	"strings"
 	"testing"
 
+	"github.com/dsifry/metareview/internal/contextprofile"
 	"github.com/dsifry/metareview/internal/findings"
+	"github.com/dsifry/metareview/internal/gitcontext"
+	"github.com/dsifry/metareview/internal/githubcontext"
+	"github.com/dsifry/metareview/internal/knowledge"
 	"github.com/dsifry/metareview/internal/runchain"
 )
 
@@ -49,3 +53,28 @@ func TestRunChainMarkdownIncludesEscalationDetails(t *testing.T) {
 		}
 	}
 }
+
+func TestContextMarkdownIncludesReviewManifest(t *testing.T) {
+	body := contextMarkdown(
+		"mrv-pr",
+		gitcontext.Context{BaseSHA: "base", HeadSHA: "head", Branch: "feature", ChangedFiles: []string{"internal/a.go"}},
+		contextprofile.Profile{Files: []contextprofile.FileProfile{{Path: "internal/a.go", DiffBytes: 10}}},
+		knowledge.Context{},
+		nil,
+		"go test ./... exited 0",
+		githubcontext.Context{},
+		"## metareview PR Evidence\n\nvalidation",
+		"gate",
+	)
+
+	for _, required := range []string{
+		"## Review Manifest",
+		"Manifest verdict:",
+		"Runtime assessment: static-only; runtime not assessed",
+		"internal/a.go",
+	} {
+		if !strings.Contains(body, required) {
+			t.Fatalf("pr-ready context missing %q:\n%s", required, body)
+		}
+	}
+}
diff --git a/internal/taskdone/review.go b/internal/taskdone/review.go
index 25738cd..bb16862 100644
--- a/internal/taskdone/review.go
+++ b/internal/taskdone/review.go
@@ -14,6 +14,7 @@ import (
 	"github.com/dsifry/metareview/internal/markdown"
 	"github.com/dsifry/metareview/internal/repo"
 	"github.com/dsifry/metareview/internal/reviewers"
+	"github.com/dsifry/metareview/internal/reviewmanifest"
 	"github.com/dsifry/metareview/internal/runchain"
 	"github.com/dsifry/metareview/internal/state"
 	"github.com/dsifry/metareview/internal/tasksource"
@@ -388,12 +389,22 @@ func contextMarkdown(runID string, task tasksource.Source, git gitcontext.Contex
 		"- Gate effect: " + markdown.InlineCode(gateEffect) + "\n\n" +
 		contextprofile.Markdown(profile) + "\n\n" +
 		contextprofile.ShardPlanMarkdown(profile, contextprofile.ShardOptions{MaxBytesPerShard: contextprofile.DefaultMaxBytesPerShard, GroupBy: "path"}) + "\n\n" +
+		reviewManifestMarkdown("task-done", map[string]string{"type": taskTargetType(task), "id": task.ID}, profile) + "\n\n" +
 		"## Changed Files\n\n" + markdownList(changed, "No changed files.") + "\n\n" +
 		"## Diff\n\n" + markdown.FencedCodeBlock("diff", strings.Join([]string{git.Diff, git.StagedDiff, git.WorkingTreeDiff, git.UntrackedExcerpts}, "\n")) + "\n\n" +
 		"## Knowledge And Registries\n\n" + knowledgeMarkdown(knowledgeContext) + "\n\n" +
 		"## Evidence\n\n" + firstNonEmpty(evidenceText, "No external validation evidence supplied.") + "\n"
 }
 
+func reviewManifestMarkdown(scope string, target map[string]string, profile contextprofile.Profile) string {
+	plan, err := contextprofile.PlanShards(profile, contextprofile.ShardOptions{MaxBytesPerShard: contextprofile.DefaultMaxBytesPerShard, GroupBy: "path"})
+	if err != nil {
+		return "## Review Manifest\n\nUnable to generate review manifest: " + err.Error()
+	}
+	manifest := reviewmanifest.Build(reviewmanifest.Input{Scope: scope, Target: target, Profile: profile, ShardPlan: plan})
+	return reviewmanifest.Markdown(manifest, reviewmanifest.Aggregate(manifest))
+}
+
 func knowledgeMarkdown(context knowledge.Context) string {
 	service := "Service inventory: none\n\nNo service inventory found."
 	if context.ServiceInventoryPath != "" {
diff --git a/internal/taskdone/review_markdown_test.go b/internal/taskdone/review_markdown_test.go
index a037438..e1bd131 100644
--- a/internal/taskdone/review_markdown_test.go
+++ b/internal/taskdone/review_markdown_test.go
@@ -4,8 +4,12 @@ import (
 	"strings"
 	"testing"
 
+	"github.com/dsifry/metareview/internal/contextprofile"
 	"github.com/dsifry/metareview/internal/findings"
+	"github.com/dsifry/metareview/internal/gitcontext"
+	"github.com/dsifry/metareview/internal/knowledge"
 	"github.com/dsifry/metareview/internal/runchain"
+	"github.com/dsifry/metareview/internal/tasksource"
 )
 
 func TestReviewMarkdownSeparatesNonBlockingFindings(t *testing.T) {
@@ -49,3 +53,26 @@ func TestRunChainMarkdownIncludesEscalationDetails(t *testing.T) {
 		}
 	}
 }
+
+func TestContextMarkdownIncludesReviewManifest(t *testing.T) {
+	body := contextMarkdown(
+		"mrv-task",
+		tasksource.Source{ID: "task-1", Body: "Review manifest task"},
+		gitcontext.Context{BaseSHA: "base", HeadSHA: "head", Branch: "feature", ChangedFiles: []string{"internal/a.go"}},
+		contextprofile.Profile{Files: []contextprofile.FileProfile{{Path: "internal/a.go", DiffBytes: 10}}},
+		knowledge.Context{},
+		"go test ./... exited 0",
+		"gate",
+	)
+
+	for _, required := range []string{
+		"## Review Manifest",
+		"Manifest verdict:",
+		"Runtime assessment: static-only; runtime not assessed",
+		"internal/a.go",
+	} {
+		if !strings.Contains(body, required) {
+			t.Fatalf("task-done context missing %q:\n%s", required, body)
+		}
+	}
+}
--- docs/superpowers/plans/2026-07-05-issue-2-wu4-review-manifest-shard-aggregation.md
+# Issue #2 WU4: Review Manifest Shard Aggregation
+
+## Objective
+
+Add the first review manifest slice for Issue #2: a deterministic schema and aggregator that can represent shard review coverage, shard results, and the required cross-shard review for multi-shard changes.
+
+## Scope
+
+- Add an internal manifest package for review coverage and shard aggregation.
+- Reuse the existing `internal/contextprofile` shard plan shape from WU3.
+- Render a manifest summary into task-done and pr-ready context packs so agents can see the shard coverage contract during review.
+- Define the result-ingestion boundary as deterministic, versioned data passed to the manifest builder. In this slice, task-done and pr-ready will render an empty pending-result manifest; later CLI or skill work can populate shard results from stable files or evidence records using the same schema.
+- Keep agent execution out of the Go CLI. The CLI should model and aggregate externally produced shard results, not spawn Codex, Claude, or other runtimes.
+
+## Non-Goals
+
+- Do not implement a full shard runner.
+- Do not ingest GitHub review threads.
+- Do not add runtime/deployment evidence.
+- Do not replace existing task-done or pr-ready verdict logic in this slice. Aggregation produces a manifest verdict/status that is rendered as evidence; existing gates do not consume it as an additional blocker until a later integration slice.
+
+## Acceptance Criteria
+
+- A manifest records source paths, generated or out-of-scope path dispositions, the shard plan, shard review results, and an optional cross-shard result.
+- The canonical coverage universe accounts for every changed path exactly once: generated-filtered `contextprofile.Profile.Files` paths are primary source paths, and every generated-excluded or caller-marked out-of-scope changed path must appear as a disposition with a valid rationale.
+- Generated or out-of-scope path dispositions must include a rationale. A rationale is invalid when it is blank after trimming, shorter than 12 characters, or uses placeholder text such as `n/a`, `none`, `todo`, or `tbd`.
+- Aggregation blocks when a canonical source path is assigned to zero or multiple primary shards.
+- Aggregation blocks when any planned shard lacks a review result.
+- The manifest exposes a deterministic `sourceManifestHash` computed from schema version, source paths, path dispositions, shard IDs, shard paths, shard byte counts, and the underlying WU3 source diff hash. This hash is the freshness key for shard and cross-shard results.
+- Aggregation blocks when a shard result is stale for the current `sourceManifestHash`.
+- Aggregation blocks when a cross-shard result is stale for the current `sourceManifestHash` or does not cover the current shard set.
+- Shard and cross-shard results must use a versioned schema with canonical ordering and closed enums for verdict (`PASS`, `PASS_ADVISORY`, `NEEDS_REVISION`, `ESCALATED`), severity (`low`, `medium`, `high`, `critical`), and disposition (`fixed`, `waived`, `accepted-risk`, `false-positive`, `deferred`, `open`).
+- Shard and cross-shard results must carry evidence-backed provenance: result ID or path, reviewer/source, source manifest hash, covered paths, file:line evidence or acceptance/path coverage notes, severity, and disposition.
+- Aggregation blocks when any shard result or cross-shard result has blockers, a blocking verdict, missing provenance, missing coverage evidence, or unresolved medium-or-higher findings without an explicit disposition.
+- Aggregation requires a cross-shard review for multi-shard manifests.
+- Aggregation passes when every primary shard has a fresh passing result and the required cross-shard review passes.
+- Task-done and pr-ready context packs include a readable Review Manifest section whose manifest verdict is separate from the existing gate verdict.
+- The manifest summary explicitly says `Runtime assessment: static-only; runtime not assessed` so a passing manifest cannot imply live/deployed verification.
--- internal/reviewmanifest/manifest.go
+package reviewmanifest
+
+import (
+	"crypto/sha256"
+	"encoding/hex"
+	"fmt"
+	"sort"
+	"strings"
+
+	"github.com/dsifry/metareview/internal/contextprofile"
+)
+
+const (
+	SchemaVersion = 1
+
+	VerdictPass          = "PASS"
+	VerdictPassAdvisory  = "PASS_ADVISORY"
+	VerdictNeedsRevision = "NEEDS_REVISION"
+	VerdictEscalated     = "ESCALATED"
+
+	SeverityLow      = "low"
+	SeverityMedium   = "medium"
+	SeverityHigh     = "high"
+	SeverityCritical = "critical"
+
+	DispositionGenerated     = "generated"
+	DispositionOutOfScope    = "out-of-scope"
+	DispositionFixed         = "fixed"
+	DispositionWaived        = "waived"
+	DispositionAcceptedRisk  = "accepted-risk"
+	DispositionFalsePositive = "false-positive"
+	DispositionDeferred      = "deferred"
+	DispositionOpen          = "open"
+
+	CrossShardID = "cross-shard"
+)
+
+type Input struct {
+	Scope            string
+	Target           map[string]string
+	Profile          contextprofile.Profile
+	ShardPlan        contextprofile.ShardPlan
+	PathDispositions []PathDisposition
+	ShardResults     []ReviewResult
+	CrossShardResult *ReviewResult
+}
+
+type Manifest struct {
+	SchemaVersion          int
+	Scope                  string
+	Target                 map[string]string
+	SourcePaths            []string
+	GeneratedExcludedPaths []string
+	PathDispositions       []PathDisposition
+	ShardPlan              contextprofile.ShardPlan
+	ShardResults           []ReviewResult
+	CrossShardResult       *ReviewResult
+	SourceManifestHash     string
+	RuntimeAssessment      string
+}
+
+type PathDisposition struct {
+	Path        string
+	Disposition string
+	Rationale   string
+}
+
+type ReviewResult struct {
+	SchemaVersion      int
+	ID                 string
+	Path               string
+	ShardID            string
+	Verdict            string
+	SourceManifestHash string
+	Reviewer           string
+	CoveredPaths       []string
+	CoveredShardIDs    []string
+	Evidence           []EvidenceRef
+	Findings           []ResultFinding
+	BlockingCount      int
+}
+
+type EvidenceRef struct {
+	Path string
+	Line int
+	Note string
+}
+
+type ResultFinding struct {
+	Severity    string
+	Disposition string
+	Evidence    []EvidenceRef
+}
+
+type AggregateResult struct {
+	Verdict  string
+	Blockers []string
+}
+
+func Build(input Input) Manifest {
+	manifest := Manifest{
+		SchemaVersion:          SchemaVersion,
+		Scope:                  strings.TrimSpace(input.Scope),
+		Target:                 copyStringMap(input.Target),
+		SourcePaths:            sourcePaths(input.Profile),
+		GeneratedExcludedPaths: cleanSortedUnique(input.Profile.GeneratedExcludedFiles),
+		PathDispositions:       canonicalPathDispositions(input.PathDispositions),
+		ShardPlan:              canonicalShardPlan(input.ShardPlan),
+		ShardResults:           canonicalReviewResults(input.ShardResults),
+		RuntimeAssessment:      "static-only; runtime not assessed",
+	}
+	if input.CrossShardResult != nil {
+		cross := *input.CrossShardResult
+		manifest.CrossShardResult = &cross
+	}
+	manifest.SourceManifestHash = sourceManifestHash(manifest)
+	return manifest
+}
+
+func Aggregate(manifest Manifest) AggregateResult {
+	var blockers []string
+	blockers = append(blockers, pathDispositionBlockers(manifest)...)
+	blockers = append(blockers, sourceAssignmentBlockers(manifest)...)
+	blockers = append(blockers, shardResultBlockers(manifest)...)
+	blockers = append(blockers, crossShardBlockers(manifest)...)
+	verdict := VerdictPass
+	if len(blockers) > 0 {
+		verdict = VerdictNeedsRevision
+	}
+	sort.Strings(blockers)
+	return AggregateResult{Verdict: verdict, Blockers: blockers}
+}
+
+func Markdown(manifest Manifest, aggregate AggregateResult) string {
+	lines := []string{
+		"## Review Manifest",
+		"",
+		"- Manifest verdict: `" + firstNonEmpty(aggregate.Verdict, VerdictPass) + "`",
+		"- Source manifest hash: `" + manifest.SourceManifestHash + "`",
+		"- Runtime assessment: " + firstNonEmpty(manifest.RuntimeAssessment, "static-only; runtime not assessed"),
+		"",
+		"### Source Paths",
+	}
+	lines = append(lines, markdownList(manifest.SourcePaths, "No source paths recorded.")..
--- internal/reviewmanifest/manifest_test.go
+package reviewmanifest
+
+import (
+	"strings"
+	"testing"
+
+	"github.com/dsifry/metareview/internal/contextprofile"
+)
+
+func TestAggregateRequiresCompletePathAccounting(t *testing.T) {
+	profile := contextprofile.Profile{
+		Files: []contextprofile.FileProfile{
+			{Path: "internal/reviewmanifest/manifest.go", DiffBytes: 100},
+		},
+		GeneratedExcludedFiles: []string{"docs/metareview/context/run-context.md"},
+	}
+	plan := singleShardPlan("source-hash", "internal/reviewmanifest/manifest.go")
+
+	manifest := Build(Input{Scope: "task-done", Target: map[string]string{"type": "task", "id": "wu4"}, Profile: profile, ShardPlan: plan})
+	aggregate := Aggregate(manifest)
+
+	assertBlocker(t, aggregate, "missing disposition for docs/metareview/context/run-context.md")
+
+	manifest = Build(Input{
+		Scope:     "task-done",
+		Target:    map[string]string{"type": "task", "id": "wu4"},
+		Profile:   profile,
+		ShardPlan: plan,
+		PathDispositions: []PathDisposition{{
+			Path:        "docs/metareview/context/run-context.md",
+			Disposition: DispositionGenerated,
+			Rationale:   "metareview generated context artifact",
+		}},
+	})
+	aggregate = Aggregate(manifest)
+
+	assertNoBlocker(t, aggregate, "missing disposition")
+
+	manifest.PathDispositions[0].Rationale = "tbd"
+	aggregate = Aggregate(manifest)
+
+	assertBlocker(t, aggregate, "invalid disposition rationale")
+}
+
+func TestAggregateBlocksZeroAndDuplicatePrimaryShardAssignments(t *testing.T) {
+	profile := contextprofile.Profile{
+		Files: []contextprofile.FileProfile{
+			{Path: "internal/a.go", DiffBytes: 50},
+			{Path: "internal/b.go", DiffBytes: 50},
+		},
+	}
+	plan := contextprofile.ShardPlan{
+		SourceDiffHash: "source-hash",
+		Shards: []contextprofile.Shard{
+			{ID: "shard-01", SourceDiffHash: "source-hash", Paths: []string{"internal/a.go"}, ByteCount: 50},
+			{ID: "shard-02", SourceDiffHash: "source-hash", Paths: []string{"internal/a.go"}, ByteCount: 50},
+		},
+	}
+
+	aggregate := Aggregate(Build(Input{Profile: profile, ShardPlan: plan}))
+
+	assertBlocker(t, aggregate, "internal/a.go assigned to multiple primary shards")
+	assertBlocker(t, aggregate, "internal/b.go is not assigned to a primary shard")
+}
+
+func TestSourceManifestHashIsStableAndSensitive(t *testing.T) {
+	profile := contextprofile.Profile{
+		Files: []contextprofile.FileProfile{
+			{Path: "internal/b.go", DiffBytes: 20},
+			{Path: "internal/a.go", DiffBytes: 10},
+		},
+		GeneratedExcludedFiles: []string{"docs/metareview/context/a.md"},
+	}
+	plan := contextprofile.ShardPlan{
+		SourceDiffHash: "source-hash",
+		Shards: []contextprofile.Shard{
+			{ID: "shard-02", SourceDiffHash: "source-hash", Paths: []string{"internal/b.go"}, ByteCount: 20},
+			{ID: "shard-01", SourceDiffHash: "source-hash", Paths: []string{"internal/a.go"}, ByteCount: 10},
+		},
+	}
+	dispositions := []PathDisposition{{
+		Path:        "docs/metareview/context/a.md",
+		Disposition: DispositionGenerated,
+		Rationale:   "metareview generated context artifact",
+	}}
+
+	first := Build(Input{Profile: profile, ShardPlan: plan, PathDispositions: dispositions})
+	second := Build(Input{Profile: profile, ShardPlan: plan, PathDispositions: dispositions})
+	if first.SourceManifestHash == "" || first.SourceManifestHash != second.SourceManifestHash {
+		t.Fatalf("source manifest hash should be stable: first=%q second=%q", first.SourceManifestHash, second.SourceManifestHash)
+	}
+
+	plan.Shards[0].Paths = []string{"internal/b.go", "internal/c.go"}
+	changed := Build(Input{Profile: profile, ShardPlan: plan, PathDispositions: dispositions})
+	if changed.SourceManifestHash == first.SourceManifestHash {
+		t.Fatalf("source manifest hash should change when shard paths change: %q", first.SourceManifestHash)
+	}
+}
+
+func TestAggregateRequiresFreshEvidenceBackedShardResults(t *testing.T) {
+	profile := contextprofile.Profile{Files: []contextprofile.FileProfile{{Path: "internal/a.go", DiffBytes: 10}}}
+	plan := singleShardPlan("source-hash", "internal/a.go")
+	manifest := Build(Input{Profile: profile, ShardPlan: plan})
+
+	aggregate := Aggr
```

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

go test ./... exited 0
bash tests/run-all.sh exited 0

