# metareview context: docs/superpowers/plans/2026-07-05-issue-2-wu4-review-manifest-shard-aggregation.md

Run ID: `mrv-20260705-053741420316000-artifact-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff`

## Target

- Path: `docs/superpowers/plans/2026-07-05-issue-2-wu4-review-manifest-shard-aggregation.md`
- Repository mode: `metaswarm-extension`
- Git branch: `codex/issue-2-wu4`
- Git head: `7614500`

## Artifact Excerpt

```markdown
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

```

## Service Inventory

No service inventory found.

## Knowledge Facts

No Beads knowledge facts found.

## Suggested Reviewers

- feasibility
- completeness
- scope/alignment
- architecture
- intent preservation
