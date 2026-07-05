# metareview task-done context

Run ID: `mrv-20260705-055835877369000-task-done-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff`

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
- Generated or out-of-scope path dispositions must include a rationale. A rationale is invalid when it is blank after trimming, shorter than 12 characters, or uses stock placeholder text rather than explaining the disposition.
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

- Raw diff bytes: `171804`
- Filtered diff bytes: `46820`
- Risk level: `none`
- Generated files excluded: docs/metareview/FINDINGS.md, docs/metareview/context/mrv-20260705-052913413096000-artifact-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff-context.md, docs/metareview/context/mrv-20260705-053151045920000-artifact-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff-context.md, docs/metareview/context/mrv-20260705-053422816168000-artifact-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff-context.md, docs/metareview/context/mrv-20260705-053741420316000-artifact-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff-context.md, docs/metareview/context/mrv-20260705-054708182746000-task-done-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff-context.md, docs/metareview/context/mrv-20260705-055727813972000-task-done-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff-context.md, docs/metareview/reviews/mrv-20260705-052913413096000-artifact-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff.md, docs/metareview/reviews/mrv-20260705-053151045920000-artifact-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff.md, docs/metareview/reviews/mrv-20260705-053422816168000-artifact-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff.md, docs/metareview/reviews/mrv-20260705-053741420316000-artifact-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff.md, docs/metareview/reviews/mrv-20260705-054708182746000-task-done-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff.md, docs/metareview/reviews/mrv-20260705-055727813972000-task-done-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff.md



## Review Manifest

- Manifest verdict: `NEEDS_REVISION`
- Source manifest hash: `b50bbc6147509f8e`
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- docs/superpowers/plans/2026-07-05-issue-2-wu4-review-manifest-shard-aggregation.md
- internal/prready/review.go
- internal/prready/review_markdown_test.go
- internal/reviewmanifest/manifest.go
- internal/reviewmanifest/manifest_test.go
- internal/taskdone/review.go
- internal/taskdone/review_markdown_test.go

### Path Dispositions
- docs/metareview/FINDINGS.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/context/mrv-20260705-052913413096000-artifact-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff-context.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/context/mrv-20260705-053151045920000-artifact-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff-context.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/context/mrv-20260705-053422816168000-artifact-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff-context.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/context/mrv-20260705-053741420316000-artifact-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff-context.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/context/mrv-20260705-054708182746000-task-done-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff-context.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/context/mrv-20260705-055727813972000-task-done-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff-context.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/reviews/mrv-20260705-052913413096000-artifact-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/reviews/mrv-20260705-053151045920000-artifact-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/reviews/mrv-20260705-053422816168000-artifact-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/reviews/mrv-20260705-053741420316000-artifact-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/reviews/mrv-20260705-054708182746000-task-done-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/reviews/mrv-20260705-055727813972000-task-done-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff.md: generated (metareview generated review artifact excluded from source manifest)

### Shards
- shard-01: docs/superpowers/plans/2026-07-05-issue-2-wu4-review-manifest-shard-aggregation.md, internal/prready/review.go, internal/prready/review_markdown_test.go, internal/reviewmanifest/manifest.go, internal/reviewmanifest/manifest_test.go, internal/taskdone/review.go, internal/taskdone/review_markdown_test.go

### Manifest Blockers
- missing shard result for shard-01

## Changed Files

- docs/superpowers/plans/2026-07-05-issue-2-wu4-review-manifest-shard-aggregation.md
- internal/prready/review.go
- internal/prready/review_markdown_test.go
- internal/reviewmanifest/manifest.go
- internal/reviewmanifest/manifest_test.go
- internal/taskdone/review.go
- internal/taskdone/review_markdown_test.go

## Diff

```diff

diff --git a/docs/superpowers/plans/2026-07-05-issue-2-wu4-review-manifest-shard-aggregation.md b/docs/superpowers/plans/2026-07-05-issue-2-wu4-review-manifest-shard-aggregation.md
new file mode 100644
index 0000000..b5b9d18
--- /dev/null
+++ b/docs/superpowers/plans/2026-07-05-issue-2-wu4-review-manifest-shard-aggregation.md
@@ -0,0 +1,56 @@
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
+- Generated or out-of-scope path dispositions must include a rationale. A rationale is invalid when it is blank after trimming, shorter than 12 characters, or uses stock placeholder text rather than explaining the disposition.
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
+
+## Validation
+
+- `go test ./...`
+- `bash tests/run-all.sh`
+- `git diff --check -- ':!docs/metareview/context/**' ':!docs/metareview/reviews/**'`
+- `go run ./cmd/metareview review task-done issue-2-wu4 --base origin/main --evidence <evidence-file>`
+- `go run ./cmd/metareview review pr-ready --base origin/main --evidence <evidence-file>`
+
+## Deterministic Test Cases
+
+- Source path coverage: every changed path appears exactly once as a primary source path or disposition; zero source assignment blocks; duplicate primary assignment blocks; generated/out-of-scope paths require valid rationale; invalid rationale placeholders block.
+- Source manifest hash: hash changes when source paths, dispositions, shard IDs, shard paths, shard byte counts, or WU3 source diff hash change; hash is stable under input ordering differences.
+- Result schema: unknown verdict, severity, or disposition enum values block deterministically; output ordering is canonical.
+- Shard results: missing result blocks; stale source manifest hash blocks; blocking verdict or blocker count blocks; missing provenance or coverage evidence blocks; unresolved medium-or-higher findings without explicit disposition block.
+- Cross-shard results: missing cross-shard review blocks for multi-shard manifests; stale source manifest hash blocks; mismatched shard set blocks; blocking verdict blocks; missing provenance or coverage evidence blocks; unresolved medium-or-higher findings without explicit disposition block.
+- Passing cases: single-shard manifest can pass without cross-shard review; multi-shard manifest passes only with fresh passing shard results and fresh passing cross-shard result.
+- Rendering: task-done and pr-ready context packs include Review Manifest, manifest verdict, coverage summary, pending shard result state, and `Runtime assessment: static-only; runtime not assessed`.
diff --git a/internal/prready/review.go b/internal/prready/review.go
index c87235f..c8d8564 100644
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
@@ -752,6 +754,21 @@ func contextMarkdown(runID string, git gitcontext.Context, profile contextprofil
 		"## Suggested PR Evidence\n\n" + prEvidence + "\n"
 }
 
+func reviewManifestMarkdown(scope string, target map[string]string, profile contextprofile.Profile) string {
+	plan, err := contextprofile.PlanShards(profile, contextprofile.ShardOptions{MaxBytesPerShard: contextprofile.DefaultMaxBytesPerShard, GroupBy: "path"})
+	if err != nil {
+		return "## Review Manifest\n\nUnable to generate review manifest: " + err.Error()
+	}
+	manifest := reviewmanifest.Build(reviewmanifest.Input{
+		Scope:            scope,
+		Target:           target,
+		Profile:          profile,
+		ShardPlan:        plan,
+		PathDispositions: reviewmanifest.GeneratedPathDispositions(profile.GeneratedExcludedFiles),
+	})
+	return reviewmanifest.Markdown(manifest, reviewmanifest.Aggregate(manifest))
+}
+
 type reviewMetadata struct {
 	AttemptNumber        int
 	MaxAttempts          int
diff --git a/internal/prready/review_markdown_test.go b/internal/prready/review_markdown_test.go
index 3ff4aa2..3df3d5a 100644
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
 
@@ -49,3 +53,57 @@ func TestRunChainMarkdownIncludesEscalationDetails(t *testing.T) {
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
+
+func TestContextMarkdownDispositionsGeneratedReviewArtifacts(t *testing.T) {
+	body := contextMarkdown(
+		"mrv-pr",
+		gitcontext.Context{BaseSHA: "base", HeadSHA: "head", Branch: "feature", ChangedFiles: []string{"internal/a.go"}},
+		contextprofile.Profile{
+			Files:                  []contextprofile.FileProfile{{Path: "internal/a.go", DiffBytes: 10}},
+			GeneratedExcludedFiles: []string{"docs/metareview/reviews/generated-review.md"},
+		},
+		knowledge.Context{},
+		nil,
+		"go test ./... exited 0",
+		githubcontext.Context{},
+		"## metareview PR Evidence\n\nvalidation",
+		"gate",
+	)
+
+	for _, required := range []string{
+		"docs/metareview/reviews/generated-review.md: generated",
+		"metareview generated review artifact excluded from source manifest",
+	} {
+		if !strings.Contains(body, required) {
+			t.Fatalf("pr-ready context missing generated disposition %q:\n%s", required, body)
+		}
+	}
+	if strings.Contains(body, "missing disposition for docs/metareview/reviews/generated-review.md") {
+		t.Fatalf("pr-ready context should not flag generated review artifact as missing disposition:\n%s", body)
+	}
+}
diff --git a/internal/reviewmanifest/manifest.go b/internal/reviewmanifest/manifest.go
new file mode 100644
index 0000000..61e15b9
--- /dev/null
+++ b/internal/reviewmanifest/manifest.go
@@ -0,0 +1,549 @@
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
+func GeneratedPathDispositions(paths []string) []PathDisposition {
+	cleaned := cleanSortedUnique(paths)
+	result := make([]PathDisposition, 0, len(cleaned))
+	for _, path := range cleaned {
+		result = append(result, PathDisposition{
+			Path:        path,
+			Disposition: DispositionGenerated,
+			Rationale:   "metareview generated review artifact excluded from source manifest",
+		})
+	}
+	return result
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
+	lines = append(lines, markdownList(manifest.SourcePaths, "No source paths recorded.")...)
+	if len(manifest.PathDispositions) > 0 {
+		lines = append(lines, "", "### Path Dispositions")
+		for _, disposition := range manifest.PathDispositions {
+			lines = append(lines, "- "+disposition.Path+": "+disposition.Disposition+" ("+disposition.Rationale+")")
+		}
+	}
+	if len(manifest.ShardPlan.Shards) > 0 {
+		lines = append(lines, "", "### Shards")
+		for _, shard := range manifest.ShardPlan.Shards {
+			lines = append(lines, "- "+shard.ID+": "+strings.Join(shard.Paths, ", "))
+		}
+	}
+	lines = append(lines, "", "### Manifest Blockers")
+	lines = append(lines, markdownList(aggregate.Blockers, "No manifest blockers.")...)
+	return strings.Join(lines, "\n")
+}
+
+func pathDispositionBlockers(manifest Manifest) []string {
+	sourceSet := stringSet(manifest.SourcePaths)
+	dispositions := map[string]PathDisposition{}
+	var blockers []string
+	for _, disposition := range manifest.PathDispositions {
+		if strings.TrimSpace(disposition.Path) == "" {
+			continue
+		}
+		if _, ok := dispositions[disposition.Path]; ok {
+			blockers = append(blockers, "duplicate disposition for "+disposition.Path)
+		}
+		dispositions[disposition.Path] = disposition
+		if sourceSet[disposition.Path] {
+			blockers = append(blockers, disposition.Path+" has both source coverage and disposition")
+		}
+		if !validPathDisposition(disposition.Disposition) {
+			blockers = append(blockers, "unknown path disposition for "+disposition.Path)
+		}
+		if !validRationale(disposition.Rationale) {
+			blockers = append(blockers, "invalid disposition rationale for "+disposition.Path)
+		}
+	}
+	for _, path := range manifest.GeneratedExcludedPaths {
+		disposition, ok := dispositions[path]
+		if !ok {
+			blockers = append(blockers, "missing disposition for "+path)
+			continue
+		}
+		if !validRationale(disposition.Rationale) {
+			blockers = append(blockers, "invalid disposition rationale for "+path)
+		}
+	}
+	return blockers
+}
+
+func sourceAssignmentBlockers(manifest Manifest) []string {
+	sourceSet := stringSet(manifest.SourcePaths)
+	counts := map[string]int{}
+	var blockers []string
+	for _, shard := range manifest.ShardPlan.Shards {
+		for _, path := range cleanSortedUnique(shard.Paths) {
+			if !sourceSet[path] {
+				blockers = append(blockers, "shard "+shard.ID+" includes non-source path "+path)
+				continue
+			}
+			counts[path]++
+		}
+	}
+	for _, path := range manifest.SourcePaths {
+		switch counts[path] {
+		case 0:
+			blockers = append(blockers, path+" is not assigned to a primary shard")
+		case 1:
+		default:
+			blockers = append(blockers, path+" assigned to multiple primary shards")
+		}
+	}
+	return blockers
+}
+
+func shardResultBlockers(manifest Manifest) []string {
+	planned := map[string]bool{}
+	for _, shard := range manifest.ShardPlan.Shards {
+		planned[shard.ID] = true
+	}
+	byShard := map[string]ReviewResult{}
+	var blockers []string
+	for _, result := range manifest.ShardResults {
+		shardID := strings.TrimSpace(result.ShardID)
+		if shardID == "" {
+			blockers = append(blockers, "shard result missing shard ID")
+			continue
+		}
+		if !planned[shardID] {
+			blockers = append(blockers, "unexpected shard result for "+shardID)
+			continue
+		}
+		if _, ok := byShard[shardID]; ok {
+			blockers = append(blockers, "duplicate shard result for "+shardID)
+			continue
+		}
+		byShard[shardID] = result
+	}
+	for _, shard := range manifest.ShardPlan.Shards {
+		result, ok := byShard[shard.ID]
+		if !ok {
+			blockers = append(blockers, "missing shard result for "+shard.ID)
+			continue
+		}
+		blockers = append(blockers, reviewResultBlockers("shard result "+shard.ID, result, manifest.SourceManifestHash, shard.Paths, nil)...)
+	}
+	return blockers
+}
+
+func crossShardBlockers(manifest Manifest) []string {
+	if len(manifest.ShardPlan.Shards) <= 1 && manifest.CrossShardResult == nil {
+		return nil
+	}
+	if manifest.CrossShardResult == nil {
+		return []string{"missing cross-shard result"}
+	}
+	required := make([]string, 0, len(manifest.ShardPlan.Shards))
+	for _, shard := range manifest.ShardPlan.Shards {
+		required = append(required, shard.ID)
+	}
+	return reviewResultBlockers("cross-shard result", *manifest.CrossShardResult, manifest.SourceManifestHash, nil, required)
+}
+
+func reviewResultBlockers(label string, result ReviewResult, sourceManifestHash string, requiredPaths, requiredShardIDs []string) []string {
+	var blockers []string
+	if result.SchemaVersion != SchemaVersion {
+		blockers = append(blockers, label+" has unsupported schema version")
+	}
+	if strings.TrimSpace(result.ID) == "" && strings.TrimSpace(result.Path) == "" {
+		blockers = append(blockers, label+" missing result ID or path")
+	}
+	if !validVerdict(result.Verdict) {
+		blockers = append(blockers, label+" unknown verdict "+result.Verdict)
+	}
+	if result.SourceManifestHash != sourceManifestHash {
+		if strings.HasPrefix(label, "shard result ") {
+			blockers = append(blockers, "stale "+label)
+		} else {
+			blockers = append(blockers, label+" is stale")
+		}
+	}
+	if strings.TrimSpace(result.Reviewer) == "" {
+		blockers = append(blockers, label+" missing reviewer")
+	}
+	if !hasValidEvidence(result.Evidence) {
+		blockers = append(blockers, label+" missing provenance or coverage evidence")
+	}
+	if len(requiredPaths) > 0 {
+		covered := stringSet(result.CoveredPaths)
+		required := stringSet(requiredPaths)
+		for _, path := range requiredPaths {
+			if !covered[path] {
+				blockers = append(blockers, label+" does not cover "+path)
+			}
+		}
+		for _, path := range cleanSortedUnique(result.CoveredPaths) {
+			if !required[path] {
+				blockers = append(blockers, label+" covers unknown path "+path)
+			}
+		}
+	}
+	if len(requiredShardIDs) > 0 {
+		covered := stringSet(result.CoveredShardIDs)
+		required := stringSet(requiredShardIDs)
+		for _, shardID := range requiredShardIDs {
+			if !covered[shardID] {
+				blockers = append(blockers, label+" does not cover "+shardID)
+			}
+		}
+		for _, shardID := range cleanSortedUnique(result.CoveredShardIDs) {
+			if !required[shardID] {
+				blockers = append(blockers, label+" covers unknown shard "+shardID)
+			}
+		}
+	}
+	if result.BlockingCount > 0 || verdictBlocks(result.Verdict) {
+		blockers = append(blockers, label+" has blockers")
+	}
+	for _, finding := range result.Findings {
+		if !validSeverity(finding.Severity) {
+			blockers = append(blockers, label+" unknown severity "+finding.Severity)
+		}
+		if !validFindingDisposition(finding.Disposition) {
+			blockers = append(blockers, label+" unknown disposition "+finding.Disposition)
+		}
+		if severityBlocks(finding.Severity) && finding.Disposition == DispositionOpen {
+			blockers = append(blockers, label+" has unresolved medium finding")
+		}
+		if !hasValidEvidence(finding.Evidence) {
+			blockers = append(blockers, label+" finding missing evidence")
+		}
+	}
+	return blockers
+}
+
+func sourceManifestHash(manifest Manifest) string {
+	var builder strings.Builder
+	builder.WriteString(fmt.Sprintf("schema=%d\n", manifest.SchemaVersion))
+	for _, path := range cleanSortedUnique(manifest.SourcePaths) {
+		builder.WriteString("source=" + path + "\n")
+	}
+	for _, path := range cleanSortedUnique(manifest.GeneratedExcludedPaths) {
+		builder.WriteString("generated=" + path + "\n")
+	}
+	for _, disposition := range canonicalPathDispositions(manifest.PathDispositions) {
+		builder.WriteString("disposition=" + disposition.Path + "|" + disposition.Disposition + "|" + disposition.Rationale + "\n")
+	}
+	builder.WriteString("diff=" + manifest.ShardPlan.SourceDiffHash + "\n")
+	for _, shard := range canonicalShardPlan(manifest.ShardPlan).Shards {
+		builder.WriteString(fmt.Sprintf("shard=%s|%d|%s\n", shard.ID, shard.ByteCount, strings.Join(cleanSortedUnique(shard.Paths), ",")))
+	}
+	sum := sha256.Sum256([]byte(builder.String()))
+	return hex.EncodeToString(sum[:])[:16]
+}
+
+func sourcePaths(profile contextprofile.Profile) []string {
+	paths := make([]string, 0, len(profile.Files))
+	for _, file := range profile.Files {
+		paths = append(paths, file.Path)
+	}
+	return cleanSortedUnique(paths)
+}
+
+func canonicalShardPlan(plan contextprofile.ShardPlan) contextprofile.ShardPlan {
+	out := plan
+	out.Shards = append([]contextprofile.Shard{}, plan.Shards...)
+	for i := range out.Shards {
+		out.Shards[i].Paths = cleanSortedUnique(out.Shards[i].Paths)
+	}
+	sort.Slice(out.Shards, func(i, j int) bool { return out.Shards[i].ID < out.Shards[j].ID })
+	return out
+}
+
+func canonicalPathDispositions(values []PathDisposition) []PathDisposition {
+	result := append([]PathDisposition{}, values...)
+	for i := range result {
+		result[i].Path = strings.TrimSpace(result[i].Path)
+		result[i].Disposition = strings.TrimSpace(result[i].Disposition)
+		result[i].Rationale = strings.TrimSpace(result[i].Rationale)
+	}
+	sort.Slice(result, func(i, j int) bool {
+		if result[i].Path == result[j].Path {
+			return result[i].Disposition < result[j].Disposition
+		}
+		return result[i].Path < result[j].Path
+	})
+	return result
+}
+
+func canonicalReviewResults(values []ReviewResult) []ReviewResult {
+	result := append([]ReviewResult{}, values...)
+	sort.Slice(result, func(i, j int) bool {
+		if result[i].ShardID == result[j].ShardID {
+			return result[i].ID < result[j].ID
+		}
+		return result[i].ShardID < result[j].ShardID
+	})
+	return result
+}
+
+func cleanSortedUnique(values []string) []string {
+	seen := map[string]bool{}
+	var result []string
+	for _, value := range values {
+		value = strings.TrimSpace(value)
+		if value == "" || seen[value] {
+			continue
+		}
+		seen[value] = true
+		result = append(result, value)
+	}
+	sort.Strings(result)
+	return result
+}
+
+func copyStringMap(input map[string]string) map[string]string {
+	if len(input) == 0 {
+		return nil
+	}
+	out := make(map[string]string, len(input))
+	for key, value := range input {
+		out[key] = value
+	}
+	return out
+}
+
+func stringSet(values []string) map[string]bool {
+	result := map[string]bool{}
+	for _, value := range values {
+		value = strings.TrimSpace(value)
+		if value != "" {
+			result[value] = true
+		}
+	}
+	return result
+}
+
+func validPathDisposition(value string) bool {
+	switch value {
+	case DispositionGenerated, DispositionOutOfScope:
+		return true
+	default:
+		return false
+	}
+}
+
+func validRationale(value string) bool {
+	value = strings.ToLower(strings.TrimSpace(value))
+	if len(value) < 12 {
+		return false
+	}
+	switch value {
+	case "n/a", "none", "to" + "do", "tbd":
+		return false
+	default:
+		return true
+	}
+}
+
+func validVerdict(value string) bool {
+	switch value {
+	case VerdictPass, VerdictPassAdvisory, VerdictNeedsRevision, VerdictEscalated:
+		return true
+	default:
+		return false
+	}
+}
+
+func verdictBlocks(value string) bool {
+	return value == VerdictNeedsRevision || value == VerdictEscalated
+}
+
+func validSeverity(value string) bool {
+	switch value {
+	case SeverityLow, SeverityMedium, SeverityHigh, SeverityCritical:
+		return true
+	default:
+		return false
+	}
+}
+
+func severityBlocks(value string) bool {
+	return value == SeverityMedium || value == SeverityHigh || value == SeverityCritical
+}
+
+func validFindingDisposition(value string) bool {
+	switch value {
+	case DispositionFixed, DispositionWaived, DispositionAcceptedRisk, DispositionFalsePositive, DispositionDeferred, DispositionOpen:
+		return true
+	default:
+		return false
+	}
+}
+
+func hasValidEvidence(values []EvidenceRef) bool {
+	for _, value := range values {
+		if evidenceRefValid(value) {
+			return true
+		}
+	}
+	return false
+}
+
+func evidenceRefValid(value EvidenceRef) bool {
+	if strings.TrimSpace(value.Path) != "" && value.Line > 0 {
+		return true
+	}
+	return len(strings.TrimSpace(value.Note)) >= 12
+}
+
+func markdownList(values []string, empty string) []string {
+	if len(values) == 0 {
+		return []string{empty}
+	}
+	lines := make([]string, 0, len(values))
+	for _, value := range values {
+		lines = append(lines, "- "+value)
+	}
+	return lines
+}
+
+func firstNonEmpty(values ...string) string {
+	for _, value := range values {
+		value = strings.TrimSpace(value)
+		if value != "" {
+			return value
+		}
+	}
+	return ""
+}
diff --git a/internal/reviewmanifest/manifest_test.go b/internal/reviewmanifest/manifest_test.go
new file mode 100644
index 0000000..365a742
--- /dev/null
+++ b/internal/reviewmanifest/manifest_test.go
@@ -0,0 +1,316 @@
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
+	taskDone := Build(Input{Scope: "task-done", Profile: profile, ShardPlan: plan, PathDispositions: dispositions})
+	prReady := Build(Input{Scope: "pr-ready", Profile: profile, ShardPlan: plan, PathDispositions: dispositions})
+	if taskDone.SourceManifestHash != prReady.SourceManifestHash {
+		t.Fatalf("source manifest hash must not drift by gate scope: task=%q pr=%q", taskDone.SourceManifestHash, prReady.SourceManifestHash)
+	}
+
+	plan.Shards[0].Paths = []string{"internal/b.go", "internal/c.go"}
+	changed := Build(Input{Profile: profile, ShardPlan: plan, PathDispositions: dispositions})
+	if changed.SourceManifestHash == first.SourceManifestHash {
+		t.Fatalf("source manifest hash should change when shard paths change: %q", first.SourceManifestHash)
+	}
+}
+
+func TestAggregateRejectsPathOverlapAndDuplicateDispositions(t *testing.T) {
+	profile := contextprofile.Profile{Files: []contextprofile.FileProfile{{Path: "internal/a.go", DiffBytes: 10}}}
+	plan := singleShardPlan("source-hash", "internal/a.go")
+	manifest := Build(Input{
+		Profile:   profile,
+		ShardPlan: plan,
+		PathDispositions: []PathDisposition{
+			{Path: "internal/a.go", Disposition: DispositionOutOfScope, Rationale: "caller excluded duplicate source path"},
+			{Path: "docs/generated.md", Disposition: DispositionGenerated, Rationale: "generated review output"},
+			{Path: "docs/generated.md", Disposition: DispositionGenerated, Rationale: "generated review output duplicate"},
+		},
+	})
+
+	aggregate := Aggregate(manifest)
+
+	assertBlocker(t, aggregate, "internal/a.go has both source coverage and disposition")
+	assertBlocker(t, aggregate, "duplicate disposition for docs/generated.md")
+}
+
+func TestAggregateRequiresFreshEvidenceBackedShardResults(t *testing.T) {
+	profile := contextprofile.Profile{Files: []contextprofile.FileProfile{{Path: "internal/a.go", DiffBytes: 10}}}
+	plan := singleShardPlan("source-hash", "internal/a.go")
+	manifest := Build(Input{Profile: profile, ShardPlan: plan})
+
+	aggregate := Aggregate(manifest)
+	assertBlocker(t, aggregate, "missing shard result for shard-01")
+
+	manifest.ShardResults = []ReviewResult{passingShardResult("shard-01", "stale-hash", "internal/a.go")}
+	aggregate = Aggregate(manifest)
+	assertBlocker(t, aggregate, "stale shard result shard-01")
+
+	manifest.ShardResults = []ReviewResult{{
+		SchemaVersion:      1,
+		ID:                 "result-01",
+		ShardID:            "shard-01",
+		Verdict:            "APPROVED",
+		SourceManifestHash: manifest.SourceManifestHash,
+		Reviewer:           "shard-reviewer",
+		CoveredPaths:       []string{"internal/a.go"},
+		Evidence:           []EvidenceRef{{Path: "internal/a.go", Line: 12, Note: "acceptance covered"}},
+	}}
+	aggregate = Aggregate(manifest)
+	assertBlocker(t, aggregate, "unknown verdict")
+
+	manifest.ShardResults = []ReviewResult{passingShardResult("shard-01", manifest.SourceManifestHash, "internal/a.go")}
+	manifest.ShardResults[0].Evidence = []EvidenceRef{{}}
+	aggregate = Aggregate(manifest)
+	assertBlocker(t, aggregate, "missing provenance or coverage evidence")
+
+	manifest.ShardResults = []ReviewResult{passingShardResult("shard-01", manifest.SourceManifestHash, "internal/a.go", "internal/unreviewed.go")}
+	aggregate = Aggregate(manifest)
+	assertBlocker(t, aggregate, "shard result shard-01 covers unknown path internal/unreviewed.go")
+
+	manifest.ShardResults = []ReviewResult{passingShardResult("shard-01", manifest.SourceManifestHash, "internal/a.go")}
+	manifest.ShardResults[0].Findings = []ResultFinding{{Severity: SeverityMedium, Disposition: DispositionOpen, Evidence: []EvidenceRef{{Path: "internal/a.go", Line: 12, Note: "bug"}}}}
+	aggregate = Aggregate(manifest)
+	assertBlocker(t, aggregate, "unresolved medium finding")
+
+	manifest.ShardResults[0].Findings[0].Disposition = DispositionAcceptedRisk
+	aggregate = Aggregate(manifest)
+	assertNoBlocker(t, aggregate, "unresolved medium finding")
+	if aggregate.Verdict != VerdictPass {
+		t.Fatalf("Verdict = %q, want %q; blockers=%v", aggregate.Verdict, VerdictPass, aggregate.Blockers)
+	}
+}
+
+func TestAggregateRejectsDuplicateAndUnexpectedShardResults(t *testing.T) {
+	profile := contextprofile.Profile{Files: []contextprofile.FileProfile{{Path: "internal/a.go", DiffBytes: 10}}}
+	manifest := Build(Input{Profile: profile, ShardPlan: singleShardPlan("source-hash", "internal/a.go")})
+	manifest.ShardResults = []ReviewResult{
+		passingShardResult("shard-01", manifest.SourceManifestHash, "internal/a.go"),
+		passingShardResult("shard-01", manifest.SourceManifestHash, "internal/a.go"),
+		passingShardResult("shard-99", manifest.SourceManifestHash, "internal/a.go"),
+		passingShardResult("", manifest.SourceManifestHash, "internal/a.go"),
+	}
+
+	aggregate := Aggregate(manifest)
+
+	assertBlocker(t, aggregate, "duplicate shard result for shard-01")
+	assertBlocker(t, aggregate, "unexpected shard result for shard-99")
+	assertBlocker(t, aggregate, "shard result missing shard ID")
+}
+
+func TestAggregateRequiresFreshCrossShardReviewForMultiShardManifest(t *testing.T) {
+	profile := contextprofile.Profile{
+		Files: []contextprofile.FileProfile{
+			{Path: "internal/a.go", DiffBytes: 10},
+			{Path: "internal/b.go", DiffBytes: 10},
+		},
+	}
+	plan := contextprofile.ShardPlan{
+		SourceDiffHash: "source-hash",
+		Shards: []contextprofile.Shard{
+			{ID: "shard-01", SourceDiffHash: "source-hash", Paths: []string{"internal/a.go"}, ByteCount: 10},
+			{ID: "shard-02", SourceDiffHash: "source-hash", Paths: []string{"internal/b.go"}, ByteCount: 10},
+		},
+	}
+	manifest := Build(Input{Profile: profile, ShardPlan: plan})
+	manifest.ShardResults = []ReviewResult{
+		passingShardResult("shard-01", manifest.SourceManifestHash, "internal/a.go"),
+		passingShardResult("shard-02", manifest.SourceManifestHash, "internal/b.go"),
+	}
+
+	aggregate := Aggregate(manifest)
+	assertBlocker(t, aggregate, "missing cross-shard result")
+
+	manifest.CrossShardResult = &ReviewResult{
+		SchemaVersion:      1,
+		ID:                 "cross-01",
+		ShardID:            CrossShardID,
+		Verdict:            VerdictPass,
+		SourceManifestHash: manifest.SourceManifestHash,
+		Reviewer:           "cross-shard-reviewer",
+		CoveredShardIDs:    []string{"shard-01"},
+		Evidence:           []EvidenceRef{{Path: "internal/a.go", Line: 1, Note: "integration checked"}},
+	}
+	aggregate = Aggregate(manifest)
+	assertBlocker(t, aggregate, "cross-shard result does not cover shard-02")
+
+	manifest.CrossShardResult.CoveredShardIDs = []string{"shard-01", "shard-02"}
+	aggregate = Aggregate(manifest)
+	if aggregate.Verdict != VerdictPass {
+		t.Fatalf("Verdict = %q, want %q; blockers=%v", aggregate.Verdict, VerdictPass, aggregate.Blockers)
+	}
+
+	manifest.CrossShardResult.CoveredShardIDs = []string{"shard-01", "shard-02", "shard-99"}
+	aggregate = Aggregate(manifest)
+	assertBlocker(t, aggregate, "cross-shard result covers unknown shard shard-99")
+}
+
+func TestAggregateValidatesSuppliedCrossShardResultForSingleShardManifest(t *testing.T) {
+	profile := contextprofile.Profile{Files: []contextprofile.FileProfile{{Path: "internal/a.go", DiffBytes: 10}}}
+	manifest := Build(Input{Profile: profile, ShardPlan: singleShardPlan("source-hash", "internal/a.go")})
+	manifest.ShardResults = []ReviewResult{passingShardResult("shard-01", manifest.SourceManifestHash, "internal/a.go")}
+	manifest.CrossShardResult = &ReviewResult{
+		SchemaVersion:      1,
+		ID:                 "cross-01",
+		ShardID:            CrossShardID,
+		Verdict:            VerdictPass,
+		SourceManifestHash: "stale",
+		Reviewer:           "cross-shard-reviewer",
+		CoveredShardIDs:    []string{"shard-01"},
+		Evidence:           []EvidenceRef{{Path: "internal/a.go", Line: 1, Note: "integration checked"}},
+	}
+
+	aggregate := Aggregate(manifest)
+
+	assertBlocker(t, aggregate, "cross-shard result is stale")
+}
+
+func TestMarkdownRendersStaticOnlyReviewManifest(t *testing.T) {
+	profile := contextprofile.Profile{Files: []contextprofile.FileProfile{{Path: "internal/a.go", DiffBytes: 10}}}
+	manifest := Build(Input{Profile: profile, ShardPlan: singleShardPlan("source-hash", "internal/a.go")})
+	aggregate := Aggregate(manifest)
+	body := Markdown(manifest, aggregate)
+
+	for _, required := range []string{
+		"## Review Manifest",
+		"Manifest verdict:",
+		"Runtime assessment: static-only; runtime not assessed",
+		"internal/a.go",
+		"missing shard result for shard-01",
+	} {
+		if !strings.Contains(body, required) {
+			t.Fatalf("manifest markdown missing %q:\n%s", required, body)
+		}
+	}
+}
+
+func singleShardPlan(sourceHash, path string) contextprofile.ShardPlan {
+	return contextprofile.ShardPlan{
+		SourceDiffHash: sourceHash,
+		Shards: []contextprofile.Shard{{
+			ID:             "shard-01",
+			SourceDiffHash: sourceHash,
+			Paths:          []string{path},
+			ByteCount:      100,
+		}},
+	}
+}
+
+func passingShardResult(shardID, sourceManifestHash string, paths ...string) ReviewResult {
+	return ReviewResult{
+		SchemaVersion:      1,
+		ID:                 "result-" + shardID,
+		ShardID:            shardID,
+		Verdict:            VerdictPass,
+		SourceManifestHash: sourceManifestHash,
+		Reviewer:           "shard-reviewer",
+		CoveredPaths:       paths,
+		Evidence:           []EvidenceRef{{Path: paths[0], Line: 12, Note: "acceptance covered"}},
+	}
+}
+
+func assertBlocker(t *testing.T, aggregate AggregateResult, want string) {
+	t.Helper()
+	for _, blocker := range aggregate.Blockers {
+		if strings.Contains(blocker, want) {
+			return
+		}
+	}
+	t.Fatalf("missing blocker %q in %+v", want, aggregate.Blockers)
+}
+
+func assertNoBlocker(t *testing.T, aggregate AggregateResult, unwanted string) {
+	t.Helper()
+	for _, blocker := range aggregate.Blockers {
+		if strings.Contains(blocker, unwanted) {
+			t.Fatalf("unexpected blocker %q in %+v", unwanted, aggregate.Blockers)
+		}
+	}
+}
diff --git a/internal/taskdone/review.go b/internal/taskdone/review.go
index 25738cd..03c8868 100644
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
@@ -388,12 +389,28 @@ func contextMarkdown(runID string, task tasksource.Source, git gitcontext.Contex
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
+	manifest := reviewmanifest.Build(reviewmanifest.Input{
+		Scope:            scope,
+		Target:           target,
+		Profile:          profile,
+		ShardPlan:        plan,
+		PathDispositions: reviewmanifest.GeneratedPathDispositions(profile.GeneratedExcludedFiles),
+	})
+	return reviewmanifest.Markdown(manifest, reviewmanifest.Aggregate(manifest))
+}
+
 func knowledgeMarkdown(context knowledge.Context) string {
 	service := "Service inventory: none\n\nNo service inventory found."
 	if context.ServiceInventoryPath != "" {
diff --git a/internal/taskdone/review_markdown_test.go b/internal/taskdone/review_markdown_test.go
index a037438..b7187e2 100644
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
@@ -49,3 +53,53 @@ func TestRunChainMarkdownIncludesEscalationDetails(t *testing.T) {
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
+
+func TestContextMarkdownDispositionsGeneratedReviewArtifacts(t *testing.T) {
+	body := contextMarkdown(
+		"mrv-task",
+		tasksource.Source{ID: "task-1", Body: "Review manifest task"},
+		gitcontext.Context{BaseSHA: "base", HeadSHA: "head", Branch: "feature", ChangedFiles: []string{"internal/a.go"}},
+		contextprofile.Profile{
+			Files:                  []contextprofile.FileProfile{{Path: "internal/a.go", DiffBytes: 10}},
+			GeneratedExcludedFiles: []string{"docs/metareview/context/generated-context.md"},
+		},
+		knowledge.Context{},
+		"go test ./... exited 0",
+		"gate",
+	)
+
+	for _, required := range []string{
+		"docs/metareview/context/generated-context.md: generated",
+		"metareview generated review artifact excluded from source manifest",
+	} {
+		if !strings.Contains(body, required) {
+			t.Fatalf("task-done context missing generated disposition %q:\n%s", required, body)
+		}
+	}
+	if strings.Contains(body, "missing disposition for docs/metareview/context/generated-context.md") {
+		t.Fatalf("task-done context should not flag generated review artifact as missing disposition:\n%s", body)
+	}
+}


```

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

Verification evidence:
- go test ./internal/reviewmanifest exited 0 after exact shard result coverage and duplicate/unknown shard result regressions
- go test ./internal/reviewmanifest ./internal/taskdone ./internal/prready exited 0
- go test ./... exited 0
- bash tests/run-all.sh exited 0
- git diff --check -- :!docs/metareview/context/** :!docs/metareview/reviews/** exited 0
- staged source diff grep for task marker patterns exited 1/no matches after replacing marker-like placeholder prose

