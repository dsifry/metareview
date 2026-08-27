# Changelog

## 0.8.3 - 2026-08-27

0.8.3 makes a branch too large for the review context reviewable. metareview measures the real,
untruncated branch diff, cuts it into content-stable shards, writes a prompt pack per shard, and
then accepts the result files a reviewing agent writes about those packs. When every shard of the
current plan has a fresh passing result, the context-risk blocker that nothing could clear becomes
advisory and the deterministic lints run over the whole branch diff.

Result files are evidence the reviewing agent writes about its own work, exactly like `--evidence`.
metareview checks that a result is about the current content and that every shard is covered. It
does not try to prove a review happened, and it does not defend against a hostile branch.

### Added

- **Process overrides.** `metareview override request|grant|list` records that the review workflow was
  deliberately stepped outside of. Requesting is available to whoever drives the run (an orchestrating
  agent included) and does not clear the gate — the finding keeps blocking and `override list --pending`
  exits nonzero, so CI stays red until an authority outside the workflow grants it. Both halves record
  actor, timestamp and reason; overrides render under "Process Overrides" in `docs/metareview/FINDINGS.md`
  and never read as fixes (`fixedInRunId` stays empty), so exceptions can be analysed separately from
  resolutions. The actor that requested an override cannot also grant it; `--by` is audit metadata rather
  than authentication, so environments that need a hard boundary should gate `override grant` behind
  whatever authenticates their actors.

- **Measured branch diff.** Per-file sizes come from the untruncated, exclude-filtered diff, so the
  shard plan reports real bytes instead of the truncated fiction it reported before.
- **Content-stable shard plan.** Paths are bucketed by hash and an over-budget bucket is split
  locally, so editing one file re-cuts only that file's shard. New risk reasons
  `LOCAL_DIFF_TRUNCATED` and `DIFF_OVERSIZE`.
- **Prompt packs** under the transient `.metareview/shards/<scope>/<target-slug>/<planHash>/`:
  `shard-<id>.md`, `cross-shard.md` and a `plan.json` carrying everything a host needs to write
  conforming results.
- **Review results.** Result files live in the durable
  `docs/metareview/shards/<scope>/<target-slug>/` as `shard-<id>.<shardHash>.result.json` and
  `cross-shard.<planHash>.result.json`, and are committed with the review log. `--shard-result`
  (repeatable) and `--cross-shard-result` add files explicitly on `review pr-ready` and
  `review task-done`; an invalid path exits 2 with nothing written.
- **Sharded gate.** With risk reasons limited to `DIFF_TRUNCATED`/`LARGE_DIFF`, every shard covered,
  a cross-shard result for a multi-shard plan and a passing manifest, "Review context risk" becomes
  the advisory `architecture:context-risk-covered` and "Diff context was truncated" becomes advisory
  too. The review log gains a `## Sharded Review` section listing what was ingested, what was
  ignored and why, and which files were reviewed as chunks.

### Changed

- Freshness is by content hash, and a result that matches nothing is ignored with a reason rather
  than blocking: there is no "stale" category.
- The context-risk fingerprint is reason-independent in all three scopes; the reasons moved into
  the finding's `Found`.
- The review manifest hash is the plan hash, so generated paths and dispositions no longer churn it.
- `learn-post-merge` excludes `docs/metareview/shards/**`; the JSONL readers accept 1 MiB lines.
- `epic-ready` renders "not sharded" where it printed a shard plan computed from the truncated diff.

### Upgrade

- On the first 0.8.3 run for a target, an open finding carrying a legacy reason-bearing context-risk
  fingerprint is marked `superseded` — neither open nor fixed, with `fixedInRunId` left empty — after
  `.metareview/findings.jsonl` is backed up once. No other fingerprint changes.
- CI cannot produce results itself, so a sharded gate is run by the operator's agent. Moving `--base`
  changes every hash, so all committed results for that target become ignored and the loop is re-run.
- Superseded result files are collected after a passing gate, so the audit record covers the passing
  plan rather than every plan the branch passed through.

## 0.8.2 - 2026-08-26

0.8.2 adds **orchestrator discipline** guidance to the review-artifact skill: the orchestrator
(the host agent running the workflow) is a thin dispatcher and aggregator, and the lenses do the
analysis. Three efficiencies that cut orchestrator cost without affecting lens recall or
precision.

### Added

- **Be terse.** The orchestrator runs the commands, dispatches the lenses, writes the review log,
  and returns the verdict — no planning or progress narration. (Orchestrator output tokens.)
- **Trust the lenses; do not re-verify.** After the lenses return, the orchestrator writes each
  lens's findings and verdict into the review log and aggregates the verdict. It does not re-read
  files, re-run `git diff`, or re-check lens findings — the lenses already did that work.
  (Orchestrator turns.)
- **Keep the aggregation small.** Each lens's findings are written to the review log as that lens
  returns (per-lens edits), not held in one large final write; the orchestrator's final reply is
  the verdict plus a one-line summary, not a re-emission of the findings. On large diffs a single
  findings-laden message can overflow the model's per-message output limit and truncate the
  review; per-lens writes avoid this.

### Fixed

- `.claude-plugin/marketplace.json` plugin version had drifted to 0.6.0 while `package.json`
  advanced to 0.8.0; resynced to 0.8.2 (the manifest version-consistency test now passes).

### Notes

- Validated in the harnesseval lab (branch `mrv-0.8.1-slim-orchestration`). The 0.8.1 experiment
  bundled a fourth fix (embed the diff in the prompt and forbid lens file-exploration) that cut
  tokens further but cost recall — it lost a golden that requires surrounding-file context the
  embedded diff did not show — so 0.8.2 reverts it: lenses keep file access, as in 0.8.0. 0.8.2
  keeps the three orchestrator-side fixes above, which reduced orchestrator tokens with no
  recall/precision regression.
- The lens set (8 adversarial lenses), rubric content, and deterministic gates are unchanged from
  0.8.0. The only change to the Go binary is the reported version string; behavior is identical.

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
