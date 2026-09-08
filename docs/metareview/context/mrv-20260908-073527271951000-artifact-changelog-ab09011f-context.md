# metareview context: CHANGELOG.md

Run ID: `mrv-20260908-073527271951000-artifact-changelog-ab09011f`

## Target

- Path: `CHANGELOG.md`
- Repository mode: `metaswarm-extension`
- Git branch: `lens-upgrade-runtime-reliability`
- Git head: `c02e26e`

## Artifact Excerpt

```markdown
# Changelog

## Unreleased

### Added

- **Runtime-reliability lens (10th required artifact-review lens, benchmark-driven).** Artifact
  review now runs a tenth lens that attacks the assumption that every runtime failure path in the
  diff is handled, observable, and honest. It owns the finding classes the harnesseval
  head-to-head vs. Compound Engineering showed metareview's lens taxonomy structurally missing
  (~90 of 241 CE-only confirmed findings across three clusters): **unhandled async failure**
  (`.then` without `.catch`, unreturned inner promises, fire-and-forget refreshes — the
  model-independent `destroyRecord` blind spot was CE-only across 6 different models),
  **optimistic-state desync** (state mutated before the request resolves, no in-flight guard,
  out-of-order responses, offset bookkeeping committed early), **silent partial success** (work
  skipped while the API returns success — unknown IDs dropped, throttle keys committed before the
  operation succeeds), **outbound-call hardening** (no timeout on `open(url)`/`fetch`, unbounded
  payloads, missing rate limits), **error-shape leakage** (a raw 500 where the contract promises a
  4xx), and **cross-boundary credential/token lifecycle** (a sync-mode response that can never
  satisfy the provider's refresh schema; a client rebuilt from the pre-refresh token). The lens
  set is still enumerated once in `internal/lens`; the lens-era table freezes the prior nine-lens
  rubric at its 2026-08-31 date so reviews written before this addition stay judged against the
  nine they were required to cover. Anti-overlap: Architecture keeps design-level failure
  *propagation shape* (its cascading-failure/sentinel hunts); Runtime-reliability owns concrete
  error-path handling in this diff's code. Security, Testing-quality, Data-migration, and
  Completeness's "does NOT flag" lines now defer runtime error-path findings here. Deliberately
  NOT added (handoff §4): style/deprecation/dedup hunting — 5 of 10 external-reviewer misses live
  there and suppressing them is precision working as designed — and no separate api-contract
  lens (Architecture's hunt was broadened instead).

- **Testing-gap claims are now verified against the repository head, not just the diff (issue #146).**
  The #140 mechanism (PR #145) only searched covering-test evidence among added diff lines, so a
  covering test outside every changed hunk was invisible and a false absence claim could be confirmed.
  The adjudicator now also searches the repository at the pinned head: `claimcheck.SubjectTokens`
  exports the subject-token logic, `judge.RepoTestEvidence` runs a two-stage grep-then-read search
  over injectable git seams (`judge.GrepSeam`/`ShowSeam` — the single implementation of the pinned-rev
  contract shared with the escalation sandbox and the eval), and `ContextForGapClaim` injects the
  candidates' line-capped content under a provenance-checked disclosure. A completed-but-empty search
  is disclosed as the search record a testing-gap confirmation must cite; a failed or skipped search
  stays silent and is counted as `repo_search_errors` in the claimcheck rollup — an infrastructure
  failure never reads as evidence of absence. The prompt criterion demands that search record before
  a gap claim is confirmable. `cmd/claimcheck-eval -repos <dir>` adds the repo-side structural pass
  over the harnesseval corpus (combined diff×repo matrix, no-clone/repo-error rows disclosed); the
  A/B re-judge against the v2 ground truth needs model spend and stays open in the issue.
- **`cmd/claimcheck-ab`, the A/B re-judge driver for #146.** Re-judges the harnesseval gap-claim
  corpus twice with one judge model — arm A replays the #145-era prompt (diff-only context, the
  pre-#146 `RubricAddendum` verbatim from git history), arm B runs the production render (repo-side
  evidence + the current criterion) — and scores both arms against the v2 three-way ground truth
  (`readjudication3.json`), rep
```

## Service Inventory

No service inventory found.

## Knowledge Facts

No Beads knowledge facts found.

## Suggested Reviewers

- Feasibility
- Completeness
- Scope and alignment
- Architecture
- Intent preservation
- Security
- Testing-quality
- Data-migration
- Runtime-reliability
- Mechanical-precision
