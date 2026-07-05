---
name: review-pr-ready
description: Run metareview's deterministic PR-ready gate before pushing or opening a PR; checks unresolved blockers, validation evidence, branch diff risks, generated PR evidence, and optional GitHub review context.
---

# Review PR Ready

Run this before pushing a PR branch or asking external reviewers to spend time.

## Command

```bash
metareview review pr-ready [--base <ref>] [--previous-run <run-id>] [--evidence <path>] [--github-pr <number>]
```

Use `--base` for the reviewed branch diff, `--previous-run` after fixing blockers, `--evidence` for structured receipts or test output, and `--github-pr` to include available GitHub PR context.

Prefer structured evidence receipts:

```bash
go run ./cmd/metareview evidence run -- go test ./...
go run ./cmd/metareview evidence import --github-checks <pr-number>
```

Freeform evidence remains accepted as a fallback, but receipts preserve command, exit code, timestamps, and output hashes.

## Workflow

1. Run the command from the repository root.
2. If it exits `1`, open the generated review log and fix every blocking finding.
3. Re-run with `--previous-run <run-id>` until the verdict is `PASS` or `PASS_ADVISORY`.
4. Use the generated `metareview PR Evidence` section in the PR description or handoff.

GitHub context is optional in local mode. Missing `gh`, auth, remote, or PR number is recorded as unavailable context rather than a blocker.
