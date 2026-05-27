# learn-post-merge

Curate local post-merge review learning:

```bash
metareview learn --post-merge <pr-number> --base <pre-merge-ref> [--github-pr <number>] [--session-root <path>]
```

Review the accepted learning and discarded candidate Markdown logs before committing the generated knowledge and calibration records.

In metaswarm integration, post-merge learning is advisory by default. A caller may opt into strict mode by treating a nonzero command exit as blocking release cleanup until the learning run succeeds or is explicitly waived.
