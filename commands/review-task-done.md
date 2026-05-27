# review-task-done

Run the local task-done review gate:

```bash
metareview review task-done <task-id-or-path> [--base <ref>] [--previous-run <run-id>] [--evidence <path>]
```

Resolve blocking findings, then re-run with `--previous-run` until the generated review log reports `PASS` or `PASS_ADVISORY`.
