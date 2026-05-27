# review-epic-ready

Run the local epic-ready review gate:

```bash
metareview review epic-ready <epic-id-or-path> [--base <ref>] [--previous-run <run-id>] [--evidence <path>]
```

Resolve blocking findings, then re-run with `--previous-run` until the generated review log reports `PASS` or `PASS_ADVISORY`.
