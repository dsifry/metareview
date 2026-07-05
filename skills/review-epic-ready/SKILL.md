---
name: review-epic-ready
description: Run metareview's deterministic epic-ready gate before closing or landing an epic; checks child review evidence, unresolved blockers, integration contradictions, and intent drift.
---

# Review Epic Ready

Run this before declaring an epic ready to land.

## Command

```bash
metareview review epic-ready <epic-id-or-path> [--base <ref>] [--previous-run <run-id>] [--max-attempts <n>] [--evidence <path>]
```

Use `--base` for the reviewed diff, `--previous-run` after fixes, and `--evidence` for validation or acceptance notes. Use `--max-attempts` only on the first run; it sets the chain budget (default 3), with the first blocker run as attempt 1.

## Workflow

1. Run the command from the repository root.
2. Exit handling: `0` means verify `PASS`/`PASS_ADVISORY` with zero blockers; `1` with a review path means follow that log; nonzero without a path means read stderr.
3. `NEEDS_REVISION`: fix blockers and re-run with `--previous-run <run-id>`.
4. `ESCALATED`: stop same-target retries; human must narrow, split, or redesign the target.
5. After a passing verdict, re-check that the final result still satisfies the original epic intent.

The review updates `.metareview/findings.jsonl`, `.metareview/runs.jsonl`, `docs/metareview/FINDINGS.md`, and Markdown review/context artifacts.
