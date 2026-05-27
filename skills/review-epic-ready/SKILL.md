---
name: review-epic-ready
description: Run metareview's deterministic epic-ready gate before closing or landing an epic; checks child review evidence, unresolved blockers, integration contradictions, and intent drift.
---

# Review Epic Ready

Run this before declaring an epic ready to land.

## Command

```bash
metareview review epic-ready <epic-id-or-path> [--base <ref>] [--previous-run <run-id>] [--evidence <path>]
```

Use `--base` for the reviewed diff, `--previous-run` after fixing blockers, and `--evidence` for validation or acceptance notes.

## Workflow

1. Run the command from the repository root.
2. If it exits `1`, open the generated review log and fix every blocking finding.
3. Re-run with `--previous-run <run-id>` until the verdict is `PASS` or `PASS_ADVISORY`.
4. Re-check that the final result still satisfies the original epic intent.

The review updates `.metareview/findings.jsonl`, `.metareview/runs.jsonl`, `docs/metareview/FINDINGS.md`, and Markdown review/context artifacts.
