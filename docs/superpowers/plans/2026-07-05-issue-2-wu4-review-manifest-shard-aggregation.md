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
