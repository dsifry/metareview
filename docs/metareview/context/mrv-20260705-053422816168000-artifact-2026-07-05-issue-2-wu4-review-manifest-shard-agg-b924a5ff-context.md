# metareview context: docs/superpowers/plans/2026-07-05-issue-2-wu4-review-manifest-shard-aggregation.md

Run ID: `mrv-20260705-053422816168000-artifact-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff`

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
- Define the result-ingestion boundary as deterministic data passed to the manifest builder. In this slice, task-done and pr-ready will render an empty pending-result manifest; later CLI or skill work can populate shard results from stable files or evidence records.
- Keep agent execution out of the Go CLI. The CLI should model and aggregate externally produced shard results, not spawn Codex, Claude, or other runtimes.

## Non-Goals

- Do not implement a full shard runner.
- Do not ingest GitHub review threads.
- Do not add runtime/deployment evidence.
- Do not replace existing task-done or pr-ready verdict logic in this slice. Aggregation produces a manifest verdict/status that is rendered as evidence; existing gates do not consume it as an additional blocker until a later integration slice.

## Acceptance Criteria

- A manifest records source paths, generated or out-of-scope path dispositions, the shard plan, shard review results, and an optional cross-shard result.
- The canonical coverage universe is the generated-filtered `contextprofile.Profile.Files` path set, plus explicit path dispositions for any generated or out-of-scope paths included by callers.
- Generated or out-of-scope path dispositions must include a rationale. A rationale is invalid when it is blank after trimming, shorter than 12 characters, or uses placeholder text such as `n/a`, `none`, `todo`, or `tbd`.
- Aggregation blocks when a canonical source path is assigned to zero or multiple primary shards.
- Aggregation blocks when any planned shard lacks a review result.
- Aggregation blocks when a shard result is stale for the current source diff hash.
- Aggregation blocks when a cross-shard result is stale for the current source diff hash or does not cover the current shard set.
- Shard and cross-shard results must carry evidence-backed provenance: result ID or path, reviewer/source, source diff hash, covered paths, file:line evidence or acceptance/path coverage notes, severity, and disposition.
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

- Source path coverage: canonical universe is exactly generated-filtered `contextprofile.Profile.Files`; zero assignment blocks; duplicate primary assignment blocks; generated/out-of-scope paths require valid rationale; invalid rationale placeholders block.
- Shard results: missing result blocks; stale source diff hash blocks; blocking verdict or bloc
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
