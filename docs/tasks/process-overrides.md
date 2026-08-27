# Process overrides — recording out-of-workflow escalations

A blocking finding is normally cleared by fixing it. There was no way to record the other case: the
workflow being deliberately stepped outside of. In practice that case kept arising (a review chain
exhausting its attempts overnight, a branch structurally unable to pass a gate yet), and it was
improvised by hand-editing a review log's `## Verdict` line — untracked, unattributable, and
indistinguishable afterwards from a review that genuinely passed.

This adds the exception as a first-class, analysable object.

- `internal/findings`: statuses `override-pending` and `overridden`; `RequestOverride`, `GrantOverride`,
  `ListOverrides`, `PendingOverrides`; `Blocks(status)` as the single blocking rule (`open` and
  `override-pending` block; `overridden` does not). Provenance fields record actor, timestamp, reason and
  escalation context for both halves. An override is never a fix — `fixedInRunId` stays empty — so
  post-merge learning can separate exceptions from resolutions.
- Reconciliation: a recurring fingerprint reuses its overridden record instead of spawning a duplicate,
  so a persistent condition does not need re-overriding on every run.
- `internal/reviewlog` uses the same blocking rule.
- `docs/metareview/FINDINGS.md` gains a "Process Overrides" section; an override is never silent.
- `cmd/metareview`: `override request|grant|list [--pending]`. `--pending` exits 1 while any exception is
  unacknowledged, which is the CI hook.
- `CLAUDE.md`, `AGENTS.md`, CHANGELOG document the flow and the separation of requesting from granting.

The separation is the point: requesting is available to whoever drives the run, including an orchestrating
agent; granting must come from outside the workflow. A reviewing agent never clears its own findings.

Done when `go test ./...`, `tests/run-all.sh` (including `tests/go/test-override.sh`) and `go vet` pass.
