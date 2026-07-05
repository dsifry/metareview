---
name: review-task-done
description: Run metareview's deterministic task-done code review gate before claiming a local task is complete; use for task closure, chunk review, or pre-PR local review when code changed.
---

# Review Task Done

Run this before saying a coding task is done.

## Command

```bash
metareview review task-done <task-id-or-path> [--base <ref>] [--previous-run <run-id>] [--evidence <path>]
```

Use `--base` to define the reviewed diff. Use `--previous-run` when re-reviewing after fixes. Use `--evidence` for validation output such as structured receipts or test logs.

Prefer structured evidence receipts:

```bash
go run ./cmd/metareview evidence run -- go test ./...
```

Freeform evidence remains accepted as a fallback, but receipts preserve command, exit code, timestamps, and output hashes.

## Workflow

1. Run the command from the repository root.
2. If it exits `1`, open the generated review log and fix every blocking finding.
3. Re-run with `--previous-run <run-id>` until the verdict is `PASS` or `PASS_ADVISORY`.
4. Do not claim task completion while unresolved blocking findings remain.

The review updates `.metareview/findings.jsonl`, `.metareview/runs.jsonl`, `docs/metareview/FINDINGS.md`, and Markdown review/context artifacts.
