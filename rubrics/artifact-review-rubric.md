# Artifact Review Rubric

Use this rubric for specs, plans, designs, decompositions, architecture docs, pre-mortems,
runbooks, and acceptance reports.

## Adversarial Stance

Every lens in this rubric takes an **adversarial** stance. The reviewer assumes the creator's
intent is GOOD. The reviewer is NOT hostile to the author. The reviewer IS hostile to
unexamined assumptions. The stance is: *assume there may be a fundamental mistake hiding in
this design — find it.* Each lens actively hunts for invalid assumptions, missing requirements,
unstated invariants, failure modes, race conditions, security boundaries, scaling cliffs,
operational complexity, irreversible decisions, hidden coupling, unnecessary abstractions,
cases the happy-path architecture cannot represent, simpler architectures the author prematurely
ruled out, and reasons the proposed design might be fundamentally wrong. The adversarial reviewer
is allowed to conclude the best improvement is to throw away part or all of the proposed design.

## Verdicts

- PASS: no blocking findings.
- NEEDS_REVISION: one or more blocking findings.
- ESCALATE: human decision required.
- NOT_APPLICABLE: the reviewer lens does not apply.

## Anchored Confidence & Suppression

Applies to ALL lenses.

### Anchored confidence rubric

Each finding carries a **confidence anchor** (0/25/50/75/100) and a **severity** (P0–P3):

| Anchor | Meaning |
|--------|---------|
| 100 | Verifiable from code alone — mechanical, no judgment. |
| 75 | Double-checked, will affect users. |
| 50 | Real but may be a nitpick. |
| 25 | Might be a false positive. |
| 0 | Not confident. |

| Severity | Meaning |
|----------|---------|
| P0 | Critical breakage / data loss. |
| P1 | High-impact defect. |
| P2 | Moderate. |
| P3 | Low. |

**Suppression threshold:** suppress findings below the lens's threshold (default: suppress <50
unless P0). A P0 finding is never suppressed regardless of confidence.

### What you don't flag (anti-overlap suppression)

Each lens states what it does NOT flag, to prevent overlap. Reviewers defer flagged-but-out-of-
scope items to the owning lens instead of double-reporting. Borrowed from Compound Engineering's
persona-anti-overlap pattern.

- Feasibility does NOT flag completeness of requirements or scope drift.
- Completeness does NOT flag feasibility of paths or architecture soundness.
- Scope and alignment does NOT flag completeness or architecture soundness.
- Architecture does NOT flag security vulnerabilities (defer to Security), test quality (defer to
  Testing-quality), or migration safety (defer to Data-migration).
- Intent preservation does NOT flag feasibility, completeness, scope, or architecture soundness.
- Security does NOT flag code style, architecture correctness, test quality, or migration
  safety.
- Testing-quality does NOT flag security vulns, architecture soundness, or migration safety.
- Data-migration does NOT flag security vulns, test quality, or architecture soundness.

## Required Lenses

### Feasibility

- Attack the assumption that paths, commands, dependencies, and stated prerequisites are
  correct against repository reality. Find the fabricated path, the impossible ordering, the
  missing tool, the invalid command the artifact assumes works.
- Block on fabricated paths, impossible ordering, missing tools, or invalid commands.
- Does NOT flag: whether requirements are complete (defer to Completeness); whether the
  architecture is sound (defer to Architecture).

### Completeness

- Attack the assumption that the artifact covers every requirement. Find the requirement this
  artifact silently drops — the missing acceptance criterion, the missing verification, the
  obvious edge case no section addresses.
- Block on missing acceptance criteria, missing verification, or unhandled obvious edge cases.
- Does NOT flag: whether a path is feasible (defer to Feasibility); scope drift (defer to Scope
  and alignment); architecture soundness (defer to Architecture).

### Scope And Alignment

- Attack the assumption that the artifact solves only the stated intent without unrelated
  expansion. Find the work that drifts, the under-scoping, the implementation not traceable to
  any requirement — what invariant does this NOT enforce?
- Block on scope drift, under-scoping, or implementation work not traceable to requirements.
- Does NOT flag: whether requirements are complete (defer to Completeness); whether the
  architecture is sound (defer to Architecture).

### Architecture

- Attack the assumption that the boundaries, ownership, data model, and integration shape are
  correct. Hunt for the case where each breaks. Find the fundamental mistake hiding in the
  design.
- Hunt for boundary/ownership/registry failures: parallel service paths, contradictions with
  existing architecture, duplication risk, registry impact, and integration shape that can't
  represent the real domain.
- Hunt for the wrong data structure for the operation: list for membership where a set/map gives
  O(1); nested loops over the same collection → O(n^2); repeated linear scans.
- Hunt for unbounded materialization: loading all rows into memory with no LIMIT/streaming on a
  hot path.
- Hunt for N+1 query patterns: a query inside a loop over earlier results.
- Hunt for missing schema invariants: missing FK/index/NOT NULL/UNIQUE/CHECK where the business
  rule implies them; lists jammed into one text/JSON column ("Jaywalking") instead of a
  join/child table; polymorphic `(entity_type, entity_id)` pairs that can't enforce a real FK.
- Hunt for scalability cliffs: hot paths that don't paginate or assume small N; hardcoded
  limits masking unbounded queries; adding a new type/category requires a migration (ENUM) when
  a lookup/child table would be data-driven.
- Hunt for type-clarity traps: magic strings / bare ints used as discriminators (`status =
  "open"`, `kind: 1`) scattered across the diff instead of a named enum/typed constant so
  adding a variant is compile-checked, not a find-and-replace; untyped containers (`dict`,
  `object`) where a named typed struct/record would make the shape explicit; stringly-typed data
  a typed enum would prevent from drifting. Prefer the typed form unless the diff is
  intentionally dynamic.
- Hunt for redundant derived data: a new column storing derivable data (a cached
  total/count/formatted string) with no invalidation that can drift; a value duplicated across
  two tables with no single-source-of-truth rule; god-tables/fat interfaces mixing concerns.
- Hunt for query/write inefficiency: `SELECT *` when few columns are read; non-sargable
  predicates (`DATE(col)`, `LOWER(col)`, leading-wildcard `LIKE '%x'`); queries inside loops
  instead of a batched `IN`/join; write amplification.
- Hunt for semantic-correctness failures (the principal-engineer attack): does each constraint
  enforce the REAL business invariant, or a weaker/wrong one? Under-scoped uniqueness
  (`UNIQUE(email)` on a multi-tenant table that should be `UNIQUE(org_id, email)`; or the
  reverse, a per-tenant slug that should be global); a status field conflating orthogonal facts
  (workflow stage + payment state) so a legal combination is unrepresentable; a model that can
  represent an illegal state the schema doesn't forbid (`shipped_at` AND `cancelled_at` both
  set with no `CHECK`); soft-delete columns that defeat uniqueness (a `deleted_at` +
  `UNIQUE(org_id,email)` permanently blocks re-registration — needs a partial index).
- Hunt for unguarded state transitions: a state machine enforced only in one app method a
  second caller can bypass (an `UPDATE ... SET status='active'` with no `WHERE status IN (...)`
  guard); terminal states reachable again via a bulk/admin path; effective-dated/versioned rows
  with no exclusion constraint preventing overlap or gaps; soft-delete not filtered in every
  read path (joins, aggregates, FK targets, raw SQL bypassing the ORM default scope);
  audit/history tables written out-of-transaction or skipped on batch/admin paths.
- Hunt for concurrency & consistency failures at the data layer: mutable shared records without
  optimistic-concurrency (`version`/`etag` with `WHERE version = $expected` + rowcount assert);
  read-modify-write on a balance/counter without `FOR UPDATE` or an atomic `SET x = x - $1`; a
  check-then-insert backed only by a `SELECT` (TOCTOU) instead of a real unique index; money or
  quantity stored as `float`/`REAL` instead of `NUMERIC(p,s)`/`Decimal`; non-idempotent
  handlers under redelivery (no idempotency key with a unique index).
- Hunt for coupling & evolvability traps: a current business rule baked into the schema shape so
  the next rule change forces a migration (roles as `is_admin`/`is_editor` boolean columns
  instead of a `roles`/`user_roles` table); an internal representation leaked into an API
  contract (DB ids, an enum's integer ordinal, ORM field names) so a rename is a public break.
- Hunt for LLM-specific failure modes (be most suspicious where the code looks most idiomatic):
  a cached/derived column (`*_count`, `*_total`, `last_*_at`) maintained by nothing — no
  trigger, no transactional increment, permanently 0; indexes that don't match the queries IN
  THIS diff (`INDEX(created_at)` when every query is `WHERE tenant_id=? AND status=? ORDER BY
  created_at`); typed data hidden in JSONB that is then filtered/joined/constrained
  (`metadata->>'status'`); an invented relationship plausible from training but absent in the
  domain (a `company_id` FK on a single-user product, an `ON DELETE CASCADE` that would delete
  paid invoices); docstrings describing behavior (cascade, validation, uniqueness) the code
  doesn't implement.
- Hunt for **sentinel-meaning-change**: a return value that changed meaning in this diff — a
  `null`/empty/`[]` that previously meant "nothing here" now meaning "not yet loaded" or "error
  suppressed"; a status sentinel whose semantics shifted so existing callers now misbehave.
- Hunt for **cascading-failure paths**: trace the failure propagation — when one dependency
  fails, does the design degrade gracefully or cascade? A sync call chain with no
  timeout/circuit-breaker/fallback; a queue consumer whose failure poisons the batch; a shared
  resource (cache, connection pool) whose exhaustion takes down all tenants.
- Hunt for **stand-in-guard-fidelity**: a CI gate, check, or test that can go green while
  production is red — a guard that tests a proxy/mock instead of the real code path; a check
  that passes because the production-only branch is `#ifdef`-ed out or feature-flagged away; a
  "green" build that never exercised the changed code.
- Hunt for **api-contract breaking changes**: renamed or removed fields, narrowed inputs,
  widened returns, missing versioning on breaking changes; a response shape that existing
  callers depend on but the diff silently changes; a field re-typed (int→string) with no
  version bump.
- Block on parallel service paths, contradictions with existing architecture, O(n^2) over a
  growing collection, unbounded materialization on a hot path, N+1 query loops, derivable data
  stored without invalidation, an illegal state the schema permits (no `CHECK` forbidding it),
  an unguarded state transition, a lost-update on a balance/counter, money as float, a
  phantom-maintained derived column, a sentinel-meaning-change with no caller update, a
  cascading-failure path with no degradation, a stand-in guard that can go green while prod is
  red, or an unversioned breaking API-contract change.
- Does NOT flag: security vulnerabilities (defer to Security); test quality (defer to
  Testing-quality); migration safety (defer to Data-migration).

### Intent Preservation

- Attack the assumption that the final artifact still matches the original intent and accepted
  constraints. Find where review iterations silently changed the objective without explicit
  human acceptance — what happens when the happy-path intent drifts?
- Block when review iterations changed the objective without explicit human acceptance.
- Does NOT flag: feasibility (defer to Feasibility); completeness (defer to Completeness); scope
  or architecture soundness (defer to Scope and alignment / Architecture).

### Security

- Hunt for security vulnerabilities the change introduces or fails to prevent, across the
  OWASP classes a diff-review can see: broken access control/IDOR, injection
  (SQL/NoSQL/command/deserialization), cryptographic failures/secrets, SSRF, XSS/escaping,
  insecure design, auth/session failures, deserialization/integrity, security
  misconfiguration. Use `rubrics/security-review-rubric.md`.
- Hunt for **IDOR / ownership scoping**: DB queries/lookups using a user-supplied id without an
  ownership/org/tenant scope check — `WHERE id = $user_id` with no `org_id`/tenant filter so
  any user reads any tenant's row.
- Hunt for **injection variants** beyond SQL: command injection (`exec`/`spawn` with user
  input), NoSQL injection (user-controlled operators/keys in a query document), deserialization
  injection (unvalidated `pickle`/`yaml.load`/`unserialize` of untrusted bytes into executable
  structures).
- Hunt for **SSRF protocol-bypass**: server-side fetch of unvalidated user URLs where a naive
  localhost string check (`url.includes('localhost')`) is defeated by `file://`, `gopher://`,
  `127.0.0.1` in decimal/IPv6 notation, or DNS rebinding.
- Hunt for **secrets in logs** (distinct from secrets in code): PII, tokens, or credentials
  written to log output, error messages, or telemetry — not hardcoded in source, but leaked at
  runtime through logging paths the diff adds or changes.
- Block on user-supplied-id lookups without ownership scope, string-interpolated SQL/commands,
  unvalidated deserialization of untrusted input, hardcoded secrets in committed code, secrets
  written to logs, server-side fetch of unvalidated user URLs (including protocol-bypass),
  unescaped user input to HTML/JS output, weakened token integrity/entropy. Do not double-report
  issues the deterministic gates already catch (the `eval(` gate covers bare `eval(` injection;
  flag injection the gate does not catch, e.g. SQL string interpolation).
- Does NOT flag: code style; architecture correctness (defer to Architecture); test quality
  (defer to Testing-quality); migration safety (defer to Data-migration).

### Testing-Quality

- Attack the assumption that the tests verify the behavior they claim to. The tests may lie —
  find where they do. Use `rubrics/testing-quality-rubric.md`.
- Hunt for false-confidence assertions: `toBeTruthy()`, `expect(x).toBeDefined()`, "doesn't
  throw", or `assert(x)` that assert nothing about the behavior — a test that passes regardless
  of whether the code is correct.
- Hunt for behavioral-change-in-the-diff with ZERO test modifications: new logic, changed
  branches, or modified state transitions in the diff with no corresponding test change — the
  tests are stale and will pass even though they no longer cover the new behavior.
- Hunt for tests verifying mocks not real logic: a test that asserts the mock was called with
  certain args but never checks the real return value or side effect; a test where the mock
  replaces the unit under test so thoroughly that the real code is never exercised.
- Hunt for untested new branches/lifecycle paths: a new `if`/`switch` branch, a new error path,
  a new lifecycle hook (onMount/onUnmount/beforeDestroy) with no test that triggers it.
- Hunt for sentinel-semantics reuse in mocks: a mock returning `null`/empty/`[]` that no longer
  matches what the real function returns in the new code, so the test passes against a stale
  sentinel.
- Hunt for mirror-tests-that-miss-the-machine: tests that mirror the implementation's structure
  so closely (copy the same conditional logic into the test) that they pass even when both are
  wrong — they test the code against itself, not against the spec.
- Block on a behavioral change in the diff with no test modifications, a test that asserts
  nothing (false-confidence), or a test that exercises only a mock. Do not double-report
  `eval(` or `missing-test` issues the deterministic gates already catch.
- **Missing-test ownership (precedence):** the deterministic `missing-test` gate owns the
  *boolean* "source changed, test file unchanged" (free, exact) — it fires on the absence of
  test-file changes, not on test quality. Testing-quality owns the *qualitative* "tests exist
  but don't cover the new behavior" — the gate cannot assess whether existing tests are stale,
  false-confidence, or mock-only. Completeness owns *missing verification for a requirement when
  no test code is in the diff at all*. The boundary: a changed-behavior cell with NO test file
  change -> deterministic gate (boolean); a requirement with no test code in the diff ->
  Completeness; a test that exists but doesn't verify the new behavior -> Testing-quality
  (qualitative). Do not re-report a finding another lens or gate already caught.
- Does NOT flag: security vulnerabilities (defer to Security); architecture soundness (defer to
  Architecture); migration safety (defer to Data-migration); whether tests exist at all when no
  test code is in the diff (defer to Completeness for "missing verification").

### Data-Migration

- Attack the assumption that the migration is safe and reversible. Find the failure that loses
  data or can't be rolled back. Use `rubrics/data-migration-rubric.md`.
- Hunt for **schema drift** (diff against review-base): the migration's schema changes differ
  from what the review-base schema expects; a column added/renamed/typed differently in the
  migration vs the code that reads it.
- Hunt for **irreversible migrations**: `DROP COLUMN`, `DROP TABLE`, or destructive `ALTER`
  without a backfill or a documented rollback — a migration that, once applied, cannot be
  undone without data loss.
- Hunt for **missing backfills for new NOT NULL columns**: a new `NOT NULL` column added
  without a `DEFAULT` or a backfill step, so existing rows can't be inserted/updated and the
  deploy breaks mid-rollout.
- Hunt for **deploy-window breaks (expand+contract violations)**: a contract change applied in
  one step that breaks rolling deploys — a column rename in the same PR that adds the new name,
  so old-code pods crash reading the old name; a column dropped before all readers are updated.
- Hunt for **dual-write gaps**: a migration that should dual-write old + new columns/tables but
  only writes one side, so backfill or cutover finds missing data.
- Hunt for **orphaned refs**: a FK or reference added/changed in the migration that points at
  rows that don't exist, or a FK dropped without cleaning up dangling references.
- Hunt for **silent data loss**: a migration that drops, overwrites, or truncates data without a
  backup/export step; a `DELETE` with a broader `WHERE` than intended; a column repurposed
  (same name, new meaning) so old data is silently misinterpreted.
- Block on irreversible migrations without rollback, missing backfills for NOT NULL columns,
  expand+contract violations that break rolling deploys, silent data loss, or orphaned refs.
- Does NOT flag: security vulnerabilities (defer to Security); test quality (defer to
  Testing-quality); architecture soundness beyond migration safety (defer to Architecture).

## Evidence Rules

Every blocking finding must cite at least one concrete source:

- file path and line
- command output
- artifact section
- git SHA
- Beads task ID
- review log section

Session-derived facts are hints unless the finding is about intent or process history.

## Output Structure (orchestrator notes vs findings)

The review markdown has two distinct prose sections; keep them separate:

- **`## Orchestrator Notes (not findings)`** — orchestrator context and synthesis (checkout
  sparse, filtered file-not-found artifacts, consolidation narrative, "all N lenses returned").
  This is **audit trail only**. It is NOT a finding stream. Downstream consumers (and the
  harnesseval extractor) MUST NOT extract sentences from here as review findings.
- **`## Findings`** (and the classified `## Blocking Findings` / `## Advisory Findings` /
  `## Follow-up Findings` / `## Warnings` sections) — the only source of review findings. These
  come from the LLM lens subagents. The deterministic gates (eval-injection, missing-test, etc.)
  remain `blocking` findings for metareview's own verdict; downstream eval extractors should
  skip findings sourced from `metareview-deterministic/*` and `metareview-session` (they are
  gate verdicts / orchestrator prose, not lens findings).

The orchestrator's job is to plan and aggregate, not to produce review findings. Its prose is
audit trail; the lenses produce the findings. Putting orchestrator narrative into the findings
stream adds hallucination (harnesseval adjudicates orchestrator prose at ~92% hallucination).

Write each lens's findings into the review log as that lens returns (per-lens edits), and keep the
orchestrator's final reply to the verdict plus a one-line summary — do not re-emit the findings in
the final message. On large diffs a single findings-laden message can overflow the model's
per-message output limit and truncate the review; per-lens writes avoid this (see Orchestrator
Discipline in `skills/review-artifact/SKILL.md`).
