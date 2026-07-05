---
name: review-pr-ready
description: Run metareview's deterministic PR-ready gate before pushing or opening a PR; checks unresolved blockers, validation evidence, branch diff risks, generated PR evidence, and optional GitHub review context.
---

# Review PR Ready

Run this before pushing a PR branch or asking external reviewers to spend time.

## Command

```bash
metareview review pr-ready [--base <ref>] [--previous-run <run-id>] [--max-attempts <n>] [--evidence <path>] [--github-pr <number>] [--include-working-tree]
```

Use `--base` for the reviewed branch diff, `--previous-run` after fixes, and `--evidence` for validation output. Use `--max-attempts` only on the first run; it sets the chain budget (default 3), with the first blocker run as attempt 1. Use `--github-pr` to include available GitHub PR context. By default, PR-ready reviews the committed branch diff and blocks on non-generated working-tree changes; use `--include-working-tree` only when those changes intentionally belong to the review.

Prefer structured evidence receipts:

```bash
go run ./cmd/metareview evidence run -- go test ./...
go run ./cmd/metareview evidence import --github-checks <pr-number>
```

Freeform evidence remains accepted as a fallback, but receipts preserve command, exit code, timestamps, and output hashes.

## Workflow

1. Run the command from the repository root.
2. Exit handling: `0` means verify `PASS`/`PASS_ADVISORY` with zero blockers; `1` with a review path means follow that log; nonzero without a path means read stderr.
3. `NEEDS_REVISION`: fix blockers and re-run with `--previous-run <run-id>`.
4. `ESCALATED`: stop same-target retries; human must narrow, split, or redesign the target.
5. After a passing verdict, use the generated `metareview PR Evidence` section in the PR description or handoff.

GitHub context is optional in local mode. Missing `gh`, auth, remote, or PR number is recorded as unavailable context rather than a blocker.
