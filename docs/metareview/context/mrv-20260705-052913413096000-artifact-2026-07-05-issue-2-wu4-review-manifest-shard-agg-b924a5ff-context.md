# metareview context: docs/superpowers/plans/2026-07-05-issue-2-wu4-review-manifest-shard-aggregation.md

Run ID: `mrv-20260705-052913413096000-artifact-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff`

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
- Keep agent execution out of the Go CLI. The CLI should model and aggregate externally produced shard results, not spawn Codex, Claude, or other runtimes.

## Non-Goals

- Do not implement a full shard runner.
- Do not ingest GitHub review threads.
- Do not add runtime/deployment evidence.
- Do not replace existing task-done or pr-ready verdict logic in this slice.

## Acceptance Criteria

- A manifest records source paths, generated or out-of-scope path dispositions, the shard plan, shard review results, and an optional cross-shard result.
- Aggregation blocks when a changed source path is assigned to zero or multiple primary shards.
- Aggregation blocks when any planned shard lacks a review result.
- Aggregation blocks when a shard result is stale for the current source diff hash.
- Aggregation blocks when any shard result or cross-shard result has blockers or a blocking verdict.
- Aggregation requires a cross-shard review for multi-shard manifests.
- Aggregation passes when every primary shard has a fresh passing result and the required cross-shard review passes.
- Task-done and pr-ready context packs include a readable Review Manifest section.

## Validation

- `go test ./...`
- `bash tests/run-all.sh`
- `git diff --check -- ':!docs/metareview/context/**' ':!docs/metareview/reviews/**'`
- `go run ./cmd/metareview review task-done issue-2-wu4 --base origin/main --evidence <evidence-file>`
- `go run ./cmd/metareview review pr-ready --base origin/main --evidence <evidence-file>`

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
