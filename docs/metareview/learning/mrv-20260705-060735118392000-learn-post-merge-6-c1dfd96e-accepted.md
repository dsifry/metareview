# metareview Accepted Learning

Run ID: `mrv-20260705-060735118392000-learn-post-merge-6-c1dfd96e`

Post-merge PR: `6`

## Source Status

- Git base: `761450000b48d1b14dfca36c0ca4d26e5dc93d41`
- Git head: `8206670cda7f3dd75f78d59718033d372944c945`
- GitHub: available
- Session history: available

## Git Diff Summary

- `docs/superpowers/plans/2026-07-05-issue-2-wu4-review-manifest-shard-aggregation.md`
- `internal/prready/review.go`
- `internal/prready/review_markdown_test.go`
- `internal/reviewmanifest/manifest.go`
- `internal/reviewmanifest/manifest_test.go`
- `internal/taskdone/review.go`
- `internal/taskdone/review_markdown_test.go`


## GitHub Context

- PR: https://github.com/dsifry/metareview/pull/6
- Title: Issue #2 WU4: add review manifest aggregation
- Body excerpt: ## Summary

- Add `internal/reviewmanifest` with a versioned static review manifest, deterministic source manifest hashing, exact source/disposition accounting, shard result aggregation, and cross-shard freshness/coverage checks.
- Render the Review Manifest into task-done and pr-ready context packs without replacing the existing gate verdicts.
- Auto-disposition generated `docs/metareview/**` artifacts so generated review evidence does not create false missing-disposition blockers.

## Validati...

Comments:
- coderabbitai https://github.com/dsifry/metareview/pull/6#issuecomment-4885041918: <!-- This is an auto-generated comment: summarize by coderabbit.ai -->
<!-- This is an auto-generated comment: rate limited by coderabbit.ai -->

> [!WARNING]
> ## Review limit reached
>
> You’ve reached a temporary PR review limit under our [Fair Usage Limits Policy](https://docs.coderabbit.ai/management/plans#fair-usage-limits-policy).<br>
> Your recent review volume is higher than typical usage, so adaptive limits are currently applied.
>
> **Next review available in:** **7 minutes**
>
> E...

## Accepted Learning

- For metareview changes that generate durable docs/metareview artifacts, stage the intended source and artifact payload before rerunning task-done or pr-ready so generated artifacts receive explicit generated dispositions and large untracked review artifacts do not create false context-risk blockers.
  - Provenance: review finding fixed in a later run
  - Confidence: high
  - Source refs: finding mrvf-20260705-054708182746000-task-done-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff-001; fixed-run mrv-20260705-055727813972000-task-done-2026-07-05-issue-2-wu4-review-manifest-shard-agg-b924a5ff

## Calibration Candidates

No reviewer calibration candidates.
