# Task-Done Review Rubric

Use this rubric for `metareview review task-done <task-id-or-path>`.

## Blocking Policy

Critical, high, and spec-contract findings block task closure.

Block when:

- The diff introduces unsafe execution such as `eval`.
- Source changes lack relevant tests or explicit validation evidence.
- The diff adds `TODO` or `FIXME` markers.
- Diff context is truncated.
- The change appears to duplicate an inventoried service or code path.
- Unsafe untracked source files are present.

Advisory (does not block): orphaned stale-head findings the ledger accumulated across heads are surfaced as a
`generated-artifact-hygiene-reviewer` advisory (R5) and partitioned into the committed `FINDINGS.md` "Stale"
section. Re-review at the current head or clear the cross-head ledger.

## Pass Policy

Pass only when the current review has no blocking findings and prior blocking findings for the previous run have been fixed or otherwise resolved.

In advisory mode, use `PASS_ADVISORY` when validation evidence is present but the repository is not using Beads or metaswarm.
