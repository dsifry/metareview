# review-pr-ready

Run the local PR-ready review gate:

```bash
metareview review pr-ready [--base <ref>] [--previous-run <run-id>] [--evidence <path>] [--github-pr <number>]
```

Resolve blocking findings, then re-run with `--previous-run` until the generated review log reports `PASS` or `PASS_ADVISORY`. Use the generated `metareview PR Evidence` section when preparing the PR.
