# Task-Done Review Rubric

Use this rubric for `metareview review task-done <task-id-or-path>`.

## Blocking Policy

Critical, high, and spec-contract findings block task closure.

Block when:

- The diff introduces unsafe execution such as `eval`.
- Source changes lack relevant tests or explicit validation evidence. A claim that tests are
  absent must be verified, not assumed: search the diff (and the repo, with your tools) for
  the test files the claim says do not exist — `spec/**`, `test/**`, `tests/**`,
  `__tests__/**`, `*.test.*`, `*.spec.*`, `*_test.go`, `test_*.py` — and cite the specific
  assertion gap when a candidate test is found. A "no tests" claim contradicted by a test in
  the same diff is a fabricated finding, not a blocker.
- The diff adds `TODO` or `FIXME` markers.
- Diff context is truncated.
- The change appears to duplicate an inventoried service or code path.
- Unsafe untracked source files are present.

## Pass Policy

Pass only when the current review has no blocking findings and prior blocking findings for the previous run have been fixed or otherwise resolved.

In advisory mode, use `PASS_ADVISORY` when validation evidence is present but the repository is not using Beads or metaswarm.
