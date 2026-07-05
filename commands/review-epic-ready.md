# review-epic-ready

Run the local epic-ready review gate:

```bash
metareview review epic-ready <epic-id-or-path> [--base <ref>] [--previous-run <run-id>] [--max-attempts <n>] [--evidence <path>]
```

Exit handling: `0` means verify `PASS`/`PASS_ADVISORY` with zero blockers; `1` with a review path means follow that log; nonzero without a path means read stderr. `NEEDS_REVISION` means fix blockers and rerun with `--previous-run`; `ESCALATED` means stop same-target retries and ask the human to narrow, split, or redesign.
