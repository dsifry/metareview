# Changelog

## 0.8.0 - 2026-08-24

0.8.0 re-stances every lens as **adversarial** rather than collaborative: each lens now assumes
there may be a fundamental mistake hiding in the design and hunts for it instead of confirming
the artifact is well-shaped. The reviewer is not hostile to the author (intent is assumed good)
but is hostile to unexamined assumptions; each lens is allowed to conclude the best improvement
is to throw away part or all of the proposed design. Two new lenses (Testing-quality and
Data-migration) take the lens set 6 -> 8, the Security and Architecture lenses gain targeted
grafts, and a new anchored confidence rubric plus a per-lens "what you don't flag" suppression
list (anti-overlap) are introduced. Driven by harnesseval eval signal and mined from metaswarm +
Compound Engineering.

### Added

- New **Testing-quality** lens for the artifact-review rubric: false-confidence tests (tests
  that pass regardless of correctness), behavioral-change-without-test-work (behavior changes
  but no test was added or updated), and mocks-not-real-logic (mocks that stub the very logic
  under test).
- New **Data-migration** lens for the artifact-review rubric: schema-drift, irreversible
  migrations, missing backfills, and dual-write gaps.
- New **anchored confidence rubric**: 0/25/50/75/100 scores paired with P0-P3 severity bands.
- New per-lens **"what you don't flag" suppression list** (anti-overlap), mined from Compound
  Engineering, so each lens explicitly records the findings it leaves to other lenses or gates.
- Security lens grafts: IDOR/ownership scoping, injection variants (command/NoSQL/
  deserialization), SSRF protocol-bypass, and secrets-in-logs.
- Architecture lens grafts: sentinel-meaning-change, cascading-failure, stand-in-guard-fidelity,
  and api-contract-breaking-change.

### Changed

- All lenses now take an **adversarial stance**: assume there may be a fundamental mistake
  hiding in the design; hunt for it; do not confirm the artifact is well-shaped. The reviewer is
  not hostile to the author (intent assumed good) but is hostile to unexamined assumptions. Each
  lens is allowed to conclude the best improvement is to throw away part or all of the proposed
  design.
- Lens count 6 -> 8 (Feasibility, Completeness, Scope-and-Alignment, Architecture,
  Intent-Preservation, Security, Testing-quality, Data-migration).

### Notes

- Mined from metaswarm + Compound Engineering. Non-diff-reviewable items (STRIDE
  threat-modeling, pnpm audit/CVE, release 7-gate pipeline, project-vertical checks,
  plan-time architect gating) are deliberately excluded.

## 0.7.0 - 2026-08-24

0.7.0 adds a dedicated **Security** lens to the artifact-review rubric and deepens the
**Architecture** lens from a boundaries/shape check into a principal-engineer data-model review
(security + semantic correctness + lifecycle + concurrency + evolvability + LLM-specific failure
modes). Driven by harnesseval eval signal (docs/METAREVIEW_IMPROVEMENTS.md H1/H1b/H1c): the
5-lens set under-recalled on security and data goldens vs a vanilla prompt, and the architecture
lens checked whether the model was tidy, not whether it was right. Lens count goes 5 -> 6.

### Added

- New **Security** lens (`rubrics/security-review-rubric.md`) for the artifact-review rubric:
  OWASP Top 10 (2021 edition) classes scoped to a diff review (broken access control/IDOR,
  injection, cryptographic failures/secrets, SSRF, XSS/escaping, insecure design, auth/session
  failures, deserialization/integrity, security misconfiguration). Covers A01-A05, A07, A08, A10
  + XSS; A06 (vulnerable components) and A09 (logging/monitoring) are deliberately excluded as
  non-diff-reviewable. Explicitly does not double-report issues the deterministic gates already
  catch (the `eval(` gate covers bare eval injection).
- Security lens added to the required-lens set in `rubrics/artifact-review-rubric.md` and
  `skills/review-artifact/SKILL.md` (now 6 required lenses: Feasibility, Completeness,
  Scope-and-Alignment, Architecture, Intent-Preservation, Security).

### Changed

- The **Architecture** lens is substantially deepened. It still checks boundaries, ownership,
  duplication, and integration shape, and now ALSO runs a principal-engineer data-model pass:
  - **Data model & data-structure design/efficiency**: wrong structure for the operation
    (list vs set/map, O(n^2) nested loops, N+1, unbounded materialization).
  - **Schema invariants**: missing FK/index/NOT NULL/UNIQUE/CHECK; lists jammed into one
    text/JSON column; polymorphic `(entity_type, entity_id)` pairs.
  - **Scalability/expandability**: hot paths that don't paginate or assume small N; hardcoded
    limits masking unbounded queries; ENUM-as-new-type requiring a migration vs a lookup table.
  - **Type clarity**: magic strings/bare ints as discriminators vs named enums/typed constants;
    untyped dict/object vs named typed structs; stringly-typed data.
  - **Redundancy**: derivable data stored as a column with no invalidation; duplicated values
    across two tables with no single source of truth; god-tables/fat interfaces.
  - **Query/write efficiency**: SELECT *; non-sargable predicates; queries in loops; write
    amplification.
  - **Semantic correctness**: under-scoped uniqueness (`UNIQUE(email)` vs `UNIQUE(org_id,email)`);
    conflated orthogonal statuses; illegal states the schema permits (no CHECK); soft-delete
    defeating uniqueness.
  - **Data lifecycle & state transitions**: state machines enforced only in one app method a
    second caller can bypass; terminal states reachable again; temporal overlap/gaps; soft-delete
    not filtered in every read path; audit tables written out-of-transaction.
  - **Concurrency at the data layer**: missing optimistic-concurrency; read-modify-write
    without FOR UPDATE; TOCTOU check-then-insert without a unique index; money/quantity as
    float not Decimal; non-idempotent handlers.
  - **Coupling & evolvability**: business rule baked into schema shape; internal representation
    leaked to an API contract; destructive migration in one step.
  - **LLM-specific failure modes**: phantom-maintained derived columns (no trigger/increment);
    indexes that don't match the queries in the diff; typed data hidden in JSONB; invented
    relationships; docstrings describing unimplemented behavior.
- New blocking criteria: O(n^2) over a growing collection, unbounded materialization on a hot
  path, N+1 query loops, derivable data without invalidation, an illegal state the schema
  permits, an unguarded state transition, a lost-update on a balance/counter, money as float,
  or a phantom-maintained derived column.

### Notes

- The Security lens and the enriched Architecture lens are mined from the harnesseval eval
  signal and a principal-engineer data-model-review research brief. The eval's confirming
  experiment (enriched vs prior lens on security/data/concurrency-bearing PRs, 50 PRs + CIs) is
  Phase C.
- The deterministic gates (`eval(`, TODO/FIXME, missing-tests, duplicate-path, truncated-diff)
  are unchanged; the Security lens explicitly avoids double-reporting gate catches.

## 0.6.0 - 2026-07-05

0.6.0 is the release that turned metareview from a basic local review gate into a more evidence-backed, stateful, and shard-aware review harness. There was no public 0.5.0 tag; these notes cover the work between `v0.4.0` and `v0.6.0`.

### Added

- Structured validation receipts with `metareview evidence run -- <command>`. Receipts preserve command, working directory, exit code, timestamps, output hashes, summary, and coverage labels so reviewers can distinguish real validation from prose.
- GitHub check import with `metareview evidence import --github-checks <pr-number> [--repo <owner/repo>]`.
- Context profiles in task-done, epic-ready, and PR-ready context packs, including raw diff bytes, filtered diff bytes, generated review-artifact exclusions, untracked-file omissions, truncation signals, and deterministic context-risk reasons.
- Context shard planning for large or risky diffs. The shard plan records source diff hashes, shard IDs, shard paths, byte counts, prompt-pack paths, and reviewer instructions for shard-local and cross-shard findings.
- Review Manifest sections in task-done and PR-ready context packs. Manifests account for reviewed source paths, generated path dispositions, shard assignments, source-manifest hashes, manifest blockers, and static runtime-assessment status.
- Review Manifest aggregation validation for stale shard hashes, missing or duplicate shard results, unknown shard IDs, incomplete cross-shard coverage, invalid evidence references, and extra or unassigned covered paths.
- PR-ready review-state projection so previous blockers are reconciled by target and run chain before a branch is blocked by older review state.
- Post-merge learning artifacts for the 0.6.0 work, including accepted learning and discarded low-value candidates.

### Changed

- `task-done` and `pr-ready` now parse structured receipts as validation evidence while still accepting freeform evidence as a fallback. `epic-ready` accepts evidence files and uses their text for child-completion evidence.
- `task-done`, `epic-ready`, and `pr-ready` fail closed when context risk is detected instead of silently treating truncated, omitted, or oversized context as a normal review surface.
- Generated `docs/metareview/**` review artifacts are filtered out of source review context and represented explicitly as generated dispositions in the Review Manifest.
- Plugin metadata and package metadata now agree on `0.6.0` across npm, Codex, Claude Code, and Go source checkout version reporting.
- Review skill assets and integration docs now prefer structured receipts and document the receipt workflow.

### Fixed

- PR-ready no longer keeps unrelated or superseded blockers alive after follow-up runs clear the relevant target.
- The release-blocking manifest version mismatch was fixed before `v0.6.0`.
- Shard and manifest validation now reports stale or incomplete review evidence explicitly so missing coverage is visible in the Review Manifest.

### Validation

The release was validated with:

- `go test ./...`
- `bash tests/run-all.sh`
- `npm pack --dry-run`
- `git diff --check`

The `metareview@0.6.0` npm package was then published manually.
