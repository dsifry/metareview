# metareview Agent Instructions

These instructions apply to coding agents working in this repository or in repositories that install metareview.

## Required Review Gates

Run metareview before claiming completion:

- Specs, plans, designs, decompositions, and docs: `metareview review artifact <path>`.
- Local task or chunk complete: `metareview review task-done <task-id-or-path> --base <base-ref> --evidence <file>`.
- Epic or parent task ready to land: `metareview review epic-ready <epic-id-or-path>`.
- PR ready to push or merge: `metareview review pr-ready --base <base-ref>`.
- After PR merge: `metareview learn --post-merge <pr-number> --base <pre-merge-ref>`.

Use `go run ./cmd/metareview ...` when running from a source checkout without a built `bin/metareview`.

## Blocker Policy

Do not claim completion while any blocking finding remains open.

Exit handling: `0` means verify `PASS`/`PASS_ADVISORY` with zero blockers; `1` with a review path means follow that log; nonzero without a path means read stderr.

Lifecycle gate verdicts have this contract:

- `PASS`: proceed.
- `PASS_ADVISORY`: proceed only when the review explicitly reports zero blocking findings.
- `NEEDS_REVISION`: fix blockers, then re-run the same gate with `--previous-run <run-id>`.
- `ESCALATED`: stop same-target retries; human must narrow, split, or redesign the target.

Advisory notes can be recorded for later, but blockers are current work.

## Evidence Policy

For task-done and pr-ready reviews, provide evidence:

```bash
tmp_evidence="$(mktemp)"
printf '%s\n' \
  'Verification evidence:' \
  '- go test ./... exited 0' \
  '- bash tests/run-all.sh exited 0' \
  '- git diff --check exited 0' > "$tmp_evidence"
metareview review task-done <task-id-or-path> --base <base-ref> --evidence "$tmp_evidence"
```

Evidence must reflect commands actually run in the current workspace.

## Artifact Policy

Commit durable artifacts:

- `docs/metareview/reviews/`
- `docs/metareview/context/`
- `docs/metareview/learning/`
- `.beads/knowledge/metareview.jsonl` when Beads owns knowledge
- `.metareview/knowledge/metareview.jsonl` in standalone fallback mode
- `.metareview/calibration.jsonl`
- `.metareview/learning-runs.jsonl`

Keep transient state local:

- `.metareview/findings.jsonl`
- `.metareview/runs.jsonl`
- generated binaries such as `bin/metareview`

## Metaswarm Fit

When metaswarm is present, it remains the lifecycle owner. Follow metaswarm's decomposition, Superpowers, Beads, and PR shepherding process, and insert metareview as the deeper review harness at artifact, task-done, epic-ready, pr-ready, and post-merge checkpoints.

When metaswarm is absent, use `metareview setup --bootstrap-prereqs --dry-run` before proposing local prerequisites or registries such as `docs/SERVICE_INVENTORY.md`.
