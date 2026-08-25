# Data-Migration Review Rubric

Use this rubric for the **Data-migration** lens of an artifact/code review. The Data-migration
lens attacks the assumption that the migration is safe and reversible. Migrations can lose
data, break rolling deploys, or create states that can't be rolled back. This is a *diff-scoped*
lens: judge whether the migration changes in this diff are safe against the review-base schema,
not whether the entire database is well-modeled (that is Architecture's job).

The adversarial stance: assume there may be a fundamental mistake hiding in the migration —
find it. Do not confirm the migration is well-shaped; hunt for the failure that loses data or
can't be rolled back.

## Verdicts

- PASS: no blocking data-migration findings.
- NEEDS_REVISION: one or more blocking data-migration findings (irreversible migrations,
  missing backfills, expand+contract violations, silent data loss).
- ESCALATE: a migration's safety depends on runtime/deploy context the diff can't settle
  (concurrent writer behavior, zero-downtime window constraints).
- NOT_APPLICABLE: the diff touches no schema, migration, or data-persistence changes. State
  the surface you checked and found absent.

## What To Hunt For

For each category, find the case where the migration breaks. Report each distinct issue with
file:line and the migration code. Only flag issues you are confident are real migration-safety
defects in THIS diff, not generic "migrations are risky" advice.

### Schema Drift (Diff Against Review-Base)
- The migration's schema changes differ from what the review-base schema expects: a column
  added/renamed/typed differently in the migration vs the code that reads it.
- A model/ORM definition updated but the migration not generated (or vice versa) — the code and
  the DB schema disagree.
- Block on schema drift between the migration and the code that reads/writes the schema.

### Irreversible Migrations
- `DROP COLUMN`, `DROP TABLE`, or destructive `ALTER` (type narrowing, `SET NOT NULL` without a
  default) without a backfill or a documented rollback.
- A migration that, once applied, cannot be undone without data loss — the forward path
  destroys data the backward path needs.
- Block on irreversible migrations without a documented rollback or backfill.

### Missing Backfills For New NOT NULL Columns
- A new `NOT NULL` column added without a `DEFAULT` or a backfill step, so existing rows can't be
  inserted/updated and the deploy breaks mid-rollout.
- A backfill that runs after the code expects the column to be populated (deploy-order break).
- Block on new NOT NULL columns without a default or backfill.

### Deploy-Window Breaks (Expand+Contract Violations)
- A contract change applied in one step that breaks rolling deploys: a column rename in the
  same PR that adds the new name, so old-code pods crash reading the old name; a column dropped
  before all readers are updated.
- Expand+contract violations: skipping the expand phase (add new column) or the contract phase
  (dual-read/dual-write) or the cleanup phase (drop old) — compressing the safe sequence into
  one step.
- Block on expand+contract violations that break rolling deploys.

### Dual-Write Gaps
- A migration that should dual-write old + new columns/tables but only writes one side, so
  backfill or cutover finds missing data.
- A cutover that reads from the new column before all rows are backfilled from the old column.
- Block on dual-write gaps that leave data on one side unpopulated.

### Orphaned Refs
- A FK or reference added/changed in the migration that points at rows that don't exist, or a
  FK dropped without cleaning up dangling references.
- A renamed/removed referenced row with no cascade or cleanup for dependents.
- Block on orphaned refs or dangling references introduced by the migration.

### Silent Data Loss
- A migration that drops, overwrites, or truncates data without a backup/export step.
- A `DELETE` with a broader `WHERE` than intended; a column repurposed (same name, new meaning)
  so old data is silently misinterpreted.
- A type conversion (`ALTER COLUMN ... TYPE`) that narrows or truncates (varchar→int,
  text→varchar(10)) and silently drops values that don't fit.
- Block on silent data loss with no backup, rollback, or validation step.

## What NOT To Flag (Anti-Overlap)

- Do NOT flag security vulnerabilities (defer to Security).
- Do NOT flag test quality (defer to Testing-quality).
- Do NOT flag architecture soundness beyond migration safety — data-model correctness,
  semantic validity, concurrency, and coupling are Architecture's job. This lens judges only
  whether the *migration itself* is safe and reversible.
- Do NOT flag whether the schema is well-designed (defer to Architecture); this lens judges
  whether the *transition* from the old schema to the new one is safe.

## Evidence Rules

Every blocking finding must cite the migration code (file:line + the verbatim SQL/DDL or
migration function that makes the finding true — the "quote-the-line" gate) and state the
failure mode (what breaks / what data is lost / why it can't roll back), not just "this
migration is risky." If the diff's stated intent is to change data shape, judge whether the
migration safely transitions from the old shape to the new one.
