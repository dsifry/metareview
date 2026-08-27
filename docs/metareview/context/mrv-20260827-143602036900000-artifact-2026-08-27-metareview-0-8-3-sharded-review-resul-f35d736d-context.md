# metareview context: docs/specs/2026-08-27-metareview-0.8.3-sharded-review-results.md

Run ID: `mrv-20260827-143602036900000-artifact-2026-08-27-metareview-0-8-3-sharded-review-resul-f35d736d`

## Target

- Path: `docs/specs/2026-08-27-metareview-0.8.3-sharded-review-results.md`
- Repository mode: `metaswarm-extension`
- Git branch: `pr-ready-shard-results`
- Git head: `61ffdf7`

## Artifact Excerpt

```markdown
# metareview 0.8.3 — sharded review results (r1)

Status: draft for adversarial review. Branch `pr-ready-shard-results` off `main` (0.8.2). Ships as **0.8.3** on
`main` so that `fsm-enhancements` (0.9.0, under manual QA) can rebase onto it and be gated honestly.

## 1. Problem

`metareview review pr-ready` (and `task-done`, `epic-ready`) cannot pass on a branch whose diff exceeds the
120 KB context cap, and the tooling that was designed to make that case reviewable is half-built:

1. `internal/gitcontext` truncates every diff at `maxDiffBytes = 120000` and sets `DiffTruncated`; the
   context profile turns that into risk reason `DIFF_TRUNCATED`; the architecture-reviewer turns any
   context risk into a **blocking** "Review context risk" finding (`internal/reviewers/taskdone.go:66`,
   `prready.go` via `branchDiffFindings`, `epicready.go:54`). Nothing downstream can clear it.
2. The Context Shard Plan is computed from the **truncated** diff: `prready.branchOnlyGitContext` sets
   `RawDiffBytes = FilteredDiffBytes = len(git.Diff)` (`internal/prready/review.go:595-596`) and
   `contextprofile.filesFromGit` sizes files by scanning `git.Diff` text. On the 0.9.0 branch (≈976 KB of
   code) the plan reported "shard-01: 120 files, 59,967 bytes" — the numbers are fiction, so the plan cannot
   be used as-is. `sourceDiffHash` (the freshness key for every shard result) is derived from the same
   fiction.
3. Every shard names a prompt pack `docs/metareview/shards/<hash>-<id>.md` that is **never written**
   (`contextprofile.PlanShards` sets the path; no writer exists).
4. `reviewmanifest.Manifest` has `ShardResults` and `CrossShardResult` with a complete, tested aggregator
   (WU4, `docs/superpowers/plans/2026-07-05-issue-2-wu4-review-manifest-shard-aggregation.md`), but no
   command populates them; the manifest always renders "missing shard result for shard-NN" and the
   manifest verdict is decorative (it is not consulted by any reviewer).
5. The deterministic lints (`eval(`, `TODO|FIXME`, inventory paths) scan `addedLines(context.Git)` — the
   truncated diff — so on a large branch they silently cover only the first 120 KB.

WU4 §Scope already set the boundary this spec keeps: *"Keep agent execution out of the Go CLI. The CLI
should model and aggregate externally produced shard results, not spawn Codex, Claude, or other
runtimes."*

## 2. Goals / non-goals

Goals: (a) true sizes and a trustworthy plan; (b) prompt packs actually written; (c) a documented, closed
result-file format the host agent writes per shard, discovered and validated deterministically; (d) the
context-risk blocker is *satisfied* — not suppressed — when the manifest aggregate passes; (e) lints cover
the whole diff; (f) an end-to-end shell test that drives a >120 KB branch from `NEEDS_REVISION` to `PASS`
with shard results and back to blocked with a stale result.

Non-goals: raising or parameterising `maxDiffBytes`; spawning reviewers from Go; changing the artifact
review flow; changing `.metareview/` state formats; `epic-ready` result ingestion (it keeps its current
blocker; it is reviewed on the parent's diff, which is the child task ranges already gated — see §9).

## 3. True sizes (`internal/gitcontext`, `internal/contextprofile`, `internal/prready`)

- `gitcontext.Context` gains `FileDiffBytes map[string]int` (branch diff, keyed by post-image path) measured
  from the **untruncated** `git diff base..HEAD` output inside `limitedGitMeasured`'s caller, using the same
  `diff --git` header walk `contextprofile.addDiffProfiles` uses today (that helper moves to `gitcontext`
  as `DiffFileBytes(text string) map[string]int`; `contextprofile` calls it). Staged, working-tree and
  untracked contributions keep their current (text-scan) measurement — they are small by construction and
  outside the branch gate.
- `contextprofile.filesFromGit` uses `git.FileDiffBytes` for the branch diff when present and falls back to
  scanning `git.Diff` only when the
```

## Service Inventory

No service inventory found.

## Knowledge Facts

No Beads knowledge facts found.

## Suggested Reviewers

- feasibility
- completeness
- scope/alignment
- architecture
- intent preservation
