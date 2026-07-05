---
name: review-task-done
description: Run metareview's deterministic task-done code review gate before claiming a local task is complete; use for task closure, chunk review, or pre-PR local review when code changed.
---

# Review Task Done

Run this before saying a coding task is done.

## Command

```bash
metareview review task-done <task-id-or-path> [--base <ref>] [--previous-run <run-id>] [--max-attempts <n>] [--evidence <path>]
```

Use `--base` for the reviewed diff, `--previous-run` after fixes, and `--evidence` for validation output. Use `--max-attempts` only on the first run; it sets the chain budget (default 3), with the first blocker run as attempt 1.

Prefer structured evidence receipts:

```bash
go run ./cmd/metareview evidence run -- go test ./...
```

Freeform evidence remains accepted as a fallback, but receipts preserve command, exit code, timestamps, and output hashes.

## Workflow

1. Run the command from the repository root.
2. Exit handling: `0` means verify `PASS`/`PASS_ADVISORY` with zero blockers; `1` with a review path means follow that log; nonzero without a path means read stderr.
3. `NEEDS_REVISION`: fix blockers and re-run with `--previous-run <run-id>`.
4. `ESCALATED`: stop same-target retries; human must narrow, split, or redesign the target.

The review updates `.metareview/findings.jsonl`, `.metareview/runs.jsonl`, `docs/metareview/FINDINGS.md`, and Markdown review/context artifacts.
