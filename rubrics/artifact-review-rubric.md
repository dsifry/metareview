# Artifact Review Rubric

Use this rubric for specs, plans, designs, decompositions, architecture docs, pre-mortems, runbooks, and acceptance reports.

## Verdicts

- PASS: no blocking findings.
- NEEDS_REVISION: one or more blocking findings.
- ESCALATE: human decision required.
- NOT_APPLICABLE: the reviewer lens does not apply.

## Required Lenses

### Feasibility

- Verify paths, commands, dependencies, and stated prerequisites against repository reality.
- Block on fabricated paths, impossible ordering, missing tools, or invalid commands.

### Completeness

- Map user/spec requirements to artifact sections.
- Block on missing acceptance criteria, missing verification, or unhandled obvious edge cases.

### Scope And Alignment

- Check whether the artifact solves the stated intent without unrelated expansion.
- Block on scope drift, under-scoping, or implementation work not traceable to requirements.

### Architecture

- Check boundaries, ownership, duplication risk, registry impact, and integration shape.
- Check the data model and data-structure design: wrong structure for the operation (list for
  membership where a set/map gives O(1); nested loops over the same collection -> O(n^2);
  repeated linear scans), unbounded materialization (loading all rows into memory with no
  LIMIT/streaming), N+1 query patterns (a query inside a loop over earlier results).
- Check schema invariants: missing FK/index/NOT NULL/UNIQUE/CHECK where the business rule implies
  them; lists jammed into one text/JSON column ("Jaywalking") instead of a join/child table;
  polymorphic `(entity_type, entity_id)` pairs that can't enforce a real FK.
- Check scalability & expandability: hot paths that don't paginate or assume small N; hardcoded
  limits masking unbounded queries; adding a new type/category requires a migration (ENUM) when a
  lookup/child table would be data-driven.
- Check type clarity: magic strings / bare ints used as discriminators (`status = "open"`,
  `kind: 1`) scattered across the diff instead of a named enum/typed constant so adding a variant
  is compile-checked, not a find-and-replace; untyped containers (`dict`, `object`) where a named
  typed struct/record would make the shape explicit; stringly-typed data that a typed enum would
  prevent from drifting. Prefer the typed form unless the diff is intentionally dynamic.
- Check redundancy: a new column storing derivable data (a cached total/count/formatted string)
  with no invalidation that can drift; a value duplicated across two tables with no
  single-source-of-truth rule; god-tables/fat interfaces mixing concerns.
- Check query/write efficiency: `SELECT *` when few columns are read; non-sargable predicates
  (`DATE(col)`, `LOWER(col)`, leading-wildcard `LIKE '%x'`); queries inside loops instead of a
  batched `IN`/join; write amplification.
- Check semantic correctness of the model (the principal-engineer pass): does each constraint
  enforce the REAL business invariant, or a weaker/wrong one? Under-scoped uniqueness
  (`UNIQUE(email)` on a multi-tenant table that should be `UNIQUE(org_id, email)`; or the
  reverse, a per-tenant slug that should be global); a status field conflating orthogonal facts
  (workflow stage + payment state) so a legal combination is unrepresentable; a model that can
  represent an illegal/illegal state the schema doesn't forbid (`shipped_at` AND `cancelled_at`
  both set with no `CHECK`); soft-delete columns that defeat uniqueness (a `deleted_at` +
  `UNIQUE(org_id,email)` permanently blocks re-registration — needs a partial index).
- Check data lifecycle & state transitions: a state machine enforced only in one app method a
  second caller can bypass (an `UPDATE ... SET status='active'` with no `WHERE status IN (...)`
  guard); terminal states reachable again via a bulk/admin path; effective-dated/versioned rows
  with no exclusion constraint preventing overlap or gaps; soft-delete not filtered in every read
  path (joins, aggregates, FK targets, raw SQL bypassing the ORM default scope); audit/history
  tables written out-of-transaction or skipped on batch/admin paths.
- Check concurrency & consistency at the data layer: mutable shared records without
  optimistic-concurrency (`version`/`etag` with `WHERE version = $expected` + rowcount assert);
  read-modify-write on a balance/counter without `FOR UPDATE` or an atomic `SET x = x - $1`; a
  check-then-insert backed only by a `SELECT` (TOCTOU) instead of a real unique index; money or
  quantity stored as `float`/`REAL` instead of `NUMERIC(p,s)`/`Decimal`; non-idempotent handlers
  under redelivery (no idempotency key with a unique index).
- Check coupling & evolvability: a current business rule baked into the schema shape so the next
  rule change forces a migration (roles as `is_admin`/`is_editor` boolean columns instead of a
  `roles`/`user_roles` table); an internal representation leaked into an API contract (DB ids,
  an enum's integer ordinal, ORM field names) so a rename is a public break; a destructive
  migration in one step (drop old column in the same PR that adds the new one) with rolling
  deploy in flight.
- Check LLM-specific failure modes (be most suspicious where the code looks most idiomatic): a
  cached/derived column (`*_count`, `*_total`, `last_*_at`) maintained by nothing — no trigger,
  no transactional increment, permanently 0; indexes that don't match the queries IN THIS diff
  (`INDEX(created_at)` when every query is `WHERE tenant_id=? AND status=? ORDER BY created_at`);
  typed data hidden in JSONB that is then filtered/joined/constrained (`metadata->>'status'`);
  an invented relationship plausible from training but absent in the domain (a `company_id` FK
  on a single-user product, an `ON DELETE CASCADE` that would delete paid invoices); docstrings
  describing behavior (cascade, validation, uniqueness) the code doesn't implement.
- Block on parallel service paths, contradictions with existing architecture, O(n^2) over a
  growing collection, unbounded materialization on a hot path, N+1 query loops, derivable
  data stored without invalidation, an illegal state the schema permits (no `CHECK` forbidding
  it), an unguarded state transition, a lost-update on a balance/counter, money as float, or
  a phantom-maintained derived column.

### Intent Preservation

- Compare final artifact direction against original intent and accepted constraints.
- Block when review iterations changed the objective without explicit human acceptance.

### Security

- Hunt for security vulnerabilities the change introduces or fails to prevent, across the
  OWASP classes a diff-review can see: broken access control/IDOR, injection (SQL/NoSQL/command),
  cryptographic failures/secrets, SSRF, XSS/escaping, insecure design, auth/session failures,
  deserialization/integrity, security misconfiguration. Use `rubrics/security-review-rubric.md`.
- Block on user-supplied-id lookups without ownership scope, string-interpolated SQL/commands,
  hardcoded secrets in committed code, server-side fetch of unvalidated user URLs, unescaped
  user input to HTML/JS output, weakened token integrity/entropy. Do not double-report issues
  the deterministic gates already catch (the `eval(` gate covers bare `eval(` injection).

## Evidence Rules

Every blocking finding must cite at least one concrete source:

- file path and line
- command output
- artifact section
- git SHA
- Beads task ID
- review log section

Session-derived facts are hints unless the finding is about intent or process history.
