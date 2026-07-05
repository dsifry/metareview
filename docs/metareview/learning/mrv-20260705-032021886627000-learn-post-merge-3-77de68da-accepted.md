# metareview Accepted Learning

Run ID: `mrv-20260705-032021886627000-learn-post-merge-3-77de68da`

Post-merge PR: `3`

## Source Status

- Git base: `afdda501dd2196197d5417dc0418aee55acdef3b`
- Git head: `e12cc97a6399bc2ddfdbf5daeff8a5a6ad65ab4b`
- GitHub: available
- Session history: available

## Git Diff Summary

- `.claude-plugin/marketplace.json`
- `.claude-plugin/plugin.json`
- `.codex-plugin/plugin.json`
- `internal/prready/review.go`
- `internal/reviewlog/reviewlog.go`
- `internal/reviewlog/reviewlog_test.go`
- `internal/reviewstate/projector.go`
- `internal/reviewstate/projector_test.go`
- `internal/version/version.go`
- `package.json`
- `tests/go/test-pr-ready-review.sh`


## GitHub Context

- PR: https://github.com/dsifry/metareview/pull/3
- Title: [codex] Fix PR-ready review state projection and 0.6.0 metadata
- Body excerpt: ## Summary

This PR implements WU1 for issue #2: PR-ready review state projection now scopes historical review findings by validated run chains, branch/path target relevance, and changed-path overlap so stale or unrelated metareview artifacts do not block current PR readiness.

It also fixes the manifest/version drift that was blocking `tests/run-all.sh` by aligning the package, Go binary version, Codex plugin manifest, Claude plugin manifest, and Claude marketplace entry to `0.6.0`.

## Changes...

Comments:
- coderabbitai https://github.com/dsifry/metareview/pull/3#issuecomment-4884664899: <!-- This is an auto-generated comment: summarize by coderabbit.ai -->
<!-- This is an auto-generated comment: review in progress by coderabbit.ai -->

> [!NOTE]
> Currently processing new changes in this PR. This may take a few minutes, please wait...
> 
> <details>
> <summary>⚙️ Run configuration</summary>
> 
> **Configuration used**: Organization UI
> 
> **Review profile**: ASSERTIVE
> 
> **Plan**: Pro
> 
> **Run ID**: `ce1de8bf-52f3-4710-83f6-1cd0958248fe`
> 
> </details>
> 
> <details>
> <s...

## Accepted Learning

No accepted learning candidates.

## Calibration Candidates

No reviewer calibration candidates.
