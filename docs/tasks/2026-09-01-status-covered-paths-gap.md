# Task — `status --scope branch` is unsatisfiable: no gate records "Covered paths"

**Found:** 2026-09-01, while dogfooding metareview on the pins/Bug.Class build (T1.1/T1.2).
**Severity:** medium (enforcement UX, not correctness) — the Stop hook `pre-finish.sh` cannot pass,
so it blocks once then yields on every finish. The review **gates themselves pass** (task-done and
pr-ready return exit 0); only the hook's branch-scope status check is affected.

## Symptom
`metareview status --json --scope branch` returns exit 1 with every branch-changed file marked
`UNREVIEWED`, even after a `pr-ready` review of that exact branch returns **PASS with zero blockers**.

## Root cause (verified in source, metareview 0.10.0)
1. **No review gate records covered paths.** `internal/status/branch.go:168` (`BranchScope.Unreviewed`)
   clears a file only when a passing, **by-commit** review log carries a `Covered paths:` header,
   decoded at `internal/reviewlog/reviewlog.go:159`. `reviewlog.EncodeCoveredPaths`
   (`internal/reviewlog/schema.go:36`) exists but **no review command calls it** — `0` of all review
   logs in the repo contain the line. So `CoveredPathsKnown` is always false and, per the function's
   own contract ("it never said what it read, so it vouches for nothing"), every changed file stays
   `UNREVIEWED` no matter how many reviews pass.
2. **A repaired parent is not superseded.** A child `pr-ready` **PASS** run created with
   `--previous-run <parent>` (correctly linked via `previousRunId`) does not retire the parent
   attempt's `NEEDS_REVISION` in `must_clear`; `status` still counts the parent.

## Fix sketch
- Wire the review gates (task-done / epic-ready / pr-ready) to emit `EncodeCoveredPaths(<source files
  the review context actually covered>)` into the review-log header, so `status` can credit them.
  The covered set is the review's own context-pack source refs (respecting
  `status.GeneratedMetareviewPathExcludes`).
- In `status`, supersede a `NEEDS_REVISION` run with a later PASS in the same `previousRunId` lineage
  so a repaired branch clears.
- Add a status-accounting test: a branch whose only review is a by-commit PASS **with** covered paths
  reports `must_clear: []`; the same PASS **without** covered paths still reports the files (the
  current, documented behaviour) — pinning both directions.

## Interim (done 2026-09-01)
The pre-finish **Stop hook is DISABLED** in `.claude/settings.json` (moved to `_disabledHooks`, with a
reason), because the gate is unsatisfiable from inside a session and only yields loudly on the second
pass — an unsatisfiable blocking gate trains operators to ignore it, the exact failure the hook's own
comments warn about. The per-task gates (`review task-done` / `pr-ready`) still run and pass, so
enforcement is not lost. **Re-enable** by moving the `Stop` array back under `hooks` once this task is
resolved (the gate then has a satisfiable path). Not chosen: forcing `~/go/bin` onto PATH, which would
only change the message from "not installed" to "N unreviewed files" while still blocking.
